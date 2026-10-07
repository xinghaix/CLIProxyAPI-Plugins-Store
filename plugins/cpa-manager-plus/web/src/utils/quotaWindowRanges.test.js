import { describe, expect, it } from 'vitest';
import {
  buildQuotaUsageRanges, normalizeCredentialQuotaWindow, buildAccountWindowUsageTargets,
  resolveWindowUsagePresentation, usageItemToMetrics,
} from './quotaWindowRanges.js';

const now = Date.parse('2026-09-17T10:00:00Z');
const credential = { rowKey: 'row', authIndex: 'index', authId: 'id', fileName: 'a.json', source: 'real-source', provider: 'codex' };
const window = { id: 'five_hour', kind: 'five_hour', usedPercent: 20, periodHours: 5, resetAt: '2026-09-17T11:52:00Z', modelScope: 'account' };
const snapshot = (overrides = {}, at = now) => buildAccountWindowUsageTargets(credential, { quotaWindows: [{ ...window, ...overrides }] }, at);
const response = (target, overrides = {}) => ({
  request_key: target.request_key, from_ms: target.from_ms, to_ms: target.to_ms, period: target.period,
  matched: true, sync_status: 'ready', scope_match_status: 'complete', cost_complete: true,
  total_requests: 10, total_tokens: 1000, total_cost: 2, ...overrides,
});
const responses = (targets, overrides = {}) => new Map(targets.map(target => [target.request_key, response(target, overrides)]));
const present = (data, usage = responses(data.targets), at = now) => resolveWindowUsagePresentation(data.windows[0], usage, credential.rowKey, at);

describe('quotaWindowRanges', () => {
  it('skips stale fixed windows without inventing ranges', () => {
    const at = Date.parse('2026-09-17T12:00:00Z');
    const def = normalizeCredentialQuotaWindow({ ...window, usedPercent: 100 }, 0, at);
    expect(def.windowMode).toBe('fixed');
    expect(def.stale).toBe(true);
    expect(buildQuotaUsageRanges(def, at)).toEqual([]);
  });

  it('builds current+previous when window still active', () => {
    const def = normalizeCredentialQuotaWindow({ ...window, usedPercent: 63 }, 0, now);
    expect(def.stale).toBe(false);
    const ranges = buildQuotaUsageRanges(def, now);
    expect(ranges.map(r => r.period)).toEqual(['current', 'previous']);
    expect(ranges[0].toMs).toBe(now);
    expect(ranges[0].fromMs).toBe(def.cycleStartMs);
  });

  it('builds rolling ranges without resetAt', () => {
    const def = normalizeCredentialQuotaWindow({ id: 'weekly', kind: 'weekly', usedPercent: 40, periodHours: 168 }, 0, now);
    expect(def.windowMode).toBe('rolling');
    const ranges = buildQuotaUsageRanges(def, now);
    expect(ranges).toHaveLength(2);
    expect(ranges[0].period).toBe('current');
    expect(ranges[1].period).toBe('previous_equal_range');
  });

  it('builds usage targets from credential + probe without parent range', () => {
    const { targets } = snapshot();
    expect(targets.length).toBeGreaterThanOrEqual(2);
    expect(targets.every(t => t.from_ms > 0 && t.to_ms > t.from_ms)).toBe(true);
    expect(targets.every(t => t.auth_index === 'index')).toBe(true);
  });
});

