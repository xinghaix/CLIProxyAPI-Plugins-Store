package pricesync

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"unicode"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

const (
	ModelsDevURL  = "https://models.dev/api.json"
	LiteLLMURL    = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	OpenRouterURL = "https://openrouter.ai/api/v1/models"
	maxBodyBytes  = 8 << 20

	sourceModelsDev      = "models.dev"
	sourceModelsDevXAI   = "models.dev:xai"
	sourceModelsDevOR    = "models.dev:openrouter"
	sourceModelsDevOther = "models.dev:other"
	sourceLiteLLM        = "litellm"
	sourceOpenRouter     = "openrouter"
)

type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}
type Fetcher func(context.Context, string, http.Header) (HTTPResponse, error)
type SourceResult struct {
	Source   string `json:"source"`
	Priority int    `json:"priority,omitempty"`
	Models   int    `json:"models"`
	Matched  int    `json:"matched,omitempty"`
	Applied  int    `json:"applied,omitempty"`
	Skipped  int    `json:"skipped"`
	Error    string `json:"error,omitempty"`
}
type Candidate struct {
	Source        string      `json:"source"`
	SourceModelID string      `json:"sourceModelId"`
	Score         float64     `json:"score"`
	Reason        string      `json:"reason"`
	Price         store.Price `json:"price"`
}
type CandidateGroup struct {
	Model      string      `json:"model"`
	Candidates []Candidate `json:"candidates"`
}
type Result struct {
	Source          string                 `json:"source"`
	Sources         []string               `json:"sources"`
	Imported        int                    `json:"imported"`
	Skipped         int                    `json:"skipped"`
	ProtectedManual int                    `json:"protectedManual"`
	Matched         map[string]store.Price `json:"matched"`
	Candidates      []CandidateGroup       `json:"candidates"`
	Unmatched       []string               `json:"unmatched"`
	SourceResults   []SourceResult         `json:"sourceResults"`
	Prices          map[string]store.Price `json:"prices"`
}

type remotePrice struct {
	Source   string
	ID       string
	Key      string
	MatchIDs []string
	Priority int
	Present  priceFields
	Price    store.Price
}

type priceFields uint8

const (
	fieldPrompt priceFields = 1 << iota
	fieldCompletion
	fieldCache
	fieldCacheRead
	fieldCacheCreation
)

const (
	priorityModelsDevXAI   = 10
	priorityModelsDevOR    = 20
	priorityModelsDevOther = 30
	priorityLiteLLM        = 40
	priorityOpenRouter     = 50
)

// Run fetches public price catalogs in priority order. The first source owns a
// field when it provides it; lower-priority sources only fill missing fields.
func Run(ctx context.Context, targets []string, fetch Fetcher) (Result, error) {
	result := Result{
		Source:  "multi",
		Sources: []string{sourceModelsDevXAI, sourceModelsDevOR, sourceModelsDevOther, sourceLiteLLM, sourceOpenRouter},
		Matched: map[string]store.Price{},
	}
	if len(targets) == 0 {
		return result, nil
	}

	modelsDev, modelsDevResults := fetchModelsDev(ctx, fetch)
	lite, liteResult := fetchLiteLLM(ctx, fetch)
	openrouter, openResult := fetchOpenRouter(ctx, fetch)
	result.SourceResults = append(result.SourceResults, modelsDevResults...)
	result.SourceResults = append(result.SourceResults, liteResult, openResult)
	sort.SliceStable(result.SourceResults, func(i, j int) bool {
		return result.SourceResults[i].Priority < result.SourceResults[j].Priority
	})

	modelsDevFailed := sourceResultsFailed(modelsDevResults)
	if modelsDevFailed && liteResult.Error != "" && openResult.Error != "" {
		return result, fmt.Errorf("model price sync failed: models.dev: %s; LiteLLM: %s; OpenRouter: %s", sourceError(modelsDevResults), liteResult.Error, openResult.Error)
	}

	remote := merge(modelsDev, lite, openrouter)
	for _, target := range targets {
		if match, ok := exactMatch(target, remote); ok {
			result.Matched[target] = match.Price
			continue
		}
		candidates := fuzzyCandidates(target, remote)
		if len(candidates) > 0 {
			result.Candidates = append(result.Candidates, CandidateGroup{Model: target, Candidates: candidates})
			continue
		}
		result.Unmatched = append(result.Unmatched, target)
	}
	result.Skipped = liteResult.Skipped + openResult.Skipped
	for _, sourceResult := range modelsDevResults {
		result.Skipped += sourceResult.Skipped
	}
	recordMatchedSources(&result)
	return result, nil
}

