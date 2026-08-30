package store

import (
	"context"
	"testing"
	"time"
)

func TestInsertFailedEventPersistsUsage(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, committed, err := store.InsertEventsCommitted(context.Background(), []Event{{
		Hash: "h1", TimestampMS: time.Now().UnixMilli(), Model: "gpt-test", Failed: true, AuthID: "codex.json", FailSummary: "boom",
	}})
	if err != nil {
		t.Fatalf("insert failed event: %v", err)
	}
	if len(committed) != 1 {
		t.Fatalf("committed=%d", len(committed))
	}
	count, err := store.EventCount(context.Background())
	if err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestPingWriteSucceeds(t *testing.T) {
	store, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.PingWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
}
