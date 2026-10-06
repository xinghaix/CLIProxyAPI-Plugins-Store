package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/pricesync"
	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/store"
)

func auditAccountRuntime(t *testing.T) *Runtime {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := &Runtime{store: db, connection: connection{BaseURL: "http://audit.invalid", ManagementKey: "fake"}, legacyAccountOpsMasters: LegacyAccountOpsMasters{AutoBan: true, Inspection: true}, autoBanSettings: DefaultAutoBanSettings(), inspectionSettings: DefaultCodexInspectionSettings()}
	r.autoBanSettings.Enabled = true
	r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{{Name: "codex.json", AuthIndex: "idx", Provider: "codex", Account: "account"}}, nil
	}
	return r
}

func TestAccountOpsDryRunPreservesExpiredCooldown(t *testing.T) {
	r := auditAccountRuntime(t)
	ctx := context.Background()
	sig := store.BanSignal{AccountKey: "oauth:codex:idx", Provider: "codex", AccountKind: "oauth_auth_file", FileName: "codex.json", AuthIndex: "idx", StatusCode: 429, ErrorKind: "rate_limited", Source: "usage", Capabilities: store.AutoBanCapDisable | store.AutoBanCapEnable}
	if _, err := r.store.ApplyAutoBanSignal(ctx, sig, false); err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Second).UnixMilli()
	if _, err := r.store.TransitionAutoBanAction(ctx, sig.AccountKey, store.AutoBanActionCooldownEnable, true, "", &expired, "system", "audit"); err != nil {
		t.Fatal(err)
	}
	r.autoBanSettings.DryRun = true
	calls := 0
	r.httpDo = func(_ context.Context, method, target string, _ http.Header, body []byte) (pricesync.HTTPResponse, error) {
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if method != http.MethodPatch || payload["disabled"] != false {
			t.Fatalf("unexpected action %s %s %s", method, target, body)
		}
		calls++
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	r.applyAutoBanSignal(ctx, store.BanSignal{AccountKey: sig.AccountKey, Provider: "codex", AuthIndex: "idx", Source: "usage", Success: true})
	state, err := r.store.GetAutoBanAccount(ctx, sig.AccountKey)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 || state.State != store.AutoBanStateCooling {
		t.Fatalf("dry-run changed account: calls=%d state=%s", calls, state.State)
	}
}

