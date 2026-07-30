package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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

func TestUsageServiceConfigIgnoresRedactedManagementKeyAndPersistsCodex(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	if err := runtime.UpdateConnection(context.Background(), "http://127.0.0.1:8317", "secret-key"); err != nil {
		t.Fatal(err)
	}

	// Legacy/redacted echo must not 400 or clear the saved key.
	response := Handle(context.Background(), runtime, []byte(`{"method":"PUT","path":"/usage-service/config","body":{"cpaConnection":{"cpaBaseUrl":"http://127.0.0.1:8317","managementKey":false},"codexInspection":{"enabled":true,"schedule":{"mode":"interval","intervalMinutes":30,"timePoints":[],"timeZone":""},"targetType":"codex","workers":4,"deleteWorkers":4,"timeout":15000,"retries":0,"userAgent":"test-agent","usedPercentThreshold":90,"sampleSize":0,"autoActionMode":"none"}}}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("put redacted key = %d: %s", response.StatusCode, response.Body)
	}
	base, hasKey := runtime.Connection()
	if base != "http://127.0.0.1:8317" || !hasKey {
		t.Fatalf("connection after redacted put = %q %v", base, hasKey)
	}
	settings := runtime.CodexInspectionSettings()
	if !settings.Enabled || settings.Schedule.IntervalMinutes != 30 || settings.UsedPercentThreshold != 90 {
		t.Fatalf("settings after put = %#v", settings)
	}

	get := Handle(context.Background(), runtime, []byte(`{"method":"GET","path":"/usage-service/config"}`))
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get config = %d: %s", get.StatusCode, get.Body)
	}
	var payload map[string]any
	if err := json.Unmarshal(get.Body, &payload); err != nil {
		t.Fatal(err)
	}
	cfg := payload["config"].(map[string]any)
	conn := cfg["cpaConnection"].(map[string]any)
	if _, exists := conn["managementKey"]; exists {
		t.Fatalf("GET must not expose managementKey: %#v", conn)
	}
	if conn["hasManagementKey"] != true {
		t.Fatalf("hasManagementKey = %#v", conn["hasManagementKey"])
	}
	inspection := cfg["codexInspection"].(map[string]any)
	schedule := inspection["schedule"].(map[string]any)
	if inspection["enabled"] != true || schedule["intervalMinutes"] != float64(30) {
		t.Fatalf("GET codexInspection = %#v", inspection)
	}

	// Invalid inspection config still returns a clear validation error.
	bad := Handle(context.Background(), runtime, []byte(`{"method":"PUT","path":"/usage-service/config","body":{"config":{"codexInspection":{"enabled":true,"schedule":{"mode":"interval","intervalMinutes":0,"timePoints":[]},"workers":0,"deleteWorkers":1,"timeout":1,"retries":0,"usedPercentThreshold":10,"sampleSize":0,"autoActionMode":"none"}}}}`))
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid codex put = %d: %s", bad.StatusCode, bad.Body)
	}
}

func TestUsageServiceConfigAcceptsLegacyFlatCodexSettings(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	response := Handle(context.Background(), runtime, []byte(`{"method":"PUT","path":"/usage-service/config","body":{"config":{"codexInspection":{"enabled":true,"scheduleMode":"interval","intervalMinutes":30,"autoActionMode":"disable"}}}}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("put legacy codex settings = %d: %s", response.StatusCode, response.Body)
	}
	settings := runtime.CodexInspectionSettings()
	if !settings.Enabled || settings.Schedule.Mode != "interval" || settings.Schedule.IntervalMinutes != 30 || settings.AutoActionMode != "disable" {
		t.Fatalf("legacy settings = %#v", settings)
	}
	if settings.Workers < 1 || settings.DeleteWorkers < 1 || settings.Timeout < 1 {
		t.Fatalf("legacy settings must seed newer required fields: %#v", settings)
	}
}

func TestDeleteModelPrice(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()

	put := Handle(ctx, runtime, []byte(`{"method":"PUT","path":"/v0/management/model-prices","body":{"prices":{"gpt-test":{"prompt":1,"completion":2},"acme/claude":{"prompt":3,"completion":4}}}}`))
	if put.StatusCode != http.StatusOK {
		t.Fatalf("put prices = %d: %s", put.StatusCode, put.Body)
	}
	deleteResponse := Handle(ctx, runtime, []byte(`{"method":"DELETE","path":"/v0/management/model-prices","query":"model=gpt-test"}`))
	if deleteResponse.StatusCode != http.StatusOK || !strings.Contains(string(deleteResponse.Body), `"deleted":true`) {
		t.Fatalf("delete model = %d: %s", deleteResponse.StatusCode, deleteResponse.Body)
	}
	get := Handle(ctx, runtime, []byte(`{"method":"GET","path":"/v0/management/model-prices"}`))
	var payload struct {
		Prices map[string]any `json:"prices"`
	}
	if err := json.Unmarshal(get.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload.Prices["gpt-test"]; ok {
		t.Fatalf("deleted price remained: %#v", payload.Prices)
	}
	if _, ok := payload.Prices["acme/claude"]; !ok {
		t.Fatalf("unrelated price was removed: %#v", payload.Prices)
	}

	repeat := Handle(ctx, runtime, []byte(`{"method":"DELETE","path":"/v0/management/model-prices","query":"model=gpt-test"}`))
	if repeat.StatusCode != http.StatusOK || !strings.Contains(string(repeat.Body), `"deleted":false`) {
		t.Fatalf("repeat delete = %d: %s", repeat.StatusCode, repeat.Body)
	}
	slash := Handle(ctx, runtime, []byte(`{"method":"DELETE","path":"/v0/management/model-prices","query":"model=acme%2Fclaude"}`))
	if slash.StatusCode != http.StatusOK || !strings.Contains(string(slash.Body), `"deleted":true`) {
		t.Fatalf("delete slash model = %d: %s", slash.StatusCode, slash.Body)
	}
	missing := Handle(ctx, runtime, []byte(`{"method":"DELETE","path":"/v0/management/model-prices"}`))
	if missing.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing model = %d: %s", missing.StatusCode, missing.Body)
	}
}

func TestAutoBanSettingsAndRulesRoutes(t *testing.T) {
	ctx := context.Background()
	runtime, err := app.New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()

	get := Handle(ctx, runtime, []byte(`{"method":"GET","path":"/v0/management/auto-ban/settings"}`))
	if get.StatusCode != http.StatusOK || !strings.Contains(string(get.Body), `"enabled":false`) {
		t.Fatalf("get auto-ban settings = %d: %s", get.StatusCode, get.Body)
	}
	put := Handle(ctx, runtime, []byte(`{"method":"PUT","path":"/v0/management/auto-ban/settings","body":{"enabled":true,"sources":{"usage":true,"inspection":true},"schedulerIntervalSeconds":30,"defaultCodexCooldownHours":6,"historyRetentionDays":90,"dryRun":false}}`))
	if put.StatusCode != http.StatusOK || !strings.Contains(string(put.Body), `"defaultCodexCooldownHours":6`) {
		t.Fatalf("put auto-ban settings = %d: %s", put.StatusCode, put.Body)
	}

	create := Handle(ctx, runtime, []byte(`{"method":"POST","path":"/v0/management/auto-ban/rules","body":{"enabled":true,"priority":90,"name":"codex-418-review","providerScope":"codex","accountKind":"oauth_auth_file","matchStatusCodes":[418],"matchErrorKinds":[],"matchBodySubstrings":[],"sourceMask":1,"thresholdMode":"consecutive","thresholdCount":1,"successResetsConsecutive":true,"action":"review","cooldownSource":"header_or_default","respectHostCooldown":false}}`))
	if create.StatusCode != http.StatusCreated {
		t.Fatalf("create auto-ban rule = %d: %s", create.StatusCode, create.Body)
	}
	list := Handle(ctx, runtime, []byte(`{"method":"GET","path":"/v0/management/auto-ban/rules"}`))
	if list.StatusCode != http.StatusOK || !strings.Contains(string(list.Body), "codex-418-review") {
		t.Fatalf("list auto-ban rules = %d: %s", list.StatusCode, list.Body)
	}
}
