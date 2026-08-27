package app

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestNormalizeInspectionTargetTypesIncludesCredentialProviders(t *testing.T) {
	got := normalizeInspectionTargetTypes([]string{"kimi", "claude", "unknown"}, "")
	if len(got) != 2 || got[0] != "claude" || got[1] != "kimi" {
		t.Fatalf("got %#v", got)
	}
	all := normalizeInspectionTargetTypes([]string{"all"}, "")
	if len(all) != len(inspectionProviders) {
		t.Fatalf("all = %#v", all)
	}
}

func TestApplyLoadCodeAssistCredits(t *testing.T) {
	result := applyLoadCodeAssistCredits(store.InspectionResult{}, map[string]any{
		"paidTier": map[string]any{
			"id": "tier-1",
			"availableCredits": []any{
				map[string]any{"creditType": "GOOGLE_ONE_AI", "creditAmount": "25000"},
			},
		},
	})
	if result.PlanType != "tier-1" {
		t.Fatalf("plan = %q", result.PlanType)
	}
	windows := asWindowSlice(result.QuotaWindows)
	if len(windows) != 1 || windows[0]["id"] != "GOOGLE_ONE_AI" {
		t.Fatalf("windows = %#v", result.QuotaWindows)
	}
}

func TestFindInspectionAccountDoesNotCrossProviders(t *testing.T) {
	auths := []pluginapi.HostAuthFileEntry{
		{Provider: "codex", Email: "user@gmail.com", Name: "codex-user@gmail.com-plus.json", AuthIndex: "codex-idx", ID: "codex-id"},
		{Provider: "xai", Email: "user@gmail.com", Name: "xai-user@gmail.com.json", AuthIndex: "xai-idx", ID: "xai-id"},
	}
	account, ok := findInspectionAccount(auths, "", "xai", "user@gmail.com")
	if !ok || account.Provider != "xai" || account.FileName != "xai-user@gmail.com.json" {
		t.Fatalf("xai account = %#v ok=%v", account, ok)
	}
	account, ok = findInspectionAccount(auths, "codex-idx", "xai", "user@gmail.com")
	if !ok || account.Provider != "xai" {
		t.Fatalf("xai by rejected codex index = %#v ok=%v", account, ok)
	}
}

func TestApplyXAIBillingWindows(t *testing.T) {
	result := applyXAIBilling(store.InspectionResult{}, map[string]any{
		"rateLimit": map[string]any{
			"remainingRequests":   16,
			"totalRequests":       100,
			"windowEnd":           "2026-09-01T16:20:00Z",
			"remainingGrokBuilds": 16,
			"totalGrokBuilds":     100,
		},
		"credits": map[string]any{
			"payAsYouGoEnabled":  false,
			"remainingCredits":   0,
			"monthlyCredits":     0,
			"nextMonthlyRefresh": "2026-09-01T08:00:00Z",
		},
	}, nil)
	windows := asWindowSlice(result.QuotaWindows)
	if len(windows) < 2 {
		t.Fatalf("windows = %#v", result.QuotaWindows)
	}
	if windows[0]["label"] != "周限额" || windows[0]["usedPercent"] != float64(84) {
		t.Fatalf("weekly = %#v", windows[0])
	}
	if windows[1]["label"] != "GrokBuild 使用" {
		t.Fatalf("grokbuild = %#v", windows[1])
	}
}

func TestFindInspectionAccountBySource(t *testing.T) {
	auths := []pluginapi.HostAuthFileEntry{
		{Provider: "kimi", Email: "user@kimi.local", Name: "kimi-user.json", AuthIndex: "idx-1", ID: "id-1"},
		{Provider: "claude", Email: "dev@claude.local", Name: "claude.json", AuthIndex: "idx-2", ID: "id-2"},
	}
	account, ok := findInspectionAccount(auths, "", "kimi", "user@kimi.local")
	if !ok || account.Provider != "kimi" || account.AuthIndex != "idx-1" {
		t.Fatalf("account = %#v ok=%v", account, ok)
	}
}
