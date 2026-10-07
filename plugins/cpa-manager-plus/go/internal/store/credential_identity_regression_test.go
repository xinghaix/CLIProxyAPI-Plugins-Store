package store

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

// Exercise both consumers against the migrated schema, not a synthetic lookup.
func TestCredentialEnrichmentIdentityIsolation(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	run, err := st.StartInspection(ctx, "manual")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	for _, tc := range []struct {
		name                              string
		creds                             []CredentialIdentity
		index, id, source, provider, want string
	}{
		{"strong-index-wins", []CredentialIdentity{
			{Key: "a", AuthIndex: "idx-a", AuthID: "a.json", FileName: "shared.json", Provider: "codex"},
			{Key: "b", AuthIndex: "idx-b", AuthID: "b.json", FileName: "shared.json", Provider: "codex"},
		}, "idx-a", "b.json", "shared.json", "codex", "a"},
		{"strong-conflict-no-fallback", []CredentialIdentity{
			{Key: "a", AuthIndex: "idx-a", AuthID: "a.json", FileName: "shared.json", Provider: "codex"},
		}, "other-index", "a.json", "shared.json", "codex", ""},
		{"strong-provider-conflict-no-fallback", []CredentialIdentity{
			{Key: "a", AuthIndex: "idx-a", Provider: "codex"},
			{Key: "b", FileName: "shared.json", Provider: "claude"},
		}, "idx-a", "", "shared.json", "claude", ""},
		{"same-provider-weak-collision", []CredentialIdentity{
			{Key: "a", AuthIndex: "idx-a", AuthID: "/a/shared.json", Provider: "codex"},
			{Key: "b", AuthIndex: "idx-b", AuthID: "/b/shared.json", Provider: "codex"},
		}, "", "shared.json", "shared.json", "codex", ""},
		{"provider-selects-compatible-weak-owner", []CredentialIdentity{
			{Key: "a", FileName: "shared.json", Provider: "codex"},
			{Key: "b", FileName: "shared.json", Provider: "claude"},
		}, "", "shared.json", "shared.json", "CODEX", "a"},
		{"unknown-provider-cannot-resolve-collision", []CredentialIdentity{
			{Key: "a", FileName: "shared.json", Provider: "codex"},
			{Key: "b", FileName: "shared.json", Provider: "claude"},
		}, "", "shared.json", "shared.json", "", ""},
		{"exact-id-before-filename", []CredentialIdentity{
			{Key: "a", AuthID: "/a/shared.json", Provider: "codex"},
			{Key: "b", FileName: "shared.json", Provider: "codex"},
		}, "", "/a/shared.json", "shared.json", "codex", "a"},
		{"duplicate-aliases-are-one-owner", []CredentialIdentity{
			{Key: "a", AuthID: "/a/shared.json", FileName: "shared.json", Source: "/a/shared.json", Provider: "codex"},
		}, "", "shared.json", "shared.json", "codex", "a"},
		{"disagreeing-weak-aliases-are-ambiguous", []CredentialIdentity{
			{Key: "a", FileName: "a.json", Provider: "codex"},
			{Key: "b", FileName: "b.json", Provider: "codex"},
		}, "", "a.json", "b.json", "codex", ""},
		{"index-case-sensitive", []CredentialIdentity{
			{Key: "a", AuthIndex: "Index", Provider: "codex"},
			{Key: "b", AuthIndex: "index", Provider: "codex"},
		}, "Index", "", "", "codex", "a"},
		{"id-case-sensitive", []CredentialIdentity{
			{Key: "a", AuthID: "Account.json", Provider: "codex"},
			{Key: "b", AuthID: "account.json", Provider: "codex"},
		}, "", "Account.json", "", "codex", "a"},
		{"source-case-sensitive", []CredentialIdentity{
			{Key: "a", Source: "Account.json", Provider: "codex"},
			{Key: "b", Source: "account.json", Provider: "codex"},
		}, "", "", "Account.json", "codex", "a"},
		{"blank-identities-do-not-match", []CredentialIdentity{
			{Key: "a", AuthIndex: " ", AuthID: " ", FileName: " ", Source: " ", Provider: "codex"},
			{Key: "b", FileName: ".", Provider: "codex"},
		}, " ", " ", " ", "codex", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := st.db.ExecContext(ctx, `delete from usage_events; delete from codex_inspection_results`); err != nil {
				t.Fatal(err)
			}
			if _, err := st.InsertEvents(ctx, []Event{{Hash: tc.name, TimestampMS: now, Model: "unpriced-test-model", AuthIndex: tc.index, AuthID: tc.id, Source: tc.source, Provider: tc.provider, TotalTokens: 17, Failed: true}}); err != nil {
				t.Fatal(err)
			}
			if _, err := st.InsertInspectionResult(ctx, InspectionResult{RunID: run.ID, AccountKey: tc.name, AuthIndex: tc.index, AuthID: tc.id, FileName: tc.source, Provider: tc.provider, PlanType: "owned-plan", QuotaWindows: []string{"owned-window"}}); err != nil {
				t.Fatal(err)
			}
			got, err := st.EnrichCredentials(ctx, tc.creds, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range tc.creds {
				enr := got[c.Key]
				wantCalls := int64(0)
				wantPlan := ""
				wantStatuses := make([]string, 8)
				var wantQuota any
				if c.Key == tc.want {
					wantCalls, wantPlan = 1, "owned-plan"
					wantStatuses[7] = "fail"
					wantQuota = []any{"owned-window"}
				}
				if enr == nil || enr.History == nil {
					t.Fatalf("missing %s", c.Key)
				}
				if enr.History.Requests != wantCalls || enr.History.Tokens != 17*wantCalls || enr.History.FailureCalls != wantCalls || enr.History.UnpricedCalls != wantCalls || enr.History.CostComplete != (wantCalls == 0) {
					t.Errorf("%s history = %+v; want %d calls", c.Key, enr.History, wantCalls)
				}
				if !reflect.DeepEqual(enr.RecentStatuses, wantStatuses) {
					t.Errorf("%s statuses = %v; want %v", c.Key, enr.RecentStatuses, wantStatuses)
				}
				if enr.PlanLabel != wantPlan || !reflect.DeepEqual(enr.QuotaWindows, wantQuota) {
					t.Errorf("%s cache = %q %#v; want %q %#v", c.Key, enr.PlanLabel, enr.QuotaWindows, wantPlan, wantQuota)
				}
			}
		})
	}
}

