package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/config"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/keeper"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/store"
)

func TestManagementConnectionAndResume(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings := config.Default()
	settings.Enabled = true
	if err := db.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := db.TouchAccount(ctx, store.Account{AuthID: "auth-1", AuthIndex: "idx-1"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetPause(ctx, "auth-1", "reauth"); err != nil {
		t.Fatal(err)
	}
	k := &keeper.Keeper{Store: db, Wake: make(chan struct{}, 1)}
	service := Service{Keeper: k}
	status, _ := service.Handle(ctx, http.MethodPut, "/v0/management/codex-window-keeper/connection", []byte(`{"base_url":"http://127.0.0.1:8317","management_key":"secret-test-key"}`))
	if status != http.StatusOK {
		t.Fatalf("connection status = %d", status)
	}
	key, keySet, err := db.LoadManagementKey(ctx)
	if err != nil || !keySet || key != "secret-test-key" {
		t.Fatalf("key state = %q %v %v", key, keySet, err)
	}
	status, body := service.Handle(ctx, http.MethodGet, "/v0/management/codex-window-keeper/settings", nil)
	if status != http.StatusOK || strings.Contains(string(body), "secret-test-key") {
		t.Fatalf("settings response leaks key or failed: %d %s", status, body)
	}
	status, body = service.Handle(ctx, http.MethodPost, "/v0/management/codex-window-keeper/accounts/resume", []byte(`{"auth_id":"auth-1"}`))
	if status != http.StatusOK {
		t.Fatalf("resume status = %d: %s", status, body)
	}
	accounts, err := db.ListAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].PauseReason != "" {
		t.Fatalf("accounts = %+v, %v", accounts, err)
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil || response["ok"] != true {
		t.Fatalf("response = %s, %v", body, err)
	}
}
