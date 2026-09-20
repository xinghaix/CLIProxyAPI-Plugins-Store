package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

const responseObservationTTL = 7 * 24 * time.Hour
const maxOrphanResponseObservations = 10_000

func (s *Store) ensureResponseObservationSchema(ctx context.Context) error {
	for _, statement := range []string{
		`create index if not exists idx_usage_events_response_correlation on usage_events(response_correlation_key)`,
		`create table if not exists response_observations (
   correlation_key text primary key,
   model text not null,
   evidence_id text not null,
   ambiguous integer not null default 0,
   referenced integer not null default 0,
   updated_at_ms integer not null
  )`,
		`create index if not exists idx_response_observations_orphans on response_observations(updated_at_ms desc, correlation_key) where referenced = 0`,
	} {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// RecordResponseObservation stores metadata only: it never inserts or changes usage.
// key is the opaque, hashed correlation key produced by responsemodel.CorrelationKey.
// A second evidence identity or conflicting nonblank model permanently quarantines
// the key. Replays cannot clear quarantine, including after a process restart.
func (s *Store) RecordResponseObservation(ctx context.Context, key, model, evidenceID string, ambiguous bool) error {
	key, model, evidenceID = strings.TrimSpace(key), strings.TrimSpace(model), strings.TrimSpace(evidenceID)
	if key == "" {
		return nil
	}
	// Missing evidence cannot safely establish a unique upstream response.
	if evidenceID == "" {
		ambiguous = true
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	ambiguous = ambiguous || s.responseObservationsSuppressed.Load()
	_, err = tx.ExecContext(ctx, `insert into response_observations(correlation_key,model,evidence_id,ambiguous,updated_at_ms,referenced)
 values(?,?,?,?,?,exists(select 1 from usage_events where response_correlation_key=?)) on conflict(correlation_key) do update set
 ambiguous = response_observations.ambiguous or excluded.ambiguous
  or response_observations.evidence_id <> excluded.evidence_id
  or (response_observations.model <> '' and excluded.model <> '' and response_observations.model <> excluded.model),
 model = case when response_observations.model = '' then excluded.model else response_observations.model end,
 referenced = excluded.referenced,
 updated_at_ms = excluded.updated_at_ms`, key, model, evidenceID, boolInt(ambiguous), now, key)
	if err != nil {
		return err
	}
	if err := cleanupResponseObservations(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit()
}

// Only orphan metadata can expire or be evicted. In particular, quarantines still
// referenced by usage must survive forever. Work/deletions per write are bounded;
// the cap retains the most recent orphan observations for out-of-order usage.
func cleanupResponseObservations(ctx context.Context, tx *sql.Tx, now int64) error {
	_, err := tx.ExecContext(ctx, `delete from response_observations where correlation_key in (
 select correlation_key from response_observations o where referenced = 0 and updated_at_ms < ?
 and not exists (select 1 from usage_events u where u.response_correlation_key = o.correlation_key)
 order by updated_at_ms limit 256)`, now-responseObservationTTL.Milliseconds())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `delete from response_observations where correlation_key in (
 select correlation_key from response_observations o
 where referenced = 0 and not exists (select 1 from usage_events u where u.response_correlation_key = o.correlation_key)
 order by updated_at_ms desc, correlation_key limit 256 offset ?)`, maxOrphanResponseObservations)
	return err
}

type responseModelMetadata struct {
	Model, Host, Observed, Source string
	Conflict                      bool
}

func resolveResponseModel(host, observed string, present, ambiguous bool, usageCount int64, failed bool) responseModelMetadata {
	result := responseModelMetadata{Host: strings.TrimSpace(host)}
	result.Model = result.Host
	if result.Host != "" {
		result.Source = "host"
	}
	if !present || failed {
		return result
	}
	if ambiguous || usageCount != 1 {
		result.Conflict = true
		return result
	}
	result.Observed = strings.TrimSpace(observed)
	if result.Observed == "" {
		return result
	}
	if result.Host == "" {
		result.Model, result.Source = result.Observed, "observer"
	} else if result.Host == result.Observed {
		result.Source = "confirmed"
	} else {
		result.Conflict = true
	}
	return result
}

// SuppressResponseObservations immediately disables observer attribution for this
// Store lifetime, without waiting for a database lock. A worker must also call
// QuarantineResponseObservations to persist revocation before restarting.
func (s *Store) SuppressResponseObservations() {
	s.responseObservationsSuppressed.Store(true)
}

// QuarantineResponseObservations revokes all derived metadata after an observer
// delivery gap. Explicit host metadata and all usage/billing data remain untouched.
func (s *Store) QuarantineResponseObservations(ctx context.Context) error {
	s.SuppressResponseObservations()
	_, err := s.db.ExecContext(ctx, `update response_observations set ambiguous=1 where ambiguous=0`)
	return err
}
