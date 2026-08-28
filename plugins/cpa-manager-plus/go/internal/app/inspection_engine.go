package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

const (
	codexUsageURL                 = "https://chatgpt.com/backend-api/wham/usage"
	codexResetCreditsURL          = "https://chatgpt.com/backend-api/wham/rate-limit-reset-credits"
	xaiOfficialAPIBaseURL         = "https://api.x.ai/v1"
	xaiBillingWeeklyURL           = "https://cli-chat-proxy.grok.com/v1/billing?format=credits"
	xaiBillingMonthlyURL          = "https://cli-chat-proxy.grok.com/v1/billing"
	xaiInferenceURL               = "https://cli-chat-proxy.grok.com/v1/responses"
	claudeModelsURL               = "https://api.anthropic.com/v1/models"
	claudeUsageURL                = "https://api.anthropic.com/api/oauth/usage"
	claudeProfileURL              = "https://api.anthropic.com/api/oauth/profile"
	kimiModelsURL                 = "https://api.kimi.com/coding/v1/models"
	kimiUsageURL                  = "https://api.kimi.com/coding/v1/usages"
	antigravityAssistURL          = "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	antigravityQuotaURL           = "https://daily-cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	antigravitySandboxQuotaURL    = "https://daily-cloudcode-pa.sandbox.googleapis.com/v1internal:retrieveUserQuotaSummary"
	antigravityProductionQuotaURL = "https://cloudcode-pa.googleapis.com/v1internal:retrieveUserQuotaSummary"
	googleUserInfoURL             = "https://www.googleapis.com/oauth2/v3/userinfo"
	maxInspectionBody             = 2048
)

var antigravityQuotaURLs = []string{antigravityQuotaURL, antigravitySandboxQuotaURL, antigravityProductionQuotaURL}

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
		r.applyAutoBanInspectionResult(context.WithoutCancel(runCtx), result)
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
		allowed[strings.ToLower(strings.TrimSpace(provider))] = true
	}
	accounts := make([]store.InspectionAccount, 0, len(auths))
	for _, auth := range auths {
		provider := strings.ToLower(strings.TrimSpace(firstNonEmpty(auth.Provider, auth.Type)))
		if !allowed[provider] {
			continue
		}
		authType := inspectionAuthType(auth)
		metadata := inspectionAuthMetadataFromEntry(auth, authType)
		key := firstNonEmpty(auth.AuthIndex, auth.ID, auth.Name)
		display := firstNonEmpty(auth.Email, auth.ProjectID, auth.Label, auth.Name)
		if display == "" && authType == "oauth" {
			display = auth.Account
		}
		accountID := auth.Account
		if authType == "apikey" {
			accountID = ""
		}
		accounts = append(accounts, store.InspectionAccount{
			Key: key, FileName: firstNonEmpty(auth.Name, auth.ID), DisplayName: display,
			AuthID: auth.ID, AuthIndex: auth.AuthIndex, AuthType: authType, AccountID: accountID,
			Provider: provider, Status: auth.Status, Disabled: auth.Disabled, Metadata: metadata,
		})
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
	account = r.enrichInspectionAccount(ctx, account)
	base := store.InspectionResult{
		AccountKey: account.Key, FileName: account.FileName, DisplayAccount: account.DisplayName,
		AuthID: account.AuthID, AuthIndex: account.AuthIndex, AuthType: account.AuthType, AccountID: account.AccountID,
		Provider: account.Provider, Disabled: account.Disabled, Status: account.Status,
		AuthMetadata: inspectionAuthMetadataResult(account), Action: "keep", ActionReason: "探测中", ActionStatus: "pending",
	}
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
	case "claude":
		result = r.probeClaude(ctx, settings, base)
	case "kimi":
		result = r.probeKimi(ctx, settings, base)
	case "antigravity":
		result = r.probeAntigravityQuota(ctx, settings, base)
	case "gemini-cli":
		result = r.probeLoadCodeAssist(ctx, settings, base, "IDE_UNSPECIFIED")
	case "vertex":
		result = r.probeVertex(ctx, settings, base)
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

func (r *Runtime) probeClaude(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	return r.probeClaudeQuota(ctx, settings, result)
}

func (r *Runtime) probeKimi(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	return r.probeKimiQuota(ctx, settings, result)
}

func (r *Runtime) probeLoadCodeAssist(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult, ideType string) store.InspectionResult {
	if strings.TrimSpace(ideType) == "" {
		ideType = "ANTIGRAVITY"
	}
	response, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodPost, antigravityAssistURL, map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Content-Type":  "application/json",
		"User-Agent":    "antigravity/hub/1.23.2",
	}, map[string]any{"metadata": map[string]any{"ideType": ideType}})
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	result = resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, result.Provider)
	if result.ErrorKind == "healthy" {
		// The official quota page only treats Antigravity as a quota provider;
		// Gemini CLI keeps the health check but has no quantitative quota card.
		if result.Provider != "antigravity" {
			result.QuotaWindows = nil
			result.QuotaMetadata = map[string]any{"provider": result.Provider, "mode": "health_only", "quotaSupported": false}
			result.UsedPercent = nil
			result.ActionReason = result.Provider + " 身份探测正常；官方未提供可量化额度"
			return result
		}
		result = applyLoadCodeAssistCredits(result, response.Body)
		if result.PlanType == "" && len(asWindowSlice(result.QuotaWindows)) == 0 {
			result.ActionReason = result.Provider + " 身份探测正常"
		}
	}
	return result
}

