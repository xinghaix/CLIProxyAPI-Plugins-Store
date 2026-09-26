package pricing

import "strings"

// Cache input accounting modes. Portions adapted from CPA-Manager-Plus (MIT, Seakee).
const (
	CacheInputModeIncluded                     = "included_in_input"
	CacheInputModeSeparate                     = "separate_from_input"
	CacheInputModeReadIncludedCreationSeparate = "read_included_creation_separate"
)

// CacheAccounting is the normalized token breakdown used for billing aggregates.
type CacheAccounting struct {
	Mode                string
	UncachedInputTokens int64
	TotalInputTokens    int64
	CacheReadTokens     int64
	CacheCreationTokens int64
}

// CacheInputContext supplies identity hints for InferCacheInputMode.
type CacheInputContext struct {
	ExplicitMode     string
	ExecutorType     string
	Provider         string
	ProviderSnapshot string
	AuthType         string
	ResolvedModel    string
	RequestedModel   string
	DisplayModel     string
}

// CompatibleCachedTokens returns residual OpenAI-style cached tokens after
// fine-grained cache_read/cache_creation have been removed.
func CompatibleCachedTokens(cachedTokens, cacheTokens, cacheReadTokens, cacheCreationTokens int64) int64 {
	cached := cachedTokens
	if cacheTokens > cached {
		cached = cacheTokens
	}
	if cached <= 0 {
		return 0
	}
	fineGrained := int64(0)
	if cacheReadTokens > 0 {
		fineGrained += cacheReadTokens
	}
	if cacheCreationTokens > 0 {
		fineGrained += cacheCreationTokens
	}
	if cached <= fineGrained {
		return 0
	}
	return cached - fineGrained
}

// NormalizeCacheAccounting converts provider-specific cache reporting into a
// consistent uncached/total/read/creation breakdown for pricing.
func NormalizeCacheAccounting(context CacheInputContext, inputTokens, cachedTokens, cacheTokens, cacheReadTokens, cacheCreationTokens int64) CacheAccounting {
	mode := InferCacheInputMode(context, cacheReadTokens, cacheCreationTokens)
	input := maxInt64(inputTokens, 0)
	cacheRead := CompatibleCachedTokens(cachedTokens, cacheTokens, cacheReadTokens, cacheCreationTokens) + maxInt64(cacheReadTokens, 0)
	cacheCreation := maxInt64(cacheCreationTokens, 0)
	accounting := CacheAccounting{
		Mode:                mode,
		CacheReadTokens:     cacheRead,
		CacheCreationTokens: cacheCreation,
	}
	switch mode {
	case CacheInputModeSeparate:
		accounting.UncachedInputTokens = input
		accounting.TotalInputTokens = input + cacheRead + cacheCreation
	case CacheInputModeReadIncludedCreationSeparate:
		accounting.UncachedInputTokens = maxInt64(input-cacheRead, 0)
		accounting.TotalInputTokens = input + cacheCreation
	default:
		accounting.UncachedInputTokens = maxInt64(input-cacheRead-cacheCreation, 0)
		accounting.TotalInputTokens = input
	}
	return accounting
}

// InferCacheInputMode picks included / separate / hybrid based on identity hints.
func InferCacheInputMode(context CacheInputContext, cacheReadTokens, cacheCreationTokens int64) string {
	mode := normalizeCacheInputMode(context.ExplicitMode)
	if mode == CacheInputModeIncluded || mode == CacheInputModeSeparate || mode == CacheInputModeReadIncludedCreationSeparate {
		return mode
	}
	if classified, ok := classifyExecutorCacheInputMode(context.ExecutorType); ok {
		return classified
	}
	for _, provider := range []string{context.Provider, context.ProviderSnapshot} {
		if classified, ok := classifyProviderCacheInputMode(provider); ok {
			return classified
		}
	}
	for _, model := range []string{context.ResolvedModel, context.RequestedModel, context.DisplayModel} {
		if classified, ok := classifyModelCacheInputMode(model); ok {
			return classified
		}
	}
	if cacheReadTokens > 0 || cacheCreationTokens > 0 {
		return CacheInputModeSeparate
	}
	return CacheInputModeIncluded
}

func normalizeCacheInputMode(mode string) string {
	return strings.ToLower(strings.TrimSpace(mode))
}

func classifyExecutorCacheInputMode(executorType string) (string, bool) {
	executor := strings.ToLower(strings.TrimSpace(executorType))
	if executor == "" {
		return "", false
	}
	if executor == "devinexecutor" {
		return CacheInputModeReadIncludedCreationSeparate, true
	}
	if strings.Contains(executor, "claude") {
		return CacheInputModeSeparate, true
	}
	for _, marker := range []string{
		"openaicompat", "openai_compat", "openai-compat", "openai",
		"codex", "gemini", "aistudio", "ai_studio", "ai-studio",
		"antigravity", "xai", "kimi",
	} {
		if strings.Contains(executor, marker) {
			return CacheInputModeIncluded, true
		}
	}
	return "", false
}

func classifyProviderCacheInputMode(provider string) (string, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return "", false
	}
	if provider == "devin" || strings.HasPrefix(provider, "devin/") {
		return CacheInputModeReadIncludedCreationSeparate, true
	}
	if strings.Contains(provider, "anthropic") || strings.Contains(provider, "claude") {
		return CacheInputModeSeparate, true
	}
	for _, marker := range []string{
		"openai", "codex", "gemini", "vertex", "aistudio", "ai_studio",
		"ai-studio", "interaction", "antigravity", "xai", "kimi", "moonshot",
	} {
		if strings.Contains(provider, marker) {
			return CacheInputModeIncluded, true
		}
	}
	return "", false
}

func classifyModelCacheInputMode(model string) (string, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return "", false
	}
	if model == "devin" || strings.HasPrefix(model, "devin/") {
		return CacheInputModeReadIncludedCreationSeparate, true
	}
	if strings.Contains(model, "anthropic") || strings.Contains(model, "claude") {
		return CacheInputModeSeparate, true
	}
	for _, marker := range []string{
		"gpt-", "openai", "codex", "gemini", "vertex", "aistudio",
		"antigravity", "grok", "xai", "kimi", "moonshot",
	} {
		if strings.Contains(model, marker) {
			return CacheInputModeIncluded, true
		}
	}
	return "", false
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

// UsageFromNormalized builds an EstimateCost Usage from normalized cache accounting.
// Total input is always treated as an OpenAI-style included total so the existing
// official schedule / flat estimator can price Claude-separate and hybrid modes.
func UsageFromNormalized(model string, accounting CacheAccounting, outputTokens int64, serviceTier, responseServiceTier string, responseTierAmbiguous bool, responseObservationCount int64, responseObservationError bool) Usage {
	return Usage{
		Model:                    model,
		InputTokens:              accounting.TotalInputTokens,
		OutputTokens:             maxInt64(outputTokens, 0),
		CachedTokens:             accounting.CacheReadTokens,
		CacheReadTokens:          accounting.CacheReadTokens,
		CacheWriteTokens:         accounting.CacheCreationTokens,
		ServiceTier:              serviceTier,
		ResponseServiceTier:      responseServiceTier,
		ResponseTierAmbiguous:    responseTierAmbiguous,
		ResponseObservationCount: responseObservationCount,
		ResponseObservationError: responseObservationError,
	}
}
