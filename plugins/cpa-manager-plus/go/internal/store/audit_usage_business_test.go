package store_test

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/ingest"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func auditDB(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func auditInsert(t *testing.T, s *store.Store, events []store.Event) {
	t.Helper()
	n, err := s.InsertEvents(context.Background(), events)
	if err != nil || n != len(events) {
		t.Fatalf("inserted=%d wanted=%d err=%v", n, len(events), err)
	}
}

func TestAuditClaudeCachePricing(t *testing.T) {
	s := auditDB(t)
	ctx := context.Background()
	const model = "claude-audit"
	price := store.Price{Prompt: 3, Completion: 15, CacheRead: 0.3, CacheCreation: 3.75}
	if err := s.ReplacePrices(ctx, map[string]store.Price{model: price}); err != nil {
		t.Fatal(err)
	}
	var events []store.Event
	for i, read := range []int64{50, 500} {
		events = append(events, ingest.ToEvent(pluginapi.UsageRecord{Provider: "claude", Model: model, RequestedAt: time.UnixMilli(1000 + int64(i)), Detail: pluginapi.UsageDetail{InputTokens: 100, OutputTokens: 20, CachedTokens: read, CacheReadTokens: read, CacheCreationTokens: 20, TotalTokens: 140 + read}}))
	}
	// Stored legacy rows must be repaired at estimation, not only at ingest.
	events = append(events, store.Event{Hash: "legacy-claude-write-only", TimestampMS: 1002, Provider: "anthropic", Model: model, InputTokens: 100, OutputTokens: 20, CachedTokens: 20, CacheCreationTokens: 20, TotalTokens: 140})
	auditInsert(t, s, events)
	a, err := s.Analytics(ctx, store.AnalyticsRequest{FromMS: 0, ToMS: 2000, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range a["events"].(map[string]any)["items"].([]map[string]any) {
		e := item["cost_estimate"].(pricing.Estimate)
		read := item["cache_read_tokens"].(int64)
		want := (100*price.Prompt + 20*price.Completion + float64(read)*price.CacheRead + 20*price.CacheCreation) / 1e6
		t.Logf("claude input=100 read=%d write=20 output=20 status=%s note=%s cost=%g expected=%g", read, e.Status, e.Note, e.Amount, want)
		if e.Status != pricing.StatusEstimated || math.Abs(e.Amount-want) > 1e-12 {
			t.Errorf("valid independent Claude buckets priced incorrectly")
		}
	}
}

func TestAuditAnalyticsCandidateCutoff(t *testing.T) {
	s := auditDB(t)
	ctx := context.Background()
	events := make([]store.Event, 10001)
	for i := range events {
		events[i] = store.Event{Hash: fmt.Sprintf("audit-cap-%d", i), TimestampMS: int64(i + 1), Model: "newer", InputTokens: 1, TotalTokens: 1}
	}
	events[0].Model = "older-target"
	events[0].Provider = "older-provider"
	events[0].AuthIndex = "older-account"
	events[0].APIKeyHash = "older-key"
	events[0].Source = "older-search"
	if err := s.ReplacePrices(ctx, map[string]store.Price{"newer": {Prompt: 1}, "older-target": {Prompt: 1}}); err != nil {
		t.Fatal(err)
	}
	auditInsert(t, s, events)
	count, err := s.EventCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.Analytics(ctx, store.AnalyticsRequest{FromMS: 0, ToMS: 20000, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := a["summary"].(map[string]any)["calls"].(int64)
	t.Logf("database=%d analytics calls=%d cost_complete=%v", count, got, a["summary"].(map[string]any)["cost_complete"])
	if len(a["events"].(map[string]any)["items"].([]map[string]any)) != 1 {
		t.Error("display limit must still apply")
	}
	if got != count {
		t.Errorf("summary silently omits %d events", count-got)
	}
	a, err = s.Analytics(ctx, store.AnalyticsRequest{FromMS: 0, ToMS: 20000, Limit: 1, Models: []string{"older-target"}})
	if err != nil {
		t.Fatal(err)
	}
	got = a["summary"].(map[string]any)["calls"].(int64)
	t.Logf("filter model=older-target calls=%d expected=1", got)
	if got != 1 {
		t.Error("filter lost existing matching event before filtering")
	}
	for _, req := range []store.AnalyticsRequest{
		{Providers: []string{"older-provider"}}, {Accounts: []string{"older-account"}},
		{APIKeyHashes: []string{"older-key"}}, {Search: "older-search"},
	} {
		req.FromMS, req.ToMS, req.Limit = 0, 20000, 1
		filtered, err := s.Analytics(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		if filtered["summary"].(map[string]any)["calls"].(int64) != 1 {
			t.Errorf("full-range filter lost oldest event: %+v", req)
		}
	}
}

func TestAuditRollingThirtyMinuteBoundary(t *testing.T) {
	s := auditDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 6, 12, 10, 0, 0, time.UTC)
	day := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	var events []store.Event
	for i, minutes := range []int{70, 20, 5, 2} {
		events = append(events, store.Event{Hash: fmt.Sprintf("audit-roll-%d", i), TimestampMS: now.Add(-time.Duration(minutes) * time.Minute).UnixMilli(), Model: "audit", InputTokens: 101, TotalTokens: 101})
	}
	auditInsert(t, s, events)
	d, err := s.Dashboard(ctx, day.UnixMilli(), now.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	rolling := d["rolling_30m"].(map[string]any)
	t.Logf("now=12:10 UTC event ages=[70,20,5,2]m rolling_30m=%v expected calls=3 tokens=303", rolling)
	if rolling["total_calls"].(int64) != 3 || rolling["total_tokens"].(int64) != 303 {
		t.Error("rolling window is current hourly bucket, not preceding 30 minutes")
	}
	if rpm, ok := rolling["rpm"].(float64); !ok || math.Abs(rpm-0.1) > 1e-12 {
		t.Errorf("fractional rpm lost: %v", rolling["rpm"])
	}
	if tpm, ok := rolling["tpm"].(float64); !ok || math.Abs(tpm-10.1) > 1e-12 {
		t.Errorf("fractional tpm lost: %v", rolling["tpm"])
	}
}

func TestAuditWriterShutdownDropsAcceptedQueue(t *testing.T) {
	s := auditDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := ingest.NewWriter(s, 1000, 1, func([]store.Event) { cancel() })
	for i := 0; i < 1000; i++ {
		w.EnqueueEvent(store.Event{Hash: fmt.Sprintf("audit-drain-%d", i), TimestampMS: int64(i + 1), Model: "audit"})
	}
	w.Run(ctx)
	count, err := s.EventCount(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("accepted=1000 persisted=%d queue_depth_after_shutdown=%d dropped=%d failed=%d", count, w.Depth(), w.Dropped(), w.Failed())
	if count != 1000 {
		t.Errorf("graceful shutdown lost %d accepted events", 1000-count)
	}
}

func TestAuditRollingMidnightAndEmpty(t *testing.T) {
	s := auditDB(t)
	now := time.Date(2026, 10, 6, 0, 10, 0, 0, time.UTC).UnixMilli()
	events := []store.Event{
		{Hash: "outside", TimestampMS: now - 30*60*1000 - 1, Model: "test", TotalTokens: 1000},
		{Hash: "boundary", TimestampMS: now - 30*60*1000, Model: "test", TotalTokens: 7},
		{Hash: "previous-day", TimestampMS: now - 20*60*1000, Model: "test", TotalTokens: 11, Failed: true},
		{Hash: "now", TimestampMS: now, Model: "test", TotalTokens: 1},
		{Hash: "future", TimestampMS: now + 1, Model: "test", TotalTokens: 1000},
	}
	auditInsert(t, s, events)
	d, err := s.Dashboard(context.Background(), now-10*60*1000, now)
	if err != nil {
		t.Fatal(err)
	}
	roll := d["rolling_30m"].(map[string]any)
	if roll["total_calls"].(int64) != 3 || roll["total_tokens"].(int64) != 19 || math.Abs(roll["tpm"].(float64)-19.0/30) > 1e-12 {
		t.Fatalf("midnight window=%v", roll)
	}
	d, err = s.Dashboard(context.Background(), now+60*60*1000, now+70*60*1000)
	if err != nil {
		t.Fatal(err)
	}
	roll = d["rolling_30m"].(map[string]any)
	if roll["total_calls"].(int64) != 0 || roll["total_tokens"].(int64) != 0 || roll["rpm"].(float64) != 0 || roll["tpm"].(float64) != 0 {
		t.Fatalf("empty window=%v", roll)
	}
}

func TestAuditAnalyticsExtraFilters(t *testing.T) {
	s := auditDB(t)
	auditInsert(t, s, []store.Event{
		{Hash: "slow", TimestampMS: 1, Model: "test", AuthID: "slow.json", AuthIndex: "slow-index", LatencyMS: 1000, CacheReadTokens: 7},
		{Hash: "fast", TimestampMS: 2, Model: "test", AuthID: "fast.json", AuthIndex: "fast-index", LatencyMS: 100, CacheCreationTokens: 7},
	})
	for _, req := range []store.AnalyticsRequest{
		{AuthFiles: []string{"slow.json"}}, {AuthFiles: []string{"slow-index"}}, {MinLatencyMS: 1000}, {CacheStatus: "hit"},
		{AuthFiles: []string{"slow-index"}, MinLatencyMS: 1000, CacheStatus: "hit"},
	} {
		req.FromMS, req.ToMS, req.Limit = 0, 100, 10
		a, err := s.Analytics(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		summary := a["summary"].(map[string]any)
		items := a["events"].(map[string]any)["items"].([]map[string]any)
		if summary["calls"].(int64) != 1 || len(items) != 1 || items[0]["auth_file_snapshot"] != "slow.json" {
			t.Errorf("filter ignored: %+v summary=%v items=%v", req, summary, items)
		}
	}
}
