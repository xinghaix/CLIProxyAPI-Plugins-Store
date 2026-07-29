package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

const (
	codexUsageURL        = "https://chatgpt.com/backend-api/wham/usage"
	xaiBillingWeeklyURL  = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	xaiBillingMonthlyURL = "https://cli-chat-proxy.grok.com/v1/billing"
	xaiInferenceURL      = "https://cli-chat-proxy.grok.com/v1/responses"
	maxInspectionBody    = 2048
)

type inspectionAPIResponse struct {
	StatusCode int
	Body       any
	BodyText   string
}

func (r *Runtime) runInspection(ctx context.Context, trigger, triggerKey string) (map[string]any, error) {
	runCtx, settings, run, release, err := r.beginInspection(ctx, trigger, triggerKey)
	if err != nil {
		return nil, err
	}
	defer release()
	return r.executeInspection(runCtx, settings, run)
}

// StartInspection records a running local inspection and executes it in the
// runtime background so the management UI can poll and cancel it.
func (r *Runtime) StartInspection(ctx context.Context, trigger, triggerKey string) (map[string]any, error) {
	runCtx, settings, run, release, err := r.beginInspection(ctx, trigger, triggerKey)
	if err != nil {
		return nil, err
	}
	go func() {
		defer release()
		_, _ = r.executeInspection(runCtx, settings, run)
	}()
	return r.store.InspectionDetail(ctx, run.ID)
}

