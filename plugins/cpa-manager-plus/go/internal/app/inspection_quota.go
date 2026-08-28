package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

// normalizeInspectionAuthType uses the same two credential categories emitted
// by the host usage records while accepting the host's API-key spellings.
func normalizeInspectionAuthType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "oauth", "oauth2":
		return "oauth"
	case "api", "api_key", "api-key", "apikey":
		return "apikey"
	default:
		return strings.TrimSpace(value)
	}
}

func inspectionAuthType(auth pluginapi.HostAuthFileEntry) string {
	return normalizeInspectionAuthType(auth.AccountType)
}

func inspectionAuthMetadataFromEntry(auth pluginapi.HostAuthFileEntry, authType string) store.InspectionAuthMetadata {
	if authType == "" {
		authType = inspectionAuthType(auth)
	}
	account := strings.TrimSpace(auth.Account)
	if authType == "apikey" {
		account = ""
	}
	metadata := store.InspectionAuthMetadata{
		ID:            strings.TrimSpace(auth.ID),
		AuthIndex:     strings.TrimSpace(auth.AuthIndex),
		Name:          strings.TrimSpace(auth.Name),
		Type:          strings.TrimSpace(auth.Type),
		Provider:      strings.TrimSpace(firstNonEmpty(auth.Provider, auth.Type)),
		AuthType:      authType,
		Label:         strings.TrimSpace(auth.Label),
		Status:        strings.TrimSpace(auth.Status),
		StatusMessage: strings.TrimSpace(auth.StatusMessage),
		Disabled:      auth.Disabled,
		Unavailable:   auth.Unavailable,
		RuntimeOnly:   auth.RuntimeOnly,
		Source:        strings.TrimSpace(auth.Source),
		Path:          strings.TrimSpace(auth.Path),
		Email:         strings.TrimSpace(auth.Email),
		ProjectID:     strings.TrimSpace(auth.ProjectID),
		AccountType:   strings.TrimSpace(auth.AccountType),
		Account:       account,
		Size:          auth.Size,
		Priority:      inspectionIntPointer(auth.Priority),
	}
	if !auth.ModTime.IsZero() {
		metadata.ModTime = auth.ModTime.Format(time.RFC3339Nano)
	}
	if !auth.UpdatedAt.IsZero() {
		metadata.UpdatedAt = auth.UpdatedAt.Format(time.RFC3339Nano)
	}
	if !auth.CreatedAt.IsZero() {
		metadata.CreatedAt = auth.CreatedAt.Format(time.RFC3339Nano)
	}
	if !auth.LastRefresh.IsZero() {
		metadata.LastRefresh = auth.LastRefresh.Format(time.RFC3339Nano)
	}
	if !auth.NextRetryAfter.IsZero() {
		metadata.NextRetryAfter = auth.NextRetryAfter.Format(time.RFC3339Nano)
	}
	return metadata
}

func inspectionIntPointer(value int) *int {
	if value == 0 {
		return nil
	}
	copy := value
	return &copy
}

func authDocumentScopes(document map[string]any) []map[string]any {
	if document == nil {
		return nil
	}
	scopes := []map[string]any{document}
	for _, key := range []string{"metadata", "attributes", "oauth", "user", "subscription", "installed", "web", "token"} {
		if value := mapValue(document[key]); value != nil {
			scopes = append(scopes, value)
		}
	}
	return scopes
}

func authDocumentValue(document map[string]any, keys ...string) any {
	for _, scope := range authDocumentScopes(document) {
		for _, key := range keys {
			if value, ok := scope[key]; ok && value != nil {
				return value
			}
		}
	}
	return nil
}

