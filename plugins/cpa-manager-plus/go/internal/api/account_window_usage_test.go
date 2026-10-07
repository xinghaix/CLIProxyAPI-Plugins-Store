package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestAccountWindowWireModelScope(t *testing.T) {
	runtime, err := app.New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()
	if _, err := runtime.Store().InsertEvents(ctx, []store.Event{
		{Hash: "opus", TimestampMS: 10, Provider: "claude", AuthID: "opaque-id", Model: "claude-opus-4-6", TotalTokens: 100},
		{Hash: "sonnet", TimestampMS: 20, Provider: "claude", AuthID: "opaque-id", Model: "claude-sonnet-4-6", TotalTokens: 200},
	}); err != nil {
		t.Fatal(err)
	}
	response := Handle(ctx, runtime, []byte(`{"method":"POST","path":"/v0/management/monitoring/account-window-usage","body":{"windows":[{"row_key":"a","provider_window_id":"claude-seven-day-opus","period":"current","from_ms":10,"to_ms":30,"auth_index":"new-index","auth_id":"opaque-id","auth_file_snapshot":"a.json","source":"/auth/a.json","auth_provider_snapshot":"claude","model_scope_models":["claude-opus-4-6"]},{"row_key":"a","provider_window_id":"claude-seven-day-sonnet","period":"current","from_ms":10,"to_ms":30,"auth_id":"opaque-id","auth_provider_snapshot":"claude","model_scope_models":["claude-sonnet-4-6"]},{"row_key":"a","provider_window_id":"claude-seven-day-opus","period":"current","from_ms":10,"to_ms":30,"auth_id":"opaque-id","auth_provider_snapshot":"claude"}]}}`))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response=%d %s", response.StatusCode, response.Body)
	}
	var result struct {
		Items []store.AccountWindowUsageItem `json:"items"`
	}
	if err := json.Unmarshal(response.Body, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items=%s", response.Body)
	}
	if result.Items[0].TotalTokens != 100 || result.Items[1].TotalTokens != 200 || result.Items[2].ScopeMatchStatus != "unknown" || result.Items[2].Matched || result.Items[2].CostComplete {
		t.Fatalf("wire scope=%s", response.Body)
	}
	for _, models := range []string{`[""]`, `["  "]`, `[" claude-opus-4-6 "]`, `"claude-opus-4-6"`} {
		raw := `{"method":"POST","path":"/v0/management/monitoring/account-window-usage","body":{"windows":[{"row_key":"a","provider_window_id":"history","period":"current","from_ms":10,"to_ms":30,"auth_index":"idx","auth_provider_snapshot":"claude","model_scope_models":` + models + `}]}}`
		if got := Handle(ctx, runtime, []byte(raw)); got.StatusCode != http.StatusBadRequest {
			t.Errorf("invalid scope accepted: %s => %d %s", models, got.StatusCode, got.Body)
		}
	}
}
