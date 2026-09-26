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
	accounting := pricing.NormalizeCacheAccounting(pricing.CacheInputContext{
		ExecutorType:     row.ExecutorType,
		Provider:         row.Provider,
		ProviderSnapshot: row.Provider,
		AuthType:         row.AuthType,
		ResolvedModel:    row.Model,
		RequestedModel:   requestedModel(row),
		DisplayModel:     row.Model,
	}, row.InputTokens, row.CachedTokens, 0, row.CacheReadTokens, row.CacheCreationTokens)
	return pricing.EstimateCost(pricing.UsageFromNormalized(
		row.Model,
		accounting,
		row.OutputTokens,
		row.ServiceTier,
		row.ObservedServiceTier,
		row.ResponseTierAmbiguous,
		row.ResponseUsageCount,
		row.ResponseObservationAmbiguous,
	), fallback)
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
