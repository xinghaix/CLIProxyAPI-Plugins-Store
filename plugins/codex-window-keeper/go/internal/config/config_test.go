package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigDefaultsAndNormalize(t *testing.T) {
	def := Default()
	if def.Model != "gpt-5.4" || def.Effort != "low" || def.PollSeconds != 20 || def.SkewSeconds != 3 {
		t.Fatalf("unexpected defaults: %+v", def)
	}
	norm, err := Normalize(def)
	if err != nil {
		t.Fatal(err)
	}
	if norm.UserAgent != "codex_cli_rs/0.76.0" || norm.WindowMode != "auto" {
		t.Fatalf("unexpected normalized values: %+v", norm)
	}

	// Bad service_tier
	badTier := def
	badTier.ServiceTier = "super-fast"
	if _, err := Normalize(badTier); err == nil {
		t.Fatal("expected error on invalid service tier")
	}

	// Bad window_mode
	badMode := def
	badMode.WindowMode = "manual"
	if _, err := Normalize(badMode); err == nil {
		t.Fatal("expected error on invalid window mode")
	}

	// Valid YAML parsing
	yamlData := []byte(`
poll_interval_seconds: 45
activation_skew_seconds: 5
request_timeout_seconds: 120
model: gpt-5.4-mini
reasoning_effort: medium
service_tier: flex
prompt: Ping
window_kinds: ["five_hour", "weekly"]
`)
	var file File
	if err := yaml.Unmarshal(yamlData, &file); err != nil {
		t.Fatal(err)
	}
	settings := FromFile(file)
	if settings.PollSeconds != 45 || settings.SkewSeconds != 5 || settings.RequestTimeoutSeconds != 120 {
		t.Fatalf("unexpected settings from file: %+v", settings)
	}
	if settings.Model != "gpt-5.4-mini" || settings.Effort != "medium" || settings.ServiceTier != "flex" {
		t.Fatalf("unexpected model/effort/tier: %+v", settings)
	}
	if len(settings.Kinds) != 2 || settings.Kinds[0] != "five_hour" {
		t.Fatalf("unexpected kinds: %+v", settings.Kinds)
	}
	if _, err := Normalize(settings); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
}
