package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/config"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/keeper"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/store"
)

type Service struct{ Keeper *keeper.Keeper }

func (s Service) Handle(ctx context.Context, method, path string, body []byte) (int, []byte) {
	if s.Keeper == nil || s.Keeper.Store == nil {
		return jsonStatus(http.StatusServiceUnavailable, map[string]any{"error": "runtime is not initialized"})
	}
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	path = strings.TrimPrefix(path, "/v0/management")
	switch {
	case method == http.MethodGet && path == "/codex-window-keeper/health":
		return s.health(ctx)
	case method == http.MethodGet && path == "/codex-window-keeper/settings":
		return s.getSettings(ctx)
	case method == http.MethodPut && path == "/codex-window-keeper/settings":
		return s.putSettings(ctx, body)
	case method == http.MethodPut && path == "/codex-window-keeper/connection":
		return s.putConnection(ctx, body)
	case method == http.MethodGet && path == "/codex-window-keeper/accounts":
		return s.accounts(ctx)
	case method == http.MethodPut && path == "/codex-window-keeper/accounts/override":
		return s.putOverride(ctx, "", body)
	case method == http.MethodPut && strings.HasPrefix(path, "/codex-window-keeper/accounts/") && !strings.Contains(strings.TrimPrefix(path, "/codex-window-keeper/accounts/"), "/"):
		return s.putOverride(ctx, strings.TrimPrefix(path, "/codex-window-keeper/accounts/"), body)
	case method == http.MethodPost && path == "/codex-window-keeper/accounts/probe":
		return s.act(ctx, "", body, false)
	case method == http.MethodPost && strings.HasPrefix(path, "/codex-window-keeper/accounts/") && strings.HasSuffix(path, "/probe"):
		return s.act(ctx, strings.TrimSuffix(strings.TrimPrefix(path, "/codex-window-keeper/accounts/"), "/probe"), body, false)
	case method == http.MethodPost && path == "/codex-window-keeper/accounts/activate":
		return s.act(ctx, "", body, true)
	case method == http.MethodPost && strings.HasPrefix(path, "/codex-window-keeper/accounts/") && strings.HasSuffix(path, "/activate"):
		return s.act(ctx, strings.TrimSuffix(strings.TrimPrefix(path, "/codex-window-keeper/accounts/"), "/activate"), body, true)
	case method == http.MethodPost && path == "/codex-window-keeper/accounts/resume":
		return s.resume(ctx, "", body)
	case method == http.MethodPost && strings.HasPrefix(path, "/codex-window-keeper/accounts/") && strings.HasSuffix(path, "/resume"):
		return s.resume(ctx, strings.TrimSuffix(strings.TrimPrefix(path, "/codex-window-keeper/accounts/"), "/resume"), body)
	case method == http.MethodGet && path == "/codex-window-keeper/attempts":
		return s.attempts(ctx)
	default:
		return jsonStatus(http.StatusNotFound, map[string]any{"error": "not found"})
	}
}

func (s Service) health(ctx context.Context) (int, []byte) {
	settings, _, err := s.Keeper.Store.LoadSettings(ctx)
	if err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	_, keySet, _ := s.Keeper.Store.LoadManagementKey(ctx)
	return jsonStatus(http.StatusOK, map[string]any{"ok": true, "enabled": settings.Enabled, "management_key_set": keySet, "database": s.Keeper.Store.Path()})
}

func (s Service) getSettings(ctx context.Context) (int, []byte) {
	settings, ok, err := s.Keeper.Store.LoadSettings(ctx)
	if err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	if !ok {
		settings = config.Default()
	}
	_, keySet, _ := s.Keeper.Store.LoadManagementKey(ctx)
	return jsonStatus(http.StatusOK, map[string]any{"settings": settings, "management_key_set": keySet})
}