func TestAccountOpsResetAfterHeader(t *testing.T) {
	r := auditAccountRuntime(t)
	r.autoBanSettings.DefaultCodexCooldownHours = 12
	sig := store.BanSignal{AccountKey: "oauth:codex:idx", Provider: "codex", AuthIndex: "idx", StatusCode: 429, ErrorKind: "rate_limited", Source: "usage", AtMS: time.Now().UnixMilli(), Headers: parseAutoBanHeaders(`{"x-ratelimit-reset-after":["60"]}`)}
	r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	r.applyAutoBanSignal(context.Background(), sig)
	state, err := r.store.GetAutoBanAccount(context.Background(), sig.AccountKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.CooldownUntilMS == nil {
		t.Fatalf("state=%#v", state)
	}
	duration := time.Duration(*state.CooldownUntilMS-sig.AtMS) * time.Millisecond
	if duration != time.Minute {
		t.Fatalf("reset-after=60: cooldown=%s, want 1m", duration)
	}
}

func TestAccountOpsHealthyOwnedCredentialRecovers(t *testing.T) {
	r := auditAccountRuntime(t)
	ctx := context.Background()
	r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	if err := r.executeInspectionAction(ctx, store.InspectionResult{Action: "disable", FileName: "codex.json", Provider: "codex", AuthIndex: "idx", AccountID: "account"}, true); err != nil {
		t.Fatal(err)
	}
	if _, owned, err := r.store.DisableOwnership(ctx, "codex.json"); err != nil || !owned {
		t.Fatalf("ownership=%v err=%v", owned, err)
	}
	r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
		return []pluginapi.HostAuthFileEntry{{Name: "codex.json", AuthIndex: "idx", Provider: "codex", Account: "account", Disabled: true}}, nil
	}
	r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{"status_code":200,"body":{"plan_type":"plus","rate_limit":{"allowed":true,"primary_window":{"used_percent":0,"limit_window_seconds":18000}}}}`)}, nil
	}
	result := r.probeInspectionAccount(ctx, DefaultCodexInspectionSettings(), store.InspectionAccount{Key: "idx", Provider: "codex", FileName: "codex.json", AuthIndex: "idx", AccountID: "account", Disabled: true})
	if result.Action != "enable" || result.ErrorKind != "healthy" || !result.AutoRecoverEligible {
		t.Fatalf("healthy owned credential not eligible: %#v", result)
	}
	run, err := r.store.StartInspectionRun(ctx, "audit", "", "{}")
	if err != nil {
		t.Fatal(err)
	}
	result.RunID = run.ID
	if _, err := r.store.InsertInspectionResult(ctx, result); err != nil {
		t.Fatal(err)
	}
	calls := 0
	r.httpDo = func(_ context.Context, method, target string, _ http.Header, body []byte) (pricesync.HTTPResponse, error) {
		calls++
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	settings := DefaultCodexInspectionSettings()
	settings.AutoRecoverEnabled = true
	if err := r.executeAutomaticInspectionActions(ctx, run.ID, settings, func(string, string, any) {}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
	if _, owned, err := r.store.DisableOwnership(ctx, "codex.json"); err != nil || owned {
		t.Fatalf("ownership not cleared after recovery: owned=%v err=%v", owned, err)
	}
}

func TestAccountOpsCooldownHeaderAndFallbackPrecedence(t *testing.T) {
	at := time.Now().Truncate(time.Second).UnixMilli()
	absolute := fmt.Sprintf("%d", (at+120000)/1000)
	absoluteMS := fmt.Sprintf("%d", at+120000)
	for _, tc := range []struct {
		name, headers, source string
		ruleMS                *int64
		want                  time.Duration
		execute               bool
	}{
		{name: "no_header_uses_setting", want: 12 * time.Hour, execute: true},
		{name: "relative_string_headers", headers: `{"X-RATELIMIT-RESET-AFTER":"60"}`, want: time.Minute, execute: true},
		{name: "absolute_seconds_wins", headers: fmt.Sprintf(`{"x-Ratelimit-Reset":["%s"],"x-ratelimit-reset-after":["60"]}`, absolute), want: 2 * time.Minute, execute: true},
		{name: "absolute_milliseconds", headers: fmt.Sprintf(`{"X-Ratelimit-Reset":["%s"]}`, absoluteMS), want: 2 * time.Minute, execute: true},
		{name: "relative_wins_retry", headers: `{"x-ratelimit-reset-after":["60"],"Retry-After":["300"]}`, want: time.Minute, execute: true},
		{name: "retry_after", headers: `{"Retry-After":["90"]}`, want: 90 * time.Second, execute: true},
		{name: "retry_http_date", headers: fmt.Sprintf(`{"Retry-After":["%s"]}`, time.UnixMilli(at+180000).UTC().Format(http.TimeFormat)), want: 3 * time.Minute, execute: true},
		{name: "invalid_absolute_uses_relative", headers: `{"x-ratelimit-reset":["garbage"],"x-ratelimit-reset-after":["60"]}`, want: time.Minute, execute: true},
		{name: "invalid_relative_uses_retry", headers: `{"x-ratelimit-reset-after":["-60"],"Retry-After":["90"]}`, want: 90 * time.Second, execute: true},
		{name: "invalid_header_uses_setting", headers: `{"x-ratelimit-reset-after":["garbage"]}`, want: 12 * time.Hour, execute: true},
		{name: "partial_integer_rejected", headers: `{"x-ratelimit-reset":["123junk"],"Retry-After":["60junk"]}`, want: 12 * time.Hour, execute: true},
		{name: "overflow_rejected", headers: `{"x-ratelimit-reset-after":["9223372036854775807"]}`, want: 12 * time.Hour, execute: true},
		{name: "expired_absolute_rejected", headers: `{"x-ratelimit-reset":["1"]}`, want: 12 * time.Hour, execute: true},
		{name: "rule_fallback", ruleMS: int64AccountOps(7200000), want: 2 * time.Hour, execute: true},
		{name: "header_beats_rule_fallback", headers: `{"x-ratelimit-reset-after":["60"]}`, ruleMS: int64AccountOps(7200000), want: time.Minute, execute: true},
		{name: "fixed_rule_beats_header", source: "fixed", headers: `{"x-ratelimit-reset-after":["60"]}`, ruleMS: int64AccountOps(7200000), want: 2 * time.Hour, execute: true},
		{name: "header_only_accepts_relative", source: "header_only", headers: `{"x-ratelimit-reset-after":["60"]}`, want: time.Minute, execute: true},
		{name: "header_only_rejects_invalid", source: "header_only", headers: `{"x-ratelimit-reset-after":["garbage"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := auditAccountRuntime(t)
			r.autoBanSettings.DefaultCodexCooldownHours = 12
			rules, err := r.store.ListAutoBanRules(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, rule := range rules {
				if rule.Name == "codex-429-cooldown" {
					rule.CooldownMS = tc.ruleMS
					if tc.source != "" {
						rule.CooldownSource = tc.source
					}
					if _, err := r.store.UpsertAutoBanRule(context.Background(), rule); err != nil {
						t.Fatal(err)
					}
				}
			}
			calls := 0
			r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
				calls++
				return pricesync.HTTPResponse{StatusCode: 200}, nil
			}
			sig := store.BanSignal{AccountKey: "oauth:codex:idx", Provider: "codex", AuthIndex: "idx", StatusCode: 429, ErrorKind: "rate_limited", Source: "usage", AtMS: at, Headers: parseAutoBanHeaders(tc.headers)}
			r.applyAutoBanSignal(context.Background(), sig)
			state, err := r.store.GetAutoBanAccount(context.Background(), sig.AccountKey)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.execute {
				if calls != 0 || state.CooldownUntilMS != nil {
					t.Fatalf("invalid header executed: calls=%d state=%#v", calls, state)
				}
				return
			}
			if calls != 1 || state.CooldownUntilMS == nil {
				t.Fatalf("calls=%d state=%#v", calls, state)
			}
			if got := time.Duration(*state.CooldownUntilMS-at) * time.Millisecond; got != tc.want {
				t.Fatalf("cooldown=%s want=%s", got, tc.want)
			}
		})
	}
}

