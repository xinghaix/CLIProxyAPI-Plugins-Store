package app

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestRuntimePersistsUsageAndStops(t *testing.T) {
	dir := t.TempDir()
	runtime, err := New([]byte("data_dir: " + dir + "\nqueue_capacity: 4\nbatch_size: 1\n"))
	if err != nil {
		t.Fatal(err)
	}
	runtime.HandleUsage(pluginapi.UsageRecord{Model: "gpt-test", Provider: "codex", RequestedAt: time.Now(), Detail: pluginapi.UsageDetail{InputTokens: 3, OutputTokens: 5, TotalTokens: 8}})
	deadline := time.Now().Add(2 * time.Second)
	for {
		count, err := runtime.Store().EventCount(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if count == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("usage record was not persisted")
		}
		time.Sleep(10 * time.Millisecond)
	}
	health := runtime.Health(context.Background())
	if health["runtime"] != "local" || health["event_count"] != int64(1) {
		t.Fatalf("health = %#v", health)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "usage.sqlite")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "master.key")); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeEncryptsConnectionAndCallsHostForEnable(t *testing.T) {
	dir := t.TempDir()
	runtime, err := New([]byte("data_dir: " + dir))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.UpdateConnection(context.Background(), "http://127.0.0.1:8317", "secret"); err != nil {
		t.Fatal(err)
	}
	base, hasKey := runtime.Connection()
	if base != "http://127.0.0.1:8317" || !hasKey {
		t.Fatalf("connection = %q, %v", base, hasKey)
	}
	if err := runtime.Store().UpsertFailureCandidate(context.Background(), store.Event{Failed: true, AuthID: "codex.json", AuthIndex: "7", TimestampMS: time.Now().UnixMilli(), Provider: "codex", FailSummary: "failed"}); err != nil {
		t.Fatal(err)
	}
	candidates, err := runtime.Store().Candidates(context.Background(), "pending", 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates=%#v err=%v", candidates, err)
	}
	called := false
	runtime.SetHTTPDo(func(_ context.Context, method, target string, headers http.Header, body []byte) (pricesync.HTTPResponse, error) {
		called = method == http.MethodPatch && strings.Contains(target, "/v0/management/auth-files/status") && headers.Get("Authorization") == "Bearer secret"
		return pricesync.HTTPResponse{StatusCode: http.StatusOK}, nil
	})
	if err := runtime.ExecuteCandidate(context.Background(), candidates[0].ID, "enable"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("CPA enable endpoint was not invoked")
	}
	contents, err := os.ReadFile(filepath.Join(dir, "usage.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(contents), "secret") {
		t.Fatal("SQLite must not contain the CPA management key in plaintext")
	}
}

func TestInspectionUsesHostCredentialList(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	runtime.SetAuthList(func() ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{{Name: "codex.json", AuthIndex: "1", Provider: "codex", Email: "user@example.test", Status: "available"}}, nil
	})
	detail, err := runtime.RunInspection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	results, ok := detail["results"].([]store.InspectionResult)
	if !ok || len(results) != 1 || results[0].Action != "keep" {
		t.Fatalf("inspection detail = %#v", detail)
	}
}

func TestRuntimeRejectsDataDirectoryHotSwap(t *testing.T) {
	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.Reconfigure([]byte("data_dir: " + t.TempDir())); err == nil {
		t.Fatal("expected data_dir hot swap rejection")
	}
}

func TestNextTimePointDelayUsesProvidedClock(t *testing.T) {
	now := time.Date(2035, time.January, 2, 10, 15, 0, 0, time.UTC)
	delay, key := nextTimePointDelay(CodexInspectionSchedule{
		Mode:       "time_points",
		TimePoints: []string{"09:00", "11:30"},
		TimeZone:   "UTC",
	}, now, "")
	if delay != 75*time.Minute {
		t.Fatalf("delay = %s, want 1h15m", delay)
	}
	if key != "2035-01-02T11:30:00Z" {
		t.Fatalf("key = %q", key)
	}
}
