package app

import (
	"context"
	"testing"
	"time"
)

func TestLegacyAccountOpsMastersDefaultOffForceStopsEngines(t *testing.T) {
	// Emergency compile-time gate stays on; masters default false.
	restore := SetLegacyAccountOpsEnginesEnabledForTest(true)
	defer restore()

	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()

	masters := runtime.LegacyAccountOpsMasters()
	if masters.AutoBan || masters.Inspection {
		t.Fatalf("masters must default false, got %#v", masters)
	}
	if runtime.autoBanEngineAllowed() || runtime.inspectionEngineAllowed() {
		t.Fatal("engines must be disallowed while masters are off")
	}

	// Tab-level enabled can be stored true; engines still stay off.
	if err := runtime.UpdateAutoBanSettings(ctx, AutoBanSettings{
		Enabled:                   true,
		Sources:                   AutoBanSources{Usage: true, Inspection: true},
		SchedulerIntervalSeconds:  30,
		DefaultCodexCooldownHours: 5,
		HistoryRetentionDays:      90,
	}); err != nil {
		t.Fatal(err)
	}
	if !runtime.AutoBanSettings().Enabled {
		t.Fatal("tab-level auto-ban enabled should persist while master is off")
	}
	if runtime.autoBanEngineAllowed() {
		t.Fatal("auto-ban engine must stay gated by master")
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
	if !runtime.CodexInspectionSettings().Enabled {
		t.Fatal("tab-level inspection enabled should persist while master is off")
	}
	if runtime.inspectionEngineAllowed() {
		t.Fatal("inspection engine must stay gated by master")
	}

	delay, _ := nextInspectionDelay(runtime.CodexInspectionSettings(), runtime.inspectionEngineAllowed(), time.Now(), "")
	if delay < 23*time.Hour {
		t.Fatalf("disabled inspection scheduler should sleep ~24h, got %s", delay)
	}
}

func TestLegacyAccountOpsMastersOnFallsThroughToTabSettings(t *testing.T) {
	restore := SetLegacyAccountOpsEnginesEnabledForTest(true)
	defer restore()

	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()

	if err := runtime.UpdateLegacyAccountOpsMasters(ctx, LegacyAccountOpsMasters{AutoBan: true, Inspection: true}); err != nil {
		t.Fatal(err)
	}
	if !runtime.autoBanEngineAllowed() || !runtime.inspectionEngineAllowed() {
		t.Fatal("engines should be allowed when masters are on")
	}

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
		t.Fatal("auto-ban should enable when master is on")
	}

	// Turning only Auto-Ban master off must not affect Inspection.
	if err := runtime.UpdateLegacyAccountOpsMasters(ctx, LegacyAccountOpsMasters{AutoBan: false, Inspection: true}); err != nil {
		t.Fatal(err)
	}
	if runtime.autoBanEngineAllowed() {
		t.Fatal("auto-ban master off must stop auto-ban engine")
	}
	if !runtime.inspectionEngineAllowed() {
		t.Fatal("inspection master should remain independent")
	}
}

func TestLegacyAccountOpsEmergencyKillSwitchOverridesMasters(t *testing.T) {
	restore := SetLegacyAccountOpsEnginesEnabledForTest(false)
	defer restore()

	runtime, err := New([]byte("data_dir: " + t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx := context.Background()

	if err := runtime.UpdateLegacyAccountOpsMasters(ctx, LegacyAccountOpsMasters{AutoBan: true, Inspection: true}); err != nil {
		t.Fatal(err)
	}
	if runtime.autoBanEngineAllowed() || runtime.inspectionEngineAllowed() {
		t.Fatal("emergency kill-switch must override masters")
	}
}

func TestLegacyAccountOpsMastersPersistAcrossReload(t *testing.T) {
	restore := SetLegacyAccountOpsEnginesEnabledForTest(true)
	defer restore()

	dir := t.TempDir()
	runtime, err := New([]byte("data_dir: " + dir))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := runtime.UpdateLegacyAccountOpsMasters(ctx, LegacyAccountOpsMasters{AutoBan: true, Inspection: false}); err != nil {
		t.Fatal(err)
	}
	runtime.Close()

	runtime2, err := New([]byte("data_dir: " + dir))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime2.Close()
	masters := runtime2.LegacyAccountOpsMasters()
	if !masters.AutoBan || masters.Inspection {
		t.Fatalf("persisted masters = %#v", masters)
	}
}
