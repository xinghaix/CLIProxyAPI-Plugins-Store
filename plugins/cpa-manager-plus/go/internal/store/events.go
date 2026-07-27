package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Event struct {
	Hash                string
	TimestampMS         int64
	Provider            string
	ExecutorType        string
	Model               string
	APIKeyHash          string
	AuthID              string
	AuthIndex           string
	AuthType            string
	Source              string
	ReasoningEffort     string
	ServiceTier         string
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CachedTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
	LatencyMS           int64
	TTFTMS              int64
	Failed              bool
	FailStatusCode      int
	FailSummary         string
	ResponseHeadersJSON string
}

func (s *Store) InsertEvents(ctx context.Context, events []Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `insert or ignore into usage_events (
		event_hash, timestamp_ms, provider, executor_type, model, api_key_hash, auth_id, auth_index, auth_type, source,
		reasoning_effort, service_tier, input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_read_tokens,
		cache_creation_tokens, total_tokens, latency_ms, ttft_ms, failed, fail_status_code, fail_summary,
		response_headers_json, created_at_ms
	) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	inserted := 0
	now := time.Now().UnixMilli()
	for _, event := range events {
		result, err := stmt.ExecContext(ctx,
			event.Hash, event.TimestampMS, event.Provider, event.ExecutorType, event.Model, event.APIKeyHash, event.AuthID,
			event.AuthIndex, event.AuthType, event.Source, event.ReasoningEffort, event.ServiceTier, event.InputTokens,
			event.OutputTokens, event.ReasoningTokens, event.CachedTokens, event.CacheReadTokens, event.CacheCreationTokens,
			event.TotalTokens, event.LatencyMS, event.TTFTMS, boolInt(event.Failed), nullableInt(event.FailStatusCode),
			nullableString(event.FailSummary), nullableString(event.ResponseHeadersJSON), now,
		)
		if err != nil {
			return 0, fmt.Errorf("insert usage event: %w", err)
		}
		if rows, _ := result.RowsAffected(); rows > 0 {
			inserted += int(rows)
			if event.Failed && event.AuthID != "" {
				if err := upsertFailureCandidateTx(ctx, tx, event, now); err != nil {
					return 0, err
				}
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) EventCount(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `select count(*) from usage_events`).Scan(&count)
	return count, err
}

func (s *Store) LastEventAt(ctx context.Context) (int64, error) {
	var timestamp sql.NullInt64
	err := s.db.QueryRowContext(ctx, `select max(timestamp_ms) from usage_events`).Scan(&timestamp)
	if err != nil {
		return 0, err
	}
	return timestamp.Int64, nil
}