func authDocumentString(document map[string]any, keys ...string) string {
	value := authDocumentValue(document, keys...)
	if value == nil {
		return ""
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	return text
}

func mergeInspectionAuthMetadata(metadata store.InspectionAuthMetadata, document map[string]any) store.InspectionAuthMetadata {
	if document == nil {
		return metadata
	}
	if metadata.ID == "" {
		metadata.ID = authDocumentString(document, "id", "auth_id", "authId")
	}
	if metadata.Name == "" {
		metadata.Name = authDocumentString(document, "name", "file_name", "fileName")
	}
	if metadata.Label == "" {
		metadata.Label = authDocumentString(document, "label")
	}
	if metadata.Email == "" {
		metadata.Email = authDocumentString(document, "email")
	}
	if metadata.ProjectID == "" {
		metadata.ProjectID = authDocumentString(document, "project_id", "projectId", "gemini_virtual_project")
	}
	if metadata.Note == "" {
		metadata.Note = authDocumentString(document, "note")
	}
	if metadata.AuthType == "" {
		metadata.AuthType = normalizeInspectionAuthType(authDocumentString(document, "auth_kind", "authKind", "account_type", "accountType"))
	}
	if metadata.AccountType == "" {
		metadata.AccountType = authDocumentString(document, "account_type", "accountType")
	}
	if metadata.Source == "" {
		metadata.Source = authDocumentString(document, "source")
	}
	if metadata.Path == "" {
		metadata.Path = authDocumentString(document, "path")
	}
	if metadata.Priority == nil {
		if value, ok := numberFromOK(authDocumentScope(document), "priority"); ok {
			priority := int(value)
			metadata.Priority = &priority
		}
	}
	if metadata.Weight == nil {
		if value, ok := numberFromOK(authDocumentScope(document), "weight"); ok {
			weight := int(value)
			metadata.Weight = &weight
		}
	}
	return metadata
}

func authDocumentScope(document map[string]any) map[string]any {
	if document == nil {
		return nil
	}
	return document
}

func parseJSONMap(raw []byte) map[string]any {
	var value map[string]any
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	return value
}

func parseTokenPayload(value any) map[string]any {
	if parsed := mapValue(value); parsed != nil {
		return parsed
	}
	text, ok := value.(string)
	if !ok {
		return nil
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ".")
	if len(parts) < 2 {
		return nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(parts[1])
	}
	if err != nil {
		return nil
	}
	return parseJSONMap(decoded)
}

func authDocumentClaims(document map[string]any) []map[string]any {
	claims := []map[string]any{}
	for _, scope := range authDocumentScopes(document) {
		for _, key := range []string{"id_token", "idToken"} {
			if value := parseTokenPayload(scope[key]); value != nil {
				claims = append(claims, value)
				if nested := mapValue(value["https://api.openai.com/auth"]); nested != nil {
					claims = append(claims, nested)
				}
			}
		}
	}
	return claims
}

func resolveCodexDocumentValues(document map[string]any) (planType string, subscriptionActiveUntil any, accountID string) {
	scopes := append([]map[string]any{}, authDocumentScopes(document)...)
	scopes = append(scopes, authDocumentClaims(document)...)
	for _, scope := range scopes {
		if planType == "" {
			planType = strings.ToLower(firstString(scope, "plan_type", "planType"))
		}
		if subscriptionActiveUntil == nil {
			subscriptionActiveUntil = firstDateValue(scope, "chatgpt_subscription_active_until", "chatgptSubscriptionActiveUntil", "subscription_active_until", "subscriptionActiveUntil")
		}
		if accountID == "" {
			accountID = firstString(scope, "chatgpt_account_id", "chatgptAccountId")
		}
	}
	return
}

func firstDateValue(scope map[string]any, keys ...string) any {
	if scope == nil {
		return nil
	}
	for _, key := range keys {
		if value := normalizeDateValue(scope[key]); value != nil {
			return value
		}
	}
	return nil
}

func normalizeDateValue(value any) any {
	if value == nil {
		return nil
	}
	if number, ok := numberValue(value); ok {
		if number <= 0 {
			return nil
		}
		return number
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" || text == "0" {
		return nil
	}
	return text
}

func (r *Runtime) inspectionAuthDocument(ctx context.Context, authIndex string) map[string]any {
	r.mu.Lock()
	get := r.authGet
	r.mu.Unlock()
	if get == nil || strings.TrimSpace(authIndex) == "" {
		return nil
	}
	response, err := get(strings.TrimSpace(authIndex))
	if err != nil || len(response.JSON) == 0 {
		return nil
	}
	return parseJSONMap(response.JSON)
}

func (r *Runtime) enrichInspectionAccount(ctx context.Context, account store.InspectionAccount) store.InspectionAccount {
	document := r.inspectionAuthDocument(ctx, account.AuthIndex)
	if document == nil {
		return account
	}
	account.Metadata = mergeInspectionAuthMetadata(account.Metadata, document)
	if account.AuthID == "" {
		account.AuthID = account.Metadata.ID
	}
	if account.AuthType == "" {
		account.AuthType = account.Metadata.AuthType
	}
	if account.DisplayName == "" || account.DisplayName == account.FileName {
		account.DisplayName = inspectionDisplayName(account.Metadata)
	}
	if account.Provider == "" {
		account.Provider = strings.ToLower(strings.TrimSpace(account.Metadata.Provider))
	}
	return account
}

func inspectionDisplayName(metadata store.InspectionAuthMetadata) string {
	if metadata.Email != "" {
		return metadata.Email
	}
	if metadata.ProjectID != "" {
		return metadata.ProjectID
	}
	if metadata.Label != "" {
		return metadata.Label
	}
	if metadata.AuthType != "apikey" && metadata.Account != "" {
		return metadata.Account
	}
	return firstNonEmpty(metadata.Name, metadata.ID)
}

func inspectionAuthMetadataResult(account store.InspectionAccount) *store.InspectionAuthMetadata {
	metadata := account.Metadata
	if metadata.ID == "" && account.AuthID != "" {
		metadata.ID = account.AuthID
	}
	if metadata.AuthIndex == "" {
		metadata.AuthIndex = account.AuthIndex
	}
	if metadata.Name == "" {
		metadata.Name = account.FileName
	}
	if metadata.Provider == "" {
		metadata.Provider = account.Provider
	}
	if metadata.AuthType == "" {
		metadata.AuthType = account.AuthType
	}
	if metadata.Status == "" {
		metadata.Status = account.Status
	}
	if !metadata.Disabled {
		metadata.Disabled = account.Disabled
	}
	if metadata.Email == "" && account.AuthType == "oauth" && account.DisplayName != account.FileName {
		metadata.Email = account.DisplayName
	}
	return &metadata
}

func quotaMetadata(result store.InspectionResult, metadata map[string]any, windows []map[string]any) store.InspectionResult {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if windows != nil {
		result.QuotaWindows = windows
		metadata["windows"] = windows
	}
	if result.PlanType != "" {
		metadata["planType"] = result.PlanType
	}
	result.QuotaMetadata = metadata
	return result
}

func inspectBodyObject(value any) map[string]any {
	root := mapValue(value)
	if root == nil {
		return nil
	}
	if nested := mapValue(root["body"]); nested != nil {
		return nested
	}
	return root
}

func inspectionResetAt(scope map[string]any) string {
	if scope == nil {
		return ""
	}
	for _, key := range []string{"reset_at", "resetAt", "reset_time", "resetTime"} {
		if value := formatInspectionInstant(scope[key]); value != "" {
			return value
		}
	}
	for _, key := range []string{"reset_after_seconds", "resetAfterSeconds", "reset_in", "resetIn", "ttl"} {
		if seconds, ok := numberValue(scope[key]); ok && seconds > 0 {
			return time.Now().Add(time.Duration(seconds * float64(time.Second))).UTC().Format(time.RFC3339Nano)
		}
	}
	return ""
}

func formatInspectionInstant(value any) string {
	if value == nil {
		return ""
	}
	if number, ok := numberValue(value); ok && number > 0 {
		if number < 1e11 {
			number *= 1000
		}
		return time.UnixMilli(int64(number)).UTC().Format(time.RFC3339Nano)
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return ""
	}
	if number, err := strconv.ParseFloat(text, 64); err == nil && number > 0 {
		return formatInspectionInstant(number)
	}
	if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	if parsed, err := time.Parse(time.RFC3339, text); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return text
}

func inspectionPeriodHours(seconds any) any {
	value, ok := numberValue(seconds)
	if !ok || value <= 0 {
		return nil
	}
	return value / 3600
}

func inspectionUsedPercent(scope map[string]any, limitReached, allowed bool, resetAt string) (float64, bool) {
	if value, ok := numberFromOK(scope, "used_percent", "usedPercent", "usage_percent", "usagePercent", "utilization"); ok {
		return clampInspectionPercent(value), true
	}
	if (limitReached || !allowed) && resetAt != "" {
		return 100, true
	}
	return 0, false
}

func makeInspectionWindow(id, kind, label string, scope map[string]any, usedPercent *float64) map[string]any {
	window := map[string]any{"id": id, "kind": kind, "label": label}
	if usedPercent != nil {
		used := clampInspectionPercent(*usedPercent)
		window["usedPercent"] = used
		window["remainingPercent"] = clampInspectionPercent(100 - used)
	}
	if resetAt := inspectionResetAt(scope); resetAt != "" {
		window["resetAt"] = resetAt
	}
	if period := inspectionPeriodHours(firstValue(scope["limit_window_seconds"], scope["limitWindowSeconds"])); period != nil {
		window["periodHours"] = period
	}
	return window
}

func maxWindowUsedPercent(windows []map[string]any) *float64 {
	var result *float64
	for _, window := range windows {
		value, ok := numberValue(window["usedPercent"])
		if !ok {
			continue
		}
		if result == nil || value > *result {
			copy := value
			result = &copy
		}
	}
	return result
}

func codexLimitInfo(root map[string]any, key string) map[string]any {
	if root == nil {
		return nil
	}
	return mapValue(firstValue(root[key], root[strings.ReplaceAll(key, "_", "")]))
}

func codexClassifiedWindows(info map[string]any) (fiveHour, secondary map[string]any) {
	if info == nil {
		return nil, nil
	}
	primary := mapValue(firstValue(info["primary_window"], info["primaryWindow"]))
	secondaryRaw := mapValue(firstValue(info["secondary_window"], info["secondaryWindow"]))
	windows := []map[string]any{primary, secondaryRaw}
	primaryUsedAsSecondary := false
	secondaryUsedAsFiveHour := false
	for index, window := range windows {
		if window == nil {
			continue
		}
		seconds, _ := numberFromOK(window, "limit_window_seconds", "limitWindowSeconds")
		switch {
		case seconds == 18000 && fiveHour == nil:
			fiveHour = window
			if index == 1 {
				secondaryUsedAsFiveHour = true
			}
		case (seconds == 604800 || (seconds >= 28*24*3600 && seconds <= 31*24*3600)) && secondary == nil:
			secondary = window
			if index == 0 {
				primaryUsedAsSecondary = true
			}
		}
	}
	if fiveHour == nil && primary != nil && !primaryUsedAsSecondary {
		fiveHour = primary
	}
	if secondary == nil && secondaryRaw != nil && !secondaryUsedAsFiveHour {
		secondary = secondaryRaw
	}
	return fiveHour, secondary
}

func appendCodexWindow(windows *[]map[string]any, id, kind, label string, window, info map[string]any) {
	if window == nil {
		return
	}
	limitReached, _ := boolFrom(info, "limit_reached", "limitReached")
	allowed, hasAllowed := boolFrom(info, "allowed")
	if !hasAllowed {
		allowed = true
	}
	resetAt := inspectionResetAt(window)
	used, hasUsed := inspectionUsedPercent(window, limitReached, allowed, resetAt)
	var usedPtr *float64
	if hasUsed {
		usedPtr = &used
	}
	item := makeInspectionWindow(id, kind, label, window, usedPtr)
	if resetAt != "" {
		item["resetAt"] = resetAt
	}
	if seconds, ok := numberFromOK(window, "limit_window_seconds", "limitWindowSeconds"); ok {
		item["periodHours"] = seconds / 3600
	}
	if limitReached || !allowed {
		item["limitReached"] = true
	}
	*windows = append(*windows, item)
}

func normalizeInspectionWindowID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	lastDash := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastDash = false
		} else if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func buildCodexInspectionWindows(root map[string]any) []map[string]any {
	if root == nil {
		return nil
	}
	windows := []map[string]any{}
	limit := codexLimitInfo(root, "rate_limit")
	five, secondary := codexClassifiedWindows(limit)
	appendCodexWindow(&windows, "five-hour", "five_hour", "5 小时限额", five, limit)
	secondaryID, secondaryKind, secondaryLabel := "weekly", "weekly", "周限额"
	if seconds, ok := numberFromOK(secondary, "limit_window_seconds", "limitWindowSeconds"); ok && seconds >= 28*24*3600 && seconds <= 31*24*3600 {
		secondaryID, secondaryKind, secondaryLabel = "monthly", "monthly", "月限额"
	}
	appendCodexWindow(&windows, secondaryID, secondaryKind, secondaryLabel, secondary, limit)

	codeReview := codexLimitInfo(root, "code_review_rate_limit")
	reviewFive, reviewSecondary := codexClassifiedWindows(codeReview)
	appendCodexWindow(&windows, "code-review-five-hour", "code_review_five_hour", "代码审查 5 小时限额", reviewFive, codeReview)
	reviewID, reviewKind, reviewLabel := "code-review-weekly", "code_review_weekly", "代码审查周限额"
	if seconds, ok := numberFromOK(reviewSecondary, "limit_window_seconds", "limitWindowSeconds"); ok && seconds >= 28*24*3600 && seconds <= 31*24*3600 {
		reviewID, reviewKind, reviewLabel = "code-review-monthly", "code_review_monthly", "代码审查月限额"
	}
	appendCodexWindow(&windows, reviewID, reviewKind, reviewLabel, reviewSecondary, codeReview)

	additional := arrayValue(firstValue(root["additional_rate_limits"], root["additionalRateLimits"]))
	for index, raw := range additional {
		item := mapValue(raw)
		if item == nil {
			continue
		}
		info := mapValue(firstValue(item["rate_limit"], item["rateLimit"]))
		if info == nil {
			continue
		}
		name := firstString(item, "limit_name", "limitName", "metered_feature", "meteredFeature")
		if name == "" {
			name = fmt.Sprintf("额外限额 %d", index+1)
		}
		prefix := normalizeInspectionWindowID(name)
		if prefix == "" {
			prefix = fmt.Sprintf("additional-%d", index+1)
		}
		five, secondary := codexClassifiedWindows(info)
		appendCodexWindow(&windows, fmt.Sprintf("%s-five-hour-%d", prefix, index), "additional_five_hour", name+" 5 小时限额", five, info)
		secondaryID, secondaryKind, secondaryLabel := "weekly", "additional_weekly", name+" 周限额"
		if seconds, ok := numberFromOK(secondary, "limit_window_seconds", "limitWindowSeconds"); ok && seconds >= 28*24*3600 && seconds <= 31*24*3600 {
			secondaryID, secondaryKind, secondaryLabel = "monthly", "additional_monthly", name+" 月限额"
		}
		appendCodexWindow(&windows, fmt.Sprintf("%s-%s-%d", prefix, secondaryID, index), secondaryKind, secondaryLabel, secondary, info)
	}
	return windows
}

