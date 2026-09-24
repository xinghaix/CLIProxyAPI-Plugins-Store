package app

// LegacyAccountOpsEnginesEnabled is an emergency compile-time kill-switch that
// ANDs with the runtime masters in legacy_account_ops_masters_v1.
//
// Prefer the settings-backed masters (Config UI 总控) for day-to-day control.
// Keep this true in official builds; set false only for an emergency hard-stop
// that cannot be overridden from the UI.
//
// Tests may temporarily override via SetLegacyAccountOpsEnginesEnabledForTest.
var LegacyAccountOpsEnginesEnabled = true

// SetLegacyAccountOpsEnginesEnabledForTest overrides the emergency kill-switch.
// Always defer the returned restore function.
func SetLegacyAccountOpsEnginesEnabledForTest(enabled bool) (restore func()) {
	prev := LegacyAccountOpsEnginesEnabled
	LegacyAccountOpsEnginesEnabled = enabled
	return func() { LegacyAccountOpsEnginesEnabled = prev }
}
