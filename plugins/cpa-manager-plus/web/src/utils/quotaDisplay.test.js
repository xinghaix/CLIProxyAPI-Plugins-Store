import { describe, expect, it } from 'vitest';
import { normalizeQuotaWindows } from './quotaDisplay.js';

describe('normalizeQuotaWindows', () => {
  it('keeps zero usage as a real percentage', () => {
    expect(normalizeQuotaWindows({ quotaWindows: [{ id: 'weekly', usedPercent: 0 }] })).toEqual([
      {
        id: 'weekly',
        label: 'weekly',
        hasUsedPercent: true,
        usedPercent: 0,
        remaining: null,
        remainingText: '',
        resetText: '',
      },
    ]);
  });

  it('does not copy the aggregate result percentage into another window', () => {
    const windows = normalizeQuotaWindows({
      usedPercent: 84,
      quotaWindows: [{ id: 'on-demand', label: 'Pay as you go', remaining: 'Enabled' }],
    });
    expect(windows).toEqual([{
      id: 'on-demand',
      label: 'Pay as you go',
      hasUsedPercent: false,
      usedPercent: 0,
      remaining: null,
      remainingText: 'Enabled',
      resetText: '',
    }]);
  });

  it('keeps reset-only windows visible', () => {
    expect(normalizeQuotaWindows({ quotaWindows: [{ id: 'monthly', resetAt: '2026-10-01T00:00:00Z' }] })).toEqual([
      {
        id: 'monthly',
        label: 'monthly',
        hasUsedPercent: false,
        usedPercent: 0,
        remaining: null,
        remainingText: '',
        resetText: '2026-10-01T00:00:00Z',
      },
    ]);
  });
});