func parseCodexResetCredits(value any) (available, applicable *int, credits []map[string]any, valid bool) {
	root := mapValue(value)
	if root == nil {
		return nil, nil, nil, false
	}
	valid = false
	for _, key := range []string{"credits", "available_count", "availableCount", "applicable_available_count", "applicableAvailableCount"} {
		if _, exists := root[key]; exists {
			valid = true
		}
	}
	if value, ok := numberFromOK(root, "available_count", "availableCount"); ok {
		item := int(value)
		available = &item
	}
	if value, ok := numberFromOK(root, "applicable_available_count", "applicableAvailableCount"); ok {
		item := int(value)
		applicable = &item
	}
	for _, raw := range arrayValue(root["credits"]) {
		item := mapValue(raw)
		if item == nil || !strings.EqualFold(firstString(item, "reset_type", "resetType"), "codex_rate_limits") || !strings.EqualFold(firstString(item, "status"), "available") {
			continue
		}
		expiresAt := firstString(item, "expires_at", "expiresAt")
		if expiresAt == "" {
			continue
		}
		credits = append(credits, map[string]any{
			"id":        firstString(item, "id"),
			"status":    firstString(item, "status"),
			"grantedAt": firstString(item, "granted_at", "grantedAt"),
			"expiresAt": expiresAt,
		})
	}
	if available == nil && len(credits) > 0 {
		item := len(credits)
		available = &item
	}
	if applicable == nil && available != nil {
		item := *available
		applicable = &item
	}
	return available, applicable, credits, valid
}

