package store

import "testing"

func TestAggregateAccountAPIKeyStats(t *testing.T) {
	rows := []eventRow{
		{ID: 1, TimestampMS: 100, Model: "model", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai", TotalTokens: 10},
		{ID: 2, TimestampMS: 200, Model: "model", AuthIndex: "account-a", APIKeyHash: "key-1", Provider: "openai", TotalTokens: 20},
		{ID: 3, TimestampMS: 300, Model: "model", AuthIndex: "account-a", APIKeyHash: "key-2", Provider: "openai", TotalTokens: 30},
		{ID: 4, TimestampMS: 400, Model: "model", AuthIndex: "account-b", APIKeyHash: "key-1", Provider: "xai", TotalTokens: 40},
	}

	result := aggregate(rows, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	combined := result["account_api_key_stats"].([]map[string]any)
	if len(combined) != 3 {
		t.Fatalf("account_api_key_stats count = %d, want 3", len(combined))
	}

	byIdentity := map[string]map[string]any{}
	for _, row := range combined {
		byIdentity[row["account_snapshot"].(string)+"/"+row["api_key_hash"].(string)] = row
	}
	if calls := byIdentity["account-a/key-1"]["calls"]; calls != int64(2) {
		t.Fatalf("account-a/key-1 calls = %#v, want 2", calls)
	}
	if tokens := byIdentity["account-a/key-1"]["total_tokens"]; tokens != int64(30) {
		t.Fatalf("account-a/key-1 tokens = %#v, want 30", tokens)
	}
	if byIdentity["account-b/key-1"]["auth_provider_snapshot"] != "xai" {
		t.Fatalf("account-b/key-1 provider = %#v, want xai", byIdentity["account-b/key-1"]["auth_provider_snapshot"])
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
	if accountSnapshot(empty) != "unknown" || apiKeySnapshot(empty) != "unknown" {
		t.Fatalf("empty snapshots = %q / %q", accountSnapshot(empty), apiKeySnapshot(empty))
	}
	result := aggregate([]eventRow{empty}, nil, AnalyticsRequest{Limit: 100, Granularity: "hour"})
	combined := result["account_api_key_stats"].([]map[string]any)
	if len(combined) != 1 || combined[0]["account_snapshot"] != "unknown" || combined[0]["api_key_hash"] != "unknown" {
		t.Fatalf("empty composite row = %#v", combined)
	}
}

func TestEventJSONUsesAccountSnapshot(t *testing.T) {
	value := eventJSON(eventRow{AuthID: "fallback-account"}, Price{})
	if value["account_snapshot"] != "fallback-account" {
		t.Fatalf("account_snapshot = %#v", value["account_snapshot"])
	}
}
