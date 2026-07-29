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
	ID                                                                                                           int64
	TimestampMS                                                                                                  int64
	Provider, ExecutorType, Model, APIKeyHash, AuthID, AuthIndex, AuthType, Source, ReasoningEffort, ServiceTier string
	InputTokens, OutputTokens, ReasoningTokens, CachedTokens, CacheReadTokens, CacheCreationTokens, TotalTokens  int64
	LatencyMS, TTFTMS                                                                                            sql.NullInt64
	Failed                                                                                                       int
	FailStatus                                                                                                   sql.NullInt64
	FailSummary                                                                                                  sql.NullString
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

func (s *Store) Analytics(ctx context.Context, request AnalyticsRequest) (map[string]any, error) {
	if request.ToMS <= 0 {
		request.ToMS = time.Now().UnixMilli()
	}
	if request.FromMS < 0 || request.FromMS >= request.ToMS {
		return nil, fmt.Errorf("invalid time range")
	}
	if request.Limit < 1 || request.Limit > 1_000 {
		request.Limit = 300
	}
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
	query := `select id,timestamp_ms,coalesce(provider,''),coalesce(executor_type,''),model,coalesce(api_key_hash,''),coalesce(auth_id,''),coalesce(auth_index,''),coalesce(auth_type,''),coalesce(source,''),coalesce(reasoning_effort,''),coalesce(service_tier,''),input_tokens,output_tokens,reasoning_tokens,cached_tokens,cache_read_tokens,cache_creation_tokens,total_tokens,latency_ms,ttft_ms,failed,fail_status_code,fail_summary from usage_events where timestamp_ms >= ? and timestamp_ms <= ?`
	args := []any{request.FromMS, request.ToMS}
	if request.FailedOnly {
		query += ` and failed = 1`
	} else if !request.IncludeFailed {
		query += ` and failed = 0`
	}
	if search := strings.TrimSpace(request.Search); search != "" {
		query += ` and (model like ? or provider like ? or auth_index like ? or source like ? or fail_summary like ?)`
		like := "%" + search + "%"
		args = append(args, like, like, like, like, like)
	}
	query += ` order by timestamp_ms desc limit ?`
	args = append(args, 10_000)
	dbRows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()
	var results []eventRow
	for dbRows.Next() {
		var row eventRow
		if err := dbRows.Scan(&row.ID, &row.TimestampMS, &row.Provider, &row.ExecutorType, &row.Model, &row.APIKeyHash, &row.AuthID, &row.AuthIndex, &row.AuthType, &row.Source, &row.ReasoningEffort, &row.ServiceTier, &row.InputTokens, &row.OutputTokens, &row.ReasoningTokens, &row.CachedTokens, &row.CacheReadTokens, &row.CacheCreationTokens, &row.TotalTokens, &row.LatencyMS, &row.TTFTMS, &row.Failed, &row.FailStatus, &row.FailSummary); err != nil {
			return nil, err
		}
		if matches(row, request) {
			results = append(results, row)
		}
	}
	return results, dbRows.Err()
}

func matches(row eventRow, request AnalyticsRequest) bool {
	return includes(request.Models, row.Model) && includes(request.Providers, row.Provider) && includes(request.Accounts, accountSnapshot(row)) && includes(request.APIKeyHashes, apiKeySnapshot(row))
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
	Latency                                                                                     int64
	LatencySamples                                                                              int64
	Cost                                                                                        float64
	Last                                                                                        int64
}