func buildClaudeInspectionWindows(root map[string]any) []map[string]any {
	if root == nil {
		return nil
	}
	labels := map[string]string{
		"five_hour":            "5 小时限额",
		"seven_day":            "周限额",
		"seven_day_oauth_apps": "周限额（OAuth 应用）",
		"seven_day_opus":       "周限额（Opus）",
		"seven_day_sonnet":     "周限额（Sonnet）",
		"seven_day_cowork":     "周限额（Cowork）",
		"iguana_necktie":       "周限额（Fable）",
	}
	windows := []map[string]any{}
	fable := findClaudeFableLimit(arrayValue(root["limits"]))
	for _, key := range []string{"five_hour", "seven_day", "seven_day_oauth_apps", "seven_day_opus", "seven_day_sonnet", "seven_day_cowork", "iguana_necktie"} {
		if key == "iguana_necktie" && fable != nil {
			continue
		}
		window := mapValue(root[key])
		if window == nil {
			continue
		}
		used, ok := numberFromOK(window, "utilization")
		if !ok {
			continue
		}
		item := makeInspectionWindow("claude-"+strings.ReplaceAll(key, "_", "-"), key, labels[key], window, &used)
		item["periodHours"] = 5.0
		if key != "five_hour" {
			item["periodHours"] = 168.0
		}
		if resetAt := inspectionResetAt(window); resetAt != "" {
			item["resetAt"] = resetAt
		}
		windows = append(windows, item)
	}
	if fable != nil {
		used, ok := numberFromOK(fable, "percent")
		if ok {
			item := makeInspectionWindow("claude-seven-day-fable", "seven_day_fable", "周限额（Fable）", fable, &used)
			item["periodHours"] = 168.0
			windows = append(windows, item)
		}
	}
	return windows
}

