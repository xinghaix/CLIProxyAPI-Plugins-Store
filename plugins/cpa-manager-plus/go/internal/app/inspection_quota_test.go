package app

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func TestBuildCodexInspectionWindowsClassifiesOfficialPeriods(t *testing.T) {
	windows := buildCodexInspectionWindows(map[string]any{
		"rate_limit": map[string]any{
			"primary_window": map[string]any{
				"used_percent":         25,
				"limit_window_seconds": 18000,
				"reset_at":             1788000000,
			},
			"secondary_window": map[string]any{
				"used_percent":         40,
				"limit_window_seconds": 604800,
			},
		},
		"code_review_rate_limit": map[string]any{
			"primary_window": map[string]any{
				"used_percent":         10,
				"limit_window_seconds": 30 * 24 * 60 * 60,
			},
		},
	})
	if len(windows) != 3 {
		t.Fatalf("windows = %#v, want 3 windows", windows)
	}
	if windows[0]["id"] != "five-hour" || windows[0]["kind"] != "five_hour" || windows[0]["remainingPercent"] != float64(75) {
		t.Fatalf("five-hour window = %#v", windows[0])
	}
	if windows[1]["id"] != "weekly" || windows[1]["periodHours"] != float64(168) {
		t.Fatalf("weekly window = %#v", windows[1])
	}
	if windows[2]["id"] != "code-review-monthly" || windows[2]["kind"] != "code_review_monthly" {
		t.Fatalf("code review monthly window = %#v", windows[2])
	}
}

func TestParseCodexResetCreditsKeepsOnlyAvailableRateLimitCredits(t *testing.T) {
	available, applicable, credits, valid := parseCodexResetCredits(map[string]any{
		"available_count":            2,
		"applicable_available_count": 1,
		"credits": []any{
			map[string]any{"id": "one", "reset_type": "codex_rate_limits", "status": "available", "expires_at": "2026-09-01T00:00:00Z"},
			map[string]any{"id": "used", "reset_type": "codex_rate_limits", "status": "consumed", "expires_at": "2026-09-02T00:00:00Z"},
			map[string]any{"id": "other", "reset_type": "other", "status": "available", "expires_at": "2026-09-03T00:00:00Z"},
		},
	})
	if !valid || available == nil || *available != 2 || applicable == nil || *applicable != 1 || len(credits) != 1 {
		t.Fatalf("reset credits = %v %v %#v valid=%v", available, applicable, credits, valid)
	}
	if credits[0]["id"] != "one" || credits[0]["expiresAt"] != "2026-09-01T00:00:00Z" {
		t.Fatalf("credit = %#v", credits[0])
	}
}

func TestBuildClaudeInspectionWindowsUsesModernFableLimit(t *testing.T) {
	windows := buildClaudeInspectionWindows(map[string]any{
		"five_hour":      map[string]any{"utilization": 12, "resets_at": "2026-09-01T00:00:00Z"},
		"iguana_necktie": map[string]any{"utilization": 90, "resets_at": "2026-09-02T00:00:00Z"},
		"limits": []any{
			map[string]any{
				"kind":      "weekly_scoped",
				"percent":   30,
				"is_active": true,
				"resets_at": "2026-09-03T00:00:00Z",
				"scope":     map[string]any{"model": map[string]any{"display_name": "Fable 5"}},
			},
		},
	})
	if len(windows) != 2 {
		t.Fatalf("windows = %#v, want five-hour and modern Fable", windows)
	}
	if windows[0]["id"] != "claude-five-hour" || windows[0]["periodHours"] != float64(5) {
		t.Fatalf("five-hour = %#v", windows[0])
	}
	if windows[1]["id"] != "claude-seven-day-fable" || windows[1]["usedPercent"] != float64(30) {
		t.Fatalf("Fable = %#v", windows[1])
	}
}

