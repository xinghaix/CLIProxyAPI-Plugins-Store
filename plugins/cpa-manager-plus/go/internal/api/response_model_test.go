package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
)

func TestUsagePayloadResponseModelReachesAPIWithoutChangingCost(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir() + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()
	response := Handle(ctx, runtime, []byte(`{"method":"PUT","path":"/v0/management/model-prices","body":{"prices":{"billed":{"prompt":2},"upstream":{"prompt":99}}}}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("prices: %s", response.Body)
	}
	for _, payload := range []string{
		`{"Model":"billed","Alias":"billed","ResponseModel":" upstream ","RequestedAt":"2026-01-01T00:00:00Z","Detail":{"InputTokens":1000000,"TotalTokens":1000000}}`,
		`{"Model":"billed","Alias":"requested","response_model":"upstream","RequestedAt":"2026-01-01T00:00:01Z","Detail":{"InputTokens":1000000,"TotalTokens":1000000}}`,
		`{"Model":"billed","RequestedAt":"2026-01-01T00:00:02Z","Detail":{"InputTokens":1000000,"TotalTokens":1000000}}`,
	} {
		if err := runtime.HandleUsagePayload([]byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.Health(ctx)["event_count"] != int64(3) {
		if time.Now().After(deadline) {
			t.Fatal("usage not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	response = Handle(ctx, runtime, []byte(`{"method":"POST","path":"/v0/management/monitoring/analytics","body":{"from_ms":0,"to_ms":4102444800000,"include":{"events_page":{"limit":50},"granularity":"hour"}}}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("analytics: %s", response.Body)
	}
	var body struct {
		Events struct {
			Items []struct {
				Model          string  `json:"model"`
				RequestedModel string  `json:"requested_model"`
				ResolvedModel  string  `json:"resolved_model"`
				ResponseModel  string  `json:"response_model"`
				Cost           float64 `json:"cost"`
			} `json:"items"`
		} `json:"events"`
	}
	if err := json.Unmarshal(response.Body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Events.Items) != 3 {
		t.Fatalf("body: %s", response.Body)
	}
	for i, want := range []string{"", "upstream", "upstream"} {
		item := body.Events.Items[i]
		if item.Model != "billed" || item.ResolvedModel != "billed" || item.ResponseModel != want || item.Cost != 2 {
			t.Fatalf("item=%#v", item)
		}
	}
	if item := body.Events.Items[2]; item.RequestedModel != item.Model || item.ResponseModel == item.Model {
		t.Fatalf("same requested/billed with distinct response lost: %#v", item)
	}
}
