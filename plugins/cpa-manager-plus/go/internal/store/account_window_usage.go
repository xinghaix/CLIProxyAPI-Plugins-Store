package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
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
	Source               string   `json:"source,omitempty"`
	ModelScopeModels     []string `json:"-"`
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
	prices, err := s.Prices(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]AccountWindowUsageItem, 0, len(windows))
	for _, window := range windows {
		item, err := s.accountWindowUsageOne(ctx, window, prices)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) accountWindowUsageOne(ctx context.Context, window AccountWindowUsageTarget, prices map[string]Price) (AccountWindowUsageItem, error) {
	window.RowKey = strings.TrimSpace(window.RowKey)
	window.ProviderWindowID = strings.TrimSpace(window.ProviderWindowID)
	if window.ProviderWindowID == "" {
		window.ProviderWindowID = strings.TrimSpace(window.WindowKey)
	}
	window.Period = normalizeAccountWindowPeriod(window.Period)
	window.RequestKey = strings.TrimSpace(window.RequestKey)
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

	rows, err := s.accountWindowEvents(ctx, window)
	if err != nil {
		return item, err
	}
	if len(rows) == 0 {
		return item, nil
	}
	total := stats{}
	for _, row := range rows {
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

func accountWindowHasCredentialIdentity(window AccountWindowUsageTarget) bool {
	authFile := strings.TrimSpace(window.AuthFileSnapshot)
	source := strings.TrimSpace(window.Source)
	account := strings.TrimSpace(window.AccountSnapshot)
	label := strings.TrimSpace(window.AuthLabelSnapshot)
	provider := strings.TrimSpace(window.AuthProviderSnapshot)
	if authFile != "" || (source != "" && source != account && source != label) {
		return provider != ""
	}
	if provider == "" {
		return false
	}
	return strings.TrimSpace(window.AuthIndex) != "" || account != "" || label != ""
}

func (s *Store) accountWindowEvents(ctx context.Context, window AccountWindowUsageTarget) ([]eventRow, error) {
	// Half-open [from, to) matching Plus window semantics. Narrow by the
	// strongest available identity in SQL, then confirm with Go matching.
	query := `select response_correlation_key,id,timestamp_ms,coalesce(provider,''),coalesce(executor_type,''),model,coalesce(alias,''),coalesce(response_model,''),coalesce(api_key_hash,''),coalesce(auth_id,''),coalesce(auth_index,''),coalesce(auth_type,''),coalesce(source,''),coalesce(reasoning_effort,''),coalesce(service_tier,''),input_tokens,output_tokens,reasoning_tokens,cached_tokens,cache_read_tokens,cache_creation_tokens,total_tokens,latency_ms,ttft_ms,failed,fail_status_code,fail_summary from usage_events where timestamp_ms >= ? and timestamp_ms < ?`
	args := []any{window.FromMS, window.ToMS}
	if authIndex := strings.TrimSpace(window.AuthIndex); authIndex != "" {
		query += ` and auth_index = ?`
		args = append(args, authIndex)
	} else if authFile := strings.TrimSpace(window.AuthFileSnapshot); authFile != "" {
		query += ` and (auth_id = ? or source = ?)`
		args = append(args, authFile, authFile)
	} else if source := strings.TrimSpace(window.Source); source != "" {
		query += ` and source = ?`
		args = append(args, source)
	} else if account := strings.TrimSpace(window.AccountSnapshot); account != "" {
		query += ` and (auth_index = ? or auth_id = ? or source = ?)`
		args = append(args, account, account, account)
	}
	query += ` order by timestamp_ms desc limit 20000`

	dbRows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()

	var candidates []eventRow
	for dbRows.Next() {
		var row eventRow
		var correlationKey sql.NullString
		if err := dbRows.Scan(&correlationKey, &row.ID, &row.TimestampMS, &row.Provider, &row.ExecutorType, &row.Model, &row.Alias, &row.ResponseModel, &row.APIKeyHash, &row.AuthID, &row.AuthIndex, &row.AuthType, &row.Source, &row.ReasoningEffort, &row.ServiceTier, &row.InputTokens, &row.OutputTokens, &row.ReasoningTokens, &row.CachedTokens, &row.CacheReadTokens, &row.CacheCreationTokens, &row.TotalTokens, &row.LatencyMS, &row.TTFTMS, &row.Failed, &row.FailStatus, &row.FailSummary); err != nil {
			return nil, err
		}
		if !accountWindowRowMatches(row, window) {
			continue
		}
		candidates = append(candidates, row)
	}
	return candidates, dbRows.Err()
}

func accountWindowRowMatches(row eventRow, window AccountWindowUsageTarget) bool {
	authIndex := strings.TrimSpace(window.AuthIndex)
	authFile := strings.TrimSpace(window.AuthFileSnapshot)
	source := strings.TrimSpace(window.Source)
	account := strings.TrimSpace(window.AccountSnapshot)
	provider := strings.ToLower(strings.TrimSpace(window.AuthProviderSnapshot))

	matchedIdentity := false
	if authIndex != "" && strings.TrimSpace(row.AuthIndex) == authIndex {
		matchedIdentity = true
	}
	if !matchedIdentity && authFile != "" && (strings.TrimSpace(row.AuthID) == authFile || strings.TrimSpace(row.Source) == authFile) {
		matchedIdentity = true
	}
	if !matchedIdentity && source != "" && strings.TrimSpace(row.Source) == source {
		matchedIdentity = true
	}
	if !matchedIdentity && account != "" && (strings.TrimSpace(row.AuthIndex) == account || strings.TrimSpace(row.AuthID) == account || strings.TrimSpace(row.Source) == account) {
		matchedIdentity = true
	}
	if !matchedIdentity {
		return false
	}
	if provider == "" {
		return true
	}
	rowProvider := strings.ToLower(strings.TrimSpace(row.Provider))
	return rowProvider == "" || rowProvider == provider
}

// AccountSparklineBuckets returns recent hourly request counts for an account
// identity, using the configured analytics timezone.
func (s *Store) AccountSparklineBuckets(ctx context.Context, authIndex, authID, source, provider string, fromMS, toMS int64, location *time.Location, bucketCount int) ([]map[string]any, error) {
	if bucketCount < 1 {
		bucketCount = 24
	}
	if location == nil {
		location = time.UTC
	}
	if toMS <= 0 {
		toMS = time.Now().UnixMilli()
	}
	if fromMS <= 0 || fromMS >= toMS {
		fromMS = toMS - int64(bucketCount)*int64(time.Hour/time.Millisecond)
	}
	window := AccountWindowUsageTarget{
		AuthIndex:            authIndex,
		AuthFileSnapshot:     authID,
		Source:               source,
		AuthProviderSnapshot: provider,
		FromMS:               fromMS,
		ToMS:                 toMS,
		AccountSnapshot:      authIndex,
		RowKey:               "spark",
		ProviderWindowID:     "spark",
		Period:               "current",
	}
	rows, err := s.accountWindowEvents(ctx, window)
	if err != nil {
		return nil, err
	}
	counts := map[int64]int64{}
	for _, row := range rows {
		bucket := AnalyticsBucketMS(row.TimestampMS, "hour", location)
		counts[bucket]++
	}
	out := make([]map[string]any, 0, bucketCount)
	start := AnalyticsBucketMS(fromMS, "hour", location)
	step := int64(time.Hour / time.Millisecond)
	for i := 0; i < bucketCount; i++ {
		bucket := start + int64(i)*step
		if bucket >= toMS {
			break
		}
		out = append(out, map[string]any{"bucket_ms": bucket, "calls": counts[bucket]})
	}
	return out, nil
}
