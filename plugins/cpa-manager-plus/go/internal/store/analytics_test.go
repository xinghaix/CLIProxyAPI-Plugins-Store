package store

import (
	"fmt"
	"testing"
	"time"
)

func TestAggregateAccountAPIKeyStatsBySource(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", Source: "oauth@example.com", AuthType: "oauth", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", Source: "oauth@example.com", AuthType: "oauth", AuthIndex: "account-b", APIKeyHash: "key-2", Provider: "xai", TotalTokens: 20},
		{ID: 3, TimestampMS: 300, Model: "model", Source: "sk-custom-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai", TotalTokens: 30},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	combined := result["account_api_key_stats"].([]map[string]any)
	if len(combined) != 2 {
		t.Fatalf("account_api_key_stats count = %d, want 2", len(combined))
	}

	bySource := map[string]map[string]any{}
	for _, row := range combined {
		bySource[row["source"].(string)] = row
	}
	oauth := bySource["oauth@example.com"]
	if calls := oauth["calls"]; calls != int64(2) {
		t.Fatalf("oauth calls = %#v, want 2", calls)
	}
	if tokens := oauth["total_tokens"]; tokens != int64(30) {
		t.Fatalf("oauth tokens = %#v, want 30", tokens)
	}
	if oauth["auth_provider_snapshot"] != "xai" {
		t.Fatalf("oauth provider = %#v, want xai", oauth["auth_provider_snapshot"])
	}
	if oauth["account_snapshot"] != "oauth@example.com" {
		t.Fatalf("compat account snapshot = %#v", oauth["account_snapshot"])
	}
	if oauth["auth_type"] != "oauth" {
		t.Fatalf("oauth auth type = %#v", oauth["auth_type"])
	}
	if bySource["sk-custom-key"]["calls"] != int64(1) || bySource["sk-custom-key"]["auth_type"] != "apikey" {
		t.Fatalf("custom source row = %#v", bySource["sk-custom-key"])
	}
	if len(result["account_stats"].([]map[string]any)) != 2 || len(result["api_key_stats"].([]map[string]any)) != 2 {
		t.Fatalf("legacy dimensions must remain available: %#v", result)
	}
}