// fetchModelsDev makes one request and splits the catalog into source groups so
// the UI can show the same priority that the matcher uses. xAI and OpenRouter
// get dedicated groups; all other providers are kept under one lower-priority
// group to avoid presenting hundreds of source pills.
func fetchModelsDev(ctx context.Context, fetch Fetcher) ([]remotePrice, []SourceResult) {
	response, err := fetch(ctx, ModelsDevURL, publicHeaders())
	if err != nil {
		return nil, []SourceResult{{Source: sourceModelsDev, Priority: priorityModelsDevXAI, Error: err.Error()}}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, []SourceResult{{Source: sourceModelsDev, Priority: priorityModelsDevXAI, Error: fmt.Sprintf("HTTP %d", response.StatusCode)}}
	}
	if len(response.Body) > maxBodyBytes {
		return nil, []SourceResult{{Source: sourceModelsDev, Priority: priorityModelsDevXAI, Error: "response exceeds 8 MiB"}}
	}

	var providers map[string]modelsDevProvider
	if err := json.Unmarshal(response.Body, &providers); err != nil {
		return nil, []SourceResult{{Source: sourceModelsDev, Priority: priorityModelsDevXAI, Error: "invalid JSON"}}
	}

	providerIDs := make([]string, 0, len(providers))
	for providerID := range providers {
		providerIDs = append(providerIDs, providerID)
	}
	sort.Strings(providerIDs)

	out := make([]remotePrice, 0)
	counts := map[string]*SourceResult{
		sourceModelsDevXAI:   {Source: sourceModelsDevXAI, Priority: priorityModelsDevXAI},
		sourceModelsDevOR:    {Source: sourceModelsDevOR, Priority: priorityModelsDevOR},
		sourceModelsDevOther: {Source: sourceModelsDevOther, Priority: priorityModelsDevOther},
	}
	for _, providerID := range providerIDs {
		provider := providers[providerID]
		providerID = strings.TrimSpace(providerID)
		if providerID == "" {
			continue
		}
		group := sourceModelsDevOther
		priority := priorityModelsDevOther
		if strings.EqualFold(providerID, "xai") {
			group = sourceModelsDevXAI
			priority = priorityModelsDevXAI
		} else if strings.EqualFold(providerID, "openrouter") {
			group = sourceModelsDevOR
			priority = priorityModelsDevOR
		}
		modelIDs := make([]string, 0, len(provider.Models))
		for modelID := range provider.Models {
			modelIDs = append(modelIDs, modelID)
		}
		sort.Strings(modelIDs)
		for _, mapModelID := range modelIDs {
			model := provider.Models[mapModelID]
			modelID := strings.TrimSpace(model.ID)
			if modelID == "" {
				modelID = strings.TrimSpace(mapModelID)
			}
			if modelID == "" {
				counts[group].Skipped++
				continue
			}
			price, present, ok := modelsDevPrice(model.Cost)
			if !ok {
				counts[group].Skipped++
				continue
			}
			sourceModelID := providerID + "/" + modelID
			matchIDs := []string{sourceModelID}
			if strings.EqualFold(providerID, "xai") || strings.EqualFold(providerID, "openrouter") {
				matchIDs = append(matchIDs, modelID)
			}
			price.Source = group
			price.SourceModelID = sourceModelID
			out = append(out, remotePrice{
				Source:   group,
				ID:       sourceModelID,
				Key:      logicalModelKey(providerID, modelID),
				MatchIDs: uniqueStrings(matchIDs),
				Priority: priority,
				Present:  present,
				Price:    price,
			})
			counts[group].Models++
		}
	}

	results := []SourceResult{*counts[sourceModelsDevXAI], *counts[sourceModelsDevOR], *counts[sourceModelsDevOther]}
	return out, results
}

