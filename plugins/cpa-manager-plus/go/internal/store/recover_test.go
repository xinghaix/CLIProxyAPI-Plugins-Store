package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsCorrupt(t *testing.T) {
	if !IsCorrupt(fmt.Errorf("insert usage event: database disk image is malformed (11)")) {
		t.Fatal("expected corrupt")
	}
	if IsCorrupt(fmt.Errorf("constraint failed")) {
		t.Fatal("constraint is not corrupt")
	}
	if IsCorrupt(nil) {
		t.Fatal("nil is not corrupt")
	}
}

func TestRebuildUsageEventsPreservesRows(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for i := 1; i <= 3; i++ {
		if _, _, err := store.InsertEventsCommitted(ctx, []Event{{
			Hash: fmt.Sprintf("h%d", i), TimestampMS: int64(i), Model: "gpt-test",
		}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.rebuildUsageEvents(ctx); err != nil {
		t.Fatal(err)
	}
	count, err := store.EventCount(ctx)
	if err != nil || count != 3 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if _, _, err := store.InsertEventsCommitted(ctx, []Event{{
		Hash: "h4", TimestampMS: 4, Model: "gpt-test",
	}}); err != nil {
		t.Fatalf("insert after rebuild: %v", err)
	}
}

func TestOpenRepairsCorruptWAL(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.InsertEventsCommitted(ctx, []Event{{
		Hash: "keep-me", TimestampMS: time.Now().UnixMilli(), Model: "gpt-test",
	}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "usage.sqlite")
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+"-wal", bytes.Repeat([]byte{0xFF}, 8192), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err = Open(ctx, dir)
	if err != nil {
		t.Fatalf("open after corrupt wal: %v", err)
	}
	defer store.Close()
	count, err := store.EventCount(ctx)
	if err != nil || count < 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if _, _, err := store.InsertEventsCommitted(ctx, []Event{{
		Hash: "after-repair", TimestampMS: time.Now().UnixMilli(), Model: "gpt-test",
	}}); err != nil {
		t.Fatalf("insert after wal repair: %v", err)
	}
}
