package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestClampEventLimitAllowsThreeThousandWindow(t *testing.T) {
	if got := clampEventLimit(3_000); got != 3_000 {
		t.Fatalf("max window = %d, want 3000", got)
	}
	if got := clampEventLimit(1); got != 1 {
		t.Fatalf("min window = %d, want 1", got)
	}
	if got := clampEventLimit(0); got != 3_000 {
		t.Fatalf("empty limit = %d, want 3000", got)
	}
	if got := clampEventLimit(3_001); got != 3_000 {
		t.Fatalf("over max = %d, want 3000", got)
	}
}

func TestAggregateBackfillsProviderAcrossEventsForSameSource(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", Source: "sk-custom-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", Source: "sk-custom-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai-compatible-wzw.pp.ua", TotalTokens: 20},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 2 || events[0]["provider"] != "openai-compatible-wzw.pp.ua" || events[0]["auth_provider_snapshot"] != "openai-compatible-wzw.pp.ua" {
		t.Fatalf("backfilled events = %#v", events)
	}
	accounts := result["account_api_key_stats"].([]map[string]any)
	if len(accounts) != 1 || accounts[0]["auth_provider_snapshot"] != "openai-compatible-wzw.pp.ua" {
		t.Fatalf("backfilled account summary = %#v", accounts)
	}
}

func TestAggregateProviderBackfillUsesAuthIdentityForSharedAPIKey(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", Source: "sk-shared", AuthType: "apikey", AuthIndex: "auth-a", APIKeyHash: "key-shared", Provider: "openai-compatible-a.example", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", Source: "sk-shared", AuthType: "apikey", AuthIndex: "auth-b", APIKeyHash: "key-shared", Provider: "openai-compatible-b.example", TotalTokens: 20},
		{ID: 3, TimestampMS: 300, Model: "model", Source: "sk-shared", AuthType: "apikey", AuthIndex: "auth-a", APIKeyHash: "key-shared", TotalTokens: 30},
		{ID: 4, TimestampMS: 400, Model: "model", Source: "sk-shared", AuthType: "apikey", AuthIndex: "auth-b", APIKeyHash: "key-shared", TotalTokens: 40},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 4 || events[2]["provider"] != "openai-compatible-a.example" || events[3]["provider"] != "openai-compatible-b.example" {
		t.Fatalf("shared API-key providers were mixed = %#v", events)
	}
}

func TestAggregateProviderBackfillLeavesProviderUnknownWhenIdentityIsAmbiguous(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", Source: "sk-changing", AuthType: "apikey", AuthIndex: "auth-a", APIKeyHash: "key-changing", Provider: "openai-compatible-a.example", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", Source: "sk-changing", AuthType: "apikey", AuthIndex: "auth-a", APIKeyHash: "key-changing", Provider: "openai-compatible-b.example", TotalTokens: 20},
		{ID: 3, TimestampMS: 300, Model: "model", Source: "sk-changing", AuthType: "apikey", AuthIndex: "auth-a", APIKeyHash: "key-changing", TotalTokens: 30},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 3 || events[2]["provider"] != "" {
		t.Fatalf("ambiguous provider was invented = %#v", events)
	}
}

func TestAggregateProviderBackfillPrefersSourceSnapshot(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", Source: "sk-custom-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai-compatible-old.example", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", Source: "sk-other-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai-compatible-new.example", TotalTokens: 20},
		{ID: 3, TimestampMS: 300, Model: "model", Source: "sk-custom-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", TotalTokens: 30},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 3 || events[0]["provider"] != "openai-compatible-old.example" || events[2]["provider"] != "openai-compatible-old.example" {
		t.Fatalf("provider snapshots crossed or regressed = %#v", events)
	}
}

