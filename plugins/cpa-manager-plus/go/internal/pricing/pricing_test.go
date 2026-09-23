package pricing

import (
	"math"
	"os"
	"strings"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/responsemodel"
)

func closeTo(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %.12f, want %.12f", got, want)
	}
}

func TestEstimateUsesOfficialTokenBuckets(t *testing.T) {
	result := EstimateCost(Usage{
		Model: "gpt-5.6-sol", InputTokens: 1_000, OutputTokens: 500,
		CachedTokens: 200, CacheReadTokens: 200, CacheWriteTokens: 100,
		ServiceTier: "default",
	}, nil)
	if result.Status != StatusEstimated || result.ContextTier != ContextShort || result.ServiceTier != TierStandard {
		t.Fatalf("unexpected estimate metadata: %+v", result)
	}
	closeTo(t, result.UncachedInputCost, 0.0028)
	closeTo(t, result.CachedInputCost, 0.00008)
	closeTo(t, result.CacheWriteCost, 0.0005)
	closeTo(t, result.OutputCost, 0.01)
	closeTo(t, result.Amount, 0.01338)
}

func TestLongContextThresholdIsPerRequestAndStrictlyGreater(t *testing.T) {
	atThreshold := EstimateCost(Usage{Model: "gpt-5.4", InputTokens: LongContextTokens, ServiceTier: "standard"}, nil)
	if atThreshold.ContextTier != ContextShort {
		t.Fatalf("exact threshold should use short context: %+v", atThreshold)
	}
	closeTo(t, atThreshold.Amount, 272_000*2.50/float64(TokensPerPriceUnit))

	overThreshold := EstimateCost(Usage{Model: "gpt-5.4", InputTokens: LongContextTokens + 1, ServiceTier: "standard"}, nil)
	if overThreshold.ContextTier != ContextLong {
		t.Fatalf("above threshold should use long context: %+v", overThreshold)
	}
	closeTo(t, overThreshold.Amount, 272_001*5/float64(TokensPerPriceUnit))
}

func TestFastAliasesUsePublishedPerModelRates(t *testing.T) {
	fast := EstimateCost(Usage{Model: "gpt-5.4", InputTokens: 100_000, ServiceTier: "fast"}, nil)
	if fast.ServiceTier != TierFast || fast.TierSource != TierSourceRequested {
		t.Fatalf("unexpected Fast metadata: %+v", fast)
	}
	closeTo(t, fast.Amount, 0.5)

	priority := EstimateCost(Usage{Model: "gpt-5.5", OutputTokens: 1_000_000, ServiceTier: "priority"}, nil)
	if priority.ServiceTier != TierFast {
		t.Fatalf("priority should normalize to Fast: %+v", priority)
	}
	closeTo(t, priority.Amount, 75)
}

func TestActualResponseTierOverridesRequestAndDowngrade(t *testing.T) {
	result := EstimateCost(Usage{
		Model: "gpt-5.6-sol", InputTokens: 100_000,
		ServiceTier: "fast", ResponseServiceTier: "default", ResponseObservationCount: 1,
	}, nil)
	if result.ServiceTier != TierStandard || result.TierSource != TierSourceActual || result.Note != "" {
		t.Fatalf("actual downgrade should override request without an assumed note: %+v", result)
	}
	closeTo(t, result.Amount, 0.4)
}

func TestAutoTierIsExplicitlyMarkedAsAssumed(t *testing.T) {
	result := EstimateCost(Usage{Model: "gpt-5.4", InputTokens: 100_000, ServiceTier: "auto"}, nil)
	if result.Status != StatusEstimated || result.ServiceTier != TierStandard || result.TierSource != TierSourceAssume || result.Note != "service_tier_assumed_standard" {
		t.Fatalf("auto tier should be marked as an assumption: %+v", result)
	}
	closeTo(t, result.Amount, 0.25)
}

func TestUnsupportedFastContextIsNotGuessed(t *testing.T) {
	result := EstimateCost(Usage{Model: "gpt-5.5", InputTokens: LongContextTokens + 1, ServiceTier: "fast"}, nil)
	if result.Status != StatusUnpriced || result.Note != "long_context_rate_unavailable" {
		t.Fatalf("missing Fast long-context rate should remain unpriced: %+v", result)
	}
}

func TestUnavailableCacheWriteRateIsNotTreatedAsFree(t *testing.T) {
	result := EstimateCost(Usage{Model: "gpt-5.5", InputTokens: 100, CacheWriteTokens: 10, ServiceTier: "standard"}, nil)
	if result.Status != StatusUnpriced || result.Note != "cache_write_rate_unavailable" {
		t.Fatalf("unsupported cache-write category should remain unpriced: %+v", result)
	}
}