func (r *Runtime) beginInspection(ctx context.Context, trigger, triggerKey string) (context.Context, CodexInspectionSettings, store.InspectionRun, func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.inspectionRunMu.Lock()
	if r.inspectionCancel != nil {
		r.inspectionRunMu.Unlock()
		return nil, CodexInspectionSettings{}, store.InspectionRun{}, nil, fmt.Errorf("inspection is already running")
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.inspectionCancel = cancel
	r.inspectionRunMu.Unlock()
	settings := r.CodexInspectionSettings()
	rawSettings, _ := json.Marshal(settings)
	run, err := r.store.StartInspectionRun(runCtx, trigger, triggerKey, string(rawSettings))
	if err != nil {
		cancel()
		r.inspectionRunMu.Lock()
		r.inspectionCancel = nil
		r.inspectionRunMu.Unlock()
		return nil, CodexInspectionSettings{}, store.InspectionRun{}, nil, err
	}
	release := func() {
		cancel()
		r.inspectionRunMu.Lock()
		r.inspectionCancel = nil
		r.inspectionRunMu.Unlock()
	}
	return runCtx, settings, run, release, nil
}

func (r *Runtime) executeInspection(runCtx context.Context, settings CodexInspectionSettings, run store.InspectionRun) (map[string]any, error) {
	log := func(level, message string, detail any) {
		_, _ = r.store.AppendInspectionLog(context.WithoutCancel(runCtx), store.InspectionLog{RunID: run.ID, Level: level, Message: message, Detail: detail})
	}
	log("info", "本地凭证健康巡检开始", map[string]any{"targetTypes": settings.TargetTypes})

	r.mu.Lock()
	list := r.authList
	connection := r.connection
	r.mu.Unlock()
	if list == nil {
		_, _ = r.store.FinishInspectionRun(context.WithoutCancel(runCtx), run, "failed", "host auth callback is unavailable")
		return r.store.InspectionDetail(context.Background(), run.ID)
	}
	if connection.BaseURL == "" || connection.ManagementKey == "" {
		_, _ = r.store.FinishInspectionRun(context.WithoutCancel(runCtx), run, "failed", "CPA 账号处置授权未配置；真实巡检需要管理 API")
		return r.store.InspectionDetail(context.Background(), run.ID)
	}
	auths, err := list()
	if err != nil {
		_, _ = r.store.FinishInspectionRun(context.WithoutCancel(runCtx), run, "failed", err.Error())
		return r.store.InspectionDetail(context.Background(), run.ID)
	}
	accounts := filterInspectionAccounts(auths, settings)
	run.TotalFiles, run.ProbeSetCount = int64(len(auths)), int64(len(accounts))
	accounts = sampleInspectionAccounts(accounts, settings.SampleSize)
	run.SampledCount = int64(len(accounts))
	for _, account := range accounts {
		if account.Disabled {
			run.DisabledCount++
		} else {
			run.EnabledCount++
		}
	}
	_ = r.store.UpdateInspectionProgress(context.WithoutCancel(runCtx), run)
	log("info", "巡检集合已准备", map[string]any{"totalFiles": run.TotalFiles, "probeSetCount": run.ProbeSetCount, "sampledCount": run.SampledCount})
	results := r.probeInspectionAccounts(runCtx, settings, accounts)
	for _, result := range results {
		result.RunID = run.ID
		if _, err := r.store.InsertInspectionResult(context.WithoutCancel(runCtx), result); err != nil {
			log("error", "保存巡检结果失败", map[string]any{"error": err.Error(), "account": result.DisplayAccount})
			continue
		}
		log(resultLogLevel(result), "账号探测完成", map[string]any{"provider": result.Provider, "fileName": result.FileName, "action": result.Action, "statusCode": result.StatusCode, "usedPercent": result.UsedPercent, "errorKind": result.ErrorKind})
	}
	if runCtx.Err() != nil {
		_, _ = r.store.FinishInspectionRun(context.WithoutCancel(runCtx), run, "cancelled", runCtx.Err().Error())
		return r.store.InspectionDetail(context.Background(), run.ID)
	}
	if settings.AutoActionMode != "none" || settings.AutoRecoverEnabled {
		if err := r.executeAutomaticInspectionActions(runCtx, run.ID, settings, log); err != nil {
			log("warning", "自动处置未完全完成", map[string]any{"error": err.Error()})
		}
	}
	if _, err := r.store.FinishInspectionRun(context.WithoutCancel(runCtx), run, "completed", ""); err != nil {
		return nil, err
	}
	return r.store.InspectionDetail(context.Background(), run.ID)
}

func filterInspectionAccounts(auths []pluginapi.HostAuthFileEntry, settings CodexInspectionSettings) []store.InspectionAccount {
	allowed := map[string]bool{}
	for _, provider := range settings.TargetTypes {
		allowed[provider] = true
	}
	accounts := make([]store.InspectionAccount, 0, len(auths))
	for _, auth := range auths {
		provider := strings.ToLower(strings.TrimSpace(auth.Provider))
		if !allowed[provider] {
			continue
		}
		key := firstNonEmpty(auth.AuthIndex, auth.ID, auth.Name)
		accounts = append(accounts, store.InspectionAccount{Key: key, FileName: auth.Name, DisplayName: firstNonEmpty(auth.Email, auth.Account, auth.Label, auth.Name), AuthIndex: auth.AuthIndex, AccountID: auth.Account, Provider: provider, Status: auth.Status, Disabled: auth.Disabled})
	}
	return accounts
}

func sampleInspectionAccounts(accounts []store.InspectionAccount, limit int) []store.InspectionAccount {
	if limit <= 0 {
		return accounts
	}
	byProvider := map[string][]store.InspectionAccount{}
	for _, account := range accounts {
		byProvider[account.Provider] = append(byProvider[account.Provider], account)
	}
	providers := make([]string, 0, len(byProvider))
	for provider := range byProvider {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	out := []store.InspectionAccount{}
	for _, provider := range providers {
		items := byProvider[provider]
		sort.SliceStable(items, func(i, j int) bool { return items[i].Key < items[j].Key })
		if len(items) > limit {
			items = items[:limit]
		}
		out = append(out, items...)
	}
	return out
}

func (r *Runtime) probeInspectionAccounts(ctx context.Context, settings CodexInspectionSettings, accounts []store.InspectionAccount) []store.InspectionResult {
	workers := settings.Workers
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan store.InspectionAccount)
	results := make(chan store.InspectionResult, len(accounts))
	var wait sync.WaitGroup
	for index := 0; index < workers && index < len(accounts); index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for account := range jobs {
				results <- r.probeInspectionAccount(ctx, settings, account)
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, account := range accounts {
			select {
			case <-ctx.Done():
				return
			case jobs <- account:
			}
		}
	}()
	go func() { wait.Wait(); close(results) }()
	out := make([]store.InspectionResult, 0, len(accounts))
	for result := range results {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FileName < out[j].FileName })
	return out
}