func int64AccountOps(n int64) *int64 { return &n }

func TestAccountOpsAutomaticRecoveryHoldsAndDryRun(t *testing.T) {
	r := auditAccountRuntime(t)
	ctx := context.Background()
	calls := 0
	r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		calls++
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	for _, tc := range []struct {
		name, action, kind string
		dryRun             bool
	}{
		{name: "manual_hold", action: "hold"},
		{name: "cooling", action: store.AutoBanActionCooldownEnable},
		{name: "disabled", action: store.AutoBanActionDisable},
		{name: "dry_run", dryRun: true},
		{name: "unhealthy", kind: "rate_limited"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r.autoBanSettings.DryRun = tc.dryRun
			result := store.InspectionResult{Action: "enable", FileName: tc.name + ".json", Provider: "codex", AuthIndex: tc.name, AccountID: "account", Disabled: true, AutoRecoverEligible: true, ErrorKind: "healthy"}
			r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
				return []pluginapi.HostAuthFileEntry{{Name: result.FileName, AuthIndex: result.AuthIndex, Provider: result.Provider, Account: result.AccountID, Disabled: true}}, nil
			}
			if tc.kind != "" {
				result.ErrorKind = tc.kind
			}
			if err := r.store.PutDisableOwnership(ctx, store.InspectionDisableOwnership{FileName: result.FileName, Provider: result.Provider, AuthIndex: result.AuthIndex, AccountID: result.AccountID}); err != nil {
				t.Fatal(err)
			}
			key := store.AutoBanAccountKey(result.Provider, "oauth_auth_file", result.AuthIndex, result.FileName, result.AccountID, "")
			if _, err := r.store.ApplyAutoBanSignal(ctx, store.BanSignal{AccountKey: key, Provider: result.Provider, AuthIndex: result.AuthIndex, Success: true}, false); err != nil {
				t.Fatal(err)
			}
			if tc.action != "" {
				until := time.Now().Add(time.Hour).UnixMilli()
				if _, err := r.store.TransitionAutoBanAction(ctx, key, tc.action, true, "", &until, "system", "test"); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.executeInspectionAction(ctx, result, true); err == nil || calls != 0 {
				t.Fatalf("unsafe recovery: calls=%d err=%v", calls, err)
			}
			if _, owned, err := r.store.DisableOwnership(ctx, result.FileName); err != nil || !owned {
				t.Fatalf("suppression lost ownership: owned=%v err=%v", owned, err)
			}
		})
	}
}

