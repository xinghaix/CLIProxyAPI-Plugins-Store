package store

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"
)

func regressionWindow() AccountWindowUsageTarget {
	return AccountWindowUsageTarget{RowKey: "a", ProviderWindowID: "history", Period: "current", FromMS: 10, ToMS: 30000, AuthIndex: "idx", AuthFileSnapshot: "a.json", Source: "/auth/a.json", AuthProviderSnapshot: "codex"}
}

func windowResult(t *testing.T, st *Store, w AccountWindowUsageTarget) AccountWindowUsageItem {
	t.Helper()
	items, err := st.AccountWindowUsage(context.Background(), []AccountWindowUsageTarget{w})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%#v", items)
	}
	return items[0]
}

func TestAccountWindowPreservesExplicitRequestKey(t *testing.T) {
	st := observationStore(t)
	for _, key := range []string{" key \t", "\t ", "row\x00window\x00current"} {
		w := regressionWindow()
		w.RequestKey = key
		if got := windowResult(t, st, w); got.RequestKey != key {
			t.Errorf("opaque request_key changed: got %q want %q", got.RequestKey, key)
		}
	}
}

func TestAccountWindowCompleteBeyondDisplayLimit(t *testing.T) {
	st := observationStore(t)
	events := make([]Event, 20001)
	for i := range events {
		events[i] = Event{Hash: fmt.Sprint(i), TimestampMS: int64(10 + i), Provider: "codex", AuthIndex: "idx", Model: "gpt-5.4", InputTokens: 1, TotalTokens: 1, Failed: i == 0}
	}
	events = append(events, Event{Hash: "exclusive-end", TimestampMS: 30000, Provider: "codex", AuthIndex: "idx", Model: "gpt-5.4", TotalTokens: 999})
	if _, err := st.InsertEvents(context.Background(), events); err != nil {
		t.Fatal(err)
	}
	got := windowResult(t, st, regressionWindow())
	if got.TotalRequests != 20001 || got.TotalTokens != 20001 || got.FailureCalls != 1 || got.SuccessCalls != 20000 || !got.CostComplete {
		t.Fatalf("truncated/boundary totals: %#v", got)
	}
}

func TestAccountWindowResponsePricingParity(t *testing.T) {
	for _, tc := range []struct {
		name, requested, observed, mode string
		cost                            float64
		complete                        bool
	}{
		{"auto-priority", "auto", "priority", "", .8, true},
		{"priority-default", "priority", "default", "", .4, true},
		{"default-priority", "default", "priority", "", .8, true},
		{"priority-flex", "priority", "flex", "", 0, false},
		{"auto-flex", "auto", "flex", "", 0, false},
		{"flex-default", "flex", "default", "", .4, true},
		{"global-key-collision", "priority", "default", "collision", .8, true},
		{"tier-ambiguity", "priority", "default", "tier", .8, true},
		{"observation-ambiguity", "priority", "default", "observation", .8, true},
		{"suppressed", "priority", "default", "suppressed", .8, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := observationStore(t)
			ctx := context.Background()
			now := time.Now().UnixMilli()
			w := regressionWindow()
			w.FromMS = now - 1000
			w.ToMS = now
			e := Event{Hash: "one", TimestampMS: now - 500, Provider: "codex", AuthIndex: "idx", Model: "gpt-5.4", ServiceTier: tc.requested, InputTokens: 100000, OutputTokens: 10000, TotalTokens: 110000, ResponseCorrelationKey: "key"}
			if _, err := st.InsertEvents(ctx, []Event{e}); err != nil {
				t.Fatal(err)
			}
			if err := st.RecordResponseMetadata(ctx, ResponseMetadata{Key: "key", ServiceTier: tc.observed}); err != nil {
				t.Fatal(err)
			}
			switch tc.mode {
			case "collision":
				e.Hash = "two"
				e.AuthIndex = "other"
				e.Model = "other"
				e.TimestampMS = now + 1000
				e.Failed = true
				if _, err := st.InsertEvents(ctx, []Event{e}); err != nil {
					t.Fatal(err)
				}
			case "tier":
				if err := st.RecordResponseMetadata(ctx, ResponseMetadata{Key: "key", ServiceTier: "priority"}); err != nil {
					t.Fatal(err)
				}
			case "observation":
				if err := st.RecordResponseObservation(ctx, "key", "", "", true); err != nil {
					t.Fatal(err)
				}
			case "suppressed":
				st.SuppressResponseObservations()
			}
			a, err := st.Analytics(ctx, AnalyticsRequest{FromMS: w.FromMS, ToMS: w.ToMS - 1, IncludeFailed: true})
			if err != nil {
				t.Fatal(err)
			}
			summary := a["summary"].(map[string]any)
			if math.Abs(summary["cost"].(float64)-tc.cost) > 1e-10 || summary["cost_complete"] != tc.complete {
				t.Fatalf("fixture/analytics mismatch: %#v", summary)
			}
			got := windowResult(t, st, w)
			if math.Abs(got.TotalCost-tc.cost) > 1e-10 || got.CostComplete != tc.complete || got.UnpricedCalls != summary["unpriced_calls"] || got.PricedCalls != summary["priced_calls"] {
				t.Errorf("window=%#v analytics=%#v", got, summary)
			}
			// SQLite/fsync contention can exceed one second; this is a pricing
			// parity check, not a wall-clock expiry test.
			enriched, err := st.EnrichCredentials(ctx, []CredentialIdentity{{Key: "a", AuthIndex: "idx", Provider: "codex"}}, int64(time.Hour/time.Millisecond))
			if err != nil {
				t.Fatal(err)
			}
			h := enriched["a"].History
			if h.Requests != 1 || math.Abs(h.Cost-tc.cost) > 1e-10 || h.CostComplete != tc.complete {
				t.Errorf("list history differs: %#v", h)
			}
		})
	}
}

