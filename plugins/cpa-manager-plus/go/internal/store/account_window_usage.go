package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// AccountWindowUsageTarget describes one previous/current window query.
// Field names mirror CPA-Manager-Plus monitoring account-window-usage.
type AccountWindowUsageTarget struct {
	RequestKey           string   `json:"request_key,omitempty"`
	RowKey               string   `json:"row_key"`
	WindowKey            string   `json:"window_key,omitempty"`
	ProviderWindowID     string   `json:"provider_window_id,omitempty"`
	Period               string   `json:"period,omitempty"`
	FromMS               int64    `json:"from_ms"`
	ToMS                 int64    `json:"to_ms"`
	AccountSnapshot      string   `json:"account_snapshot,omitempty"`
	AuthLabelSnapshot    string   `json:"auth_label_snapshot,omitempty"`
	AuthFileSnapshot     string   `json:"auth_file_snapshot,omitempty"`
	AuthProviderSnapshot string   `json:"auth_provider_snapshot,omitempty"`
	AuthIndex            string   `json:"auth_index,omitempty"`
	AuthID               string   `json:"auth_id,omitempty"`
	Source               string   `json:"source,omitempty"`
	ModelScopeModels     []string `json:"model_scope_models,omitempty"`
}

// AccountWindowUsageItem is one aggregated window result.
type AccountWindowUsageItem struct {
	RequestKey        string   `json:"request_key"`
	RowKey            string   `json:"row_key"`
	WindowKey         string   `json:"window_key,omitempty"`
	ProviderWindowID  string   `json:"provider_window_id"`
	Period            string   `json:"period"`
	FromMS            int64    `json:"from_ms"`
	ToMS              int64    `json:"to_ms"`
	Matched           bool     `json:"matched"`
	TotalRequests     int64    `json:"total_requests"`
	SuccessCalls      int64    `json:"success_calls"`
	FailureCalls      int64    `json:"failure_calls"`
	TotalTokens       int64    `json:"total_tokens"`
	TotalCost         float64  `json:"total_cost"`
	SuccessRate       *float64 `json:"success_rate"`
	LastSeenMS        *int64   `json:"last_seen_ms"`
	SyncStatus        string   `json:"sync_status"`
	ScopeMatchStatus  string   `json:"scope_match_status"`
	UnmatchedRequests int64    `json:"unmatched_requests"`
	PricedCalls       int64    `json:"priced_calls,omitempty"`
	UnpricedCalls     int64    `json:"unpriced_calls,omitempty"`
	CostComplete      bool     `json:"cost_complete"`
}

const maxAccountWindowUsageItems = 200

