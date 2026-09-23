package store

import "github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricing"

func estimateEventCost(row eventRow, price Price) pricing.Estimate {
	var fallback *pricing.FlatRates
	if price.Prompt != 0 || price.Completion != 0 || price.Cache != 0 || price.CacheRead != 0 || price.CacheCreation != 0 {
		fallback = &pricing.FlatRates{
			Input:         price.Prompt,
			Output:        price.Completion,
			CachedInput:   price.Cache,
			CacheRead:     price.CacheRead,
			CacheCreation: price.CacheCreation,
		}
	}
	return pricing.EstimateCost(pricing.Usage{
		Model:                    row.Model,
		InputTokens:              row.InputTokens,
		OutputTokens:             row.OutputTokens,
		CachedTokens:             row.CachedTokens,
		CacheReadTokens:          row.CacheReadTokens,
		CacheWriteTokens:         row.CacheCreationTokens,
		ServiceTier:              row.ServiceTier,
		ResponseServiceTier:      row.ObservedServiceTier,
		ResponseTierAmbiguous:    row.ResponseTierAmbiguous,
		ResponseObservationCount: row.ResponseUsageCount,
		ResponseObservationError: row.ResponseObservationAmbiguous,
	}, fallback)
}

func (s *stats) addEstimate(estimate pricing.Estimate) {
	if estimate.Status == pricing.StatusEstimated {
		s.Cost += estimate.Amount
		s.PricedCalls++
		return
	}
	s.UnpricedCalls++
}

func displayedCost(estimate pricing.Estimate) any {
	if estimate.Status != pricing.StatusEstimated {
		return nil
	}
	return estimate.Amount
}

func displayedResponseTier(row eventRow) string {
	if row.ResponseObservationAmbiguous || row.ResponseTierAmbiguous || row.ResponseUsageCount != 1 {
		return ""
	}
	return row.ObservedServiceTier
}
