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
	case method == http.MethodDelete && path == "/v0/management/model-prices":
		query, err := url.ParseQuery(request.Query)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid query"})
		}
		deleted, err := runtime.Store().DeletePrice(ctx, query.Get("model"))
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "deleted": deleted})
	case method == http.MethodGet && path == "/v0/management/model-prices/usage-summary":
		models, err := runtime.Store().PriceSyncTargets(ctx)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"models": models})
	case method == http.MethodPost && path == "/v0/management/model-prices/sync":
		result, err := runtime.SyncPrices(ctx)
		if err != nil {
			if err.Error() == "price sync is already running" {
				return jsonResponse(http.StatusConflict, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusBadGateway, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, result)
	case method == http.MethodGet && path == "/v0/management/model-prices/sync-settings":
		return jsonResponse(http.StatusOK, runtime.PriceSyncSettings())
	case method == http.MethodPut && path == "/v0/management/model-prices/sync-settings":
		var settings app.PriceSyncSettings
		if err := json.Unmarshal(request.Body, &settings); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid sync settings"})
		}
		if err := runtime.UpdatePriceSyncSettings(ctx, settings); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, runtime.PriceSyncSettings())
	case method == http.MethodGet && path == "/v0/management/model-prices/sync-status":
		return jsonResponse(http.StatusOK, runtime.PriceSyncStatus())
	case method == http.MethodPost && path == "/v0/management/model-prices/sync-confirm":
		var payload struct {
			Model string      `json:"model"`
			Price store.Price `json:"price"`
		}
		if err := json.Unmarshal(request.Body, &payload); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid sync candidate"})
		}
		if err := runtime.ConfirmPriceSyncCandidate(ctx, payload.Model, payload.Price); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "status": runtime.PriceSyncStatus()})
	case method == http.MethodPost && path == "/v0/management/model-prices/sync-dismiss":
		var payload struct {
			Models []string `json:"models"`
		}
		if len(request.Body) > 0 {
			if err := json.Unmarshal(request.Body, &payload); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid dismiss payload"})
			}
		}
		if err := runtime.DismissPriceSyncCandidates(ctx, payload.Models); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "status": runtime.PriceSyncStatus()})
	case method == http.MethodGet && path == "/usage-service/config":
		cfg := runtime.Config()
		baseURL, hasKey := runtime.Connection()
		inspection := runtime.CodexInspectionSettings()
		autoBan := runtime.AutoBanSettings()
		return jsonResponse(http.StatusOK, map[string]any{
			"source": "plugin",
			"config": map[string]any{
				"dataDir": cfg.DataDir,
				"cpaConnection": map[string]any{
					"cpaBaseUrl":       baseURL,
					"hasManagementKey": hasKey,
				},
				"collector": map[string]any{
					"enabled":       cfg.Collector.Enabled,
					"queueCapacity": cfg.QueueCapacity,
					"batchSize":     cfg.BatchSize,
				},
				"codexInspection": inspection,
				"autoBan":         autoBan,
			},
		})
	case method == http.MethodPut && path == "/usage-service/config":
		payload, err := parseUsageServiceConfigPut(request.Body)
		if err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid config"})
		}
		if payload.BaseURL != "" || payload.ManagementKey != "" {
			if err := runtime.UpdateConnection(ctx, payload.BaseURL, payload.ManagementKey); err != nil {
				return errorResponse(err)
			}
		}
		if payload.CollectorEnabled != nil {
			runtime.UpdateCollector(*payload.CollectorEnabled)
		}
		if payload.Codex != nil {
			if err := runtime.UpdateCodexInspectionSettings(ctx, *payload.Codex); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
		}
		return Handle(ctx, runtime, []byte(`{"method":"GET","path":"/usage-service/config"}`))
	case strings.HasPrefix(path, "/v0/management/auto-ban"):
		return handleAutoBanRoute(ctx, runtime, method, path, request.Query, request.Body)
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
		detail, err := runtime.StartInspection(ctx, "manual", "")
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
	if len(parts) == 2 && parts[1] == "cancel" && method == http.MethodPost {
		if !runtime.CancelInspection() {
			return jsonResponse(http.StatusConflict, map[string]any{"error": "inspection is not running"})
		}
		return jsonResponse(http.StatusOK, map[string]any{"ok": true, "runId": id})
	}
	if len(parts) == 2 && parts[1] == "actions" && method == http.MethodPost {
		var payload struct {
			ResultIDs []int64 `json:"resultIds"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid actions"})
		}
		result, err := runtime.ExecuteInspectionActions(ctx, id, payload.ResultIDs)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, result)
	}
	if len(parts) == 2 && parts[1] == "acknowledge" && method == http.MethodPost {
		var payload struct {
			ResultIDs []int64 `json:"resultIds"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid acknowledgement"})
		}
		result, err := runtime.AcknowledgeInspectionResults(ctx, id, payload.ResultIDs)
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, result)
	}
	return jsonResponse(http.StatusNotFound, map[string]any{"error": "inspection operation not found"})
}

