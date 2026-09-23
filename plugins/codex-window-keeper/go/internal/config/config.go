package config

import (
	"fmt"
	"strings"
)

type Settings struct {
	Enabled               bool     `json:"enabled"`
	Model                 string   `json:"model"`
	Effort                string   `json:"effort"`
	Prompt                string   `json:"prompt"`
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

type File struct {
	DataDir               string   `yaml:"data_dir"`
	Enabled               *bool    `yaml:"enabled"`
	Model                 string   `yaml:"model"`
	ReasoningEffort       string   `yaml:"reasoning_effort"`
	Prompt                string   `yaml:"prompt"`
	WindowKinds           []string `yaml:"window_kinds"`
	IncludeCodeReview     *bool    `yaml:"include_code_review"`
	IncludeAdditional     string   `yaml:"include_additional"`
	PollIntervalSeconds   int      `yaml:"poll_interval_seconds"`
	ActivationSkewSeconds *int     `yaml:"activation_skew_seconds"`
	MaxAttempts           int      `yaml:"max_attempts"`
	RetryBaseSeconds      int      `yaml:"retry_base_seconds"`
	RetryMaxSeconds       int      `yaml:"retry_max_seconds"`
	RequestTimeoutSeconds int      `yaml:"request_timeout_seconds"`
	MaxConcurrentSends    int      `yaml:"max_concurrent_sends"`
	MaxConcurrentProbes   int      `yaml:"max_concurrent_probes"`
}

func Default() Settings {
	return Settings{
		Model: "gpt-5.4", Effort: "low", Prompt: "Reply with exactly OK.",
		IncludeAdditional: "fill_gaps", PollSeconds: 20, SkewSeconds: 3,
		MaxAttempts: 5, RetryBaseSeconds: 2, RetryMaxSeconds: 300,
		RequestTimeoutSeconds: 90, MaxConcurrentSends: 1, MaxConcurrentProbes: 2,
		BaseURL: "http://127.0.0.1:8317",
	}
}

func FromFile(file File) Settings {
	settings := Default()
	if file.Enabled != nil {
		settings.Enabled = *file.Enabled
	}
	if strings.TrimSpace(file.Model) != "" {
		settings.Model = strings.TrimSpace(file.Model)
	}
	if strings.TrimSpace(file.ReasoningEffort) != "" {
		settings.Effort = strings.TrimSpace(file.ReasoningEffort)
	}
	if strings.TrimSpace(file.Prompt) != "" {
		settings.Prompt = file.Prompt
	}
	if file.WindowKinds != nil {
		settings.Kinds = file.WindowKinds
	}
	if file.IncludeCodeReview != nil {
		settings.IncludeCodeReview = *file.IncludeCodeReview
	}
	if strings.TrimSpace(file.IncludeAdditional) != "" {
		settings.IncludeAdditional = file.IncludeAdditional
	}
	if file.PollIntervalSeconds > 0 {
		settings.PollSeconds = file.PollIntervalSeconds
	}
	if file.ActivationSkewSeconds != nil {
		settings.SkewSeconds = *file.ActivationSkewSeconds
	}
	applyPositive(&settings.MaxAttempts, file.MaxAttempts)
	applyPositive(&settings.RetryBaseSeconds, file.RetryBaseSeconds)
	applyPositive(&settings.RetryMaxSeconds, file.RetryMaxSeconds)
	applyPositive(&settings.RequestTimeoutSeconds, file.RequestTimeoutSeconds)
	applyPositive(&settings.MaxConcurrentSends, file.MaxConcurrentSends)
	applyPositive(&settings.MaxConcurrentProbes, file.MaxConcurrentProbes)
	return settings
}

func Normalize(settings Settings) (Settings, error) {
	settings.Model = strings.TrimSpace(settings.Model)
	settings.Effort = strings.TrimSpace(settings.Effort)
	settings.Prompt = strings.TrimSpace(settings.Prompt)
	settings.BaseURL = strings.TrimRight(strings.TrimSpace(settings.BaseURL), "/")
	if settings.Model == "" {
		return settings, fmt.Errorf("model is required")
	}
	switch settings.Effort {
	case "minimal", "low", "medium", "high", "xhigh":
	default:
		return settings, fmt.Errorf("invalid reasoning effort")
	}
	if settings.Prompt == "" || len([]rune(settings.Prompt)) > 500 {
		return settings, fmt.Errorf("prompt must be 1 to 500 characters")
	}
	if settings.PollSeconds < 5 || settings.PollSeconds > 600 {
		return settings, fmt.Errorf("poll interval must be 5 to 600 seconds")
	}
	if settings.SkewSeconds < 0 || settings.SkewSeconds > 120 {
		return settings, fmt.Errorf("skew must be 0 to 120 seconds")
	}
	if settings.MaxAttempts < 1 || settings.MaxAttempts > 8 {
		return settings, fmt.Errorf("max attempts must be 1 to 8")
	}
	switch settings.IncludeAdditional {
	case "fill_gaps", "all", "none":
	default:
		return settings, fmt.Errorf("invalid additional limit mode")
	}
	for _, kind := range settings.Kinds {
		switch strings.TrimSpace(kind) {
		case "five_hour", "weekly", "monthly", "custom":
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

func applyPositive(target *int, value int) {
	if value > 0 {
		*target = value
	}
}