type modelsDevProvider struct {
	ID     string                    `json:"id"`
	Models map[string]modelsDevModel `json:"models"`
}

type modelsDevModel struct {
	ID   string         `json:"id"`
	Cost *modelsDevCost `json:"cost"`
}

type modelsDevCost struct {
	Input      *float64 `json:"input"`
	Output     *float64 `json:"output"`
	CacheRead  *float64 `json:"cache_read"`
	CacheWrite *float64 `json:"cache_write"`
}

func modelsDevPrice(cost *modelsDevCost) (store.Price, priceFields, bool) {
	if cost == nil {
		return store.Price{}, 0, false
	}
	var price store.Price
	var present priceFields
	assign := func(value *float64, target *float64, field priceFields) bool {
		if value == nil {
			return true
		}
		if *value < 0 || math.IsInf(*value, 0) || math.IsNaN(*value) {
			return false
		}
		*target = *value
		present |= field
		return true
	}
	if !assign(cost.Input, &price.Prompt, fieldPrompt) ||
		!assign(cost.Output, &price.Completion, fieldCompletion) ||
		!assign(cost.CacheRead, &price.CacheRead, fieldCacheRead) ||
		!assign(cost.CacheWrite, &price.CacheCreation, fieldCacheCreation) {
		return store.Price{}, 0, false
	}
	return price, present, present != 0
}

