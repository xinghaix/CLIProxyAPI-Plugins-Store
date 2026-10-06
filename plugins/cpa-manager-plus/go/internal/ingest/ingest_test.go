package ingest

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestToEventCopiesMappedModelAndAlias(t *testing.T) {
	event := ToEvent(pluginapi.UsageRecord{
		Model:       "gpt-5",
		Alias:       "g5",
		RequestedAt: time.UnixMilli(1_700_000_000_000),
		Detail:      pluginapi.UsageDetail{TotalTokens: 8},
	})
	if event.Model != "gpt-5" {
		t.Fatalf("model = %q, want gpt-5", event.Model)
	}
	if event.Alias != "g5" {
		t.Fatalf("alias = %q, want g5", event.Alias)
	}
}

func TestToEventTrimsAliasAndLeavesItEmptyWhenMissing(t *testing.T) {
	event := ToEvent(pluginapi.UsageRecord{Model: " gpt-5 ", Alias: "  "})
	if event.Model != "gpt-5" {
		t.Fatalf("model = %q, want gpt-5", event.Model)
	}
	if event.Alias != "" {
		t.Fatalf("alias = %q, want empty", event.Alias)
	}
}

func TestKeepUnflushedBatchOnBusy(t *testing.T) {
	if !keepUnflushedBatch(fmt.Errorf("database is locked (517)")) {
		t.Fatal("busy insert must keep the batch")
	}
	if keepUnflushedBatch(fmt.Errorf("constraint failed")) {
		t.Fatal("non-busy insert must drop the batch")
	}
	if !keepUnflushedBatch(fmt.Errorf("insert usage event: database disk image is malformed (11)")) {
		t.Fatal("corrupt insert must keep the batch")
	}
	if keepUnflushedBatch(nil) {
		t.Fatal("success must not keep the batch")
	}
}

func TestToEventRewritesEpochTimestamp(t *testing.T) {
	before := time.Now().Add(-time.Second).UnixMilli()
	event := ToEvent(pluginapi.UsageRecord{Model: "gpt-5", RequestedAt: time.Unix(0, 0)})
	if event.TimestampMS <= 0 || event.TimestampMS < before {
		t.Fatalf("timestamp = %d, want current time", event.TimestampMS)
	}
}

func TestWriterShutdownRejectsLateEnqueues(t *testing.T) {
	db, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var w *Writer
	w = NewWriter(db, 10, 1, func([]store.Event) { w.EnqueueEvent(store.Event{Hash: "late", TimestampMS: 2, Model: "test"}) })
	w.EnqueueEvent(store.Event{Hash: "accepted", TimestampMS: 1, Model: "test"})
	w.Run(ctx)
	n, err := db.EventCount(context.Background())
	if err != nil || n != 1 || w.Depth() != 0 || w.Dropped() != 1 {
		t.Fatalf("count=%d depth=%d dropped=%d err=%v", n, w.Depth(), w.Dropped(), err)
	}
}

func TestWriterShutdownRetriesBusyWithinDeadline(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lock, err := sql.Open("sqlite", filepath.Join(dir, "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err = lock.Exec("begin immediate"); err != nil {
		t.Fatal(err)
	}
	defer lock.Exec("rollback")
	w := NewWriter(db, 10, 10, nil)
	w.EnqueueEvent(store.Event{Hash: "busy", TimestampMS: 1, Model: "test"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var released sync.WaitGroup
	released.Add(1)
	go func() { defer released.Done(); time.Sleep(5200 * time.Millisecond); _, err = lock.Exec("rollback") }()
	start := time.Now()
	w.Run(ctx)
	released.Wait()
	if err != nil {
		t.Fatal(err)
	}
	n, err := db.EventCount(context.Background())
	if err != nil || n != 1 || w.Depth() != 0 {
		t.Fatalf("busy retry lost event: count=%d depth=%d err=%v last=%s", n, w.Depth(), err, w.LastError())
	}
	if elapsed := time.Since(start); elapsed > 11*time.Second {
		t.Fatalf("shutdown unbounded: %s", elapsed)
	}
}

func TestWriterShutdownBusyDeadline(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	lock, err := sql.Open("sqlite", filepath.Join(dir, "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err = lock.Exec("begin immediate"); err != nil {
		t.Fatal(err)
	}
	defer lock.Exec("rollback")
	w := NewWriter(db, 10, 10, nil)
	w.EnqueueEvent(store.Event{Hash: "locked", TimestampMS: 1, Model: "test"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("shutdown hung on persistent SQLite lock")
	}
	if w.Failed() == 0 || w.LastError() == "" {
		t.Fatal("failed shutdown must expose write failure")
	}
}
