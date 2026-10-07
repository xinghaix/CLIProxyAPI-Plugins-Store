import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRenderer, nextTick, ssrContextKey } from 'vue';
import { createI18n } from 'vue-i18n';
import CredentialsTab from './CredentialsTab.vue';
import MonitoringView from '../MonitoringView.vue';
import en from '../../i18n/messages/en.js';
import { clearAllQuotaCache, getQuotaCacheEntry, quotaCacheKey, setQuotaCacheEntry, QUOTA_ERROR_COOLDOWN_MS } from '../../utils/quotaCache.js';

// Real setup/controller, following MonitoringView.test.js; no browser dependency.
const renderer = createRenderer({ createComment: () => ({}), insert() {}, remove() {}, parentNode: () => null, nextSibling: () => null });
const now = Date.parse('2026-09-17T10:00:00Z');
const window = { id: 'five_hour', kind: 'five_hour', modelScope: 'account', usedPercent: 80, remainingPercent: 20, periodHours: 5, resetAt: '2026-09-17T11:52:00Z' };
const credential = { rowKey: 'a', authIndex: 'a', authId: 'a.json', fileName: 'a.json', source: 'real-source', provider: 'codex', authType: 'oauth', status: 'active', disabled: false };
const monitoringRow = { auth_index: 'a', auth_id: 'a.json', file_name: 'a.json', source: 'real-source', auth_provider_snapshot: 'codex', auth_type: 'oauth' };
const probe = { action: 'keep', errorKind: 'healthy', quotaWindows: [window] };
const deferred = () => Promise.withResolvers();
const flush = async () => { for (let i = 0; i < 16; i++) await Promise.resolve(); await nextTick(); };
const usageResponse = targets => ({ items: targets.map(target => ({ ...target, matched: true, sync_status: 'ready', scope_match_status: 'complete', total_requests: 10, total_tokens: 1000, total_cost: 2, cost_complete: true })) });
let apps;
function mount(component, proxyCall, ready = false) {
  const app = renderer.createApp({ ...component, render: () => null }, { ready, proxyCall });
  app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
  app.provide(ssrContextKey, {});
  apps.push(app);
  return app.mount({}).$.setupState;
}
beforeEach(() => {
  apps = [];
  vi.useFakeTimers(); vi.setSystemTime(now); clearAllQuotaCache();
  vi.stubGlobal('window', { innerWidth: 1200, innerHeight: 800, setInterval, clearInterval });
  vi.stubGlobal('HTMLElement', class {});
  vi.stubGlobal('document', { activeElement: null, querySelectorAll: () => [] });
  vi.stubGlobal('requestAnimationFrame', () => 0);
});
afterEach(() => { apps.forEach(app => app.unmount()); clearAllQuotaCache(); vi.unstubAllGlobals(); vi.useRealTimers(); });

