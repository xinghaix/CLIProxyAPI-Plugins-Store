package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

const autoBanActionTimeout = 15 * time.Second

func (r *Runtime) handleCommittedUsageEvents(events []store.Event) {
	if r == nil || r.closed.Load() || len(events) == 0 {
		return
	}
	go func() {
		for _, event := range events {
			r.applyAutoBanSignal(context.Background(), autoBanSignalFromUsageEvent(event))
		}
	}()
}

func autoBanSignalFromUsageEvent(event store.Event) store.BanSignal {
	provider := strings.ToLower(strings.TrimSpace(event.Provider))
	if provider == "" {
		provider = "custom"
	}
	kind := "oauth_auth_file"
	if provider != "codex" && provider != "xai" {
		kind = "custom_provider"
	}
	headers := parseAutoBanHeaders(event.ResponseHeadersJSON)
	return store.BanSignal{
		AccountKey:  store.AutoBanAccountKey(provider, kind, event.AuthIndex, "", event.AuthID, event.APIKeyHash),
		Provider:    provider,
		AccountKind: kind,
		AuthIndex:   event.AuthIndex,
		AuthID:      event.AuthID,
		APIKeyHash:  event.APIKeyHash,
		StatusCode:  event.FailStatusCode,
		ErrorKind:   autoBanErrorKind(event.FailStatusCode, event.FailSummary),
		FailSummary: event.FailSummary,
		Headers:     headers,
		Source:      "usage",
		AtMS:        event.TimestampMS,
		Success:     !event.Failed,
	}
}

func parseAutoBanHeaders(raw string) map[string]string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var values map[string][]string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return firstAutoBanHeaderValues(values)
	}
	var stringsOnly map[string]string
	if err := json.Unmarshal([]byte(raw), &stringsOnly); err != nil {
		return nil
	}
	out := map[string]string{}
	for key, value := range stringsOnly {
		if isAutoBanHeader(key) && strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func firstAutoBanHeaderValues(values map[string][]string) map[string]string {
	out := map[string]string{}
	for key, value := range values {
		if !isAutoBanHeader(key) || len(value) == 0 || strings.TrimSpace(value[0]) == "" {
			continue
		}
		out[key] = strings.TrimSpace(value[0])
	}
	return out
}

func isAutoBanHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "x-ratelimit-reset", "x-ratelimit-reset-after", "retry-after", "x-request-id", "x-trace-id":
		return true
	default:
		return false
	}
}

func autoBanErrorKind(status int, summary string) string {
	body := strings.ToLower(summary)
	switch {
	case strings.Contains(body, "free-usage-exhausted") || strings.Contains(body, "spending-limit") || strings.Contains(body, "used all available credits"):
		return "quota_exhausted"
	case status == http.StatusTooManyRequests:
		return "rate_limited"
	case status == http.StatusUnauthorized || strings.Contains(body, "invalid_grant") || strings.Contains(body, "invalid token"):
		return "auth_invalid"
	case status >= 500:
		return "server_error"
	default:
		return ""
	}
}

func (r *Runtime) applyAutoBanSignal(ctx context.Context, signal store.BanSignal) {
	settings := r.AutoBanSettings()
	if !settings.Enabled || (signal.Source == "usage" && !settings.Sources.Usage) || (signal.Source == "inspection" && !settings.Sources.Inspection) {
		return
	}
	if signal.AtMS == 0 {
		signal.AtMS = time.Now().UnixMilli()
	}
	if !signal.Success {
		signal.Capabilities, signal.FileName = r.autoBanCapabilities(signal)
	}
	r.ensureAutoBanCooldown(ctx, signal.AccountKey)
	result, err := r.store.ApplyAutoBanSignal(ctx, signal, settings.DryRun)
	if err != nil || !result.ShouldExecute {
		return
	}
	cooldown := result.CooldownUntilMS
	if result.ExecuteAction == store.AutoBanActionCooldownEnable && result.MatchedRule != nil {
		cooldown = r.autoBanCooldownUntil(result.MatchedRule, signal, settings, cooldown)
	}
	_ = r.executeAutoBanStateAction(ctx, result.State, result.ExecuteAction, cooldown, "system", signal.Source)
}

