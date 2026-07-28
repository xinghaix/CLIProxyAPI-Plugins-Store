package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SyncUpsertResult struct {
	Imported        int
	ProtectedManual int
	Skipped         int
}

func (s *Store) UpsertSyncedPrices(ctx context.Context, prices map[string]Price) (SyncUpsertResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncUpsertResult{}, err
	}
	defer tx.Rollback()
	find, err := tx.PrepareContext(ctx, `select coalesce(source, '') from model_prices where model = ?`)
	if err != nil {
		return SyncUpsertResult{}, err
	}
	defer find.Close()
	upsert, err := tx.PrepareContext(ctx, `insert into model_prices(model,prompt,completion,cache,cache_read,cache_creation,source,source_model_id,synced_at_ms,updated_at_ms) values(?,?,?,?,?,?,?,?,?,?) on conflict(model) do update set prompt=excluded.prompt,completion=excluded.completion,cache=excluded.cache,cache_read=excluded.cache_read,cache_creation=excluded.cache_creation,source=excluded.source,source_model_id=excluded.source_model_id,synced_at_ms=excluded.synced_at_ms,updated_at_ms=excluded.updated_at_ms`)
	if err != nil {
		return SyncUpsertResult{}, err
	}
	defer upsert.Close()
	result := SyncUpsertResult{}
	now := time.Now().UnixMilli()
	for model, price := range prices {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 256 || !finiteNonNegative(price.Prompt, price.Completion, price.Cache, price.CacheRead, price.CacheCreation) {
			result.Skipped++
			continue
		}
		var existingSource string
		err := find.QueryRowContext(ctx, model).Scan(&existingSource)
		if err != nil && err != sql.ErrNoRows {
			return SyncUpsertResult{}, err
		}
		if strings.EqualFold(existingSource, "manual") {
			result.ProtectedManual++
			continue
		}
		if price.Source == "" {
			return SyncUpsertResult{}, fmt.Errorf("synced price %q has no source", model)
		}
		if price.SyncedAtMS == 0 {
			price.SyncedAtMS = now
		}
		if _, err := upsert.ExecContext(ctx, model, price.Prompt, price.Completion, price.Cache, price.CacheRead, price.CacheCreation, price.Source, price.SourceModelID, price.SyncedAtMS, now); err != nil {
			return SyncUpsertResult{}, err
		}
		result.Imported++
	}
	if err := tx.Commit(); err != nil {
		return SyncUpsertResult{}, err
	}
	return result, nil
}

func (s *Store) PriceSyncTargets(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `select model from model_prices union select distinct model from usage_events where model <> '' order by model`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	models := []string{}
	for rows.Next() {
		var model string
		if err := rows.Scan(&model); err != nil {
			return nil, err
		}
		if model = strings.TrimSpace(model); model != "" {
			models = append(models, model)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(models)
	return models, nil
}