describe('credential cache and inventory contracts', () => {
  it.each(['credentials', 'monitoring'])('shares one raw probe result when %s starts first', async leader => {
    const pending = deferred(), proxyCall = vi.fn(() => pending.promise);
    const creds = mount(CredentialsTab, proxyCall), monitor = mount(MonitoringView, proxyCall);
    const calls = { credentials: () => creds.probeCredential(credential), monitoring: () => monitor.queryAccountQuota(monitoringRow) };
    const first = calls[leader](), second = calls[leader === 'credentials' ? 'monitoring' : 'credentials']();
    await flush(); expect(proxyCall).toHaveBeenCalledTimes(1);
    pending.resolve(probe);
    expect(await first).toEqual(probe); expect(await second).toEqual(probe);
    expect(getQuotaCacheEntry(quotaCacheKey(monitoringRow)).result).toEqual(probe);
  });
  it.each([
    ['monitoring', { error: 'upstream unavailable' }],
    ['credentials', { error: 'upstream unavailable' }],
    ['monitoring', { action: 'review', errorKind: 'network', actionReason: 'offline' }],
    ['credentials', { action: 'review', errorKind: 'network', actionReason: 'offline' }],
  ])('does not replace a current failure with historical success (%s, %j)', async (leader, failure) => {
    const pending = deferred();
    const stored = { ...probe, provider: 'codex', authId: 'a.json', authIndex: 'a', fileName: 'a.json' };
    const proxyCall = vi.fn(p => {
      if (p.method === 'POST') return pending.promise;
      if (p.path.endsWith('/runs')) return Promise.resolve({ items: [{ id: 'old', status: 'completed' }] });
      if (p.path.endsWith('/runs/old')) return Promise.resolve({ results: [stored] });
      return Promise.resolve({ items: [credential] });
    });
    const creds = mount(CredentialsTab, proxyCall), monitor = mount(MonitoringView, proxyCall);
    const calls = { credentials: () => creds.probeCredential(credential), monitoring: () => monitor.queryAccountQuota(monitoringRow) };
    const first = calls[leader](), second = calls[leader === 'credentials' ? 'monitoring' : 'credentials']();
    await flush(); pending.resolve(failure);
    expect(await first).toEqual(failure); expect(await second).toEqual(failure);
    const entry = getQuotaCacheEntry(quotaCacheKey(monitoringRow));
    expect(entry.nextRequestAt - entry.fetchedAt).toBe(QUOTA_ERROR_COOLDOWN_MS);
    const loaded = mount(CredentialsTab, proxyCall, true); await flush();
    expect(loaded.rows[0]).toMatchObject({ probeFailed: true, statusBucket: 'attention' });
    expect(proxyCall.mock.calls.filter(([p]) => p.method === 'POST')).toHaveLength(1);
  });
  it('uses short TTL and attention for a cached transport error', async () => {
    const proxyCall = vi.fn(() => Promise.reject(new Error('offline')));
    const state = mount(CredentialsTab, proxyCall);
    expect(await state.probeCredential(credential)).toMatchObject({ error: 'offline' });
    const entry = getQuotaCacheEntry(quotaCacheKey(monitoringRow));
    expect(entry.nextRequestAt - entry.fetchedAt).toBe(QUOTA_ERROR_COOLDOWN_MS);
    proxyCall.mockResolvedValue({ items: [credential] });
    const loaded = mount(CredentialsTab, proxyCall, true); await flush();
    expect(loaded.rows[0]).toMatchObject({ probeFailed: true, statusBucket: 'attention' });
    expect(proxyCall.mock.calls.filter(([p]) => p.method === 'POST')).toHaveLength(1);
  });
  it('normalizes cached inventory windows and requests only local usage when opened', async () => {
    const statuses = [null, null, 'ok', 'fail', 'ok', 'ok', 'fail', 'ok'];
    const proxyCall = vi.fn(payload => {
      if (payload.method === 'GET') return Promise.resolve({ items: [{ ...credential, quotaWindows: [window], recentStatuses: statuses, history: { lastSeenMs: now - 2000 } }] });
      if (payload.path.endsWith('/account-window-usage')) return Promise.resolve(usageResponse(payload.body.windows));
      throw new Error('unexpected upstream quota probe');
    });
    const state = mount(CredentialsTab, proxyCall, true); await flush();
    expect(proxyCall).toHaveBeenCalledTimes(1);
    expect(state.rows[0].quotaWindows[0]).toMatchObject({ key: 'five_hour', cycleEndMs: Date.parse(window.resetAt), sampledAtMs: now });
    expect(state.kpi.quotaRisk).toBe(1); expect(state.rows[0].recentStatuses).toEqual(statuses);
    expect(state.historyFromMs).toBe(now - 90 * 24 * 3600_000);
    state.openDrawer(state.rows[0]); await flush();
    expect(proxyCall).toHaveBeenCalledTimes(2);
    expect(state.selectedWindowCards[0].current).toMatchObject({ tokens: 1000, cost: 2 });
    expect(state.rows[0].quotaDisplays[0].usageLine).toContain('1.0K');
  });
  it('refresh inventory reads only unexpired shared quota cache', async () => {
    setQuotaCacheEntry(quotaCacheKey(monitoringRow), { ...probe, error: 'cached failure' });
    const proxyCall = vi.fn(() => Promise.resolve({ items: [credential] }));
    const state = mount(CredentialsTab, proxyCall, true); await flush();
    expect(state.rows[0].statusBucket).toBe('attention');
    vi.advanceTimersByTime(5 * 60_000); await state.loadCredentials();
    expect(state.rows[0].probe).toBeNull(); expect(state.rows[0].statusBucket).toBe('available');
    expect(proxyCall.mock.calls.every(([p]) => p.method === 'GET')).toBe(true);
  });
});