describe('quota usage snapshot isolation', () => {
  it('preserves exact model IDs and skips unknown or unmapped model scopes', () => {
    const models = ['GPT-5', 'claude-sonnet-4-20250514'];
    const data = snapshot({ modelScope: 'model', modelScopeModels: models });
    expect(data.targets).toHaveLength(2);
    expect(data.windows[0]).toMatchObject({ sampledAtMs: now, modelScope: 'model', modelScopeModels: models });
    for (const target of data.targets) expect(target.model_scope_models).toEqual(models);
    for (const scope of [undefined, 'unknown', 'model']) expect(snapshot({ modelScope: scope }).targets).toEqual([]);
    expect(snapshot({ modelScope: 'unknown', modelScopeModels: models }).targets).toEqual([]);
    for (const models of [[''], [' GPT-5'], ['GPT-5 '], [null]]) {
      expect(snapshot({ modelScope: 'model', modelScopeModels: models }).targets).toEqual([]);
    }
    expect(snapshot().targets).toHaveLength(2);
    expect(snapshot().targets.every(target => target.model_scope_models.length === 0)).toBe(true);
  });

  it('handles raw list definitions without inventing a query or throwing', () => {
    for (const modelScope of ['unknown', 'account']) {
      expect(resolveWindowUsagePresentation({ modelScope }, new Map(), 'row', now))
        .toMatchObject({ current: null, previous: null, forecast: null });
    }
  });

  it('retains missing percentages instead of converting them to zero', () => {
    for (const usedPercent of [null, undefined, '']) expect(snapshot({ usedPercent }).windows[0].usedPercent).toBeNull();
    expect(snapshot({ usedPercent: 0 }).windows[0].usedPercent).toBe(0);
  });

  it('uses exact fixed boundaries and preserves them while rendering later', () => {
    const data = snapshot();
    expect(data.targets.map(({ period, from_ms, to_ms }) => ({ period, from_ms, to_ms }))).toEqual([
      { period: 'current', from_ms: Date.parse('2026-09-17T06:52:00Z'), to_ms: now },
      { period: 'previous', from_ms: Date.parse('2026-09-17T01:52:00Z'), to_ms: Date.parse('2026-09-17T06:52:00Z') },
    ]);
    expect(present(data, responses(data.targets), now + 1000)).toMatchObject({
      current: { requests: 10 }, previous: { requests: 10 },
      forecast: { requests: 50, tokens: 5000, cost: 10, basis: 'quota' }, currentToMs: now,
    });
  });

  it('does not reuse old responses across cycle, scope, percent or sample revisions', () => {
    const old = snapshot();
    const usage = responses(old.targets);
    for (const data of [
      snapshot({ resetAt: '2026-09-17T16:52:00Z' }),
      snapshot({ modelScope: 'model', modelScopeModels: ['GPT-5'] }),
      snapshot({ usedPercent: 40 }), snapshot({}, now + 1000),
    ]) {
      expect(data.targets.every(target => !usage.has(target.request_key))).toBe(true);
      expect(present(data, usage, now + 1000)).toMatchObject({ current: null, previous: null, forecast: null });
    }
    const a = snapshot({ modelScope: 'model', modelScopeModels: ['GPT-5'] });
    const b = snapshot({ modelScope: 'model', modelScopeModels: ['gpt-5'] });
    expect(present(b, responses(a.targets))).toMatchObject({ current: null, previous: null, forecast: null });
  });

  it('keeps rolling query ranges stable for a snapshot, not render time', () => {
    const data = snapshot({ resetAt: null });
    expect(data.targets.map(target => [target.period, target.from_ms, target.to_ms])).toEqual([
      ['current', now - 5 * 3600_000, now],
      ['previous_equal_range', now - 10 * 3600_000, now - 5 * 3600_000],
    ]);
    expect(present(data, responses(data.targets), now + 60_000)).toMatchObject({
      current: { requests: 10 }, previousPeriod: 'previous_equal_range', currentToMs: now,
    });
  });

  it.each(['from_ms', 'to_ms'])('rejects mismatched response %s boundaries', field => {
    const data = snapshot();
    const usage = new Map(data.targets.map(target => [target.request_key, response(target, { [field]: target[field] + 1 })]));
    expect(present(data, usage)).toMatchObject({ current: null, previous: null, forecast: null, currentFromMs: null, previousToMs: null });
  });

  it('requires the echoed request identity as well as the map key', () => {
    const data = snapshot();
    expect(present(data, responses(data.targets, { request_key: 'old-key' })))
      .toMatchObject({ current: null, previous: null, forecast: null });
  });

  it.each([
    { matched: false }, { sync_status: 'syncing' }, { sync_status: undefined },
    { scope_match_status: 'partial' }, { scope_match_status: undefined },
  ])('rejects unready or incomplete responses: %j', overrides => {
    const data = snapshot();
    expect(usageItemToMetrics(response(data.targets[0], overrides))).toBeNull();
    expect(present(data, responses(data.targets, overrides)))
      .toMatchObject({ current: null, previous: null, forecast: null });
  });

  it('expires exactly at reset without predicting from previous usage', () => {
    const data = snapshot();
    const expiry = data.windows[0].cycleEndMs;
    expect(present(data, responses(data.targets), expiry - 1).forecast).not.toBeNull();
    expect(buildQuotaUsageRanges(data.windows[0], expiry)).toEqual([]);
    expect(present(data, responses(data.targets), expiry)).toMatchObject({ current: null, previous: null, forecast: null });
  });

  it.each([{ cost_complete: false }, { unpriced_calls: 1 }])('shows usage but does not forecast incomplete costs: %j', overrides => {
    const data = snapshot();
    expect(present(data, responses(data.targets, overrides)))
      .toMatchObject({ current: { requests: 10, costComplete: false }, previous: { costComplete: false }, forecast: null });
    const noProgress = snapshot({ usedPercent: null });
    expect(present(noProgress, responses(noProgress.targets, overrides)).forecast).toBeNull();
  });

  it('keeps auth ID, index, filename and real source separate in window payloads', () => {
    for (const target of snapshot().targets) {
      expect(target).toMatchObject({ auth_id: 'id', auth_index: 'index', auth_file_snapshot: 'a.json', source: 'real-source' });
    }
    const idOnly = { rowKey: 'row', authId: 'id' };
    for (const target of [
      ...buildAccountWindowUsageTargets(idOnly, { quotaWindows: [window] }, now).targets,
    ]) expect(target).toMatchObject({ auth_id: 'id', auth_index: '', auth_file_snapshot: '', source: '' });
  });
});
