import { describe, expect, it } from 'vitest';
import {
  buildQuotaUsageRanges,
  normalizeCredentialQuotaWindow,
  buildAccountWindowUsageTargets,
} from './quotaWindowRanges.js';

describe('quotaWindowRanges', () => {
  it('skips stale fixed windows without inventing ranges', () => {
    const now = Date.parse('2026-09-17T12:00:00Z');
    const resetAt = '2026-09-17T11:52:00Z';
    const def = normalizeCredentialQuotaWindow({
      id: 'five_hour',
      kind: 'five_hour',
      usedPercent: 100,
      periodHours: 5,
      resetAt,
    }, 0, now);
    expect(def.windowMode).toBe('fixed');
    expect(def.stale).toBe(true);
    expect(buildQuotaUsageRanges(def, now)).toEqual([]);
  });

  it('builds current+previous when window still active', () => {
    const now = Date.parse('2026-09-17T10:00:00Z');
    const resetAt = '2026-09-17T11:52:00Z';
    const def = normalizeCredentialQuotaWindow({
      id: 'five_hour',
      kind: 'five_hour',
      usedPercent: 63,
      periodHours: 5,
      resetAt,
    }, 0, now);
    expect(def.stale).toBe(false);
    const ranges = buildQuotaUsageRanges(def, now);
    expect(ranges.map((r) => r.period)).toEqual(['current', 'previous']);
    expect(ranges[0].toMs).toBe(now);
    expect(ranges[0].fromMs).toBe(def.cycleStartMs);
  });

  it('builds rolling ranges without resetAt', () => {
    const now = 1_800_000_000_000;
    const def = normalizeCredentialQuotaWindow({
      id: 'weekly',
      kind: 'weekly',
      usedPercent: 40,
      periodHours: 168,
    }, 0, now);
    expect(def.windowMode).toBe('rolling');
    const ranges = buildQuotaUsageRanges(def, now);
    expect(ranges).toHaveLength(2);
    expect(ranges[0].period).toBe('current');
    expect(ranges[1].period).toBe('previous_equal_range');
  });

  it('builds usage targets from credential + probe without parent range', () => {
    const now = Date.parse('2026-09-17T10:00:00Z');
    const { targets } = buildAccountWindowUsageTargets(
      { rowKey: 'auth-1', authIndex: 'auth-1', provider: 'codex', fileName: 'a.json', displayName: 'u@x.com' },
      {
        quotaWindows: [{
          id: 'five_hour', kind: 'five_hour', usedPercent: 20, periodHours: 5,
          resetAt: '2026-09-17T11:52:00Z',
        }],
      },
      now
    );
    expect(targets.length).toBeGreaterThanOrEqual(2);
    expect(targets.every((t) => t.from_ms > 0 && t.to_ms > t.from_ms)).toBe(true);
    expect(targets.every((t) => t.auth_index === 'auth-1')).toBe(true);
  });
});