func findClaudeFableLimit(limits []any) map[string]any {
	var fallback map[string]any
	for _, raw := range limits {
		item := mapValue(raw)
		if item == nil || !strings.EqualFold(firstString(item, "kind"), "weekly_scoped") {
			continue
		}
		scope := mapValue(item["scope"])
		model := mapValue(scope["model"])
		name := strings.ToLower(firstString(model, "display_name", "displayName", "id"))
		if name != "fable" && name != "fable 5" {
			continue
		}
		if _, ok := numberFromOK(item, "percent"); !ok {
			continue
		}
		if active, ok := boolFrom(item, "is_active", "isActive"); ok && active {
			return item
		}
		if fallback == nil {
			fallback = item
		}
	}
	return fallback
}

func resolveClaudePlan(profile map[string]any) string {
	if profile == nil {
		return ""
	}
	account := mapValue(profile["account"])
	if flag, ok := boolFrom(account, "has_claude_max", "hasClaudeMax"); ok && flag {
		return "plan_max"
	}
	if flag, ok := boolFrom(account, "has_claude_pro", "hasClaudePro"); ok && flag {
		return "plan_pro"
	}
	organization := mapValue(profile["organization"])
	if strings.EqualFold(firstString(organization, "organization_type", "organizationType"), "claude_team") && strings.EqualFold(firstString(organization, "subscription_status", "subscriptionStatus"), "active") {
		return "plan_team"
	}
	max, hasMax := boolFrom(account, "has_claude_max", "hasClaudeMax")
	pro, hasPro := boolFrom(account, "has_claude_pro", "hasClaudePro")
	if hasMax && hasPro && !max && !pro {
		return "plan_free"
	}
	return ""
}

func buildKimiInspectionWindows(root map[string]any) []map[string]any {
	if root == nil {
		return nil
	}
	windows := []map[string]any{}
	for index, raw := range arrayValue(root["limits"]) {
		item := mapValue(raw)
		if item == nil {
			continue
		}
		detail := mapValue(item["detail"])
		if detail == nil {
			detail = item
		}
		windowInfo := mapValue(item["window"])
		duration, hasDuration := numberFromOK(windowInfo, "duration")
		if !hasDuration {
			duration, hasDuration = numberFromOK(item, "duration")
		}
		if !hasDuration {
			duration, hasDuration = numberFromOK(detail, "duration")
		}
		unit := firstString(windowInfo, "timeUnit")
		if unit == "" {
			unit = firstString(item, "timeUnit")
		}
		if unit == "" {
			unit = firstString(detail, "timeUnit")
		}
		label := firstString(item, "name", "title", "scope")
		if label == "" {
			label = firstString(detail, "name", "title", "scope")
		}
		if label == "" && hasDuration && duration > 0 {
			label = fmt.Sprintf("窗口 %s", kimiDurationToken(duration, unit))
		}
		if label == "" {
			label = fmt.Sprintf("限额 %d", index+1)
		}
		windows = appendKimiWindow(windows, fmt.Sprintf("kimi-limit-%d", index), "limit", label, detail, duration, unit)
	}
	if usage := mapValue(root["usage"]); usage != nil {
		windows = appendKimiWindow(windows, "kimi-summary", "summary", "周限额", usage, 0, "")
	}
	return windows
}

