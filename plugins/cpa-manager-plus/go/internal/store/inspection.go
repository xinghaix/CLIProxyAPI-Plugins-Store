package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type InspectionRun struct {
	ID            int64  `json:"id"`
	TriggerType   string `json:"triggerType"`
	TriggerKey    string `json:"triggerKey,omitempty"`
	Status        string `json:"status"`
	StartedAtMS   int64  `json:"startedAtMs"`
	FinishedAtMS  *int64 `json:"finishedAtMs,omitempty"`
	TotalFiles    int64  `json:"totalFiles"`
	ProbeSetCount int64  `json:"probeSetCount"`
	SampledCount  int64  `json:"sampledCount"`
	DisabledCount int64  `json:"disabledCount"`
	EnabledCount  int64  `json:"enabledCount"`
	DeleteCount   int64  `json:"deleteCount"`
	DisableCount  int64  `json:"disableCount"`
	EnableCount   int64  `json:"enableCount"`
	ReauthCount   int64  `json:"reauthCount"`
	KeepCount     int64  `json:"keepCount"`
	Error         string `json:"error,omitempty"`
	SettingsJSON  string `json:"settings,omitempty"`
}

type InspectionResult struct {
	ID                  int64    `json:"id"`
	RunID               int64    `json:"runId"`
	AccountKey          string   `json:"accountKey"`
	FileName            string   `json:"fileName"`
	DisplayAccount      string   `json:"displayAccount"`
	AuthIndex           string   `json:"authIndex,omitempty"`
	AccountID           string   `json:"accountId,omitempty"`
	Provider            string   `json:"provider"`
	Disabled            bool     `json:"disabled"`
	Status              string   `json:"status,omitempty"`
	State               string   `json:"state,omitempty"`
	Action              string   `json:"action"`
	ActionReason        string   `json:"actionReason"`
	ActionStatus        string   `json:"actionStatus"`
	ExecutedAction      string   `json:"executedAction,omitempty"`
	ActionError         string   `json:"actionError,omitempty"`
	StatusCode          *int     `json:"statusCode,omitempty"`
	UsedPercent         *float64 `json:"usedPercent,omitempty"`
	IsQuota             bool     `json:"isQuota"`
	AutoRecoverEligible bool     `json:"autoRecoverEligible"`
	PlanType            string   `json:"planType,omitempty"`
	QuotaWindows        any      `json:"quotaWindows,omitempty"`
	Error               string   `json:"error,omitempty"`
	ErrorKind           string   `json:"errorKind,omitempty"`
	ErrorDetail         string   `json:"errorDetail,omitempty"`
}

type InspectionLog struct {
	ID          int64  `json:"id"`
	RunID       int64  `json:"runId"`
	Level       string `json:"level"`
	Message     string `json:"message"`
	Detail      any    `json:"detail,omitempty"`
	CreatedAtMS int64  `json:"createdAtMs"`
}

type InspectionAccount struct {
	Key, FileName, DisplayName, Provider, Status string
	AuthIndex, AccountID                         string
	Disabled                                     bool
}

type InspectionDisableOwnership struct {
	FileName, Provider, AuthIndex, AccountID string
	DisabledAtMS                             int64
}

func (s *Store) StartInspection(ctx context.Context, trigger string) (InspectionRun, error) {
	return s.StartInspectionRun(ctx, trigger, "", "{}")
}