func (r *Runtime) probeVertex(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	response, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, googleUserInfoURL, map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Accept":        "application/json",
	}, nil)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	result = resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, "vertex")
	if result.ErrorKind == "healthy" {
		result.ActionReason = "Vertex 凭证有效；GCP 额度需在 Cloud Console 查看"
	}
	return result
}

func applyXAIBilling(result store.InspectionResult, weeklyBody, monthlyBody any) store.InspectionResult {
	weekly := mapValue(weeklyBody)
	monthly := mapValue(monthlyBody)
	if weekly == nil {
		weekly = map[string]any{}
	}
	if monthly == nil {
		monthly = map[string]any{}
	}
	weeklyConfig := xaiBillingConfig(weekly)
	monthlyConfig := xaiBillingConfig(monthly)
	weeklyPeriod := mapValue(firstMap(weeklyConfig["currentPeriod"], weeklyConfig["current_period"]))
	weeklyReset := firstString(weeklyPeriod, "end")
	if weeklyReset == "" {
		weeklyReset = firstString(weeklyConfig, "periodEnd", "period_end")
	}

	windows := []map[string]any{}
	percentValues := []float64{}
	addWindow := func(id, label string, usedPercent *float64, resetAt string, remaining any) {
		kind := strings.TrimPrefix(id, "xai-")
		if strings.HasPrefix(kind, "product-") {
			kind = "product"
		}
		window := map[string]any{"id": id, "kind": kind, "label": label}
		if usedPercent != nil {
			value := clampInspectionPercent(*usedPercent)
			window["usedPercent"] = value
			window["remainingPercent"] = clampInspectionPercent(100 - value)
			percentValues = append(percentValues, value)
		}
		if strings.TrimSpace(resetAt) != "" {
			window["resetAt"] = strings.TrimSpace(resetAt)
		}
		if remaining != nil {
			window["remaining"] = remaining
		}
		windows = append(windows, window)
	}

	weeklyUsed, hasWeeklyUsed := numberFromOK(weeklyConfig, "creditUsagePercent", "credit_usage_percent")
	if hasWeeklyUsed {
		addWindow("xai-weekly", "周限额", &weeklyUsed, weeklyReset, nil)
	}

	productScope := weeklyConfig
	products := arrayValue(firstValue(productScope["productUsage"], productScope["product_usage"]))
	if len(products) == 0 {
		productScope = monthlyConfig
		products = arrayValue(firstValue(productScope["productUsage"], productScope["product_usage"]))
	}
	hasGrokBuild := false
	for index, rawProduct := range products {
		product := mapValue(rawProduct)
		if product == nil {
			continue
		}
		name := firstString(product, "product")
		if name == "" {
			name = fmt.Sprintf("Product %d", index+1)
		}
		normalizedName := strings.ToLower(strings.ReplaceAll(name, " ", ""))
		label := name
		if strings.Contains(normalizedName, "grokbuild") {
			label = "GrokBuild 使用"
			hasGrokBuild = true
		}
		used, ok := numberFromOK(product, "usagePercent", "usage_percent")
		if !ok {
			continue
		}
		addWindow(fmt.Sprintf("xai-product-%d", index), label, &used, weeklyReset, nil)
	}

	monthlyLimit, hasMonthlyLimit := numberFromOK(monthlyConfig, "monthlyLimit", "monthly_limit")
	monthlyUsed, hasMonthlyUsed := numberFromOK(monthlyConfig, "used")
	billingPeriodEnd := firstString(monthlyConfig, "billingPeriodEnd", "billing_period_end")
	if hasMonthlyLimit || hasMonthlyUsed || billingPeriodEnd != "" {
		var usedPercent *float64
		if hasMonthlyLimit && monthlyLimit > 0 && hasMonthlyUsed {
			value := monthlyUsed / monthlyLimit * 100
			usedPercent = &value
		}
		var remaining any
		if hasMonthlyLimit {
			remainingValue := monthlyLimit - monthlyUsed
			if !hasMonthlyUsed {
				remainingValue = monthlyLimit
			}
			if remainingValue < 0 {
				remainingValue = 0
			}
			remaining = remainingValue
		}
		addWindow("xai-monthly", "月度额度", usedPercent, billingPeriodEnd, remaining)
		if result.PlanType == "" {
			switch monthlyLimit {
			case 15000:
				result.PlanType = "SuperGrok"
			case 150000:
				result.PlanType = "SuperGrok Heavy"
			}
		}
	}

	onDemandCap, hasOnDemandCap := numberFromOK(monthlyConfig, "onDemandCap", "on_demand_cap")
	onDemandUsed, hasOnDemandUsed := numberFromOK(monthlyConfig, "onDemandUsed", "on_demand_used")
	if hasOnDemandCap && onDemandCap > 0 || hasOnDemandUsed && onDemandUsed > 0 {
		var usedPercent *float64
		if hasOnDemandCap && onDemandCap > 0 && hasOnDemandUsed {
			value := onDemandUsed / onDemandCap * 100
			usedPercent = &value
		}
		addWindow("xai-on-demand", "按量付费", usedPercent, billingPeriodEnd, nil)
	}

	legacyRate := mapValue(firstMap(weekly["rateLimit"], weekly["rate_limit"], monthly["rateLimit"], monthly["rate_limit"]))
	legacyCredits := mapValue(firstMap(weekly["credits"], monthly["credits"]))
	if legacyRate == nil {
		legacyRate = map[string]any{}
	}
	if legacyCredits == nil {
		legacyCredits = map[string]any{}
	}
	if !hasWeeklyUsed {
		if total, ok := numberFromOK(legacyRate, "totalRequests", "total_requests"); ok && total > 0 {
			remaining := numberFrom(legacyRate, "remainingRequests", "remaining_requests")
			used := (total - remaining) / total * 100
			addWindow("weekly", "周限额", &used, firstString(legacyRate, "windowEnd", "window_end", "resetAt", "reset_at"), nil)
		}
	}
	if !hasGrokBuild {
		if total, ok := numberFromOK(legacyRate, "totalGrokBuilds", "total_grok_builds"); ok && total > 0 {
			remaining := numberFrom(legacyRate, "remainingGrokBuilds", "remaining_grok_builds")
			used := (total - remaining) / total * 100
			addWindow("grokbuild", "GrokBuild 使用", &used, "", nil)
		}
	}
	if payg, ok := boolFrom(legacyCredits, "payAsYouGoEnabled", "pay_as_you_go_enabled"); ok {
		detail := "未启用"
		if payg {
			detail = "已启用"
		}
		addWindow("payg", "按量付费", nil, "", detail)
	}
	if !hasMonthlyLimit && !hasMonthlyUsed && billingPeriodEnd == "" {
		legacyLimit, hasLegacyLimit := numberFromOK(legacyCredits, "monthlyCredits", "monthly_credits")
		legacyRemaining, hasLegacyRemaining := numberFromOK(legacyCredits, "remainingCredits", "remaining_credits")
		legacyReset := firstString(legacyCredits, "nextMonthlyRefresh", "next_monthly_refresh")
		if hasLegacyLimit || hasLegacyRemaining || legacyReset != "" {
			used := 0.0
			if hasLegacyLimit && legacyLimit > 0 {
				used = (legacyLimit - legacyRemaining) / legacyLimit * 100
			}
			addWindow("monthly", "月度额度", &used, legacyReset, legacyRemaining)
		}
	}

	if len(percentValues) > 0 {
		maxPercent := percentValues[0]
		for _, value := range percentValues[1:] {
			if value > maxPercent {
				maxPercent = value
			}
		}
		result.UsedPercent = &maxPercent
	}

	// Keep xAI's separate clocks and money units intact. The weekly endpoint
	// describes rate-limited capacity; the monthly endpoint describes included
	// billing credits and the optional on-demand cap.
	periodType := "unknown"
	periodStart := ""
	periodEnd := ""
	periodTypeRaw := strings.ToLower(firstString(weeklyPeriod, "type"))
	if strings.Contains(periodTypeRaw, "weekly") {
		periodType = "weekly"
	} else if strings.Contains(periodTypeRaw, "monthly") {
		periodType = "monthly"
	}
	periodStart = firstString(weeklyPeriod, "start")
	periodEnd = firstString(weeklyPeriod, "end")
	if periodType == "unknown" {
		monthlyPeriod := mapValue(firstMap(monthlyConfig["currentPeriod"], monthlyConfig["current_period"]))
		monthlyTypeRaw := strings.ToLower(firstString(monthlyPeriod, "type"))
		if strings.Contains(monthlyTypeRaw, "weekly") {
			periodType = "weekly"
		} else if strings.Contains(monthlyTypeRaw, "monthly") {
			periodType = "monthly"
		}
		if periodStart == "" {
			periodStart = firstString(monthlyPeriod, "start")
		}
		if periodEnd == "" {
			periodEnd = firstString(monthlyPeriod, "end")
		}
	}
	if periodType == "unknown" && len(windows) > 0 {
		periodType = "weekly"
	}
	billingPeriodStart := firstString(monthlyConfig, "billingPeriodStart", "billing_period_start")
	billingPeriodEnd = firstString(monthlyConfig, "billingPeriodEnd", "billing_period_end")
	monthlyRemaining := 0.0
	if hasMonthlyLimit {
		monthlyRemaining = monthlyLimit
		if hasMonthlyUsed {
			monthlyRemaining = monthlyLimit - monthlyUsed
		}
		if monthlyRemaining < 0 {
			monthlyRemaining = 0
		}
	}
	includedUsed := monthlyUsed
	if hasMonthlyLimit && hasMonthlyUsed && includedUsed > monthlyLimit {
		includedUsed = monthlyLimit
	}
	derivedOnDemandUsed := 0.0
	if hasMonthlyLimit && hasMonthlyUsed && monthlyUsed > monthlyLimit {
		derivedOnDemandUsed = monthlyUsed - monthlyLimit
	}
	if !hasOnDemandUsed && derivedOnDemandUsed > 0 {
		onDemandUsed = derivedOnDemandUsed
		hasOnDemandUsed = true
	}
	for _, window := range windows {
		switch fmt.Sprint(window["id"]) {
		case "xai-monthly", "monthly":
			if hasMonthlyLimit {
				window["limit"] = monthlyLimit
				window["remaining"] = monthlyRemaining
			}
			if hasMonthlyUsed {
				window["used"] = monthlyUsed
			}
		case "xai-on-demand":
			if hasOnDemandCap {
				window["limit"] = onDemandCap
			}
			if hasOnDemandUsed {
				window["used"] = onDemandUsed
				if hasOnDemandCap {
					window["remaining"] = maxFloat(onDemandCap - onDemandUsed)
				}
			}
		}
	}
	productUsage := []map[string]any{}
	for _, rawProduct := range products {
		if product := mapValue(rawProduct); product != nil {
			productUsage = append(productUsage, map[string]any{
				"product":      firstString(product, "product"),
				"usagePercent": numberFrom(product, "usagePercent", "usage_percent"),
			})
		}
	}
	billingMetadata := map[string]any{
		"provider":           "xai",
		"mode":               "billing",
		"periodType":         periodType,
		"periodStart":        periodStart,
		"periodEnd":          periodEnd,
		"billingPeriodStart": billingPeriodStart,
		"billingPeriodEnd":   billingPeriodEnd,
		"productUsage":       productUsage,
	}
	if hasWeeklyUsed {
		billingMetadata["usagePercent"] = clampInspectionPercent(weeklyUsed)
	}
	if hasMonthlyLimit {
		billingMetadata["monthlyLimitCents"] = monthlyLimit
	}
	if hasMonthlyUsed {
		billingMetadata["usedCents"] = monthlyUsed
		billingMetadata["includedUsedCents"] = includedUsed
	}
	if hasOnDemandCap {
		billingMetadata["onDemandCapCents"] = onDemandCap
	}
	if hasOnDemandUsed {
		billingMetadata["onDemandUsedCents"] = onDemandUsed
		if hasOnDemandCap && onDemandCap > 0 {
			billingMetadata["onDemandUsedPercent"] = clampInspectionPercent(onDemandUsed / onDemandCap * 100)
		}
	}
	if result.PlanType != "" {
		billingMetadata["planType"] = result.PlanType
	}
	result = quotaMetadata(result, billingMetadata, windows)
	if len(windows) > 0 {
		result.ActionReason = "已读取 xAI 套餐与额度窗口"
		if result.ErrorKind == "" {
			result.ErrorKind = "healthy"
		}
	}
	return result
}