func (r *Runtime) probeInspectionAccount(ctx context.Context, settings CodexInspectionSettings, account store.InspectionAccount) store.InspectionResult {
	base := store.InspectionResult{AccountKey: account.Key, FileName: account.FileName, DisplayAccount: account.DisplayName, AuthIndex: account.AuthIndex, AccountID: account.AccountID, Provider: account.Provider, Disabled: account.Disabled, Status: account.Status, Action: "keep", ActionReason: "探测中", ActionStatus: "pending"}
	if account.AuthIndex == "" {
		base.Action, base.ActionReason, base.ErrorKind, base.ErrorDetail = "review", "缺少 CPA auth_index，无法安全代理探测", "missing_auth_index", "host auth entry has no auth_index"
		return base
	}
	var result store.InspectionResult
	switch account.Provider {
	case "codex":
		result = r.probeCodex(ctx, settings, base)
	case "xai":
		result = r.probeXAI(ctx, settings, base)
	default:
		base.Action, base.ActionReason, base.ErrorKind = "review", "不支持的巡检提供商", "unsupported_provider"
		return base
	}
	if result.Disabled && result.Action == "keep" && (result.ErrorKind == "healthy" || result.ErrorKind == "inference_healthy") {
		if _, owned, err := r.store.DisableOwnership(ctx, result.FileName); err == nil && owned {
			result.Action = "enable"
			result.ActionReason = "巡检此前自动禁用的凭证已恢复健康"
			result.AutoRecoverEligible = true
		}
	}
	return result
}

func (r *Runtime) probeCodex(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	response, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, codexUsageURL, map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "User-Agent": settings.UserAgent}, nil)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	return resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, "codex")
}

func (r *Runtime) probeXAI(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	metadata, _ := r.inspectionAuthMetadata(ctx, result.AuthIndex)
	baseURL, officialAPI, userID := resolveXAIProbeMetadata(metadata)
	if officialAPI {
		response, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/me", map[string]string{"Authorization": "Bearer $TOKEN$", "Accept": "application/json"}, nil)
		if err != nil {
			return inspectionFailure(result, 0, "upstream_error", err.Error())
		}
		result = resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, "xai")
		if result.ErrorKind == "healthy" {
			result.ActionReason = "xAI 官方 API 身份探测正常"
		}
		if settings.XAIInferenceEnabled && result.Action == "keep" {
			return r.probeXAIInference(ctx, settings, result, strings.TrimSuffix(baseURL, "/")+"/responses", map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "User-Agent": settings.XAIInferenceUserAgent})
		}
		return result
	}
	headers := map[string]string{"Authorization": "Bearer $TOKEN$", "x-xai-token-auth": "xai-grok-cli", "x-grok-client-version": "0.2.101", "User-Agent": "grok-pager/0.2.101 grok-shell/0.2.101"}
	if userID != "" {
		headers["x-userid"] = userID
	}
	weekly, weeklyErr := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, xaiBillingWeeklyURL, headers, nil)
	monthly, monthlyErr := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, xaiBillingMonthlyURL, headers, nil)
	response := weekly
	if weeklyErr != nil || weekly.StatusCode < 200 || weekly.StatusCode >= 300 {
		response = monthly
	}
	if monthlyErr != nil && weeklyErr != nil {
		return inspectionFailure(result, 0, "upstream_error", weeklyErr.Error())
	}
	result = resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, "xai")
	if settings.XAIInferenceEnabled && result.Action == "keep" {
		inferenceHeaders := map[string]string{"Authorization": "Bearer $TOKEN$", "x-xai-token-auth": "xai-grok-cli", "x-grok-client-version": "0.2.101", "User-Agent": settings.XAIInferenceUserAgent, "Content-Type": "application/json"}
		if userID != "" {
			inferenceHeaders["x-userid"] = userID
		}
		return r.probeXAIInference(ctx, settings, result, xaiInferenceURL, inferenceHeaders)
	}
	return result
}

