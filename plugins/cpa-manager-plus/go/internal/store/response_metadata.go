package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

func (s *Store) ensureResponseObservationColumn(ctx context.Context, name, definition string) error {
	rows, err := s.db.QueryContext(ctx, `pragma table_info(response_observations)`)
	if err != nil {
		return err
	}
	exists := false
	for rows.Next() {
		var cid, notNull, primaryKey int
		var columnName, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &columnName, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		if columnName == name {
			exists = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	if exists {
		return nil
	}
	_, err = s.db.ExecContext(ctx, "alter table response_observations add column "+name+" "+definition)
	return err
}

// ResponseMetadata is the normalized observation stored for one correlation key.
// It intentionally contains no response body.
type ResponseMetadata struct {
	Key, Model, ServiceTier, EvidenceID string
	TierAmbiguous, Ambiguous            bool
}

// RecordResponseMetadata stores normalized response metadata only, never response bodies.
// Conflicting nonblank model, evidence, or service-tier values quarantine that field.
// The SQL conflict rule matches responsemodel.mergeText: blank fills, mismatch sticks.
func (s *Store) RecordResponseMetadata(ctx context.Context, meta ResponseMetadata) error {
	key := strings.TrimSpace(meta.Key)
	model := strings.TrimSpace(meta.Model)
	serviceTier := strings.TrimSpace(meta.ServiceTier)
	evidenceID := strings.TrimSpace(meta.EvidenceID)
	tierAmbiguous := meta.TierAmbiguous
	ambiguous := meta.Ambiguous
	if key == "" {
		return nil
	}
	if evidenceID == "" && serviceTier == "" && !tierAmbiguous {
		ambiguous = true
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	suppressed := s.responseObservationsSuppressed.Load()
	ambiguous = ambiguous || suppressed
	tierAmbiguous = tierAmbiguous || ambiguous || suppressed
	if tierAmbiguous {
		serviceTier = ""
	}
	_, err = tx.ExecContext(ctx, `insert into response_observations(correlation_key,model,evidence_id,service_tier,service_tier_ambiguous,ambiguous,updated_at_ms,referenced)
 values(?,?,?,?,?,?,?,exists(select 1 from usage_events where response_correlation_key=?)) on conflict(correlation_key) do update set
 ambiguous = response_observations.ambiguous or excluded.ambiguous
  or (response_observations.evidence_id <> '' and excluded.evidence_id <> '' and response_observations.evidence_id <> excluded.evidence_id)
  or (response_observations.model <> '' and excluded.model <> '' and response_observations.model <> excluded.model),
 service_tier_ambiguous = response_observations.service_tier_ambiguous or excluded.service_tier_ambiguous
  or (response_observations.service_tier <> '' and excluded.service_tier <> '' and response_observations.service_tier <> excluded.service_tier),
 model = case when response_observations.model = '' then excluded.model else response_observations.model end,
 evidence_id = case when response_observations.evidence_id = '' then excluded.evidence_id else response_observations.evidence_id end,
 service_tier = case when response_observations.service_tier = '' then excluded.service_tier else response_observations.service_tier end,
 referenced = excluded.referenced,
 updated_at_ms = excluded.updated_at_ms`, key, model, evidenceID, serviceTier, boolInt(tierAmbiguous), boolInt(ambiguous), now, key)
	if err != nil {
		return err
	}
	if err := cleanupResponseObservations(ctx, tx, now); err != nil {
		return err
	}
	return tx.Commit()
}
