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

func TestBuildWindowKeeperRequestBody(t *testing.T) {
	base := windowkeeper.Settings{
		Model:  "gpt-5.4",
		Prompt: "Reply with exactly OK.",
	}

	t.Run("omits reasoning when effort none", func(t *testing.T) {
		body := buildWindowKeeperRequestBody(base)
		if _, ok := body["reasoning"]; ok {
			t.Fatalf("expected reasoning omitted for none, got %#v", body["reasoning"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Fatalf("must not send top-level reasoning_effort, got %#v", body["reasoning_effort"])
		}
		if _, ok := body["service_tier"]; ok {
			t.Fatalf("expected service_tier omitted when empty")
		}
		if body["model"] != "gpt-5.4" || body["store"] != false {
			t.Fatalf("unexpected base fields: %#v", body)
		}
	})

	t.Run("omits reasoning when effort empty", func(t *testing.T) {
		s := base
		s.Effort = "  "
		body := buildWindowKeeperRequestBody(s)
		if _, ok := body["reasoning"]; ok {
			t.Fatalf("expected reasoning omitted for empty effort")
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Fatalf("must not send top-level reasoning_effort")
		}
	})

	t.Run("nested reasoning.effort only when set", func(t *testing.T) {
		s := base
		s.Effort = "medium"
		s.ServiceTier = "flex"
		body := buildWindowKeeperRequestBody(s)
		reasoning, ok := body["reasoning"].(map[string]any)
		if !ok {
			t.Fatalf("expected nested reasoning object, got %#v", body["reasoning"])
		}
		if reasoning["effort"] != "medium" {
			t.Fatalf("effort=%v", reasoning["effort"])
		}
		if _, ok := body["reasoning_effort"]; ok {
			t.Fatalf("must not send top-level reasoning_effort")
		}
		if body["service_tier"] != "flex" {
			t.Fatalf("service_tier=%v", body["service_tier"])
		}
	})
}

func TestBuildExcerptFromOpen(t *testing.T) {
	t.Run("detail from body JSON", func(t *testing.T) {
		got := buildExcerptFromOpen([]byte(`{"detail":"Unsupported parameter: reasoning_effort"}`), "")
		if got != "Unsupported parameter: reasoning_effort" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("falls back to err text detail JSON", func(t *testing.T) {
		got := buildExcerptFromOpen(nil, `{"detail":"Unsupported parameter: reasoning_effort"}`)
		if got != "Unsupported parameter: reasoning_effort" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("raw snippet when not JSON", func(t *testing.T) {
		got := buildExcerptFromOpen(nil, "  plain failure  ")
		if got != "plain failure" {
			t.Fatalf("got %q", got)
		}
	})
}
