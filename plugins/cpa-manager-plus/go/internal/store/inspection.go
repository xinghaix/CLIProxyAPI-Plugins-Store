package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type InspectionRun struct {
	ID           int64  `json:"id"`
	TriggerType  string `json:"triggerType"`
	Status       string `json:"status"`
	StartedAtMS  int64  `json:"startedAtMs"`
	FinishedAtMS *int64 `json:"finishedAtMs,omitempty"`
	TotalFiles   int64  `json:"totalFiles"`
	Error        string `json:"error,omitempty"`
}

type InspectionResult struct {
	ID             int64  `json:"id"`
	AccountKey     string `json:"accountKey"`
	FileName       string `json:"fileName"`
	DisplayAccount string `json:"displayAccount"`
	Provider       string `json:"provider"`
	Action         string `json:"action"`
	ActionReason   string `json:"actionReason"`
	ActionStatus   string `json:"actionStatus"`
}

type InspectionAccount struct {
	Key, FileName, DisplayName, Provider, Status string
	Disabled                                     bool
}

func (s *Store) StartInspection(ctx context.Context, trigger string) (InspectionRun, error) {
	return s.StartInspectionWithAccounts(ctx, trigger, nil)
}

func (s *Store) StartInspectionWithAccounts(ctx context.Context, trigger string, accounts []InspectionAccount) (InspectionRun, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return InspectionRun{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `insert into codex_inspection_runs(trigger_type,status,started_at_ms,finished_at_ms,total_files,created_at_ms,updated_at_ms) values(?,?,?,?,?,?,?)`, trigger, "completed", now, now, len(accounts), now, now)
	if err != nil {
		return InspectionRun{}, err
	}
	id, _ := result.LastInsertId()
	for _, account := range accounts {
		action, reason := "keep", "credential is available"
		if account.Disabled {
			action, reason = "enable", "credential is disabled"
		}
		if account.Status != "" && account.Status != "available" && account.Status != "ok" {
			action, reason = "review", account.Status
		}
		if _, err := tx.ExecContext(ctx, `insert into codex_inspection_results(run_id,account_key,file_name,display_account,provider,disabled,status,action,action_reason,action_status,created_at_ms) values(?,?,?,?,?,?,?,?,?,?,?)`, id, account.Key, account.FileName, account.DisplayName, account.Provider, boolInt(account.Disabled), account.Status, action, reason, "pending", now); err != nil {
			return InspectionRun{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return InspectionRun{}, err
	}
	finished := now
	return InspectionRun{ID: id, TriggerType: trigger, Status: "completed", StartedAtMS: now, FinishedAtMS: &finished, TotalFiles: int64(len(accounts))}, nil
}

func (s *Store) InspectionRuns(ctx context.Context, limit int) ([]InspectionRun, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `select id,trigger_type,status,started_at_ms,finished_at_ms,total_files,coalesce(error,'') from codex_inspection_runs order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InspectionRun{}
	for rows.Next() {
		var r InspectionRun
		var finished sql.NullInt64
		if err := rows.Scan(&r.ID, &r.TriggerType, &r.Status, &r.StartedAtMS, &finished, &r.TotalFiles, &r.Error); err != nil {
			return nil, err
		}
		if finished.Valid {
			v := finished.Int64
			r.FinishedAtMS = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) InspectionDetail(ctx context.Context, id int64) (map[string]any, error) {
	var run InspectionRun
	var finished sql.NullInt64
	err := s.db.QueryRowContext(ctx, `select id,trigger_type,status,started_at_ms,finished_at_ms,total_files,coalesce(error,'') from codex_inspection_runs where id=?`, id).Scan(&run.ID, &run.TriggerType, &run.Status, &run.StartedAtMS, &finished, &run.TotalFiles, &run.Error)
	if err != nil {
		return nil, err
	}
	if finished.Valid {
		v := finished.Int64
		run.FinishedAtMS = &v
	}
	results := []InspectionResult{}
	rows, err := s.db.QueryContext(ctx, `select id,account_key,file_name,display_account,coalesce(provider,''),action,coalesce(action_reason,''),coalesce(action_status,'') from codex_inspection_results where run_id=? order by id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var r InspectionResult
		if err := rows.Scan(&r.ID, &r.AccountKey, &r.FileName, &r.DisplayAccount, &r.Provider, &r.Action, &r.ActionReason, &r.ActionStatus); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return map[string]any{"run": run, "results": results, "logs": []any{}}, rows.Err()
}

func (s *Store) ExecuteInspectionActions(ctx context.Context, id int64, resultIDs []int64) (map[string]any, error) {
	if id < 1 {
		return nil, fmt.Errorf("invalid run id")
	}
	outcomes := make([]map[string]any, 0, len(resultIDs))
	for _, resultID := range resultIDs {
		result, err := s.db.ExecContext(ctx, `update codex_inspection_results set action_status='acknowledged' where run_id=? and id=?`, id, resultID)
		if err != nil {
			return nil, err
		}
		changed, _ := result.RowsAffected()
		outcomes = append(outcomes, map[string]any{"id": resultID, "success": changed == 1})
	}
	detail, err := s.InspectionDetail(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"detail": detail, "outcomes": outcomes}, nil
}
