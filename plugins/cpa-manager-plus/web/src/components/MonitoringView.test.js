import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRenderer, nextTick, ssrContextKey } from 'vue';
import { createI18n } from 'vue-i18n';
import MonitoringView from './MonitoringView.vue';
import DashboardView from './DashboardView.vue';
import en from '../i18n/messages/en.js';

// Exercise the real setup/controller without a browser DOM or network calls.
const renderer = createRenderer({
  createComment: () => ({}),
  insert: () => {},
  remove: () => {},
  parentNode: () => null,
  nextSibling: () => null,
});

describe('event model popup interactions', () => {
  let app;
  let state;
  const event = { currentTarget: { getBoundingClientRect: () => ({ left: 100, top: 100, bottom: 120 }) } };
  const row = { id: 'event-1', model: 'billed', mappedModel: 'billed', responseModel: 'upstream', showResponseModel: true };

  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('window', { innerWidth: 1200, innerHeight: 800, setInterval, clearInterval });
    // These raw-detail fixtures contain no HTML entities; emulate textarea decoding.
    vi.stubGlobal('document', { createElement: () => ({
      value: '',
      set innerHTML(value) { this.value = value; },
    }) });
    app = renderer.createApp({ ...MonitoringView, render: () => null }, { ready: false, proxyCall: vi.fn() });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    // Vite loads SFCs through its SSR transform in the Node test environment.
    app.provide(ssrContextKey, {});
    const vm = app.mount({});
    state = vm.$.setupState;
  });

  afterEach(() => {
    app?.unmount();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it('normalizes API response identity without changing the requested or billed model', () => {
    const built = state.buildEventTableRow({ model: 'billed', alias: 'billed', response_model: ' upstream ' }, new Map());
    expect(built).toMatchObject({ model: 'billed', mappedModel: 'billed', responseModel: 'upstream', showResponseModel: true, hasModelDetails: true });
    const same = state.buildEventTableRow({ model: 'billed', response_model: 'billed' }, new Map());
    expect(same).toMatchObject({ showResponseModel: false, hasModelDetails: false });
  });

  it('shows concise tiered pricing only for codex oauth accounts', () => {
    const actual = state.buildEventTableRow({
      provider: 'codex',
      auth_type: 'oauth',
      model: 'gpt-5.6-sol',
      service_tier: 'auto',
      reasoning_effort: 'xhigh',
      reasoning_tokens: 27,
      cost_estimate: {amount: 0.42, currency: 'USD', basis: 'openai_api_equivalent', status: 'estimated', schedule_id: 'openai-api-pricing-2026-09', context_tier: 'short', service_tier: 'fast', tier_source: 'response'},
    }, new Map());
    expect(actual.cost).toBe(0.42);
    expect(actual.costMeta).toBe('Fast · ≤272K');
    expect(actual.costTooltip).toBe('Fast · ≤272K · actual response · request auto');

    const assumed = state.buildEventTableRow({
      provider: 'codex',
      auth_type: 'oauth',
      model: 'gpt-5.6-sol', service_tier: 'auto', reasoning_effort: 'high',
      cost_estimate: {amount: 0.42, status: 'estimated', context_tier: 'short', service_tier: 'standard', tier_source: 'assumed-standard'},
    }, new Map());
    expect(assumed.costMeta).toBe('Standard · ≤272K');
    expect(assumed.costTooltip).toBe('Standard · ≤272K · Standard assumed · request auto');

    const dynamicThreshold = state.buildEventTableRow({
      provider: 'codex',
      auth_type: 'oauth',
      model: 'gpt-6',
      cost_estimate: {amount: 1.0, status: 'estimated', context_tier: 'long', context_threshold_tokens: 500000, service_tier: 'standard'},
    }, new Map());
    expect(dynamicThreshold.costMeta).toBe('Standard · >500K');

    const customFlat = state.buildEventTableRow({
      provider: 'codex',
      auth_type: 'oauth',
      model: 'gpt-6-sol', service_tier: 'auto', response_service_tier: 'fast', reasoning_effort: 'xhigh',
      cost_estimate: {amount: 0.102, status: 'estimated', schedule_id: 'model-price-flat', context_tier: 'flat'},
    }, new Map());
    expect(customFlat.costMeta).toBe('Custom price');
    expect(customFlat.costTooltip).toBe('Custom price · flat price is not adjusted for service tier or context');

    const unpriced = state.buildEventTableRow({
      provider: 'codex',
      auth_type: 'oauth',
      model: 'gpt-5.5',
      cost_estimate: {amount: 0, status: 'unpriced', note: 'long_context_rate_unavailable'},
    }, new Map());
    expect(unpriced.cost).toBeNull();
    expect(unpriced.costText).toBe('—');
    expect(unpriced.costMeta).toBe('No published price');
    expect(unpriced.costTooltip).toContain('Long-context pricing is not published for this Fast model.');

    // Non-Codex OAuth accounts must NOT show costMeta
    const nonCodex = state.buildEventTableRow({
      provider: 'antigravity',
      auth_type: 'oauth',
      model: 'gemini-3.7-flash',
      cost_estimate: {amount: 0.0109, status: 'estimated', schedule_id: 'model-price-flat', context_tier: 'flat'},
    }, new Map());
    expect(nonCodex.cost).toBe(0.0109);
    expect(nonCodex.costMeta).toBe('');
    expect(nonCodex.costTooltip).toBe(nonCodex.hints.cost);
  });

  it('prioritizes upstream response_service_tier over inbound service_tier in tierDisplay', () => {
    const row = state.buildEventTableRow({
      model: 'gpt-6-luna',
      service_tier: 'auto',
      response_service_tier: 'priority',
      reasoning_effort: 'max',
    }, new Map());
    expect(row.intensityDisplay).toBe('max');
    expect(row.tierDisplay).toBe('priority');
    expect(row.tier).toBe('priority');

    const fallback = state.buildEventTableRow({
      model: 'gpt-6-luna',
      service_tier: 'auto',
      reasoning_effort: 'max',
    }, new Map());
    expect(fallback.tierDisplay).toBe('auto');
  });

  it.each([
    ['billed', 'billed', undefined, false, false, false],
    ['alias', 'billed', undefined, false, true, false],
    ['alias', 'billed', '   ', false, true, false],
    ['billed', 'billed', 'billed', false, false, false],
    ['alias', 'billed', ' billed ', false, true, false],
    ['billed', 'billed', 'upstream', true, true, true],
    ['alias', 'billed', 'upstream', true, true, true],
    ['alias', 'billed', 'alias', true, true, false],
    ['billed', 'billed', 'billed-2026-01-01', true, true, false],
    ['billed', 'billed', 'BILLED', true, true, false],
    ['grok-4.7', 'grok-4.7', 'grok-4.7-build', true, true, false],
    ['gemini-3.7-flash', 'gemini-3.7-flash-high', 'gemini-3.7-flash', true, true, false],
    ['gemini-3.8-flash', 'gemini-3.8-flash-high', 'gemini-3.8-flash', true, true, false],
  ])('keeps allowed response variants non-red: requested=%s billed=%s response=%s', (requested, billed, response, highlight, popup, mismatch) => {
    for (const failed of [false, true]) {
      const built = state.buildEventTableRow({ alias: requested, model: billed, response_model: response, failed }, new Map());
      // The popup keeps every response difference visible, while red is reserved for unrelated models.
      expect(built.showResponseModel).toBe(highlight);
      expect(built.responseModelMismatch).toBe(mismatch);
      expect(built.hasModelDetails).toBe(popup);
      expect(built.model).toBe(requested);
      expect(built.mappedModel).toBe(billed);
    }
  });

  it.each([
    // source, chosen, host, observed, conflict, response row/red, popup
    ['', '', '', '', false, false, false],
    ['', 'upstream', '', '', false, true, true], // old response-only event
    ['host', 'upstream', 'upstream', '', false, true, true],
    ['observer', 'upstream', '', 'upstream', false, true, true],
    ['confirmed', 'upstream', 'upstream', 'upstream', false, true, true],
    ['confirmed', 'billed', 'billed', 'billed', false, false, false],
    ['host', 'billed', 'billed', 'observed-other', true, false, true],
    ['host', 'upstream', 'upstream', 'observed-other', true, true, true],
    ['host', 'billed', 'billed', '', true, false, true], // ambiguous observer
    ['', '', '', '', true, false, true], // ambiguity with no chosen identity
  ])('dual-source popup: source=%s chosen=%s host=%s observed=%s conflict=%s', (source, chosen, host, observed, conflict, red, popup) => {
    const raw = { model: 'billed', response_model: chosen, host_response_model: host, observed_response_model: observed, response_model_source: source, response_model_conflict: conflict };
    const built = state.buildEventTableRow(raw, new Map());
    expect(built).toMatchObject({ model: 'billed', mappedModel: 'billed', responseModel: chosen, hostResponseModel: host, observedResponseModel: observed, responseModelSource: source, responseModelConflict: conflict, showResponseModel: red, hasModelDetails: popup });
    if (popup) {
      state.toggleModelRouteTooltip(event, built);
      expect(state.modelRouteTooltip.visible).toBe(true);
      expect(state.modelRouteTooltip.row).toMatchObject({ responseModelConflict: conflict, observedResponseModel: observed, showResponseModel: red });
    }
    state.selectedEvent = raw;
    expect(state.eventBaseDetail).toMatchObject(raw);
  });

  async function updateEvents(items) {
    state.data = { events: { items } };
    await nextTick();
  }

  it('rebinds an open popup through async observer confirmation and conflict transitions', async () => {
    const raw = { event_hash: 'stable', model: 'billed', response_model: 'upstream', response_model_source: 'host' };
    await updateEvents([raw]);
    state.showModelRouteTooltip(event, state.pagedEvents[0]);
    const oldRow = state.modelRouteTooltip.row;
    await updateEvents([{ ...raw, observed_response_model: 'upstream', response_model_source: 'confirmed' }]);
    expect(state.modelRouteTooltip.row).not.toBe(oldRow);
    expect(state.modelRouteTooltip.row).toEqual(state.pagedEvents[0]);
    expect(state.modelRouteTooltip.row.responseModelSource).toBe('confirmed');
    await updateEvents([{ ...raw, response_model: 'billed', observed_response_model: 'other', response_model_conflict: true }]);
    expect(state.modelRouteTooltip.visible).toBe(true);
    expect(state.modelRouteTooltip.row).toMatchObject({ responseModel: 'billed', observedResponseModel: 'other', responseModelConflict: true, showResponseModel: false });
    await updateEvents([{ ...raw, response_model: '', response_model_source: '', response_model_conflict: true }]);
    expect(state.modelRouteTooltip.visible).toBe(true);
    expect(state.modelRouteTooltip.row).toMatchObject({ responseModel: '', observedResponseModel: '', responseModelConflict: true, showResponseModel: false });
    await updateEvents([{ ...raw, response_model: 'billed', response_model_conflict: false }]);
    expect(state.modelRouteTooltip.visible).toBe(false);
  });

  it('preserves pending hide timers across row rebinding', async () => {
    const raw = { event_hash: 'stable', model: 'billed', response_model: 'upstream', failed: true };
    await updateEvents([raw]);
    state.showModelRouteTooltip(event, state.pagedEvents[0]);
    state.hideModelRouteTooltip();
    await updateEvents([{ ...raw, response_model_source: 'confirmed' }]);
    expect(state.modelRouteTooltip.visible).toBe(true);
    vi.advanceTimersByTime(200);
    expect(state.modelRouteTooltip.visible).toBe(false);
    state.showFailureTooltip(event, state.pagedEvents[0]);
    state.hideFailureTooltip();
    await updateEvents([{ ...raw, fail_summary: 'Updated failure' }]);
    expect(state.failureTooltip.row.failSummary).toBe('Updated failure');
    vi.advanceTimersByTime(200);
    expect(state.failureTooltip.visible).toBe(false);
  });

  it.each(['model', 'failure'])('closes %s popup when its event disappears', async (kind) => {
    await updateEvents([{ event_hash: 'stable', model: 'billed', response_model_conflict: true, failed: true }]);
    if (kind === 'model') state.showModelRouteTooltip(event, state.pagedEvents[0]);
    else state.showFailureTooltip(event, state.pagedEvents[0]);
    await updateEvents([]);
    expect(state.modelRouteTooltip.visible).toBe(false);
    expect(state.failureTooltip.visible).toBe(false);
  });

  it.each(['activeDataTab', 'eventPage', 'eventPageSize'])('closes both popup types when %s changes', async (key) => {
    for (const kind of ['model', 'failure']) {
      await updateEvents([{ event_hash: 'stable', model: 'billed', response_model_conflict: true, failed: true }]);
      const current = state.eventTableRows[0];
      if (kind === 'model') state.showModelRouteTooltip(event, current);
      else state.showFailureTooltip(event, current);
      state[key] = key === 'activeDataTab' ? (state[key] === 'events' ? 'accounts' : 'models') : state[key] + 1;
      await nextTick();
      expect(state.modelRouteTooltip.visible).toBe(false);
      expect(state.failureTooltip.visible).toBe(false);
    }
  });

  it('keeps model and failure popups mutually exclusive, including pending hide timers', () => {
    state.showFailureTooltip(event, row);
    state.hideFailureTooltip();
    state.showModelRouteTooltip(event, row);
    expect(state.failureTooltip.visible).toBe(false);
    expect(state.modelRouteTooltip.visible).toBe(true);
    state.hideModelRouteTooltip();
    state.showFailureTooltip(event, row);
    expect(state.modelRouteTooltip.visible).toBe(false);
    expect(state.failureTooltip.visible).toBe(true);
    vi.advanceTimersByTime(200);
    expect(state.failureTooltip.visible).toBe(true);
  });

  it('allows crossing into the popup and toggling the same row closed', () => {
    state.toggleModelRouteTooltip(event, row);
    expect(state.modelRouteTooltip.visible).toBe(true);
    state.hideModelRouteTooltip();
    state.keepModelRouteTooltip();
    vi.advanceTimersByTime(200);
    expect(state.modelRouteTooltip.visible).toBe(true);
    state.toggleModelRouteTooltip(event, row);
    expect(state.modelRouteTooltip.visible).toBe(false);
  });

  it('replaces response details when opening another row', () => {
    state.showModelRouteTooltip(event, row);
    const mappedOnly = { id: 'event-2', model: 'alias', mappedModel: 'billed', showResponseModel: false };
    state.showModelRouteTooltip(event, mappedOnly);
    expect(state.modelRouteTooltip.row).toEqual(mappedOnly);
    expect(state.modelRouteTooltip.row.showResponseModel).toBe(false);
  });
});