// AccountWindowUsage aggregates usage_events for each target window using the
// same OpenAI API-equivalent estimator as monitoring analytics.
func (s *Store) AccountWindowUsage(ctx context.Context, windows []AccountWindowUsageTarget) ([]AccountWindowUsageItem, error) {
	if len(windows) == 0 {
		return nil, fmt.Errorf("windows are required")
	}
	if len(windows) > maxAccountWindowUsageItems {
		return nil, fmt.Errorf("windows must be less than or equal to %d", maxAccountWindowUsageItems)
	}
	from, to := windows[0].FromMS, windows[0].ToMS
	for _, window := range windows {
		if window.FromMS <= 0 || window.ToMS <= window.FromMS {
			return nil, fmt.Errorf("from_ms and to_ms are required and from_ms must be less than to_ms")
		}
		if window.FromMS < from {
			from = window.FromMS
		}
		to = max64(to, window.ToMS)
	}
	// Load pricing context once for the batch, never run full Analytics per window.
	rows, err := s.enrichedPricingEvents(ctx, `timestamp_ms >= ? and timestamp_ms < ?`, from, to)
	if err != nil {
		return nil, err
	}
	prices, err := s.Prices(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AccountWindowUsageItem, 0, len(windows))
	for _, window := range windows {
		item, err := accountWindowUsageOne(window, prices, rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func accountWindowUsageOne(window AccountWindowUsageTarget, prices map[string]Price, rows []eventRow) (AccountWindowUsageItem, error) {
	window.RowKey = strings.TrimSpace(window.RowKey)
	window.ProviderWindowID = strings.TrimSpace(window.ProviderWindowID)
	if window.ProviderWindowID == "" {
		window.ProviderWindowID = strings.TrimSpace(window.WindowKey)
	}
	window.Period = normalizeAccountWindowPeriod(window.Period)
	if window.RequestKey == "" {
		window.RequestKey = strings.Join([]string{window.RowKey, window.ProviderWindowID, window.Period}, "\x00")
	}
	item := AccountWindowUsageItem{
		RequestKey:       window.RequestKey,
		RowKey:           window.RowKey,
		WindowKey:        window.WindowKey,
		ProviderWindowID: window.ProviderWindowID,
		Period:           window.Period,
		FromMS:           window.FromMS,
		ToMS:             window.ToMS,
		SyncStatus:       "empty",
		ScopeMatchStatus: "complete",
		CostComplete:     true,
	}
	if window.RowKey == "" {
		return item, fmt.Errorf("row_key is required")
	}
	if window.ProviderWindowID == "" {
		return item, fmt.Errorf("provider_window_id is required")
	}
	if window.Period == "" {
		return item, fmt.Errorf("period must be current, previous, or previous_equal_range")
	}
	if window.FromMS <= 0 || window.ToMS <= 0 || window.FromMS >= window.ToMS {
		return item, fmt.Errorf("from_ms and to_ms are required and from_ms must be less than to_ms")
	}
	if !accountWindowHasCredentialIdentity(window) {
		return item, fmt.Errorf("account target credential identity is required")
	}

	for _, model := range window.ModelScopeModels {
		if model == "" || strings.TrimSpace(model) != model {
			return item, fmt.Errorf("model_scope_models must contain nonempty exact model IDs without surrounding whitespace")
		}
	}
	// 未指定精确子模型列表时，配额窗口流量默认直接跟随凭证本身统计。
	total := stats{}
	// ponytail: O(events * windows), capped at 200 windows; index in memory
	// by credential if large batches become costly. Aggregate inputs are uncapped.
	var candidates []eventRow
	for _, row := range rows {
		if row.TimestampMS >= window.FromMS && row.TimestampMS < window.ToMS {
			candidates = append(candidates, row)
		}
	}
	providerLookup := providerSnapshots(candidates)
	for _, row := range candidates {
		row.Provider = resolvedProvider(row, providerLookup)
		if !accountWindowRowMatches(row, window) || !includes(window.ModelScopeModels, row.Model) {
			continue
		}
		total.add(row, prices[row.Model])
	}
	item.Matched = true
	item.TotalRequests = total.Calls
	item.SuccessCalls = total.Success
	item.FailureCalls = total.Failure
	item.TotalTokens = total.Tokens
	item.TotalCost = total.Cost
	item.PricedCalls = total.PricedCalls
	item.UnpricedCalls = total.UnpricedCalls
	item.CostComplete = total.UnpricedCalls == 0
	item.SyncStatus = "ready"
	if total.Calls > 0 {
		rate := float64(total.Success) / float64(total.Calls)
		item.SuccessRate = &rate
	}
	if total.Last > 0 {
		last := total.Last
		item.LastSeenMS = &last
	}
	return item, nil
}

func normalizeAccountWindowPeriod(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "current":
		return "current"
	case "previous":
		return "previous"
	case "previous_equal_range":
		return "previous_equal_range"
	default:
		return ""
	}
}

// AccountWindowIsAccountScope 判断配额窗口是否按凭证维度统计流量。
// 只要属于凭证的窗口，默认均跟随该凭证的全部流量请求，不再受硬编码窗口 ID 白名单限制。
func AccountWindowIsAccountScope(provider, id string) bool {
	return strings.TrimSpace(provider) != "" || strings.TrimSpace(id) != ""
}

func normalizeWindowScopeSlug(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// InferStandardWindowPeriodHours infers standard periodHours (5 / 24 / 168 / 720)
// from window metadata when upstream responses or cached snapshots omit it.
func InferStandardWindowPeriodHours(values ...string) (float64, bool) {
	for _, raw := range values {
		slug := normalizeWindowScopeSlug(raw)
		if slug == "" {
			continue
		}
		switch {
		case slug == "5h" || strings.Contains(slug, "five-hour") || strings.Contains(slug, "5-hour"):
			return 5, true
		case slug == "weekly" || slug == "week" || strings.Contains(slug, "weekly") || strings.Contains(slug, "seven-day") || strings.Contains(slug, "7-day"):
			return 168, true
		case slug == "daily" || slug == "day" || strings.Contains(slug, "daily"):
			return 24, true
		case slug == "monthly" || slug == "month" || strings.Contains(slug, "monthly"):
			return 720, true
		}
	}
	return 0, false
}

// NormalizeQuotaWindows enriches cached or cross-module quota windows with
// modelScope and periodHours so consumers work even when another module wrote
// an older snapshot format. Non-map elements are preserved verbatim.
func NormalizeQuotaWindows(provider string, raw any) any {
	switch typed := raw.(type) {
	case []map[string]any:
		out := make([]map[string]any, len(typed))
		for i, w := range typed {
			out[i] = normalizeOneQuotaWindowMap(provider, w)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			if m, ok := item.(map[string]any); ok && m != nil {
				out[i] = normalizeOneQuotaWindowMap(provider, m)
			} else {
				out[i] = item
			}
		}
		return out
	default:
		return raw
	}
}

func normalizeOneQuotaWindowMap(provider string, window map[string]any) map[string]any {
	if window == nil {
		return nil
	}
	cloned := make(map[string]any, len(window)+2)
	for k, v := range window {
		cloned[k] = v
	}
	id := strings.TrimSpace(fmt.Sprint(cloned["id"]))
	if id == "<nil>" {
		id = ""
	}
	label := strings.TrimSpace(fmt.Sprint(cloned["label"]))
	if label == "<nil>" {
		label = ""
	}
	kind := strings.TrimSpace(fmt.Sprint(cloned["kind"]))
	if kind == "<nil>" {
		kind = ""
	}
	winText := strings.TrimSpace(fmt.Sprint(cloned["window"]))
	if winText == "<nil>" {
		winText = ""
	}
	scope := strings.TrimSpace(fmt.Sprint(cloned["modelScope"]))
	if scope == "<nil>" {
		scope = ""
	}
	if scope == "" || scope == "unknown" {
		cloned["modelScope"] = "account"
	}
	if _, hasPeriod := cloned["periodHours"]; !hasPeriod {
		if _, hasSnake := cloned["period_hours"]; !hasSnake {
			if hours, ok := InferStandardWindowPeriodHours(winText, id, label, kind); ok {
				cloned["periodHours"] = hours
			}
		}
	}
	return cloned
}

func accountWindowHasCredentialIdentity(window AccountWindowUsageTarget) bool {
	return strings.TrimSpace(window.AuthProviderSnapshot) != "" &&
		(strings.TrimSpace(window.AuthIndex) != "" || strings.TrimSpace(window.AuthID) != "" ||
			strings.TrimSpace(window.AuthFileSnapshot) != "" || strings.TrimSpace(window.Source) != "")
}

func matchExactOrBaseName(expected, candidate string) bool {
	expected = strings.TrimSpace(expected)
	candidate = strings.TrimSpace(candidate)
	if expected == "" || candidate == "" {
		return false
	}
	if expected == candidate {
		return true
	}
	expBase := filepath.Base(expected)
	candBase := filepath.Base(candidate)
	if expBase == "." || expBase == "/" || candBase == "." || candBase == "/" {
		return false
	}
	return expBase == candBase
}

func accountWindowRowMatches(row eventRow, window AccountWindowUsageTarget) bool {
	provider := strings.TrimSpace(window.AuthProviderSnapshot)
	rowProvider := strings.TrimSpace(row.Provider)
	if provider != "" && rowProvider != "" && !strings.EqualFold(provider, rowProvider) {
		return false
	}
	authIndex, rowIndex := strings.TrimSpace(window.AuthIndex), strings.TrimSpace(row.AuthIndex)
	// A different strong index must never fall through to shared filenames.
	if authIndex != "" && rowIndex != "" {
		return authIndex == rowIndex
	}
	authID := strings.TrimSpace(window.AuthID)
	authFile := strings.TrimSpace(window.AuthFileSnapshot)
	source := strings.TrimSpace(window.Source)
	rowID := strings.TrimSpace(row.AuthID)
	rowSource := strings.TrimSpace(row.Source)
	return (authID != "" && authID == rowID) ||
		(authFile != "" && (matchExactOrBaseName(authFile, rowID) || matchExactOrBaseName(authFile, rowSource))) ||
		(source != "" && matchExactOrBaseName(source, rowSource))
}
