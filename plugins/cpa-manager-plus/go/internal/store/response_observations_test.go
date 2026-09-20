package store

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func observationStore(t *testing.T) *Store {
	t.Helper()
	db, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func observationItems(t *testing.T, db *Store, req AnalyticsRequest) []map[string]any {
	t.Helper()
	result, err := db.Analytics(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return result["events"].(map[string]any)["items"].([]map[string]any)
}

func assertResponseMetadata(t *testing.T, item map[string]any, chosen, host, observed, source string, conflict bool) {
	t.Helper()
	want := map[string]any{"response_model": chosen, "host_response_model": host, "observed_response_model": observed, "response_model_source": source, "response_model_conflict": conflict}
	for key, value := range want {
		if item[key] != value {
			t.Fatalf("%s=%#v, want %#v; item=%#v", key, item[key], value, item)
		}
	}
}

func TestResolveResponseModelConservativeEdges(t *testing.T) {
	for _, tc := range []struct {
		name, host, observed       string
		present, ambiguous, failed bool
		count                      int64
		want                       responseModelMetadata
	}{
		{name: "no observation despite shared key", host: "host", count: 2, want: responseModelMetadata{Model: "host", Host: "host", Source: "host"}},
		{name: "blank observation is not confirmed", host: "host", present: true, count: 1, want: responseModelMetadata{Model: "host", Host: "host", Source: "host"}},
		{name: "ambiguous host preserved", host: "host", observed: "model", present: true, ambiguous: true, count: 1, want: responseModelMetadata{Model: "host", Host: "host", Source: "host", Conflict: true}},
		{name: "failed host preserved", host: "host", observed: "model", present: true, failed: true, count: 1, want: responseModelMetadata{Model: "host", Host: "host", Source: "host"}},
		{name: "empty models not confirmed", present: true, count: 1, want: responseModelMetadata{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveResponseModel(tc.host, tc.observed, tc.present, tc.ambiguous, tc.count, tc.failed); got != tc.want {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
		})
	}
}

func TestResponseObservationArrivalOrdersAndHostResolution(t *testing.T) {
	ctx := context.Background()
	for _, first := range []bool{true, false} {
		for _, tc := range []struct {
			name, host, observed, chosen, source string
			conflict                             bool
		}{
			{"observer only", "", "upstream", "upstream", "observer", false},
			{"confirmed", "upstream", "upstream", "upstream", "confirmed", false},
			{"discordant", "host", "upstream", "host", "host", true},
			{"host only", "host", "", "host", "host", false},
		} {
			t.Run(fmt.Sprintf("%s/observer_first=%v", tc.name, first), func(t *testing.T) {
				db := observationStore(t)
				event := Event{Hash: "event", TimestampMS: 1, Model: "billed", ResponseModel: tc.host, ResponseCorrelationKey: "key"}
				record := func() {
					if tc.observed != "" {
						if err := db.RecordResponseObservation(ctx, "key", tc.observed, "evidence", false); err != nil {
							t.Fatal(err)
						}
					}
				}
				if first {
					record()
				}
				if n, _, err := db.InsertEventsCommitted(ctx, []Event{event, event}); err != nil || n != 1 {
					t.Fatalf("insert=%d %v", n, err)
				}
				if !first {
					record()
				}
				record() // Exact duplicate evidence must remain usable.
				assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 2, Limit: 10})[0], tc.chosen, tc.host, tc.observed, tc.source, tc.conflict)
				if n, err := db.EventCount(ctx); err != nil || n != 1 {
					t.Fatalf("count=%d %v", n, err)
				}
				var host sql.NullString
				if err := db.db.QueryRowContext(ctx, "select response_model from usage_events").Scan(&host); err != nil {
					t.Fatal(err)
				}
				if host.String != tc.host {
					t.Fatalf("observer overwrote host column: %q", host.String)
				}
			})
		}
	}
}

func TestResponseObservationLaterConflictRevokesPermanently(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, model, evidence string
		ambiguous             bool
	}{
		{"different model", "other", "evidence", false},
		{"different evidence", "upstream", "other", false},
		{"explicit ambiguous", "", "evidence", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.InsertEvents(ctx, []Event{{Hash: "e", TimestampMS: 1, Model: "billed", ResponseCorrelationKey: "key"}}); err != nil {
				t.Fatal(err)
			}
			if err := db.RecordResponseObservation(ctx, "key", "upstream", "evidence", false); err != nil {
				t.Fatal(err)
			}
			assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 2, Limit: 10})[0], "upstream", "", "upstream", "observer", false)
			if err := db.RecordResponseObservation(ctx, "key", tc.model, tc.evidence, tc.ambiguous); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.RecordResponseObservation(ctx, "key", "upstream", "evidence", false); err != nil {
				t.Fatal(err)
			}
			assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 2, Limit: 10})[0], "", "", "", "", true)
		})
	}
}

