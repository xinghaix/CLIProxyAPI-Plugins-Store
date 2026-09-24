/**
 * UI feature flags for CPA Manager Plus.
 *
 * Compile-time emergency gates (keep true in official builds). Day-to-day
 * visibility of Account Actions / Inspection tabs is driven by runtime masters
 * in plugin settings key `legacy_account_ops_masters_v1` (Config UI 总控).
 * Set a FEATURE_* constant to false only for an emergency hard-hide that
 * cannot be overridden from the UI.
 */
export const FEATURE_ACCOUNT_ACTIONS_UI = true;
export const FEATURE_INSPECTION_UI = true;

/**
 * Tab visible iff compile-time FEATURE is on AND the corresponding runtime
 * master is on. Engines run iff master ON AND tab-level settings.enabled.
 */
export function isAccountOpsTabVisible(tabKey, masters = {}) {
  if (tabKey === 'account-actions') {
    return FEATURE_ACCOUNT_ACTIONS_UI && Boolean(masters.autoBan);
  }
  if (tabKey === 'inspection') {
    return FEATURE_INSPECTION_UI && Boolean(masters.inspection);
  }
  return true;
}

/** @deprecated Prefer isAccountOpsTabVisible(tab, masters). Kept for older callers. */
export const HIDDEN_ACCOUNT_OPS_TABS = Object.freeze(['account-actions', 'inspection']);
