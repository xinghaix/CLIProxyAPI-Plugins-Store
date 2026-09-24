package store

import (
	"context"
	"testing"
	"time"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

func TestWindowKeeperStoreOperations(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Settings
	settings := windowkeeper.DefaultSettings()
	settings.Enabled = true
	settings.Model = "gpt-5.4"
	if err := s.SaveWindowKeeperSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	loadedSettings, ok, err := s.LoadWindowKeeperSettings(ctx)
	if err != nil || !ok || !loadedSettings.Enabled || loadedSettings.Model != "gpt-5.4" {
		t.Fatalf("loaded settings mismatch: %+v, ok=%v, err=%v", loadedSettings, ok, err)
	}

	// Accounts
	now := time.Now().UTC().Truncate(time.Millisecond)
	acc := windowkeeper.Account{
		AuthID: "auth-1", AuthIndex: "idx-1", Email: "test@example.com", Name: "Test Account",
	}
	if err := s.TouchWindowKeeperAccount(ctx, acc); err != nil {
		t.Fatal(err)
	}
	accounts, err := s.ListWindowKeeperAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].AuthID != "auth-1" {
		t.Fatalf("accounts mismatch: %+v, err=%v", accounts, err)
	}

	if err := s.SetWindowKeeperPlan(ctx, "auth-1", "plus", "plus_team", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWindowKeeperOverride(ctx, "auth-1", `{"enabled":"on","model":"gpt-5.4-mini"}`); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWindowKeeperPause(ctx, "auth-1", "reauth"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetWindowKeeperNotBefore(ctx, "auth-1", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	accounts, err = s.ListWindowKeeperAccounts(ctx)
	if err != nil || accounts[0].PlanType != "plus" || accounts[0].PauseReason != "reauth" {
		t.Fatalf("account update mismatch: %+v", accounts[0])
	}

	// Windows
	states := []windowkeeper.State{
		{
			LimitID: "codex", Slot: "primary", Kind: windowkeeper.KindFiveHour, PeriodSeconds: 18000,
			StartsAt: now, EndsAt: now.Add(5 * time.Hour), UsedPercent: 50, Gating: true, Phase: windowkeeper.PhaseBlocked,
		},
	}
	if err := s.SaveWindowKeeperWindows(ctx, "auth-1", states); err != nil {
		t.Fatal(err)
	}
	loadedWindows, err := s.LoadWindowKeeperWindows(ctx, "auth-1")
	if err != nil || len(loadedWindows) != 1 || loadedWindows[0].Phase != windowkeeper.PhaseBlocked {
		t.Fatalf("windows mismatch: %+v, err=%v", loadedWindows, err)
	}

	// Claims
	claimed, err := s.ClaimWindowKeeper(ctx, "auth-1", "worker-1", now, now.Add(time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim failed: %v, %v", claimed, err)
	}
	secondClaim, err := s.ClaimWindowKeeper(ctx, "auth-1", "worker-2", now, now.Add(time.Minute))
	if err != nil || secondClaim {
		t.Fatalf("second claim should fail: %v, %v", secondClaim, err)
	}
	if err := s.ReleaseWindowKeeper(ctx, "auth-1", "worker-1"); err != nil {
		t.Fatal(err)
	}

	// Attempts
	attID, err := s.StartWindowKeeperAttempt(ctx, windowkeeper.Attempt{
		AccountID: "auth-1", GenerationKey: "gen-1", StartedAt: now, AttemptNo: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	count, err := s.WindowKeeperAttemptCount(ctx, "auth-1", "gen-1")
	if err != nil || count != 1 {
		t.Fatalf("attempt count = %d, want 1", count)
	}
	if err := s.FinishWindowKeeperAttempt(ctx, attID, windowkeeper.AttemptFinish{
		Status: "succeeded", HTTPStatus: 200, Excerpt: "OK", ResponseID: "resp-1",
		ReqHeaders: `{"Content-Type":["application/json"]}`, ReqBody: `{"model":"gpt-5.4"}`,
		RespHeaders: `{"Content-Type":["application/json"]}`, RespBody: `{"ok":true}`,
	}); err != nil {
		t.Fatal(err)
	}
	hasSuccess, err := s.HasWindowKeeperSuccess(ctx, "auth-1", "gen-1")
	if err != nil || !hasSuccess {
		t.Fatalf("has success = %v, want true", hasSuccess)
	}
	attempts, err := s.ListWindowKeeperAttempts(ctx, 10)
	if err != nil || len(attempts) != 1 || attempts[0].Status != "succeeded" {
		t.Fatalf("attempts mismatch: %+v", attempts)
	}
	if attempts[0].ReqBody == "" || attempts[0].RespBody == "" {
		t.Fatalf("expected persisted req/resp bodies: %+v", attempts[0])
	}

	failID, err := s.StartWindowKeeperAttempt(ctx, windowkeeper.Attempt{
		AccountID: "auth-1", GenerationKey: "manual:1", StartedAt: now, AttemptNo: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishWindowKeeperAttempt(ctx, failID, windowkeeper.AttemptFinish{
		Status: "failed", HTTPStatus: 400, Kind: "config", Excerpt: "bad",
		ReqBody: `{"model":"x"}`, RespBody: `{"detail":"bad"}`,
	}); err != nil {
		t.Fatal(err)
	}
	attempts, err = s.ListWindowKeeperAttempts(ctx, 10)
	if err != nil || len(attempts) != 2 {
		t.Fatalf("attempts after fail: %+v", attempts)
	}
	var failed *windowkeeper.Attempt
	for i := range attempts {
		if attempts[i].Status == "failed" {
			failed = &attempts[i]
			break
		}
	}
	if failed == nil || failed.RespBody != `{"detail":"bad"}` {
		t.Fatalf("failed attempt detail missing: %+v", failed)
	}
}
