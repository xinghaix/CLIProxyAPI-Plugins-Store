package store

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/clock"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/codex-window-keeper/go/internal/config"
	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
	key  []byte
}

type Account struct {
	AuthID        string    `json:"auth_id"`
	AuthIndex     string    `json:"auth_index"`
	AccountID     string    `json:"account_id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	PlanType      string    `json:"plan_type"`
	PlanGroup     string    `json:"plan_group"`
	ShapeMismatch string    `json:"shape_mismatch"`
	Disabled      bool      `json:"disabled"`
	Unavailable   bool      `json:"unavailable"`
	OverrideJSON  string    `json:"override_json"`
	PauseReason   string    `json:"pause_reason"`
	NotBefore     time.Time `json:"not_before"`
}

type Attempt struct {
	ID            int64
	AccountID     string
	GenerationKey string
	StartedAt     time.Time
	FinishedAt    time.Time
	Status        string
	HTTPStatus    int
	ErrorKind     string
	AttemptNo     int
	NotBefore     time.Time
	Excerpt       string
	ResponseID    string
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, "keeper.sqlite")
	key, err := loadKey(filepath.Join(dataDir, "keeper.key"))
	if err != nil {
		return nil, err
	}
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path, key: key}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Path() string { return s.path }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		"pragma journal_mode = WAL",
		"pragma synchronous = FULL",
		"pragma busy_timeout = 5000",
		"pragma foreign_keys = ON",
		"create table if not exists schema_migrations (version integer primary key, applied_at_ms integer not null)",
		"create table if not exists settings (key text primary key, value blob not null, updated_at_ms integer not null)",
		`create table if not exists accounts (
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
		`create table if not exists windows (
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
		`create table if not exists attempts (
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
		"create unique index if not exists attempts_success on attempts(account_id, generation_key) where status = 'succeeded'",
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	if err := s.ensureColumn(ctx, "windows", "last_blocked_ends_ms", "integer"); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := s.db.ExecContext(ctx, "insert or ignore into schema_migrations(version, applied_at_ms) values(1, ?)", now); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, "insert or ignore into schema_migrations(version, applied_at_ms) values(2, ?)", now)
	return err
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "pragma table_info("+table+")")
	if err != nil {
		return err
	}
	for rows.Next() {
		var cid, notNull, primary int
		var name, kind string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primary); err != nil {
			_ = rows.Close()
			return err
		}
		if name == column {
			return rows.Close()
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "alter table "+table+" add column "+column+" "+definition)
	return err
}

func (s *Store) LoadSettings(ctx context.Context) (config.Settings, bool, error) {
	var raw []byte
	err := s.db.QueryRowContext(ctx, "select value from settings where key = 'runtime'").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return config.Settings{}, false, nil
	}
	if err != nil {
		return config.Settings{}, false, err
	}
	var settings config.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return config.Settings{}, false, err
	}
	return settings, true, nil
}

func (s *Store) SaveSettings(ctx context.Context, settings config.Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "insert into settings(key, value, updated_at_ms) values('runtime', ?, ?) on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms", raw, time.Now().UnixMilli())
	return err
}

func (s *Store) SaveManagementKey(ctx context.Context, plain string) error {
	blob, err := encrypt(s.key, []byte(plain))
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, "insert into settings(key, value, updated_at_ms) values('management_key', ?, ?) on conflict(key) do update set value = excluded.value, updated_at_ms = excluded.updated_at_ms", blob, time.Now().UnixMilli())
	return err
}

func (s *Store) LoadManagementKey(ctx context.Context) (string, bool, error) {
	var blob []byte
	err := s.db.QueryRowContext(ctx, "select value from settings where key = 'management_key'").Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	plain, err := decrypt(s.key, blob)
	if err != nil {
		return "", false, err
	}
	return string(plain), true, nil
}

func (s *Store) TouchAccount(ctx context.Context, account Account) error {
	now := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, `insert into accounts(auth_id, auth_index, chatgpt_account_id, email, display_name, disabled, unavailable, updated_at_ms)
		values(?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(auth_id) do update set auth_index = excluded.auth_index, chatgpt_account_id = excluded.chatgpt_account_id,
			email = excluded.email, display_name = excluded.display_name, disabled = excluded.disabled, unavailable = excluded.unavailable, updated_at_ms = excluded.updated_at_ms`,
		account.AuthID, account.AuthIndex, account.AccountID, account.Email, account.Name, boolInt(account.Disabled), boolInt(account.Unavailable), now)
	return err
}