func (s *Store) StartInspectionRun(ctx context.Context, trigger, triggerKey, settingsJSON string) (InspectionRun, error) {
	now := time.Now().UnixMilli()
	result, err := s.db.ExecContext(ctx, `insert into codex_inspection_runs(trigger_type,trigger_key,status,started_at_ms,total_files,settings_json,created_at_ms,updated_at_ms) values(?,?,?,?,?,?,?,?)`, trigger, nullText(triggerKey), "running", now, 0, settingsJSON, now, now)
	if err != nil {
		return InspectionRun{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return InspectionRun{}, err
	}
	return InspectionRun{ID: id, TriggerType: trigger, TriggerKey: triggerKey, Status: "running", StartedAtMS: now, SettingsJSON: settingsJSON}, nil
}

// StartInspectionWithAccounts remains for compatibility with historical callers.
func (s *Store) StartInspectionWithAccounts(ctx context.Context, trigger string, accounts []InspectionAccount) (InspectionRun, error) {
	run, err := s.StartInspectionRun(ctx, trigger, "", "{}")
	if err != nil {
		return InspectionRun{}, err
	}
	for _, account := range accounts {
		action, reason := "keep", "credential is available"
		if account.Disabled {
			action, reason = "enable", "credential is disabled"
		}
		if account.Status != "" && account.Status != "available" && account.Status != "ok" {
			action, reason = "review", account.Status
		}
		if _, err := s.InsertInspectionResult(ctx, InspectionResult{RunID: run.ID, AccountKey: account.Key, FileName: account.FileName, DisplayAccount: account.DisplayName, AuthIndex: account.AuthIndex, AccountID: account.AccountID, Provider: account.Provider, Disabled: account.Disabled, Status: account.Status, Action: action, ActionReason: reason, ActionStatus: "pending"}); err != nil {
			return InspectionRun{}, err
		}
	}
	run.TotalFiles, run.ProbeSetCount, run.SampledCount = int64(len(accounts)), int64(len(accounts)), int64(len(accounts))
	return s.FinishInspectionRun(ctx, run, "completed", "")
}

func (s *Store) FinishInspectionRun(ctx context.Context, run InspectionRun, status, runErr string) (InspectionRun, error) {
	if run.ID < 1 {
		return InspectionRun{}, fmt.Errorf("invalid inspection run")
	}
	if status != "completed" && status != "failed" && status != "cancelled" && status != "interrupted" {
		return InspectionRun{}, fmt.Errorf("invalid inspection terminal status")
	}
	finished := time.Now().UnixMilli()
	summary, err := s.inspectionSummary(ctx, run.ID)
	if err != nil {
		return InspectionRun{}, err
	}
	if run.TotalFiles == 0 {
		run.TotalFiles = summary.Total
	}
	if run.ProbeSetCount == 0 {
		run.ProbeSetCount = summary.Total
	}
	if run.SampledCount == 0 {
		run.SampledCount = summary.Total
	}
	run.DeleteCount, run.DisableCount, run.EnableCount, run.ReauthCount, run.KeepCount = summary.Delete, summary.Disable, summary.Enable, summary.Reauth, summary.Keep
	run.Status, run.Error, run.FinishedAtMS = status, runErr, &finished
	_, err = s.db.ExecContext(ctx, `update codex_inspection_runs set status=?,finished_at_ms=?,total_files=?,probe_set_count=?,sampled_count=?,disabled_count=?,enabled_count=?,delete_count=?,disable_count=?,enable_count=?,reauth_count=?,keep_count=?,error=?,updated_at_ms=? where id=?`, run.Status, finished, run.TotalFiles, run.ProbeSetCount, run.SampledCount, run.DisabledCount, run.EnabledCount, run.DeleteCount, run.DisableCount, run.EnableCount, run.ReauthCount, run.KeepCount, nullText(run.Error), finished, run.ID)
	if err != nil {
		return InspectionRun{}, err
	}
	return run, nil
}

func (s *Store) UpdateInspectionProgress(ctx context.Context, run InspectionRun) error {
	_, err := s.db.ExecContext(ctx, `update codex_inspection_runs set total_files=?,probe_set_count=?,sampled_count=?,disabled_count=?,enabled_count=?,updated_at_ms=? where id=? and status='running'`, run.TotalFiles, run.ProbeSetCount, run.SampledCount, run.DisabledCount, run.EnabledCount, time.Now().UnixMilli(), run.ID)
	return err
}

func (s *Store) InsertInspectionResult(ctx context.Context, result InspectionResult) (InspectionResult, error) {
	if result.RunID < 1 || result.AccountKey == "" {
		return InspectionResult{}, fmt.Errorf("inspection result requires run and account")
	}
	if result.Action == "" {
		result.Action = "keep"
	}
	if result.ActionStatus == "" {
		result.ActionStatus = "pending"
	}
	quotaJSON := ""
	if result.QuotaWindows != nil {
		if raw, err := json.Marshal(result.QuotaWindows); err == nil {
			quotaJSON = string(raw)
		}
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `insert into codex_inspection_results(run_id,account_key,file_name,display_account,auth_index,account_id,provider,disabled,status,state,action,action_reason,action_status,executed_action,action_error,status_code,used_percent,is_quota,auto_recover_eligible,plan_type,quota_windows_json,error,error_kind,error_detail,created_at_ms) values(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) on conflict(run_id,account_key) do update set file_name=excluded.file_name,display_account=excluded.display_account,auth_index=excluded.auth_index,account_id=excluded.account_id,provider=excluded.provider,disabled=excluded.disabled,status=excluded.status,state=excluded.state,action=excluded.action,action_reason=excluded.action_reason,action_status=excluded.action_status,executed_action=excluded.executed_action,action_error=excluded.action_error,status_code=excluded.status_code,used_percent=excluded.used_percent,is_quota=excluded.is_quota,auto_recover_eligible=excluded.auto_recover_eligible,plan_type=excluded.plan_type,quota_windows_json=excluded.quota_windows_json,error=excluded.error,error_kind=excluded.error_kind,error_detail=excluded.error_detail returning id`, result.RunID, result.AccountKey, result.FileName, result.DisplayAccount, nullText(result.AuthIndex), nullText(result.AccountID), nullText(result.Provider), boolInt(result.Disabled), nullText(result.Status), nullText(result.State), result.Action, nullText(result.ActionReason), result.ActionStatus, nullText(result.ExecutedAction), nullText(result.ActionError), nullInt(result.StatusCode), nullFloat(result.UsedPercent), boolInt(result.IsQuota), boolInt(result.AutoRecoverEligible), nullText(result.PlanType), nullText(quotaJSON), nullText(result.Error), nullText(result.ErrorKind), nullText(result.ErrorDetail), time.Now().UnixMilli()).Scan(&id)
	if err != nil {
		return InspectionResult{}, err
	}
	result.ID = id
	return result, nil
}

func (s *Store) AppendInspectionLog(ctx context.Context, entry InspectionLog) (InspectionLog, error) {
	if entry.RunID < 1 {
		return InspectionLog{}, fmt.Errorf("inspection log requires run")
	}
	if entry.Level == "" {
		entry.Level = "info"
	}
	if entry.CreatedAtMS == 0 {
		entry.CreatedAtMS = time.Now().UnixMilli()
	}
	detail := ""
	if entry.Detail != nil {
		if raw, err := json.Marshal(entry.Detail); err == nil {
			detail = string(raw)
		}
	}
	result, err := s.db.ExecContext(ctx, `insert into codex_inspection_logs(run_id,level,message,detail_json,created_at_ms) values(?,?,?,?,?)`, entry.RunID, entry.Level, entry.Message, nullText(detail), entry.CreatedAtMS)
	if err != nil {
		return InspectionLog{}, err
	}
	entry.ID, _ = result.LastInsertId()
	return entry, nil
}

func (s *Store) InspectionRuns(ctx context.Context, limit int) ([]InspectionRun, error) {
	if limit < 1 || limit > 100 {
		limit = 30
	}
	rows, err := s.db.QueryContext(ctx, `select id,trigger_type,coalesce(trigger_key,''),status,started_at_ms,finished_at_ms,total_files,probe_set_count,sampled_count,disabled_count,enabled_count,delete_count,disable_count,enable_count,reauth_count,keep_count,coalesce(error,''),coalesce(settings_json,'{}') from codex_inspection_runs order by id desc limit ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []InspectionRun{}
	for rows.Next() {
		run, err := scanInspectionRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

func (s *Store) InspectionDetail(ctx context.Context, id int64) (map[string]any, error) {
	row := s.db.QueryRowContext(ctx, `select id,trigger_type,coalesce(trigger_key,''),status,started_at_ms,finished_at_ms,total_files,probe_set_count,sampled_count,disabled_count,enabled_count,delete_count,disable_count,enable_count,reauth_count,keep_count,coalesce(error,''),coalesce(settings_json,'{}') from codex_inspection_runs where id=?`, id)
	run, err := scanInspectionRun(row)
	if err != nil {
		return nil, err
	}
	results, err := s.inspectionResults(ctx, id)
	if err != nil {
		return nil, err
	}
	logs, err := s.inspectionLogs(ctx, id)
	if err != nil {
		return nil, err
	}
	return map[string]any{"run": run, "results": results, "logs": logs}, nil
}

func (s *Store) InspectionResults(ctx context.Context, runID int64) ([]InspectionResult, error) {
	return s.inspectionResults(ctx, runID)
}

func (s *Store) inspectionResults(ctx context.Context, runID int64) ([]InspectionResult, error) {
	rows, err := s.db.QueryContext(ctx, `select id,run_id,account_key,file_name,display_account,coalesce(auth_index,''),coalesce(account_id,''),coalesce(provider,''),disabled,coalesce(status,''),coalesce(state,''),action,coalesce(action_reason,''),coalesce(action_status,''),coalesce(executed_action,''),coalesce(action_error,''),status_code,used_percent,is_quota,auto_recover_eligible,coalesce(plan_type,''),coalesce(quota_windows_json,''),coalesce(error,''),coalesce(error_kind,''),coalesce(error_detail,'') from codex_inspection_results where run_id=? order by id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InspectionResult{}
	for rows.Next() {
		var item InspectionResult
		var disabled, quota, recover int
		var code sql.NullInt64
		var percent sql.NullFloat64
		var quotaJSON string
		if err := rows.Scan(&item.ID, &item.RunID, &item.AccountKey, &item.FileName, &item.DisplayAccount, &item.AuthIndex, &item.AccountID, &item.Provider, &disabled, &item.Status, &item.State, &item.Action, &item.ActionReason, &item.ActionStatus, &item.ExecutedAction, &item.ActionError, &code, &percent, &quota, &recover, &item.PlanType, &quotaJSON, &item.Error, &item.ErrorKind, &item.ErrorDetail); err != nil {
			return nil, err
		}
		item.Disabled, item.IsQuota, item.AutoRecoverEligible = disabled != 0, quota != 0, recover != 0
		if code.Valid {
			value := int(code.Int64)
			item.StatusCode = &value
		}
		if percent.Valid {
			value := percent.Float64
			item.UsedPercent = &value
		}
		if quotaJSON != "" {
			_ = json.Unmarshal([]byte(quotaJSON), &item.QuotaWindows)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) inspectionLogs(ctx context.Context, runID int64) ([]InspectionLog, error) {
	rows, err := s.db.QueryContext(ctx, `select id,run_id,level,message,coalesce(detail_json,''),created_at_ms from codex_inspection_logs where run_id=? order by id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs := []InspectionLog{}
	for rows.Next() {
		var entry InspectionLog
		var detail string
		if err := rows.Scan(&entry.ID, &entry.RunID, &entry.Level, &entry.Message, &detail, &entry.CreatedAtMS); err != nil {
			return nil, err
		}
		if detail != "" {
			_ = json.Unmarshal([]byte(detail), &entry.Detail)
		}
		logs = append(logs, entry)
	}
	return logs, rows.Err()
}

func (s *Store) ExecuteInspectionActions(ctx context.Context, id int64, resultIDs []int64) (map[string]any, error) {
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

func (s *Store) UpdateInspectionAction(ctx context.Context, resultID int64, action, status, actionErr string) error {
	_, err := s.db.ExecContext(ctx, `update codex_inspection_results set executed_action=?,action_status=?,action_error=? where id=?`, nullText(action), status, nullText(actionErr), resultID)
	return err
}

func (s *Store) DisableOwnership(ctx context.Context, fileName string) (InspectionDisableOwnership, bool, error) {
	var item InspectionDisableOwnership
	err := s.db.QueryRowContext(ctx, `select file_name,coalesce(provider,''),coalesce(auth_index,''),coalesce(account_id,''),disabled_at_ms from inspection_disable_ownership where file_name=?`, fileName).Scan(&item.FileName, &item.Provider, &item.AuthIndex, &item.AccountID, &item.DisabledAtMS)
	if err == sql.ErrNoRows {
		return InspectionDisableOwnership{}, false, nil
	}
	return item, err == nil, err
}

func (s *Store) PutDisableOwnership(ctx context.Context, item InspectionDisableOwnership) error {
	if item.FileName == "" {
		return fmt.Errorf("ownership requires file name")
	}
	if item.DisabledAtMS == 0 {
		item.DisabledAtMS = time.Now().UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `insert into inspection_disable_ownership(file_name,provider,auth_index,account_id,disabled_at_ms,updated_at_ms) values(?,?,?,?,?,?) on conflict(file_name) do update set provider=excluded.provider,auth_index=excluded.auth_index,account_id=excluded.account_id,disabled_at_ms=excluded.disabled_at_ms,updated_at_ms=excluded.updated_at_ms`, item.FileName, nullText(item.Provider), nullText(item.AuthIndex), nullText(item.AccountID), item.DisabledAtMS, time.Now().UnixMilli())
	return err
}

func (s *Store) DeleteDisableOwnership(ctx context.Context, fileName string) error {
	_, err := s.db.ExecContext(ctx, `delete from inspection_disable_ownership where file_name=?`, fileName)
	return err
}

type inspectionSummary struct{ Total, Delete, Disable, Enable, Reauth, Keep int64 }

func (s *Store) inspectionSummary(ctx context.Context, runID int64) (inspectionSummary, error) {
	var value inspectionSummary
	err := s.db.QueryRowContext(ctx, `select count(*),coalesce(sum(case when action='delete' then 1 else 0 end),0),coalesce(sum(case when action='disable' then 1 else 0 end),0),coalesce(sum(case when action='enable' then 1 else 0 end),0),coalesce(sum(case when action='reauth' then 1 else 0 end),0),coalesce(sum(case when action='keep' then 1 else 0 end),0) from codex_inspection_results where run_id=?`, runID).Scan(&value.Total, &value.Delete, &value.Disable, &value.Enable, &value.Reauth, &value.Keep)
	return value, err
}

type scanner interface{ Scan(...any) error }

func scanInspectionRun(row scanner) (InspectionRun, error) {
	var run InspectionRun
	var finished sql.NullInt64
	err := row.Scan(&run.ID, &run.TriggerType, &run.TriggerKey, &run.Status, &run.StartedAtMS, &finished, &run.TotalFiles, &run.ProbeSetCount, &run.SampledCount, &run.DisabledCount, &run.EnabledCount, &run.DeleteCount, &run.DisableCount, &run.EnableCount, &run.ReauthCount, &run.KeepCount, &run.Error, &run.SettingsJSON)
	if err != nil {
		return InspectionRun{}, err
	}
	if finished.Valid {
		value := finished.Int64
		run.FinishedAtMS = &value
	}
	return run, nil
}
func nullText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
func nullInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}
