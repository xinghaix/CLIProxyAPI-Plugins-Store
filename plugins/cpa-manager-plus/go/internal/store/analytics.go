package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"
)

type Price struct {
	Prompt        float64 `json:"prompt"`
	Completion    float64 `json:"completion"`
	Cache         float64 `json:"cache"`
	CacheRead     float64 `json:"cacheRead"`
	CacheCreation float64 `json:"cacheCreation"`
	Source        string  `json:"source,omitempty"`
	SourceModelID string  `json:"sourceModelId,omitempty"`
	SyncedAtMS    int64   `json:"syncedAtMs,omitempty"`
	UpdatedAtMS   int64   `json:"updatedAtMs,omitempty"`
}

type AnalyticsRequest struct {
	FromMS            int64
	ToMS              int64
	Limit             int
	Models            []string
	Providers         []string
	Accounts          []string
	AuthFiles         []string
	MinLatencyMS      int64
	CacheStatus       string
	APIKeyHashes      []string
	FailedOnly        bool
	IncludeFailed     bool
	Search            string
	Granularity       string
	DrilldownBucketMS int64
	DrilldownLimit    int
	TimeZone          string
	Location          *time.Location
}

type eventRow struct {
	EventHash                                                                                                                          string
	ResponseCorrelationKey                                                                                                             string
	ResponseHeadersJSON                                                                                                                string
	ObservedResponseModel                                                                                                              string
	ResponseObservationPresent, ResponseObservationAmbiguous, ResponseTierAmbiguous                                                    bool
	ObservedServiceTier                                                                                                                string
	ResponseUsageCount                                                                                                                 int64
	ID                                                                                                                                 int64
	TimestampMS                                                                                                                        int64
	Provider, ExecutorType, Model, Alias, ResponseModel, APIKeyHash, AuthID, AuthIndex, AuthType, Source, ReasoningEffort, ServiceTier string
	InputTokens, OutputTokens, ReasoningTokens, CachedTokens, CacheReadTokens, CacheCreationTokens, TotalTokens                        int64
	LatencyMS, TTFTMS                                                                                                                  sql.NullInt64
	Failed                                                                                                                             int
	FailStatus                                                                                                                         sql.NullInt64
	FailSummary                                                                                                                        sql.NullString
}

