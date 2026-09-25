import { describe, expect, it } from 'vitest';
import {
  buildRecentStatusSlots,
  formatCompactNumber,
  formatCompactUsd,
  formatCredentialCost,
  formatSuccessRate,
  groupRecentEventsByCredential,
  maskEmail,
  resolveAvailability,
  shortWindowLabel,
} from './credentialPresentation.js';

const t = (key, params = {}) => {
  if (key.includes('cooldown')) return `${params.window} cooldown`;
  if (key.includes('exhausted')) return `${params.window} exhausted`;
  if (key.includes('low')) return `${params.window} low`;
  if (key.includes('available')) return 'Available';
  if (key.includes('disabled')) return 'Disabled';
  if (key.includes('attention')) return 'Needs attention';
  return key;
};

describe('credentialPresentation', () => {
  it('formats compact numbers and usd', () => {
    expect(formatCompactNumber(3700)).toBe('3.7K');
    expect(formatCompactNumber(454400000)).toMatch(/454\.?4?M/);
    expect(formatCompactUsd(205.88)).toBe('$205.88');
    expect(formatCompactUsd(1200)).toBe('$1.2K');
  });

  it('never shows bare $0 for incomplete pricing', () => {
    expect(formatCredentialCost({ cost: 0, costComplete: false, unpricedCalls: 3 }, 'n/a')).toBe('n/a');
    expect(formatCredentialCost({ cost: 12.5, costComplete: false, unpricedCalls: 1 }, 'n/a')).toBe('~$12.50*');
    expect(formatCredentialCost({ cost: 12.5, costComplete: true, unpricedCalls: 0 })).toBe('$12.50');
  });

  it('formats success rate from ratio or percent', () => {
    expect(formatSuccessRate(0.9935)).toBe('99.35%');
    expect(formatSuccessRate(99.35)).toBe('99.35%');
  });

  it('masks emails like the list identity column', () => {
    expect(maskEmail('huiabc@gmail.com')).toBe('hui***@gmail.com');
    expect(maskEmail('ab@x.com')).toBe('a***@x.com');
  });

  it('uses cooldown wording when a window is exhausted', () => {
    const result = resolveAvailability(
      { quotaWindows: [{ label: '5-hour limit', kind: 'five_hour', remainingPercent: 0 }] },
      {},
      t
    );
    expect(result.label).toBe('5h cooldown');
    expect(result.tone).toBe('cooldown');
    expect(result.bucket).toBe('quota_risk');
  });

  it('builds padded recent status slots oldest to newest', () => {
    const slots = buildRecentStatusSlots(
      [{ failed: false }, { failed: true }, { failed: false }],
      5
    );
    expect(slots).toEqual([null, null, 'ok', 'fail', 'ok']);
  });

  it('groups analytics events onto credential row keys', () => {
    const map = groupRecentEventsByCredential(
      [
        { auth_index: 'a1', failed: false, timestamp_ms: 3 },
        { source: 'b.json', failed: true, timestamp_ms: 2 },
        { auth_index: 'missing', failed: false, timestamp_ms: 1 },
      ],
      [
        { rowKey: 'row-a', authIndex: 'a1', fileName: 'a.json' },
        { rowKey: 'row-b', authIndex: 'b1', fileName: 'b.json' },
      ]
    );
    expect(map.get('row-a')).toHaveLength(1);
    expect(map.get('row-b')).toHaveLength(1);
    expect(map.get('row-b')[0].failed).toBe(true);
  });

  it('shortens window labels for list denseness', () => {
    expect(shortWindowLabel('5-hour limit', 'five_hour')).toBe('5h');
    expect(shortWindowLabel('Weekly limit', 'weekly')).toBe('Weekly');
  });
});
