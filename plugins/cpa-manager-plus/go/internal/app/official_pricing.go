package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

const officialPricingSettingKey = "openai_official_pricing_v1"

type storedOfficialPricing struct {
	ScheduleID  string                           `json:"schedule_id"`
	FetchedAtMS int64                            `json:"fetched_at_ms"`
	Source      string                           `json:"source"`
	Rates       map[string]pricing.ModelSchedule `json:"rates"`
}

func (r *Runtime) loadOfficialPricing(ctx context.Context) error {
	record, ok, err := r.store.OfficialPriceRecord(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return r.migrateOfficialPricingSetting(ctx)
	}
	rates, err := decodeOfficialRates(record.RatesJSON)
	if err != nil || record.ScheduleID == "" {
		_ = r.store.RenewOfficialPriceTask(ctx, time.Now().Add(officialFirstDelay()).UnixMilli(), "stored official pricing is unreadable")
		return nil
	}
	return pricing.UseOfficialCatalog(record.ScheduleID, rates)
}

func (r *Runtime) migrateOfficialPricingSetting(ctx context.Context) error {
	raw, ok, err := r.store.Setting(ctx, officialPricingSettingKey)
	if err != nil || !ok {
		return err
	}
	var stored storedOfficialPricing
	if err := json.Unmarshal(raw, &stored); err != nil || stored.ScheduleID == "" || len(stored.Rates) == 0 {
		return nil
	}
	ratesJSON, err := json.Marshal(stored.Rates)
	if err != nil {
		return nil
	}
	next := time.Now().Add(officialRenewalDelay(false))
	if err := r.store.SaveOfficialPriceRecord(ctx, store.OfficialPriceRecord{ScheduleID: stored.ScheduleID, Source: stored.Source, RatesJSON: string(ratesJSON), FetchedAtMS: stored.FetchedAtMS, NextRefreshAtMS: next.UnixMilli()}); err != nil {
		return err
	}
	return pricing.UseOfficialCatalog(stored.ScheduleID, stored.Rates)
}

func (r *Runtime) officialPriceLoop(ctx context.Context) {
	timer := time.NewTimer(r.officialRefreshWait(ctx))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_ = r.refreshOfficialPricing(ctx)
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(r.officialRefreshWait(ctx))
		}
	}
}

func (r *Runtime) officialRefreshWait(ctx context.Context) time.Duration {
	record, ok, err := r.store.OfficialPriceRecord(ctx)
	if err != nil || !ok {
		return officialFirstDelay()
	}
	return officialRefreshWait(record.NextRefreshAtMS, time.Now())
}

func officialRefreshWait(nextRefreshAtMS int64, now time.Time) time.Duration {
	if nextRefreshAtMS <= 0 {
		return officialFirstDelay()
	}
	wait := time.UnixMilli(nextRefreshAtMS).Sub(now)
	if wait <= 0 {
		return officialFirstDelay()
	}
	return wait
}

func officialFirstDelay() time.Duration {
	return time.Duration(30+rand.IntN(60)) * time.Second
}

func officialRenewalDelay(failed bool) time.Duration {
	if failed {
		return 15*time.Minute + time.Duration(rand.IntN(30))*time.Minute
	}
	return 8*time.Hour + time.Duration(rand.IntN(22*60))*time.Minute
}

func (r *Runtime) refreshOfficialPricing(ctx context.Context) error {
	if !r.officialMu.TryLock() {
		return nil
	}
	defer r.officialMu.Unlock()
	r.mu.Lock()
	do := r.httpDo
	r.mu.Unlock()
	if do == nil {
		return r.renewOfficialPricingAfterError(ctx, fmt.Errorf("host HTTP callback is unavailable"))
	}
	response, err := do(ctx, http.MethodGet, pricing.OfficialPricingURL, nil, nil)
	if err != nil {
		return r.renewOfficialPricingAfterError(ctx, err)
	}
	if response.StatusCode != http.StatusOK {
		return r.renewOfficialPricingAfterError(ctx, fmt.Errorf("official pricing fetch returned %d", response.StatusCode))
	}
	rates, err := pricing.ParseOfficialMarkdown(string(response.Body))
	if err != nil {
		return r.renewOfficialPricingAfterError(ctx, err)
	}
	fetched := time.Now().UTC()
	id, err := pricing.OfficialScheduleID(rates, fetched)
	if err != nil {
		return r.renewOfficialPricingAfterError(ctx, err)
	}
	ratesJSON, err := json.Marshal(rates)
	if err != nil {
		return r.renewOfficialPricingAfterError(ctx, err)
	}
	record := store.OfficialPriceRecord{
		ScheduleID: id, Source: pricing.OfficialPricingURL, RatesJSON: string(ratesJSON),
		FetchedAtMS: fetched.UnixMilli(), NextRefreshAtMS: fetched.Add(officialRenewalDelay(false)).UnixMilli(),
	}
	if err := r.store.SaveOfficialPriceRecord(ctx, record); err != nil {
		return err
	}
	return pricing.UseOfficialCatalog(id, rates)
}

func (r *Runtime) renewOfficialPricingAfterError(ctx context.Context, cause error) error {
	if err := r.store.RenewOfficialPriceTask(ctx, time.Now().Add(officialRenewalDelay(true)).UnixMilli(), cause.Error()); err != nil {
		return cause
	}
	return cause
}

func decodeOfficialRates(raw string) (map[string]pricing.ModelSchedule, error) {
	if raw == "" {
		return nil, fmt.Errorf("empty official pricing")
	}
	var rates map[string]pricing.ModelSchedule
	if err := json.Unmarshal([]byte(raw), &rates); err != nil {
		return nil, err
	}
	if len(rates) == 0 {
		return nil, fmt.Errorf("empty official pricing")
	}
	return rates, nil
}