func appendKimiWindow(windows []map[string]any, id, kind, label string, detail map[string]any, duration float64, unit string) []map[string]any {
	if detail == nil {
		return windows
	}
	limit, hasLimit := numberFromOK(detail, "limit")
	used, hasUsed := numberFromOK(detail, "used")
	remaining, hasRemaining := numberFromOK(detail, "remaining")
	if !hasUsed && hasRemaining && hasLimit {
		used = limit - remaining
		hasUsed = true
	}
	if !hasLimit && !hasUsed {
		return windows
	}
	item := map[string]any{"id": id, "kind": kind, "label": label, "used": maxFloat(used), "limit": maxFloat(limit)}
	if hasRemaining {
		item["remaining"] = maxFloat(remaining)
	} else if hasLimit {
		item["remaining"] = maxFloat(limit - used)
	}
	if hasLimit && limit > 0 && hasUsed {
		usedPercent := clampInspectionPercent(used / limit * 100)
		item["usedPercent"] = usedPercent
		item["remainingPercent"] = clampInspectionPercent(100 - usedPercent)
	}
	if resetAt := inspectionResetAt(detail); resetAt != "" {
		item["resetAt"] = resetAt
	}
	if duration > 0 {
		item["duration"] = duration
		item["timeUnit"] = unit
		item["periodHours"] = kimiDurationHours(duration, unit)
	}
	return append(windows, item)
}

func maxFloat(value float64) float64 {
	if value < 0 {
		return 0
	}
	return value
}

func normalizeKimiUnit(value string) string {
	unit := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(value), "TIME_UNIT_")))
	switch unit {
	case "seconds", "second":
		return "second"
	case "hours", "hour":
		return "hour"
	case "days", "day":
		return "day"
	case "weeks", "week":
		return "week"
	default:
		return "minute"
	}
}

func kimiDurationToken(duration float64, rawUnit string) string {
	value := int(duration)
	switch normalizeKimiUnit(rawUnit) {
	case "second":
		return fmt.Sprintf("%ds", value)
	case "hour":
		return fmt.Sprintf("%dh", value)
	case "day":
		return fmt.Sprintf("%dd", value)
	case "week":
		return fmt.Sprintf("%dw", value)
	default:
		if value%60 == 0 {
			return fmt.Sprintf("%dh", value/60)
		}
		return fmt.Sprintf("%dm", value)
	}
}

func kimiDurationHours(duration float64, rawUnit string) float64 {
	switch normalizeKimiUnit(rawUnit) {
	case "second":
		return duration / 3600
	case "hour":
		return duration
	case "day":
		return duration * 24
	case "week":
		return duration * 24 * 7
	default:
		return duration / 60
	}
}

func buildAntigravitySubscription(root map[string]any) map[string]any {
	if root == nil {
		return nil
	}
	current := mapValue(firstValue(root["currentTier"], root["current_tier"]))
	paid := mapValue(firstValue(root["paidTier"], root["paid_tier"]))
	effective := paid
	if firstString(effective, "id") == "" {
		effective = current
	}
	if effective == nil {
		return nil
	}
	id := firstString(effective, "id")
	name := firstString(effective, "name")
	plan := map[string]string{
		"free-tier":          "free",
		"g1-pro-tier":        "pro",
		"g1-ultra-tier":      "ultra",
		"g1-ultra-lite-tier": "ultra-lite",
	}[id]
	if plan == "" {
		plan = "unknown"
	}
	return map[string]any{"plan": plan, "tierId": id, "tierName": name}
}

func normalizeAntigravityFraction(value any) (float64, bool) {
	if number, ok := numberValue(value); ok {
		return number, true
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if strings.HasSuffix(text, "%") {
		parsed, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(text, "%")), 64)
		if err == nil {
			return parsed / 100, true
		}
	}
	return 0, false
}

func antigravityPeriodHours(window string) any {
	switch strings.ToLower(strings.TrimSpace(window)) {
	case "5h", "five-hour", "five_hour":
		return 5.0
	case "weekly", "week":
		return 168.0
	default:
		return nil
	}
}

func buildAntigravityQuota(root map[string]any) (groups []map[string]any, windows []map[string]any) {
	if root == nil {
		return nil, nil
	}
	for groupIndex, rawGroup := range arrayValue(root["groups"]) {
		group := mapValue(rawGroup)
		if group == nil {
			continue
		}
		label := firstString(group, "displayName", "display_name")
		if label == "" {
			label = fmt.Sprintf("Quota Group %d", groupIndex+1)
		}
		groupID := normalizeInspectionWindowID(label)
		if groupID == "" {
			groupID = fmt.Sprintf("quota-group-%d", groupIndex+1)
		}
		parsedGroup := map[string]any{"id": groupID, "label": label}
		if description := firstString(group, "description"); description != "" {
			parsedGroup["description"] = description
		}
		buckets := []map[string]any{}
		for bucketIndex, rawBucket := range arrayValue(group["buckets"]) {
			bucket := mapValue(rawBucket)
			if bucket == nil {
				continue
			}
			fraction, ok := normalizeAntigravityFraction(firstValue(bucket["remainingFraction"], bucket["remaining_fraction"]))
			if !ok {
				continue
			}
			bucketID := firstString(bucket, "bucketId", "bucket_id")
			if bucketID == "" {
				bucketID = fmt.Sprintf("%s-bucket-%d", groupID, bucketIndex+1)
			}
			bucketLabel := firstString(bucket, "displayName", "display_name")
			if bucketLabel == "" {
				bucketLabel = bucketID
			}
			parsedBucket := map[string]any{
				"id":                bucketID,
				"label":             bucketLabel,
				"window":            firstString(bucket, "window"),
				"remainingFraction": clampFraction(fraction),
				"remainingPercent":  clampInspectionPercent(clampFraction(fraction) * 100),
			}
			if reset := firstString(bucket, "resetTime", "reset_time"); reset != "" {
				parsedBucket["resetAt"] = formatInspectionInstant(reset)
			}
			if description := firstString(bucket, "description"); description != "" {
				parsedBucket["description"] = description
			}
			if period := antigravityPeriodHours(firstString(bucket, "window")); period != nil {
				parsedBucket["periodHours"] = period
			}
			buckets = append(buckets, parsedBucket)
			used := clampInspectionPercent((1 - clampFraction(fraction)) * 100)
			flat := map[string]any{
				"id":                bucketID,
				"kind":              "antigravity_bucket",
				"label":             bucketLabel,
				"usedPercent":       used,
				"remainingPercent":  clampInspectionPercent(clampFraction(fraction) * 100),
				"remainingFraction": clampFraction(fraction),
			}
			for _, key := range []string{"resetAt", "periodHours", "window", "description"} {
				if value, exists := parsedBucket[key]; exists {
					flat[key] = value
				}
			}
			windows = append(windows, flat)
		}
		if len(buckets) == 0 {
			continue
		}
		parsedGroup["buckets"] = buckets
		groups = append(groups, parsedGroup)
	}
	return groups, windows
}