describe('latest refresh snapshots', () => {
  let app;
  beforeEach(() => {
    vi.useFakeTimers();
    vi.stubGlobal('window', { innerWidth: 1200, innerHeight: 800, setInterval, clearInterval });
  });
  afterEach(() => { app?.unmount(); vi.unstubAllGlobals(); vi.useRealTimers(); });

  const cases = [
    ['monitoring', MonitoringView, 'refresh', 'data', '/v0/management/monitoring/analytics'],
    ['dashboard analytics', DashboardView, 'refreshAnalytics', 'analyticsData', '/v0/management/monitoring/analytics'],
    ['dashboard summary', DashboardView, 'refreshDashboard', 'dashData', '/v0/management/dashboard/summary'],
  ];

  it.each(cases)('%s ignores an older response arriving last', async (_name, component, refresh, dataKey, path) => {
    const pending = [];
    const proxyCall = vi.fn(payload => new Promise((resolve, reject) => pending.push({ payload, resolve, reject })));
    app = renderer.createApp({ ...component, render: () => null }, { ready: true, proxyCall });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;
    const old = pending.find(request => request.payload.path === path);
    if (refresh === 'refresh') {
      state.filters.model = 'new-model';
      await nextTick();
    } else {
      state[refresh](true);
    }
    const latest = pending.filter(request => request.payload.path === path).at(-1);
    expect(latest).not.toBe(old);
    latest.resolve({ marker: 'latest', events: { items: [] } });
    await nextTick();
    expect(state[dataKey].marker).toBe('latest');
    old.resolve({ marker: 'stale', events: { items: [] } });
    await nextTick();
    expect(state[dataKey].marker).toBe('latest');
  });

  it('a stale monitoring error cannot end the latest loading state', async () => {
    const pending = [];
    const proxyCall = vi.fn(() => new Promise((resolve, reject) => pending.push({ resolve, reject })));
    app = renderer.createApp({ ...MonitoringView, render: () => null }, { ready: true, proxyCall });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;
    const latestRun = state.refresh(true);
    pending[0].reject(new Error('old filter failed'));
    await nextTick();
    expect(state.error).toBe('');
    expect(state.loading).toBe(true);
    pending[1].resolve({ marker: 'latest', events: { items: [] } });
    await latestRun;
    expect(state.loading).toBe(false);
  });

  it('retains full time-window filter options when filtered to a single sub-item', async () => {
    const proxyCall = vi.fn().mockResolvedValue({
      summary: { total_calls: 1, cached_tokens: 500, cache_read_tokens: 500, cache_tokens: 500, total_tokens: 600 },
      events: { items: [{ id: 1, event_hash: 'h1', provider: 'gemini', model: 'gemini-2.5-pro' }] },
      model_stats: [{ model: 'gemini-2.5-pro', calls: 1 }],
      account_stats: [{ account_snapshot: 'idx-gem', calls: 1 }],
      api_key_stats: [{ api_key_hash: 'hash-gem-12345678', calls: 1 }],
      filter_options: {
        providers: ['antigravity', 'codex', 'gemini'],
        model_stats: [{ model: 'gemini-2.5-pro' }, { model: 'gpt-5' }],
        accounts: [
          { value: 'idx-gem', account_snapshot: 'idx-gem', source: 'gem@example.com', auth_type: 'oauth' },
          { value: 'idx-codex', account_snapshot: 'idx-codex', source: 'codex@example.com', auth_type: 'oauth' },
        ],
        api_keys: [
          { value: 'hash-gem-12345678', api_key_hash: 'hash-gem-12345678', source: 'gem@example.com', auth_type: 'oauth' },
          { value: 'hash-codex-87654321', api_key_hash: 'hash-codex-87654321', account_snapshot: 'unknown' },
        ],
      },
    });
    app = renderer.createApp({ ...MonitoringView, render: () => null }, { ready: true, proxyCall });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;
    await nextTick();
    await nextTick();

    state.filters.provider = 'gemini';
    await nextTick();
    expect(state.optionProviders).toEqual(['antigravity', 'codex', 'gemini']);
    expect(state.optionModels).toEqual(['gemini-2.5-pro', 'gpt-5']);
    expect(state.optionAccounts).toEqual([
      { value: 'idx-gem', label: 'g••••m@example.com · idx-gem' },
      { value: 'idx-codex', label: 'c••••x@example.com · idx-codex' },
    ]);
    expect(state.optionApiKeys).toEqual([
      { value: 'hash-gem-12345678', label: 'hash-ge…45678 · g••••m@example.com' },
      { value: 'hash-codex-87654321', label: 'hash-co…54321' },
    ]);
    expect(state.summaryCards[7].value).toBe('500');
  });

  it('does not fabricate priority tier from reasoning_effort or fallback ttftMs to latencyMs', () => {
    const proxyCall = vi.fn();
    app = renderer.createApp({ ...MonitoringView, render: () => null }, { ready: false, proxyCall });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;

    const reasoningOnly = state.buildEventTableRow({
      model: 'gpt-5',
      reasoning_effort: 'high',
      latency_ms: 1200,
      ttft_ms: 0,
    }, new Map());
    expect(reasoningOnly.intensityDisplay).toBe('high');
    expect(reasoningOnly.tierDisplay).toBe('—');
    expect(reasoningOnly.tier).toBe('default');
    expect(reasoningOnly.ttftMs).toBeNull();

    const tierOnly = state.buildEventTableRow({
      model: 'gpt-5',
      service_tier: 'priority',
      latency_ms: 800,
    }, new Map());
    expect(tierOnly.intensityDisplay).toBe('—');
    expect(tierOnly.intensity).toBe('-');
    expect(tierOnly.tierDisplay).toBe('priority');
    expect(tierOnly.tier).toBe('priority');
  });

  it('applies external filter payload without duplicate refresh calls', async () => {
    const proxyCall = vi.fn().mockResolvedValue({ events: { items: [] } });
    app = renderer.createApp({ ...MonitoringView, render: () => null }, { ready: true, proxyCall });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;
    await nextTick();
    proxyCall.mockClear();

    state.activeDataTab = 'models';
    state.setModelFilter({ model: 'gpt-5' });
    await nextTick();
    expect(state.activeDataTab).toBe('events');
    expect(state.filters.model).toBe('gpt-5');
    expect(proxyCall).toHaveBeenCalledTimes(1);

    proxyCall.mockClear();
    state.consumePendingMonitoringFilter({ authFile: 'codex-a.json' });
    await nextTick();
    expect(state.filters.account).toBe('codex-a.json');
    expect(state.filters.model).toBe('all');
    expect(proxyCall).toHaveBeenCalledTimes(1);
  });

  it('collapses email sources by default and auto-collapses again after fixed timeout when expanded', () => {
    vi.useFakeTimers();
    app = renderer.createApp({ ...MonitoringView, render: () => null }, { ready: false, proxyCall: vi.fn() });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;

    const built = state.buildEventTableRow({
      event_hash: 'ev-email-1',
      source: 'zhangsan@gmail.com',
      auth_type: 'oauth',
      model: 'gpt-5',
    }, new Map());
    expect(built.sourceIsApiKey).toBe(true);
    expect(state.isEventKeyExpanded(built, 'source')).toBe(false);
    expect(state.eventApiKeyDisplay(built.sourceName, state.isEventKeyExpanded(built, 'source'))).toBe('zh••••an@gmail.com');

    state.toggleEventKey(built, 'source');
    expect(state.isEventKeyExpanded(built, 'source')).toBe(true);
    expect(state.eventApiKeyDisplay(built.sourceName, state.isEventKeyExpanded(built, 'source'))).toBe('zhangsan@gmail.com');

    vi.advanceTimersByTime(10000);
    expect(state.isEventKeyExpanded(built, 'source')).toBe(false);
    expect(state.eventApiKeyDisplay(built.sourceName, state.isEventKeyExpanded(built, 'source'))).toBe('zh••••an@gmail.com');
  });

  it('merges dashboard summary and usage analytics into unified KPIs, vertical timeline, and rank cards', async () => {
    const proxyCall = vi.fn().mockImplementation(({ path }) => {
      if (path === '/v0/management/dashboard/summary') {
        return Promise.resolve({
          today: { total_calls: 120, total_cost: 8.5, zero_token_calls: 1, average_latency_ms: 950 },
          rolling_30m: { rpm: 14.2, tpm: 48000, total_calls: 426 },
          channel_health: [{ channel: 'codex', success_rate: 0.995, calls: 100 }],
          recent_failures: [{ event_hash: 'f1', fail_status_code: 429, model: 'gpt-5', fail_summary: 'Rate limit', timestamp_ms: 1700000000000 }],
        });
      }
      return Promise.resolve({
        granularity: 'hour',
        summary: {
          total_calls: 500,
          success_calls: 495,
          failure_calls: 5,
          success_rate: 0.99,
          total_cost: 35.2,
          average_cost_per_call: 0.0704,
          total_tokens: 100000,
          input_tokens: 30000,
          output_tokens: 10000,
          cache_tokens: 60000,
        },
        timeline: [
          { bucket_ms: 1700003600000, label: '11-15 09:00', calls: 80, tokens: 20000, cost: 5.2 },
          { bucket_ms: 1700000000000, label: '11-15 08:00', calls: 40, tokens: 10000, cost: 2.6 },
        ],
        model_stats: [
          { model: 'gpt-5', provider: 'codex', calls: 300, total_tokens: 70000, cost: 25.0, success_rate: 0.99 },
          { model: 'claude-sonnet-4-6', provider: 'claude', calls: 200, total_tokens: 30000, cost: 10.2, success_rate: 0.99 },
        ],
        credential_stats: [
          { id: 'codex-1.json', auth_file: 'codex-1.json', provider: 'codex', calls: 300, total_tokens: 70000, cost: 25.0, success_rate: 0.99 },
        ],
        api_key_stats: [
          { api_key_hash: 'sk-cpa-1234567890abcdef', provider: 'codex', calls: 500, total_tokens: 100000, cost: 35.2, success_rate: 0.99 },
        ],
      });
    });

    app = renderer.createApp({ ...DashboardView, render: () => null }, { ready: true, proxyCall });
    app.use(createI18n({ legacy: false, locale: 'en', messages: { en } }));
    app.provide(ssrContextKey, {});
    const state = app.mount({}).$.setupState;
    await nextTick();
    await nextTick();

    expect(state.aSummary.total_calls).toBe(500);
    expect(state.dSummary.total_calls).toBe(120);
    expect(state.rolling.rpm).toBe(14.2);
    expect(state.unifiedTokenMix.map(s => s.key)).toEqual(['in', 'cache', 'out']);
    expect(state.chartTimelineRows.map(r => r.calls)).toEqual([40, 80]);
    expect(state.topModels[0].model).toBe('gpt-5');
    expect(state.topCredentials[0].auth_file).toBe('codex-1.json');
    expect(state.topApiKeys[0].calls).toBe(500);

    state.filters.provider = 'codex';
    state.filters.status = 'failed';
    expect(state.activeFilterCount).toBe(2);
    state.resetAnalyticsFilters();
    expect(state.activeFilterCount).toBe(0);
  });
});
