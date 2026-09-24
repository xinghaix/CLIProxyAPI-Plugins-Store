// Package pricing owns the versioned API-equivalent rate schedule and the
// token-bucket estimate used by billing analytics.
package pricing

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	ScheduleID         = "openai-api-pricing-2026-09"
	CurrencyUSD        = "USD"
	BasisAPICost       = "openai_api_equivalent"
	TokensPerPriceUnit = 1_000_000
	LongContextTokens  = int64(272_000)
)

const (
	StatusEstimated     = "estimated"
	StatusUnpriced      = "unpriced"
	StatusInvalid       = "invalid"
	ContextShort        = "short"
	ContextLong         = "long"
	TierStandard        = "standard"
	TierFast            = "fast"
	TierSourceActual    = "response"
	TierSourceRequested = "requested"
	TierSourceAssume    = "assumed-standard"

	NoteMissingModel           = "missing_model"
	NoteMissingPrice           = "missing_price"
	NoteInvalidTokenCounts     = "invalid_token_counts"
	NoteInvalidFlatRates       = "invalid_flat_rates"
	NoteInvalidEstimate        = "invalid_estimate"
	NoteTokenBreakdownExceeds  = "token_breakdown_exceeds_input"
	NoteLongContextUnavailable = "long_context_rate_unavailable"
	NoteCachedInputUnavailable = "cached_input_rate_unavailable"
	NoteCacheWriteUnavailable  = "cache_write_rate_unavailable"
	NoteFastUnavailable        = "fast_rate_unavailable"
	NoteServiceTierUnpriced    = "service_tier_unpriced"
	NoteAssumedStandard        = "service_tier_assumed_standard"
	NoteZeroFlatRate           = "zero_flat_rate"
)

// EstimateNotes is the complete set of note values emitted by EstimateCost.
// The monitoring UI maps each one in costEstimateDisplay.js.
func EstimateNotes() []string {
	return []string{
		NoteMissingModel, NoteMissingPrice, NoteInvalidTokenCounts, NoteInvalidFlatRates,
		NoteInvalidEstimate, NoteTokenBreakdownExceeds, NoteLongContextUnavailable,
		NoteCachedInputUnavailable, NoteCacheWriteUnavailable, NoteFastUnavailable,
		NoteServiceTierUnpriced, NoteAssumedStandard, NoteZeroFlatRate,
	}
}

// Usage contains per-request counters. InputTokens is the OpenAI input total;
// cached counters are subsets of that total. CachedTokens and CacheReadTokens
// can repeat the same OpenAI count, so the larger value prevents double charge.
type Usage struct {
	Model                    string
	InputTokens              int64
	OutputTokens             int64
	CachedTokens             int64
	CacheReadTokens          int64
	CacheWriteTokens         int64
	ServiceTier              string
	ResponseServiceTier      string
	ResponseTierAmbiguous    bool
	ResponseObservationCount int64
	ResponseObservationError bool
}

// FlatRates preserves existing editable model prices for models without an
// official OpenAI schedule.
type FlatRates struct {
	Input, Output, CachedInput, CacheRead, CacheCreation float64
}

// TokenRates uses USD per million tokens. Support flags distinguish a published
// zero price from a token category the pricing table does not publish.
type TokenRates struct {
	Input, CachedInput, CacheWrite, Output float64
	HasCachedInput, HasCacheWrite          bool
}

type PriceBands struct {
	Short TokenRates
	Long  *TokenRates
}

type ModelSchedule struct {
	Standard               PriceBands  `json:"standard"`
	Fast                   *PriceBands `json:"fast,omitempty"`
	ContextThresholdTokens int64       `json:"context_threshold_tokens,omitempty"`
}

// Estimate is an API-price-equivalent estimate. A zero Amount is valid only
// when StatusEstimated; callers must not show an unavailable estimate as zero.
type Estimate struct {
	Amount            float64 `json:"amount"`
	Currency          string  `json:"currency"`
	Basis             string  `json:"basis"`
	Status            string  `json:"status"`
	Model             string  `json:"model,omitempty"`
	ScheduleID             string  `json:"schedule_id,omitempty"`
	ContextTier            string  `json:"context_tier,omitempty"`
	ContextThresholdTokens int64   `json:"context_threshold_tokens,omitempty"`
	ServiceTier            string  `json:"service_tier,omitempty"`
	TierSource        string  `json:"tier_source,omitempty"`
	UncachedInputCost float64 `json:"uncached_input_cost,omitempty"`
	CachedInputCost   float64 `json:"cached_input_cost,omitempty"`
	CacheWriteCost    float64 `json:"cache_write_cost,omitempty"`
	OutputCost        float64 `json:"output_cost,omitempty"`
	Note              string  `json:"note,omitempty"`
}

