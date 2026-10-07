package app

import (
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestQuotaMetadataDeclaresScope(t *testing.T) {
	for _, tc := range []struct {
		provider string
		windows  []map[string]any
		want     []string
	}{
		{"claude", buildClaudeInspectionWindows(map[string]any{
			"five_hour":        map[string]any{"utilization": 10},
			"seven_day":        map[string]any{"utilization": 20},
			"seven_day_opus":   map[string]any{"utilization": 30},
			"seven_day_sonnet": map[string]any{"utilization": 40},
			"iguana_necktie":   map[string]any{"utilization": 50},
		}), []string{"account", "account", "unknown", "unknown", "unknown"}},
		{"codex", buildCodexInspectionWindows(map[string]any{
			"rate_limit":             map[string]any{"primary_window": map[string]any{"used_percent": 10, "limit_window_seconds": 18000}},
			"code_review_rate_limit": map[string]any{"primary_window": map[string]any{"used_percent": 20, "limit_window_seconds": 18000}},
			"additional_rate_limits": []any{map[string]any{"limit_name": "Special", "rate_limit": map[string]any{"primary_window": map[string]any{"used_percent": 30, "limit_window_seconds": 18000}}}},
		}), []string{"account", "unknown", "unknown"}},
		{"antigravity", []map[string]any{{"id": "claude-bucket", "kind": "antigravity_bucket"}}, []string{"unknown"}},
		{"xai", []map[string]any{{"id": "xai-weekly"}, {"id": "xai-product-0"}}, []string{"account", "unknown"}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			result := quotaMetadata(store.InspectionResult{Provider: tc.provider}, map[string]any{"provider": tc.provider}, tc.windows)
			windows := asWindowSlice(result.QuotaWindows)
			if len(windows) != len(tc.want) {
				t.Fatalf("windows=%#v", windows)
			}
			for i, w := range windows {
				if w["modelScope"] != tc.want[i] {
					t.Errorf("scope=%#v want %s", w, tc.want[i])
				}
				if _, ok := w["modelScopeModels"]; ok {
					t.Errorf("invented exact model mapping: %#v", w)
				}
			}
		})
	}
}
