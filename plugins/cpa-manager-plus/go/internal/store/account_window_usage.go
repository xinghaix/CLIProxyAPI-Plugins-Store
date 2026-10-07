package store

import (
	"context"
	"fmt"
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
	rows, err := s.enrichedEvents(ctx, `timestamp_ms >= ? and timestamp_ms < ?`, from, to)
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
	if len(window.ModelScopeModels) == 0 && !AccountWindowIsAccountScope(window.AuthProviderSnapshot, window.ProviderWindowID) {
		item.ScopeMatchStatus = "unknown"
		item.SyncStatus = "unknown"
		item.CostComplete = false
		return item, nil
	}
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
	if total.Calls == 0 {
		return item, nil
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

// AccountWindowIsAccountScope is shared by quota producers and usage queries.
// New/product/family windows remain unknown until exact model IDs are supplied.
func AccountWindowIsAccountScope(provider, id string) bool {
	if id == "history" || id == "spark" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex":
		return id == "five-hour" || id == "five_hour" || id == "weekly" || id == "monthly"
	case "claude":
		return id == "claude-five-hour" || id == "claude-seven-day"
	case "xai":
		return id == "xai-weekly" || id == "xai-monthly"
	case "kimi":
		return id == "kimi-summary"
	}
	return false
}

func accountWindowHasCredentialIdentity(window AccountWindowUsageTarget) bool {
	return strings.TrimSpace(window.AuthProviderSnapshot) != "" &&
		(strings.TrimSpace(window.AuthIndex) != "" || strings.TrimSpace(window.AuthID) != "" ||
			strings.TrimSpace(window.AuthFileSnapshot) != "" || strings.TrimSpace(window.Source) != "")
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
	return (authID != "" && authID == strings.TrimSpace(row.AuthID)) ||
		(authFile != "" && (authFile == strings.TrimSpace(row.AuthID) || authFile == strings.TrimSpace(row.Source))) ||
		(source != "" && source == strings.TrimSpace(row.Source))
}