func (s *Store) SetPlan(ctx context.Context, authID, plan, group, mismatch string) error {
	_, err := s.db.ExecContext(ctx, "update accounts set plan_type = ?, plan_group = ?, shape_mismatch = ?, updated_at_ms = ? where auth_id = ?", plan, group, mismatch, time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) SetOverride(ctx context.Context, authID, raw string) error {
	_, err := s.db.ExecContext(ctx, "update accounts set override_json = ?, updated_at_ms = ? where auth_id = ?", raw, time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) SetPause(ctx context.Context, authID, reason string) error {
	_, err := s.db.ExecContext(ctx, "update accounts set pause_reason = ?, updated_at_ms = ? where auth_id = ?", reason, time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) SetNotBefore(ctx context.Context, authID string, when time.Time) error {
	_, err := s.db.ExecContext(ctx, "update accounts set not_before_ms = ?, updated_at_ms = ? where auth_id = ?", when.UnixMilli(), time.Now().UnixMilli(), authID)
	return err
}

func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.db.QueryContext(ctx, `select auth_id, ifnull(auth_index, ''), ifnull(chatgpt_account_id, ''), ifnull(email, ''), ifnull(display_name, ''), ifnull(plan_type, ''), ifnull(plan_group, ''), ifnull(shape_mismatch, ''), disabled, unavailable, ifnull(override_json, '{}'), ifnull(pause_reason, ''), not_before_ms from accounts order by email, auth_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Account
	for rows.Next() {
		var account Account
		var disabled, unavailable int
		var notBefore int64
		if err := rows.Scan(&account.AuthID, &account.AuthIndex, &account.AccountID, &account.Email, &account.Name, &account.PlanType, &account.PlanGroup, &account.ShapeMismatch, &disabled, &unavailable, &account.OverrideJSON, &account.PauseReason, &notBefore); err != nil {
			return nil, err
		}
		account.Disabled = disabled == 1
		account.Unavailable = unavailable == 1
		if notBefore > 0 {
			account.NotBefore = time.UnixMilli(notBefore).UTC()
		}
		out = append(out, account)
	}
	return out, rows.Err()
}

func (s *Store) SaveWindows(ctx context.Context, authID string, states []clock.State) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	seen := map[string]bool{}
	for _, state := range states {
		seen[clock.Key(state)] = true
		if _, err := tx.ExecContext(ctx, `insert into windows(account_id, limit_id, slot, period_seconds, kind, starts_at_ms, ends_at_ms, last_blocked_ends_ms, time_source, used_percent, gating, phase, seen_blocked, hypothesis, absent)
			values(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			on conflict(account_id, limit_id, slot, period_seconds) do update set kind = excluded.kind, starts_at_ms = excluded.starts_at_ms, ends_at_ms = excluded.ends_at_ms, last_blocked_ends_ms = excluded.last_blocked_ends_ms, time_source = excluded.time_source, used_percent = excluded.used_percent, gating = excluded.gating, phase = excluded.phase, seen_blocked = excluded.seen_blocked, hypothesis = excluded.hypothesis, absent = excluded.absent`,
			authID, state.LimitID, state.Slot, state.PeriodSeconds, state.Kind, millis(state.StartsAt), millis(state.EndsAt), millis(state.LastBlockedEnd), state.TimeSource, state.UsedPercent, boolInt(state.Gating), state.Phase, boolInt(state.SeenBlocked), state.Hypothesis, boolInt(state.Absent)); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, "select limit_id, slot, period_seconds from windows where account_id = ?", authID)
	if err != nil {
		return err
	}
	type key struct {
		limit, slot string
		period      int64
	}
	var stale []key
	for rows.Next() {
		var item key
		if err := rows.Scan(&item.limit, &item.slot, &item.period); err != nil {
			rows.Close()
			return err
		}
		if !seen[item.limit+"|"+item.slot+"|"+fmt.Sprint(item.period)] {
			stale = append(stale, item)
		}
	}
	rows.Close()
	for _, item := range stale {
		if _, err := tx.ExecContext(ctx, "update windows set absent = 1, gating = 0, phase = 'absent' where account_id = ? and limit_id = ? and slot = ? and period_seconds = ?", authID, item.limit, item.slot, item.period); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LoadWindows(ctx context.Context, authID string) ([]clock.State, error) {
	rows, err := s.db.QueryContext(ctx, "select limit_id, slot, period_seconds, kind, starts_at_ms, ends_at_ms, last_blocked_ends_ms, time_source, used_percent, gating, phase, seen_blocked, hypothesis, absent from windows where account_id = ?", authID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clock.State
	for rows.Next() {
		var state clock.State
		var start, end, lastBlocked sql.NullInt64
		var gating, seen, absent int
		if err := rows.Scan(&state.LimitID, &state.Slot, &state.PeriodSeconds, &state.Kind, &start, &end, &lastBlocked, &state.TimeSource, &state.UsedPercent, &gating, &state.Phase, &seen, &state.Hypothesis, &absent); err != nil {
			return nil, err
		}
		state.StartsAt = timeOrZero(start)
		state.EndsAt = timeOrZero(end)
		state.LastBlockedEnd = timeOrZero(lastBlocked)
		state.Gating = gating == 1
		state.SeenBlocked = seen == 1
		state.Absent = absent == 1
		out = append(out, state)
	}
	return out, rows.Err()
}

func (s *Store) Claim(ctx context.Context, authID, owner string, now, until time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, "update accounts set lease_owner = ?, lease_until_ms = ? where auth_id = ? and (lease_until_ms is null or lease_until_ms < ?)", owner, until.UnixMilli(), authID, now.UnixMilli())
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

func (s *Store) Release(ctx context.Context, authID, owner string) error {
	_, err := s.db.ExecContext(ctx, "update accounts set lease_owner = null, lease_until_ms = null where auth_id = ? and lease_owner = ?", authID, owner)
	return err
}

func (s *Store) ReleaseAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "update accounts set lease_owner = null, lease_until_ms = null")
	return err
}

func (s *Store) RequeueStarted(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, "update accounts set not_before_ms = ? where auth_id in (select account_id from attempts where status = 'started' and finished_at_ms is null)", now.UnixMilli())
	return err
}

func (s *Store) AttemptCount(ctx context.Context, authID, generation string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "select count(1) from attempts where account_id = ? and generation_key = ? and (status = 'started' or coalesce(error_kind, '') not in ('quota', 'auth', 'config'))", authID, generation).Scan(&count)
	return count, err
}

func (s *Store) HasSuccess(ctx context.Context, authID, generation string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "select count(1) from attempts where account_id = ? and generation_key = ? and status = 'succeeded'", authID, generation).Scan(&count)
	return count > 0, err
}

func (s *Store) StartAttempt(ctx context.Context, attempt Attempt) (int64, error) {
	result, err := s.db.ExecContext(ctx, "insert into attempts(account_id, generation_key, started_at_ms, status, attempt_no) values(?, ?, ?, 'started', ?)", attempt.AccountID, attempt.GenerationKey, attempt.StartedAt.UnixMilli(), attempt.AttemptNo)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *Store) FinishAttempt(ctx context.Context, id int64, status string, httpStatus int, kind, excerpt, responseID string) error {
	_, err := s.db.ExecContext(ctx, "update attempts set status = ?, finished_at_ms = ?, http_status = ?, error_kind = ?, output_excerpt = ?, response_id = ? where id = ?", status, time.Now().UnixMilli(), httpStatus, kind, excerpt, responseID, id)
	return err
}

func (s *Store) ListAttempts(ctx context.Context, limit int) ([]Attempt, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, "select id, account_id, generation_key, started_at_ms, ifnull(finished_at_ms, 0), status, ifnull(http_status, 0), ifnull(error_kind, ''), attempt_no, ifnull(output_excerpt, ''), ifnull(response_id, '') from attempts order by id desc limit ?", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Attempt
	for rows.Next() {
		var attempt Attempt
		var started, finished int64
		if err := rows.Scan(&attempt.ID, &attempt.AccountID, &attempt.GenerationKey, &started, &finished, &attempt.Status, &attempt.HTTPStatus, &attempt.ErrorKind, &attempt.AttemptNo, &attempt.Excerpt, &attempt.ResponseID); err != nil {
			return nil, err
		}
		attempt.StartedAt = time.UnixMilli(started).UTC()
		if finished > 0 {
			attempt.FinishedAt = time.UnixMilli(finished).UTC()
		}
		out = append(out, attempt)
	}
	return out, rows.Err()
}

func loadKey(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err == nil && len(raw) == 32 {
		return raw, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func encrypt(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return append(nonce, gcm.Seal(nil, nonce, plain, nil)...), nil
}

func decrypt(key, blob []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, body := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, body, nil)
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func millis(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UnixMilli()
}

func timeOrZero(value sql.NullInt64) time.Time {
	if !value.Valid || value.Int64 == 0 {
		return time.Time{}
	}
	return time.UnixMilli(value.Int64).UTC()
}

func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "sqlite_busy")
}
