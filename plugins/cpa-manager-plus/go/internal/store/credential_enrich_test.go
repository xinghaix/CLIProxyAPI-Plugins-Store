package store

import (
	"context"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

func TestEnrichCredentials(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().UnixMilli()
	from := now - 100000
	events := []Event{
		{Hash: "e1", TimestampMS: from + 1000, Provider: "codex", Model: "gpt-5", AuthIndex: "idx-1", InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		{Hash: "e2", TimestampMS: from + 2000, Provider: "codex", Model: "gpt-5", AuthIndex: "idx-1", InputTokens: 200, OutputTokens: 50, TotalTokens: 250, Failed: true},
	}
	if _, err := st.InsertEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplacePrices(ctx, map[string]Price{"gpt-5": {Prompt: 1, Completion: 2}}); err != nil {
		t.Fatal(err)
	}

	creds := []CredentialIdentity{
		{Key: "row-1", AuthIndex: "idx-1"},
	}
	res, err := st.EnrichCredentials(ctx, creds, 200000)
	if err != nil {
		t.Fatal(err)
	}
	enr, ok := res["row-1"]
	if !ok || enr.History == nil {
		t.Fatalf("missing enrichment: %#v", res)
	}
	if enr.History.Requests != 2 || enr.History.SuccessCalls != 1 || enr.History.FailureCalls != 1 {
		t.Fatalf("unexpected history: %#v", enr.History)
	}
	if len(enr.RecentStatuses) != 8 {
		t.Fatalf("expected 8 status slots, got %d", len(enr.RecentStatuses))
	}
	if enr.RecentStatuses[7] != "fail" || enr.RecentStatuses[6] != "ok" {
		t.Fatalf("unexpected recent statuses: %#v", enr.RecentStatuses)
	}
}

func TestEnrichCredentialsCrossModuleQuotaCooperation(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// 1. Standalone quota snapshot works even when inspection never ran.
	if err := st.SaveCredentialQuotaSnapshot(ctx, InspectionResult{
		AccountKey: "ag-1",
		AuthIndex:  "ag-idx",
		Provider:   "antigravity",
		AuthType:   "oauth",
		PlanType:   "pro",
		QuotaWindows: []map[string]any{
			{"id": "b1", "kind": "antigravity_bucket", "label": "Weekly Limit Remaining", "remainingPercent": 100.0},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// 2. Window Keeper table provides Codex windows when inspection is disabled.
	if err := st.TouchWindowKeeperAccount(ctx, windowkeeper.Account{
		AuthID:    "wk-auth.json",
		AuthIndex: "wk-idx",
		AccountID: "acc-1",
		Email:     "wk@example.com",
		Name:      "wk-auth.json",
		PlanType:  "plus",
		PlanGroup: "plus",
	}); err != nil {
		t.Fatal(err)
	}
	endAt := time.Now().Add(2 * time.Hour)
	if err := st.SaveWindowKeeperWindows(ctx, "wk-auth.json", []windowkeeper.State{{
		LimitID:       "codex",
		Slot:          "primary",
		PeriodSeconds: 18000,
		Kind:          "rolling",
		EndsAt:        endAt,
		UsedPercent:   74,
		Gating:        true,
		Phase:         "steady",
	}}); err != nil {
		t.Fatal(err)
	}

	res, err := st.EnrichCredentials(ctx, []CredentialIdentity{
		{Key: "ag", AuthIndex: "ag-idx", Provider: "antigravity", AuthType: "oauth"},
		{Key: "wk", AuthIndex: "wk-idx", AuthID: "wk-auth.json", FileName: "wk-auth.json", Provider: "codex", AuthType: "oauth"},
	}, 100000)
	if err != nil {
		t.Fatal(err)
	}

	agWin, ok := res["ag"].QuotaWindows.([]any)
	if !ok || len(agWin) != 1 {
		t.Fatalf("expected antigravity snapshot window, got %#v", res["ag"].QuotaWindows)
	}
	agMap := agWin[0].(map[string]any)
	if agMap["modelScope"] != "account" || agMap["periodHours"] != float64(168) {
		t.Fatalf("expected normalized antigravity scope and periodHours, got %#v", agMap)
	}

	wkWin, ok := res["wk"].QuotaWindows.([]map[string]any)
	if !ok || len(wkWin) != 1 || wkWin[0]["modelScope"] != "account" || res["wk"].PlanLabel != "plus" {
		t.Fatalf("expected window_keeper fallback windows, got plan=%q windows=%#v", res["wk"].PlanLabel, res["wk"].QuotaWindows)
	}
}
