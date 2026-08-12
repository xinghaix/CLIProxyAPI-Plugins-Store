package pricesync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

// LookupResult contains all exact or canonical source prices for one local model.
type LookupResult struct {
	Model   string         `json:"model"`
	Sources []LookupSource `json:"sources"`
}

// LookupSource is a selectable price from one public catalog.
type LookupSource struct {
	Source        string      `json:"source"`
	SourceModelID string      `json:"sourceModelId"`
	Priority      int         `json:"priority,omitempty"`
	Available     []string    `json:"available,omitempty"`
	Price         store.Price `json:"price"`
}

// Lookup fetches the public catalogs and returns only exact/canonical matches
// for the requested model. It never writes to the local price store.
func Lookup(ctx context.Context, model string, fetch Fetcher) (LookupResult, error) {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 256 {
		return LookupResult{}, fmt.Errorf("model is required")
	}

	modelsDev, modelsDevResults := fetchModelsDev(ctx, fetch)
	lite, liteResult := fetchLiteLLM(ctx, fetch)
	openrouter, openResult := fetchOpenRouter(ctx, fetch)
	if sourceResultsFailed(modelsDevResults) && liteResult.Error != "" && openResult.Error != "" {
		return LookupResult{}, fmt.Errorf("model price lookup failed: models.dev: %s; LiteLLM: %s; OpenRouter: %s", sourceError(modelsDevResults), liteResult.Error, openResult.Error)
	}

	remote := append(append(append([]remotePrice{}, modelsDev...), lite...), openrouter...)
	matches := exactRemoteMatches(model, remote)
	result := LookupResult{Model: model, Sources: make([]LookupSource, 0, len(matches))}
	for _, match := range matches {
		result.Sources = append(result.Sources, LookupSource{
			Source:        match.Price.Source,
			SourceModelID: match.Price.SourceModelID,
			Priority:      match.Priority,
			Available:     availablePriceFields(match.Present),
			Price:         match.Price,
		})
	}
	return result, nil
}

func availablePriceFields(fields priceFields) []string {
	result := make([]string, 0, 5)
	if fields&fieldPrompt != 0 {
		result = append(result, "prompt")
	}
	if fields&fieldCompletion != 0 {
		result = append(result, "completion")
	}
	if fields&fieldCache != 0 {
		result = append(result, "cache")
	}
	if fields&fieldCacheRead != 0 {
		result = append(result, "cacheRead")
	}
	if fields&fieldCacheCreation != 0 {
		result = append(result, "cacheCreation")
	}
	return result
}

func exactRemoteMatches(target string, remote []remotePrice) []remotePrice {
	matches := make([]remotePrice, 0)
	seen := map[string]bool{}
	add := func(entry remotePrice) {
		key := entry.Price.Source + "\x00" + entry.Price.SourceModelID
		if seen[key] {
			return
		}
		seen[key] = true
		matches = append(matches, entry)
	}
	for _, entry := range remote {
		for _, id := range entry.MatchIDs {
			if id == target {
				add(entry)
				break
			}
		}
	}
	if len(matches) == 0 {
		for _, entry := range remote {
			for _, id := range entry.MatchIDs {
				if strings.EqualFold(id, target) {
					add(entry)
					break
				}
			}
		}
	}
	if len(matches) == 0 {
		key := canonical(target)
		for _, entry := range remote {
			for _, id := range entry.MatchIDs {
				if canonical(id) == key {
					add(entry)
					break
				}
			}
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority < matches[j].Priority
		}
		if matches[i].Source != matches[j].Source {
			return matches[i].Source < matches[j].Source
		}
		return matches[i].Price.SourceModelID < matches[j].Price.SourceModelID
	})
	return matches
}