// officialRates deliberately contains only schedules verified from primary pricing data.
var officialRates = map[string]ModelSchedule{
	"gpt-5.4": {
		ContextThresholdTokens: LongContextTokens,
		Standard: PriceBands{
			Short: TokenRates{Input: 2.50, CachedInput: 0.25, Output: 15, HasCachedInput: true},
			Long:  ratePtr(rates(5, 0.50, 0, 22.50, true, false)),
		},
		Fast: &PriceBands{Short: TokenRates{Input: 5, CachedInput: 0.50, Output: 30, HasCachedInput: true}},
	},
	"gpt-5.5": {
		ContextThresholdTokens: LongContextTokens,
		Standard: PriceBands{
			Short: TokenRates{Input: 5, CachedInput: 0.50, Output: 30, HasCachedInput: true},
			Long:  ratePtr(rates(10, 1, 0, 45, true, false)),
		},
		Fast: &PriceBands{Short: TokenRates{Input: 12.50, CachedInput: 1.25, Output: 75, HasCachedInput: true}},
	},
	"gpt-5.6-sol": {
		ContextThresholdTokens: LongContextTokens,
		Standard: bands(rates(4, 0.40, 5, 20, true, true), rates(8, 0.80, 10, 30, true, true)),
		Fast:     &PriceBands{Short: rates(8, 0.80, 10, 40, true, true), Long: ratePtr(rates(16, 1.60, 20, 60, true, true))},
	},
}

func rates(input, cached, write, output float64, hasCached, hasWrite bool) TokenRates {
	return TokenRates{Input: input, CachedInput: cached, CacheWrite: write, Output: output, HasCachedInput: hasCached, HasCacheWrite: hasWrite}
}

func ratePtr(value TokenRates) *TokenRates { return &value }

func bands(short, long TokenRates) PriceBands { return PriceBands{Short: short, Long: ratePtr(long)} }

// EstimateCost applies a per-request context band and exact Standard/Fast
// rates. Unknown models retain the legacy editable flat price as a fallback.
func EstimateCost(usage Usage, flat *FlatRates) Estimate {
	model := strings.ToLower(strings.TrimSpace(usage.Model))
	if model == "" {
		return unavailable(model, NoteMissingModel)
	}
	if schedule, scheduleID, ok := lookupSchedule(model); ok {
		return estimateOfficial(usage, model, scheduleID, schedule)
	}
	if flat == nil {
		return unavailable(model, NoteMissingPrice)
	}
	return estimateFlat(usage, model, *flat)
}

type tierChoice struct {
	Bands        PriceBands
	Tier, Source string
	Note         string
	OK           bool
}

func estimateOfficial(usage Usage, model, scheduleID string, schedule ModelSchedule) Estimate {
	result := Estimate{Currency: CurrencyUSD, Basis: BasisAPICost, Status: StatusUnpriced, Model: model, ScheduleID: scheduleID}
	if negativeTokens(usage) {
		result.Status, result.Note = StatusInvalid, NoteInvalidTokenCounts
		return result
	}
	threshold := schedule.ContextThresholdTokens
	if threshold <= 0 {
		threshold = LongContextTokens
	}
	result.ContextThresholdTokens = threshold
	contextTier := ContextShort
	if usage.InputTokens > threshold {
		contextTier = ContextLong
	}
	result.ContextTier = contextTier
	choice := selectTier(schedule, usage)
	if !choice.OK {
		result.Note = choice.Note
		return result
	}
	result.ServiceTier, result.TierSource = choice.Tier, choice.Source
	var tokenRates TokenRates
	if contextTier == ContextLong {
		if choice.Bands.Long == nil {
			result.Note = NoteLongContextUnavailable
			return result
		}
		tokenRates = *choice.Bands.Long
	} else {
		tokenRates = choice.Bands.Short
	}
	cached, ok := cachedInputCount(usage)
	if !ok {
		result.Status, result.Note = StatusInvalid, NoteTokenBreakdownExceeds
		return result
	}
	if cached > 0 && !tokenRates.HasCachedInput {
		result.Note = NoteCachedInputUnavailable
		return result
	}
	if usage.CacheWriteTokens > 0 && !tokenRates.HasCacheWrite {
		result.Note = NoteCacheWriteUnavailable
		return result
	}
	return finishEstimate(applyRates(result, usage, cached, tokenRates.Input, tokenRates.CachedInput, tokenRates.CacheWrite, tokenRates.Output, choice.Note))
}

func selectTier(schedule ModelSchedule, usage Usage) tierChoice {
	tierValue := strings.ToLower(strings.TrimSpace(usage.ResponseServiceTier))
	tierSource := TierSourceActual
	if usage.ResponseTierAmbiguous || usage.ResponseObservationError || usage.ResponseObservationCount != 1 {
		tierValue = ""
	}
	if tierValue == "" {
		tierValue = strings.ToLower(strings.TrimSpace(usage.ServiceTier))
		tierSource = TierSourceRequested
	}
	if tierValue == "" || tierValue == "auto" {
		tierValue, tierSource = TierStandard, TierSourceAssume
	}
	switch tierValue {
	case "default", TierStandard:
		note := ""
		if tierSource == TierSourceAssume {
			note = NoteAssumedStandard
		}
		return tierChoice{Bands: schedule.Standard, Tier: TierStandard, Source: tierSource, Note: note, OK: true}
	case "priority", TierFast:
		if schedule.Fast == nil {
			return tierChoice{Tier: TierFast, Source: tierSource, Note: NoteFastUnavailable}
		}
		return tierChoice{Bands: *schedule.Fast, Tier: TierFast, Source: tierSource, OK: true}
	case "flex":
		return tierChoice{Tier: tierValue, Source: tierSource, Note: NoteServiceTierUnpriced}
	default:
		return tierChoice{Tier: tierValue, Source: tierSource, Note: NoteServiceTierUnpriced}
	}
}