func xaiBillingConfig(body map[string]any) map[string]any {
	if body == nil {
		return map[string]any{}
	}
	if config := mapValue(body["config"]); config != nil {
		return config
	}
	return body
}

func clampInspectionPercent(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}

func firstValue(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstMap(values ...any) any {
	for _, value := range values {
		if mapValue(value) != nil {
			return value
		}
	}
	return nil
}

func firstString(scope map[string]any, keys ...string) string {
	if scope == nil {
		return ""
	}
	for _, key := range keys {
		if text := strings.TrimSpace(fmt.Sprint(scope[key])); text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func numberFrom(scope map[string]any, keys ...string) float64 {
	number, _ := numberFromOK(scope, keys...)
	return number
}

func numberFromOK(scope map[string]any, keys ...string) (float64, bool) {
	if scope == nil {
		return 0, false
	}
	for _, key := range keys {
		if number, ok := numberValue(scope[key]); ok {
			return number, true
		}
	}
	return 0, false
}

func boolFrom(scope map[string]any, keys ...string) (bool, bool) {
	if scope == nil {
		return false, false
	}
	for _, key := range keys {
		value, exists := scope[key]
		if !exists || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "1", "yes", "on":
				return true, true
			case "false", "0", "no", "off":
				return false, true
			}
		case float64:
			return typed != 0, true
		}
	}
	return false, false
}

func applyLoadCodeAssistCredits(result store.InspectionResult, body any) store.InspectionResult {
	root := mapValue(body)
	if root == nil {
		return result
	}
	paid := mapValue(root["paidTier"])
	if paid == nil {
		paid = mapValue(root["currentTier"])
	}
	if paid == nil {
		paid = map[string]any{}
	}
	if id := strings.TrimSpace(fmt.Sprint(paid["id"])); id != "" && id != "<nil>" {
		result.PlanType = id
	}
	windows := []map[string]any{}
	if credits, ok := paid["availableCredits"].([]any); ok {
		for _, item := range credits {
			credit := mapValue(item)
			label := strings.TrimSpace(fmt.Sprint(credit["creditType"]))
			amount, okAmount := numberValue(credit["creditAmount"])
			if !okAmount {
				if text := strings.TrimSpace(fmt.Sprint(credit["creditAmount"])); text != "" && text != "<nil>" {
					if parsed, err := strconv.ParseFloat(text, 64); err == nil {
						amount, okAmount = parsed, true
					}
				}
			}
			if label == "" || label == "<nil>" {
				label = "credits"
			}
			window := map[string]any{"id": label, "label": label}
			if okAmount {
				window["remaining"] = amount
			}
			windows = append(windows, window)
		}
	}
	if len(windows) > 0 {
		result.QuotaWindows = windows
		result.ActionReason = "已读取 Code Assist 额度"
	}
	return result
}

func asWindowSlice(value any) []map[string]any {
	switch typed := value.(type) {
	case []map[string]any:
		return typed
	case []any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if converted := mapValue(item); converted != nil {
				out = append(out, converted)
			}
		}
		return out
	default:
		return nil
	}
}

