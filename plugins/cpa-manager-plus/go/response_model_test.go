package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestUsageMethodPreservesResponseModel(t *testing.T) {
	stopRuntime()
	t.Cleanup(stopRuntime)
	config, err := json.Marshal(lifecycleRequest{ConfigYAML: []byte("data_dir: " + t.TempDir() + "\nbatch_size: 1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := configure(config); err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"Model":"billed","Alias":"requested","ResponseModel":"upstream","RequestedAt":"2026-01-01T00:00:00Z","Detail":{"TotalTokens":8}}`)
	if _, err := handleMethod(pluginabi.MethodUsageHandle, payload); err != nil {
		t.Fatal(err)
	}
	runtime := currentRuntime()
	deadline := time.Now().Add(3 * time.Second)
	for runtime.Health(context.Background())["event_count"] != int64(1) {
		if time.Now().After(deadline) {
			t.Fatal("usage not committed")
		}
		time.Sleep(10 * time.Millisecond)
	}
	result, err := runtime.Store().Analytics(context.Background(), store.AnalyticsRequest{ToMS: 4102444800000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	items := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(items) != 1 || items[0]["response_model"] != "upstream" || items[0]["model"] != "billed" || items[0]["requested_model"] != "requested" {
		t.Fatalf("items=%#v", items)
	}
	if _, err := handleMethod(pluginabi.MethodUsageHandle, []byte(`{"Model":"billed","ResponseModel":42}`)); err == nil {
		t.Fatal("invalid response metadata accepted")
	}
}