func TestAccountSnapshotUsesAuthIDAndMatchesFilters(t *testing.T) {
	row := eventRow{AuthID: "fallback-account", APIKeyHash: "key-1"}
	if accountSnapshot(row) != "fallback-account" {
		t.Fatalf("account snapshot = %q", accountSnapshot(row))
	}
	if !matches(row, AnalyticsRequest{Accounts: []string{"fallback-account"}, APIKeyHashes: []string{"key-1"}}) {
		t.Fatal("fallback account and API key filters must match")
	}

	empty := eventRow{}
	if accountSnapshot(empty) != "unknown" || apiKeySnapshot(empty) != "unknown" || sourceSnapshot(empty) != "unknown" {
		t.Fatalf("empty snapshots = %q / %q / %q", accountSnapshot(empty), apiKeySnapshot(empty), sourceSnapshot(empty))
	}
	result := aggregate([]eventRow{empty}, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	combined := result["account_api_key_stats"].([]map[string]any)
	if len(combined) != 1 || combined[0]["source"] != "unknown" {
		t.Fatalf("empty source row = %#v", combined)
	}
}

func TestEventJSONIncludesSourceAndAuthType(t *testing.T) {
	value := eventJSON(eventRow{AuthID: "fallback-account", Source: "oauth@example.com", AuthType: "oauth"}, Price{})
	if value["account_snapshot"] != "fallback-account" || value["source"] != "oauth@example.com" || value["auth_type"] != "oauth" {
		t.Fatalf("event JSON = %#v", value)
	}

	empty := eventJSON(eventRow{}, Price{})
	if empty["source"] != "unknown" {
		t.Fatalf("empty event source = %#v", empty["source"])
	}
}

func TestEventJSONIncludesRequestProtocol(t *testing.T) {
	websocket := eventJSON(eventRow{ExecutorType: "CodexWebsocketsExecutor"}, Price{})
	if websocket["executor_type"] != "CodexWebsocketsExecutor" || websocket["protocol"] != "websocket" {
		t.Fatalf("websocket event JSON = %#v", websocket)
	}

	xai := eventJSON(eventRow{ExecutorType: "XAIWebsocketsExecutor"}, Price{})
	if xai["protocol"] != "websocket" {
		t.Fatalf("xai websocket protocol = %#v", xai["protocol"])
	}

	httpRow := eventJSON(eventRow{ExecutorType: "OpenAICompatExecutor"}, Price{})
	if httpRow["executor_type"] != "OpenAICompatExecutor" || httpRow["protocol"] != "http" {
		t.Fatalf("http event JSON = %#v", httpRow)
	}

	empty := eventJSON(eventRow{}, Price{})
	if empty["executor_type"] != "" || empty["protocol"] != "http" {
		t.Fatalf("empty protocol = %#v / %#v", empty["executor_type"], empty["protocol"])
	}
}

func TestCacheHitRateIsExposedForEventsAndAggregates(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "gpt-test", InputTokens: 1_000, CachedTokens: 400, TotalTokens: 1_000},
		{ID: 2, TimestampMS: 200, Provider: "anthropic", Model: "gpt-test", InputTokens: 100, CacheReadTokens: 30, CacheCreationTokens: 20, TotalTokens: 150},
	}
	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	summary := result["summary"].(map[string]any)
	if summary["cache_hit_tokens"] != int64(430) || summary["cache_hit_input_tokens"] != int64(1_150) {
		t.Fatalf("cache totals = %#v", summary)
	}
	if rate := summary["cache_hit_rate"].(float64); rate < 0.3739 || rate > 0.3740 {
		t.Fatalf("summary cache hit rate = %v, want about %v", rate, 430.0/1150.0)
	}
	modelStats := result["model_stats"].([]map[string]any)
	if len(modelStats) != 1 || modelStats[0]["cache_hit_rate"] != summary["cache_hit_rate"] {
		t.Fatalf("model cache hit rate = %#v, summary = %#v", modelStats, summary)
	}
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 2 || events[0]["cache_hit_rate"] != 0.4 || events[1]["cache_hit_rate"] != 0.2 {
		t.Fatalf("event cache hit rate = %#v", events)
	}
}

func TestAggregateHeatmapBucketsByUTCWeekdayAndHour(t *testing.T) {
	sundayAfternoon := time.Date(2024, 1, 7, 15, 10, 0, 0, time.UTC).UnixMilli()
	mondayMorning := time.Date(2024, 1, 8, 9, 5, 0, 0, time.UTC).UnixMilli()
	result := aggregate([]eventRow{
		{ID: 1, TimestampMS: sundayAfternoon, Model: "a", TotalTokens: 10, Failed: 1},
		{ID: 2, TimestampMS: sundayAfternoon + 1000, Model: "a", TotalTokens: 20},
		{ID: 3, TimestampMS: mondayMorning, Model: "b", TotalTokens: 5},
	}, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	points := result["heatmap"].([]map[string]any)
	if len(points) != 2 {
		t.Fatalf("heatmap points = %d, want 2: %#v", len(points), points)
	}
	byKey := map[string]map[string]any{}
	for _, point := range points {
		byKey[fmt.Sprintf("%v-%v", point["weekday"], point["hour"])] = point
	}
	sunday := byKey["0-15"]
	if sunday == nil || sunday["calls"] != int64(2) {
		t.Fatalf("sunday afternoon = %#v", sunday)
	}
	if sunday["failure_rate"].(float64) < 0.49 || sunday["failure_rate"].(float64) > 0.51 {
		t.Fatalf("sunday failure rate = %#v", sunday["failure_rate"])
	}
	monday := byKey["1-9"]
	if monday == nil || monday["calls"] != int64(1) {
		t.Fatalf("monday morning = %#v", monday)
	}
}

func TestCacheHitRateClampsMalformedValues(t *testing.T) {
	if got := cacheHitRate(1_500, 1_000); got != 1 {
		t.Fatalf("cache hit rate = %v, want 1", got)
	}
	if got := cacheHitRate(1_500, 0); got != 0 {
		t.Fatalf("empty cache hit rate = %v, want 0", got)
	}
}
