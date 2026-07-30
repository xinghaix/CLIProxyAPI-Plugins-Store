package store

import (
	"context"
	"testing"
	"time"
)

func TestAutoBanCodexRateLimitTransitionsToCooling(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	now := time.Now().UnixMilli()
	signal := BanSignal{
		AccountKey:   "oauth:codex:codex-1",
		Provider:     "codex",
		AccountKind:  "oauth_auth_file",
		FileName:     "codex.json",
		AuthIndex:    "codex-1",
		StatusCode:   429,
		ErrorKind:    "rate_limited",
		Source:       "usage",
		AtMS:         now,
		Capabilities: AutoBanCapDisable | AutoBanCapEnable | AutoBanCapDelete,
	}
	result, err := database.ApplyAutoBanSignal(ctx, signal, false)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ShouldExecute || result.ExecuteAction != AutoBanActionCooldownEnable {
		t.Fatalf("apply result = %#v", result)
	}
	if result.State.State != AutoBanStatePendingAction || result.State.ConsecutiveHits != 1 {
		t.Fatalf("pending state = %#v", result.State)
	}
	if _, err := database.TransitionAutoBanAction(ctx, signal.AccountKey, AutoBanActionCooldownEnable, true, "", result.CooldownUntilMS, "system", "test"); err != nil {
		t.Fatal(err)
	}
	state, err := database.GetAutoBanAccount(ctx, signal.AccountKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != AutoBanStateCooling || state.CooldownUntilMS == nil || *state.CooldownUntilMS <= now {
		t.Fatalf("cooling state = %#v", state)
	}
}

func TestAutoBanRuleChangeAndSuccessResetCounters(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	rule, err := database.UpsertAutoBanRule(ctx, AutoBanRule{
		Enabled: true, Priority: 1, Name: "test-503-review", ProviderScope: "test", AccountKind: "oauth_auth_file",
		MatchStatusCodes: []int{503}, SourceMask: AutoBanSourceUsage, ThresholdMode: "consecutive", ThresholdCount: 2,
		SuccessResetsConsecutive: true, Action: AutoBanActionReview,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := "oauth:test:test-1"
	first, err := database.ApplyAutoBanSignal(ctx, BanSignal{AccountKey: key, Provider: "test", AccountKind: "oauth_auth_file", StatusCode: 503, Source: "usage", AtMS: 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.State.ActiveRuleID == nil || *first.State.ActiveRuleID != rule.ID || first.State.ConsecutiveHits != 1 {
		t.Fatalf("first state = %#v", first.State)
	}
	if _, err := database.ApplyAutoBanSignal(ctx, BanSignal{AccountKey: key, Provider: "test", AccountKind: "oauth_auth_file", Source: "usage", Success: true, AtMS: 2}, false); err != nil {
		t.Fatal(err)
	}
	state, err := database.GetAutoBanAccount(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if state.ConsecutiveHits != 0 {
		t.Fatalf("success did not reset counter: %#v", state)
	}
}

func TestAutoBanHeaderOnlyCooldownSuppressesWithoutResetHeader(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	rule, err := database.UpsertAutoBanRule(ctx, AutoBanRule{
		Enabled: true, Priority: 1, Name: "header-only", ProviderScope: "header-test", AccountKind: "oauth_auth_file",
		MatchStatusCodes: []int{429}, SourceMask: AutoBanSourceUsage, ThresholdMode: "consecutive", ThresholdCount: 1,
		SuccessResetsConsecutive: true, Action: AutoBanActionCooldownEnable, CooldownSource: "header_only",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := database.ApplyAutoBanSignal(ctx, BanSignal{AccountKey: "oauth:header-test:1", Provider: "header-test", AccountKind: "oauth_auth_file", StatusCode: 429, Source: "usage", AtMS: 100, Capabilities: AutoBanCapDisable}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.MatchedRule == nil || result.MatchedRule.ID != rule.ID || result.ShouldExecute || result.Suppressed != "missing_reset" || result.State.State != AutoBanStateFlagged {
		t.Fatalf("header-only result = %#v", result)
	}
}

func TestAutoBanDeleteRuleRequiresDailyCap(t *testing.T) {
	database, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	_, err = database.UpsertAutoBanRule(context.Background(), AutoBanRule{
		Enabled: true, Priority: 1, Name: "unsafe-delete", ProviderScope: "codex", AccountKind: "oauth_auth_file",
		MatchStatusCodes: []int{401}, SourceMask: AutoBanSourceUsage, ThresholdMode: "consecutive", ThresholdCount: 1,
		Action: AutoBanActionDelete,
	})
	if err == nil {
		t.Fatal("delete rule without a daily cap was accepted")
	}
}