func (r *Runtime) probeXAIInference(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult, target string, headers map[string]string) store.InspectionResult {
	payload := map[string]any{"model": settings.XAIInferenceModel, "input": settings.XAIInferencePrompt, "stream": false}
	inference, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodPost, target, headers, payload)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	if inference.StatusCode < 200 || inference.StatusCode >= 300 {
		return resolveInspectionHTTPResult(result, inference, settings.UsedPercentThreshold, "xai")
	}
	result.ActionReason, result.ErrorKind = "xAI billing 与 inference 探测正常", "inference_healthy"
	return result
}

func (r *Runtime) inspectionAuthMetadata(ctx context.Context, authIndex string) (map[string]any, error) {
	response, err := r.callCPA(ctx, http.MethodGet, "/v0/management/auth-files", nil)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("CPA auth-files returned HTTP %d", response.StatusCode)
	}
	var value any
	if err := json.Unmarshal(response.Body, &value); err != nil {
		return nil, err
	}
	return findInspectionAuthMetadata(value, authIndex), nil
}

func findInspectionAuthMetadata(value any, authIndex string) map[string]any {
	switch value := value.(type) {
	case []any:
		for _, item := range value {
			if found := findInspectionAuthMetadata(item, authIndex); found != nil {
				return found
			}
		}
	case map[string]any:
		for _, key := range []string{"auth_index", "authIndex", "index"} {
			if strings.TrimSpace(fmt.Sprint(value[key])) == authIndex {
				return value
			}
		}
		for _, key := range []string{"items", "auths", "data", "files"} {
			if found := findInspectionAuthMetadata(value[key], authIndex); found != nil {
				return found
			}
		}
	}
	return nil
}

func resolveXAIProbeMetadata(metadata map[string]any) (baseURL string, officialAPI bool, userID string) {
	baseURL = "https://api.x.ai/v1"
	if metadata == nil {
		return baseURL, false, ""
	}
	read := func(keys ...string) string {
		for _, scope := range []map[string]any{metadata, mapValue(metadata["metadata"]), mapValue(metadata["attributes"]), mapValue(metadata["user"])} {
			for _, key := range keys {
				if value := strings.TrimSpace(fmt.Sprint(scope[key])); value != "" && value != "<nil>" {
					return value
				}
			}
		}
		return ""
	}
	candidate := strings.TrimSuffix(read("base_url", "baseUrl"), "/")
	kind := strings.ToLower(read("auth_kind", "authKind", "type"))
	usingAPI := strings.EqualFold(read("using_api", "usingApi"), "true") || kind == "api_key" || kind == "api"
	if candidate != "" && !strings.Contains(strings.ToLower(candidate), "cli-chat-proxy.grok.com") {
		baseURL, officialAPI = candidate, true
	}
	if usingAPI {
		officialAPI = true
	}
	userID = read("user_id", "userId", "sub", "subject", "id")
	return baseURL, officialAPI, userID
}

func mapValue(value any) map[string]any { result, _ := value.(map[string]any); return result }