func TestAccountOpsDryRunSharedAutomaticAndManualActions(t *testing.T) {
	r := auditAccountRuntime(t)
	ctx := context.Background()
	sig := store.BanSignal{AccountKey: "oauth:codex:idx", Provider: "codex", FileName: "codex.json", AuthIndex: "idx", Source: "usage", StatusCode: 429, ErrorKind: "rate_limited", Capabilities: store.AutoBanCapDisable | store.AutoBanCapEnable}
	applied, err := r.store.ApplyAutoBanSignal(ctx, sig, false)
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-time.Second).UnixMilli()
	state, err := r.store.TransitionAutoBanAction(ctx, sig.AccountKey, store.AutoBanActionCooldownEnable, true, "", &expired, "system", "test")
	if err != nil {
		t.Fatal(err)
	}
	r.autoBanSettings.DryRun = true
	calls := 0
	r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		calls++
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	for _, source := range []string{"usage", "inspection", "scheduler", "lazy"} {
		for _, action := range []string{store.AutoBanActionDisable, store.AutoBanActionDelete, store.AutoBanActionCooldownEnable, "cooldown_expire"} {
			if err := r.executeAutoBanStateAction(ctx, state, action, applied.CooldownUntilMS, "system", source); err != nil {
				t.Fatal(err)
			}
		}
	}
	if calls != 0 {
		t.Fatalf("dry-run automatic requests=%d", calls)
	}
	state, err = r.store.GetAutoBanAccount(ctx, sig.AccountKey)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != store.AutoBanStateCooling || state.LastAction != store.AutoBanActionCooldownEnable {
		t.Fatalf("dry-run recorded action: %#v", state)
	}
	for _, action := range []string{"disable", "enable", "unban", "delete"} {
		if _, err := r.ExecuteAutoBanAccountAction(ctx, state.ID, action, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 4 {
		t.Fatalf("manual requests=%d want=4", calls)
	}
}

func TestAccountOpsRecoveryOwnershipAndExecutionGuards(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mutate   func(*Runtime, *store.InspectionResult)
		eligible bool
	}{
		{name: "owned", eligible: true},
		{name: "manual_disabled", mutate: func(r *Runtime, _ *store.InspectionResult) {
			_ = r.store.DeleteDisableOwnership(context.Background(), "codex.json")
		}},
		{name: "provider_replaced", mutate: func(r *Runtime, _ *store.InspectionResult) {
			_ = r.store.PutDisableOwnership(context.Background(), store.InspectionDisableOwnership{FileName: "codex.json", Provider: "xai", AuthIndex: "idx", AccountID: "account"})
		}},
		{name: "auth_index_replaced", mutate: func(r *Runtime, _ *store.InspectionResult) {
			_ = r.store.PutDisableOwnership(context.Background(), store.InspectionDisableOwnership{FileName: "codex.json", Provider: "codex", AuthIndex: "other", AccountID: "account"})
		}},
		{name: "account_replaced", mutate: func(r *Runtime, _ *store.InspectionResult) {
			_ = r.store.PutDisableOwnership(context.Background(), store.InspectionDisableOwnership{FileName: "codex.json", Provider: "codex", AuthIndex: "idx", AccountID: "other"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := auditAccountRuntime(t)
			ctx := context.Background()
			if err := r.store.PutDisableOwnership(ctx, store.InspectionDisableOwnership{FileName: "codex.json", Provider: "codex", AuthIndex: "idx", AccountID: "account"}); err != nil {
				t.Fatal(err)
			}
			if tc.mutate != nil {
				tc.mutate(r, nil)
			}
			r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
				return pricesync.HTTPResponse{StatusCode: 200, Body: []byte(`{"status_code":200,"body":{"plan_type":"plus","rate_limit":{"allowed":true,"primary_window":{"used_percent":0,"limit_window_seconds":18000}}}}`)}, nil
			}
			result := r.probeInspectionAccount(ctx, DefaultCodexInspectionSettings(), store.InspectionAccount{Key: "idx", Provider: "codex", FileName: "codex.json", AuthIndex: "idx", AccountID: "account", Disabled: true})
			if result.AutoRecoverEligible != tc.eligible || (tc.eligible && result.Action != "enable") || (!tc.eligible && result.Action != "keep") {
				t.Fatalf("result=%#v", result)
			}
			r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
				return []pluginapi.HostAuthFileEntry{{Name: "codex.json", AuthIndex: "idx", Provider: "codex", Account: "account", Disabled: true}}, nil
			}
			// A saved historical enable candidate must not bypass current ownership.
			result.Action = "enable"
			result.ErrorKind = "healthy"
			result.AutoRecoverEligible = true
			calls := 0
			r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
				calls++
				return pricesync.HTTPResponse{StatusCode: 200}, nil
			}
			err := r.executeInspectionAction(ctx, result, true)
			if tc.eligible {
				if err != nil || calls != 1 {
					t.Fatalf("owned recovery: calls=%d err=%v", calls, err)
				}
			} else {
				if err == nil || calls != 0 {
					t.Fatalf("unsafe historical recovery: calls=%d err=%v", calls, err)
				}
			}
		})
	}
}