func TestBuildClaudeInspectionWindowsTreatsDollarIguanaAsCloudSessionCredits(t *testing.T) {
	windows := buildClaudeInspectionWindows(map[string]any{
		"five_hour": map[string]any{"utilization": 10},
		"iguana_necktie": map[string]any{
			"utilization":       40,
			"limit_dollars":     50,
			"used_dollars":      20,
			"remaining_dollars": 30,
		},
		"limits": []any{
			map[string]any{
				"kind":      "weekly_scoped",
				"percent":   15,
				"is_active": true,
				"scope":     map[string]any{"model": map[string]any{"display_name": "Fable 5"}},
			},
		},
	})
	if len(windows) != 3 {
		t.Fatalf("windows = %#v, want five-hour, cloud-session-credits, and fable", windows)
	}
	if windows[1]["id"] != "cloud-session-credits" || windows[1]["label"] != "云端会话额度" || windows[1]["remainingDollars"] != float64(30) {
		t.Fatalf("cloud session credits = %#v", windows[1])
	}
	if _, hasPeriod := windows[1]["periodHours"]; hasPeriod {
		t.Fatalf("cloud session credits should not have fixed periodHours: %#v", windows[1])
	}
}

func TestResolveClaudePlanSupportsMaxMultipliersAndTeamPriority(t *testing.T) {
	if got := resolveClaudePlan(map[string]any{
		"account":      map[string]any{"has_claude_max": true},
		"organization": map[string]any{"organization_type": "claude_team", "subscription_status": "active"},
	}); got != "plan_team" {
		t.Fatalf("team priority = %q, want plan_team", got)
	}
	if got := resolveClaudePlan(map[string]any{
		"account":      map[string]any{"has_claude_max": true},
		"organization": map[string]any{"rate_limit_tier": "default_claude_max_20x"},
	}); got != "plan_max20" {
		t.Fatalf("max20 = %q, want plan_max20", got)
	}
	if got := resolveClaudePlan(map[string]any{
		"account":      map[string]any{"has_claude_max": true},
		"organization": map[string]any{"rate_limit_tier": "default_claude_max_5x"},
	}); got != "plan_max5" {
		t.Fatalf("max5 = %q, want plan_max5", got)
	}
}

func TestBuildKimiInspectionWindowsPreservesProviderWindowDetails(t *testing.T) {
	windows := buildKimiInspectionWindows(map[string]any{
		"limits": []any{
			map[string]any{
				"name":   "Coding 5h",
				"window": map[string]any{"duration": 300, "timeUnit": "TIME_UNIT_MINUTE"},
				"detail": map[string]any{
					"used":      25,
					"limit":     100,
					"remaining": 75,
					"reset_in":  3600,
				},
			},
		},
		"usage": map[string]any{"used": 10, "limit": 50, "remaining": 40},
	})
	if len(windows) != 2 {
		t.Fatalf("windows = %#v, want limit and summary", windows)
	}
	if windows[0]["label"] != "Coding 5h" || windows[0]["used"] != float64(25) || windows[0]["limit"] != float64(100) || windows[0]["periodHours"] != float64(5) {
		t.Fatalf("Kimi limit = %#v", windows[0])
	}
	if windows[1]["id"] != "kimi-summary" || windows[1]["usedPercent"] != float64(20) {
		t.Fatalf("Kimi summary = %#v", windows[1])
	}
}

