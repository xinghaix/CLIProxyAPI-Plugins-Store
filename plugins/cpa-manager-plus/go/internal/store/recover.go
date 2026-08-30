package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"
)

func IsCorrupt(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "malformed") || strings.Contains(msg, "sqlite_corrupt") || strings.Contains(msg, "disk image is malformed")
}

func isWALSalvageError(err error) bool {
	if IsCorrupt(err) {
		return true
	}
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not a database")
}

func configureDB(db *sql.DB) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
}

func (s *Store) ensureWritable(ctx context.Context) error {
	if err := s.PingWrite(ctx); err != nil {
		return err
	}
	return s.probeUsageInsert(ctx)
}

func (s *Store) probeUsageInsert(ctx context.Context) error {
	hash := fmt.Sprintf("write-probe-%d", time.Now().UnixNano())
	_, _, err := s.insertEventsCommitted(ctx, []Event{
		{Hash: hash, TimestampMS: time.Now().UnixMilli(), Model: "_write_probe"},
	})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `delete from usage_events where event_hash=?`, hash)
	return err
}

func (s *Store) repair(ctx context.Context, cause error) error {
	if cause != nil && !IsCorrupt(cause) {
		return cause
	}
	_, _ = s.db.ExecContext(ctx, `reindex`)
	_, _ = s.db.ExecContext(ctx, `vacuum`)
	if s.ensureWritable(ctx) == nil {
		return nil
	}
	if err := s.resetWAL(ctx); err == nil && s.ensureWritable(ctx) == nil {
		return nil
	}
	if err := s.rebuildUsageEvents(ctx); err != nil {
		return fmt.Errorf("rebuild usage_events: %w", err)
	}
	if err := s.ensureWritable(ctx); err != nil {
		return fmt.Errorf("usage_events still unwritable: %w", err)
	}
	return nil
}

func (s *Store) resetWAL(ctx context.Context) error {
	_ = s.db.Close()
	_ = os.Remove(s.path + "-wal")
	_ = os.Remove(s.path + "-shm")
	db, err := sql.Open("sqlite", s.dsn)
	if err != nil {
		return err
	}
	configureDB(db)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return s.migrate(ctx)
}

func (s *Store) rebuildUsageEvents(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `pragma foreign_keys = OFF`); err != nil {
		return err
	}
	defer func() { _, _ = s.db.ExecContext(ctx, `pragma foreign_keys = ON`) }()
	if _, err := s.db.ExecContext(ctx, `alter table usage_events rename to usage_events_corrupt_src`); err != nil {
		return err
	}
	if err := s.migrate(ctx); err != nil {
		_, _ = s.db.ExecContext(ctx, `alter table usage_events_corrupt_src rename to usage_events`)
		return err
	}
	_ = s.copyTableBestEffort(ctx, "usage_events", "usage_events_corrupt_src")
	_, _ = s.db.ExecContext(ctx, `drop table if exists usage_events_corrupt_src`)
	s.syncAutoIncrement(ctx, "usage_events")
	return nil
}

func (s *Store) copyTableBestEffort(ctx context.Context, dest, src string) error {
	cols, err := s.tableColumnList(ctx, src)
	if err != nil || len(cols) == 0 {
		return err
	}
	list := strings.Join(cols, ",")
	if _, err := s.db.ExecContext(ctx, `insert or ignore into `+dest+` (`+list+`) select `+list+` from `+src); err == nil {
		return nil
	}
	var minID, maxID int64
	if err := s.db.QueryRowContext(ctx, `select coalesce(min(id),0), coalesce(max(id),0) from `+src).Scan(&minID, &maxID); err != nil {
		return err
	}
	s.copyIDRange(ctx, dest, src, list, minID, maxID)
	return nil
}

func (s *Store) copyIDRange(ctx context.Context, dest, src, list string, lo, hi int64) {
	if lo > hi {
		return
	}
	_, err := s.db.ExecContext(ctx, `insert or ignore into `+dest+` (`+list+`) select `+list+` from `+src+` where id between ? and ?`, lo, hi)
	if err == nil {
		return
	}
	if lo == hi {
		return
	}
	mid := lo + (hi-lo)/2
	s.copyIDRange(ctx, dest, src, list, lo, mid)
	s.copyIDRange(ctx, dest, src, list, mid+1, hi)
}

func (s *Store) tableColumnList(ctx context.Context, table string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `pragma table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func (s *Store) syncAutoIncrement(ctx context.Context, table string) {
	_, _ = s.db.ExecContext(ctx, `delete from sqlite_sequence where name=?`, table)
	_, _ = s.db.ExecContext(ctx, `insert into sqlite_sequence(name, seq) select ?, coalesce(max(id), 0) from `+table, table)
}