func TestAggregateAccountAPIKeyStatsByStableIdentity(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", Source: "oauth@example.com", AuthType: "oauth", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", Source: "oauth@example.com", AuthType: "oauth", AuthIndex: "account-b", APIKeyHash: "key-2", Provider: "xai", TotalTokens: 20},
		{ID: 3, TimestampMS: 300, Model: "model", Source: "sk-custom-key", AuthType: "apikey", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai", TotalTokens: 30},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	combined := result["account_api_key_stats"].([]map[string]any)
	if len(combined) != 3 {
		t.Fatalf("account_api_key_stats count = %d, want 3", len(combined))
	}

	byIdentity := map[string]map[string]any{}
	for _, row := range combined {
		byIdentity[row["id"].(string)] = row
	}
	firstOAuth := byIdentity["auth-index::oauth::openai::account-a"]
	if firstOAuth["calls"] != int64(1) || firstOAuth["total_tokens"] != int64(10) || firstOAuth["auth_provider_snapshot"] != "openai" {
		t.Fatalf("first OAuth identity = %#v", firstOAuth)
	}
	secondOAuth := byIdentity["auth-index::oauth::xai::account-b"]
	if secondOAuth["calls"] != int64(1) || secondOAuth["total_tokens"] != int64(20) || secondOAuth["auth_provider_snapshot"] != "xai" {
		t.Fatalf("second OAuth identity = %#v", secondOAuth)
	}
	custom := byIdentity["auth-index::apikey::openai::account-a"]
	if custom["calls"] != int64(1) || custom["auth_type"] != "apikey" || custom["source"] != "sk-custom-key" {
		t.Fatalf("custom identity = %#v", custom)
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

func TestEventJSONIncludesModelAlias(t *testing.T) {
	mapped := eventJSON(eventRow{Model: "gpt-5", Alias: "g5"}, Price{})
	if mapped["model"] != "gpt-5" || mapped["alias"] != "g5" || mapped["requested_model"] != "g5" || mapped["resolved_model"] != "gpt-5" {
		t.Fatalf("mapped event JSON = %#v", mapped)
	}

	same := eventJSON(eventRow{Model: "gpt-5", Alias: "gpt-5"}, Price{})
	if same["alias"] != "gpt-5" || same["requested_model"] != "gpt-5" || same["resolved_model"] != "gpt-5" {
		t.Fatalf("unmapped event JSON = %#v", same)
	}

	legacy := eventJSON(eventRow{Model: "gpt-5"}, Price{})
	if legacy["alias"] != "" || legacy["requested_model"] != "gpt-5" || legacy["resolved_model"] != "gpt-5" {
		t.Fatalf("legacy event JSON = %#v", legacy)
	}
}

func TestIncludesModelMatchesAliasOrMappedName(t *testing.T) {
	row := eventRow{Model: "gpt-5", Alias: "g5"}
	if !includesModel([]string{"g5"}, row) || !includesModel([]string{"gpt-5"}, row) {
		t.Fatal("filter must match requested alias or mapped model")
	}
	if includesModel([]string{"other"}, row) {
		t.Fatal("unrelated model filter must not match")
	}
	if !includesModel(nil, row) {
		t.Fatal("empty model filter must match")
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

func TestAnalyticsPersistsAndSearchesModelAlias(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.InsertEvents(ctx, []Event{
		{Hash: "alias-1", TimestampMS: 1_000, Model: "gpt-5", Alias: "g5", TotalTokens: 10},
		{Hash: "plain-1", TimestampMS: 2_000, Model: "claude-sonnet", TotalTokens: 4},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := database.Analytics(ctx, AnalyticsRequest{FromMS: 0, ToMS: 3_000, Limit: 10, Search: "g5", IncludeFailed: true})
	if err != nil {
		t.Fatal(err)
	}
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 1 || events[0]["model"] != "gpt-5" || events[0]["alias"] != "g5" {
		t.Fatalf("search by alias = %#v", events)
	}

	filtered, err := database.Analytics(ctx, AnalyticsRequest{FromMS: 0, ToMS: 3_000, Limit: 10, Models: []string{"g5"}, IncludeFailed: true})
	if err != nil {
		t.Fatal(err)
	}
	filteredEvents := filtered["events"].(map[string]any)["items"].([]map[string]any)
	if len(filteredEvents) != 1 || filteredEvents[0]["alias"] != "g5" {
		t.Fatalf("filter by alias = %#v", filteredEvents)
	}
}

func TestAnalyticsProviderFilterUsesSourceSnapshotForMissingEvents(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.InsertEvents(ctx, []Event{
		{Hash: "provider-missing", TimestampMS: 1_000, Model: "model", Source: "sk-custom-key", AuthType: "apikey", Provider: "", TotalTokens: 10},
		{Hash: "provider-known", TimestampMS: 2_000, Model: "model", Source: "sk-custom-key", AuthType: "apikey", Provider: "openai-compatible-wzw.pp.ua", TotalTokens: 20},
	}); err != nil {
		t.Fatal(err)
	}

	result, err := database.Analytics(ctx, AnalyticsRequest{
		FromMS: 0, ToMS: 3_000, Limit: 10,
		Providers: []string{"openai-compatible-wzw.pp.ua"}, IncludeFailed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	events := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(events) != 2 || events[0]["provider"] != "openai-compatible-wzw.pp.ua" || events[1]["provider"] != "openai-compatible-wzw.pp.ua" {
		t.Fatalf("provider-filtered events = %#v", events)
	}

	searched, err := database.Analytics(ctx, AnalyticsRequest{
		FromMS: 0, ToMS: 3_000, Limit: 10, Search: "wzw.pp.ua", IncludeFailed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	searchedEvents := searched["events"].(map[string]any)["items"].([]map[string]any)
	if len(searchedEvents) != 2 || searchedEvents[0]["provider"] != "openai-compatible-wzw.pp.ua" || searchedEvents[1]["provider"] != "openai-compatible-wzw.pp.ua" {
		t.Fatalf("provider-searched events = %#v", searchedEvents)
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
