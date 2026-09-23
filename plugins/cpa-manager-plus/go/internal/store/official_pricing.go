package store

import (
	"context"
	"database/sql"
	"time"
)

// OfficialPriceRecord is the single saved OpenAI schedule and its next refresh.
type OfficialPriceRecord struct {
	ScheduleID      string
	Source          string
	RatesJSON       string
	FetchedAtMS     int64
	NextRefreshAtMS int64
	LastError       string
}

func (s *Store) OfficialPriceRecord(ctx context.Context) (OfficialPriceRecord, bool, error) {
	var record OfficialPriceRecord
	err := s.db.QueryRowContext(ctx, `select schedule_id, source, rates_json, fetched_at_ms, next_refresh_at_ms, last_error from official_price_schedule where id=1`).Scan(&record.ScheduleID, &record.Source, &record.RatesJSON, &record.FetchedAtMS, &record.NextRefreshAtMS, &record.LastError)
	if err == sql.ErrNoRows {
		return OfficialPriceRecord{}, false, nil
	}
	if err != nil {
		return OfficialPriceRecord{}, false, err
	}
	return record, true, nil
}

// SaveOfficialPriceRecord replaces the saved schedule and renews its next refresh.
func (s *Store) SaveOfficialPriceRecord(ctx context.Context, record OfficialPriceRecord) error {
	_, err := s.db.ExecContext(ctx, `insert into official_price_schedule(id,schedule_id,source,rates_json,fetched_at_ms,next_refresh_at_ms,last_error,updated_at_ms) values(1,?,?,?,?,?,'',?) on conflict(id) do update set schedule_id=excluded.schedule_id, source=excluded.source, rates_json=excluded.rates_json, fetched_at_ms=excluded.fetched_at_ms, next_refresh_at_ms=excluded.next_refresh_at_ms, last_error='', updated_at_ms=excluded.updated_at_ms`, record.ScheduleID, record.Source, record.RatesJSON, record.FetchedAtMS, record.NextRefreshAtMS, time.Now().UnixMilli())
	return err
}

// RenewOfficialPriceTask moves the next refresh without touching a saved schedule.
func (s *Store) RenewOfficialPriceTask(ctx context.Context, nextRefreshAtMS int64, lastError string) error {
	_, err := s.db.ExecContext(ctx, `insert into official_price_schedule(id,schedule_id,source,rates_json,fetched_at_ms,next_refresh_at_ms,last_error,updated_at_ms) values(1,'','','',0,?,?,?) on conflict(id) do update set next_refresh_at_ms=excluded.next_refresh_at_ms, last_error=excluded.last_error, updated_at_ms=excluded.updated_at_ms`, nextRefreshAtMS, lastError, time.Now().UnixMilli())
	return err
}
