package main

import (
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestShouldNormalizeDefaultsOnlyDeepSeekTargets(t *testing.T) {
	activeConfig.Store(defaultPluginConfig())

	matched := pluginapi.RequestTransformRequest{
		ToFormat: "openai",
		Model:    "provider/deepseek-chat",
		Body:     []byte(`{"messages":[{"role":"developer","content":"rules"}]}`),
	}
	if !shouldNormalize(matched) {
		t.Fatalf("expected default config to normalize deepseek openai target")
	}

	unsupportedFormat := matched
	unsupportedFormat.ToFormat = "anthropic"
	if shouldNormalize(unsupportedFormat) {
		t.Fatalf("expected non-openai-compatible target format to be skipped")
	}

	unmatchedModel := matched
	unmatchedModel.Model = "gpt-4.1"
	if shouldNormalize(unmatchedModel) {
		t.Fatalf("expected non-deepseek model to be skipped by scheme B defaults")
	}
}

func TestShouldNormalizeUsesBodyModelFallback(t *testing.T) {
	activeConfig.Store(defaultPluginConfig())

	req := pluginapi.RequestTransformRequest{
		ToFormat: "codex",
		Body:     []byte(`{"model":"deepseek-reasoner","messages":[{"role":"developer","content":"rules"}]}`),
	}
	if !shouldNormalize(req) {
		t.Fatalf("expected body model fallback to match deepseek")
	}
}

func TestMatchesModelIncludeExclude(t *testing.T) {
	rule := modelMatchRule{
		Mode:    "contains",
		Include: []string{"deepseek", "qwen"},
		Exclude: []string{"deepseek-official-compatible"},
	}

	if !matchesModel(rule, "QWEN-plus") {
		t.Fatalf("expected case-insensitive include match")
	}
	if matchesModel(rule, "deepseek-official-compatible-v1") {
		t.Fatalf("expected exclude to override include")
	}
	if matchesModel(rule, "gpt-4.1") {
		t.Fatalf("expected unmatched model to be skipped")
	}
}

func TestNormalizeDeveloperRole(t *testing.T) {
	input := []byte(`{"messages":[{"role":"developer","content":"rules"},{"role":"user","content":"hi"}]}`)
	got := string(normalizeDeveloperRole(input))
	want := `{"messages":[{"role":"system","content":"rules"},{"role":"user","content":"hi"}]}`
	if got != want {
		t.Fatalf("unexpected normalized payload\nwant: %s\n got: %s", want, got)
	}
}