func (r *Runtime) autoBanCooldownUntil(rule *store.AutoBanRule, signal store.BanSignal, settings AutoBanSettings, fallback *int64) *int64 {
	if rule == nil || rule.CooldownSource != "header_or_default" || rule.CooldownMS != nil || hasAutoBanResetHeader(signal.Headers) {
		return fallback
	}
	until := signal.AtMS + int64(settings.DefaultCodexCooldownHours)*int64(time.Hour/time.Millisecond)
	return &until
}

func hasAutoBanResetHeader(headers map[string]string) bool {
	for key := range headers {
		if strings.EqualFold(key, "x-ratelimit-reset") || strings.EqualFold(key, "x-ratelimit-reset-after") || strings.EqualFold(key, "retry-after") {
			return true
		}
	}
	return false
}

func (r *Runtime) autoBanCapabilities(signal store.BanSignal) (int, string) {
	if signal.AuthIndex == "" {
		return 0, ""
	}
	r.mu.Lock()
	list := r.authList
	r.mu.Unlock()
	if list == nil {
		return 0, ""
	}
	auths, err := list()
	if err != nil {
		return 0, ""
	}
	for _, auth := range auths {
		if auth.AuthIndex == signal.AuthIndex && strings.EqualFold(strings.TrimSpace(auth.Provider), signal.Provider) {
			return store.AutoBanCapDisable | store.AutoBanCapEnable | store.AutoBanCapDelete, auth.Name
		}
	}
	return 0, ""
}

func (r *Runtime) ensureAutoBanCooldown(ctx context.Context, accountKey string) {
	if strings.TrimSpace(accountKey) == "" {
		return
	}
	state, err := r.store.GetAutoBanAccount(ctx, accountKey)
	if err != nil || state.State != store.AutoBanStateCooling || state.CooldownUntilMS == nil || *state.CooldownUntilMS > time.Now().UnixMilli() || state.ManualHold {
		return
	}
	_ = r.executeAutoBanStateAction(ctx, state, "cooldown_expire", nil, "system", "lazy")
}

func (r *Runtime) scheduleAutoBan(ctx context.Context) {
	for {
		settings := r.AutoBanSettings()
		delay := time.Duration(settings.SchedulerIntervalSeconds) * time.Second
		if delay <= 0 {
			delay = 30 * time.Second
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-r.autoBanWake:
			timer.Stop()
			continue
		case <-timer.C:
		}
		if !settings.Enabled {
			continue
		}
		states, err := r.store.ListDueAutoBanCooldowns(ctx, time.Now().UnixMilli(), 100)
		if err != nil {
			continue
		}
		for _, state := range states {
			_ = r.executeAutoBanStateAction(ctx, state, "cooldown_expire", nil, "system", "scheduler")
		}
	}
}

func (r *Runtime) executeAutoBanStateAction(ctx context.Context, state store.AutoBanAccountState, action string, cooldownUntilMS *int64, actor, source string) error {
	r.autoBanActionMu.Lock()
	defer r.autoBanActionMu.Unlock()
	if state.FileName == "" || state.AuthIndex == "" {
		_, _ = r.store.TransitionAutoBanAction(context.WithoutCancel(ctx), state.AccountKey, action, false, "account has no manageable auth-file", cooldownUntilMS, actor, source)
		return fmt.Errorf("account has no manageable auth-file")
	}
	method := http.MethodPatch
	route := "/v0/management/auth-files/status"
	var body []byte
	switch action {
	case store.AutoBanActionDisable, store.AutoBanActionCooldownEnable, "manual_disable":
		body, _ = json.Marshal(map[string]any{"name": state.FileName, "auth_index": state.AuthIndex, "disabled": true})
	case "enable", "manual_enable", "manual_unban", "cooldown_expire":
		body, _ = json.Marshal(map[string]any{"name": state.FileName, "auth_index": state.AuthIndex, "disabled": false})
	case store.AutoBanActionDelete, "manual_delete":
		method = http.MethodDelete
		route = "/v0/management/auth-files?name=" + url.QueryEscape(state.FileName)
	default:
		return fmt.Errorf("unsupported auto-ban action %q", action)
	}
	requestCtx, cancel := context.WithTimeout(ctx, autoBanActionTimeout)
	defer cancel()
	response, err := r.callCPA(requestCtx, method, route, body)
	if err == nil && (response.StatusCode < 200 || response.StatusCode >= 300) {
		err = fmt.Errorf("CPA auto-ban action returned HTTP %d", response.StatusCode)
	}
	if err != nil {
		_, _ = r.store.TransitionAutoBanAction(context.WithoutCancel(ctx), state.AccountKey, action, false, err.Error(), cooldownUntilMS, actor, source)
		return err
	}
	_, err = r.store.TransitionAutoBanAction(context.WithoutCancel(ctx), state.AccountKey, action, true, "", cooldownUntilMS, actor, source)
	return err
}

