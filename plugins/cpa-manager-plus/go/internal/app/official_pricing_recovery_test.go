package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestOfficialPricingRestoresFromDatabaseAndRenewsAfterFailure(t *testing.T) {
	t.Cleanup(pricing.ResetOfficialCatalog)
	dir := t.TempDir()
	config := []byte("data_dir: " + dir)
	runtime, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetHTTPDo(officialPricingFixture())
	if err := runtime.refreshOfficialPricing(context.Background()); err != nil {
		t.Fatal(err)
	}
	saved, ok, err := runtime.Store().OfficialPriceRecord(context.Background())
	if err != nil || !ok || saved.ScheduleID == "" || saved.RatesJSON == "" {
		t.Fatalf("schedule was not saved: ok=%v err=%v record=%+v", ok, err, saved)
	}
	if saved.NextRefreshAtMS < time.Now().Add(7*time.Hour).UnixMilli() {
		t.Fatalf("successful sync did not renew the task: %d", saved.NextRefreshAtMS)
	}
	runtime.Close()
	pricing.ResetOfficialCatalog()

	restored, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	result := pricing.EstimateCost(pricing.Usage{Model: "gpt-6-sol", InputTokens: 100_000, ServiceTier: "standard"}, nil)
	if result.Status != pricing.StatusEstimated || result.Amount != 0.2 || result.ScheduleID != saved.ScheduleID {
		t.Fatalf("restart did not restore the saved schedule: %+v", result)
	}
	restored.SetHTTPDo(func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		return pricesync.HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	})
	if err := restored.refreshOfficialPricing(context.Background()); err == nil {
		t.Fatal("failed fetch reported success")
	}
	kept, ok, err := restored.Store().OfficialPriceRecord(context.Background())
	if err != nil || !ok {
		t.Fatal(err)
	}
	if kept.ScheduleID != saved.ScheduleID || kept.RatesJSON != saved.RatesJSON || kept.LastError == "" {
		t.Fatalf("failure replaced the saved schedule: %+v", kept)
	}
	wait := time.Until(time.UnixMilli(kept.NextRefreshAtMS))
	if wait < 10*time.Minute || wait > 50*time.Minute {
		t.Fatalf("failure did not renew a short retry: %s", wait)
	}
	still := pricing.EstimateCost(pricing.Usage{Model: "gpt-6-sol", InputTokens: 100_000, ServiceTier: "standard"}, nil)
	if still.ScheduleID != saved.ScheduleID || still.Amount != 0.2 {
		t.Fatalf("failure dropped the in-memory schedule: %+v", still)
	}
}

func TestCorruptOfficialPricingFallsBackAndReschedules(t *testing.T) {
	t.Cleanup(pricing.ResetOfficialCatalog)
	dir := t.TempDir()
	config := []byte("data_dir: " + dir)
	runtime, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Store().SaveOfficialPriceRecord(context.Background(), store.OfficialPriceRecord{ScheduleID: "bad", Source: pricing.OfficialPricingURL, RatesJSON: "{", FetchedAtMS: time.Now().UnixMilli(), NextRefreshAtMS: time.Now().Add(12 * time.Hour).UnixMilli()}); err != nil {
		t.Fatal(err)
	}
	runtime.Close()
	restored, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	result := pricing.EstimateCost(pricing.Usage{Model: "gpt-6-sol", InputTokens: 100_000}, nil)
	if result.Status != pricing.StatusUnpriced {
		t.Fatalf("corrupt schedule was applied: %+v", result)
	}
	record, ok, err := restored.Store().OfficialPriceRecord(context.Background())
	if err != nil || !ok || record.RatesJSON != "{" {
		t.Fatalf("corrupt row was deleted: ok=%v err=%v record=%+v", ok, err, record)
	}
	if time.Until(time.UnixMilli(record.NextRefreshAtMS)) > 2*time.Minute {
		t.Fatalf("corrupt schedule was not queued for a prompt retry: %d", record.NextRefreshAtMS)
	}
}

func TestOfficialRefreshWaitUsesSavedDeadline(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	if officialRefreshWait(now.Add(3*time.Hour).UnixMilli(), now) != 3*time.Hour {
		t.Fatal("future deadline was not honored")
	}
	overdue := officialRefreshWait(now.Add(-time.Minute).UnixMilli(), now)
	if overdue < 30*time.Second || overdue >= 90*time.Second {
		t.Fatalf("overdue task was not retried promptly: %s", overdue)
	}
}

func officialPricingFixture() func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
	fixture := strings.Join([]string{
		"### Standard pricing data",
		"| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| gpt-6-sol | $2.00 | $0.20 | $2.50 | $10.00 | $4.00 | $0.40 | $5.00 | $15.00 |",
		"| gpt-5.4 | $2.50 | $0.25 | - | $15.00 | $5.00 | $0.50 | - | $22.50 |",
		"| gpt-5.5 | $5.00 | $0.50 | - | $30.00 | $10.00 | $1.00 | - | $45.00 |",
		"### Fast pricing data",
		"| Model | Short context input | Short context cached input | Short context cache writes | Short context output | Long context input | Long context cached input | Long context cache writes | Long context output |",
		"| --- | --- | --- | --- | --- | --- | --- | --- | --- |",
		"| gpt-6-sol | $4.00 | $0.40 | $5.00 | $20.00 | $8.00 | $0.80 | $10.00 | $30.00 |",
		"| gpt-5.4 | $5.00 | $0.50 | - | $30.00 | - | - | - | - |",
		"| gpt-5.5 | $12.50 | $1.25 | - | $75.00 | - | - | - | - |",
	}, "\n")
	return func(_ context.Context, _ string, target string, _ http.Header, _ []byte) (pricesync.HTTPResponse, error) {
		if target == pricing.OfficialPricingURL {
			return pricesync.HTTPResponse{StatusCode: http.StatusOK, Body: []byte(fixture)}, nil
		}
		return pricesync.HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	}
}
