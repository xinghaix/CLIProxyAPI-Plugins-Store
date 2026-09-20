package store

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
)

func TestResponseModelPersistsWithoutChangingBilling(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplacePrices(ctx, map[string]Price{"billed": {Prompt: 2}, "upstream": {Prompt: 99}}); err != nil {
		t.Fatal(err)
	}
	for i, model := range []string{"", "billed", "upstream"} {
		event := Event{Hash: fmt.Sprint(i), TimestampMS: int64(i + 1), Model: "billed", Alias: "requested", ResponseModel: model, InputTokens: 1_000_000, TotalTokens: 1_000_000}
		count, committed, err := db.InsertEventsCommitted(ctx, []Event{event, event})
		if err != nil || count != 1 || len(committed) != 1 || committed[0].ResponseModel != model {
			t.Fatalf("insert %q = %d %#v %v", model, count, committed, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	result, err := db.Analytics(ctx, AnalyticsRequest{ToMS: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	items := result["events"].(map[string]any)["items"].([]map[string]any)
	if len(items) != 3 {
		t.Fatalf("items=%#v", items)
	}
	for i, want := range []string{"upstream", "billed", ""} {
		item := items[i]
		if item["response_model"] != want || item["resolved_model"] != "billed" || item["model"] != "billed" || item["requested_model"] != "requested" || item["cost"] != float64(2) {
			t.Fatalf("item=%#v", item)
		}
	}
	filtered, err := db.Analytics(ctx, AnalyticsRequest{ToMS: 100, Limit: 10, Models: []string{"upstream"}})
	if err != nil {
		t.Fatal(err)
	}
	if items := filtered["events"].(map[string]any)["items"].([]map[string]any); len(items) != 0 {
		t.Fatalf("response model changed filtering: %#v", items)
	}
}

func TestResponseModelMigratesOldDatabases(t *testing.T) {
	for _, removeAlias := range []bool{false, true} {
		t.Run(fmt.Sprint("without_alias=", removeAlias), func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			db, err := Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.InsertEvents(ctx, []Event{{Hash: "legacy", TimestampMS: 1, Model: "billed"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.ExecContext(ctx, "alter table usage_events drop column response_model"); err != nil {
				t.Fatal(err)
			}
			if removeAlias {
				if _, err := db.db.ExecContext(ctx, "alter table usage_events drop column alias"); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			for pass := 0; pass < 2; pass++ {
				db, err = Open(ctx, dir)
				if err != nil {
					t.Fatal(err)
				}
				var stored sql.NullString
				if err := db.db.QueryRowContext(ctx, "select response_model from usage_events where event_hash='legacy'").Scan(&stored); err != nil {
					t.Fatal(err)
				}
				if stored.Valid {
					t.Fatalf("invented historical response model: %#v", stored)
				}
				result, err := db.Analytics(ctx, AnalyticsRequest{ToMS: 1, Limit: 10})
				if err != nil {
					t.Fatal(err)
				}
				item := result["events"].(map[string]any)["items"].([]map[string]any)[0]
				if item["response_model"] != "" || item["resolved_model"] != "billed" {
					t.Fatalf("legacy projection=%#v", item)
				}
				if _, err := db.InsertEvents(ctx, []Event{{Hash: fmt.Sprint("new", pass), TimestampMS: 2, Model: "billed", ResponseModel: "upstream"}}); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