func (r *Runtime) ProbeAccountQuota(ctx context.Context, authIndex, provider, source string) (store.InspectionResult, error) {
	return r.ProbeAccountQuotaWithIdentity(ctx, authIndex, provider, source, "", "", "")
}

func (r *Runtime) ProbeAccountQuotaWithIdentity(ctx context.Context, authIndex, provider, source, authID, authType, fileName string) (store.InspectionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	list := r.authList
	r.mu.Unlock()
	if list == nil {
		return store.InspectionResult{}, fmt.Errorf("host auth callback is unavailable")
	}
	auths, err := list()
	if err != nil {
		return store.InspectionResult{}, err
	}
	account, ok := findInspectionAccountByIdentity(auths, authIndex, provider, source, authID, authType, fileName)
	if !ok {
		return store.InspectionResult{}, fmt.Errorf("未找到对应认证文件")
	}
	if !supportedInspectionProvider(account.Provider) {
		return store.InspectionResult{}, fmt.Errorf("不支持的巡检提供商: %s", account.Provider)
	}
	if normalized := normalizeInspectionAuthType(authType); normalized != "" {
		account.AuthType = normalized
		account.Metadata.AuthType = normalized
	}
	if strings.TrimSpace(authID) != "" {
		account.AuthID = strings.TrimSpace(authID)
		account.Metadata.ID = account.AuthID
	}
	settings := r.CodexInspectionSettings()
	settings.XAIInferenceEnabled = false
	return r.probeInspectionAccount(ctx, settings, account), nil
}