func TestResponseObservationGlobalUsageUniqueness(t *testing.T) {
	ctx := context.Background()
	for _, secondFailed := range []bool{false, true} {
		t.Run(fmt.Sprint(secondFailed), func(t *testing.T) {
			db := observationStore(t)
			first := Event{Hash: "one", TimestampMS: 10, Model: "billed", Provider: "p", ResponseCorrelationKey: "key"}
			if _, err := db.InsertEvents(ctx, []Event{first, first}); err != nil {
				t.Fatal(err)
			}
			if err := db.RecordResponseObservation(ctx, "key", "upstream", "evidence", false); err != nil {
				t.Fatal(err)
			}
			assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 10, Limit: 1})[0], "upstream", "", "upstream", "observer", false)
			// Different event hash, outside time/model/provider filters, still invalidates.
			second := Event{Hash: "two", TimestampMS: 100, Model: "split", Provider: "other", Failed: secondFailed, ResponseCorrelationKey: "key"}
			if _, err := db.InsertEvents(ctx, []Event{second}); err != nil {
				t.Fatal(err)
			}
			items := observationItems(t, db, AnalyticsRequest{ToMS: 10, Limit: 1, Models: []string{"billed"}, Providers: []string{"p"}})
			if len(items) != 1 {
				t.Fatalf("items=%#v", items)
			}
			assertResponseMetadata(t, items[0], "", "", "", "", true)
			if !secondFailed {
				for _, item := range observationItems(t, db, AnalyticsRequest{ToMS: 200, Limit: 10}) {
					assertResponseMetadata(t, item, "", "", "", "", true)
				}
			}
		})
	}
}

