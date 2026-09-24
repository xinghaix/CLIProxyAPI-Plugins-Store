package app

// LegacyAccountOpsEnginesEnabled gates Auto-Ban and Codex Inspection scheduled
// engines (and Auto-Ban signal processing). Official builds keep this false:
//
//   - Scheduler loops never start work even if SQLite still has enabled=true
//     from an older install.
//   - UpdateAutoBanSettings / UpdateCodexInspectionSettings clamp Enabled to
//     false so saves cannot re-enable while the kill-switch is off.
//   - Defaults for new installs already use enabled=false.
//
// Flip to true (and restore FEATURE_*_UI in web/src/features.js) to bring back
// the legacy Account Actions / Inspection operator workflow. Engine code, APIs,
// and SQLite schema remain in tree either way.
//
// Tests may temporarily override via SetLegacyAccountOpsEnginesEnabledForTest.
var LegacyAccountOpsEnginesEnabled = false

// SetLegacyAccountOpsEnginesEnabledForTest overrides the kill-switch for tests.
// Always defer the returned restore function.
func SetLegacyAccountOpsEnginesEnabledForTest(enabled bool) (restore func()) {
	prev := LegacyAccountOpsEnginesEnabled
	LegacyAccountOpsEnginesEnabled = enabled
	return func() { LegacyAccountOpsEnginesEnabled = prev }
}

func accountOpsEnginesAllowed() bool {
	return LegacyAccountOpsEnginesEnabled
}