func (s Service) putSettings(ctx context.Context, body []byte) (int, []byte) {
	var settings config.Settings
	if err := json.Unmarshal(body, &settings); err != nil {
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	normalized, err := config.Normalize(settings)
	if err != nil {
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	if err := s.Keeper.Store.SaveSettings(ctx, normalized); err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	s.Keeper.WakeUp()
	return jsonStatus(http.StatusOK, map[string]any{"settings": normalized})
}

func (s Service) putConnection(ctx context.Context, body []byte) (int, []byte) {
	var request struct {
		BaseURL       string `json:"base_url"`
		ManagementKey string `json:"management_key"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": err.Error()})
	}
	settings, ok, err := s.Keeper.Store.LoadSettings(ctx)
	if err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	if !ok {
		settings = config.Default()
	}
	if strings.TrimSpace(request.BaseURL) != "" {
		settings.BaseURL = strings.TrimRight(strings.TrimSpace(request.BaseURL), "/")
	}
	if err := s.Keeper.Store.SaveSettings(ctx, settings); err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	if strings.TrimSpace(request.ManagementKey) != "" {
		if err := s.Keeper.Store.SaveManagementKey(ctx, strings.TrimSpace(request.ManagementKey)); err != nil {
			return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
	}
	s.Keeper.WakeUp()
	return jsonStatus(http.StatusOK, map[string]any{"management_key_set": true})
}

func (s Service) accounts(ctx context.Context) (int, []byte) {
	accounts, err := s.Keeper.Store.ListAccounts(ctx)
	if err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	type view struct {
		store.Account
		Windows any `json:"windows"`
	}
	out := make([]view, 0, len(accounts))
	for _, account := range accounts {
		windows, err := s.Keeper.Store.LoadWindows(ctx, account.AuthID)
		if err != nil {
			return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
		}
		out = append(out, view{Account: account, Windows: windows})
	}
	return jsonStatus(http.StatusOK, map[string]any{"accounts": out})
}

func (s Service) putOverride(ctx context.Context, pathAuthID string, body []byte) (int, []byte) {
	var request struct {
		AuthID  string `json:"auth_id"`
		Enabled string `json:"enabled"`
		Model   string `json:"model"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &request)
	}
	if strings.TrimSpace(request.AuthID) == "" {
		request.AuthID = pathAuthID
	}
	if strings.TrimSpace(request.AuthID) == "" {
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
	}
	switch request.Enabled {
	case "", "inherit", "on", "off":
	default:
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": "invalid enabled override"})
	}
	if request.Enabled == "" {
		request.Enabled = "inherit"
	}
	raw, _ := json.Marshal(map[string]string{"enabled": request.Enabled, "model": strings.TrimSpace(request.Model)})
	if err := s.Keeper.Store.SetOverride(ctx, request.AuthID, string(raw)); err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	s.Keeper.WakeUp()
	return jsonStatus(http.StatusOK, map[string]any{"ok": true})
}

func (s Service) act(ctx context.Context, pathAuthID string, body []byte, activate bool) (int, []byte) {
	var request struct {
		AuthID string `json:"auth_id"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &request)
	}
	if strings.TrimSpace(request.AuthID) == "" {
		request.AuthID = pathAuthID
	}
	if strings.TrimSpace(request.AuthID) == "" {
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
	}
	var err error
	if activate {
		err = s.Keeper.Activate(ctx, request.AuthID)
	} else {
		err = s.Keeper.Probe(ctx, request.AuthID)
	}
	if err != nil {
		return jsonStatus(http.StatusBadGateway, map[string]any{"error": err.Error()})
	}
	return jsonStatus(http.StatusOK, map[string]any{"ok": true})
}

func (s Service) resume(ctx context.Context, pathAuthID string, body []byte) (int, []byte) {
	var request struct {
		AuthID string `json:"auth_id"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &request)
	}
	if strings.TrimSpace(request.AuthID) == "" {
		request.AuthID = pathAuthID
	}
	if strings.TrimSpace(request.AuthID) == "" {
		return jsonStatus(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
	}
	if err := s.Keeper.Store.SetPause(ctx, request.AuthID, ""); err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	if err := s.Keeper.Store.SetNotBefore(ctx, request.AuthID, time.Now().UTC()); err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	s.Keeper.WakeUp()
	return jsonStatus(http.StatusOK, map[string]any{"ok": true})
}

func (s Service) attempts(ctx context.Context) (int, []byte) {
	attempts, err := s.Keeper.Store.ListAttempts(ctx, 100)
	if err != nil {
		return jsonStatus(http.StatusInternalServerError, map[string]any{"error": err.Error()})
	}
	return jsonStatus(http.StatusOK, map[string]any{"attempts": attempts})
}

func jsonStatus(status int, payload any) (int, []byte) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return http.StatusInternalServerError, []byte(`{"error":"marshal failed"}`)
	}
	return status, raw
}
