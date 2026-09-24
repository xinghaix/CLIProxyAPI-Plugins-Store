package app

import (
	"context"
	"testing"
	"time"
)

func TestLegacyAccountOpsKillSwitchClampsSettingsAndSkipsSchedulerWork(t *testing.T) {
	restore := SetLegacyAccountOpsEnginesEnabledForTest(false)
	defer restore()

	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()

	if err := runtime.UpdateAutoBanSettings(ctx, AutoBanSettings{
		Enabled:                   true,
		Sources:                   AutoBanSources{Usage: true, Inspection: true},
		SchedulerIntervalSeconds:  30,
		DefaultCodexCooldownHours: 5,
		HistoryRetentionDays:      90,
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.AutoBanSettings().Enabled {
		t.Fatalf("auto-ban enabled must clamp to false when kill-switch is off")
	}

	if err := runtime.UpdateCodexInspectionSettings(ctx, CodexInspectionSettings{
		Enabled: true,
		Schedule: CodexInspectionSchedule{
			Mode:            "interval",
			IntervalMinutes: 30,
			TimePoints:      []string{},
		},
		TargetTypes:          []string{"codex"},
		Workers:              4,
		DeleteWorkers:        4,
		Timeout:              15000,
		Retries:              0,
		UsedPercentThreshold: 90,
		AutoActionMode:       "none",
	}); err != nil {
		t.Fatal(err)
	}
	if runtime.CodexInspectionSettings().Enabled {
		t.Fatalf("inspection enabled must clamp to false when kill-switch is off")
	}

	delay, _ := nextInspectionDelay(runtime.CodexInspectionSettings(), time.Now(), "")
	if delay < 23*time.Hour {
		t.Fatalf("disabled inspection scheduler should sleep ~24h, got %s", delay)
	}
}

func TestLegacyAccountOpsKillSwitchAllowsEnableWhenOn(t *testing.T) {
	restore := SetLegacyAccountOpsEnginesEnabledForTest(true)
	defer restore()

	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()

	if err := runtime.UpdateAutoBanSettings(ctx, AutoBanSettings{
		Enabled:                   true,
		Sources:                   AutoBanSources{Usage: true},
		SchedulerIntervalSeconds:  30,
		DefaultCodexCooldownHours: 5,
		HistoryRetentionDays:      90,
	}); err != nil {
		t.Fatal(err)
	}
	if !runtime.AutoBanSettings().Enabled {
		t.Fatalf("auto-ban should enable when kill-switch is on")
	}
}