func (s *Store) Prices(ctx context.Context) (map[string]Price, error) {
	rows, err := s.db.QueryContext(ctx, `select model, prompt, completion, cache, cache_read, cache_creation, coalesce(source, ''), coalesce(source_model_id, ''), coalesce(synced_at_ms, 0), updated_at_ms from model_prices order by model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prices := map[string]Price{}
	for rows.Next() {
		var model string
		var price Price
		if err := rows.Scan(&model, &price.Prompt, &price.Completion, &price.Cache, &price.CacheRead, &price.CacheCreation, &price.Source, &price.SourceModelID, &price.SyncedAtMS, &price.UpdatedAtMS); err != nil {
			return nil, err
		}
		prices[model] = price
	}
	return prices, rows.Err()
}

func (s *Store) ReplacePrices(ctx context.Context, prices map[string]Price) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `insert into model_prices(model,prompt,completion,cache,cache_read,cache_creation,source,source_model_id,synced_at_ms,updated_at_ms) values(?,?,?,?,?,?,?,?,?,?) on conflict(model) do update set prompt=excluded.prompt,completion=excluded.completion,cache=excluded.cache,cache_read=excluded.cache_read,cache_creation=excluded.cache_creation,source=excluded.source,source_model_id=excluded.source_model_id,synced_at_ms=excluded.synced_at_ms,updated_at_ms=excluded.updated_at_ms`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	now := time.Now().UnixMilli()
	for model, price := range prices {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 256 || !finiteNonNegative(price.Prompt, price.Completion, price.Cache, price.CacheRead, price.CacheCreation) {
			return fmt.Errorf("invalid model price %q", model)
		}
		price.Source = strings.TrimSpace(price.Source)
		if price.Source == "" {
			price.Source = "manual"
		}
		if _, err := stmt.ExecContext(ctx, model, price.Prompt, price.Completion, price.Cache, price.CacheRead, price.CacheCreation, price.Source, strings.TrimSpace(price.SourceModelID), nullableSyncedAt(price.SyncedAtMS), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeletePrice removes one model price. It is idempotent: deleted is false when no row exists.
func (s *Store) DeletePrice(ctx context.Context, model string) (deleted bool, err error) {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 256 {
		return false, fmt.Errorf("invalid model %q", model)
	}
	result, err := s.db.ExecContext(ctx, `delete from model_prices where model = ?`, model)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func nullableSyncedAt(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func finiteNonNegative(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return false
		}
	}
	return true
}

const maxEventWindow = 3_000

func clampEventLimit(limit int) int {
	if limit < 1 || limit > maxEventWindow {
		return maxEventWindow
	}
	return limit
}

func (s *Store) Analytics(ctx context.Context, request AnalyticsRequest) (map[string]any, error) {
	if request.ToMS <= 0 {
		request.ToMS = time.Now().UnixMilli()
	}
	if request.FromMS < 0 || request.FromMS >= request.ToMS {
		return nil, fmt.Errorf("invalid time range")
	}
	request.Limit = clampEventLimit(request.Limit)
	rows, windowRows, err := s.eventsWithWindow(ctx, request)
	if err != nil {
		return nil, err
	}
	prices, err := s.Prices(ctx)
	if err != nil {
		return nil, err
	}
	return aggregateWithWindow(rows, windowRows, prices, request), nil
}

func (s *Store) events(ctx context.Context, request AnalyticsRequest) ([]eventRow, error) {
	rows, _, err := s.eventsWithWindow(ctx, request)
	return rows, err
}

func (s *Store) eventsWithWindow(ctx context.Context, request AnalyticsRequest) ([]eventRow, []eventRow, error) {
	where := `timestamp_ms >= ? and timestamp_ms <= ?`
	rows, err := s.enrichedEvents(ctx, where, request.FromMS, request.ToMS)
	if err != nil {
		return nil, nil, err
	}
	providerLookup := providerSnapshots(rows)
	for i := range rows {
		rows[i].Provider = resolvedProvider(rows[i], providerLookup)
	}
	results := make([]eventRow, 0, len(rows))
	for _, row := range rows {
		if matchesStatus(row, request) && matches(row, request) && matchesSearch(row, request.Search) {
			results = append(results, row)
		}
	}
	return results, rows, nil
}

func matchesStatus(row eventRow, request AnalyticsRequest) bool {
	if request.FailedOnly {
		return row.Failed != 0
	}
	if !request.IncludeFailed {
		return row.Failed == 0
	}
	return true
}

// enrichedEvents is the common pricing-context loader. Callers supply only
// internal SQL predicates and bound values. Provider stays raw: callers backfill
// within their own range before identity/model filtering.
func (s *Store) enrichedEvents(ctx context.Context, where string, args ...any) ([]eventRow, error) {
	query := `select response_correlation_key,coalesce(event_hash,''),id,timestamp_ms,coalesce(provider,''),coalesce(executor_type,''),model,coalesce(alias,''),coalesce(response_model,''),coalesce(api_key_hash,''),coalesce(auth_id,''),coalesce(auth_index,''),coalesce(auth_type,''),coalesce(source,''),coalesce(reasoning_effort,''),coalesce(service_tier,''),input_tokens,output_tokens,reasoning_tokens,cached_tokens,cache_read_tokens,cache_creation_tokens,total_tokens,latency_ms,ttft_ms,failed,fail_status_code,fail_summary,coalesce(response_headers_json,'') from usage_events where ` + where
	// ponytail: O(N) candidate memory for this range; streaming with a provider
	// evidence prepass is the upgrade path. Never limit aggregate input rows.
	// Count matches globally, not only inside the selected time/filter window.
	// The correlation index restricts grouping to keys in this time window.
	query = `with selected as (` + query + `), usage_matches as (
	 select response_correlation_key, count(*) as usage_count from usage_events
	 where response_correlation_key is not null and response_correlation_key <> '' and response_correlation_key in (select response_correlation_key from selected where response_correlation_key is not null and response_correlation_key <> '')
	 group by response_correlation_key
	) select selected.*, coalesce(o.model,''), o.correlation_key is not null,
	 coalesce(o.ambiguous,0), coalesce(o.service_tier,''), coalesce(o.service_tier_ambiguous,0), coalesce(m.usage_count,0)
	 from selected left join response_observations o on o.correlation_key=selected.response_correlation_key
	 left join usage_matches m on m.response_correlation_key=selected.response_correlation_key
	 order by selected.timestamp_ms desc, selected.id desc`
	dbRows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()
	var candidates []eventRow
	for dbRows.Next() {
		var row eventRow
		var correlationKey sql.NullString
		if err := dbRows.Scan(&correlationKey, &row.EventHash, &row.ID, &row.TimestampMS, &row.Provider, &row.ExecutorType, &row.Model, &row.Alias, &row.ResponseModel, &row.APIKeyHash, &row.AuthID, &row.AuthIndex, &row.AuthType, &row.Source, &row.ReasoningEffort, &row.ServiceTier, &row.InputTokens, &row.OutputTokens, &row.ReasoningTokens, &row.CachedTokens, &row.CacheReadTokens, &row.CacheCreationTokens, &row.TotalTokens, &row.LatencyMS, &row.TTFTMS, &row.Failed, &row.FailStatus, &row.FailSummary, &row.ResponseHeadersJSON, &row.ObservedResponseModel, &row.ResponseObservationPresent, &row.ResponseObservationAmbiguous, &row.ObservedServiceTier, &row.ResponseTierAmbiguous, &row.ResponseUsageCount); err != nil {
			return nil, err
		}
		row.ResponseCorrelationKey = correlationKey.String
		candidates = append(candidates, row)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}
	for i := range candidates {
		if s.responseObservationsSuppressed.Load() {
			candidates[i].ResponseObservationAmbiguous = true
		}
	}
	return candidates, nil
}

func matches(row eventRow, request AnalyticsRequest) bool {
	if !matchesAuthFile(request.AuthFiles, row) {
		return false
	}
	if request.MinLatencyMS > 0 && (!row.LatencyMS.Valid || row.LatencyMS.Int64 < request.MinLatencyMS) {
		return false
	}
	cached := max64(row.CachedTokens, row.CacheReadTokens)
	if pricing.IndependentCacheBuckets(pricing.Usage{Provider: row.Provider, ExecutorType: row.ExecutorType}) {
		cached = row.CacheReadTokens
	}
	if (request.CacheStatus == "hit" && cached <= 0) || (request.CacheStatus == "miss" && cached != 0) {
		return false
	}
	return includesModel(request.Models, row) && includesFold(request.Providers, row.Provider) && matchesAccount(request.Accounts, row) && includes(request.APIKeyHashes, apiKeySnapshot(row))
}

func matchesAuthFile(values []string, row eventRow) bool {
	if len(values) == 0 {
		return true
	}
	if includes(values, row.AuthID) || includes(values, accountSnapshot(row)) || includes(values, sourceSnapshot(row)) {
		return true
	}
	if trimmed := strings.TrimSpace(row.AuthID); trimmed != "" && includes(values, filepath.Base(trimmed)) {
		return true
	}
	return false
}

func matchesAccount(values []string, row eventRow) bool {
	if len(values) == 0 {
		return true
	}
	return includes(values, accountSnapshot(row)) ||
		(row.AuthIndex != "" && includes(values, row.AuthIndex)) ||
		(row.AuthID != "" && includes(values, row.AuthID)) ||
		includes(values, sourceSnapshot(row)) ||
		includes(values, accountAPIKeyIdentity(row))
}

func matchesSearch(row eventRow, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	for _, value := range []string{row.Model, row.Alias, row.Provider, row.AuthIndex, row.AuthID, row.APIKeyHash, row.Source, row.EventHash, row.FailSummary.String} {
		if strings.Contains(strings.ToLower(value), needle) {
			return true
		}
	}
	return false
}

func includesModel(values []string, row eventRow) bool {
	if len(values) == 0 {
		return true
	}
	return includes(values, row.Model) || includes(values, strings.TrimSpace(row.Alias))
}

func accountSnapshot(row eventRow) string {
	if row.AuthIndex != "" {
		return row.AuthIndex
	}
	if row.AuthID != "" {
		return row.AuthID
	}
	return "unknown"
}

func apiKeySnapshot(row eventRow) string {
	if row.APIKeyHash == "" {
		return "unknown"
	}
	return row.APIKeyHash
}

func sourceSnapshot(row eventRow) string {
	if source := strings.TrimSpace(row.Source); source != "" {
		return source
	}
	return "unknown"
}

type providerSnapshot struct {
	Provider    string
	TimestampMS int64
	ID          int64
}

func providerSnapshots(rows []eventRow) map[string][]providerSnapshot {
	lookup := map[string][]providerSnapshot{}
	for _, row := range rows {
		provider := normalizeProvider(row.Provider)
		if provider == "" {
			continue
		}
		snapshot := providerSnapshot{Provider: provider, TimestampMS: row.TimestampMS, ID: row.ID}
		for _, key := range providerLookupKeys(row) {
			lookup[key] = append(lookup[key], snapshot)
		}
	}
	return lookup
}

func resolvedProvider(row eventRow, lookup map[string][]providerSnapshot) string {
	if provider := normalizeProvider(row.Provider); provider != "" {
		return provider
	}
	for _, key := range providerLookupKeys(row) {
		if provider, ok := unambiguousProvider(lookup[key]); ok {
			return provider
		}
	}
	return ""
}

func unambiguousProvider(snapshots []providerSnapshot) (string, bool) {
	if len(snapshots) == 0 {
		return "", false
	}
	provider := snapshots[0].Provider
	for _, snapshot := range snapshots[1:] {
		if snapshot.Provider != provider {
			return "", false
		}
	}
	return provider, true
}

func providerLookupKeys(row eventRow) []string {
	keys := make([]string, 0, 4)
	add := func(prefix, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := prefix + value
		for _, existing := range keys {
			if existing == key {
				return
			}
		}
		keys = append(keys, key)
	}
	// Auth identity is more specific than the display/source value. This keeps
	// a shared API key from borrowing a different auth entry's provider.
	add("auth-index:", row.AuthIndex)
	add("auth-id:", row.AuthID)
	add("source:", row.Source)
	add("api-key:", row.APIKeyHash)
	return keys
}

func normalizeProvider(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "—" || value == "-" {
		return ""
	}
	return value
}

func includes(values []string, got string) bool {
	if len(values) == 0 {
		return true
	}
	for _, value := range values {
		if value == got {
			return true
		}
	}
	return false
}

func includesFold(values []string, got string) bool {
	if len(values) == 0 {
		return true
	}
	trimmedGot := strings.TrimSpace(got)
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), trimmedGot) {
			return true
		}
	}
	return false
}

type stats struct {
	Calls, Success, Failure, Input, Output, Reasoning, Cached, CacheRead, CacheCreation, CacheTokens, Tokens int64
	CacheHitTokens, CacheHitInputTokens                                                                      int64
	Latency                                                                                                  int64
	LatencySamples                                                                                           int64
	Cost                                                                                                     float64
	PricedCalls, UnpricedCalls                                                                               int64
	Last                                                                                                     int64
	Provider, AccountSnapshot, Source, AuthIndex, AuthID, AuthType                                           string
}

type accountAPIKeyStats struct {
	stats
	Source, APIKey, Provider, AuthType, AuthID, AuthIndex string
}

func rowCacheTokens(row eventRow) int64 {
	return max64(max64(row.CachedTokens, 0), max64(row.CacheReadTokens, 0)+max64(row.CacheCreationTokens, 0))
}

func (s *stats) add(row eventRow, price Price) {
	s.Calls++
	if row.Failed != 0 {
		s.Failure++
	} else {
		s.Success++
	}
	s.Input += row.InputTokens
	s.Output += row.OutputTokens
	s.Reasoning += row.ReasoningTokens
	s.Cached += row.CachedTokens
	s.CacheRead += row.CacheReadTokens
	s.CacheCreation += row.CacheCreationTokens
	s.CacheTokens += rowCacheTokens(row)
	hitTokens, inputTokens := cacheHitTotals(row)
	s.CacheHitTokens += hitTokens
	s.CacheHitInputTokens += inputTokens
	s.Tokens += row.TotalTokens
	if row.LatencyMS.Valid {
		s.Latency += row.LatencyMS.Int64
		s.LatencySamples++
	}
	s.addEstimate(estimateEventCost(row, price))
	if row.TimestampMS >= s.Last {
		s.Last = row.TimestampMS
		if row.Provider != "" {
			s.Provider = row.Provider
		}
		if acct := accountSnapshot(row); acct != "" && acct != "unknown" {
			s.AccountSnapshot = acct
		}
		if src := strings.TrimSpace(row.Source); src != "" && src != "unknown" {
			s.Source = src
		}
		if row.AuthIndex != "" {
			s.AuthIndex = row.AuthIndex
		}
		if row.AuthID != "" {
			s.AuthID = row.AuthID
		}
		if row.AuthType != "" {
			s.AuthType = normalizeAuthTypeSnapshot(row.AuthType)
		}
	} else {
		if s.Provider == "" && row.Provider != "" {
			s.Provider = row.Provider
		}
		if s.AccountSnapshot == "" {
			if acct := accountSnapshot(row); acct != "" && acct != "unknown" {
				s.AccountSnapshot = acct
			}
		}
		if s.Source == "" {
			if src := strings.TrimSpace(row.Source); src != "" && src != "unknown" {
				s.Source = src
			}
		}
		if s.AuthIndex == "" && row.AuthIndex != "" {
			s.AuthIndex = row.AuthIndex
		}
		if s.AuthID == "" && row.AuthID != "" {
			s.AuthID = row.AuthID
		}
		if s.AuthType == "" && row.AuthType != "" {
			s.AuthType = normalizeAuthTypeSnapshot(row.AuthType)
		}
	}
}

func (s stats) json() map[string]any {
	rate := float64(0)
	avgCost := float64(0)
	if s.Calls > 0 {
		rate = float64(s.Success) / float64(s.Calls)
		avgCost = s.Cost / float64(s.Calls)
	}
	avg := float64(0)
	if s.LatencySamples > 0 {
		avg = float64(s.Latency) / float64(s.LatencySamples)
	}
	return map[string]any{
		"calls":                 s.Calls,
		"total_calls":           s.Calls,
		"success_calls":         s.Success,
		"failure_calls":         s.Failure,
		"success_rate":          rate,
		"input_tokens":          s.Input,
		"output_tokens":         s.Output,
		"reasoning_tokens":      s.Reasoning,
		"cached_tokens":         s.Cached,
		"cache_read_tokens":     s.CacheRead,
		"cache_creation_tokens": s.CacheCreation,
		"cache_tokens":          s.CacheTokens,
		"cache_hit_tokens":      s.CacheHitTokens,
		"cache_hit_input_tokens": s.CacheHitInputTokens,
		"cache_hit_rate":        cacheHitRate(s.CacheHitTokens, s.CacheHitInputTokens),
		"total_tokens":          s.Tokens,
		"tokens":                s.Tokens,
		"average_latency_ms":    avg,
		"cost":                  s.Cost,
		"total_cost":            s.Cost,
		"average_cost_per_call": avgCost,
		"cost_currency":         "USD",
		"cost_basis":            "openai_api_equivalent",
		"priced_calls":          s.PricedCalls,
		"unpriced_calls":        s.UnpricedCalls,
		"cost_complete":         s.UnpricedCalls == 0,
		"last_seen_ms":          s.Last,
	}
}

func cacheHitTotals(row eventRow) (hitTokens, inputTokens int64) {
	cached := max64(row.CachedTokens, 0)
	cacheRead := max64(row.CacheReadTokens, 0)
	cacheCreation := max64(row.CacheCreationTokens, 0)
	if cached > 0 && cached == cacheRead {
		// When CachedTokens and CacheReadTokens are equal on a single record,
		// they mirror the same cache-read count rather than two separate read buckets.
		hit := cached
		if pricing.IndependentCacheBuckets(pricing.Usage{Provider: row.Provider, ExecutorType: row.ExecutorType}) {
			return cacheRead, max64(row.InputTokens, 0) + cacheRead + cacheCreation
		}
		return hit, max64(max64(row.InputTokens, 0), hit) + cacheCreation
	}
	// Keep the historical monitoring KPI denominator so the new event field is
	// consistent with the existing summary cards across the plugin.
	inputTokens = max64(max64(row.InputTokens, 0), cached) + cacheRead + cacheCreation
	return cached + cacheRead, inputTokens
}

func cacheHitRate(hitTokens, inputTokens int64) float64 {
	if inputTokens <= 0 {
		return 0
	}
	rate := float64(max64(hitTokens, 0)) / float64(inputTokens)
	if rate > 1 {
		return 1
	}
	return rate
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type heatCellStats struct {
	stats
	byModel      map[string]*stats
	byCredential map[string]*stats
}

func aggregate(rows []eventRow, prices map[string]Price, request AnalyticsRequest) map[string]any {
	return aggregateWithWindow(rows, rows, prices, request)
}

func aggregateWithWindow(rows []eventRow, windowRows []eventRow, prices map[string]Price, request AnalyticsRequest) map[string]any {
	if len(windowRows) == 0 && len(rows) > 0 {
		windowRows = rows
	}
	total := stats{}
	byModel := map[string]*stats{}
	byAccount := map[string]*stats{}
	byKey := map[string]*stats{}
	byAccountAPIKey := map[string]*accountAPIKeyStats{}
	byBucket := map[string]*stats{}
	byHeat := map[string]*heatCellStats{}
	byCredBucket := map[string]map[string]*stats{}
	location := request.Location
	if location == nil {
		if strings.TrimSpace(request.TimeZone) != "" {
			location = ResolveAnalyticsLocation(request.TimeZone)
		} else {
			location = time.UTC
		}
	}
	granularity := request.Granularity
	if granularity != "day" {
		granularity = "hour"
	}
	drilldownLimit := request.DrilldownLimit
	if drilldownLimit <= 0 || drilldownLimit > 100 {
		drilldownLimit = 20
	}
	// events resolved providers before filtering. Re-inferring here would erase
	// conflicting evidence excluded by a model, identity, or search filter.
	events := make([]map[string]any, 0, min(len(rows), request.Limit))
	drilldownItems := []map[string]any{}
	for _, row := range rows {
		price := prices[row.Model]
		total.add(row, price)
		addStats(byModel, row.Model, row, price)
		account := accountSnapshot(row)
		apiKey := apiKeySnapshot(row)
		addStats(byAccount, account, row, price)
		addStats(byKey, apiKey, row, price)
		addAccountAPIKeyStats(byAccountAPIKey, row, price)
		bucket := AnalyticsBucketMS(row.TimestampMS, granularity, location)
		bucketKey := fmt.Sprint(bucket)
		addStats(byBucket, bucketKey, row, price)
		if byCredBucket[account] == nil {
			byCredBucket[account] = map[string]*stats{}
		}
		addStats(byCredBucket[account], bucketKey, row, price)
		heatAt := time.UnixMilli(row.TimestampMS).In(location)
		heatKey := fmt.Sprintf("%d-%02d", int(heatAt.Weekday()), heatAt.Hour())
		cell := byHeat[heatKey]
		if cell == nil {
			cell = &heatCellStats{byModel: map[string]*stats{}, byCredential: map[string]*stats{}}
			byHeat[heatKey] = cell
		}
		cell.add(row, price)
		addStats(cell.byModel, row.Model, row, price)
		addStats(cell.byCredential, account, row, price)
		if len(events) < request.Limit {
			events = append(events, eventJSON(row, price))
		}
		if request.DrilldownBucketMS > 0 && bucket == request.DrilldownBucketMS && len(drilldownItems) < drilldownLimit {
			item := eventJSON(row, price)
			if row.Failed != 0 {
				item["failure_rate"] = 1.0
			} else {
				item["failure_rate"] = 0.0
			}
			drilldownItems = append(drilldownItems, item)
		}
	}
	tzName := location.String()
	if request.TimeZone != "" {
		tzName = request.TimeZone
	}
	timelineRows := statsRowsWithShare(byBucket, "bucket_ms", total.Cost)
	modelRows := statsRowsWithShare(byModel, "model", total.Cost)
	accountRows := statsRowsWithShare(byAccount, "account_snapshot", total.Cost)
	credentialRows := statsRowsWithShare(byAccount, "auth_file", total.Cost)
	apiKeyRows := statsRowsWithShare(byKey, "api_key_hash", total.Cost)
	return map[string]any{
		"summary":               total.json(),
		"timeline":              timelineRows,
		"model_stats":           modelRows,
		"model_share":           modelRows,
		"account_stats":         accountRows,
		"credential_stats":      credentialRows,
		"credential_timeline":   buildCredentialTimeline(byCredBucket),
		"api_key_stats":         apiKeyRows,
		"account_api_key_stats": accountAPIKeyStatsRows(byAccountAPIKey),
		"events":                map[string]any{"items": events},
		"drilldown_preview":     map[string]any{"items": drilldownItems},
		"filter_options":        buildFilterOptions(windowRows),
		"granularity":           granularity,
		"time_zone":             tzName,
		"generated_at_ms":       time.Now().UnixMilli(),
		"heatmap":               heatmapRowsWithBreakdown(byHeat),
		"anomaly_points":        buildAnomalyPoints(timelineRows),
		"recent_failures":       failureRows(rows, prices),
	}
}

func buildFilterOptions(rows []eventRow) map[string]any {
	providers := map[string]bool{}
	models := map[string]bool{}
	authFiles := map[string]bool{}
	type acctOpt struct {
		AccountSnapshot string
		Source          string
		AuthType        string
		AuthID          string
		AuthIndex       string
		Provider        string
	}
	type keyOpt struct {
		APIKeyHash      string
		AccountSnapshot string
		Source          string
		AuthType        string
		Provider        string
	}
	acctMap := map[string]*acctOpt{}
	keyMap := map[string]*keyOpt{}
	for _, row := range rows {
		if row.Provider != "" {
			providers[row.Provider] = true
		}
		if row.Model != "" {
			models[row.Model] = true
		}
		if alias := strings.TrimSpace(row.Alias); alias != "" {
			models[alias] = true
		}
		account := accountSnapshot(row)
		authFiles[account] = true
		if row.AuthID != "" {
			authFiles[row.AuthID] = true
		}
		src := strings.TrimSpace(row.Source)
		if acct := acctMap[account]; acct == nil {
			acctMap[account] = &acctOpt{
				AccountSnapshot: account,
				Source:          src,
				AuthType:        normalizeAuthTypeSnapshot(row.AuthType),
				AuthID:          row.AuthID,
				AuthIndex:       row.AuthIndex,
				Provider:        row.Provider,
			}
		} else {
			if acct.Source == "" && src != "" && src != "unknown" {
				acct.Source = src
			}
			if acct.AuthType == "" && row.AuthType != "" {
				acct.AuthType = normalizeAuthTypeSnapshot(row.AuthType)
			}
			if acct.AuthID == "" && row.AuthID != "" {
				acct.AuthID = row.AuthID
			}
			if acct.AuthIndex == "" && row.AuthIndex != "" {
				acct.AuthIndex = row.AuthIndex
			}
			if acct.Provider == "" && row.Provider != "" {
				acct.Provider = row.Provider
			}
		}
		if keyHash := strings.TrimSpace(row.APIKeyHash); keyHash != "" {
			if k := keyMap[keyHash]; k == nil {
				keyMap[keyHash] = &keyOpt{
					APIKeyHash:      keyHash,
					AccountSnapshot: account,
					Source:          src,
					AuthType:        normalizeAuthTypeSnapshot(row.AuthType),
					Provider:        row.Provider,
				}
			} else {
				if (k.AccountSnapshot == "" || k.AccountSnapshot == "unknown") && account != "" && account != "unknown" {
					k.AccountSnapshot = account
				}
				if (k.Source == "" || k.Source == "unknown") && src != "" && src != "unknown" {
					k.Source = src
				}
				if k.AuthType == "" && row.AuthType != "" {
					k.AuthType = normalizeAuthTypeSnapshot(row.AuthType)
				}
				if k.Provider == "" && row.Provider != "" {
					k.Provider = row.Provider
				}
			}
		}
	}
	acctKeys := make([]string, 0, len(acctMap))
	for k := range acctMap {
		if k != "" {
			acctKeys = append(acctKeys, k)
		}
	}
	sort.Strings(acctKeys)
	accountsList := make([]map[string]any, 0, len(acctKeys))
	for _, k := range acctKeys {
		item := acctMap[k]
		accountsList = append(accountsList, map[string]any{
			"id":               item.AccountSnapshot,
			"value":            item.AccountSnapshot,
			"account_snapshot": item.AccountSnapshot,
			"source":           item.Source,
			"auth_type":        item.AuthType,
			"auth_id":          item.AuthID,
			"auth_index":       item.AuthIndex,
			"provider":         item.Provider,
		})
	}
	keyHashes := make([]string, 0, len(keyMap))
	for k := range keyMap {
		if k != "" {
			keyHashes = append(keyHashes, k)
		}
	}
	sort.Strings(keyHashes)
	apiKeysList := make([]map[string]any, 0, len(keyHashes))
	for _, k := range keyHashes {
		item := keyMap[k]
		apiKeysList = append(apiKeysList, map[string]any{
			"id":               item.APIKeyHash,
			"value":            item.APIKeyHash,
			"api_key_hash":     item.APIKeyHash,
			"account_snapshot": item.AccountSnapshot,
			"source":           item.Source,
			"auth_type":        item.AuthType,
			"provider":         item.Provider,
		})
	}
	return map[string]any{
		"providers":   keysOf(providers),
		"model_stats": namedKeys(models, "model"),
		"auth_files":  keysOf(authFiles),
		"accounts":    accountsList,
		"api_keys":    apiKeysList,
	}
}

func buildCredentialTimeline(byCredBucket map[string]map[string]*stats) []map[string]any {
	out := []map[string]any{}
	for cred, buckets := range byCredBucket {
		for bucketStr, value := range buckets {
			var bucketMS int64
			fmt.Sscan(bucketStr, &bucketMS)
			out = append(out, map[string]any{
				"id":                 cred,
				"auth_file_snapshot": cred,
				"auth_index":         value.AuthIndex,
				"bucket_ms":          bucketMS,
				"calls":              value.Calls,
				"tokens":             value.Tokens,
				"total_tokens":       value.Tokens,
				"cost":               value.Cost,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		leftID, rightID := fmt.Sprint(out[i]["id"]), fmt.Sprint(out[j]["id"])
		if leftID != rightID {
			return leftID < rightID
		}
		return out[i]["bucket_ms"].(int64) < out[j]["bucket_ms"].(int64)
	})
	return out
}

func buildAnomalyPoints(timeline []map[string]any) []map[string]any {
	out := []map[string]any{}
	for i, point := range timeline {
		calls, _ := point["calls"].(int64)
		failures, _ := point["failure_calls"].(int64)
		if calls <= 0 {
			continue
		}
		failureRate := float64(failures) / float64(calls)
		reqChange := 0.0
		if i > 0 {
			prevCalls, _ := timeline[i-1]["calls"].(int64)
			if prevCalls > 0 {
				reqChange = float64(calls-prevCalls) / float64(prevCalls)
			}
		}
		if (failures >= 2 && failureRate >= 0.2) || (calls >= 10 && reqChange >= 1.5) {
			severity := "medium"
			if failureRate >= 0.5 || failures >= 10 {
				severity = "high"
			}
			bucketMS, _ := point["bucket_ms"].(int64)
			cost, _ := point["cost"].(float64)
			out = append(out, map[string]any{
				"bucket_ms":      bucketMS,
				"label":          time.UnixMilli(bucketMS).UTC().Format("01-02 15:04"),
				"severity":       severity,
				"calls":          calls,
				"failure_rate":   failureRate,
				"cost":           cost,
				"request_change": reqChange,
			})
		}
	}
	return out
}
func addStats(group map[string]*stats, key string, row eventRow, price Price) {
	if key == "" {
		key = "unknown"
	}
	value := group[key]
	if value == nil {
		value = &stats{}
		group[key] = value
	}
	value.add(row, price)
}
func accountAPIKeyIdentity(row eventRow) string {
	authType := normalizeAuthTypeSnapshot(row.AuthType)
	provider := normalizeProvider(row.Provider)
	if row.AuthIndex != "" {
		return strings.Join([]string{"auth-index", authType, provider, row.AuthIndex}, "::")
	}
	if row.AuthID != "" {
		return strings.Join([]string{"auth-id", authType, provider, row.AuthID}, "::")
	}
	if row.APIKeyHash != "" {
		return strings.Join([]string{"api-key", authType, provider, row.APIKeyHash}, "::")
	}
	return strings.Join([]string{"source", authType, provider, sourceSnapshot(row)}, "::")
}

func normalizeAuthTypeSnapshot(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "oauth", "oauth2":
		return "oauth"
	case "api", "api_key", "api-key", "apikey":
		return "apikey"
	default:
		return strings.TrimSpace(value)
	}
}

func addAccountAPIKeyStats(group map[string]*accountAPIKeyStats, row eventRow, price Price) {
	identity := accountAPIKeyIdentity(row)
	value := group[identity]
	if value == nil {
		value = &accountAPIKeyStats{Source: sourceSnapshot(row)}
		group[identity] = value
	}
	if row.TimestampMS >= value.Last {
		value.Source = sourceSnapshot(row)
		if row.Provider != "" {
			value.Provider = row.Provider
		}
		if row.APIKeyHash != "" || value.APIKey == "" {
			value.APIKey = apiKeySnapshot(row)
		}
		if row.AuthType != "" {
			value.AuthType = normalizeAuthTypeSnapshot(row.AuthType)
		}
		if row.AuthID != "" {
			value.AuthID = row.AuthID
		}
		if row.AuthIndex != "" {
			value.AuthIndex = row.AuthIndex
		}
	}
	value.add(row, price)
}

func accountAPIKeyStatsRows(group map[string]*accountAPIKeyStats) []map[string]any {
	out := make([]map[string]any, 0, len(group))
	for identity, value := range group {
		row := value.json()
		row["id"] = identity
		row["source"] = value.Source
		row["account_snapshot"] = value.Source
		row["api_key_hash"] = value.APIKey
		row["auth_provider_snapshot"] = value.Provider
		row["auth_type"] = value.AuthType
		row["auth_id"] = value.AuthID
		row["auth_index"] = value.AuthIndex
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		leftSource, rightSource := fmt.Sprint(out[i]["source"]), fmt.Sprint(out[j]["source"])
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		return out[i]["calls"].(int64) > out[j]["calls"].(int64)
	})
	return out
}

func heatmapRowsWithBreakdown(group map[string]*heatCellStats) []map[string]any {
	out := make([]map[string]any, 0, len(group))
	for key, value := range group {
		var weekday, hour int
		if _, err := fmt.Sscanf(key, "%d-%d", &weekday, &hour); err != nil {
			continue
		}
		row := value.json()
		row["weekday"] = weekday
		row["hour"] = hour
		row["success"] = value.Success
		row["failure"] = value.Failure
		row["tokens"] = value.Tokens
		if value.Calls > 0 {
			row["failure_rate"] = float64(value.Failure) / float64(value.Calls)
		} else {
			row["failure_rate"] = 0.0
		}
		row["top_models"] = topHeatBreakdown(value.byModel, value.Calls)
		row["top_credentials"] = topHeatBreakdown(value.byCredential, value.Calls)
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		leftDay, rightDay := out[i]["weekday"].(int), out[j]["weekday"].(int)
		if leftDay != rightDay {
			return leftDay < rightDay
		}
		return out[i]["hour"].(int) < out[j]["hour"].(int)
	})
	return out
}

func topHeatBreakdown(group map[string]*stats, totalCalls int64) []map[string]any {
	out := make([]map[string]any, 0, len(group))
	for label, st := range group {
		failRate := 0.0
		if st.Calls > 0 {
			failRate = float64(st.Failure) / float64(st.Calls)
		}
		share := 0.0
		if totalCalls > 0 {
			share = float64(st.Calls) / float64(totalCalls)
		}
		out = append(out, map[string]any{
			"label":        label,
			"calls":        st.Calls,
			"tokens":       st.Tokens,
			"cost":         st.Cost,
			"failure_rate": failRate,
			"share":        share,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		leftCalls, rightCalls := out[i]["calls"].(int64), out[j]["calls"].(int64)
		if leftCalls != rightCalls {
			return leftCalls > rightCalls
		}
		return fmt.Sprint(out[i]["label"]) < fmt.Sprint(out[j]["label"])
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func statsRows(group map[string]*stats, key string) []map[string]any {
	return statsRowsWithShare(group, key, 0)
}

func statsRowsWithShare(group map[string]*stats, key string, totalCost float64) []map[string]any {
	out := make([]map[string]any, 0, len(group))
	for name, value := range group {
		row := value.json()
		if key == "bucket_ms" {
			var n int64
			fmt.Sscan(name, &n)
			row[key] = n
		} else {
			row[key] = name
			if value.Provider != "" {
				row["provider"] = value.Provider
				row["auth_provider_snapshot"] = value.Provider
			}
			if key != "account_snapshot" && value.AccountSnapshot != "" {
				row["account_snapshot"] = value.AccountSnapshot
			}
			if value.Source != "" {
				row["source"] = value.Source
			}
			if value.AuthIndex != "" {
				row["auth_index"] = value.AuthIndex
			}
			if key != "auth_file" && value.AuthID != "" {
				row["auth_file"] = value.AuthID
			}
			if value.AuthType != "" {
				row["auth_type"] = value.AuthType
			}
			if totalCost > 0 {
				row["cost_share"] = value.Cost / totalCost
			} else {
				row["cost_share"] = 0.0
			}
		}
		out = append(out, row)
	}
	if key == "bucket_ms" {
		sort.Slice(out, func(i, j int) bool {
			left, _ := out[i][key].(int64)
			right, _ := out[j][key].(int64)
			return left < right
		})
	} else {
		sort.Slice(out, func(i, j int) bool {
			leftCalls, _ := out[i]["calls"].(int64)
			rightCalls, _ := out[j]["calls"].(int64)
			if leftCalls != rightCalls {
				return leftCalls > rightCalls
			}
			return fmt.Sprint(out[i][key]) < fmt.Sprint(out[j][key])
		})
	}
	return out
}
func keysOf(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
func namedKeys(values map[string]bool, key string) []map[string]string {
	out := make([]map[string]string, 0, len(values))
	for _, value := range keysOf(values) {
		out = append(out, map[string]string{key: value})
	}
	return out
}
func requestedModel(row eventRow) string {
	if alias := strings.TrimSpace(row.Alias); alias != "" {
		return alias
	}
	return row.Model
}

func requestProtocol(executorType string) string {
	name := strings.ToLower(strings.TrimSpace(executorType))
	if strings.Contains(name, "websocket") {
		return "websocket"
	}
	return "http"
}

func parseResponseHeadersMeta(rawJSON string) (map[string]any, string, any) {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return nil, "", nil
	}
	var parsed map[string][]string
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil || len(parsed) == 0 {
		return nil, "", nil
	}
	headersOut := map[string]any{}
	traceID := ""
	var recoverAtMS any
	for k, vals := range parsed {
		if len(vals) == 0 {
			continue
		}
		if len(vals) == 1 {
			headersOut[k] = vals[0]
		} else {
			headersOut[k] = vals
		}
		first := strings.TrimSpace(vals[0])
		if first == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "x-trace-id", "x-request-id", "request-id", "x-goog-request-id":
			if traceID == "" || strings.EqualFold(k, "x-trace-id") {
				traceID = first
			}
		case "x-ratelimit-reset", "retry-after":
			if n, err := strconv.ParseInt(first, 10, 64); err == nil && n > 0 {
				if n < 1_000_000_000 {
					recoverAtMS = n * 1000
				} else if n < 1_000_000_000_000 {
					recoverAtMS = n * 1000
				} else {
					recoverAtMS = n
				}
			}
		}
	}
	return headersOut, traceID, recoverAtMS
}

func positiveInt64OrNil(v sql.NullInt64) any {
	if !v.Valid || v.Int64 <= 0 {
		return nil
	}
	return v.Int64
}

func eventJSON(row eventRow, price Price) map[string]any {
	hitTokens, inputTokens := cacheHitTotals(row)
	response := resolveResponseModel(row.ResponseModel, row.ObservedResponseModel, row.ResponseObservationPresent, row.ResponseObservationAmbiguous, row.ResponseUsageCount, row.Failed != 0)
	estimate := estimateEventCost(row, price)
	dispTier := displayedResponseTier(row)
	eventHash := strings.TrimSpace(row.EventHash)
	if eventHash == "" {
		eventHash = fmt.Sprint(row.ID)
	}
	headersMap, traceID, recoverAtMS := parseResponseHeadersMeta(row.ResponseHeadersJSON)
	responseMetadata := map[string]any{
		"response_model":          response.Model,
		"host_response_model":     response.Host,
		"observed_response_model": response.Observed,
		"response_model_source":   response.Source,
		"response_model_conflict": response.Conflict,
		"response_service_tier":   dispTier,
	}
	if len(headersMap) > 0 {
		responseMetadata["response_headers"] = headersMap
	}
	out := map[string]any{
		"id":                      row.ID,
		"timestamp_ms":            row.TimestampMS,
		"event_hash":              eventHash,
		"provider":                row.Provider,
		"auth_provider_snapshot":  row.Provider,
		"auth_type":               row.AuthType,
		"executor_type":           row.ExecutorType,
		"protocol":                requestProtocol(row.ExecutorType),
		"model":                   row.Model,
		"alias":                   strings.TrimSpace(row.Alias),
		"requested_model":         requestedModel(row),
		"resolved_model":          row.Model,
		"response_model":          response.Model,
		"host_response_model":     response.Host,
		"observed_response_model": response.Observed,
		"response_model_source":   response.Source,
		"response_model_conflict": response.Conflict,
		"api_key_hash":            row.APIKeyHash,
		"account_snapshot":        accountSnapshot(row),
		"auth_index":              row.AuthIndex,
		"auth_file_snapshot":      row.AuthID,
		"source":                  sourceSnapshot(row),
		"reasoning_effort":        row.ReasoningEffort,
		"service_tier":            row.ServiceTier,
		"response_service_tier":   dispTier,
		"input_tokens":            row.InputTokens,
		"output_tokens":           row.OutputTokens,
		"reasoning_tokens":        row.ReasoningTokens,
		"cached_tokens":           row.CachedTokens,
		"cache_read_tokens":       row.CacheReadTokens,
		"cache_creation_tokens":   row.CacheCreationTokens,
		"cache_tokens":            rowCacheTokens(row),
		"cache_hit_tokens":        hitTokens,
		"cache_hit_input_tokens":  inputTokens,
		"cache_hit_rate":          cacheHitRate(hitTokens, inputTokens),
		"total_tokens":            row.TotalTokens,
		"latency_ms":              row.LatencyMS.Int64,
		"ttft_ms":                 positiveInt64OrNil(row.TTFTMS),
		"failed":                  row.Failed != 0,
		"fail_status_code":        positiveInt64OrNil(row.FailStatus),
		"fail_summary":            row.FailSummary.String,
		"cost":                    displayedCost(estimate),
		"cost_estimate":           estimate,
		"response_metadata":       responseMetadata,
	}
	if corr := strings.TrimSpace(row.ResponseCorrelationKey); corr != "" {
		out["request_id"] = corr
	}
	if traceID != "" {
		out["header_trace_id"] = traceID
	}
	if recoverAtMS != nil {
		out["header_quota_recover_at_ms"] = recoverAtMS
	}
	return out
}
func failureRows(rows []eventRow, prices map[string]Price) []map[string]any {
	out := []map[string]any{}
	for _, row := range rows {
		if row.Failed != 0 {
			out = append(out, eventJSON(row, prices[row.Model]))
			if len(out) == 30 {
				break
			}
		}
	}
	return out
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
