package windowkeeper

import (
	"fmt"
	"strings"
)

type Settings struct {
	Enabled               bool     `json:"enabled"`
	Model                 string   `json:"model"`
	Effort                string   `json:"effort"`
	ServiceTier           string   `json:"service_tier"`
	Prompt                string   `json:"prompt"`
	UserAgent             string   `json:"user_agent"`
	WindowMode            string   `json:"window_mode"`
	Kinds                 []string `json:"kinds"`
	IncludeCodeReview     bool     `json:"include_code_review"`
	IncludeAdditional     string   `json:"include_additional"`
	PollSeconds           int      `json:"poll_seconds"`
	SkewSeconds           int      `json:"skew_seconds"`
	MaxAttempts           int      `json:"max_attempts"`
	RetryBaseSeconds      int      `json:"retry_base_seconds"`
	RetryMaxSeconds       int      `json:"retry_max_seconds"`
	RequestTimeoutSeconds int      `json:"request_timeout_seconds"`
	MaxConcurrentSends    int      `json:"max_concurrent_sends"`
	MaxConcurrentProbes   int      `json:"max_concurrent_probes"`
	BaseURL               string   `json:"base_url"`
}

func DefaultSettings() Settings {
	return Settings{
		Model: "gpt-5.4", Effort: "none", ServiceTier: "", Prompt: "Reply with exactly OK.",
		UserAgent: "codex_cli_rs/0.76.0", WindowMode: "auto",
		IncludeAdditional: "fill_gaps", PollSeconds: 20, SkewSeconds: 3,
		MaxAttempts: 5, RetryBaseSeconds: 2, RetryMaxSeconds: 300,
		RequestTimeoutSeconds: 90, MaxConcurrentSends: 1, MaxConcurrentProbes: 2,
		BaseURL: "http://127.0.0.1:8317",
	}
}

func NormalizeSettings(settings Settings) (Settings, error) {
	settings.Model = strings.TrimSpace(settings.Model)
	settings.Effort = strings.TrimSpace(settings.Effort)
	settings.ServiceTier = strings.TrimSpace(settings.ServiceTier)
	settings.Prompt = strings.TrimSpace(settings.Prompt)
	settings.UserAgent = strings.TrimSpace(settings.UserAgent)
	settings.WindowMode = strings.TrimSpace(settings.WindowMode)
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	if settings.Model == "" {
		return settings, fmt.Errorf("model is required")
	}
	if settings.Effort == "" {
		settings.Effort = "none"
	}
	switch settings.Effort {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		return settings, fmt.Errorf("invalid reasoning effort: %s", settings.Effort)
	}
	switch settings.ServiceTier {
	case "", "default", "standard", "flex":
	default:
		return settings, fmt.Errorf("invalid service tier: %s", settings.ServiceTier)
	}
	if settings.Prompt == "" || len([]rune(settings.Prompt)) > 500 {
		return settings, fmt.Errorf("prompt must be 1 to 500 characters")
	}
	def := DefaultSettings()
	if settings.UserAgent == "" {
		settings.UserAgent = def.UserAgent
	}
	if settings.RequestTimeoutSeconds == 0 {
		settings.RequestTimeoutSeconds = def.RequestTimeoutSeconds
	}
	if settings.PollSeconds == 0 {
		settings.PollSeconds = def.PollSeconds
	}
	if settings.MaxAttempts == 0 {
		settings.MaxAttempts = def.MaxAttempts
	}
	if settings.RetryBaseSeconds == 0 {
		settings.RetryBaseSeconds = def.RetryBaseSeconds
	}
	if settings.RetryMaxSeconds == 0 {
		settings.RetryMaxSeconds = def.RetryMaxSeconds
	}
	if settings.IncludeAdditional == "" {
		settings.IncludeAdditional = def.IncludeAdditional
	}
	if settings.WindowMode == "" {
		settings.WindowMode = "auto"
	}
	switch settings.WindowMode {
	case "auto", "blocked_only", "always", "rolling":
	default:
		return settings, fmt.Errorf("invalid window mode: %s", settings.WindowMode)
	}
	if settings.PollSeconds < 5 || settings.PollSeconds > 600 {
		return settings, fmt.Errorf("poll interval must be 5 to 600 seconds")
	}
	if settings.SkewSeconds < 0 || settings.SkewSeconds > 120 {
		return settings, fmt.Errorf("skew must be 0 to 120 seconds")
	}
	if settings.RequestTimeoutSeconds < 15 || settings.RequestTimeoutSeconds > 300 {
		return settings, fmt.Errorf("request timeout must be 15 to 300 seconds")
	}
	if settings.MaxAttempts < 1 || settings.MaxAttempts > 8 {
		return settings, fmt.Errorf("max attempts must be 1 to 8")
	}
	if settings.RetryBaseSeconds < 1 || settings.RetryBaseSeconds > 60 {
		return settings, fmt.Errorf("retry base must be 1 to 60 seconds")
	}
	if settings.RetryMaxSeconds < 10 || settings.RetryMaxSeconds > 3600 {
		return settings, fmt.Errorf("retry max must be 10 to 3600 seconds")
	}
	switch settings.IncludeAdditional {
	case "fill_gaps", "all", "none":
	default:
		return settings, fmt.Errorf("invalid additional limit mode")
	}
	for _, kind := range settings.Kinds {
		switch strings.TrimSpace(kind) {
		case KindFiveHour, KindWeekly, KindMonthly, KindCustom:
		default:
			return settings, fmt.Errorf("unknown window kind %s", kind)
		}
	}
	if settings.MaxConcurrentSends < 1 {
		settings.MaxConcurrentSends = 1
	}
	if settings.MaxConcurrentSends > 4 {
		settings.MaxConcurrentSends = 4
	}
	if settings.MaxConcurrentProbes < 1 {
		settings.MaxConcurrentProbes = 1
	}
	if settings.MaxConcurrentProbes > 4 {
		settings.MaxConcurrentProbes = 4
	}
	if settings.BaseURL == "" {
		settings.BaseURL = "http://127.0.0.1:8317"
	}
	return settings, nil
}
