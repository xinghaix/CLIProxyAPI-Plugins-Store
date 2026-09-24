package pricing

import "testing"

func TestNormalizeCacheAccounting(t *testing.T) {
	tests := []struct {
		name      string
		context   CacheInputContext
		input     int64
		cached    int64
		read      int64
		creation  int64
		wantMode  string
		wantInput int64
		wantTotal int64
		wantRead  int64
	}{
		{name: "openai mirror is included", context: CacheInputContext{Provider: "openai", DisplayModel: "gpt-5.4"}, input: 1_000, cached: 400, read: 400, wantMode: CacheInputModeIncluded, wantInput: 600, wantTotal: 1_000, wantRead: 400},
		{name: "gpt 5.6 read and write are included", context: CacheInputContext{ExplicitMode: CacheInputModeIncluded, DisplayModel: "gpt-5.6-sol"}, input: 1_000, read: 300, creation: 100, wantMode: CacheInputModeIncluded, wantInput: 600, wantTotal: 1_000, wantRead: 300},
		{name: "claude cache is separate", context: CacheInputContext{ExplicitMode: CacheInputModeSeparate, DisplayModel: "claude-sonnet-4"}, input: 100, read: 300, creation: 50, wantMode: CacheInputModeSeparate, wantInput: 100, wantTotal: 450, wantRead: 300},
		{name: "devin hybrid", context: CacheInputContext{Provider: "devin"}, input: 229788, read: 228021, creation: 1775, wantMode: CacheInputModeReadIncludedCreationSeparate, wantInput: 1767, wantTotal: 231563, wantRead: 228021},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCacheAccounting(tt.context, tt.input, tt.cached, 0, tt.read, tt.creation)
			if got.Mode != tt.wantMode || got.UncachedInputTokens != tt.wantInput || got.TotalInputTokens != tt.wantTotal || got.CacheReadTokens != tt.wantRead {
				t.Fatalf("accounting = %+v, want mode=%s input=%d total=%d read=%d", got, tt.wantMode, tt.wantInput, tt.wantTotal, tt.wantRead)
			}
		})
	}
}

func TestInferCacheInputModePriority(t *testing.T) {
	tests := []struct {
		name    string
		context CacheInputContext
		want    string
	}{
		{name: "explicit wins", context: CacheInputContext{ExplicitMode: CacheInputModeSeparate, ExecutorType: "OpenAICompatExecutor"}, want: CacheInputModeSeparate},
		{name: "devin executor", context: CacheInputContext{ExecutorType: "DevinExecutor"}, want: CacheInputModeReadIncludedCreationSeparate},
		{name: "claude executor", context: CacheInputContext{ExecutorType: "ClaudeExecutor", ResolvedModel: "grok-4"}, want: CacheInputModeSeparate},
		{name: "openai compat beats claude alias", context: CacheInputContext{ExecutorType: "OpenAICompatExecutor", ResolvedModel: "claude-sonnet-4"}, want: CacheInputModeIncluded},
		{name: "anthropic provider", context: CacheInputContext{Provider: "anthropic"}, want: CacheInputModeSeparate},
		{name: "codex provider", context: CacheInputContext{Provider: "codex"}, want: CacheInputModeIncluded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferCacheInputMode(tt.context, 10, 0); got != tt.want {
				t.Fatalf("mode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestUsageFromNormalizedSeparateIsBillable(t *testing.T) {
	accounting := NormalizeCacheAccounting(CacheInputContext{ExplicitMode: CacheInputModeSeparate}, 100, 0, 0, 300, 50)
	usage := UsageFromNormalized("claude-sonnet-4", accounting, 20, "", "", false, 0, false)
	if usage.InputTokens != 450 || usage.CacheReadTokens != 300 || usage.CacheWriteTokens != 50 {
		t.Fatalf("usage = %+v", usage)
	}
	flat := &FlatRates{Input: 3, Output: 15, CachedInput: 0.3, CacheRead: 0.3, CacheCreation: 3.75}
	estimate := EstimateCost(usage, flat)
	if estimate.Status != StatusEstimated {
		t.Fatalf("estimate = %+v", estimate)
	}
}
