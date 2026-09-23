package app

import (
	"context"
	"testing"

	"github.com/xinghaix/CLIProxyAPI-Plugins-Store/plugins/cpa-manager-plus/go/internal/windowkeeper"
)

func TestWindowKeeperRuntimeIntegration(t *testing.T) {
	dir := t.TempDir()
	r, err := New([]byte("data_dir: " + dir))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	ctx := context.Background()
	settings := r.WindowKeeperSettings()
	if settings.Enabled != false || settings.Model != "gpt-5.4" {
		t.Fatalf("unexpected default settings: %+v", settings)
	}

	settings.Enabled = true
	settings.Model = "gpt-5.4-mini"
	if err := r.SaveWindowKeeperSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	saved := r.WindowKeeperSettings()
	if !saved.Enabled || saved.Model != "gpt-5.4-mini" {
		t.Fatalf("settings not saved: %+v", saved)
	}

	// Touch account
	if err := r.Store().TouchWindowKeeperAccount(ctx, windowkeeper.Account{
		AuthID: "auth-1", AuthIndex: "idx-1", Email: "test@example.com",
	}); err != nil {
		t.Fatal(err)
	}

	// Override
	if err := r.SetWindowKeeperOverride(ctx, "auth-1", `{"enabled":"off","model":"gpt-5.4"}`); err != nil {
		t.Fatal(err)
	}
	accounts, err := r.WindowKeeperAccounts(ctx)
	if err != nil || len(accounts) != 1 || accounts[0].OverrideJSON != `{"enabled":"off","model":"gpt-5.4"}` {
		t.Fatalf("accounts mismatch: %+v, %v", accounts, err)
	}

	// Pause and resume
	if err := r.Store().SetWindowKeeperPause(ctx, "auth-1", "reauth"); err != nil {
		t.Fatal(err)
	}
	if err := r.ResumeWindowKeeperAccount(ctx, "auth-1"); err != nil {
		t.Fatal(err)
	}
	accounts, err = r.WindowKeeperAccounts(ctx)
	if err != nil || accounts[0].PauseReason != "" {
		t.Fatalf("account not resumed: %+v", accounts)
	}
}
