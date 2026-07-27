package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type ActionCandidate struct {
	ID              int64  `json:"id"`
	ActionType      string `json:"action_type"`
	Status          string `json:"status"`
	Provider        string `json:"provider"`
	AuthFileName    string `json:"auth_file_name"`
	AuthIndex       string `json:"auth_index"`
	AccountSnapshot string `json:"account_snapshot"`
	AuthLabel       string `json:"auth_label"`
	ReasonCode      string `json:"reason_code"`
	Reason          string `json:"reason"`
	LastError       string `json:"last_error"`
	TriggeredAtMS   int64  `json:"triggered_at_ms"`
}

func (s *Store) UpsertFailureCandidate(ctx context.Context, event Event) error {
	if !event.Failed || event.AuthID == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := upsertFailureCandidateTx(ctx, tx, event, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertFailureCandidateTx(ctx context.Context, tx *sql.Tx, event Event, now int64) error {
	_, err := tx.ExecContext(ctx, `insert into account_action_candidates(action_type,status,provider,auth_file_name,auth_index,account_snapshot,auth_label,reason_code,reason,last_error,first_seen_at_ms,last_seen_at_ms,created_at_ms,updated_at_ms)
		values('review','pending',?,?,?,?,?,'request_failed',?,?,?,?,?,?)
		on conflict do update set last_seen_at_ms=excluded.last_seen_at_ms,hit_count=account_action_candidates.hit_count+1,last_error=excluded.last_error,updated_at_ms=excluded.updated_at_ms`,
		event.Provider, event.AuthID, event.AuthIndex, event.AuthID, event.AuthID, event.FailSummary, event.FailSummary, event.TimestampMS, event.TimestampMS, now, now)
	return err
}

func (s *Store) Candidates(ctx context.Context, status string, limit int) ([]ActionCandidate, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	query := `select id,action_type,status,coalesce(provider,''),auth_file_name,coalesce(auth_index,''),coalesce(account_snapshot,''),coalesce(auth_label,''),coalesce(reason_code,''),coalesce(reason,''),coalesce(last_error,''),last_seen_at_ms from account_action_candidates`
	args := []any{}
	if status != "" {
		query += ` where status=?`
		args = append(args, status)
	}
	query += ` order by last_seen_at_ms desc limit ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ActionCandidate{}
	for rows.Next() {
		var row ActionCandidate
		if err := rows.Scan(&row.ID, &row.ActionType, &row.Status, &row.Provider, &row.AuthFileName, &row.AuthIndex, &row.AccountSnapshot, &row.AuthLabel, &row.ReasonCode, &row.Reason, &row.LastError, &row.TriggeredAtMS); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *Store) Candidate(ctx context.Context, id int64) (ActionCandidate, error) {
	items, err := s.Candidates(ctx, "", 500)
	if err != nil {
		return ActionCandidate{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return ActionCandidate{}, sql.ErrNoRows
}

func (s *Store) ResolveCandidate(ctx context.Context, id int64, action string) error {
	if id < 1 {
		return fmt.Errorf("invalid candidate id")
	}
	status := map[string]string{"ignore": "ignored", "resolve": "resolved", "enable": "resolved", "delete": "deleted"}[strings.ToLower(action)]
	if status == "" {
		return fmt.Errorf("unsupported action")
	}
	result, err := s.db.ExecContext(ctx, `update account_action_candidates set status=?,updated_at_ms=? where id=? and status='pending'`, status, time.Now().UnixMilli(), id)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
