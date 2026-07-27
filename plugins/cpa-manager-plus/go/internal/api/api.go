package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/app"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

type Request struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Query  string          `json:"query"`
	Body   json.RawMessage `json:"body"`
}

type Response struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

func Handle(ctx context.Context, runtime *app.Runtime, raw []byte) Response {
	var request Request
	if err := json.Unmarshal(raw, &request); err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid JSON body"})
	}
	method := strings.ToUpper(strings.TrimSpace(request.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := strings.TrimRight(strings.TrimSpace(request.Path), "/")
	if path == "" {
		path = "/"
	}
	switch {
	case method == http.MethodGet && path == "/v0/management/dashboard/summary":
		query, err := url.ParseQuery(request.Query)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid query"})
		}
		result, err := runtime.Store().Dashboard(ctx, intQuery(query, "today_start_ms", 0), intQuery(query, "now_ms", time.Now().UnixMilli()))
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, result)
	case method == http.MethodPost && path == "/v0/management/monitoring/analytics":
		request, err := analyticsRequest(rawBody(request.Body))
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		result, err := runtime.Store().Analytics(ctx, request)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, result)
	case method == http.MethodGet && path == "/v0/management/model-prices":
		prices, err := runtime.Store().Prices(ctx)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"prices": prices})
	case method == http.MethodPut && path == "/v0/management/model-prices":
		var payload struct {
			Prices map[string]store.Price `json:"prices"`
		}
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid prices"})
		}
		if err := runtime.Store().ReplacePrices(ctx, payload.Prices); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})
	case method == http.MethodGet && path == "/usage-service/config":
		cfg := runtime.Config()
		baseURL, hasKey := runtime.Connection()
		return jsonResponse(http.StatusOK, map[string]any{"source": "plugin", "config": map[string]any{"dataDir": cfg.DataDir, "cpaConnection": map[string]any{"cpaBaseUrl": baseURL, "hasManagementKey": hasKey, "managementKey": hasKey}, "collector": map[string]any{"enabled": cfg.Collector.Enabled, "queueCapacity": cfg.QueueCapacity, "batchSize": cfg.BatchSize}, "codexInspection": map[string]any{"enabled": cfg.Codex.Enabled, "scheduleMode": cfg.Codex.ScheduleMode, "intervalMinutes": cfg.Codex.IntervalMinutes, "autoActionMode": cfg.Codex.AutoActionMode}}})
	case method == http.MethodPut && path == "/usage-service/config":
		var payload struct {
			Config struct {
				CPAConnection struct {
					BaseURL       string `json:"cpaBaseUrl"`
					ManagementKey string `json:"managementKey"`
				} `json:"cpaConnection"`
				Collector struct {
					Enabled *bool `json:"enabled"`
				} `json:"collector"`
			} `json:"config"`
			CPAConnection struct {
				BaseURL       string `json:"cpaBaseUrl"`
				ManagementKey string `json:"managementKey"`
			} `json:"cpaConnection"`
			Collector struct {
				Enabled *bool `json:"enabled"`
			} `json:"collector"`
		}
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid config"})
		}
		connection := payload.CPAConnection
		if connection.BaseURL == "" && connection.ManagementKey == "" {
			connection = payload.Config.CPAConnection
		}
		if err := runtime.UpdateConnection(ctx, strings.TrimSpace(connection.BaseURL), strings.TrimSpace(connection.ManagementKey)); err != nil {
			return errorResponse(err)
		}
		enabled := payload.Collector.Enabled
		if enabled == nil {
			enabled = payload.Config.Collector.Enabled
		}
		if enabled != nil {
			runtime.UpdateCollector(*enabled)
		}
		return Handle(ctx, runtime, []byte(`{"method":"GET","path":"/usage-service/config"}`))
	case method == http.MethodGet && path == "/v0/management/account-action-candidates":
		query, err := url.ParseQuery(request.Query)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid query"})
		}
		items, err := runtime.Store().Candidates(ctx, strings.TrimSpace(query.Get("status")), int(intQuery(query, "limit", 200)))
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"items": items})
	case strings.HasPrefix(path, "/v0/management/account-action-candidates/"):
		id, action, ok := candidateAction(path)
		if !ok {
			return jsonResponse(http.StatusNotFound, map[string]any{"error": "candidate operation not found"})
		}
		if (action == "delete" && method != http.MethodDelete) || (action != "delete" && method != http.MethodPost) {
			return jsonResponse(http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		}
		if err := runtime.ExecuteCandidate(ctx, id, action); err != nil {
			return jsonResponse(http.StatusConflict, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true})
	case method == http.MethodPost && path == "/v0/management/codex-inspection/run":
		detail, err := runtime.RunInspection(ctx)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, detail)
	case method == http.MethodGet && path == "/v0/management/codex-inspection/runs":
		query, err := url.ParseQuery(request.Query)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid query"})
		}
		items, err := runtime.Store().InspectionRuns(ctx, int(intQuery(query, "limit", 30)))
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"items": items})
	case strings.HasPrefix(path, "/v0/management/codex-inspection/runs/"):
		return handleInspectionRoute(ctx, runtime, method, path, request.Body)
	case strings.HasPrefix(path, "/v0/management/codex-inspection"):
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "inspection operation not found"})
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "local plugin operation not found", "path": path})
	}
}