func resolveInspectionHTTPResult(result store.InspectionResult, response inspectionAPIResponse, threshold float64, provider string) store.InspectionResult {
	result.StatusCode = intPtr(response.StatusCode)
	used := findHighestPercent(response.Body)
	if used != nil {
		result.UsedPercent = used
		result.QuotaWindows = []map[string]any{{"id": provider + "-usage", "usedPercent": *used}}
	}
	status := response.StatusCode
	body := strings.ToLower(response.BodyText)
	switch {
	case status >= 200 && status < 300:
		if used != nil && *used >= threshold && threshold < 100 {
			result.Action, result.ActionReason, result.IsQuota, result.ErrorKind = "disable", "额度达到配置阈值", true, "quota_threshold"
		} else if result.Disabled {
			result.Action, result.ActionReason, result.ErrorKind = "keep", "凭证已禁用，等待自动恢复归属校验", "disabled"
		} else {
			result.Action, result.ActionReason, result.ErrorKind = "keep", "provider 探测正常", "healthy"
		}
	case status == http.StatusUnauthorized || strings.Contains(body, "invalid_grant") || strings.Contains(body, "invalid token"):
		result.Action, result.ActionReason, result.ErrorKind = "reauth", "认证凭证已失效，需要重新登录", "auth_invalid"
	case strings.Contains(body, "free-usage-exhausted") || strings.Contains(body, "spending-limit") || strings.Contains(body, "used all available credits"):
		result.Action, result.ActionReason, result.IsQuota, result.ErrorKind = "disable", "provider 报告额度已耗尽", true, "quota_exhausted"
	case status == http.StatusTooManyRequests:
		result.Action, result.ActionReason, result.ErrorKind = "keep", "provider 限流，保留凭证等待重试", "rate_limited"
	default:
		result.Action, result.ActionReason, result.ErrorKind = "review", "provider 响应无法安全自动处置", "needs_review"
	}
	result.ErrorDetail = truncateInspection(response.BodyText)
	if result.ErrorKind == "healthy" || result.ErrorKind == "inference_healthy" || result.ErrorKind == "disabled" {
		result.ErrorDetail = ""
	}
	return result
}

func inspectionFailure(result store.InspectionResult, status int, kind, detail string) store.InspectionResult {
	result.StatusCode, result.Action, result.ActionReason, result.ErrorKind, result.ErrorDetail = intPtr(status), "review", "探测请求失败，需人工复核", kind, truncateInspection(detail)
	return result
}

func (r *Runtime) callInspectionAPI(ctx context.Context, settings CodexInspectionSettings, authIndex, method, target string, headers map[string]string, requestBody any) (inspectionAPIResponse, error) {
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.Timeout)*time.Millisecond)
	defer cancel()
	payload := map[string]any{"authIndex": authIndex, "method": method, "url": target, "header": headers}
	if requestBody != nil {
		payload["data"] = requestBody
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return inspectionAPIResponse{}, err
	}
	response, err := r.callCPA(requestCtx, http.MethodPost, "/v0/management/api-call", body)
	if err != nil {
		return inspectionAPIResponse{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return inspectionAPIResponse{}, fmt.Errorf("CPA api-call returned HTTP %d", response.StatusCode)
	}
	var envelope map[string]any
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		return inspectionAPIResponse{}, fmt.Errorf("decode CPA api-call response: %w", err)
	}
	status, _ := numberValue(envelope["status_code"])
	if status == 0 {
		status, _ = numberValue(envelope["statusCode"])
	}
	bodyValue := envelope["body"]
	bodyText := ""
	switch value := bodyValue.(type) {
	case string:
		bodyText = value
	default:
		raw, _ := json.Marshal(value)
		bodyText = string(raw)
	}
	return inspectionAPIResponse{StatusCode: int(status), Body: bodyValue, BodyText: truncateInspection(bodyText)}, nil
}

func (r *Runtime) callCPA(ctx context.Context, method, route string, body []byte) (pricesync.HTTPResponse, error) {
	r.mu.Lock()
	connection, do := r.connection, r.httpDo
	r.mu.Unlock()
	if do == nil || connection.BaseURL == "" || connection.ManagementKey == "" {
		return pricesync.HTTPResponse{}, fmt.Errorf("CPA 账号处置授权未配置")
	}
	base, err := url.Parse(connection.BaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return pricesync.HTTPResponse{}, fmt.Errorf("invalid CPA Base URL")
	}
	reference, err := url.Parse(route)
	if err != nil {
		return pricesync.HTTPResponse{}, fmt.Errorf("invalid CPA route: %w", err)
	}
	target := base.ResolveReference(reference).String()
	headers := http.Header{"Authorization": []string{"Bearer " + connection.ManagementKey}, "Accept": []string{"application/json"}}
	if len(body) > 0 {
		headers.Set("Content-Type", "application/json")
	}
	return do(ctx, method, target, headers, body)
}