func findInspectionAccount(auths []pluginapi.HostAuthFileEntry, authIndex, provider, source string) (store.InspectionAccount, bool) {
	return findInspectionAccountByIdentity(auths, authIndex, provider, source, "", "", "")
}

func findInspectionAccountByIdentity(auths []pluginapi.HostAuthFileEntry, authIndex, provider, source, authID, authType, fileName string) (store.InspectionAccount, bool) {
	authIndex = strings.TrimSpace(authIndex)
	provider = strings.ToLower(strings.TrimSpace(provider))
	source = strings.ToLower(strings.TrimSpace(source))
	authID = strings.TrimSpace(authID)
	authType = normalizeInspectionAuthType(authType)
	fileName = strings.ToLower(strings.TrimSpace(fileName))
	accounts := filterInspectionAccounts(auths, CodexInspectionSettings{TargetTypes: append([]string{}, inspectionProviders...)})
	matchProvider := func(account store.InspectionAccount) bool {
		return provider == "" || account.Provider == provider
	}
	matchType := func(account store.InspectionAccount) bool {
		return authType == "" || normalizeInspectionAuthType(account.AuthType) == authType || account.AuthType == ""
	}
	if authIndex != "" {
		for _, account := range accounts {
			if strings.TrimSpace(account.AuthIndex) == authIndex && matchProvider(account) && matchType(account) {
				return account, true
			}
		}
	}
	if authID != "" {
		for _, account := range accounts {
			if strings.TrimSpace(account.AuthID) == authID && matchProvider(account) && matchType(account) {
				return account, true
			}
		}
	}
	if fileName != "" {
		for _, account := range accounts {
			if strings.ToLower(strings.TrimSpace(account.FileName)) == fileName && matchProvider(account) && matchType(account) {
				return account, true
			}
		}
	}
	if source != "" {
		candidates := make([]store.InspectionAccount, 0, 2)
		for _, account := range accounts {
			if !matchProvider(account) || !matchType(account) {
				continue
			}
			display := strings.ToLower(strings.TrimSpace(account.DisplayName))
			if display == source {
				candidates = append(candidates, account)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], true
		}
		candidates = candidates[:0]
		for _, account := range accounts {
			if !matchProvider(account) || !matchType(account) {
				continue
			}
			name := strings.ToLower(strings.TrimSpace(account.FileName))
			if strings.Contains(name, source) || strings.Contains(strings.ToLower(account.DisplayName), source) {
				candidates = append(candidates, account)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], true
		}
	}
	return store.InspectionAccount{}, false
}

