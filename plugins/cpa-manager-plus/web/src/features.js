/**
 * UI feature flags for CPA Manager Plus.
 * Flip these to true to restore the legacy Account Actions / Inspection tabs.
 * Backend engines stay gated separately by Go LegacyAccountOpsEnginesEnabled.
 */
export const FEATURE_ACCOUNT_ACTIONS_UI = false;
export const FEATURE_INSPECTION_UI = false;

/** Tabs that must not appear in nav or be selectable while their feature flag is off. */
export const HIDDEN_ACCOUNT_OPS_TABS = Object.freeze([
  ...(FEATURE_ACCOUNT_ACTIONS_UI ? [] : ['account-actions']),
  ...(FEATURE_INSPECTION_UI ? [] : ['inspection']),
]);

export function isAccountOpsTabVisible(tabKey) {
  if (tabKey === 'account-actions') return FEATURE_ACCOUNT_ACTIONS_UI;
  if (tabKey === 'inspection') return FEATURE_INSPECTION_UI;
  return true;
}
