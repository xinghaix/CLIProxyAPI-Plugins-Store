package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
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
	FromMS        int64
	ToMS          int64
	Limit         int
	Models        []string
	Providers     []string
	Accounts      []string
	APIKeyHashes  []string
	FailedOnly    bool
	IncludeFailed bool
	Search        string
	Granularity   string
}

type eventRow struct {
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
	rows, err := s.events(ctx, request)
	if err != nil {
		return nil, err
	}
	prices, err := s.Prices(ctx)
	if err != nil {
		return nil, err
	}
	return aggregate(rows, prices, request), nil
}

func (s *Store) events(ctx context.Context, request AnalyticsRequest) ([]eventRow, error) {
	query := `select response_correlation_key,id,timestamp_ms,coalesce(provider,''),coalesce(executor_type,''),model,coalesce(alias,''),coalesce(response_model,''),coalesce(api_key_hash,''),coalesce(auth_id,''),coalesce(auth_index,''),coalesce(auth_type,''),coalesce(source,''),coalesce(reasoning_effort,''),coalesce(service_tier,''),input_tokens,output_tokens,reasoning_tokens,cached_tokens,cache_read_tokens,cache_creation_tokens,total_tokens,latency_ms,ttft_ms,failed,fail_status_code,fail_summary from usage_events where timestamp_ms >= ? and timestamp_ms <= ?`
	args := []any{request.FromMS, request.ToMS}
	if request.FailedOnly {
		query += ` and failed = 1`
	} else if !request.IncludeFailed {
		query += ` and failed = 0`
	}
	search := strings.TrimSpace(request.Search)
	query += ` order by timestamp_ms desc limit ?`
	args = append(args, 10_000)
	// Count matches globally, not only inside the selected time/filter window.
	// The correlation index restricts grouping to keys in this bounded candidate set.
	query = `with selected as (` + query + `), usage_matches as (
	 select response_correlation_key, count(*) as usage_count from usage_events
	 where response_correlation_key in (select response_correlation_key from selected)
	 group by response_correlation_key
	) select selected.*, coalesce(o.model,''), o.correlation_key is not null,
	 coalesce(o.ambiguous,0), coalesce(o.service_tier,''), coalesce(o.service_tier_ambiguous,0), coalesce(m.usage_count,0)
	 from selected left join response_observations o on o.correlation_key=selected.response_correlation_key
	 left join usage_matches m on m.response_correlation_key=selected.response_correlation_key
	 order by selected.timestamp_ms desc`
	dbRows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()
	var candidates []eventRow
	for dbRows.Next() {
		var row eventRow
		var correlationKey sql.NullString
		if err := dbRows.Scan(&correlationKey, &row.ID, &row.TimestampMS, &row.Provider, &row.ExecutorType, &row.Model, &row.Alias, &row.ResponseModel, &row.APIKeyHash, &row.AuthID, &row.AuthIndex, &row.AuthType, &row.Source, &row.ReasoningEffort, &row.ServiceTier, &row.InputTokens, &row.OutputTokens, &row.ReasoningTokens, &row.CachedTokens, &row.CacheReadTokens, &row.CacheCreationTokens, &row.TotalTokens, &row.LatencyMS, &row.TTFTMS, &row.Failed, &row.FailStatus, &row.FailSummary, &row.ObservedResponseModel, &row.ResponseObservationPresent, &row.ResponseObservationAmbiguous, &row.ObservedServiceTier, &row.ResponseTierAmbiguous, &row.ResponseUsageCount); err != nil {
			return nil, err
		}
		candidates = append(candidates, row)
	}
	if err := dbRows.Err(); err != nil {
		return nil, err
	}
	providerLookup := providerSnapshots(candidates)
	results := make([]eventRow, 0, len(candidates))
	for _, row := range candidates {
		if s.responseObservationsSuppressed.Load() {
			row.ResponseObservationAmbiguous = true
		}
		row.Provider = resolvedProvider(row, providerLookup)
		if matches(row, request) && matchesSearch(row, search) {
			results = append(results, row)
		}
	}
	return results, nil
}