func clampFraction(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func (r *Runtime) probeCodexQuota(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	document := r.inspectionAuthDocument(ctx, result.AuthIndex)
	planFromFile, subscriptionActiveUntil, accountID := resolveCodexDocumentValues(document)
	headers := map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "User-Agent": settings.UserAgent}
	if accountID != "" {
		headers["Chatgpt-Account-Id"] = accountID
	}
	response, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, codexUsageURL, headers, nil)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	result = resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, "codex")
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result
	}
	root := inspectBodyObject(response.Body)
	if plan := strings.ToLower(firstString(root, "plan_type", "planType")); plan != "" {
		result.PlanType = plan
	} else if planFromFile != "" {
		result.PlanType = planFromFile
	}
	windows := buildCodexInspectionWindows(root)
	metadata := map[string]any{"provider": "codex", "mode": "usage"}
	if subscriptionActiveUntil != nil {
		metadata["subscriptionActiveUntil"] = subscriptionActiveUntil
	}
	if result.PlanType != "" {
		metadata["planType"] = result.PlanType
	}
	resetHeaders := map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Accept":        "application/json",
		"OpenAI-Beta":   "codex-1",
		"Originator":    "Codex Desktop",
		"User-Agent":    settings.UserAgent,
	}
	if accountID != "" {
		resetHeaders["Chatgpt-Account-Id"] = accountID
	}
	resetResponse, resetErr := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, codexResetCreditsURL, resetHeaders, nil)
	if resetErr != nil {
		metadata["resetCreditsError"] = resetErr.Error()
	} else if resetResponse.StatusCode >= 200 && resetResponse.StatusCode < 300 {
		available, applicable, credits, valid := parseCodexResetCredits(resetResponse.Body)
		if !valid {
			metadata["resetCreditsError"] = "主动重置次数响应格式无法识别"
		} else {
			if available != nil {
				metadata["resetCreditsAvailableCount"] = *available
			}
			if applicable != nil {
				metadata["resetCreditsApplicableAvailableCount"] = *applicable
			}
			metadata["resetCredits"] = credits
		}
	} else {
		metadata["resetCreditsError"] = fmt.Sprintf("主动重置次数查询返回 HTTP %d", resetResponse.StatusCode)
	}
	result = quotaMetadata(result, metadata, windows)
	result.UsedPercent = maxWindowUsedPercent(windows)
	result = applyInspectionQuotaThreshold(result, settings.UsedPercentThreshold)
	if len(windows) > 0 || result.PlanType != "" || subscriptionActiveUntil != nil {
		result.ActionReason = "已读取 Codex 套餐与额度窗口"
		if result.ErrorKind == "" {
			result.ErrorKind = "healthy"
		}
	}
	return result
}

func (r *Runtime) probeClaudeQuota(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	headers := map[string]string{"Authorization": "Bearer $TOKEN$", "Content-Type": "application/json", "anthropic-beta": "oauth-2025-04-20"}
	usage, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, claudeUsageURL, headers, nil)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	result = resolveInspectionHTTPResult(result, usage, settings.UsedPercentThreshold, "claude")
	if usage.StatusCode < 200 || usage.StatusCode >= 300 {
		return result
	}
	root := inspectBodyObject(usage.Body)
	windows := buildClaudeInspectionWindows(root)
	metadata := map[string]any{"provider": "claude", "mode": "oauth_usage"}
	if extra := mapValue(root["extra_usage"]); extra != nil {
		metadata["extraUsage"] = extra
	}
	profile, profileErr := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, claudeProfileURL, headers, nil)
	if profileErr == nil && profile.StatusCode >= 200 && profile.StatusCode < 300 {
		if plan := resolveClaudePlan(inspectBodyObject(profile.Body)); plan != "" {
			result.PlanType = plan
			metadata["planType"] = plan
		}
	}
	result = quotaMetadata(result, metadata, windows)
	result.UsedPercent = maxWindowUsedPercent(windows)
	result = applyInspectionQuotaThreshold(result, settings.UsedPercentThreshold)
	if len(windows) > 0 || result.PlanType != "" {
		result.ActionReason = "已读取 Claude 套餐与额度窗口"
	} else {
		result.ActionReason = "Claude 身份探测正常；官方未返回可量化额度"
	}
	if result.ErrorKind == "" {
		result.ErrorKind = "healthy"
	}
	return result
}

