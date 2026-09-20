package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestObservationCapabilitiesAndPassThrough(t *testing.T) {
	stopRuntime()
	caps := pluginRegistration().Capabilities
	if !caps.ResponseBeforeTranslator || !caps.ResponseInterceptor || !caps.StreamChunkInterceptor || !caps.UsagePlugin {
		t.Fatalf("capabilities=%#v", caps)
	}
	for _, method := range []string{pluginabi.MethodResponseNormalizeBefore, pluginabi.MethodResponseInterceptAfter, pluginabi.MethodResponseInterceptStreamChunk} {
		for _, body := range [][]byte{[]byte("{"), []byte("null"), []byte("{}")} {
			raw, err := handleMethod(method, body)
			if err != nil {
				t.Fatal(err)
			}
			var env envelope
			if err := json.Unmarshal(raw, &env); err != nil || !env.OK {
				t.Fatalf("envelope=%s err=%v", raw, err)
			}
			var modifications struct {
				Body         []byte
				Headers      http.Header
				ClearHeaders []string
				DropChunk    bool
			}
			if err := json.Unmarshal(env.Result, &modifications); err != nil {
				t.Fatal(err)
			}
			if len(modifications.Body) != 0 || len(modifications.Headers) != 0 || len(modifications.ClearHeaders) != 0 || modifications.DropChunk {
				t.Fatalf("observer mutated response: %s", env.Result)
			}
		}
	}
}

func TestUninspectableABIRevokesPreviouslyObservedModel(t *testing.T) {
	for _, method := range []string{pluginabi.MethodResponseNormalizeBefore, pluginabi.MethodResponseInterceptAfter, pluginabi.MethodResponseInterceptStreamChunk} {
		for _, large := range []bool{false, true} {
			t.Run(method+map[bool]string{true: "/oversize", false: "/malformed"}[large], func(t *testing.T) {
				stopRuntime()
				t.Cleanup(stopRuntime)
				dir := t.TempDir()
				cfg, _ := json.Marshal(lifecycleRequest{ConfigYAML: []byte("data_dir: " + dir)})
				if err := configure(cfg); err != nil {
					t.Fatal(err)
				}
				runtime := currentRuntime()
				db := runtime.Store()
				ctx := context.Background()
				if _, err := db.InsertEvents(ctx, []store.Event{{Hash: "event", Model: "routed", ResponseModel: "host", TimestampMS: 1, ResponseCorrelationKey: "key"}}); err != nil {
					t.Fatal(err)
				}
				if err := db.RecordResponseObservation(ctx, "key", "host", "evidence", false); err != nil {
					t.Fatal(err)
				}
				bad := []byte("{")
				if large {
					bad = make([]byte, (8<<20)+1)
				}
				reply, err := handleMethod(method, bad)
				if err != nil {
					t.Fatal(err)
				}
				var env envelope
				if err = json.Unmarshal(reply, &env); err != nil || !env.OK {
					t.Fatalf("request not passed through: %s", reply)
				}
				var mod pluginapi.StreamChunkInterceptResponse
				if err = json.Unmarshal(env.Result, &mod); err != nil || len(mod.Body) > 0 || len(mod.Headers) > 0 || mod.DropChunk {
					t.Fatalf("response was modified: %s", env.Result)
				}
				observer := runtime.Health(ctx)["response_observer"].(map[string]any)
				if observer["disabled_until_restart"] != true || observer["last_error"] == "" {
					t.Fatalf("gap not visible: %#v", observer)
				}
				result, err := db.Analytics(ctx, store.AnalyticsRequest{ToMS: 100, Limit: 10})
				if err != nil {
					t.Fatal(err)
				}
				row := result["events"].(map[string]any)["items"].([]map[string]any)[0]
				if row["response_model"] != "host" || row["response_model_source"] != "host" || row["observed_response_model"] != "" || row["response_model_conflict"] != true {
					t.Fatalf("evidence not suppressed: %#v", row)
				}
				stopRuntime()
				reopened, err := store.Open(ctx, dir)
				if err != nil {
					t.Fatal(err)
				}
				defer reopened.Close()
				result, err = reopened.Analytics(ctx, store.AnalyticsRequest{ToMS: 100, Limit: 10})
				if err != nil {
					t.Fatal(err)
				}
				row = result["events"].(map[string]any)["items"].([]map[string]any)[0]
				if row["response_model_conflict"] != true || row["observed_response_model"] != "" {
					t.Fatalf("quarantine lost on reload: %#v", row)
				}
			})
		}
	}
}