func TestBuildAntigravityQuotaKeepsGroupsAndRemainingDirection(t *testing.T) {
	subscription := buildAntigravitySubscription(map[string]any{
		"paidTier": map[string]any{"id": "g1-ultra-tier", "name": "Ultra"},
	})
	if subscription["plan"] != "ultra" || subscription["tierId"] != "g1-ultra-tier" {
		t.Fatalf("subscription = %#v", subscription)
	}
	if subPro := buildAntigravitySubscription(map[string]any{
		"currentTier": map[string]any{"id": "standard-tier", "name": "Standard"},
	}); subPro["plan"] != "pro" {
		t.Fatalf("standard-tier subscription = %#v, want pro", subPro)
	}
	models := parseAntigravityGroupModels("Gemini Models", "Models within this group: Gemini 2.5 Pro, Gemini 3 Flash")
	if len(models) != 3 || models[2] != "Gemini Argon" {
		t.Fatalf("Gemini Models parsed = %#v, want Gemini Argon appended", models)
	}
	groups, windows := buildAntigravityQuota(map[string]any{
		"groups": []any{
			map[string]any{
				"displayName": "Gemini models",
				"description": "Models within this group: Gemini 2.5 Pro, Gemini 3 Flash",
				"buckets": []any{
					map[string]any{"bucketId": "five", "displayName": "5 hour limit", "window": "5h", "remainingFraction": 0.8, "resetTime": "2026-09-01T00:00:00Z"},
				},
			},
			map[string]any{
				"displayName": "Claude and GPT Models",
				"description": "Models within this group: Claude Sonnet 4.5, GPT-OSS 120B",
				"buckets": []any{
					map[string]any{"bucketId": "five", "displayName": "Five Hour Limit Remaining", "window": "5h", "remainingFraction": 1.0, "resetTime": "2026-09-01T00:00:00Z"},
				},
			},
		},
	})
	if len(groups) != 2 || len(groups[0]["buckets"].([]map[string]any)) != 1 || len(windows) != 2 {
		t.Fatalf("groups/windows = %#v %#v", groups, windows)
	}
	used, _ := windows[0]["usedPercent"].(float64)
	if windows[0]["remainingPercent"] != float64(80) || used < 19.99 || used > 20.01 || windows[0]["periodHours"] != float64(5) {
		t.Fatalf("Antigravity bucket = %#v", windows[0])
	}
	if windows[0]["id"] == windows[1]["id"] || windows[0]["groupLabel"] != "Gemini 模型" || windows[1]["groupLabel"] != "Claude 和 GPT 模型" {
		t.Fatalf("Antigravity two buckets not distinguished: %#v vs %#v", windows[0], windows[1])
	}
	if windows[0]["label"] != "Gemini 模型 · 5 小时限额" || windows[1]["label"] != "Claude 和 GPT 模型 · 5 小时限额" {
		t.Fatalf("Antigravity combined labels = %q, %q", windows[0]["label"], windows[1]["label"])
	}
}

func TestInspectionAuthMetadataDoesNotExposeAPIKeyAsAccount(t *testing.T) {
	metadata := inspectionAuthMetadataFromEntry(pluginapi.HostAuthFileEntry{ID: "api-id", AccountType: "api_key", Account: "secret-api-key"}, "apikey")
	if metadata.Account != "" {
		t.Fatalf("API key account leaked into metadata = %#v", metadata)
	}
}

func TestFilterInspectionAccountsDoesNotPersistAPIKeyAsAccountID(t *testing.T) {
	accounts := filterInspectionAccounts([]pluginapi.HostAuthFileEntry{{
		ID: "api-id", AuthIndex: "api-1", Name: "xai-api.json", Provider: "xai",
		AccountType: "api_key", Account: "secret-api-key", Status: "available",
	}}, CodexInspectionSettings{TargetTypes: []string{"xai"}})
	if len(accounts) != 1 {
		t.Fatalf("accounts = %#v", accounts)
	}
	if accounts[0].AccountID != "" || accounts[0].Metadata.Account != "" || accounts[0].DisplayName == "secret-api-key" {
		t.Fatalf("API key leaked into inspection account = %#v", accounts[0])
	}
}

func TestMergeInspectionAuthMetadataDoesNotExposeCredentialValues(t *testing.T) {
	metadata := mergeInspectionAuthMetadata(store.InspectionAuthMetadata{}, map[string]any{
		"email":        "user@example.com",
		"project_id":   "project-1",
		"auth_kind":    "oauth",
		"priority":     7,
		"weight":       3,
		"access_token": "secret-token",
	})
	if metadata.Email != "user@example.com" || metadata.ProjectID != "project-1" || metadata.AuthType != "oauth" || metadata.Priority == nil || *metadata.Priority != 7 || metadata.Weight == nil || *metadata.Weight != 3 {
		t.Fatalf("metadata = %#v", metadata)
	}
	if metadata.Account == "secret-token" || metadata.Note == "secret-token" {
		t.Fatalf("credential leaked into display metadata = %#v", metadata)
	}
}