func estimateFlat(usage Usage, model string, flat FlatRates) Estimate {
	result := Estimate{Currency: CurrencyUSD, Basis: BasisAPICost, Status: StatusEstimated, Model: model, ScheduleID: "model-price-flat", ContextTier: "flat"}
	if negativeTokens(usage) {
		result.Status, result.Note = StatusInvalid, NoteInvalidTokenCounts
		return result
	}
	for _, rate := range []float64{flat.Input, flat.Output, flat.CachedInput, flat.CacheRead, flat.CacheCreation} {
		if rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
			result.Status, result.Note = StatusInvalid, NoteInvalidFlatRates
			return result
		}
	}
	cached, ok := cachedInputCount(usage)
	if !ok {
		result.Status, result.Note = StatusInvalid, NoteTokenBreakdownExceeds
		return result
	}
	// CachedTokens and CacheReadTokens can repeat one count. Charge that count once.
	result = applyRates(result, usage, cached, flat.Input, flatCachedRate(usage, flat), flat.CacheCreation, flat.Output, "")
	result = finishEstimate(result)
	if result.Status == StatusEstimated && result.Amount == 0 && hasBillableTokens(usage) {
		result.Status, result.Note = StatusUnpriced, NoteZeroFlatRate
	}
	return result
}

func flatCachedRate(usage Usage, flat FlatRates) float64 {
	if usage.CacheReadTokens > usage.CachedTokens {
		return flat.CacheRead
	}
	if flat.CachedInput > 0 || usage.CacheReadTokens == 0 {
		return flat.CachedInput
	}
	return flat.CacheRead
}

func lookupSchedule(model string) (ModelSchedule, string, bool) {
	catalog := activeCatalog()
	if schedule, ok := catalog.rates[model]; ok {
		return schedule, catalog.id, true
	}
	// Date-pinned model IDs share the base schedule. Other suffixes are not
	// guessed; variants with different rates have their own longer key.
	keys := make([]string, 0, len(catalog.rates))
	for key := range catalog.rates {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		if strings.HasPrefix(model, key+"-") && numericDateSuffix(strings.TrimPrefix(model, key+"-")) {
			return catalog.rates[key], catalog.id, true
		}
	}
	return ModelSchedule{}, "", false
}

func numericDateSuffix(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func negativeTokens(usage Usage) bool {
	return usage.InputTokens < 0 || usage.OutputTokens < 0 || usage.CachedTokens < 0 || usage.CacheReadTokens < 0 || usage.CacheWriteTokens < 0
}

func hasBillableTokens(usage Usage) bool {
	return usage.InputTokens > 0 || usage.OutputTokens > 0 || usage.CachedTokens > 0 || usage.CacheReadTokens > 0 || usage.CacheWriteTokens > 0
}

func cachedInputCount(usage Usage) (int64, bool) {
	cached := max64(usage.CachedTokens, usage.CacheReadTokens)
	if cached > usage.InputTokens || usage.CacheWriteTokens > usage.InputTokens-cached {
		return 0, false
	}
	return cached, true
}

func applyRates(result Estimate, usage Usage, cached int64, inputRate, cachedRate, writeRate, outputRate float64, note string) Estimate {
	uncached := usage.InputTokens - cached - usage.CacheWriteTokens
	divisor := float64(TokensPerPriceUnit)
	result.UncachedInputCost = float64(uncached) / divisor * inputRate
	result.CachedInputCost = float64(cached) / divisor * cachedRate
	result.CacheWriteCost = float64(usage.CacheWriteTokens) / divisor * writeRate
	result.OutputCost = float64(usage.OutputTokens) / divisor * outputRate
	result.Amount = result.UncachedInputCost + result.CachedInputCost + result.CacheWriteCost + result.OutputCost
	result.Status = StatusEstimated
	result.Note = note
	return result
}

func finishEstimate(result Estimate) Estimate {
	if math.IsNaN(result.Amount) || math.IsInf(result.Amount, 0) {
		result.Amount = 0
		result.Status, result.Note = StatusInvalid, NoteInvalidEstimate
	}
	return result
}

func unavailable(model, note string) Estimate {
	return Estimate{Currency: CurrencyUSD, Basis: BasisAPICost, Status: StatusUnpriced, Model: model, Note: note}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
