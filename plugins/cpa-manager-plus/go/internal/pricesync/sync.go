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
	LiteLLMURL    = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
	OpenRouterURL = "https://openrouter.ai/api/v1/models"
	maxBodyBytes  = 8 << 20
)

type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}
type Fetcher func(context.Context, string, http.Header) (HTTPResponse, error)
type SourceResult struct {
	Source  string `json:"source"`
	Models  int    `json:"models"`
	Skipped int    `json:"skipped"`
	Error   string `json:"error,omitempty"`
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
	Source, ID string
	Price      store.Price
}

func Run(ctx context.Context, targets []string, fetch Fetcher) (Result, error) {
	result := Result{Source: "multi", Sources: []string{"litellm", "openrouter"}, Matched: map[string]store.Price{}}
	if len(targets) == 0 {
		return result, nil
	}
	lite, liteResult := fetchLiteLLM(ctx, fetch)
	openrouter, openResult := fetchOpenRouter(ctx, fetch)
	result.SourceResults = []SourceResult{liteResult, openResult}
	if liteResult.Error != "" && openResult.Error != "" {
		return result, fmt.Errorf("model price sync failed: LiteLLM: %s; OpenRouter: %s", liteResult.Error, openResult.Error)
	}
	remote := merge(lite, openrouter)
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
	return result, nil
}

func fetchLiteLLM(ctx context.Context, fetch Fetcher) ([]remotePrice, SourceResult) {
	response, err := fetch(ctx, LiteLLMURL, publicHeaders())
	if err != nil {
		return nil, SourceResult{Source: "litellm", Error: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, SourceResult{Source: "litellm", Error: fmt.Sprintf("HTTP %d", response.StatusCode)}
	}
	if len(response.Body) > maxBodyBytes {
		return nil, SourceResult{Source: "litellm", Error: "response exceeds 8 MiB"}
	}
	var models map[string]map[string]json.RawMessage
	if err := json.Unmarshal(response.Body, &models); err != nil {
		return nil, SourceResult{Source: "litellm", Error: "invalid JSON"}
	}
	out := []remotePrice{}
	skipped := 0
	for id, fields := range models {
		price, ok := litePrice(fields)
		if !ok {
			skipped++
			continue
		}
		price.Source = "litellm"
		price.SourceModelID = id
		out = append(out, remotePrice{Source: "litellm", ID: id, Price: price})
	}
	return out, SourceResult{Source: "litellm", Models: len(out), Skipped: skipped}
}

func fetchOpenRouter(ctx context.Context, fetch Fetcher) ([]remotePrice, SourceResult) {
	response, err := fetch(ctx, OpenRouterURL, publicHeaders())
	if err != nil {
		return nil, SourceResult{Source: "openrouter", Error: err.Error()}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, SourceResult{Source: "openrouter", Error: fmt.Sprintf("HTTP %d", response.StatusCode)}
	}
	if len(response.Body) > maxBodyBytes {
		return nil, SourceResult{Source: "openrouter", Error: "response exceeds 8 MiB"}
	}
	var payload struct {
		Data []struct {
			ID      string                     `json:"id"`
			Pricing map[string]json.RawMessage `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body, &payload); err != nil {
		return nil, SourceResult{Source: "openrouter", Error: "invalid JSON"}
	}
	out := []remotePrice{}
	skipped := 0
	for _, model := range payload.Data {
		if model.ID == "" {
			skipped++
			continue
		}
		price, ok := openRouterPrice(model.Pricing)
		if !ok {
			skipped++
			continue
		}
		price.Source = "openrouter"
		price.SourceModelID = model.ID
		out = append(out, remotePrice{Source: "openrouter", ID: model.ID, Price: price})
	}
	return out, SourceResult{Source: "openrouter", Models: len(out), Skipped: skipped}
}

func publicHeaders() http.Header {
	return http.Header{"Accept": []string{"application/json"}, "User-Agent": []string{"cpa-manager-plus/0.4"}}
}
func litePrice(fields map[string]json.RawMessage) (store.Price, bool) {
	return priceFrom(fields, map[string][]string{"prompt": {"input_cost_per_token"}, "completion": {"output_cost_per_token"}, "cache": {"cache_read_input_token_cost", "input_cache_read"}, "cacheRead": {"cache_read_input_token_cost", "input_cache_read"}, "cacheCreation": {"cache_creation_input_token_cost", "cache_write_input_token_cost", "input_cache_write", "input_cache_creation"}})
}
func openRouterPrice(fields map[string]json.RawMessage) (store.Price, bool) {
	return priceFrom(fields, map[string][]string{"prompt": {"prompt"}, "completion": {"completion"}, "cache": {"input_cache_read"}, "cacheRead": {"input_cache_read"}, "cacheCreation": {"input_cache_write", "input_cache_creation"}})
}
func priceFrom(fields map[string]json.RawMessage, names map[string][]string) (store.Price, bool) {
	var p store.Price
	found := false
	for field, keys := range names {
		value, ok := firstNumber(fields, keys)
		if !ok {
			continue
		}
		if value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
			return store.Price{}, false
		}
		found = true
		value *= 1_000_000
		switch field {
		case "prompt":
			p.Prompt = value
		case "completion":
			p.Completion = value
		case "cache":
			p.Cache = value
		case "cacheRead":
			p.CacheRead = value
		case "cacheCreation":
			p.CacheCreation = value
		}
	}
	return p, found
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
func merge(primary, secondary []remotePrice) []remotePrice {
	out := append([]remotePrice(nil), primary...)
	byCanonical := map[string]int{}
	for index, price := range out {
		byCanonical[canonical(price.ID)] = index
	}
	for _, price := range secondary {
		if index, ok := byCanonical[canonical(price.ID)]; ok {
			out[index].Price = fillMissing(out[index].Price, price.Price)
			continue
		}
		out = append(out, price)
	}
	return out
}
func fillMissing(primary, fallback store.Price) store.Price {
	if primary.Prompt == 0 {
		primary.Prompt = fallback.Prompt
	}
	if primary.Completion == 0 {
		primary.Completion = fallback.Completion
	}
	if primary.Cache == 0 {
		primary.Cache = fallback.Cache
	}
	if primary.CacheRead == 0 {
		primary.CacheRead = fallback.CacheRead
	}
	if primary.CacheCreation == 0 {
		primary.CacheCreation = fallback.CacheCreation
	}
	return primary
}
func exactMatch(target string, remote []remotePrice) (remotePrice, bool) {
	for _, entry := range remote {
		if entry.ID == target {
			return entry, true
		}
	}
	matches := []remotePrice{}
	for _, entry := range remote {
		if strings.EqualFold(entry.ID, target) {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	matches = nil
	key := canonical(target)
	for _, entry := range remote {
		if canonical(entry.ID) == key {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return remotePrice{}, false
}
func fuzzyCandidates(target string, remote []remotePrice) []Candidate {
	needle := canonical(target)
	matches := []Candidate{}
	for _, entry := range remote {
		key := canonical(entry.ID)
		if needle == "" || key == "" {
			continue
		}
		if strings.Contains(key, needle) || strings.Contains(needle, key) {
			score := float64(min(len(needle), len(key))) / float64(max(len(needle), len(key)))
			if score >= .55 {
				matches = append(matches, Candidate{Source: entry.Source, SourceModelID: entry.ID, Score: score, Reason: "canonical-name-contains", Price: entry.Price})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })
	if len(matches) > 8 {
		matches = matches[:8]
	}
	return matches
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
