package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"
)

func TestRefreshOfficialPricingReplacesCompiledSchedule(t *testing.T) {
	t.Cleanup(pricing.ResetOfficialCatalog)
	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
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
	runtime.SetHTTPDo(func(_ context.Context, _ string, target string, _ http.Header, _ []byte) (pricesync.HTTPResponse, error) {
		if target == pricing.OfficialPricingURL {
			return pricesync.HTTPResponse{StatusCode: http.StatusOK, Body: []byte(fixture)}, nil
		}
		return pricesync.HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	})
	if err := runtime.refreshOfficialPricing(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := pricing.EstimateCost(pricing.Usage{Model: "gpt-6-sol", InputTokens: 100_000, ServiceTier: "standard"}, nil)
	if result.Status != pricing.StatusEstimated || result.Amount != 0.2 || !strings.HasPrefix(result.ScheduleID, "openai-api-docs-") {
		t.Fatalf("fetched official schedule was not used: %+v", result)
	}
	runtime.SetHTTPDo(func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		return pricesync.HTTPResponse{StatusCode: http.StatusInternalServerError}, nil
	})
	if err := runtime.refreshOfficialPricing(context.Background()); err == nil {
		t.Fatal("failed official fetch replaced nothing but also reported success")
	}
	kept := pricing.EstimateCost(pricing.Usage{Model: "gpt-6-sol", InputTokens: 100_000, ServiceTier: "standard"}, nil)
	if kept.ScheduleID != result.ScheduleID || kept.Amount != 0.2 {
		t.Fatalf("failed fetch discarded the saved schedule: %+v", kept)
	}
}