func (r *Runtime) probeCodex(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	return r.probeCodexQuota(ctx, settings, result)
}

func (r *Runtime) probeXAI(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	metadata, _ := r.inspectionAuthMetadata(ctx, result.AuthIndex)
	baseURL, officialAPI, userID := resolveXAIProbeMetadata(metadata)
	if normalizeInspectionAuthType(result.AuthType) == "apikey" {
		officialAPI = true
	}
	if officialAPI {
		base := strings.TrimSuffix(baseURL, "/")
		profile, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, base+"/me", map[string]string{"Authorization": "Bearer $TOKEN$", "Accept": "application/json"}, nil)
		if err != nil {
			return inspectionFailure(result, 0, "upstream_error", err.Error())
		}
		result = resolveInspectionHTTPResult(result, profile, settings.UsedPercentThreshold, "xai")
		if profile.StatusCode >= 200 && profile.StatusCode < 300 {
			chat, chatErr := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodPost, base+"/chat/completions", map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "Accept": "application/json"}, map[string]any{
				"model": "grok-4.5", "messages": []map[string]string{{"role": "user", "content": "ping"}}, "max_tokens": 1, "stream": false,
			})
			if chatErr != nil {
				return inspectionFailure(result, 0, "upstream_error", chatErr.Error())
			}
			if chat.StatusCode < 200 || chat.StatusCode >= 300 {
				return resolveInspectionHTTPResult(result, chat, settings.UsedPercentThreshold, "xai")
			}
		}
		if result.ErrorKind == "healthy" {
			result.PlanType = "paid"
			result = quotaMetadata(result, map[string]any{"provider": "xai", "mode": "paid-health", "quotaSupported": false, "healthStatus": "chat-ok"}, nil)
			result.ActionReason = "xAI 官方 API 身份探测正常；付费额度由 xAI 账单管理"
		}
		if settings.XAIInferenceEnabled && result.Action == "keep" {
			return r.probeXAIInference(ctx, settings, result, base+"/responses", map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "User-Agent": settings.XAIInferenceUserAgent})
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
	if result.ErrorKind == "healthy" || result.ErrorKind == "" {
		result = applyXAIBilling(result, weekly.Body, monthly.Body)
		result = applyInspectionQuotaThreshold(result, settings.UsedPercentThreshold)
	}
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
	if document := r.inspectionAuthDocument(ctx, authIndex); document != nil {
		return document, nil
	}
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
	baseURL = xaiOfficialAPIBaseURL
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
	kind := strings.ToLower(read("auth_kind", "authKind"))
	credentialType := strings.ToLower(read("type"))
	rawUsingAPI := read("using_api", "usingApi")
	usingAPI := strings.EqualFold(rawUsingAPI, "true") || kind == "api_key" || kind == "apikey" || kind == "api" || credentialType == "api_key" || credentialType == "apikey" || credentialType == "api"
	if rawUsingAPI == "" && kind != "" && kind != "oauth" && kind != "oauth2" {
		usingAPI = true
	}
	if candidate != "" && !strings.Contains(strings.ToLower(candidate), "cli-chat-proxy.grok.com") {
		baseURL = candidate
		// xAI OAuth files commonly persist api.x.ai as their default base URL,
		// while OAuth chat and billing still use the CLI chat proxy unless
		// using_api is explicitly enabled.
		defaultOfficialBase := strings.EqualFold(candidate, xaiOfficialAPIBaseURL)
		oauthLike := kind == "" || kind == "oauth" || kind == "oauth2"
		officialAPI = usingAPI || !(defaultOfficialBase && oauthLike)
	}
	if usingAPI {
		officialAPI = true
	}
	userID = read("user_id", "userId", "sub", "subject", "id")
	return baseURL, officialAPI, userID
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case string:
		var parsed map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(typed)), &parsed); err == nil {
			return parsed
		}
	}
	return nil
}

