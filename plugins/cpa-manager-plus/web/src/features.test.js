import {describe, expect, it} from 'vitest';
import {FEATURE_ACCOUNT_ACTIONS_UI, FEATURE_INSPECTION_UI, isAccountOpsTabVisible} from './features.js';

describe('isAccountOpsTabVisible', () => {
  it('hides account-ops tabs when masters are off', () => {
    expect(FEATURE_ACCOUNT_ACTIONS_UI).toBe(true);
    expect(FEATURE_INSPECTION_UI).toBe(true);
    expect(isAccountOpsTabVisible('account-actions', {autoBan: false, inspection: false})).toBe(false);
    expect(isAccountOpsTabVisible('inspection', {autoBan: false, inspection: false})).toBe(false);
    expect(isAccountOpsTabVisible('dashboard', {})).toBe(true);
  });

  it('shows tabs only when corresponding master is on', () => {
    expect(isAccountOpsTabVisible('account-actions', {autoBan: true, inspection: false})).toBe(true);
    expect(isAccountOpsTabVisible('inspection', {autoBan: true, inspection: false})).toBe(false);
    expect(isAccountOpsTabVisible('inspection', {autoBan: false, inspection: true})).toBe(true);
  });
});
