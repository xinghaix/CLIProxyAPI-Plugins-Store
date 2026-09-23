package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

const windowKeeperSettingsKey = "window_keeper_settings_v1"

func (s *Store) ensureWindowKeeperSchema(ctx context.Context) error {
	statements := []string{
		`create table if not exists window_keeper_accounts (
			auth_id text primary key,
			auth_index text,
			chatgpt_account_id text,
			email text,
			display_name text,
			plan_type text,
			plan_group text,
			shape_mismatch text,
			disabled integer not null default 0,
			unavailable integer not null default 0,
			override_json text not null default '{}',
			pause_reason text,
			not_before_ms integer not null default 0,
			lease_owner text,
			lease_until_ms integer,
			updated_at_ms integer not null
		)`,
		`create table if not exists window_keeper_windows (
			account_id text not null,
			limit_id text not null,
			slot text not null,
			period_seconds integer not null,
			kind text not null,
			starts_at_ms integer,
			ends_at_ms integer,
			last_blocked_ends_ms integer,
			time_source text,
			used_percent real,
			gating integer not null default 0,
			phase text not null,
			seen_blocked integer not null default 0,
			hypothesis text,
			absent integer not null default 0,
			primary key (account_id, limit_id, slot, period_seconds)
		)`,
		`create table if not exists window_keeper_attempts (
			id integer primary key autoincrement,
			account_id text not null,
			generation_key text not null,
			started_at_ms integer not null,
			finished_at_ms integer,
			status text not null,
			http_status integer,
			error_kind text,
			attempt_no integer not null,
			not_before_ms integer,
			output_excerpt text,
			response_id text
		)`,
		`create unique index if not exists idx_window_keeper_attempts_success on window_keeper_attempts(account_id, generation_key) where status = 'succeeded'`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadWindowKeeperSettings(ctx context.Context) (windowkeeper.Settings, bool, error) {
	raw, ok, err := s.Setting(ctx, windowKeeperSettingsKey)
	if errors.Is(err, sql.ErrNoRows) || !ok {
		return windowkeeper.Settings{}, false, nil
	}
	if err != nil {
		return windowkeeper.Settings{}, false, err
	}
	var settings windowkeeper.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return windowkeeper.Settings{}, false, err
	}
	return settings, true, nil
}

func (s *Store) SaveWindowKeeperSettings(ctx context.Context, settings windowkeeper.Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.PutSetting(ctx, windowKeeperSettingsKey, raw)
}

func (s *Store) TouchWindowKeeperAccount(ctx context.Context, account windowkeeper.Account) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `insert into window_keeper_accounts(auth_id, auth_index, chatgpt_account_id, email, display_name, disabled, unavailable, updated_at_ms)
		values(?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(auth_id) do update set auth_index = excluded.auth_index, chatgpt_account_id = excluded.chatgpt_account_id,
			email = excluded.email, display_name = excluded.display_name, disabled = excluded.disabled, unavailable = excluded.unavailable, updated_at_ms = excluded.updated_at_ms`,
		account.AuthID, account.AuthIndex, account.AccountID, account.Email, account.Name, boolToInt(account.Disabled), boolToInt(account.Unavailable), now)
	return err
}

