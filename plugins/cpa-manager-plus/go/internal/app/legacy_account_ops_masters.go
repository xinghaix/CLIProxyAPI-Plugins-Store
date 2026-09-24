package app

import (
	"context"
	"encoding/json"
)

const legacyAccountOpsMastersKey = "legacy_account_ops_masters_v1"

// LegacyAccountOpsMasters are runtime master switches (总控) for legacy
// Auto-Ban and Inspection. Defaults are both false so upgraded installs
// with tab-level enabled=true stay stopped until the user turns a master on.
//
// Semantics:
//   - Master OFF → engine always stopped (even if tab settings.enabled=true);
//     corresponding nav Tab stays hidden.
//   - Master ON → fall through to tab-level settings; Tab becomes visible
//     so the operator can configure those settings.
type LegacyAccountOpsMasters struct {
	AutoBan    bool `json:"autoBan"`
	Inspection bool `json:"inspection"`
}

// DefaultLegacyAccountOpsMasters returns both masters off (safe upgrade default).
func DefaultLegacyAccountOpsMasters() LegacyAccountOpsMasters {
	return LegacyAccountOpsMasters{AutoBan: false, Inspection: false}
}

func (r *Runtime) loadLegacyAccountOpsMasters(ctx context.Context) error {
	masters := DefaultLegacyAccountOpsMasters()
	raw, ok, err := r.store.Setting(ctx, legacyAccountOpsMastersKey)
	if err != nil {
		return err
	}
	if ok {
		var loaded LegacyAccountOpsMasters
		if err := json.Unmarshal(raw, &loaded); err == nil {
			masters = loaded
		}
	}
	r.mastersMu.Lock()
	r.legacyAccountOpsMasters = masters
	r.mastersMu.Unlock()
	return nil
}

// LegacyAccountOpsMasters returns the persisted master switches.
func (r *Runtime) LegacyAccountOpsMasters() LegacyAccountOpsMasters {
	r.mastersMu.Lock()
	defer r.mastersMu.Unlock()
	return r.legacyAccountOpsMasters
}

// UpdateLegacyAccountOpsMasters persists master switches and wakes engines.
func (r *Runtime) UpdateLegacyAccountOpsMasters(ctx context.Context, masters LegacyAccountOpsMasters) error {
	raw, err := json.Marshal(masters)
	if err != nil {
		return err
	}
	if err := r.store.PutSetting(ctx, legacyAccountOpsMastersKey, raw); err != nil {
		return err
	}
	r.mastersMu.Lock()
	r.legacyAccountOpsMasters = masters
	r.mastersMu.Unlock()
	r.wakeAutoBan()
	r.wakeInspection()
	return nil
}

// autoBanEngineAllowed is true when the emergency compile-time gate and the
// Auto-Ban master are both on. Tab-level settings.enabled is checked separately.
func (r *Runtime) autoBanEngineAllowed() bool {
	if r == nil || !LegacyAccountOpsEnginesEnabled {
		return false
	}
	return r.LegacyAccountOpsMasters().AutoBan
}

// inspectionEngineAllowed is true when the emergency compile-time gate and the
// Inspection master are both on. Tab-level settings.enabled is checked separately.
func (r *Runtime) inspectionEngineAllowed() bool {
	if r == nil || !LegacyAccountOpsEnginesEnabled {
		return false
	}
	return r.LegacyAccountOpsMasters().Inspection
}
