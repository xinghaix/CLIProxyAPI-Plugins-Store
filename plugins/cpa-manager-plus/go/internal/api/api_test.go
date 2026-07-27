package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
)

func TestLocalDispatcherStoresAndQueriesPrices(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir() + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	response := Handle(context.Background(), runtime, []byte(`{"method":"PUT","path":"/v0/management/model-prices","body":{"prices":{"gpt-test":{"prompt":2,"completion":3,"cache":1}}}}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("put price = %d: %s", response.StatusCode, response.Body)
	}
	response = Handle(context.Background(), runtime, []byte(`{"method":"GET","path":"/v0/management/model-prices"}`))
	if response.StatusCode != http.StatusOK || !json.Valid(response.Body) || string(response.Body) == "{}" {
		t.Fatalf("get price = %d: %s", response.StatusCode, response.Body)
	}
	runtime.HandleUsage(pluginapi.UsageRecord{Model: "gpt-test", RequestedAt: time.Now(), Detail: pluginapi.UsageDetail{InputTokens: 1_000_000, TotalTokens: 1_000_000}})
	deadline := time.Now().Add(time.Second)
	for {
		health := runtime.Health(context.Background())
		if health["event_count"] == int64(1) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("event was not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	response = Handle(context.Background(), runtime, []byte(`{"method":"POST","path":"/v0/management/monitoring/analytics","body":{"from_ms":0,"to_ms":4102444800000,"include":{"events_page":{"limit":50},"granularity":"hour"}}}`))
	if response.StatusCode != http.StatusOK || !json.Valid(response.Body) {
		t.Fatalf("analytics = %d: %s", response.StatusCode, response.Body)
	}
}

func TestCandidateActionPath(t *testing.T) {
	id, action, ok := candidateAction("/v0/management/account-action-candidates/12/auth-file")
	if !ok || id != 12 || action != "delete" {
		t.Fatalf("candidate action = %d %s %t", id, action, ok)
	}
	if _, _, ok := candidateAction("/v0/management/account-action-candidates/12/delete"); ok {
		t.Fatal("legacy delete path must remain rejected")
	}
}