func TestAccountOpsManualDisableRevokesHistoricalRecovery(t *testing.T) {
	for _, path := range []string{"inspection", "auto_ban"} {
		t.Run(path, func(t *testing.T) {
			r := auditAccountRuntime(t)
			ctx := context.Background()
			calls := 0
			r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
				calls++
				return pricesync.HTTPResponse{StatusCode: 200}, nil
			}
			result := store.InspectionResult{Action: "disable", FileName: "codex.json", Provider: "codex", AuthIndex: "idx", AccountID: "account"}
			if err := r.executeInspectionAction(ctx, result, true); err != nil {
				t.Fatal(err)
			}
			if path == "inspection" {
				if err := r.executeInspectionAction(ctx, result, false); err != nil {
					t.Fatal(err)
				}
			} else {
				sig := store.BanSignal{AccountKey: "oauth:codex:idx", Provider: "codex", FileName: "codex.json", AuthIndex: "idx", Success: true}
				applied, err := r.store.ApplyAutoBanSignal(ctx, sig, false)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := r.ExecuteAutoBanAccountAction(ctx, applied.State.ID, "disable", ""); err != nil {
					t.Fatal(err)
				}
			}
			if _, owned, err := r.store.DisableOwnership(ctx, "codex.json"); err != nil || owned {
				t.Fatalf("manual disable retained ownership: owned=%v err=%v", owned, err)
			}
			r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
				return []pluginapi.HostAuthFileEntry{{Name: "codex.json", AuthIndex: "idx", Provider: "codex", Account: "account", Disabled: true}}, nil
			}
			result.Action = "enable"
			result.Disabled = true
			result.ErrorKind = "healthy"
			result.AutoRecoverEligible = true
			if err := r.executeInspectionAction(ctx, result, true); err == nil || calls != 2 {
				t.Fatalf("historical candidate enabled manual disable: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestAccountOpsAutomaticCurrentCredential(t *testing.T) {
	for _, action := range []string{"enable", "disable", "delete"} {
		for _, tc := range []struct {
			name, filename                            string
			mutate                                    func(*pluginapi.HostAuthFileEntry)
			missing, duplicate, listError, noCallback bool
			allow                                     bool
		}{
			{name: "matching", allow: true},
			{name: "missing_filename", filename: "unknown", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Name, a.ID = "", "" }},
			{name: "blank_filename", filename: "unknown", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Name, a.ID = " 	", "\n " }},
			{name: "literal_unknown", filename: "unknown", allow: true},
			{name: "unknown_id_fallback", filename: "unknown", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Name, a.ID = "", "unknown" }, allow: true},
			{name: "provider_replaced", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Provider = "xai" }},
			{name: "index_replaced", mutate: func(a *pluginapi.HostAuthFileEntry) { a.AuthIndex = "other" }},
			{name: "account_replaced", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Account = "other" }},
			{name: "filename_replaced", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Name = "other.json" }},
			{name: "missing_provider", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Provider = "" }},
			{name: "missing_index", mutate: func(a *pluginapi.HostAuthFileEntry) { a.AuthIndex = "" }},
			{name: "missing_account", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Account = "" }},
			{name: "missing_entry", missing: true},
			{name: "ambiguous_entry", duplicate: true},
			{name: "list_error", listError: true},
			{name: "missing_callback", noCallback: true},
			{name: "stale_disabled", mutate: func(a *pluginapi.HostAuthFileEntry) { a.Disabled = !a.Disabled }},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				r := auditAccountRuntime(t)
				ctx := context.Background()
				result := store.InspectionResult{Action: action, FileName: "codex.json", Provider: "codex", AuthIndex: "idx", AccountID: "account", Disabled: action == "enable", ErrorKind: "healthy", AutoRecoverEligible: true}
				if tc.filename != "" {
					result.FileName = tc.filename
				}
				ownership := store.InspectionDisableOwnership{FileName: result.FileName, Provider: result.Provider, AuthIndex: result.AuthIndex, AccountID: result.AccountID}
				if err := r.store.PutDisableOwnership(ctx, ownership); err != nil {
					t.Fatal(err)
				}
				entry := pluginapi.HostAuthFileEntry{Name: result.FileName, Provider: result.Provider, AuthIndex: result.AuthIndex, Account: result.AccountID, Disabled: result.Disabled}
				if tc.mutate != nil {
					tc.mutate(&entry)
				}
				reads, calls := 0, 0
				r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
					reads++
					if tc.listError {
						return nil, fmt.Errorf("host unavailable")
					}
					if tc.missing {
						return nil, nil
					}
					if tc.duplicate {
						return []pluginapi.HostAuthFileEntry{entry, entry}, nil
					}
					return []pluginapi.HostAuthFileEntry{entry}, nil
				}
				if tc.noCallback {
					r.authList = nil
				}
				r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
					calls++
					return pricesync.HTTPResponse{StatusCode: 200}, nil
				}
				err := r.executeInspectionAction(ctx, result, true)
				allowed := tc.allow || (action == "delete" && tc.name == "stale_disabled")
				if allowed {
					if err != nil || calls != 1 || reads != 1 {
						t.Fatalf("matching current identity: calls=%d reads=%d err=%v", calls, reads, err)
					}
				} else {
					if err == nil || calls != 0 {
						t.Fatalf("stale history executed: calls=%d err=%v", calls, err)
					}
					got, owned, err := r.store.DisableOwnership(ctx, result.FileName)
					if err != nil || !owned || got.Provider != ownership.Provider || got.AuthIndex != ownership.AuthIndex || got.AccountID != ownership.AccountID {
						t.Fatalf("ownership changed: %#v owned=%v err=%v", got, owned, err)
					}
					// Explicit manual actions are intentionally not gated on host snapshots.
					if err := r.executeInspectionAction(ctx, result, false); err != nil || calls != 1 {
						t.Fatalf("manual action blocked: calls=%d err=%v", calls, err)
					}
				}
			})
		}
	}
}