func arrayValue(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case string:
		var parsed []any
		if err := json.Unmarshal([]byte(strings.TrimSpace(typed)), &parsed); err == nil {
			return parsed
		}
	}
	return nil
}

func applyInspectionQuotaThreshold(result store.InspectionResult, threshold float64) store.InspectionResult {
	if result.UsedPercent == nil || threshold >= 100 || *result.UsedPercent < threshold {
		return result
	}
	result.Action, result.ActionReason, result.IsQuota, result.ErrorKind = "disable", "额度达到配置阈值", true, "quota_threshold"
	return result
}

func resolveInspectionHTTPResult(result store.InspectionResult, response inspectionAPIResponse, threshold float64, provider string) store.InspectionResult {
	result.StatusCode = intPtr(response.StatusCode)
	status := response.StatusCode
	body := strings.ToLower(response.BodyText)
	switch {
	case status >= 200 && status < 300:
		if result.Disabled {
			result.Action, result.ActionReason, result.ErrorKind = "keep", "凭证已禁用，等待自动恢复归属校验", "disabled"
		} else {
			result.Action, result.ActionReason, result.ErrorKind = "keep", "provider 探测正常", "healthy"
		}
		result = applyInspectionQuotaThreshold(result, threshold)
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
		encodedBody, err := json.Marshal(requestBody)
		if err != nil {
			return inspectionAPIResponse{}, err
		}
		payload["data"] = string(encodedBody)
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
		var parsed any
		if err := json.Unmarshal([]byte(strings.TrimSpace(value)), &parsed); err == nil {
			bodyValue = parsed
		}
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
	autoBanEnabled := r.AutoBanSettings().Enabled
	for _, result := range results {
		if settings.AutoRecoverEnabled && result.Action == "enable" && result.AutoRecoverEligible {
			if !r.autoBanBlocksInspectionRecover(result) {
				ids = append(ids, result.ID)
			}
			continue
		}
		// Auto-Ban owns status-code-driven disable/delete actions once enabled.
		if !autoBanEnabled && result.Action == settings.AutoActionMode {
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
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	case json.Number:
		v, err := value.Float64()
		return v, err == nil
	case string:
		text := strings.TrimSpace(value)
		if strings.HasSuffix(text, "%") {
			text = strings.TrimSpace(strings.TrimSuffix(text, "%"))
		}
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		return parsed, err == nil
	case map[string]any:
		for _, key := range []string{"val", "value", "amount"} {
			if nested, ok := numberValue(value[key]); ok {
				return nested, true
			}
		}
	}
	return 0, false
}
func findHighestPercent(value any) *float64 {
	var best *float64
	var walk func(any)
	walk = func(value any) {
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				norm := strings.ToLower(strings.ReplaceAll(key, "_", ""))
				if strings.Contains(norm, "usedpercent") || strings.Contains(norm, "usagepercent") {
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
