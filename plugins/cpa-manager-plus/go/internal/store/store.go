package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store serializes SQLite access for the local plugin runtime.
type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	dsn := filepath.Join(dataDir, "usage.sqlite") + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`pragma journal_mode = WAL`,
		`pragma synchronous = FULL`,
		`pragma busy_timeout = 5000`,
		`pragma foreign_keys = ON`,
		`create table if not exists schema_migrations (version integer primary key, applied_at_ms integer not null)`,
		`create table if not exists settings (key text primary key, value blob not null, updated_at_ms integer not null)`,
		`create table if not exists usage_events (
			id integer primary key autoincrement,
			event_hash text not null unique,
			timestamp_ms integer not null,
			provider text,
			executor_type text,
			model text not null,
			api_key_hash text,
			auth_id text,
			auth_index text,
			auth_type text,
			source text,
			reasoning_effort text,
			service_tier text,
			input_tokens integer not null default 0,
			output_tokens integer not null default 0,
			reasoning_tokens integer not null default 0,
			cached_tokens integer not null default 0,
			cache_read_tokens integer not null default 0,
			cache_creation_tokens integer not null default 0,
			total_tokens integer not null default 0,
			latency_ms integer,
			ttft_ms integer,
			failed integer not null default 0,
			fail_status_code integer,
			fail_summary text,
			response_headers_json text,
			created_at_ms integer not null
		)`,
		`create index if not exists idx_usage_events_timestamp on usage_events(timestamp_ms)`,
		`create index if not exists idx_usage_events_model on usage_events(model)`,
		`create index if not exists idx_usage_events_provider on usage_events(provider)`,
		`create index if not exists idx_usage_events_auth on usage_events(auth_index)`,
		`create index if not exists idx_usage_events_api_key on usage_events(api_key_hash)`,
		`create table if not exists model_prices (
			model text primary key,
			prompt real not null default 0,
			completion real not null default 0,
			cache real not null default 0,
			cache_read real not null default 0,
			cache_creation real not null default 0,
			source text,
			source_model_id text,
			synced_at_ms integer,
			updated_at_ms integer not null
		)`,
		`create table if not exists account_action_candidates (
			id integer primary key autoincrement,
			action_type text not null,
			status text not null,
			provider text,
			auth_file_name text not null,
			auth_index text,
			account_snapshot text,
			auth_label text,
			reason_code text,
			reason text,
			last_error text,
			first_seen_at_ms integer not null,
			last_seen_at_ms integer not null,
			hit_count integer not null default 1,
			created_at_ms integer not null,
			updated_at_ms integer not null
		)`,
		`create unique index if not exists idx_action_pending_identity on account_action_candidates(auth_file_name, action_type, coalesce(auth_index, ''), coalesce(reason_code, '')) where status = 'pending'`,
		`create table if not exists codex_inspection_runs (
			id integer primary key autoincrement,
			trigger_type text not null,
			status text not null,
			started_at_ms integer not null,
			finished_at_ms integer,
			total_files integer not null default 0,
			error text,
			settings_json text not null default '{}',
			created_at_ms integer not null,
			updated_at_ms integer not null
		)`,
		`create table if not exists codex_inspection_results (
			id integer primary key autoincrement,
			run_id integer not null references codex_inspection_runs(id) on delete cascade,
			account_key text not null,
			file_name text not null,
			display_account text not null,
			provider text,
			disabled integer not null default 0,
			status text,
			action text not null,
			action_reason text,
			action_status text,
			action_error text,
			created_at_ms integer not null,
			unique(run_id, account_key)
		)`,
		`create table if not exists codex_inspection_logs (
			id integer primary key autoincrement,
			run_id integer not null references codex_inspection_runs(id) on delete cascade,
			level text not null,
			message text not null,
			detail_json text,
			created_at_ms integer not null
		)`,
		`create table if not exists inspection_disable_ownership (
			file_name text primary key,
			provider text,
			auth_index text,
			account_id text,
			disabled_at_ms integer not null,
			updated_at_ms integer not null
		)`,
		`create table if not exists dead_letter_events (id integer primary key autoincrement, payload text not null, error text not null, created_at_ms integer not null)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	if err := s.ensureModelPriceColumns(ctx); err != nil {
		return err
	}
	if err := s.ensureInspectionColumns(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `insert or ignore into schema_migrations(version, applied_at_ms) values(3, ?)`, time.Now().UnixMilli())
	return err
}

func (s *Store) ensureInspectionColumns(ctx context.Context) error {
	columns := map[string][]struct{ name, definition string }{
		"codex_inspection_runs": {
			{"trigger_key", "text"}, {"probe_set_count", "integer not null default 0"}, {"sampled_count", "integer not null default 0"},
			{"disabled_count", "integer not null default 0"}, {"enabled_count", "integer not null default 0"}, {"delete_count", "integer not null default 0"},
			{"disable_count", "integer not null default 0"}, {"enable_count", "integer not null default 0"}, {"reauth_count", "integer not null default 0"}, {"keep_count", "integer not null default 0"},
		},
		"codex_inspection_results": {
			{"auth_index", "text"}, {"account_id", "text"}, {"state", "text"}, {"status_code", "integer"}, {"used_percent", "real"},
			{"is_quota", "integer not null default 0"}, {"auto_recover_eligible", "integer not null default 0"}, {"executed_action", "text"},
			{"plan_type", "text"}, {"quota_windows_json", "text"}, {"error", "text"}, {"error_kind", "text"}, {"error_detail", "text"},
		},
	}
	for table, additions := range columns {
		rows, err := s.db.QueryContext(ctx, `pragma table_info(`+table+`)`)
		if err != nil {
			return err
		}
		existing := map[string]bool{}
		for rows.Next() {
			var cid int
			var name, typ string
			var notNull, primaryKey int
			var defaultValue any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				return err
			}
			existing[name] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for _, addition := range additions {
			if existing[addition.name] {
				continue
			}
			if _, err := s.db.ExecContext(ctx, `alter table `+table+` add column `+addition.name+` `+addition.definition); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) ensureModelPriceColumns(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `pragma table_info(model_prices)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, column := range []struct{ name, definition string }{{"source_model_id", "text"}, {"synced_at_ms", "integer"}} {
		if existing[column.name] {
			continue
		}
		if _, err := s.db.ExecContext(ctx, `alter table model_prices add column `+column.name+` `+column.definition); err != nil {
			return err
		}
	}
	return nil
}