describe('credential async ownership', () => {
  it.each(['resolve', 'reject'])('ignores an old inventory %s while latest is pending', async settle => {
    const old = deferred(), latest = deferred();
    const proxyCall = vi.fn().mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise);
    const state = mount(CredentialsTab, proxyCall, true), run = state.loadCredentials();
    old[settle](settle === 'resolve' ? { items: [credential] } : new Error('old failure')); await flush();
    expect(state.loading).toBe(true); expect(state.error).toBe(''); expect(state.rows).toEqual([]);
    latest.resolve({ items: [{ ...credential, disabled: true }] }); await run;
    expect(state.rows[0].disabled).toBe(true); expect(state.loading).toBe(false);
  });
  it('cannot restore an old enabled row after inventory disables it', async () => {
    const pending = deferred(); let inventory = { ...credential };
    const proxyCall = vi.fn(p => p.method === 'GET' ? Promise.resolve({ items: [inventory] }) : pending.promise);
    const state = mount(CredentialsTab, proxyCall, true); await flush(); state.selectedRowKey = 'a';
    const oldRun = state.refreshSelectedQuota(); await flush();
    inventory = { ...credential, disabled: true }; await state.loadCredentials();
    pending.resolve({ action: 'keep', quotaWindows: [] }); await oldRun;
    expect(state.rows[0]).toMatchObject({ disabled: true, statusBucket: 'disabled' });
    expect(state.kpi.available).toBe(0); expect(state.drawerProbing).toBe(false);
  });
  it('old selection cannot clear latest loading or set its notice', async () => {
    const a = deferred(), b = deferred();
    const proxyCall = vi.fn(p => {
      if (p.method === 'GET') return Promise.resolve({ items: [credential, { ...credential, rowKey: 'b', authIndex: 'b', authId: 'b.json' }] });
      return p.body.authIndex === 'a' ? a.promise : b.promise;
    });
    const state = mount(CredentialsTab, proxyCall, true); await flush();
    state.selectedRowKey = 'a'; const oldRun = state.refreshSelectedQuota();
    state.selectedRowKey = 'b'; const newRun = state.refreshSelectedQuota();
    a.resolve({ error: 'old selection failure' }); await oldRun;
    expect(state.drawerNotice).toBe(''); expect(state.drawerProbing).toBe(true);
    b.resolve({ action: 'keep', quotaWindows: [] }); await newRun;
    expect(state.drawerProbing).toBe(false); expect(state.selectedRow.probeFailed).toBe(false);
  });
  it('updates KPI before usage completes and clears totals on a failed retry', async () => {
    let pendingUsage = deferred();
    const proxyCall = vi.fn(p => {
      if (p.method === 'GET') return Promise.resolve({ items: [credential] });
      if (p.path.endsWith('/account-quota-probe')) return Promise.resolve(probe);
      return pendingUsage.promise;
    });
    const state = mount(CredentialsTab, proxyCall, true); await flush(); expect(state.kpi.available).toBe(1);
    state.selectedRowKey = 'a'; const first = state.refreshSelectedQuota(); await flush();
    expect(state.kpi.quotaRisk).toBe(1);
    const payload = proxyCall.mock.calls.find(([p]) => p.path.endsWith('/account-window-usage'))[0];
    pendingUsage.resolve(usageResponse(payload.body.windows)); await first;
    expect(state.selectedWindowCards[0].current.tokens).toBe(1000);
    pendingUsage = deferred(); const second = state.refreshSelectedQuota(); await flush();
    expect(state.selectedWindowCards[0].current).toBeNull();
    expect(state.rows[0].quotaDisplays[0].usageLine).not.toContain('$2.00');
    pendingUsage.reject(new Error('usage offline')); await second;
    expect(state.selectedWindowCards[0].current).toBeNull(); expect(state.drawerNotice).toContain('usage offline');
  });
  it('does not update a closed drawer or cache unsolicited usage', async () => {
    const pending = deferred();
    const proxyCall = vi.fn(p => {
      if (p.method === 'GET') return Promise.resolve({ items: [credential] });
      if (p.path.endsWith('/account-quota-probe')) return Promise.resolve(probe);
      return pending.promise;
    });
    const state = mount(CredentialsTab, proxyCall, true); await flush(); state.selectedRowKey = 'a';
    const run = state.refreshSelectedQuota(); await flush(); state.closeDrawer();
    pending.resolve({ items: [{ request_key: 'unrequested', total_cost: 999 }] }); await run;
    expect(state.usageByRequestKey.has('unrequested')).toBe(false);
    expect(state.drawerNotice).toBe(''); expect(state.drawerProbing).toBe(false);
  });
  it('rejects unrequested identities and mismatched ranges while open', async () => {
    const proxyCall = vi.fn(p => {
      if (p.method === 'GET') return Promise.resolve({ items: [credential] });
      if (p.path.endsWith('/account-quota-probe')) return Promise.resolve(probe);
      return Promise.resolve({ items: [{ request_key: 'unrequested' }, ...usageResponse(p.body.windows).items.map(item => ({ ...item, from_ms: item.from_ms + 1 }))] });
    });
    const state = mount(CredentialsTab, proxyCall, true); await flush(); state.selectedRowKey = 'a';
    await state.refreshSelectedQuota();
    expect(state.usageByRequestKey.size).toBe(0); expect(state.selectedWindowCards[0].current).toBeNull();
  });
});