func handleInspectionRoute(ctx context.Context, runtime *app.Runtime, method, path string, body []byte) Response {
	const prefix = "/v0/management/codex-inspection/runs/"
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id < 1 {
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "inspection run not found"})
	}
	if len(parts) == 1 && method == http.MethodGet {
		detail, err := runtime.Store().InspectionDetail(ctx, id)
		if err != nil {
			return jsonResponse(http.StatusNotFound, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, detail)
	}
	if len(parts) == 2 && parts[1] == "actions" && method == http.MethodPost {
		var payload struct {
			ResultIDs []int64 `json:"resultIds"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid actions"})
		}
		result, err := runtime.Store().ExecuteInspectionActions(ctx, id, payload.ResultIDs)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, result)
	}
	return jsonResponse(http.StatusNotFound, map[string]any{"error": "inspection operation not found"})
}

func candidateAction(path string) (int64, string, bool) {
	const prefix = "/v0/management/account-action-candidates/"
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	if len(parts) < 2 {
		return 0, "", false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id < 1 {
		return 0, "", false
	}
	action := parts[1]
	if action == "auth-file" && len(parts) == 2 {
		return id, "delete", true
	}
	if len(parts) == 2 && (action == "enable" || action == "ignore" || action == "resolve") {
		return id, action, true
	}
	return 0, "", false
}

func rawBody(raw json.RawMessage) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte("{}")
	}
	return raw
}
func intQuery(query url.Values, key string, fallback int64) int64 {
	value, err := strconv.ParseInt(query.Get(key), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}
func jsonResponse(status int, payload any) Response {
	raw, err := json.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		raw = []byte(`{"error":"marshal failed"}`)
	}
	return Response{StatusCode: status, Headers: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}, Body: raw}
}
func errorResponse(err error) Response {
	return jsonResponse(http.StatusInternalServerError, map[string]any{"error": fmt.Sprint(err)})
}

type analyticsPayload struct {
	FromMS  int64  `json:"from_ms"`
	ToMS    int64  `json:"to_ms"`
	Search  string `json:"search_query"`
	Filters struct {
		Models        []string `json:"models"`
		Providers     []string `json:"providers"`
		Accounts      []string `json:"accounts"`
		APIKeyHashes  []string `json:"api_key_hashes"`
		FailedOnly    bool     `json:"failed_only"`
		IncludeFailed *bool    `json:"include_failed"`
	} `json:"filters"`
	Include struct {
		EventsPage struct {
			Limit int `json:"limit"`
		} `json:"events_page"`
		Granularity string `json:"granularity"`
	} `json:"include"`
}

func analyticsRequest(raw []byte) (store.AnalyticsRequest, error) {
	var payload analyticsPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return store.AnalyticsRequest{}, fmt.Errorf("invalid analytics request")
	}
	includeFailed := true
	if payload.Filters.IncludeFailed != nil {
		includeFailed = *payload.Filters.IncludeFailed
	}
	return store.AnalyticsRequest{FromMS: payload.FromMS, ToMS: payload.ToMS, Limit: payload.Include.EventsPage.Limit, Models: payload.Filters.Models, Providers: payload.Filters.Providers, Accounts: payload.Filters.Accounts, APIKeyHashes: payload.Filters.APIKeyHashes, FailedOnly: payload.Filters.FailedOnly, IncludeFailed: includeFailed, Search: payload.Search, Granularity: payload.Include.Granularity}, nil
}
