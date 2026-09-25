import { describe, expect, it } from 'vitest';
import {
  buildRecentStatusSlots,
  formatCompactNumber,
  formatCompactUsd,
  formatCredentialCost,
  formatRemainingPercent,
  formatSuccessRate,
  groupRecentEventsByCredential,
  isProbeFailure,
  localizeAuthStatus,
  maskEmail,
  probeFailureMessage,
  resolveAvailability,
  shortWindowLabel,
} from './credentialPresentation.js';

const t = (key, params = {}) => {
  if (key.includes('cooldown')) return `${params.window} cooldown`;
  if (key.includes('exhausted')) return `${params.window} exhausted`;
  if (key.includes('low')) return `${params.window} low`;
  if (key.includes('probeFailed')) return 'Probe failed';
  if (key.includes('available') && key.includes('availability')) return 'Available';
  if (key.includes('disabled')) return 'Disabled';
  if (key.includes('attention')) return 'Needs attention';
  if (key.includes('status.active')) return 'Active';
  if (key.includes('status.available')) return 'Available';
  if (key.includes('status.ok')) return 'OK';
  if (key.includes('status.enabled')) return 'Enabled';
  if (key.includes('status.error')) return 'Error';
  if (key.includes('status.unavailable')) return 'Unavailable';
  if (key.includes('status.expired')) return 'Expired';
  if (key.includes('status.pending')) return 'Pending';
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
    expect(result.bucket).toBe('attention');
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

  it('shows depleted copy instead of bare 0% when remaining ≤ 0', () => {
    expect(formatRemainingPercent(0, '已耗尽')).toBe('已耗尽');
    expect(formatRemainingPercent(-1, 'Exhausted')).toBe('Exhausted');
    expect(formatRemainingPercent(18, 'Exhausted')).toBe('18%');
    expect(formatRemainingPercent(null, 'Exhausted')).toBe('—');
  });


  it('distinguishes zero complete cost vs missing vs unpriced', () => {
    expect(formatCredentialCost({ cost: 0, costComplete: true, unpricedCalls: 0 })).toBe('$0.00');
    expect(formatCredentialCost(null, 'n/a')).toBe('—');
    expect(formatCredentialCost(undefined, 'n/a')).toBe('—');
    expect(formatCredentialCost({ cost: 0, costComplete: false, unpricedCalls: 0 }, 'n/a')).toBe('n/a');
    expect(formatCredentialCost({ cost: 0, costComplete: true, unpricedCalls: 2 }, 'n/a')).toBe('n/a');
    expect(formatCredentialCost({ cost: 0, unpricedCalls: 1 }, 'n/a')).toBe('n/a');
    expect(formatCredentialCost({ cost: 3.5, costComplete: false, unpricedCalls: 1 }, 'n/a')).toBe('~$3.50*');
  });

  it('keeps low remaining under quota_risk (not attention)', () => {
    const low = resolveAvailability(
      { quotaWindows: [{ label: 'Weekly', kind: 'weekly', remainingPercent: 18 }] },
      {},
      t
    );
    expect(low.bucket).toBe('quota_risk');
    expect(low.tone).toBe('warn');
  });

  it('maps disabled credentials to off tone', () => {
    const result = resolveAvailability({ disabled: true, quotaWindows: [] }, {}, t);
    expect(result.tone).toBe('off');
    expect(result.bucket).toBe('disabled');
  });

  it('treats CPA status "active" as healthy available (not raw English chip)', () => {
    const result = resolveAvailability({ status: 'active', quotaWindows: [] }, { action: 'keep', errorKind: 'healthy' }, t);
    expect(result.label).toBe('Available');
    expect(result.bucket).toBe('available');
    expect(result.tone).toBe('ok');
  });

  it('maps probe failure away from healthy active into attention/探测失败', () => {
    const result = resolveAvailability(
      { status: 'active', quotaWindows: [] },
      { action: 'review', actionReason: '探测请求失败，需人工复核', errorKind: 'needs_review' },
      t
    );
    expect(result.label).toBe('Probe failed');
    expect(result.bucket).toBe('attention');
    expect(result.tone).toBe('warn');
    expect(isProbeFailure({ action: 'review', actionReason: '探测请求失败，需人工复核' })).toBe(true);
    expect(probeFailureMessage({ actionReason: '探测请求失败，需人工复核' })).toBe('探测请求失败，需人工复核');
  });

  it('localizes raw auth status codes instead of surfacing English', () => {
    expect(localizeAuthStatus('active', t)).toBe('Active');
    expect(localizeAuthStatus('expired', t)).toBe('Expired');
    expect(localizeAuthStatus('weird_code', t)).toBe('Needs attention');
  });
});