func (r *Runtime) applyAutoBanInspectionResult(ctx context.Context, result store.InspectionResult) {
	statusCode := 0
	if result.StatusCode != nil {
		statusCode = *result.StatusCode
	}
	kind := "oauth_auth_file"
	signal := store.BanSignal{
		AccountKey:  store.AutoBanAccountKey(result.Provider, kind, result.AuthIndex, result.FileName, result.AccountID, ""),
		Provider:    strings.ToLower(strings.TrimSpace(result.Provider)),
		AccountKind: kind,
		FileName:    result.FileName,
		AuthIndex:   result.AuthIndex,
		AuthID:      result.AccountID,
		DisplayName: result.DisplayAccount,
		StatusCode:  statusCode,
		ErrorKind:   result.ErrorKind,
		FailSummary: result.ErrorDetail,
		Source:      "inspection",
		Success:     result.ErrorKind == "healthy" || result.ErrorKind == "inference_healthy",
	}
	r.applyAutoBanSignal(ctx, signal)
}

func (r *Runtime) autoBanBlocksInspectionRecover(result store.InspectionResult) bool {
	key := store.AutoBanAccountKey(result.Provider, "oauth_auth_file", result.AuthIndex, result.FileName, result.AccountID, "")
	state, err := r.store.GetAutoBanAccount(context.Background(), key)
	if err != nil {
		return false
	}
	return state.ManualHold || state.State == store.AutoBanStateCooling || state.State == store.AutoBanStateDisabled || state.State == store.AutoBanStateHeld || state.State == store.AutoBanStatePendingAction
}

// ExecuteAutoBanAccountAction performs an operator-initiated state change.
func (r *Runtime) ExecuteAutoBanAccountAction(ctx context.Context, id int64, action, reason string) (store.AutoBanAccountState, error) {
	state, err := r.store.GetAutoBanAccountByID(ctx, id)
	if err != nil {
		return store.AutoBanAccountState{}, err
	}
	switch action {
	case "hold":
		return r.store.SetAutoBanManualHold(ctx, state.AccountKey, true, reason)
	case "release":
		return r.store.SetAutoBanManualHold(ctx, state.AccountKey, false, "")
	case "reset_counters":
		return r.store.ResetAutoBanCounters(ctx, state.AccountKey)
	case "enable":
		err = r.executeAutoBanStateAction(ctx, state, "manual_enable", nil, "user", "manual")
	case "unban":
		err = r.executeAutoBanStateAction(ctx, state, "manual_unban", nil, "user", "manual")
	case "disable":
		err = r.executeAutoBanStateAction(ctx, state, "manual_disable", nil, "user", "manual")
	case "delete":
		err = r.executeAutoBanStateAction(ctx, state, "manual_delete", nil, "user", "manual")
	default:
		return store.AutoBanAccountState{}, fmt.Errorf("unsupported auto-ban account action %q", action)
	}
	if err != nil {
		return store.AutoBanAccountState{}, err
	}
	return r.store.GetAutoBanAccountByID(ctx, id)
}
