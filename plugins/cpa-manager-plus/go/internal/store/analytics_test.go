package store

import "testing"

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