func handleAutoBanRoute(ctx context.Context, runtime *app.Runtime, method, path, queryRaw string, body json.RawMessage) Response {
	const prefix = "/v0/management/auto-ban"
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := []string{}
	if rest != "" {
		parts = strings.Split(rest, "/")
	}
	query, err := url.ParseQuery(queryRaw)
	if err != nil {
		return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid query"})
	}
	if len(parts) == 1 && parts[0] == "settings" {
		switch method {
		case http.MethodGet:
			return jsonResponse(http.StatusOK, runtime.AutoBanSettings())
		case http.MethodPut:
			var settings app.AutoBanSettings
			if err := json.Unmarshal(rawBody(body), &settings); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid auto-ban settings"})
			}
			if err := runtime.UpdateAutoBanSettings(ctx, settings); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusOK, runtime.AutoBanSettings())
		default:
			return jsonResponse(http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		}
	}
	if len(parts) == 1 && parts[0] == "rules" {
		switch method {
		case http.MethodGet:
			rules, err := runtime.Store().ListAutoBanRules(ctx)
			if err != nil {
				return errorResponse(err)
			}
			return jsonResponse(http.StatusOK, map[string]any{"items": rules})
		case http.MethodPost:
			var rule store.AutoBanRule
			if err := json.Unmarshal(rawBody(body), &rule); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid auto-ban rule"})
			}
			created, err := runtime.Store().UpsertAutoBanRule(ctx, rule)
			if err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusCreated, created)
		default:
			return jsonResponse(http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		}
	}
	if len(parts) == 2 && parts[0] == "rules" {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id < 1 {
			return jsonResponse(http.StatusNotFound, map[string]any{"error": "auto-ban rule not found"})
		}
		switch method {
		case http.MethodGet:
			rule, err := runtime.Store().GetAutoBanRule(ctx, id)
			if err != nil {
				return jsonResponse(http.StatusNotFound, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusOK, rule)
		case http.MethodPatch:
			var rule store.AutoBanRule
			if err := json.Unmarshal(rawBody(body), &rule); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid auto-ban rule"})
			}
			rule.ID = id
			updated, err := runtime.Store().UpsertAutoBanRule(ctx, rule)
			if err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusOK, updated)
		case http.MethodDelete:
			if err := runtime.Store().DeleteAutoBanRule(ctx, id); err != nil {
				return jsonResponse(http.StatusNotFound, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusOK, map[string]any{"ok": true})
		default:
			return jsonResponse(http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		}
	}
	if len(parts) == 1 && parts[0] == "accounts" && method == http.MethodGet {
		items, err := runtime.Store().ListAutoBanAccounts(ctx, strings.TrimSpace(query.Get("state")), strings.TrimSpace(query.Get("provider")), strings.TrimSpace(query.Get("q")), int(intQuery(query, "limit", 200)))
		if err != nil {
			return errorResponse(err)
		}
		return jsonResponse(http.StatusOK, map[string]any{"items": items})
	}
	if len(parts) >= 2 && parts[0] == "accounts" {
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id < 1 {
			return jsonResponse(http.StatusNotFound, map[string]any{"error": "auto-ban account not found"})
		}
		if len(parts) == 2 && method == http.MethodGet {
			state, err := runtime.Store().GetAutoBanAccountByID(ctx, id)
			if err != nil {
				return jsonResponse(http.StatusNotFound, map[string]any{"error": err.Error()})
			}
			history, err := runtime.Store().ListAutoBanHistory(ctx, state.AccountKey, int(intQuery(query, "history_limit", 50)))
			if err != nil {
				return errorResponse(err)
			}
			return jsonResponse(http.StatusOK, map[string]any{"account": state, "history": history})
		}
		if len(parts) == 3 && parts[2] == "history" && method == http.MethodGet {
			state, err := runtime.Store().GetAutoBanAccountByID(ctx, id)
			if err != nil {
				return jsonResponse(http.StatusNotFound, map[string]any{"error": err.Error()})
			}
			history, err := runtime.Store().ListAutoBanHistory(ctx, state.AccountKey, int(intQuery(query, "limit", 100)))
			if err != nil {
				return errorResponse(err)
			}
			return jsonResponse(http.StatusOK, map[string]any{"items": history})
		}
		if len(parts) == 3 && parts[2] == "actions" && method == http.MethodPost {
			var payload struct {
				Action string `json:"action"`
				Reason string `json:"reason"`
			}
			if err := json.Unmarshal(rawBody(body), &payload); err != nil {
				return jsonResponse(http.StatusBadRequest, map[string]any{"error": "invalid auto-ban action"})
			}
			state, err := runtime.ExecuteAutoBanAccountAction(ctx, id, strings.TrimSpace(payload.Action), strings.TrimSpace(payload.Reason))
			if err != nil {
				return jsonResponse(http.StatusConflict, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusOK, state)
		}
		if len(parts) == 3 && parts[2] == "reset-counters" && method == http.MethodPost {
			state, err := runtime.ExecuteAutoBanAccountAction(ctx, id, "reset_counters", "")
			if err != nil {
				return jsonResponse(http.StatusConflict, map[string]any{"error": err.Error()})
			}
			return jsonResponse(http.StatusOK, state)
		}
	}
	return jsonResponse(http.StatusNotFound, map[string]any{"error": "auto-ban operation not found"})
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

type usageServiceConfigPut struct {
	BaseURL          string
	ManagementKey    string
	CollectorEnabled *bool
	Codex            *app.CodexInspectionSettings
}

func parseUsageServiceConfigPut(body json.RawMessage) (usageServiceConfigPut, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(rawBody(body), &root); err != nil {
		return usageServiceConfigPut{}, err
	}
	// Support both top-level and nested {"config": ...} envelopes.
	sections := []map[string]json.RawMessage{root}
	if nestedRaw, ok := root["config"]; ok {
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(nestedRaw, &nested); err != nil {
			return usageServiceConfigPut{}, err
		}
		sections = append([]map[string]json.RawMessage{nested}, sections...)
	}

	var out usageServiceConfigPut
	for _, section := range sections {
		if raw, ok := section["cpaConnection"]; ok && out.BaseURL == "" && out.ManagementKey == "" {
			baseURL, managementKey, err := parseCPAConnection(raw)
			if err != nil {
				return usageServiceConfigPut{}, err
			}
			out.BaseURL = baseURL
			out.ManagementKey = managementKey
		}
		if raw, ok := section["collector"]; ok && out.CollectorEnabled == nil {
			var collector struct {
				Enabled *bool `json:"enabled"`
			}
			if err := json.Unmarshal(raw, &collector); err != nil {
				return usageServiceConfigPut{}, err
			}
			out.CollectorEnabled = collector.Enabled
		}
		if raw, ok := section["codexInspection"]; ok && out.Codex == nil {
			settings, err := parseCodexInspectionSettings(raw)
			if err != nil {
				return usageServiceConfigPut{}, err
			}
			out.Codex = &settings
		}
	}
	return out, nil
}

func parseCPAConnection(raw json.RawMessage) (string, string, error) {
	var conn map[string]json.RawMessage
	if err := json.Unmarshal(raw, &conn); err != nil {
		return "", "", err
	}
	baseURL := ""
	if rawURL, ok := conn["cpaBaseUrl"]; ok {
		if err := json.Unmarshal(rawURL, &baseURL); err != nil {
			return "", "", err
		}
	}
	// managementKey is write-only: only accept JSON strings. Boolean/null redaction
	// placeholders from older GET responses are ignored so they never clear secrets.
	managementKey := ""
	if rawKey, ok := conn["managementKey"]; ok {
		var asString string
		if err := json.Unmarshal(rawKey, &asString); err == nil {
			managementKey = strings.TrimSpace(asString)
		}
	}
	return strings.TrimSpace(baseURL), managementKey, nil
}

func parseCodexInspectionSettings(raw json.RawMessage) (app.CodexInspectionSettings, error) {
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(raw, &flat); err != nil {
		return app.CodexInspectionSettings{}, err
	}

	settings := app.CodexInspectionSettings{}
	if _, hasSchedule := flat["schedule"]; !hasSchedule {
		// A legacy payload carried only the four runtime fields. Seed all newer
		// persisted/UI fields so it remains valid after upgrading the plugin.
		settings = app.DefaultCodexInspectionSettings()
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return app.CodexInspectionSettings{}, err
	}
	// Accept legacy flat payloads (scheduleMode/intervalMinutes at top level).
	if _, hasSchedule := flat["schedule"]; !hasSchedule {
		var legacy struct {
			ScheduleMode    string `json:"scheduleMode"`
			IntervalMinutes int    `json:"intervalMinutes"`
		}
		if err := json.Unmarshal(raw, &legacy); err != nil {
			return app.CodexInspectionSettings{}, err
		}
		if legacy.ScheduleMode != "" {
			settings.Schedule.Mode = legacy.ScheduleMode
		}
		if legacy.IntervalMinutes > 0 {
			settings.Schedule.IntervalMinutes = legacy.IntervalMinutes
		}
	}
	return settings, nil
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
