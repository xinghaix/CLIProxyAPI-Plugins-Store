package store

import (
	"context"
	"fmt"
	"math"
	"net/http"
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

func TestAutoBanCooldownHeaderParser(t *testing.T) {
	const now = int64(1_800_000_000_000)
	for _, tc := range []struct {
		name      string
		headers   map[string]string
		now, want int64
		valid     bool
	}{
		{name: "missing"},
		{name: "reset_seconds", headers: map[string]string{"X-Ratelimit-Reset": "1800000120"}, want: now + 120000, valid: true},
		{name: "reset_milliseconds", headers: map[string]string{"X-Ratelimit-Reset": "1800000120000"}, want: now + 120000, valid: true},
		{name: "mixed_case_and_space", headers: map[string]string{" x-RATELIMIT-reset-after ": " 60 "}, want: now + 60000, valid: true},
		{name: "reset_precedence", headers: map[string]string{"X-Ratelimit-Reset": "1800000120", "X-Ratelimit-Reset-After": "60", "Retry-After": "300"}, want: now + 120000, valid: true},
		{name: "reset_after_precedence", headers: map[string]string{"X-Ratelimit-Reset-After": "60", "Retry-After": "300"}, want: now + 60000, valid: true},
		{name: "retry_seconds", headers: map[string]string{"rEtRy-AfTeR": "90"}, want: now + 90000, valid: true},
		{name: "retry_date", headers: map[string]string{"Retry-After": time.UnixMilli(now + 90000).UTC().Format(http.TimeFormat)}, want: now + 90000, valid: true},
		{name: "zero_retry", headers: map[string]string{"Retry-After": "0"}, want: now, valid: true},
		{name: "invalid_reset_uses_relative", headers: map[string]string{"X-Ratelimit-Reset": "junk", "X-Ratelimit-Reset-After": "60"}, want: now + 60000, valid: true},
		{name: "expired_reset_uses_retry", headers: map[string]string{"X-Ratelimit-Reset": "1", "Retry-After": "60"}, want: now + 60000, valid: true},
		{name: "invalid_relative_uses_retry", headers: map[string]string{"X-Ratelimit-Reset-After": "-1", "Retry-After": "60"}, want: now + 60000, valid: true},
		{name: "reset_trailing_junk", headers: map[string]string{"X-Ratelimit-Reset": "1800000120junk"}},
		{name: "relative_trailing_junk", headers: map[string]string{"X-Ratelimit-Reset-After": "60junk"}},
		{name: "retry_trailing_junk", headers: map[string]string{"Retry-After": "60 junk"}},
		{name: "expired_reset", headers: map[string]string{"X-Ratelimit-Reset": "1"}},
		{name: "expired_date", headers: map[string]string{"Retry-After": time.UnixMilli(now - 1000).UTC().Format(http.TimeFormat)}},
		{name: "parse_overflow", headers: map[string]string{"X-Ratelimit-Reset": "9223372036854775808"}},
		{name: "relative_multiply_overflow", headers: map[string]string{"X-Ratelimit-Reset-After": fmt.Sprint(math.MaxInt64)}},
		{name: "retry_multiply_overflow", headers: map[string]string{"Retry-After": fmt.Sprint(math.MaxInt64)}},
		{name: "relative_add_overflow", headers: map[string]string{"Retry-After": "1"}, now: math.MaxInt64 - 999},
		{name: "relative_max_bound", headers: map[string]string{"Retry-After": "1"}, now: math.MaxInt64 - 1000, want: math.MaxInt64, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := tc.now
			if at == 0 {
				at = now
			}
			got, valid := ParseAutoBanCooldownHeader(tc.headers, at)
			if got != tc.want || valid != tc.valid {
				t.Fatalf("got=(%d,%v) want=(%d,%v)", got, valid, tc.want, tc.valid)
			}
			if at != now {
				return
			}
			fallback := int64(7200000)
			want := tc.want
			if !valid {
				want = now + fallback
			}
			if got := resolveCooldownUntilMS(AutoBanRule{CooldownSource: "header_or_default", CooldownMS: &fallback}, BanSignal{Headers: tc.headers}, at); got != want {
				t.Fatalf("resolved=%d want=%d", got, want)
			}
		})
	}
}

func TestAutoBanCooldownHeaderParserDirectStore(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	rule := AutoBanRule{Enabled: true, Priority: 1, Name: "direct-header", ProviderScope: "header-test", AccountKind: "oauth_auth_file", MatchStatusCodes: []int{429}, SourceMask: AutoBanSourceUsage, ThresholdMode: "consecutive", ThresholdCount: 1, Action: AutoBanActionCooldownEnable, CooldownSource: "header_only"}
	for i, tc := range []struct {
		name    string
		headers map[string]string
		source  string
		want    int64
		execute bool
	}{
		{name: "relative_header_only", headers: map[string]string{"x-ratelimit-reset-after": "60"}, want: 61000, execute: true},
		{name: "invalid_header_only", headers: map[string]string{"Retry-After": "60junk"}},
		{name: "invalid_uses_default", headers: map[string]string{"Retry-After": "60junk"}, source: "header_or_default", want: 1000 + 5*60*60*1000, execute: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.source != "" {
				rule.CooldownSource = tc.source
			}
			updated, err := database.UpsertAutoBanRule(ctx, rule)
			if err != nil {
				t.Fatal(err)
			}
			rule = updated
			result, err := database.ApplyAutoBanSignal(ctx, BanSignal{AccountKey: fmt.Sprintf("oauth:header-test:%d", i), Provider: "header-test", AccountKind: "oauth_auth_file", StatusCode: 429, Source: "usage", AtMS: 1000, Headers: tc.headers, Capabilities: AutoBanCapDisable}, false)
			if err != nil {
				t.Fatal(err)
			}
			if result.ShouldExecute != tc.execute {
				t.Fatalf("result=%#v", result)
			}
			if tc.execute && (result.CooldownUntilMS == nil || *result.CooldownUntilMS != tc.want) {
				t.Fatalf("cooldown=%v want=%d", result.CooldownUntilMS, tc.want)
			}
			if !tc.execute && result.Suppressed != "missing_reset" {
				t.Fatalf("result=%#v", result)
			}
		})
	}
}
