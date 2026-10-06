package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/ingest"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestRuntimeCloseTimeoutEventuallyClosesStore(t *testing.T) {
	t.Parallel()
	db, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, cancel := context.WithCancel(context.Background())
	r := &Runtime{store: db, cancel: cancel}
	r.wait.Add(1) // Model an existing worker/host callback that ignores cancellation.
	if err := r.Close(); err == nil || !strings.Contains(err.Error(), "10 seconds") {
		r.wait.Done()
		t.Fatalf("Close must report timeout, got %v", err)
	}
	if _, err := db.EventCount(context.Background()); err != nil {
		r.wait.Done()
		t.Fatalf("store closed before the worker exited: %v", err)
	}
	r.wait.Done()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, err := db.EventCount(context.Background())
		if err != nil && strings.Contains(err.Error(), "database is closed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("store leaked after timed-out worker exited: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeCloseSharesInflightFlushShutdownBudget(t *testing.T) {
	t.Parallel()
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
	if _, err := lock.Exec("begin immediate"); err != nil {
		t.Fatal(err)
	}
	defer lock.Exec("rollback")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Runtime{store: db, cancel: cancel}
	r.writer = ingest.NewWriter(db, 4, 1, nil)
	r.writer.EnqueueEvent(store.Event{Hash: "inflight", TimestampMS: 1, Model: "test"})
	r.wait.Add(1)
	go func() { defer r.wait.Done(); r.writer.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for r.writer.Depth() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("writer did not consume the event")
		}
		time.Sleep(time.Millisecond)
	}
	// A consumed batch of size one is now blocked on the real SQLite write lock.
	if err := r.Close(); err != nil && !strings.Contains(err.Error(), "10 seconds") {
		t.Fatal(err)
	}
	// SQLite's busy handler may finish after context cancellation. A persistent
	// lock can still hit Close's timeout, but must not restart an eight-second budget.
	stopped := make(chan struct{})
	go func() { r.wait.Wait(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight flush restarted the final shutdown budget")
	}
	deadline = time.Now().Add(time.Second)
	for {
		_, err := db.EventCount(context.Background())
		if err != nil && strings.Contains(err.Error(), "database is closed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("store remained open after contention worker exited: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if r.writer.Failed() == 0 || r.writer.LastError() == "" {
		t.Fatal("persistent contention must expose write failure")
	}
}

func TestRuntimeCloseRetriesInflightBatchAfterContention(t *testing.T) {
	t.Parallel()
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
	if _, err := lock.Exec("begin immediate"); err != nil {
		t.Fatal(err)
	}
	defer lock.Exec("rollback")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &Runtime{store: db, cancel: cancel}
	committed := make(chan []store.Event, 2)
	r.writer = ingest.NewWriter(db, 4, 1, func(events []store.Event) { committed <- events })
	event := store.Event{Hash: "retry", TimestampMS: 1, Model: "test"}
	r.writer.EnqueueEvent(event)
	r.writer.EnqueueEvent(event) // Duplicate queue item must not duplicate callback effects.
	r.wait.Add(1)
	go func() { defer r.wait.Done(); r.writer.Run(ctx) }()
	deadline := time.Now().Add(2 * time.Second)
	for r.writer.Depth() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("writer did not begin the first batch")
		}
		time.Sleep(time.Millisecond)
	}
	done := make(chan error, 1)
	go func() { done <- r.Close() }()
	deadline = time.Now().Add(7 * time.Second)
	for r.writer.Failed() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("in-flight contention did not exercise a failed write")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := lock.Exec("rollback"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	select {
	case events := <-committed:
		if len(events) != 1 || events[0].Hash != "retry" {
			t.Fatalf("committed = %#v", events)
		}
	default:
		t.Fatal("retained in-flight batch was lost")
	}
	if len(committed) != 0 || r.writer.Depth() != 0 || r.writer.Dropped() != 0 || r.writer.LastError() != "" {
		t.Fatalf("callbacks=%d depth=%d dropped=%d error=%q", len(committed), r.writer.Depth(), r.writer.Dropped(), r.writer.LastError())
	}
	reopened, err := store.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if n, err := reopened.EventCount(context.Background()); err != nil || n != 1 {
		t.Fatalf("count=%d err=%v", n, err)
	}
}