type accountAPIKeyStats struct {
	stats
	Account  string
	APIKey   string
	Provider string
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
	s.Tokens += row.TotalTokens
	if row.LatencyMS.Valid {
		s.Latency += row.LatencyMS.Int64
		s.LatencySamples++
	}
	s.Cost += cost(row, price)
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
	return map[string]any{"calls": s.Calls, "total_calls": s.Calls, "success_calls": s.Success, "failure_calls": s.Failure, "success_rate": rate, "input_tokens": s.Input, "output_tokens": s.Output, "reasoning_tokens": s.Reasoning, "cached_tokens": s.Cached, "cache_read_tokens": s.CacheRead, "cache_creation_tokens": s.CacheCreation, "total_tokens": s.Tokens, "tokens": s.Tokens, "average_latency_ms": avg, "cost": s.Cost, "total_cost": s.Cost, "last_seen_ms": s.Last}
}
func cost(row eventRow, price Price) float64 {
	return (float64(max64(row.InputTokens-row.CachedTokens, 0))*price.Prompt + float64(row.OutputTokens)*price.Completion + float64(row.CachedTokens)*price.Cache + float64(row.CacheReadTokens)*price.CacheRead + float64(row.CacheCreationTokens)*price.CacheCreation) / 1_000_000
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
	providers := map[string]bool{}
	models := map[string]bool{}
	accounts := map[string]bool{}
	keys := map[string]bool{}
	bucketSize := int64(3600000)
	if request.Granularity == "day" {
		bucketSize = 86400000
	}
	events := make([]map[string]any, 0, min(len(rows), request.Limit))
	for _, row := range rows {
		price := prices[row.Model]
		total.add(row, price)
		addStats(byModel, row.Model, row, price)
		account := accountSnapshot(row)
		apiKey := apiKeySnapshot(row)
		addStats(byAccount, account, row, price)
		addStats(byKey, apiKey, row, price)
		addAccountAPIKeyStats(byAccountAPIKey, account, apiKey, row, price)
		bucket := row.TimestampMS / bucketSize * bucketSize
		addStats(byBucket, fmt.Sprint(bucket), row, price)
		providers[row.Provider] = true
		models[row.Model] = true
		accounts[account] = true
		keys[row.APIKeyHash] = true
		if len(events) < request.Limit {
			events = append(events, eventJSON(row, price))
		}
	}
	return map[string]any{"summary": total.json(), "timeline": statsRows(byBucket, "bucket_ms"), "model_stats": statsRows(byModel, "model"), "model_share": statsRows(byModel, "model"), "account_stats": statsRows(byAccount, "account_snapshot"), "credential_stats": statsRows(byAccount, "auth_file"), "api_key_stats": statsRows(byKey, "api_key_hash"), "account_api_key_stats": accountAPIKeyStatsRows(byAccountAPIKey), "events": map[string]any{"items": events}, "filter_options": map[string]any{"providers": keysOf(providers), "model_stats": namedKeys(models, "model"), "auth_files": keysOf(accounts)}, "granularity": request.Granularity, "generated_at_ms": time.Now().UnixMilli(), "heatmap": []any{}, "anomaly_points": []any{}, "recent_failures": failureRows(rows, prices)}
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
func addAccountAPIKeyStats(group map[string]*accountAPIKeyStats, account, apiKey string, row eventRow, price Price) {
	key := account + "\x00" + apiKey
	value := group[key]
	if value == nil {
		value = &accountAPIKeyStats{Account: account, APIKey: apiKey}
		group[key] = value
	}
	if row.TimestampMS >= value.Last {
		value.Provider = row.Provider
	}
	value.add(row, price)
}

func accountAPIKeyStatsRows(group map[string]*accountAPIKeyStats) []map[string]any {
	out := make([]map[string]any, 0, len(group))
	for _, value := range group {
		row := value.json()
		row["id"] = value.Account + "::" + value.APIKey
		row["account_snapshot"] = value.Account
		row["api_key_hash"] = value.APIKey
		row["auth_provider_snapshot"] = value.Provider
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		leftAccount, rightAccount := fmt.Sprint(out[i]["account_snapshot"]), fmt.Sprint(out[j]["account_snapshot"])
		if leftAccount != rightAccount {
			return leftAccount < rightAccount
		}
		leftCalls, rightCalls := out[i]["calls"].(int64), out[j]["calls"].(int64)
		if leftCalls != rightCalls {
			return leftCalls > rightCalls
		}
		return fmt.Sprint(out[i]["api_key_hash"]) < fmt.Sprint(out[j]["api_key_hash"])
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
func eventJSON(row eventRow, price Price) map[string]any {
	return map[string]any{"id": row.ID, "timestamp_ms": row.TimestampMS, "event_hash": fmt.Sprint(row.ID), "provider": row.Provider, "auth_provider_snapshot": row.Provider, "model": row.Model, "api_key_hash": row.APIKeyHash, "account_snapshot": accountSnapshot(row), "auth_index": row.AuthIndex, "auth_file_snapshot": row.AuthID, "source": row.Source, "reasoning_effort": row.ReasoningEffort, "service_tier": row.ServiceTier, "input_tokens": row.InputTokens, "output_tokens": row.OutputTokens, "reasoning_tokens": row.ReasoningTokens, "cached_tokens": row.CachedTokens, "cache_read_tokens": row.CacheReadTokens, "cache_creation_tokens": row.CacheCreationTokens, "total_tokens": row.TotalTokens, "latency_ms": row.LatencyMS.Int64, "ttft_ms": row.TTFTMS.Int64, "failed": row.Failed != 0, "fail_status_code": row.FailStatus.Int64, "fail_summary": row.FailSummary.String, "cost": cost(row, price)}
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
