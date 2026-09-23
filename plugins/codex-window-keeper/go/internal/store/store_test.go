package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/clock"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/config"
)

func TestRestartResumesSameDatabase(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	original, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Default()
	settings.Enabled = true
	if err := original.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := original.SaveManagementKey(ctx, "secret-key"); err != nil {
		t.Fatal(err)
	}
	if err := original.TouchAccount(ctx, Account{AuthID: "future", Email: "future@example"}); err != nil {
		t.Fatal(err)
	}
	if err := original.TouchAccount(ctx, Account{AuthID: "due", Email: "due@example"}); err != nil {
		t.Fatal(err)
	}
	if err := original.SetNotBefore(ctx, "future", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := original.SetNotBefore(ctx, "due", now.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := original.StartAttempt(ctx, Attempt{AccountID: "due", GenerationKey: "gen-1", StartedAt: now, AttemptNo: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := original.Claim(ctx, "due", "old", now, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.ReleaseAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := reopened.RequeueStarted(ctx, now); err != nil {
		t.Fatal(err)
	}
	accounts, err := reopened.ListAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]Account{}
	for _, account := range accounts {
		found[account.AuthID] = account
	}
	if !found["future"].NotBefore.Equal(now.Add(time.Hour)) {
		t.Fatalf("future = %s", found["future"].NotBefore)
	}
	if found["due"].NotBefore.After(now) {
		t.Fatalf("due = %s", found["due"].NotBefore)
	}
	attempts, err := reopened.ListAttempts(ctx, 10)
	if err != nil || len(attempts) != 1 || attempts[0].Status != "started" {
		t.Fatalf("attempts = %+v err %v", attempts, err)
	}
	loaded, ok, err := reopened.LoadSettings(ctx)
	if err != nil || !ok || !loaded.Enabled {
		t.Fatalf("settings %+v %v %v", loaded, ok, err)
	}
	key, ok, err := reopened.LoadManagementKey(ctx)
	if err != nil || !ok || key != "secret-key" {
		t.Fatalf("key %q ok %v err %v", key, ok, err)
	}
	claimed, err := reopened.Claim(ctx, "due", "new", now, now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim %v %v", claimed, err)
	}
	again, err := reopened.Claim(ctx, "due", "other", now, now.Add(time.Minute))
	if err != nil || again {
		t.Fatalf("second claim %v %v", again, err)
	}
	if _, err := reopened.StartAttempt(ctx, Attempt{AccountID: "due", GenerationKey: "gen-1", StartedAt: now, AttemptNo: 2}); err != nil {
		t.Fatal(err)
	}
	id := attempts[0].ID
	if err := reopened.FinishAttempt(ctx, id, "succeeded", 200, "", "OK", "resp"); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.StartAttempt(ctx, Attempt{AccountID: "due", GenerationKey: "gen-1", StartedAt: now, AttemptNo: 3}); err != nil {
		t.Fatal(err)
	}
	second, err := reopened.ListAttempts(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.FinishAttempt(ctx, second[0].ID, "succeeded", 200, "", "OK", "resp"); err == nil {
		t.Fatal("expected unique success generation")
	}
	count := 0
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err == nil && strings.HasSuffix(entry.Name(), ".sqlite") {
			count++
		}
		return nil
	})
	if err != nil || count != 1 {
		t.Fatalf("sqlite files = %d err %v", count, err)
	}
}

func TestSaveWindowsMarksMissingAbsent(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	db, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.TouchAccount(ctx, Account{AuthID: "a", Email: "a@example"}); err != nil {
		t.Fatal(err)
	}
	end := time.Now().UTC().Truncate(time.Millisecond)
	if err := db.SaveWindows(ctx, "a", []clock.State{{LimitID: "codex", Slot: "primary", PeriodSeconds: 18000, Kind: "five_hour", EndsAt: end, Phase: clock.PhaseBlocked, SeenBlocked: true, Gating: true}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveWindows(ctx, "a", []clock.State{{LimitID: "codex", Slot: "secondary", PeriodSeconds: 604800, Kind: "weekly", EndsAt: end, Phase: clock.PhaseClear, Gating: true}}); err != nil {
		t.Fatal(err)
	}
	states, err := db.LoadWindows(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	absent := 0
	for _, state := range states {
		if state.Absent {
			absent++
		}
	}
	if len(states) != 2 || absent != 1 {
		t.Fatalf("states = %+v", states)
	}
}