func TestCredentialEnrichmentQuietAccountSurvivesBusyHistory(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UnixMilli()
	day := int64(24 * time.Hour / time.Millisecond)
	events := []Event{
		{Hash: "too-old", TimestampMS: now - 91*day, Model: "unpriced-test-model", AuthIndex: "quiet", TotalTokens: 10000},
		{Hash: "old-but-in-range", TimestampMS: now - 89*day, Model: "unpriced-test-model", AuthIndex: "quiet", TotalTokens: 3, Failed: true},
	}
	for i := 0; i < 10; i++ {
		events = append(events, Event{Hash: fmt.Sprintf("quiet-%d", i), TimestampMS: now - day + int64(i), Model: "priced-test-model", AuthIndex: "quiet", InputTokens: 1_000_000, TotalTokens: 1_000_000, Failed: i%3 == 0})
	}
	for i := 0; i < 2501; i++ {
		events = append(events, Event{Hash: fmt.Sprintf("busy-%d", i), TimestampMS: now - 10000 + int64(i), Model: "priced-test-model", AuthIndex: "busy", InputTokens: 1_000_000, TotalTokens: 1_000_000})
	}
	if _, err := st.InsertEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplacePrices(ctx, map[string]Price{"priced-test-model": {Prompt: 1}}); err != nil {
		t.Fatal(err)
	}
	// 201 newer rows for one account used to evict another account's cache.
	run, err := st.StartInspection(ctx, "manual")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < 202; i++ {
		idx := "busy"
		if i == 0 {
			idx = "quiet"
		}
		if _, err := tx.ExecContext(ctx, `insert into codex_inspection_results(run_id,account_key,file_name,display_account,action,created_at_ms,auth_index,plan_type,quota_windows_json) values(?,?,?,?,?,?,?,?,?)`, run.ID, fmt.Sprint(i), idx+".json", idx, "keep", now, idx, fmt.Sprintf("plan-%d", i), fmt.Sprintf(`["window-%d"]`, i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	got, err := st.EnrichCredentials(ctx, []CredentialIdentity{{Key: "quiet", AuthIndex: "quiet"}, {Key: "busy", AuthIndex: "busy"}, {Key: "empty", AuthIndex: "empty"}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	quiet := got["quiet"]
	if h := quiet.History; h.Requests != 11 || h.Tokens != 10_000_003 || h.FailureCalls != 5 || h.SuccessCalls != 6 || h.UnpricedCalls != 1 || h.CostComplete || h.Cost != 10 || h.LastSeenMS == nil || *h.LastSeenMS != now-day+9 || h.SuccessRate == nil || *h.SuccessRate != float64(6)/11 {
		t.Errorf("quiet history = %+v", h)
	}
	if want := []string{"ok", "fail", "ok", "ok", "fail", "ok", "ok", "fail"}; !reflect.DeepEqual(quiet.RecentStatuses, want) {
		t.Errorf("quiet statuses = %v; want %v", quiet.RecentStatuses, want)
	}
	if quiet.PlanLabel != "plan-0" || !reflect.DeepEqual(quiet.QuotaWindows, []any{"window-0"}) {
		t.Errorf("quiet cache lost: %+v", quiet)
	}
	if busy := got["busy"]; busy.History.Requests != 2501 || busy.History.Cost != 2501 || !busy.History.CostComplete || busy.PlanLabel != "plan-201" || !reflect.DeepEqual(busy.QuotaWindows, []any{"window-201"}) || !reflect.DeepEqual(busy.RecentStatuses, []string{"ok", "ok", "ok", "ok", "ok", "ok", "ok", "ok"}) {
		t.Errorf("busy = %+v; history = %+v", busy, busy.History)
	}
	if empty := got["empty"]; empty.History.Requests != 0 || empty.History.LastSeenMS != nil || empty.History.SuccessRate != nil || !empty.History.CostComplete || empty.PlanLabel != "" || empty.QuotaWindows != nil || !reflect.DeepEqual(empty.RecentStatuses, make([]string, 8)) {
		t.Errorf("empty = %+v", empty)
	}
}
