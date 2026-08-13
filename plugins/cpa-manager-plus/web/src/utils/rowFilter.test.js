import { describe, expect, it } from 'vitest';
import { canApplySelectedFilter, rowIdentity } from './rowFilter.js';

describe('rowIdentity', () => {
  it('prefers id then model', () => {
    expect(rowIdentity({ id: 'm1', model: 'gpt' })).toBe('m1');
    expect(rowIdentity({ model: 'gpt-test' })).toBe('gpt-test');
  });
});

describe('canApplySelectedFilter', () => {
  it('allows filter only for the selected row', () => {
    const row = { model: 'gpt-test' };
    expect(canApplySelectedFilter('', row)).toBe(false);
    expect(canApplySelectedFilter('other', row)).toBe(false);
    expect(canApplySelectedFilter('gpt-test', row)).toBe(true);
  });
});
