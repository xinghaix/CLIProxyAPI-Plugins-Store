package windowkeeper

import "testing"

func TestSettingsDefaultsAndNormalize(t *testing.T) {
	def := DefaultSettings()
	if def.Model != "gpt-5.4" || def.Effort != "none" || def.PollSeconds != 20 || def.SkewSeconds != 3 {
		t.Fatalf("unexpected defaults: %+v", def)
	}
	norm, err := NormalizeSettings(def)
	if err != nil {
		t.Fatal(err)
	}
	if norm.UserAgent != "codex_cli_rs/0.76.0" || norm.WindowMode != "auto" {
		t.Fatalf("unexpected normalized: %+v", norm)
	}
	badTier := def
	badTier.ServiceTier = "super-fast"
	if _, err := NormalizeSettings(badTier); err == nil {
		t.Fatal("expected error on invalid service tier")
	}
	badMode := def
	badMode.WindowMode = "manual"
	if _, err := NormalizeSettings(badMode); err == nil {
		t.Fatal("expected error on invalid window mode")
	}
}
