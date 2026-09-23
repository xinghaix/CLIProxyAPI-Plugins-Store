import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createRenderer, nextTick, ssrContextKey } from 'vue';
import { createI18n } from 'vue-i18n';
import MonitoringView from './MonitoringView.vue';
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

  it('shows requested tier, priced tier, and reasoning-token price basis', () => {
    const actual = state.buildEventTableRow({
      model: 'gpt-5.6-sol',
      service_tier: 'auto',
      reasoning_effort: 'xhigh',
      reasoning_tokens: 27,
      cost_estimate: {amount: 0.42, currency: 'USD', basis: 'openai_api_equivalent', status: 'estimated', schedule_id: 'openai-api-pricing-2026-09', context_tier: 'short', service_tier: 'fast', tier_source: 'response'},
    }, new Map());
    expect(actual.cost).toBe(0.42);
    expect(actual.costMeta).toBe('Fast · Short context · ≤272K input · actual response · request auto');
    expect(actual.reasoningMeta).toBe('requested effort xhigh · 27 reasoning tokens · reasoning tokens use output-token pricing; effort has no separate rate');
    expect(actual.costTooltip).toContain('not a ChatGPT subscription charge or remaining quota.');
    expect(actual.costTooltip).toContain('reasoning tokens use output-token pricing');

    const assumed = state.buildEventTableRow({
      model: 'gpt-5.6-sol', service_tier: 'auto', reasoning_effort: 'high',
      cost_estimate: {amount: 0.42, status: 'estimated', context_tier: 'short', service_tier: 'standard', tier_source: 'assumed-standard'},
    }, new Map());
    expect(assumed.costMeta).toBe('Standard · Short context · ≤272K input · Standard assumed · request auto');
    expect(assumed.reasoningMeta).toContain('effort has no separate rate');

    const customFlat = state.buildEventTableRow({
      model: 'gpt-6-sol', service_tier: 'auto', response_service_tier: 'fast', reasoning_effort: 'xhigh',
      cost_estimate: {amount: 0.102, status: 'estimated', schedule_id: 'model-price-flat', context_tier: 'flat'},
    }, new Map());
    expect(customFlat.costMeta).toBe('configured model price · actual response tier fast · request auto · flat price is not adjusted for service tier or context');
    expect(customFlat.reasoningMeta).toContain('requested effort xhigh');

    const unpriced = state.buildEventTableRow({model: 'gpt-5.5', cost_estimate: {amount: 0, status: 'unpriced', note: 'long_context_rate_unavailable'}}, new Map());
    expect(unpriced.cost).toBeNull();
    expect(unpriced.costText).toBe('—');
    expect(unpriced.costMeta).toBe('No published rate estimate');
    expect(unpriced.costTooltip).toContain('Long-context pricing is not published for this Fast model.');
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
