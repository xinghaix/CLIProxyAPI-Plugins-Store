package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

func handleWindowKeeperRoute(ctx context.Context, runtime *app.Runtime, method, path string, body []byte) Response {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	switch {
	case method == http.MethodGet && path == "/v0/management/window-keeper/settings":
		settings := runtime.WindowKeeperSettings()
		health := runtime.Health(ctx)
		boundKey, _ := health["bound_cpa_management_key"].(bool)
		return jsonResponse(http.StatusOK, map[string]any{
			"settings":           settings,
			"management_key_set": boundKey,
		})

	case method == http.MethodPut && path == "/v0/management/window-keeper/settings":
		var settings windowkeeper.Settings
		if err := json.Unmarshal(body, &settings); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		if err := runtime.SaveWindowKeeperSettings(ctx, settings); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"settings": runtime.WindowKeeperSettings()})

	case method == http.MethodGet && path == "/v0/management/window-keeper/accounts":
		accounts, err := runtime.WindowKeeperAccounts(ctx)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"accounts": accounts})

	case method == http.MethodPut && (path == "/v0/management/window-keeper/accounts/override" || strings.HasPrefix(path, "/v0/management/window-keeper/accounts/")):
		var request struct {
			AuthID  string `json:"auth_id"`
			Enabled string `json:"enabled"`
			Model   string `json:"model"`
		}
		if len(body) > 0 {
			_ = json.Unmarshal(body, &request)
		}
		if request.AuthID == "" && strings.HasPrefix(path, "/v0/management/window-keeper/accounts/") && path != "/v0/management/window-keeper/accounts/override" {
			request.AuthID = strings.TrimPrefix(path, "/v0/management/window-keeper/accounts/")
		}
		if strings.TrimSpace(request.AuthID) == "" {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
		}
		switch request.Enabled {
		case "", "inherit", "on", "off":
		default:
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid enabled override"})
		}
		if request.Enabled == "" {
			request.Enabled = "inherit"
		}
		raw, _ := json.Marshal(map[string]string{"enabled": request.Enabled, "model": strings.TrimSpace(request.Model)})
		if err := runtime.SetWindowKeeperOverride(ctx, request.AuthID, string(raw)); err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})

	case method == http.MethodPost && (path == "/v0/management/window-keeper/accounts/probe" || (strings.HasPrefix(path, "/v0/management/window-keeper/accounts/") && strings.HasSuffix(path, "/probe"))):
		authID := extractAuthID(path, "/v0/management/window-keeper/accounts/", "/probe", body)
		if authID == "" {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
		}
		if err := runtime.ProbeWindowKeeperAccount(ctx, authID); err != nil {
			return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})

	case method == http.MethodPost && (path == "/v0/management/window-keeper/accounts/activate" || (strings.HasPrefix(path, "/v0/management/window-keeper/accounts/") && strings.HasSuffix(path, "/activate"))):
		authID := extractAuthID(path, "/v0/management/window-keeper/accounts/", "/activate", body)
		if authID == "" {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
		}
		if err := runtime.ActivateWindowKeeperAccount(ctx, authID); err != nil {
			return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})

	case method == http.MethodPost && (path == "/v0/management/window-keeper/accounts/resume" || (strings.HasPrefix(path, "/v0/management/window-keeper/accounts/") && strings.HasSuffix(path, "/resume"))):
		authID := extractAuthID(path, "/v0/management/window-keeper/accounts/", "/resume", body)
		if authID == "" {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "auth_id is required"})
		}
		if err := runtime.ResumeWindowKeeperAccount(ctx, authID); err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})

	case method == http.MethodGet && path == "/v0/management/window-keeper/attempts":
		attempts, err := runtime.WindowKeeperAttempts(ctx, 100)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"attempts": attempts})

	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "window keeper endpoint not found", "path": path})
	}
}

func extractAuthID(path, prefix, suffix string, body []byte) string {
	var request struct {
		AuthID string `json:"auth_id"`
	}
	if len(body) > 0 {
		_ = json.Unmarshal(body, &request)
	}
	if strings.TrimSpace(request.AuthID) != "" {
		return strings.TrimSpace(request.AuthID)
	}
	if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, suffix) {
		trimmed := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
		return strings.Trim(trimmed, "/")
	}
	return ""
}
