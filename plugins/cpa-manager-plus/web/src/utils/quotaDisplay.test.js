import { describe, expect, it } from 'vitest';
import {
  formatQuotaResetRelative,
  formatQuotaStatusMessage,
  normalizeQuotaWindows,
} from './quotaDisplay.js';

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

  it('formats the time until a future quota reset', () => {
    const now = Date.parse('2026-08-28T06:00:00Z');
    expect(formatQuotaResetRelative('2026-08-28T06:05:00Z', now, 'zh-CN')).toBe('5分钟后');
    expect(formatQuotaResetRelative('2026-08-28T09:00:00Z', now, 'en')).toBe('in 3 hours');
    expect(formatQuotaResetRelative('2026-09-01T06:00:00Z', now, 'zh-CN')).toBe('4天后');
    expect(formatQuotaResetRelative('2026-08-28T05:59:00Z', now, 'zh-CN')).toBe('');
  });

  it('decodes and summarizes escaped structured provider errors', () => {
    expect(formatQuotaStatusMessage('{&#34;error&#34;:{&#34;message&#34;:&#34;The usage limit has been reached&#34;}}'))
      .toBe('The usage limit has been reached');
  });
});