func (r *Runtime) probeKimiQuota(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	response, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodGet, kimiUsageURL, map[string]string{"Authorization": "Bearer $TOKEN$", "Accept": "application/json"}, nil)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	result = resolveInspectionHTTPResult(result, response, settings.UsedPercentThreshold, "kimi")
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result
	}
	windows := buildKimiInspectionWindows(inspectBodyObject(response.Body))
	metadata := map[string]any{"provider": "kimi", "mode": "usage"}
	result = quotaMetadata(result, metadata, windows)
	result.UsedPercent = maxWindowUsedPercent(windows)
	result = applyInspectionQuotaThreshold(result, settings.UsedPercentThreshold)
	if len(windows) > 0 {
		result.ActionReason = "已读取 Kimi 额度窗口"
	} else {
		result.ActionReason = "Kimi 身份探测正常；官方未返回可量化额度"
	}
	if result.ErrorKind == "" {
		result.ErrorKind = "healthy"
	}
	return result
}

func (r *Runtime) probeAntigravityQuota(ctx context.Context, settings CodexInspectionSettings, result store.InspectionResult) store.InspectionResult {
	document, _ := r.inspectionAuthMetadata(ctx, result.AuthIndex)
	projectID := ""
	if result.AuthMetadata != nil {
		projectID = result.AuthMetadata.ProjectID
	}
	if projectID == "" {
		projectID = authDocumentString(document, "project_id", "projectId", "gemini_virtual_project")
	}
	loadBody := map[string]any{"metadata": map[string]any{"ideType": "ANTIGRAVITY"}}
	loadResponse, err := r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodPost, antigravityAssistURL, map[string]string{
		"Authorization": "Bearer $TOKEN$",
		"Accept":        "*/*",
		"Content-Type":  "application/json",
		"User-Agent":    "antigravity/cli/1.0.13 (aidev_client; os_type=darwin; arch=arm64)",
	}, loadBody)
	if err != nil {
		return inspectionFailure(result, 0, "upstream_error", err.Error())
	}
	result = resolveInspectionHTTPResult(result, loadResponse, settings.UsedPercentThreshold, "antigravity")
	if loadResponse.StatusCode < 200 || loadResponse.StatusCode >= 300 {
		return result
	}
	loadRoot := inspectBodyObject(loadResponse.Body)
	metadata := map[string]any{"provider": "antigravity", "mode": "quota_summary"}
	if subscription := buildAntigravitySubscription(loadRoot); subscription != nil {
		metadata["subscription"] = subscription
		result.PlanType = firstString(subscription, "plan", "tierName", "tierId")
	}
	if projectID == "" {
		metadata["quotaError"] = "缺少 project_id，无法查询 Antigravity 分组额度"
		result = quotaMetadata(result, metadata, nil)
		result.ActionReason = "Antigravity 身份探测正常；缺少 project_id，未查询额度"
		return result
	}
	var quotaResponse inspectionAPIResponse
	var quotaErr error
	lastStatus := 0
	for _, endpoint := range antigravityQuotaURLs {
		quotaResponse, quotaErr = r.callInspectionAPI(ctx, settings, result.AuthIndex, http.MethodPost, endpoint, map[string]string{
			"Authorization": "Bearer $TOKEN$",
			"Accept":        "*/*",
			"Content-Type":  "application/json",
			"User-Agent":    "antigravity/cli/1.0.13 (aidev_client; os_type=darwin; arch=arm64)",
		}, map[string]any{"project": projectID})
		if quotaErr == nil && quotaResponse.StatusCode >= 200 && quotaResponse.StatusCode < 300 {
			break
		}
		if quotaErr == nil {
			lastStatus = quotaResponse.StatusCode
		}
	}
	if quotaErr != nil || quotaResponse.StatusCode < 200 || quotaResponse.StatusCode >= 300 {
		if quotaErr != nil {
			metadata["quotaError"] = quotaErr.Error()
		} else {
			metadata["quotaError"] = fmt.Sprintf("额度查询返回 HTTP %d", lastStatus)
		}
		result = quotaMetadata(result, metadata, nil)
		result.ActionReason = "Antigravity 身份探测正常；额度查询失败"
		return result
	}
	groups, windows := buildAntigravityQuota(inspectBodyObject(quotaResponse.Body))
	metadata["projectId"] = projectID
	metadata["groups"] = groups
	result = quotaMetadata(result, metadata, windows)
	result.UsedPercent = maxWindowUsedPercent(windows)
	result = applyInspectionQuotaThreshold(result, settings.UsedPercentThreshold)
	if len(groups) > 0 {
		result.ActionReason = "已读取 Antigravity 套餐与分组额度"
	} else {
		result.ActionReason = "Antigravity 身份探测正常；官方未返回可量化额度"
	}
	if result.ErrorKind == "" {
		result.ErrorKind = "healthy"
	}
	return result
}