func (r *Runtime) executeAutomaticInspectionActions(ctx context.Context, runID int64, settings CodexInspectionSettings, log func(string, string, any)) error {
	results, err := r.store.InspectionResults(ctx, runID)
	if err != nil {
		return err
	}
	ids := []int64{}
	for _, result := range results {
		if result.Action == settings.AutoActionMode || (settings.AutoRecoverEnabled && result.Action == "enable" && result.AutoRecoverEligible) {
			ids = append(ids, result.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	_, err = r.executeInspectionActions(ctx, runID, ids, true)
	if err == nil {
		log("success", "自动处置完成", map[string]any{"count": len(ids)})
	}
	return err
}

func (r *Runtime) ExecuteInspectionActions(ctx context.Context, runID int64, ids []int64) (map[string]any, error) {
	return r.executeInspectionActions(ctx, runID, ids, false)
}

func (r *Runtime) AcknowledgeInspectionResults(ctx context.Context, runID int64, ids []int64) (map[string]any, error) {
	return r.store.AcknowledgeInspectionResults(ctx, runID, ids)
}

func (r *Runtime) executeInspectionActions(ctx context.Context, runID int64, ids []int64, automatic bool) (map[string]any, error) {
	results, err := r.store.InspectionResults(ctx, runID)
	if err != nil {
		return nil, err
	}
	wanted := map[int64]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	outcomes := []map[string]any{}
	for _, result := range results {
		if !wanted[result.ID] {
			continue
		}
		action := result.Action
		if action != "enable" && action != "disable" && action != "delete" {
			outcomes = append(outcomes, map[string]any{"id": result.ID, "success": false, "error": "action requires manual review"})
			continue
		}
		err := r.executeInspectionAction(ctx, result, automatic)
		if err != nil {
			_ = r.store.UpdateInspectionAction(context.WithoutCancel(ctx), result.ID, action, "failed", err.Error())
			outcomes = append(outcomes, map[string]any{"id": result.ID, "success": false, "error": err.Error()})
			continue
		}
		_ = r.store.UpdateInspectionAction(context.WithoutCancel(ctx), result.ID, action, "success", "")
		outcomes = append(outcomes, map[string]any{"id": result.ID, "success": true})
	}
	detail, err := r.store.InspectionDetail(ctx, runID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"detail": detail, "outcomes": outcomes}, nil
}

func (r *Runtime) executeInspectionAction(ctx context.Context, result store.InspectionResult, automatic bool) error {
	var method, route string
	var body []byte
	switch result.Action {
	case "delete":
		method, route = http.MethodDelete, "/v0/management/auth-files?name="+url.QueryEscape(result.FileName)
	case "disable", "enable":
		method, route = http.MethodPatch, "/v0/management/auth-files/status"
		body, _ = json.Marshal(map[string]any{"name": result.FileName, "auth_index": result.AuthIndex, "disabled": result.Action == "disable"})
	}
	response, err := r.callCPA(ctx, method, route, body)
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("CPA action returned HTTP %d", response.StatusCode)
	}
	if result.Action == "disable" && automatic {
		return r.store.PutDisableOwnership(ctx, store.InspectionDisableOwnership{FileName: result.FileName, Provider: result.Provider, AuthIndex: result.AuthIndex, AccountID: result.AccountID})
	}
	return r.store.DeleteDisableOwnership(ctx, result.FileName)
}

func resultLogLevel(result store.InspectionResult) string {
	switch result.Action {
	case "delete", "reauth":
		return "error"
	case "disable", "review":
		return "warning"
	case "enable":
		return "success"
	default:
		return "info"
	}
}
func intPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}
func truncateInspection(value string) string {
	if len(value) > maxInspectionBody {
		return value[:maxInspectionBody]
	}
	return value
}
func numberValue(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case json.Number:
		v, err := value.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}
func findHighestPercent(value any) *float64 {
	var best *float64
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if strings.Contains(strings.ToLower(key), "used_percent") || strings.Contains(strings.ToLower(key), "usage_percent") {
					if number, ok := numberValue(child); ok {
						if number >= 0 && number <= 100 && (best == nil || number > *best) {
							copy := number
							best = &copy
						}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(value)
	return best
}
