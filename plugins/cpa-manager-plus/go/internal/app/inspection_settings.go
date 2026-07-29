package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const inspectionSettingsKey = "codex_inspection_settings_v1"

// CodexInspectionSchedule is the nested schedule block returned to the UI.
type CodexInspectionSchedule struct {
	Mode            string   `json:"mode"`
	IntervalMinutes int      `json:"intervalMinutes"`
	TimePoints      []string `json:"timePoints"`
	TimeZone        string   `json:"timeZone"`
}

// CodexInspectionSettings is the full nested codexInspection payload used by the UI.
// Only enabled/schedule/autoActionMode currently drive the local scheduler; the rest
// is persisted for UI round-trip and future engine work.
type CodexInspectionSettings struct {
	Enabled  bool                    `json:"enabled"`
	Schedule CodexInspectionSchedule `json:"schedule"`
	// TargetTypes is the canonical provider selection. TargetType remains for
	// compatibility with settings saved by earlier plugin releases.
	TargetTypes           []string `json:"targetTypes"`
	TargetType            string   `json:"targetType"`
	Workers               int      `json:"workers"`
	DeleteWorkers         int      `json:"deleteWorkers"`
	Timeout               int      `json:"timeout"`
	Retries               int      `json:"retries"`
	UserAgent             string   `json:"userAgent"`
	XAIInferenceUserAgent string   `json:"xaiInferenceUserAgent"`
	XAIInferenceEnabled   bool     `json:"xaiInferenceEnabled"`
	XAIInferenceModel     string   `json:"xaiInferenceModel"`
	XAIInferencePrompt    string   `json:"xaiInferencePrompt"`
	UsedPercentThreshold  float64  `json:"usedPercentThreshold"`
	SampleSize            int      `json:"sampleSize"`
	AutoActionMode        string   `json:"autoActionMode"`
	AutoRecoverEnabled    bool     `json:"autoRecoverEnabled"`
}

// DefaultCodexInspectionSettings returns a complete, valid inspection configuration.
func DefaultCodexInspectionSettings() CodexInspectionSettings {
	return CodexInspectionSettings{
		Enabled: false,
		Schedule: CodexInspectionSchedule{
			Mode:            "interval",
			IntervalMinutes: 60,
			TimePoints:      []string{},
			TimeZone:        "",
		},
		TargetTypes:           []string{"codex"},
		TargetType:            "codex",
		Workers:               4,
		DeleteWorkers:         4,
		Timeout:               15000,
		Retries:               0,
		UserAgent:             "codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal",
		XAIInferenceUserAgent: "xai-grok-workspace/0.2.101",
		XAIInferenceEnabled:   false,
		XAIInferenceModel:     "grok-4.5",
		XAIInferencePrompt:    "Reply with exactly OK.",
		UsedPercentThreshold:  100,
		SampleSize:            0,
		AutoActionMode:        "none",
		AutoRecoverEnabled:    false,
	}
}