func TestAccountOpsAutomaticCurrentSnapshotAndMetadata(t *testing.T) {
	for _, authType := range []string{"", "oauth", "api_key"} {
		t.Run(authType, func(t *testing.T) {
			r := auditAccountRuntime(t)
			ctx := context.Background()
			// Missing account IDs are valid legacy metadata; API-key values are never account identities.
			entry := pluginapi.HostAuthFileEntry{ID: "codex.json", AuthIndex: "idx", Type: " CODEX ", AccountType: authType}
			if authType == "api_key" {
				entry.Account = "secret-not-an-identity"
			}
			reads, calls := 0, 0
			r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
				reads++
				return []pluginapi.HostAuthFileEntry{entry}, nil
			}
			r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
				calls++
				return pricesync.HTTPResponse{StatusCode: 200}, nil
			}
			result := store.InspectionResult{Action: "disable", FileName: "codex.json", Provider: "codex", AuthIndex: "idx"}
			if err := r.executeInspectionAction(ctx, result, true); err != nil {
				t.Fatal(err)
			}
			// History and ownership stay unchanged; only the current host credential changes.
			entry.Disabled = true
			if err := r.executeInspectionAction(ctx, result, true); err == nil || calls != 1 || reads != 2 {
				t.Fatalf("snapshot reused/adopted: calls=%d reads=%d err=%v", calls, reads, err)
			}
		})
	}
}

