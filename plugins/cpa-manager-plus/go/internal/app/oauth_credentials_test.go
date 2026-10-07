package app

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestOAuthCredentialEnrichmentAuthTypeIsolation(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	r := &Runtime{store: st}
	r.SetAuthList(func() ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{
			{AuthIndex: "oauth-index", ID: "oauth-id", Name: "shared.json", Provider: "codex", AccountType: "oauth2"},
			{AuthIndex: "api-index", ID: "api-id", Name: "shared.json", Provider: "codex", AccountType: "api_key"},
		}, nil
	})
	now := time.Now().UnixMilli()
	if _, err := st.InsertEvents(ctx, []store.Event{
		{Hash: "oauth", TimestampMS: now - 3000, Model: "test-model", AuthIndex: "oauth-index", AuthType: "oauth2", Provider: "codex", TotalTokens: 11},
		{Hash: "api-weak", TimestampMS: now - 2000, Model: "test-model", Source: "shared.json", AuthType: "api_key", Provider: "codex", TotalTokens: 23, Failed: true},
		{Hash: "api-conflicting-strong", TimestampMS: now - 1000, Model: "test-model", AuthIndex: "oauth-index", Source: "shared.json", AuthType: "api-key", Provider: "codex", TotalTokens: 99, Failed: true},
		{Hash: "unknown-weak", TimestampMS: now, Model: "test-model", Source: "shared.json", Provider: "codex", TotalTokens: 999, Failed: true},
	}); err != nil {
		t.Fatal(err)
	}
	run, err := st.StartInspection(ctx, "manual")
	if err != nil {
		t.Fatal(err)
	}
	for _, cache := range []store.InspectionResult{
		{AccountKey: "oauth", AuthIndex: "oauth-index", AuthID: "oauth-id", FileName: "shared.json", Provider: "codex", AuthType: "oauth2", PlanType: "oauth-plan", QuotaWindows: []string{"oauth-window"}},
		{AccountKey: "api-weak", FileName: "shared.json", Provider: "codex", AuthType: "api", PlanType: "api-plan", QuotaWindows: []string{"api-window"}},
		{AccountKey: "api-conflicting-strong", AuthIndex: "oauth-index", FileName: "shared.json", Provider: "codex", AuthType: "apikey", PlanType: "wrong-plan", QuotaWindows: []string{"wrong-window"}},
		{AccountKey: "unknown-weak", FileName: "shared.json", Provider: "codex", PlanType: "unknown-plan", QuotaWindows: []string{"unknown-window"}},
	} {
		cache.RunID = run.ID
		if _, err := st.InsertInspectionResult(ctx, cache); err != nil {
			t.Fatal(err)
		}
	}
	for _, includeAPIKeys := range []bool{false, true} {
		got, err := r.ListOAuthCredentials(ctx, includeAPIKeys)
		if err != nil {
			t.Fatal(err)
		}
		wantLen := 1
		if includeAPIKeys {
			wantLen = 2
		}
		if len(got) != wantLen {
			t.Fatalf("includeAPIKeys=%v: got %d credentials", includeAPIKeys, len(got))
		}
		for _, c := range got {
			wantTokens, wantPlan, wantWindow, wantStatus := int64(11), "oauth-plan", "oauth-window", "ok"
			if c.AuthType == "apikey" {
				wantTokens, wantPlan, wantWindow, wantStatus = 23, "api-plan", "api-window", "fail"
			}
			if c.History == nil || c.History.Requests != 1 || c.History.Tokens != wantTokens {
				t.Errorf("includeAPIKeys=%v %s history = %+v", includeAPIKeys, c.AuthType, c.History)
			}
			if c.PlanLabel != wantPlan || !reflect.DeepEqual(c.QuotaWindows, []any{wantWindow}) {
				t.Errorf("includeAPIKeys=%v %s cache = %q %#v", includeAPIKeys, c.AuthType, c.PlanLabel, c.QuotaWindows)
			}
			wantStatuses := make([]string, 8)
			wantStatuses[7] = wantStatus
			if !reflect.DeepEqual(c.RecentStatuses, wantStatuses) {
				t.Errorf("includeAPIKeys=%v %s statuses = %v", includeAPIKeys, c.AuthType, c.RecentStatuses)
			}
		}
	}
}