func normalizeCodexInspectionSettings(in CodexInspectionSettings) (CodexInspectionSettings, error) {
	out := DefaultCodexInspectionSettings()
	out.Enabled = in.Enabled

	mode := strings.TrimSpace(in.Schedule.Mode)
	if mode == "" {
		if len(in.Schedule.TimePoints) > 0 {
			mode = "time_points"
		} else {
			mode = "interval"
		}
	}
	switch mode {
	case "interval", "time_points":
		out.Schedule.Mode = mode
	default:
		return CodexInspectionSettings{}, fmt.Errorf("unsupported codexInspection.schedule.mode")
	}

	interval := in.Schedule.IntervalMinutes
	if interval < 1 || interval > 24*60 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.schedule.intervalMinutes must be between 1 and 1440")
	}
	out.Schedule.IntervalMinutes = interval

	points := make([]string, 0, len(in.Schedule.TimePoints))
	seen := map[string]struct{}{}
	for _, raw := range in.Schedule.TimePoints {
		point, ok := normalizeTimePoint(raw)
		if !ok {
			return CodexInspectionSettings{}, fmt.Errorf("invalid codexInspection.schedule.timePoints entry: %q", raw)
		}
		if _, exists := seen[point]; exists {
			continue
		}
		seen[point] = struct{}{}
		points = append(points, point)
	}
	if out.Schedule.Mode == "time_points" && len(points) == 0 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.schedule.timePoints must not be empty when mode is time_points")
	}
	out.Schedule.TimePoints = points
	out.Schedule.TimeZone = strings.TrimSpace(in.Schedule.TimeZone)

	out.TargetTypes = normalizeInspectionTargetTypes(in.TargetTypes, in.TargetType)
	if len(out.TargetTypes) == 0 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.targetTypes must include codex or xai")
	}
	out.TargetType = out.TargetTypes[0]
	if in.Workers < 1 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.workers must be >= 1")
	}
	out.Workers = in.Workers
	if in.DeleteWorkers < 1 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.deleteWorkers must be >= 1")
	}
	out.DeleteWorkers = in.DeleteWorkers
	if in.Timeout < 1 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.timeout must be >= 1")
	}
	out.Timeout = in.Timeout
	if in.Retries < 0 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.retries must be >= 0")
	}
	out.Retries = in.Retries
	out.UserAgent = strings.TrimSpace(in.UserAgent)
	if out.UserAgent == "" {
		out.UserAgent = DefaultCodexInspectionSettings().UserAgent
	}
	out.XAIInferenceUserAgent = strings.TrimSpace(in.XAIInferenceUserAgent)
	if out.XAIInferenceUserAgent == "" {
		out.XAIInferenceUserAgent = DefaultCodexInspectionSettings().XAIInferenceUserAgent
	}
	out.XAIInferenceEnabled = in.XAIInferenceEnabled
	out.XAIInferenceModel = strings.TrimSpace(in.XAIInferenceModel)
	if out.XAIInferenceModel == "" {
		out.XAIInferenceModel = DefaultCodexInspectionSettings().XAIInferenceModel
	}
	out.XAIInferencePrompt = strings.TrimSpace(in.XAIInferencePrompt)
	if out.XAIInferencePrompt == "" {
		out.XAIInferencePrompt = DefaultCodexInspectionSettings().XAIInferencePrompt
	}
	if in.UsedPercentThreshold < 0 || in.UsedPercentThreshold > 100 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.usedPercentThreshold must be between 0 and 100")
	}
	out.UsedPercentThreshold = in.UsedPercentThreshold
	if in.SampleSize < 0 {
		return CodexInspectionSettings{}, fmt.Errorf("codexInspection.sampleSize must be >= 0")
	}
	out.SampleSize = in.SampleSize
	switch strings.TrimSpace(in.AutoActionMode) {
	case "", "none":
		out.AutoActionMode = "none"
	case "enable", "disable", "delete":
		out.AutoActionMode = strings.TrimSpace(in.AutoActionMode)
	default:
		return CodexInspectionSettings{}, fmt.Errorf("unsupported codexInspection.autoActionMode")
	}
	out.AutoRecoverEnabled = in.AutoRecoverEnabled
	return out, nil
}

func normalizeInspectionTargetTypes(values []string, legacy string) []string {
	if values == nil {
		values = []string{legacy}
	}
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "codex" || value == "xai" {
			seen[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for _, provider := range []string{"codex", "xai"} {
		if _, ok := seen[provider]; ok {
			result = append(result, provider)
		}
	}
	return result
}

func normalizeTimePoint(value string) (string, bool) {
	value = strings.TrimSpace(value)
	var hour, minute int
	if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil {
		return "", false
	}
	// Reject values with extra trailing content by reconstructing.
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return "", false
	}
	// Ensure input is essentially HH:MM (optional leading zeros).
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", false
	}
	for _, p := range parts {
		if p == "" {
			return "", false
		}
		for _, ch := range p {
			if ch < '0' || ch > '9' {
				return "", false
			}
		}
	}
	return fmt.Sprintf("%02d:%02d", hour, minute), true
}

func (r *Runtime) loadInspectionSettings(ctx context.Context) error {
	host := r.config.Codex
	seed := DefaultCodexInspectionSettings()
	seed.Enabled = host.Enabled
	if host.ScheduleMode == "time_points" || host.ScheduleMode == "interval" {
		seed.Schedule.Mode = host.ScheduleMode
	}
	if host.IntervalMinutes >= 1 && host.IntervalMinutes <= 24*60 {
		seed.Schedule.IntervalMinutes = host.IntervalMinutes
	}
	switch host.AutoActionMode {
	case "none", "enable", "disable", "delete":
		seed.AutoActionMode = host.AutoActionMode
	}

	r.inspectionMu.Lock()
	r.inspectionSettings = seed
	r.inspectionMu.Unlock()

	raw, ok, err := r.store.Setting(ctx, inspectionSettingsKey)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	var loaded CodexInspectionSettings
	if err := json.Unmarshal(raw, &loaded); err != nil {
		return fmt.Errorf("decode codex inspection settings: %w", err)
	}
	// Accept legacy flat payloads saved by earlier prototypes.
	loaded = coerceLegacyInspectionPayload(raw, loaded)
	normalized, err := normalizeCodexInspectionSettings(loaded)
	if err != nil {
		// Keep host seed rather than failing plugin boot on bad saved data.
		return nil
	}
	r.inspectionMu.Lock()
	r.inspectionSettings = normalized
	// Mirror into Runtime.config.Codex for any code still reading host-shaped fields.
	r.config.Codex.Enabled = normalized.Enabled
	r.config.Codex.ScheduleMode = normalized.Schedule.Mode
	r.config.Codex.IntervalMinutes = normalized.Schedule.IntervalMinutes
	r.config.Codex.AutoActionMode = normalized.AutoActionMode
	r.inspectionMu.Unlock()
	return nil
}