func TestResponseObservationMissingKeysAndFailedUsage(t *testing.T) {
	ctx := context.Background()
	db := observationStore(t)
	for i, event := range []Event{
		{Hash: "missing", TimestampMS: 1, Model: "billed", ResponseModel: "host"},
		{Hash: "no observation", TimestampMS: 2, Model: "billed", ResponseCorrelationKey: "no-observation"},
		{Hash: "failed", TimestampMS: 3, Model: "billed", Failed: true, ResponseCorrelationKey: "failed"},
	} {
		if _, err := db.InsertEvents(ctx, []Event{event}); err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			if err := db.RecordResponseObservation(ctx, "failed", "upstream", "evidence", false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.RecordResponseObservation(ctx, "", "upstream", "evidence", false); err != nil {
		t.Fatal(err)
	}
	items := observationItems(t, db, AnalyticsRequest{ToMS: 10, Limit: 10, IncludeFailed: true})
	assertResponseMetadata(t, items[0], "", "", "", "", false)
	assertResponseMetadata(t, items[1], "", "", "", "", false)
	assertResponseMetadata(t, items[2], "host", "host", "", "host", false)
}

func TestResponseObservationsLeaveBillingAndGroupsUnchanged(t *testing.T) {
	ctx := context.Background()
	db := observationStore(t)
	if err := db.ReplacePrices(ctx, map[string]Price{"billed": {Prompt: 2}, "upstream": {Prompt: 99}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertEvents(ctx, []Event{{Hash: "e", TimestampMS: 1, Model: "billed", Alias: "requested", ResponseCorrelationKey: "key", InputTokens: 1_000_000, TotalTokens: 1_000_000}}); err != nil {
		t.Fatal(err)
	}
	req := AnalyticsRequest{ToMS: 10, Limit: 10}
	before, err := db.Analytics(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RecordResponseObservation(ctx, "key", "upstream", "evidence", false); err != nil {
		t.Fatal(err)
	}
	after, err := db.Analytics(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range before {
		if key == "events" || key == "generated_at_ms" {
			continue
		}
		if !reflect.DeepEqual(after[key], want) {
			t.Fatalf("%s changed: %#v != %#v", key, after[key], want)
		}
	}
	item := after["events"].(map[string]any)["items"].([]map[string]any)[0]
	if item["model"] != "billed" || item["resolved_model"] != "billed" || item["cost"] != float64(2) {
		t.Fatalf("billing changed: %#v", item)
	}
	for _, req := range []AnalyticsRequest{{ToMS: 10, Limit: 10, Models: []string{"upstream"}}, {ToMS: 10, Limit: 10, Search: "upstream"}} {
		if items := observationItems(t, db, req); len(items) != 0 {
			t.Fatalf("observer altered filters: %#v", items)
		}
	}
}

func TestResponseObservationMigrationAndRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertEvents(ctx, []Event{{Hash: "legacy", TimestampMS: 1, Model: "billed"}}); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{"drop index idx_usage_events_response_correlation", "alter table usage_events drop column response_correlation_key", "drop table response_observations"} {
		if _, err := db.db.ExecContext(ctx, statement); err != nil {
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
		var key sql.NullString
		if err := db.db.QueryRowContext(ctx, "select response_correlation_key from usage_events where event_hash='legacy'").Scan(&key); err != nil {
			t.Fatal(err)
		}
		if key.Valid {
			t.Fatalf("invented historical key: %#v", key)
		}
		assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 1, Limit: 10})[0], "", "", "", "", false)
		if pass == 0 {
			if _, err := db.InsertEvents(ctx, []Event{{Hash: "new", TimestampMS: 2, Model: "billed", ResponseModel: "host", ResponseCorrelationKey: "key"}}); err != nil {
				t.Fatal(err)
			}
			if err := db.RecordResponseObservation(ctx, "key", "observed", "evidence", false); err != nil {
				t.Fatal(err)
			}
			if err := db.rebuildUsageEvents(ctx); err != nil {
				t.Fatal(err)
			}
		}
		assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{FromMS: 2, ToMS: 3, Limit: 10})[0], "host", "host", "observed", "host", true)
		var indexes int
		if err := db.db.QueryRowContext(ctx, "select count(*) from sqlite_master where type='index' and name='idx_usage_events_response_correlation' and tbl_name='usage_events'").Scan(&indexes); err != nil || indexes != 1 {
			t.Fatalf("index=%d %v", indexes, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestResponseObservationCleanupProtectsReferencedQuarantine(t *testing.T) {
	ctx := context.Background()
	db := observationStore(t)
	if err := db.RecordResponseObservation(ctx, "keep", "model", "evidence", true); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertEvents(ctx, []Event{{Hash: "e", TimestampMS: 1, Model: "billed", ResponseCorrelationKey: "keep"}}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * responseObservationTTL).UnixMilli()
	if _, err := db.db.ExecContext(ctx, "update response_observations set updated_at_ms=?", old); err != nil {
		t.Fatal(err)
	}
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxOrphanResponseObservations+2; i++ {
		if _, err := tx.ExecContext(ctx, "insert into response_observations(correlation_key,model,evidence_id,updated_at_ms) values(?,?,?,?)", fmt.Sprintf("orphan-%05d", i), "model", "evidence", time.Now().UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.ExecContext(ctx, "insert into response_observations(correlation_key,model,evidence_id,updated_at_ms) values('expired','model','evidence',?)", old); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordResponseObservation(ctx, "trigger", "model", "evidence", false); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.db.QueryRowContext(ctx, "select count(*) from response_observations where referenced=0").Scan(&n); err != nil || n != maxOrphanResponseObservations {
		t.Fatalf("orphans=%d %v", n, err)
	}
	if err := db.db.QueryRowContext(ctx, "select count(*) from response_observations where correlation_key='expired'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("expired=%d %v", n, err)
	}
	if err := db.RecordResponseObservation(ctx, "keep", "model", "evidence", false); err != nil {
		t.Fatal(err)
	}
	assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 2, Limit: 10})[0], "", "", "", "", true)
}

func TestResponseObservationSuppressionThenDurableQuarantine(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertEvents(ctx, []Event{
		{Hash: "e", TimestampMS: 1, Model: "billed", ResponseModel: "host", ResponseCorrelationKey: "key"},
		{Hash: "missing", TimestampMS: 2, Model: "billed", ResponseModel: "host", ResponseCorrelationKey: "missing"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordResponseObservation(ctx, "key", "host", "evidence", false); err != nil {
		t.Fatal(err)
	}
	// Holding the only SQL connection proves suppression itself cannot write/block.
	tx, err := db.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	db.SuppressResponseObservations()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	items := observationItems(t, db, AnalyticsRequest{ToMS: 3, Limit: 10})
	assertResponseMetadata(t, items[0], "host", "host", "", "host", false)
	assertResponseMetadata(t, items[1], "host", "host", "", "host", true)
	var ambiguous int
	if err := db.db.QueryRowContext(ctx, "select ambiguous from response_observations where correlation_key='key'").Scan(&ambiguous); err != nil || ambiguous != 0 {
		t.Fatalf("suppression performed SQL write: %d %v", ambiguous, err)
	}
	if err := db.QuarantineResponseObservations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.responseObservationsSuppressed.Load() {
		t.Fatal("new Store inherited in-memory suppression")
	}
	if err := db.RecordResponseObservation(ctx, "key", "host", "evidence", false); err != nil {
		t.Fatal(err)
	}
	assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 3, Limit: 10})[1], "host", "host", "", "host", true)
}

func TestQuarantineResponseObservationsPreservesHost(t *testing.T) {
	ctx := context.Background()
	db := observationStore(t)
	if _, err := db.InsertEvents(ctx, []Event{{Hash: "e", TimestampMS: 1, Model: "billed", ResponseModel: "host", ResponseCorrelationKey: "key"}}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordResponseObservation(ctx, "key", "host", "evidence", false); err != nil {
		t.Fatal(err)
	}
	assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 2, Limit: 10})[0], "host", "host", "host", "confirmed", false)
	if err := db.QuarantineResponseObservations(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordResponseObservation(ctx, "key", "host", "evidence", false); err != nil {
		t.Fatal(err)
	}
	assertResponseMetadata(t, observationItems(t, db, AnalyticsRequest{ToMS: 2, Limit: 10})[0], "host", "host", "", "host", true)
}
