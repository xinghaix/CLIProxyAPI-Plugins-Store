package store

import (
	"context"
	"math"
	"testing"
)

func TestAnalyticsFilteredProviderPricingParity(t *testing.T) {
	st := observationStore(t)
	ctx := context.Background()
	if err := st.ReplacePrices(ctx, map[string]Price{"legacy": {Prompt: 1, CacheRead: .1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.InsertEvents(ctx, []Event{
		{Hash: "old", TimestampMS: 10, AuthIndex: "idx", AuthID: "a.json", Model: "legacy", InputTokens: 10, CacheReadTokens: 90, TotalTokens: 100},
		{Hash: "inside-claude", TimestampMS: 11, AuthIndex: "idx", AuthID: "a.json", Provider: "claude", Model: "legacy", InputTokens: 10, TotalTokens: 10},
		{Hash: "inside-conflict", TimestampMS: 12, AuthIndex: "idx", AuthID: "a.json", Provider: "codex", Model: "evidence", InputTokens: 10, TotalTokens: 10},
	}); err != nil {
		t.Fatal(err)
	}
	w := regressionWindow()
	w.FromMS, w.ToMS = 10, 15
	w.AuthProviderSnapshot = "claude"
	w.ModelScopeModels = []string{"legacy"}
	window := windowResult(t, st, w)
	if window.TotalRequests != 2 || window.TotalTokens != 110 || window.PricedCalls != 1 || window.UnpricedCalls != 1 || window.CostComplete || math.Abs(window.TotalCost-.00001) > 1e-12 {
		t.Fatalf("ambiguous provider must stay unpriced in the window: %#v", window)
	}

	for _, tc := range []struct {
		name    string
		request AnalyticsRequest
	}{
		{"model", AnalyticsRequest{Models: []string{"legacy"}}},
		{"search", AnalyticsRequest{Search: "legacy"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := tc.request
			request.FromMS, request.ToMS = w.FromMS, w.ToMS-1
			request.IncludeFailed = true
			result, err := st.Analytics(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			summary := result["summary"].(map[string]any)
			t.Logf("window cost=%g complete=%v calls=%d priced=%d unpriced=%d; analytics cost=%g complete=%v calls=%v priced=%v unpriced=%v", window.TotalCost, window.CostComplete, window.TotalRequests, window.PricedCalls, window.UnpricedCalls, summary["cost"], summary["cost_complete"], summary["calls"], summary["priced_calls"], summary["unpriced_calls"])
			if math.Abs(summary["cost"].(float64)-window.TotalCost) > 1e-12 || summary["cost_complete"] != window.CostComplete || summary["calls"] != window.TotalRequests || summary["total_tokens"] != window.TotalTokens || summary["priced_calls"] != window.PricedCalls || summary["unpriced_calls"] != window.UnpricedCalls {
				t.Errorf("filter discarded conflicting provider evidence: window=%#v analytics=%#v", window, summary)
			}
			events := result["events"].(map[string]any)["items"].([]map[string]any)
			// Analytics returns newest first; the older row must remain unknown,
			// not inherit Claude after the conflicting model is filtered away.
			if len(events) != 2 || events[1]["timestamp_ms"] != int64(10) || events[1]["provider"] != "" || events[1]["auth_provider_snapshot"] != "" || events[1]["cost"] != nil {
				t.Errorf("ambiguous event acquired a provider or charge: %#v", events)
			}
		})
	}
}