func TestAccountOpsManualOwnershipDespiteLedgerFailure(t *testing.T) {
	for _, tc := range []struct {
		action       string
		cleanupFails bool
	}{
		{action: "disable"}, {action: "enable"}, {action: "unban"}, {action: "delete"},
		{action: "disable", cleanupFails: true},
	} {
		t.Run(fmt.Sprintf("%s/cleanupFails=%v", tc.action, tc.cleanupFails), func(t *testing.T) {
			r := auditAccountRuntime(t)
			ctx := context.Background()
			dir := t.TempDir()
			db, err := store.Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			r.store = db
			faults, err := sql.Open("sqlite", filepath.Join(dir, "usage.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer faults.Close()
			result := store.InspectionResult{Action: "enable", FileName: "codex.json", Provider: "codex", AuthIndex: "idx", AccountID: "account", Disabled: true, ErrorKind: "healthy", AutoRecoverEligible: true}
			if err := db.PutDisableOwnership(ctx, store.InspectionDisableOwnership{FileName: result.FileName, Provider: result.Provider, AuthIndex: result.AuthIndex, AccountID: result.AccountID}); err != nil {
				t.Fatal(err)
			}
			key := store.AutoBanAccountKey(result.Provider, "oauth_auth_file", result.AuthIndex, result.FileName, result.AccountID, "")
			applied, err := db.ApplyAutoBanSignal(ctx, store.BanSignal{AccountKey: key, Provider: result.Provider, AccountKind: "oauth_auth_file", FileName: result.FileName, AuthIndex: result.AuthIndex, Success: true}, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := faults.Exec(`CREATE TRIGGER fail_ledger BEFORE UPDATE ON auto_ban_account_state BEGIN SELECT RAISE(ABORT, 'ledgerfail'); END`); err != nil {
				t.Fatal(err)
			}
			if tc.cleanupFails {
				if _, err := faults.Exec(`CREATE TRIGGER fail_cleanup BEFORE DELETE ON inspection_disable_ownership BEGIN SELECT RAISE(ABORT, 'cleanupfail'); END`); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
				calls++
				return pricesync.HTTPResponse{StatusCode: http.StatusOK}, nil
			}
			_, err = r.ExecuteAutoBanAccountAction(ctx, applied.State.ID, tc.action, "")
			if calls != 1 || err == nil || !strings.Contains(err.Error(), "ledgerfail") || (tc.cleanupFails && !strings.Contains(err.Error(), "cleanupfail")) {
				t.Fatalf("manual CPA success must retain bookkeeping errors: calls=%d err=%v", calls, err)
			}
			if _, owned, err := db.DisableOwnership(ctx, result.FileName); err != nil || owned != tc.cleanupFails {
				t.Fatalf("manual CPA success retained ownership: owned=%v err=%v", owned, err)
			}
			state, err := db.GetAutoBanAccount(ctx, key)
			if err != nil || state.State != store.AutoBanStateIdle || state.ManualHold {
				t.Fatalf("ledger failure must leave idle state: state=%#v err=%v", state, err)
			}
			if tc.cleanupFails {
				return
			}
			r.authList = func() ([]pluginapi.HostAuthFileEntry, error) {
				return []pluginapi.HostAuthFileEntry{{Name: result.FileName, Provider: result.Provider, AuthIndex: result.AuthIndex, Account: result.AccountID, Disabled: true}}, nil
			}
			if err := r.executeInspectionAction(ctx, result, true); err == nil || calls != 1 {
				t.Fatalf("historical healthy enable bypassed revoked ownership: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestAccountOpsAutomaticDisableCannotAdoptManualCredential(t *testing.T) {
	r := auditAccountRuntime(t)
	calls := 0
	r.httpDo = func(context.Context, string, string, http.Header, []byte) (pricesync.HTTPResponse, error) {
		calls++
		return pricesync.HTTPResponse{StatusCode: 200}, nil
	}
	err := r.executeInspectionAction(context.Background(), store.InspectionResult{Action: "disable", FileName: "codex.json", Provider: "codex", AuthIndex: "idx", Disabled: true}, true)
	if err == nil || calls != 0 {
		t.Fatalf("adopted manual disabled credential: calls=%d err=%v", calls, err)
	}
	if _, owned, err := r.store.DisableOwnership(context.Background(), "codex.json"); err != nil || owned {
		t.Fatalf("ownership=%v err=%v", owned, err)
	}
}
