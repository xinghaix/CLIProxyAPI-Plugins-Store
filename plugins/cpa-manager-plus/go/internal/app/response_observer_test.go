package app

import (
	"context"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/config"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/responsemodel"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func observerRows(t *testing.T, db *store.Store) []map[string]any {
	t.Helper()
	result, err := db.Analytics(context.Background(), store.AnalyticsRequest{ToMS: 100, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	return result["events"].(map[string]any)["items"].([]map[string]any)
}

func TestResponseObserverOverflowQuarantinesDerivedMetadata(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.InsertEvents(ctx, []store.Event{
		{Hash: "derived-event", Model: "routed", TimestampMS: 1, ResponseCorrelationKey: "derived-key"},
		{Hash: "explicit-event", Model: "routed", TimestampMS: 2, ResponseModel: "host-model", ResponseCorrelationKey: "explicit-key"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"derived-key", "explicit-key"} {
		if err = db.RecordResponseObservation(ctx, key, "observed", "evidence", false); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := config.Parse(nil)
	if err != nil {
		t.Fatal(err)
	}
	o := &responseObserver{tracker: responsemodel.New(), queue: make(chan responsemodel.Observation, 1)}
	r := &Runtime{store: db, config: cfg, responseObserver: o}
	r.observeResponse(func(*responsemodel.Tracker) []responsemodel.Observation {
		return []responsemodel.Observation{{Key: "first", Model: "m", EvidenceID: "e"}, {Key: "second", Ambiguous: true}}
	})
	if !o.disabled.Load() || o.dropped.Load() != 1 {
		t.Fatalf("observer=%#v", r.responseObserverHealth())
	}
	// Suppress reads immediately, before the worker can persist quarantine.
	if rows := observerRows(t, db); rows[1]["response_model"] != "" || rows[1]["response_model_conflict"] != true {
		t.Fatalf("stale evidence visible after overflow: %#v", rows)
	}
	// No future callback may create evidence after a delivery gap.
	called := false
	r.observeResponse(func(*responsemodel.Tracker) []responsemodel.Observation { called = true; return nil })
	if called {
		t.Fatal("disabled observer still processes callbacks")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	cancel()
	r.runResponseObserver(workerCtx)
	rows := observerRows(t, db)
	if len(rows) != 2 {
		t.Fatalf("usage count changed: %#v", rows)
	}
	if rows[0]["response_model"] != "host-model" || rows[0]["response_model_source"] != "host" || rows[0]["response_model_conflict"] != true {
		t.Fatalf("explicit evidence lost: %#v", rows[0])
	}
	if rows[1]["response_model"] != "" || rows[1]["response_model_conflict"] != true {
		t.Fatalf("stale observer attribution survived: %#v", rows[1])
	}
}

func TestObserverRespectsCollectorAndShutdown(t *testing.T) {
	dir := t.TempDir()
	r, err := New([]byte("data_dir: " + dir + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	r.UpdateCollector(false)
	called := false
	r.observeResponse(func(*responsemodel.Tracker) []responsemodel.Observation { called = true; return nil })
	if called {
		t.Fatal("collector disabled but observer ran")
	}
	r.UpdateCollector(true)
	if _, err = r.store.InsertEvents(context.Background(), []store.Event{{Hash: "event", Model: "routed", TimestampMS: 1, ResponseCorrelationKey: "key"}}); err != nil {
		t.Fatal(err)
	}
	r.observeResponse(func(*responsemodel.Tracker) []responsemodel.Observation {
		return []responsemodel.Observation{{Key: "key", Model: "observed", EvidenceID: "evidence"}}
	})
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	r.observeResponse(func(*responsemodel.Tracker) []responsemodel.Observation { called = true; return nil })
	if called {
		t.Fatal("closed observer still ran")
	}
	db, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows := observerRows(t, db)
	if len(rows) != 1 || rows[0]["response_model"] != "observed" || rows[0]["model"] != "routed" {
		t.Fatalf("shutdown did not drain metadata: %#v", rows)
	}
}