func TestAccountWindowHistoricalProvider(t *testing.T) {
	st := observationStore(t)
	ctx := context.Background()
	w := regressionWindow()
	w.AuthProviderSnapshot = "claude"
	if err := st.ReplacePrices(ctx, map[string]Price{"legacy": {Prompt: 1, CacheRead: .1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertEvents(ctx, []Event{
		{Hash: "old", TimestampMS: 10, AuthIndex: "idx", AuthID: "a.json", Model: "legacy", InputTokens: 10, CacheReadTokens: 90, TotalTokens: 100},
		{Hash: "new", TimestampMS: 20, AuthIndex: "idx", AuthID: "a.json", Provider: "claude", Model: "legacy", InputTokens: 10, TotalTokens: 10},
	}); err != nil {
		t.Fatal(err)
	}
	got := windowResult(t, st, w)
	if got.TotalRequests != 2 || got.PricedCalls != 2 || !got.CostComplete || math.Abs(got.TotalCost-.000029) > 1e-12 {
		t.Fatalf("historical provider: %#v", got)
	}
}

func TestAccountWindowBatchProviderInvariant(t *testing.T) {
	for _, tc := range []struct {
		name, inside, outside string
		cost                  float64
	}{
		{"outside-only", "", "claude", 0},
		{"outside-conflict", "claude", "codex", .000019},
		{"outside-wrong-provider", "", "codex", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := observationStore(t)
			ctx := context.Background()
			if err := st.ReplacePrices(ctx, map[string]Price{"legacy": {Prompt: 1, CacheRead: .1}}); err != nil {
				t.Fatal(err)
			}
			events := []Event{
				{Hash: "old", TimestampMS: 10, AuthIndex: "idx", AuthID: "a.json", Model: "legacy", InputTokens: 10, CacheReadTokens: 90, TotalTokens: 100},
				{Hash: "outside", TimestampMS: 15, AuthIndex: "idx", AuthID: "a.json", Provider: tc.outside, Model: "legacy", InputTokens: 10, TotalTokens: 10},
			}
			if tc.inside != "" {
				// Evidence from another model must survive model-scope filtering.
				events = append(events, Event{Hash: "inside", TimestampMS: 12, AuthIndex: "idx", AuthID: "a.json", Provider: tc.inside, Model: "evidence"})
			}
			if _, err := st.InsertEvents(ctx, events); err != nil {
				t.Fatal(err)
			}
			w := regressionWindow()
			w.AuthProviderSnapshot = "claude"
			w.FromMS, w.ToMS = 10, 15
			w.ModelScopeModels = []string{"legacy"}
			alone := windowResult(t, st, w)
			other := w
			other.FromMS, other.ToMS = 15, 25
			other.AuthProviderSnapshot = tc.outside
			a, err := st.Analytics(ctx, AnalyticsRequest{FromMS: 10, ToMS: 14, Models: []string{"legacy"}, IncludeFailed: true})
			if err != nil {
				t.Fatal(err)
			}
			summary := a["summary"].(map[string]any)
			if alone.TotalRequests != 1 || math.Abs(alone.TotalCost-tc.cost) > 1e-12 || alone.CostComplete != (tc.inside != "") {
				t.Fatalf("single-window fixture: %#v", alone)
			}
			for _, windows := range [][]AccountWindowUsageTarget{{w, other}, {other, w}} {
				batch, err := st.AccountWindowUsage(ctx, windows)
				if err != nil {
					t.Fatal(err)
				}
				if len(batch) != len(windows) {
					t.Fatalf("batch length=%d", len(batch))
				}
				for i, target := range windows {
					if target.FromMS != w.FromMS {
						continue
					}
					got := batch[i]
					t.Logf("alone=%g batch=%g analytics=%g requests=%d complete=%v", alone.TotalCost, got.TotalCost, summary["cost"], got.TotalRequests, got.CostComplete)
					if got.TotalRequests != alone.TotalRequests || got.TotalRequests != summary["calls"] || math.Abs(got.TotalCost-alone.TotalCost) > 1e-12 || math.Abs(got.TotalCost-summary["cost"].(float64)) > 1e-12 || got.CostComplete != alone.CostComplete || got.CostComplete != summary["cost_complete"] || got.PricedCalls != summary["priced_calls"] || got.UnpricedCalls != summary["unpriced_calls"] {
						t.Errorf("batch changed window: single=%#v batch=%#v analytics=%#v", alone, got, summary)
					}
				}
			}
		})
	}
}

func TestAccountWindowIdentityFallback(t *testing.T) {
	for _, tc := range []struct {
		name, idx, id, source, provider string
		want                            int64
	}{
		{"index", "idx", "different", "different", "codex", 1},
		{"missing-index-file", "", "a.json", "", "codex", 1},
		{"missing-index-source", "", "", "/auth/a.json", "codex", 1},
		{"missing-index-real-id", "", "opaque-id", "", "codex", 1},
		{"conflicting-index", "other", "a.json", "/auth/a.json", "codex", 0},
		{"wrong-provider", "idx", "a.json", "", "claude", 0},
		{"case-sensitive-file", "", "A.json", "", "codex", 0},
		{"display-not-identity", "", "display-name", "", "codex", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := observationStore(t)
			w := regressionWindow()
			w.AccountSnapshot = "display-name"
			// Exercise the wire contract without requiring the new field to compile RED.
			if err := json.Unmarshal([]byte(`{"auth_id":"opaque-id"}`), &w); err != nil {
				t.Fatal(err)
			}
			if _, err := st.InsertEvents(context.Background(), []Event{{Hash: "e", TimestampMS: 10, AuthIndex: tc.idx, AuthID: tc.id, Source: tc.source, Provider: tc.provider, Model: "gpt-5.4", TotalTokens: 1}}); err != nil {
				t.Fatal(err)
			}
			if got := windowResult(t, st, w); got.TotalRequests != tc.want {
				t.Fatalf("identity: %#v want %d", got, tc.want)
			}
			row := eventRow{AuthIndex: tc.idx, AuthID: tc.id, Source: tc.source, Provider: tc.provider}
			if got := accountWindowRowMatches(row, w); got != (tc.want == 1) {
				t.Fatalf("matcher=%v want %d", got, tc.want)
			}
		})
	}
}

func TestAccountWindowExactModelScope(t *testing.T) {
	st := observationStore(t)
	ctx := context.Background()
	if _, err := st.InsertEvents(ctx, []Event{
		{Hash: "opus", TimestampMS: 10, AuthIndex: "idx", Provider: "claude", Model: "claude-opus-4-6", Alias: "sonnet-alias", TotalTokens: 100},
		{Hash: "sonnet", TimestampMS: 20, AuthIndex: "idx", Provider: "claude", Model: "claude-sonnet-4-6", Alias: "claude-opus-4-6", TotalTokens: 200},
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id, models string
		tokens     int64
		status     string
	}{
		{"claude-seven-day-opus", `["claude-opus-4-6"]`, 100, "complete"},
		{"claude-seven-day-sonnet", `["claude-sonnet-4-6"]`, 200, "complete"},
		{"claude-seven-day-opus", `null`, 0, "unknown"},
		{"claude-seven-day-fable", `[]`, 0, "unknown"},
		{"claude-seven-day-oauth-apps", `null`, 0, "unknown"},
		{"claude-seven-day", `null`, 300, "complete"},
		{"future-scoped-window", `null`, 0, "unknown"},
	} {
		t.Run(tc.id+tc.models, func(t *testing.T) {
			w := regressionWindow()
			w.AuthProviderSnapshot = "claude"
			w.ProviderWindowID = tc.id
			if err := json.Unmarshal([]byte(`{"model_scope_models":`+tc.models+`}`), &w); err != nil {
				t.Fatal(err)
			}
			got := windowResult(t, st, w)
			if got.TotalTokens != tc.tokens || got.ScopeMatchStatus != tc.status {
				t.Fatalf("scope: %#v", got)
			}
			if tc.status == "unknown" && (got.Matched || got.CostComplete || got.SyncStatus != "unknown") {
				t.Fatalf("unknown fabricated usable totals: %#v", got)
			}
		})
	}
}