func fetchLiteLLM(ctx context.Context, fetch Fetcher) ([]remotePrice, SourceResult) {
	response, err := fetch(ctx, LiteLLMURL, publicHeaders())
	if err != nil {
		return nil, SourceResult{Source: sourceLiteLLM, Priority: priorityLiteLLM, Error: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, SourceResult{Source: sourceLiteLLM, Priority: priorityLiteLLM, Error: fmt.Sprintf("HTTP %d", response.StatusCode)}
	}
	if len(response.Body) > maxBodyBytes {
		return nil, SourceResult{Source: sourceLiteLLM, Priority: priorityLiteLLM, Error: "response exceeds 8 MiB"}
	}
	var models map[string]map[string]json.RawMessage
	if err := json.Unmarshal(response.Body, &models); err != nil {
		return nil, SourceResult{Source: sourceLiteLLM, Priority: priorityLiteLLM, Error: "invalid JSON"}
	}
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := []remotePrice{}
	skipped := 0
	for _, id := range ids {
		price, present, ok := litePrice(models[id])
		if !ok {
			skipped++
			continue
		}
		price.Source = sourceLiteLLM
		price.SourceModelID = id
		out = append(out, remotePrice{Source: sourceLiteLLM, ID: id, Key: logicalModelKey("", id), MatchIDs: []string{id}, Priority: priorityLiteLLM, Present: present, Price: price})
	}
	return out, SourceResult{Source: sourceLiteLLM, Priority: priorityLiteLLM, Models: len(out), Skipped: skipped}
}

func fetchOpenRouter(ctx context.Context, fetch Fetcher) ([]remotePrice, SourceResult) {
	response, err := fetch(ctx, OpenRouterURL, publicHeaders())
	if err != nil {
		return nil, SourceResult{Source: sourceOpenRouter, Priority: priorityOpenRouter, Error: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, SourceResult{Source: sourceOpenRouter, Priority: priorityOpenRouter, Error: fmt.Sprintf("HTTP %d", response.StatusCode)}
	}
	if len(response.Body) > maxBodyBytes {
		return nil, SourceResult{Source: sourceOpenRouter, Priority: priorityOpenRouter, Error: "response exceeds 8 MiB"}
	}
	var payload struct {
		Data []struct {
			ID      string                     `json:"id"`
			Pricing map[string]json.RawMessage `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, SourceResult{Source: sourceOpenRouter, Priority: priorityOpenRouter, Error: "invalid JSON"}
	}
	out := []remotePrice{}
	skipped := 0
	for _, model := range payload.Data {
		if model.ID == "" {
			skipped++
			continue
		}
		price, present, ok := openRouterPrice(model.Pricing)
		if !ok {
			skipped++
			continue
		}
		price.Source = sourceOpenRouter
		price.SourceModelID = model.ID
		out = append(out, remotePrice{Source: sourceOpenRouter, ID: model.ID, Key: logicalModelKey("", model.ID), MatchIDs: []string{model.ID}, Priority: priorityOpenRouter, Present: present, Price: price})
	}
	return out, SourceResult{Source: sourceOpenRouter, Priority: priorityOpenRouter, Models: len(out), Skipped: skipped}
}

func publicHeaders() http.Header {
	return http.Header{"Accept": []string{"application/json"}, "User-Agent": []string{"cpa-manager-plus/0.5.20"}}
}

func litePrice(fields map[string]json.RawMessage) (store.Price, priceFields, bool) {
	return priceFrom(fields, map[string][]string{"prompt": {"input_cost_per_token"}, "completion": {"output_cost_per_token"}, "cache": {"cache_read_input_token_cost", "input_cache_read"}, "cacheRead": {"cache_read_input_token_cost", "input_cache_read"}, "cacheCreation": {"cache_creation_input_token_cost", "cache_write_input_token_cost", "input_cache_write", "input_cache_creation"}})
}

func openRouterPrice(fields map[string]json.RawMessage) (store.Price, priceFields, bool) {
	return priceFrom(fields, map[string][]string{"prompt": {"prompt"}, "completion": {"completion"}, "cache": {"input_cache_read"}, "cacheRead": {"input_cache_read"}, "cacheCreation": {"input_cache_write", "input_cache_creation"}})
}

func priceFrom(fields map[string]json.RawMessage, names map[string][]string) (store.Price, priceFields, bool) {
	var p store.Price
	var present priceFields
	for field, keys := range names {
		value, ok := firstNumber(fields, keys)
		if !ok {
			continue
		}
		if value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
			return store.Price{}, 0, false
		}
		value *= 1_000_000
		switch field {
		case "prompt":
			p.Prompt = value
			present |= fieldPrompt
		case "completion":
			p.Completion = value
			present |= fieldCompletion
		case "cache":
			p.Cache = value
			present |= fieldCache
		case "cacheRead":
			p.CacheRead = value
			present |= fieldCacheRead
		case "cacheCreation":
			p.CacheCreation = value
			present |= fieldCacheCreation
		}
	}
	return p, present, present != 0
}

func firstNumber(fields map[string]json.RawMessage, keys []string) (float64, bool) {
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		var number json.Number
		if err := json.Unmarshal(raw, &number); err == nil {
			value, err := number.Float64()
			if err == nil {
				return value, true
			}
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			var value float64
			if _, err := fmt.Sscan(text, &value); err == nil {
				return value, true
			}
		}
	}
	return 0, false
}

func merge(groups ...[]remotePrice) []remotePrice {
	out := []remotePrice{}
	byKey := map[string]int{}
	for _, group := range groups {
		for _, price := range group {
			key := price.Key
			if key == "" {
				key = logicalModelKey("", price.ID)
			}
			if index, ok := byKey[key]; ok {
				if price.Priority < out[index].Priority {
					price.Price = fillMissing(price.Price, out[index].Price, price.Present, out[index].Present)
					price.Present |= out[index].Present
					price.MatchIDs = uniqueStrings(append(price.MatchIDs, out[index].MatchIDs...))
					out[index] = price
				} else {
					out[index].Price = fillMissing(out[index].Price, price.Price, out[index].Present, price.Present)
					out[index].Present |= price.Present
					out[index].MatchIDs = uniqueStrings(append(out[index].MatchIDs, price.MatchIDs...))
				}
				continue
			}
			if len(price.MatchIDs) == 0 {
				price.MatchIDs = []string{price.ID}
			}
			out = append(out, price)
			byKey[key] = len(out) - 1
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority
		}
		if out[i].Source != out[j].Source {
			return out[i].Source < out[j].Source
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func fillMissing(primary, fallback store.Price, primaryPresent, fallbackPresent priceFields) store.Price {
	if primaryPresent&fieldPrompt == 0 && fallbackPresent&fieldPrompt != 0 {
		primary.Prompt = fallback.Prompt
	}
	if primaryPresent&fieldCompletion == 0 && fallbackPresent&fieldCompletion != 0 {
		primary.Completion = fallback.Completion
	}
	if primaryPresent&fieldCache == 0 && fallbackPresent&fieldCache != 0 {
		primary.Cache = fallback.Cache
	}
	if primaryPresent&fieldCacheRead == 0 && fallbackPresent&fieldCacheRead != 0 {
		primary.CacheRead = fallback.CacheRead
	}
	if primaryPresent&fieldCacheCreation == 0 && fallbackPresent&fieldCacheCreation != 0 {
		primary.CacheCreation = fallback.CacheCreation
	}
	return primary
}

func exactMatch(target string, remote []remotePrice) (remotePrice, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return remotePrice{}, false
	}
	for _, entry := range remote {
		for _, id := range entry.MatchIDs {
			if id == target {
				return entry, true
			}
		}
	}
	matches := []remotePrice{}
	for _, entry := range remote {
		for _, id := range entry.MatchIDs {
			if strings.EqualFold(id, target) {
				matches = append(matches, entry)
				break
			}
		}
	}
	if len(matches) > 0 {
		return bestRemoteMatch(matches), true
	}
	matches = nil
	key := canonical(target)
	for _, entry := range remote {
		for _, id := range entry.MatchIDs {
			if canonical(id) == key {
				matches = append(matches, entry)
				break
			}
		}
	}
	if len(matches) > 0 {
		return bestRemoteMatch(matches), true
	}
	return remotePrice{}, false
}

func bestRemoteMatch(matches []remotePrice) remotePrice {
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority < matches[j].Priority
		}
		if matches[i].Source != matches[j].Source {
			return matches[i].Source < matches[j].Source
		}
		return matches[i].ID < matches[j].ID
	})
	return matches[0]
}

func fuzzyCandidates(target string, remote []remotePrice) []Candidate {
	needle := canonical(target)
	matches := []Candidate{}
	seen := map[string]bool{}
	for _, entry := range remote {
		bestScore := 0.0
		for _, id := range entry.MatchIDs {
			key := canonical(id)
			if needle == "" || key == "" || (!strings.Contains(key, needle) && !strings.Contains(needle, key)) {
				continue
			}
			score := float64(min(len(needle), len(key))) / float64(max(len(needle), len(key)))
			if score > bestScore {
				bestScore = score
			}
		}
		if bestScore < .55 {
			continue
		}
		candidateKey := entry.Source + "\x00" + entry.ID
		if seen[candidateKey] {
			continue
		}
		seen[candidateKey] = true
		matches = append(matches, Candidate{Source: entry.Price.Source, SourceModelID: entry.Price.SourceModelID, Score: bestScore, Reason: "canonical-name-contains", Price: entry.Price})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Source != matches[j].Source {
			return matches[i].Source < matches[j].Source
		}
		return matches[i].SourceModelID < matches[j].SourceModelID
	})
	if len(matches) > 8 {
		matches = matches[:8]
	}
	return matches
}

func recordMatchedSources(result *Result) {
	for _, price := range result.Matched {
		for index := range result.SourceResults {
			if strings.EqualFold(result.SourceResults[index].Source, price.Source) {
				result.SourceResults[index].Matched++
				break
			}
		}
	}
}

func sourceResultsFailed(results []SourceResult) bool {
	if len(results) == 0 {
		return true
	}
	for _, result := range results {
		if result.Error == "" {
			return false
		}
	}
	return true
}

func sourceError(results []SourceResult) string {
	for _, result := range results {
		if result.Error != "" {
			return result.Error
		}
	}
	return "unavailable"
}

func logicalModelKey(providerID, modelID string) string {
	providerID = strings.ToLower(strings.TrimSpace(providerID))
	modelID = strings.TrimSpace(modelID)
	if providerID == "xai" {
		return canonical(modelID)
	}
	if providerID == "openrouter" {
		return logicalModelKey("", modelID)
	}
	for _, prefix := range []string{"xai/", "x-ai/"} {
		if strings.HasPrefix(strings.ToLower(modelID), prefix) {
			return canonical(modelID[len(prefix):])
		}
	}
	if providerID == "" {
		return canonical(modelID)
	}
	return canonical(providerID + "/" + modelID)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func canonical(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}