func coerceLegacyInspectionPayload(raw []byte, loaded CodexInspectionSettings) CodexInspectionSettings {
	var flat map[string]json.RawMessage
	if err := json.Unmarshal(raw, &flat); err != nil {
		return loaded
	}
	if _, hasSchedule := flat["schedule"]; hasSchedule {
		return loaded
	}
	// Flat: scheduleMode / intervalMinutes at top level.
	var legacy struct {
		ScheduleMode    string `json:"scheduleMode"`
		IntervalMinutes int    `json:"intervalMinutes"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return loaded
	}
	if legacy.ScheduleMode != "" {
		loaded.Schedule.Mode = legacy.ScheduleMode
	}
	if legacy.IntervalMinutes > 0 {
		loaded.Schedule.IntervalMinutes = legacy.IntervalMinutes
	}
	return loaded
}

func (r *Runtime) CodexInspectionSettings() CodexInspectionSettings {
	r.inspectionMu.Lock()
	defer r.inspectionMu.Unlock()
	return r.inspectionSettings
}

func (r *Runtime) UpdateCodexInspectionSettings(ctx context.Context, settings CodexInspectionSettings) error {
	normalized, err := normalizeCodexInspectionSettings(settings)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if err := r.store.PutSetting(ctx, inspectionSettingsKey, raw); err != nil {
		return err
	}
	r.inspectionMu.Lock()
	r.inspectionSettings = normalized
	r.config.Codex.Enabled = normalized.Enabled
	r.config.Codex.ScheduleMode = normalized.Schedule.Mode
	r.config.Codex.IntervalMinutes = normalized.Schedule.IntervalMinutes
	r.config.Codex.AutoActionMode = normalized.AutoActionMode
	r.inspectionMu.Unlock()
	r.wakeInspection()
	return nil
}

func (r *Runtime) wakeInspection() {
	select {
	case r.inspectionWake <- struct{}{}:
	default:
	}
}

func (r *Runtime) scheduleInspections(ctx context.Context) {
	var lastTimePointKey string
	for {
		settings := r.CodexInspectionSettings()
		delay, fireKey := nextInspectionDelay(settings, time.Now(), lastTimePointKey)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-r.inspectionWake:
			timer.Stop()
			continue
		case <-timer.C:
			if !settings.Enabled {
				continue
			}
			if settings.Schedule.Mode == "time_points" {
				if fireKey == "" || fireKey == lastTimePointKey {
					continue
				}
				lastTimePointKey = fireKey
			}
			_, _ = r.RunInspection(ctx)
		}
	}
}

func nextInspectionDelay(settings CodexInspectionSettings, now time.Time, lastTimePointKey string) (time.Duration, string) {
	if !settings.Enabled {
		return 24 * time.Hour, ""
	}
	switch settings.Schedule.Mode {
	case "time_points":
		return nextTimePointDelay(settings.Schedule, now, lastTimePointKey)
	default:
		interval := time.Duration(settings.Schedule.IntervalMinutes) * time.Minute
		if interval <= 0 {
			interval = time.Hour
		}
		return interval, ""
	}
}

func nextTimePointDelay(schedule CodexInspectionSchedule, now time.Time, lastKey string) (time.Duration, string) {
	loc := time.Local
	if tz := strings.TrimSpace(schedule.TimeZone); tz != "" {
		if loaded, err := time.LoadLocation(tz); err == nil {
			loc = loaded
		}
	}
	localNow := now.In(loc)
	type candidate struct {
		at  time.Time
		key string
	}
	var best *candidate
	for dayOffset := 0; dayOffset <= 1; dayOffset++ {
		day := localNow.AddDate(0, 0, dayOffset)
		for _, point := range schedule.TimePoints {
			hour, minute := 0, 0
			fmt.Sscanf(point, "%d:%d", &hour, &minute)
			at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
			if !at.After(localNow) {
				continue
			}
			key := at.Format(time.RFC3339)
			if key == lastKey {
				continue
			}
			if best == nil || at.Before(best.at) {
				best = &candidate{at: at, key: key}
			}
		}
	}
	if best == nil {
		return 24 * time.Hour, ""
	}
	return best.at.Sub(now), best.key
}