func (s *Store) SetWindowKeeperPlan(ctx context.Context, authID, plan, group, mismatch string) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set plan_type = ?, plan_group = ?, shape_mismatch = ?, updated_at_ms = ? where auth_id = ?", plan, group, mismatch, time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) SetWindowKeeperOverride(ctx context.Context, authID, raw string) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set override_json = ?, updated_at_ms = ? where auth_id = ?", raw, time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) SetWindowKeeperPause(ctx context.Context, authID, reason string) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set pause_reason = ?, updated_at_ms = ? where auth_id = ?", reason, time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) SetWindowKeeperNotBefore(ctx context.Context, authID string, when time.Time) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set not_before_ms = ?, updated_at_ms = ? where auth_id = ?", when.UnixMilli(), time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) ListWindowKeeperAccounts(ctx context.Context) ([]windowkeeper.Account, error) {
	rows, err := s.db.QueryContext(ctx, `select auth_id, ifnull(auth_index, ''), ifnull(chatgpt_account_id, ''), ifnull(email, ''), ifnull(display_name, ''), ifnull(plan_type, ''), ifnull(plan_group, ''), ifnull(shape_mismatch, ''), disabled, unavailable, ifnull(override_json, '{}'), ifnull(pause_reason, ''), not_before_ms from window_keeper_accounts order by email, auth_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []windowkeeper.Account
	for rows.Next() {
		var account windowkeeper.Account
		var disabled, unavailable int
		var notBefore int64
		if err := rows.Scan(&account.AuthID, &account.AuthIndex, &account.AccountID, &account.Email, &account.Name, &account.PlanType, &account.PlanGroup, &account.ShapeMismatch, &disabled, &unavailable, &account.OverrideJSON, &account.PauseReason, &notBefore); err != nil {
			return nil, err
		}
		account.Disabled = disabled == 1
		account.Unavailable = unavailable == 1
		if notBefore > 0 {
			account.NotBefore = time.UnixMilli(notBefore)
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

func (s *Store) SaveWindowKeeperWindows(ctx context.Context, authID string, states []windowkeeper.State) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, state := range states {
		if _, err := tx.ExecContext(ctx, `insert into window_keeper_windows(account_id, limit_id, slot, period_seconds, kind, starts_at_ms, ends_at_ms, last_blocked_ends_ms, time_source, used_percent, gating, phase, seen_blocked, hypothesis, absent)
			values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			on conflict(account_id, limit_id, slot, period_seconds) do update set kind = excluded.kind, starts_at_ms = excluded.starts_at_ms, ends_at_ms = excluded.ends_at_ms, last_blocked_ends_ms = excluded.last_blocked_ends_ms, time_source = excluded.time_source, used_percent = excluded.used_percent, gating = excluded.gating, phase = excluded.phase, seen_blocked = excluded.seen_blocked, hypothesis = excluded.hypothesis, absent = excluded.absent`,
			authID, state.LimitID, state.Slot, state.PeriodSeconds, state.Kind, toMillis(state.StartsAt), toMillis(state.EndsAt), toMillis(state.LastBlockedEnd), state.TimeSource, state.UsedPercent, boolToInt(state.Gating), state.Phase, boolToInt(state.SeenBlocked), state.Hypothesis, boolToInt(state.Absent)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadWindowKeeperWindows(ctx context.Context, authID string) ([]windowkeeper.State, error) {
	rows, err := s.db.QueryContext(ctx, `select limit_id, slot, period_seconds, kind, starts_at_ms, ends_at_ms, last_blocked_ends_ms, time_source, used_percent, gating, phase, seen_blocked, hypothesis, absent from window_keeper_windows where account_id = ?`, authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []windowkeeper.State
	for rows.Next() {
		var state windowkeeper.State
		var start, end, lastBlocked sql.NullInt64
		var gating, seen, absent int
		if err := rows.Scan(&state.LimitID, &state.Slot, &state.PeriodSeconds, &state.Kind, &start, &end, &lastBlocked, &state.TimeSource, &state.UsedPercent, &gating, &state.Phase, &seen, &state.Hypothesis, &absent); err != nil {
			return nil, err
		}
		state.StartsAt = fromNullMillis(start)
		state.EndsAt = fromNullMillis(end)
		state.LastBlockedEnd = fromNullMillis(lastBlocked)
		state.Gating = gating == 1
		state.SeenBlocked = seen == 1
		state.Absent = absent == 1
		out = append(out, state)
	}
	return out, rows.Err()
}

func (s *Store) ClaimWindowKeeper(ctx context.Context, authID, owner string, now, until time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, "update window_keeper_accounts set lease_owner = ?, lease_until_ms = ? where auth_id = ? and (lease_until_ms is null or lease_until_ms < ?)", owner, until.UnixMilli(), authID, now.UnixMilli())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Store) ReleaseWindowKeeper(ctx context.Context, authID, owner string) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set lease_owner = null, lease_until_ms = null where auth_id = ? and lease_owner = ?", authID, owner)
	return err
}

func (s *Store) ReleaseAllWindowKeeper(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set lease_owner = null, lease_until_ms = null")
	return err
}

func (s *Store) RequeueStartedWindowKeeper(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_accounts set not_before_ms = ? where auth_id in (select account_id from window_keeper_attempts where status = 'started' and finished_at_ms is null)", now.UnixMilli())
	return err
}

func (s *Store) WindowKeeperAttemptCount(ctx context.Context, authID, generation string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "select count(1) from window_keeper_attempts where account_id = ? and generation_key = ? and (status = 'started' or coalesce(error_kind, '') not in ('quota', 'auth', 'config'))", authID, generation).Scan(&count)
	return count, err
}

func (s *Store) HasWindowKeeperSuccess(ctx context.Context, authID, generation string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "select count(1) from window_keeper_attempts where account_id = ? and generation_key = ? and status = 'succeeded'", authID, generation).Scan(&count)
	return count > 0, err
}

func (s *Store) StartWindowKeeperAttempt(ctx context.Context, attempt windowkeeper.Attempt) (int64, error) {
	result, err := s.db.ExecContext(ctx, "insert into window_keeper_attempts(account_id, generation_key, started_at_ms, status, attempt_no) values(?, ?, ?, 'started', ?)", attempt.AccountID, attempt.GenerationKey, attempt.StartedAt.UnixMilli(), attempt.AttemptNo)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishWindowKeeperAttempt(ctx context.Context, id int64, status string, httpStatus int, kind, excerpt, responseID string) error {
	_, err := s.db.ExecContext(ctx, "update window_keeper_attempts set status = ?, finished_at_ms = ?, http_status = ?, error_kind = ?, output_excerpt = ?, response_id = ? where id = ?", status, time.Now().UnixMilli(), httpStatus, kind, excerpt, responseID, id)
	return err
}

func (s *Store) ListWindowKeeperAttempts(ctx context.Context, limit int) ([]windowkeeper.Attempt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, "select id, account_id, generation_key, started_at_ms, ifnull(finished_at_ms, 0), status, ifnull(http_status, 0), ifnull(error_kind, ''), attempt_no, ifnull(output_excerpt, ''), ifnull(response_id, '') from window_keeper_attempts order by id desc limit ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []windowkeeper.Attempt
	for rows.Next() {
		var attempt windowkeeper.Attempt
		var started, finished int64
		if err := rows.Scan(&attempt.ID, &attempt.AccountID, &attempt.GenerationKey, &started, &finished, &attempt.Status, &attempt.HTTPStatus, &attempt.ErrorKind, &attempt.AttemptNo, &attempt.Excerpt, &attempt.ResponseID); err != nil {
			return nil, err
		}
		if started > 0 {
			attempt.StartedAt = time.UnixMilli(started)
		}
		if finished > 0 {
			attempt.FinishedAt = time.UnixMilli(finished)
		}
		out = append(out, attempt)
	}
	return out, rows.Err()
}

func toMillis(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UnixMilli()
}

func fromNullMillis(val sql.NullInt64) time.Time {
	if !val.Valid || val.Int64 <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(val.Int64)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

type WindowKeeperStore struct {
	store *Store
}

func (s *Store) WindowKeeper() *WindowKeeperStore {
	return &WindowKeeperStore{store: s}
}

var _ windowkeeper.Store = (*WindowKeeperStore)(nil)

func (w *WindowKeeperStore) LoadSettings(ctx context.Context) (windowkeeper.Settings, bool, error) {
	return w.store.LoadWindowKeeperSettings(ctx)
}

func (w *WindowKeeperStore) SaveSettings(ctx context.Context, settings windowkeeper.Settings) error {
	return w.store.SaveWindowKeeperSettings(ctx, settings)
}

func (w *WindowKeeperStore) TouchAccount(ctx context.Context, account windowkeeper.Account) error {
	return w.store.TouchWindowKeeperAccount(ctx, account)
}

func (w *WindowKeeperStore) ListAccounts(ctx context.Context) ([]windowkeeper.Account, error) {
	return w.store.ListWindowKeeperAccounts(ctx)
}

func (w *WindowKeeperStore) SetPlan(ctx context.Context, authID, plan, group, mismatch string) error {
	return w.store.SetWindowKeeperPlan(ctx, authID, plan, group, mismatch)
}

func (w *WindowKeeperStore) SetOverride(ctx context.Context, authID, raw string) error {
	return w.store.SetWindowKeeperOverride(ctx, authID, raw)
}

func (w *WindowKeeperStore) SetPause(ctx context.Context, authID, reason string) error {
	return w.store.SetWindowKeeperPause(ctx, authID, reason)
}

func (w *WindowKeeperStore) SetNotBefore(ctx context.Context, authID string, when time.Time) error {
	return w.store.SetWindowKeeperNotBefore(ctx, authID, when)
}

func (w *WindowKeeperStore) SaveWindows(ctx context.Context, authID string, states []windowkeeper.State) error {
	return w.store.SaveWindowKeeperWindows(ctx, authID, states)
}

func (w *WindowKeeperStore) LoadWindows(ctx context.Context, authID string) ([]windowkeeper.State, error) {
	return w.store.LoadWindowKeeperWindows(ctx, authID)
}

func (w *WindowKeeperStore) Claim(ctx context.Context, authID, owner string, now, until time.Time) (bool, error) {
	return w.store.ClaimWindowKeeper(ctx, authID, owner, now, until)
}

func (w *WindowKeeperStore) Release(ctx context.Context, authID, owner string) error {
	return w.store.ReleaseWindowKeeper(ctx, authID, owner)
}

func (w *WindowKeeperStore) ReleaseAll(ctx context.Context) error {
	return w.store.ReleaseAllWindowKeeper(ctx)
}

func (w *WindowKeeperStore) RequeueStarted(ctx context.Context, now time.Time) error {
	return w.store.RequeueStartedWindowKeeper(ctx, now)
}

func (w *WindowKeeperStore) AttemptCount(ctx context.Context, authID, generation string) (int, error) {
	return w.store.WindowKeeperAttemptCount(ctx, authID, generation)
}

func (w *WindowKeeperStore) HasSuccess(ctx context.Context, authID, generation string) (bool, error) {
	return w.store.HasWindowKeeperSuccess(ctx, authID, generation)
}

func (w *WindowKeeperStore) StartAttempt(ctx context.Context, attempt windowkeeper.Attempt) (int64, error) {
	return w.store.StartWindowKeeperAttempt(ctx, attempt)
}

func (w *WindowKeeperStore) FinishAttempt(ctx context.Context, id int64, status string, httpStatus int, kind, excerpt, responseID string) error {
	return w.store.FinishWindowKeeperAttempt(ctx, id, status, httpStatus, kind, excerpt, responseID)
}

func (w *WindowKeeperStore) ListAttempts(ctx context.Context, limit int) ([]windowkeeper.Attempt, error) {
	return w.store.ListWindowKeeperAttempts(ctx, limit)
}