func matches(row eventRow, request AnalyticsRequest) bool {
	return includesModel(request.Models, row) && includes(request.Providers, row.Provider) && includes(request.Accounts, accountSnapshot(row)) && includes(request.APIKeyHashes, apiKeySnapshot(row))
}

func matchesSearch(row eventRow, search string) bool {
	needle := strings.ToLower(strings.TrimSpace(search))
	if needle == "" {
		return true
	}
	for _, value := range []string{row.Model, row.Alias, row.Provider, row.AuthIndex, row.Source, row.FailSummary.String} {
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

type stats struct {
	Calls, Success, Failure, Input, Output, Reasoning, Cached, CacheRead, CacheCreation, Tokens int64
	CacheHitTokens, CacheHitInputTokens                                                         int64
	Latency                                                                                     int64
	LatencySamples                                                                              int64
	Cost                                                                                        float64
	PricedCalls, UnpricedCalls                                                                  int64
	Last                                                                                        int64
}

type accountAPIKeyStats struct {
	stats
	Source, APIKey, Provider, AuthType, AuthID, AuthIndex string
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
	hitTokens, inputTokens := cacheHitTotals(row)
	s.CacheHitTokens += hitTokens
	s.CacheHitInputTokens += inputTokens
	s.Tokens += row.TotalTokens
	if row.LatencyMS.Valid {
		s.Latency += row.LatencyMS.Int64
		s.LatencySamples++
	}
	s.addEstimate(estimateEventCost(row, price))
	if row.TimestampMS > s.Last {
		s.Last = row.TimestampMS
	}
}
func (s stats) json() map[string]any {
	rate := float64(0)
	if s.Calls > 0 {
		rate = float64(s.Success) / float64(s.Calls)
	}
	avg := float64(0)
	if s.LatencySamples > 0 {
		avg = float64(s.Latency) / float64(s.LatencySamples)
	}
	return map[string]any{"calls": s.Calls, "total_calls": s.Calls, "success_calls": s.Success, "failure_calls": s.Failure, "success_rate": rate, "input_tokens": s.Input, "output_tokens": s.Output, "reasoning_tokens": s.Reasoning, "cached_tokens": s.Cached, "cache_read_tokens": s.CacheRead, "cache_creation_tokens": s.CacheCreation, "cache_hit_tokens": s.CacheHitTokens, "cache_hit_input_tokens": s.CacheHitInputTokens, "cache_hit_rate": cacheHitRate(s.CacheHitTokens, s.CacheHitInputTokens), "total_tokens": s.Tokens, "tokens": s.Tokens, "average_latency_ms": avg, "cost": s.Cost, "total_cost": s.Cost, "cost_currency": "USD", "cost_basis": "openai_api_equivalent", "priced_calls": s.PricedCalls, "unpriced_calls": s.UnpricedCalls, "cost_complete": s.UnpricedCalls == 0, "last_seen_ms": s.Last}
}
func cacheHitTotals(row eventRow) (hitTokens, inputTokens int64) {
	cached := max64(row.CachedTokens, 0)
	cacheRead := max64(row.CacheReadTokens, 0)
	cacheCreation := max64(row.CacheCreationTokens, 0)
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

func aggregate(rows []eventRow, prices map[string]Price, request AnalyticsRequest) map[string]any {
	total := stats{}
	byModel := map[string]*stats{}
	byAccount := map[string]*stats{}
	byKey := map[string]*stats{}
	byAccountAPIKey := map[string]*accountAPIKeyStats{}
	byBucket := map[string]*stats{}
	byHeat := map[string]*stats{}
	providers := map[string]bool{}
	models := map[string]bool{}
	accounts := map[string]bool{}
	keys := map[string]bool{}
	bucketSize := int64(3600000)
	if request.Granularity == "day" {
		bucketSize = 86400000
	}
	providerLookup := providerSnapshots(rows)
	events := make([]map[string]any, 0, min(len(rows), request.Limit))
	for _, row := range rows {
		row.Provider = resolvedProvider(row, providerLookup)
		price := prices[row.Model]
		total.add(row, price)
		addStats(byModel, row.Model, row, price)
		account := accountSnapshot(row)
		apiKey := apiKeySnapshot(row)
		addStats(byAccount, account, row, price)
		addStats(byKey, apiKey, row, price)
		addAccountAPIKeyStats(byAccountAPIKey, row, price)
		bucket := row.TimestampMS / bucketSize * bucketSize
		addStats(byBucket, fmt.Sprint(bucket), row, price)
		heatAt := time.UnixMilli(row.TimestampMS).UTC()
		addStats(byHeat, fmt.Sprintf("%d-%02d", int(heatAt.Weekday()), heatAt.Hour()), row, price)
		providers[row.Provider] = true
		models[row.Model] = true
		if alias := strings.TrimSpace(row.Alias); alias != "" {
			models[alias] = true
		}
		accounts[account] = true
		keys[row.APIKeyHash] = true
		if len(events) < request.Limit {
			events = append(events, eventJSON(row, price))
		}
	}
	return map[string]any{"summary": total.json(), "timeline": statsRows(byBucket, "bucket_ms"), "model_stats": statsRows(byModel, "model"), "model_share": statsRows(byModel, "model"), "account_stats": statsRows(byAccount, "account_snapshot"), "credential_stats": statsRows(byAccount, "auth_file"), "api_key_stats": statsRows(byKey, "api_key_hash"), "account_api_key_stats": accountAPIKeyStatsRows(byAccountAPIKey), "events": map[string]any{"items": events}, "filter_options": map[string]any{"providers": keysOf(providers), "model_stats": namedKeys(models, "model"), "auth_files": keysOf(accounts)}, "granularity": request.Granularity, "generated_at_ms": time.Now().UnixMilli(), "heatmap": heatmapRows(byHeat), "anomaly_points": []any{}, "recent_failures": failureRows(rows, prices)}
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

func heatmapRows(group map[string]*stats) []map[string]any {
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
		if value.Calls > 0 {
			row["failure_rate"] = float64(value.Failure) / float64(value.Calls)
		} else {
			row["failure_rate"] = 0.0
		}
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

func statsRows(group map[string]*stats, key string) []map[string]any {
	out := make([]map[string]any, 0, len(group))
	for name, value := range group {
		row := value.json()
		if key == "bucket_ms" {
			var n int64
			fmt.Sscan(name, &n)
			row[key] = n
		} else {
			row[key] = name
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i][key]) < fmt.Sprint(out[j][key]) })
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

func eventJSON(row eventRow, price Price) map[string]any {
	hitTokens, inputTokens := cacheHitTotals(row)
	response := resolveResponseModel(row.ResponseModel, row.ObservedResponseModel, row.ResponseObservationPresent, row.ResponseObservationAmbiguous, row.ResponseUsageCount, row.Failed != 0)
	estimate := estimateEventCost(row, price)
	return map[string]any{
		"id":                      row.ID,
		"timestamp_ms":            row.TimestampMS,
		"event_hash":              fmt.Sprint(row.ID),
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
		"response_service_tier":   displayedResponseTier(row),
		"input_tokens":            row.InputTokens,
		"output_tokens":           row.OutputTokens,
		"reasoning_tokens":        row.ReasoningTokens,
		"cached_tokens":           row.CachedTokens,
		"cache_read_tokens":       row.CacheReadTokens,
		"cache_creation_tokens":   row.CacheCreationTokens,
		"cache_hit_tokens":        hitTokens,
		"cache_hit_input_tokens":  inputTokens,
		"cache_hit_rate":          cacheHitRate(hitTokens, inputTokens),
		"total_tokens":            row.TotalTokens,
		"latency_ms":              row.LatencyMS.Int64,
		"ttft_ms":                 row.TTFTMS.Int64,
		"failed":                  row.Failed != 0,
		"fail_status_code":        row.FailStatus.Int64,
		"fail_summary":            row.FailSummary.String,
		"cost":                    displayedCost(estimate),
		"cost_estimate":           estimate,
	}
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
