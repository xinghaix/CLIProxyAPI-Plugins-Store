package app

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestAutoBanUsage429DisablesAndStartsCooldown(t *testing.T) {
	ctx := context.Background()
	runtime, err := New([]byte("data_dir: " + t.TempDir() + "\nbatch_size: 1"))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := runtime.UpdateConnection(ctx, "http://127.0.0.1:8317", "test-key"); err != nil {
		t.Fatal(err)
	}
	runtime.SetAuthList(func() ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{{Name: "codex.json", AuthIndex: "codex-1", Provider: "codex", Status: "available"}}, nil
	})
	runtime.SetHTTPDo(func(_ context.Context, method, target string, _ http.Header, body []byte) (pricesync.HTTPResponse, error) {
		if method != http.MethodPatch || target != "http://127.0.0.1:8317/v0/management/auth-files/status" || len(body) == 0 {
			t.Fatalf("unexpected auto-ban CPA action: %s %s %s", method, target, body)
		}
		return pricesync.HTTPResponse{StatusCode: http.StatusOK}, nil
	})
	if err := runtime.UpdateAutoBanSettings(ctx, AutoBanSettings{
		Enabled:                   true,
		Sources:                   AutoBanSources{Usage: true, Inspection: true},
		SchedulerIntervalSeconds:  30,
		DefaultCodexCooldownHours: 5,
		HistoryRetentionDays:      90,
	}); err != nil {
		t.Fatal(err)
	}

	runtime.HandleUsage(pluginapi.UsageRecord{
		Provider: "codex", Model: "gpt-test", AuthIndex: "codex-1", AuthID: "account-1", Failed: true,
		Failure: pluginapi.UsageFailure{StatusCode: http.StatusTooManyRequests, Body: "rate limited"}, RequestedAt: time.Now(),
	})
	key := store.AutoBanAccountKey("codex", "oauth_auth_file", "codex-1", "", "account-1", "")
	deadline := time.Now().Add(2 * time.Second)
	for {
		state, err := runtime.Store().GetAutoBanAccount(ctx, key)
		if err == nil && state.State == store.AutoBanStateCooling {
			if state.CooldownUntilMS == nil || *state.CooldownUntilMS <= time.Now().UnixMilli() {
				t.Fatalf("invalid cooldown state: %#v", state)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("auto-ban state did not reach cooling: %#v, %v", state, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
