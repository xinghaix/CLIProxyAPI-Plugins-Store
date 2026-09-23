package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

func TestWindowKeeperAPIEndpoints(t *testing.T) {
	dir := t.TempDir()
	r, err := app.New([]byte("data_dir: " + dir))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx := context.Background()

	// 1. Get settings
	reqBody, _ := json.Marshal(Request{
		Method: http.MethodGet,
		Path:   "/v0/management/window-keeper/settings",
	})
	resp := Handle(ctx, r, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get settings status = %d: %s", resp.StatusCode, resp.Body)
	}
	var settingsResp map[string]any
	if err := json.Unmarshal(resp.Body, &settingsResp); err != nil {
		t.Fatal(err)
	}
	if settingsResp["settings"] == nil {
		t.Fatal("expected settings in response")
	}

	// 2. Put settings
	putPayload, _ := json.Marshal(windowkeeper.Settings{
		Enabled:     true,
		Model:       "gpt-5.4",
		Effort:      "medium",
		Prompt:      "Reply with OK",
		PollSeconds: 30,
		SkewSeconds: 5,
		MaxAttempts: 4,
	})
	reqBody, _ = json.Marshal(Request{
		Method: http.MethodPut,
		Path:   "/v0/management/window-keeper/settings",
		Body:   putPayload,
	})
	resp = Handle(ctx, r, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put settings status = %d: %s", resp.StatusCode, resp.Body)
	}

	// 3. Touch account and get accounts
	if err := r.Store().TouchWindowKeeperAccount(ctx, windowkeeper.Account{
		AuthID: "auth-1", AuthIndex: "idx-1", Email: "ada@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	reqBody, _ = json.Marshal(Request{
		Method: http.MethodGet,
		Path:   "/v0/management/window-keeper/accounts",
	})
	resp = Handle(ctx, r, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get accounts status = %d: %s", resp.StatusCode, resp.Body)
	}
	var accountsResp map[string]any
	if err := json.Unmarshal(resp.Body, &accountsResp); err != nil {
		t.Fatal(err)
	}
	accountsList, ok := accountsResp["accounts"].([]any)
	if !ok || len(accountsList) != 1 {
		t.Fatalf("expected 1 account, got: %+v", accountsResp)
	}

	// 4. Override via parameterized path
	overrideBody, _ := json.Marshal(map[string]string{
		"enabled": "off",
		"model":   "gpt-5.4-mini",
	})
	reqBody, _ = json.Marshal(Request{
		Method: http.MethodPut,
		Path:   "/v0/management/window-keeper/accounts/auth-1",
		Body:   overrideBody,
	})
	resp = Handle(ctx, r, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("override status = %d: %s", resp.StatusCode, resp.Body)
	}

	// 5. Resume via parameterized path
	if err := r.Store().SetWindowKeeperPause(ctx, "auth-1", "reauth"); err != nil {
		t.Fatal(err)
	}
	reqBody, _ = json.Marshal(Request{
		Method: http.MethodPost,
		Path:   "/v0/management/window-keeper/accounts/auth-1/resume",
	})
	resp = Handle(ctx, r, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume status = %d: %s", resp.StatusCode, resp.Body)
	}

	// 6. Attempts
	reqBody, _ = json.Marshal(Request{
		Method: http.MethodGet,
		Path:   "/v0/management/window-keeper/attempts",
	})
	resp = Handle(ctx, r, reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get attempts status = %d: %s", resp.StatusCode, resp.Body)
	}
}
