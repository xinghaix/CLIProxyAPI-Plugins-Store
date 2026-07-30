package app

import (
	"context"
	"encoding/json"
	"fmt"
)

const autoBanSettingsKey = "auto_ban_settings_v1"

// AutoBanSources selects which persisted signals may trigger Auto-Ban rules.
type AutoBanSources struct {
	Usage      bool `json:"usage"`
	Inspection bool `json:"inspection"`
}

// AutoBanSettings is persisted independently from the legacy inspection settings.
type AutoBanSettings struct {
	Enabled                   bool           `json:"enabled"`
	Sources                   AutoBanSources `json:"sources"`
	SchedulerIntervalSeconds  int            `json:"schedulerIntervalSeconds"`
	DefaultCodexCooldownHours int            `json:"defaultCodexCooldownHours"`
	HistoryRetentionDays      int            `json:"historyRetentionDays"`
	DryRun                    bool           `json:"dryRun"`
}

func DefaultAutoBanSettings() AutoBanSettings {
	return AutoBanSettings{
		Enabled:                   false,
		Sources:                   AutoBanSources{Usage: true, Inspection: true},
		SchedulerIntervalSeconds:  30,
		DefaultCodexCooldownHours: 5,
		HistoryRetentionDays:      90,
		DryRun:                    false,
	}
}

func normalizeAutoBanSettings(in AutoBanSettings) (AutoBanSettings, error) {
	out := DefaultAutoBanSettings()
	out.Enabled = in.Enabled
	out.Sources = in.Sources
	if in.SchedulerIntervalSeconds != 0 {
		if in.SchedulerIntervalSeconds < 5 || in.SchedulerIntervalSeconds > 3600 {
			return AutoBanSettings{}, fmt.Errorf("autoBan.schedulerIntervalSeconds must be between 5 and 3600")
		}
		out.SchedulerIntervalSeconds = in.SchedulerIntervalSeconds
	}
	if in.DefaultCodexCooldownHours != 0 {
		if in.DefaultCodexCooldownHours < 1 || in.DefaultCodexCooldownHours > 24*30 {
			return AutoBanSettings{}, fmt.Errorf("autoBan.defaultCodexCooldownHours must be between 1 and 720")
		}
		out.DefaultCodexCooldownHours = in.DefaultCodexCooldownHours
	}
	if in.HistoryRetentionDays != 0 {
		if in.HistoryRetentionDays < 7 || in.HistoryRetentionDays > 3650 {
			return AutoBanSettings{}, fmt.Errorf("autoBan.historyRetentionDays must be between 7 and 3650")
		}
		out.HistoryRetentionDays = in.HistoryRetentionDays
	}
	out.DryRun = in.DryRun
	return out, nil
}

func (r *Runtime) loadAutoBanSettings(ctx context.Context) error {
	settings := DefaultAutoBanSettings()
	raw, ok, err := r.store.Setting(ctx, autoBanSettingsKey)
	if err != nil {
		return err
	}
	if ok {
		var loaded AutoBanSettings
		if err := json.Unmarshal(raw, &loaded); err == nil {
			if normalized, err := normalizeAutoBanSettings(loaded); err == nil {
				settings = normalized
			}
		}
	}
	r.autoBanMu.Lock()
	r.autoBanSettings = settings
	r.autoBanMu.Unlock()
	return nil
}

func (r *Runtime) AutoBanSettings() AutoBanSettings {
	r.autoBanMu.Lock()
	defer r.autoBanMu.Unlock()
	return r.autoBanSettings
}

func (r *Runtime) UpdateAutoBanSettings(ctx context.Context, settings AutoBanSettings) error {
	normalized, err := normalizeAutoBanSettings(settings)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return err
	}
	if err := r.store.PutSetting(ctx, autoBanSettingsKey, raw); err != nil {
		return err
	}
	r.autoBanMu.Lock()
	r.autoBanSettings = normalized
	r.autoBanMu.Unlock()
	r.wakeAutoBan()
	return nil
}

func (r *Runtime) wakeAutoBan() {
	select {
	case r.autoBanWake <- struct{}{}:
	default:
	}
}