func TestDatedModelIDAndLegacyFallback(t *testing.T) {
	dated := EstimateCost(Usage{Model: "GPT-5.4-2026-03-05", InputTokens: 100_000, ServiceTier: "standard"}, nil)
	if dated.ScheduleID != ScheduleID {
		t.Fatalf("dated model ID did not resolve: %+v", dated)
	}
	closeTo(t, dated.Amount, 0.25)

	flat := &FlatRates{Input: 1, Output: 2, CachedInput: 0.5, CacheRead: 0.25, CacheCreation: 1.5}
	legacy := EstimateCost(Usage{Model: "custom-model", InputTokens: 1_000_000, OutputTokens: 500_000}, flat)
	if legacy.ScheduleID != "model-price-flat" || legacy.Status != StatusEstimated {
		t.Fatalf("legacy model price was not retained: %+v", legacy)
	}
	closeTo(t, legacy.Amount, 2)
}

func TestLegacyFlatFallbackRejectsInvalidUsageAndRates(t *testing.T) {
	cases := []struct {
		name  string
		usage Usage
		rates FlatRates
	}{
		{name: "negative token counter", usage: Usage{Model: "custom-model", InputTokens: -1}, rates: FlatRates{Input: 1}},
		{name: "negative price", usage: Usage{Model: "custom-model", InputTokens: 1}, rates: FlatRates{Input: -1}},
		{name: "non-finite price", usage: Usage{Model: "custom-model", InputTokens: 1}, rates: FlatRates{Input: math.Inf(1)}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result := EstimateCost(test.usage, &test.rates)
			if result.Status != StatusInvalid || result.Amount != 0 {
				t.Fatalf("invalid fallback estimate should not become a charge: %+v", result)
			}
		})
	}
}

func TestFlatCachedCountersAreChargedOnce(t *testing.T) {
	flat := &FlatRates{CachedInput: 0.5, CacheRead: 0.25}
	overlapped := EstimateCost(Usage{Model: "custom-model", InputTokens: 1_000_000, CachedTokens: 1_000_000, CacheReadTokens: 1_000_000}, flat)
	if overlapped.Status != StatusEstimated {
		t.Fatalf("overlapping cache counters should remain priced: %+v", overlapped)
	}
	closeTo(t, overlapped.Amount, 0.5)

	readOnly := EstimateCost(Usage{Model: "custom-model", InputTokens: 1_000_000, CacheReadTokens: 1_000_000}, flat)
	closeTo(t, readOnly.Amount, 0.25)
}

func TestDateSuffixRejectsNonDates(t *testing.T) {
	invalid := EstimateCost(Usage{Model: "gpt-5.4-99999999", InputTokens: 1}, nil)
	if invalid.Status != StatusUnpriced || invalid.ScheduleID != "" {
		t.Fatalf("non-date suffix inherited an official schedule: %+v", invalid)
	}
	overflow := EstimateCost(Usage{Model: "gpt-5.4-2026-02-31", InputTokens: 1}, nil)
	if overflow.ScheduleID == ScheduleID {
		t.Fatalf("invalid calendar date inherited an official schedule: %+v", overflow)
	}
}

func TestRecognizedServiceTiersHaveExplicitPricingDecisions(t *testing.T) {
	priced := map[string]bool{"default": true, "standard": true, "priority": true, "fast": true}
	unpriced := map[string]bool{"flex": true}
	for _, tier := range responsemodel.RecognizedServiceTiers() {
		result := EstimateCost(Usage{Model: "gpt-5.4", InputTokens: 1, ResponseServiceTier: tier, ResponseObservationCount: 1}, nil)
		switch {
		case priced[tier] && result.Status != StatusEstimated:
			t.Fatalf("recognized tier %s was not priced: %+v", tier, result)
		case unpriced[tier] && result.Note != NoteServiceTierUnpriced:
			t.Fatalf("recognized tier %s should stay explicitly unpriced: %+v", tier, result)
		case !priced[tier] && !unpriced[tier]:
			t.Fatalf("recognized tier %s has no pricing decision", tier)
		}
	}
}

func TestFrontendMapsEveryEstimateNote(t *testing.T) {
	body, err := os.ReadFile("../../../web/src/utils/costEstimateDisplay.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, note := range EstimateNotes() {
		if !strings.Contains(source, note+":") {
			t.Fatalf("monitoring display does not map estimate note %s", note)
		}
	}
}