func TestDualResponseModelABI(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, usageFirst := range []bool{false, true} {
			name := "http"
			if stream {
				name = "sse"
			}
			if usageFirst {
				name += "/usage-first"
			} else {
				name += "/observer-first"
			}
			t.Run(name, func(t *testing.T) {
				stopRuntime()
				t.Cleanup(stopRuntime)
				cfg, _ := json.Marshal(lifecycleRequest{ConfigYAML: []byte("data_dir: " + t.TempDir() + "\nbatch_size: 1")})
				if err := configure(cfg); err != nil {
					t.Fatal(err)
				}
				runtime := currentRuntime()
				if err := runtime.Store().ReplacePrices(context.Background(), map[string]store.Price{"routed": {Prompt: 2}, "upstream-version": {Prompt: 99}}); err != nil {
					t.Fatal(err)
				}
				original := []byte(`{"model":"alias","messages":[]}`)
				headers := http.Header{"X-Request-Id": []string{"native-attempt-123456"}}
				metadata := map[string]any{"selected_auth_id": "credential.json"}
				send := func(method string, value any) {
					t.Helper()
					payload, err := json.Marshal(value)
					if err != nil {
						t.Fatal(err)
					}
					raw, err := handleMethod(method, payload)
					if err != nil {
						t.Fatal(err)
					}
					var env envelope
					if err = json.Unmarshal(raw, &env); err != nil || !env.OK {
						t.Fatalf("%s: %s %v", method, raw, err)
					}
				}
				usage := func() {
					send(pluginabi.MethodUsageHandle, pluginapi.UsageRecord{Model: "routed", Alias: "alias", AuthID: "credential.json", Provider: "antigravity", RequestedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), ResponseHeaders: headers, Detail: pluginapi.UsageDetail{InputTokens: 1000000, TotalTokens: 1000000}})
				}
				observe := func() {
					send(pluginabi.MethodResponseNormalizeBefore, pluginapi.ResponseTransformRequest{FromFormat: "antigravity", ToFormat: "openai", Stream: stream, OriginalRequest: original, Body: []byte(`{"response":{"modelVersion":"upstream-version","responseId":"upstream-id-123456","candidates":[]}}`)})
					if stream {
						send(pluginabi.MethodResponseInterceptStreamChunk, pluginapi.StreamChunkInterceptRequest{RequestID: "execution-1", SourceFormat: "openai", OriginalRequest: original, ResponseHeaders: headers, Metadata: metadata, ChunkIndex: -1})
						send(pluginabi.MethodResponseInterceptStreamChunk, pluginapi.StreamChunkInterceptRequest{RequestID: "execution-1", SourceFormat: "openai", ChunkIndex: 0, Body: []byte(`data: {"id":"upstream-id-123456","model":"rewritten-alias"}

`)})
					} else {
						send(pluginabi.MethodResponseInterceptAfter, pluginapi.ResponseInterceptRequest{RequestID: "execution-1", SourceFormat: "openai", OriginalRequest: original, ResponseHeaders: headers, Metadata: metadata, StatusCode: 200, Body: []byte(`{"id":"upstream-id-123456","model":"rewritten-alias"}`)})
					}
				}
				if usageFirst {
					usage()
					observe()
				} else {
					observe()
					usage()
				}
				deadline := time.Now().Add(3 * time.Second)
				for {
					result, err := runtime.Store().Analytics(context.Background(), store.AnalyticsRequest{ToMS: 4102444800000, Limit: 10})
					if err != nil {
						t.Fatal(err)
					}
					items := result["events"].(map[string]any)["items"].([]map[string]any)
					if len(items) == 1 && items[0]["response_model"] == "upstream-version" {
						row := items[0]
						if row["model"] != "routed" || row["resolved_model"] != "routed" || row["requested_model"] != "alias" || row["cost"] != float64(2) || row["response_model_source"] != "observer" {
							t.Fatalf("unexpected row=%#v", row)
						}
						break
					}
					if time.Now().After(deadline) {
						t.Fatalf("observation not linked: %#v health=%#v", items, runtime.Health(context.Background()))
					}
					time.Sleep(10 * time.Millisecond)
				}
			})
		}
	}
}
