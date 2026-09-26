package store

import (
	"context"
	"testing"
	"time"
)

func TestAccountWindowUsageAggregatesCostAndTokens(t *testing.T) {
	ctx := context.Background()
	st, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now().UnixMilli()
	from := now - 5*int64(time.Hour/time.Millisecond)
	events := []Event{
		{Hash: "w1", TimestampMS: from + 1000, Provider: "codex", Model: "gpt-5", AuthIndex: "auth-1", AuthID: "codex-1.json", AuthType: "oauth", Source: "codex-1.json", InputTokens: 100, OutputTokens: 50, TotalTokens: 150},
		{Hash: "w2", TimestampMS: from + 2000, Provider: "codex", Model: "gpt-5", AuthIndex: "auth-1", AuthID: "codex-1.json", AuthType: "oauth", Source: "codex-1.json", InputTokens: 200, OutputTokens: 20, TotalTokens: 220, Failed: true},
		{Hash: "w3", TimestampMS: from + 3000, Provider: "codex", Model: "gpt-5", AuthIndex: "other", AuthID: "other.json", AuthType: "oauth", Source: "other.json", InputTokens: 999, OutputTokens: 1, TotalTokens: 1000},
	}
	if _, err := st.InsertEvents(ctx, events); err != nil {
		t.Fatal(err)
	}
	if err := st.ReplacePrices(ctx, map[string]Price{"gpt-5": {Prompt: 1, Completion: 2}}); err != nil {
		t.Fatal(err)
	}
	items, err := st.AccountWindowUsage(ctx, []AccountWindowUsageTarget{{
		RowKey: "row-1", ProviderWindowID: "five_hour", Period: "current",
		FromMS: from, ToMS: now, AuthIndex: "auth-1", AuthFileSnapshot: "codex-1.json",
		AuthProviderSnapshot: "codex", RequestKey: "k1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Matched || items[0].TotalRequests != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].SuccessCalls != 1 || items[0].FailureCalls != 1 || items[0].TotalTokens != 370 {
		t.Fatalf("totals = %#v", items[0])
	}
	if items[0].TotalCost <= 0 || !items[0].CostComplete {
		t.Fatalf("cost = %#v", items[0])
	}
}

func TestEstimateWindowUsageForecastHelpers(t *testing.T) {
	got := EstimateWindowUsage(1, WindowUsageMetrics{Requests: 2, Tokens: 6400, Cost: 0.02}, WindowUsageMetrics{})
	if got == nil || got.Basis != "quota" || got.Requests != 200 || got.Tokens != 640000 || got.Cost != 2 {
		t.Fatalf("got = %#v", got)
	}
	got = EstimateWindowUsage(0, WindowUsageMetrics{Requests: 1, Tokens: 10, Cost: 0.01}, WindowUsageMetrics{Requests: 60, Tokens: 600000, Cost: 6})
	if got == nil || got.Basis != "previous" || got.Requests != 60 {
		t.Fatalf("fallback = %#v", got)
	}
}
