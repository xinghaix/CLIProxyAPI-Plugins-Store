import { describe, expect, it } from 'vitest';
import {
  buildEventHints,
  buildModelMeta,
  formatCacheSub,
  formatCallsSub,
  formatTpsSub,
  hasModelMapping,
  hasModelRouteDetails,
  hasResponseModelDifference,
  hasResponseModelConflict,
  responseModelSource,
  responseModelName,
  mappedModelName,
  recentPatternSummary,
  requestedModelName,
} from './eventStreamDisplay.js';

function t(key, params = {}) {
  const dict = {
    'monitoring.eventMeta.intensity': '强度 {value}',
    'monitoring.eventMeta.mapped': '映射后 {value}',
    'monitoring.eventMeta.calls': '{value} 次',
    'monitoring.eventMeta.tps': 'TPS {value}',
    'monitoring.eventMeta.cache': '缓存 {value}',
    'monitoring.eventMeta.none': '—',
    'monitoring.eventMeta.responseModel': '上游响应模型',
    'monitoring.labels.level': '等级: {value}',
    'monitoring.labels.failed': '失败',
    'monitoring.labels.success': '成功',
    'monitoring.eventHints.model': '模型 {model} · 强度 {intensity} · 等级 {tier}',
    'monitoring.eventHints.modelMapped': '请求 {model} · 映射后 {mapped} · 强度 {intensity} · 等级 {tier}',
    'monitoring.eventHints.status': '{status} · {protocol} · 最近 {ok}/{total} 成功',
    'monitoring.eventHints.health': '成功率 {rate} · 总调用 {calls}',
    'monitoring.eventHints.speed': '首字 {ttft} · 耗时 {latency} · TPS {tps}',
    'monitoring.eventHints.usage': '总量 {total} · {breakdown}',
    'monitoring.eventHints.cost': '花费 {cost} · 缓存命中率 {cache}',
  };
  return (dict[key] || key).replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? ''));
}

describe('dual-source response metadata', () => {
  it.each(['', 'host', 'observer', 'confirmed'])('keeps source %s independent of difference and conflict', (source) => {
    for (const response of ['', 'billed', 'upstream']) {
      for (const conflict of [false, true]) {
        const raw = { model: 'billed', response_model: response, response_model_source: source, response_model_conflict: conflict };
        const normalized = { model: 'billed', responseModel: response, responseModelSource: source, responseModelConflict: conflict };
        for (const row of [raw, normalized]) {
          expect(responseModelSource(row)).toBe(source);
          expect(hasResponseModelConflict(row)).toBe(conflict);
          expect(hasResponseModelDifference(row)).toBe(response === 'upstream');
          expect(hasModelRouteDetails(row)).toBe(conflict || response === 'upstream');
          expect(mappedModelName(row)).toBe('billed');
        }
      }
    }
  });

  it('does not infer missing legacy metadata or pick a winner from source fields', () => {
    expect(responseModelSource({})).toBe('');
    expect(responseModelSource({ response_model_source: 'future-source' })).toBe('');
    expect(hasResponseModelConflict({})).toBe(false);
    expect(responseModelName({ host_response_model: 'host', observed_response_model: 'observer' })).toBe('');
    expect(hasModelRouteDetails({ model: 'billed' })).toBe(false);
  });
});

describe('event stream response model visibility', () => {
  it.each([
    // requested, billed, response, popup, response row
    ['same', 'same', undefined, false, false],
    ['same', 'same', '', false, false],
    ['same', 'same', '   ', false, false],
    ['same', 'same', 'same', false, false],
    ['alias', 'billed', undefined, true, false],
    ['alias', 'billed', 'billed', true, false],
    ['alias', 'billed', ' billed ', true, false],
    ['same', 'same', 'upstream', true, true],
    ['alias', 'billed', 'upstream', true, true],
    ['alias', 'billed', 'alias', true, true],
  ])('requested=%s billed=%s response=%s', (requested, billed, response, popup, visible) => {
    const raw = { alias: requested, model: billed, response_model: response };
    const normalized = { model: requested, mappedModel: billed, responseModel: response };
    for (const row of [raw, normalized]) {
      expect(hasResponseModelDifference(row)).toBe(visible);
      expect(hasModelRouteDetails(row)).toBe(popup);
      expect(buildEventHints(row, t).model.includes('上游响应模型')).toBe(visible);
      expect(mappedModelName(row)).toBe(billed);
      expect(requestedModelName(row)).toBe(requested);
    }
  });

  it('does not invent a response identity or compare against an unknown billed model', () => {
    expect(responseModelName({ model: 'billed', alias: 'requested' })).toBe('');
    expect(responseModelName({ response_model: ' upstream ' })).toBe('upstream');
    expect(hasResponseModelDifference({ response_model: 'upstream' })).toBe(false);
    expect(hasModelRouteDetails({})).toBe(false);
  });

  it('compares with resolved billing model rather than requested model', () => {
    const row = { requested_model: 'alias', model: 'alias', resolved_model: 'billed', response_model: 'billed' };
    expect(hasModelRouteDetails(row)).toBe(true);
    expect(hasResponseModelDifference(row)).toBe(false);
    row.response_model = 'upstream';
    expect(buildEventHints(row, t).model).toContain('上游响应模型: upstream');
  });
});

describe('event stream merged labels', () => {
  it('uses the requested alias when it differs from the mapped model', () => {
    expect(requestedModelName({ model: 'gpt-5', alias: 'g5' })).toBe('g5');
    expect(mappedModelName({ model: 'gpt-5', alias: 'g5' })).toBe('gpt-5');
    expect(requestedModelName({ model: 'gpt-5' })).toBe('gpt-5');
    expect(hasModelMapping({ model: 'gpt-5', alias: 'g5' })).toBe(true);
    expect(hasModelMapping({ model: 'gpt-5', alias: 'gpt-5' })).toBe(false);
    expect(hasModelMapping({ model: 'gpt-5' })).toBe(false);
  });

  it('keeps intensity and tier visible under the model mapping', () => {
    expect(buildModelMeta({ intensity: 'xhigh', tier: 'priority', model: 'gpt-5.4' }, t)).toBe('强度 xhigh · 等级: priority');
  });

  it('labels calls, tps and cache so merged cells stay readable', () => {
    expect(formatCallsSub('128', t)).toBe('128 次');
    expect(formatTpsSub('12.3', t)).toBe('TPS 12.3');
    expect(formatCacheSub('19.1%', t)).toBe('缓存 19.1%');
  });

  it('summarizes recent status for the tooltip', () => {
    expect(recentPatternSummary([true, true, false, true, true])).toEqual({ ok: 4, total: 5 });
  });

  it('builds hover hints that name every original field', () => {
    const hints = buildEventHints({
      model: 'gpt-5.4',
      intensity: 'xhigh',
      tier: 'priority',
      failed: false,
      protocolLabel: 'HTTP',
      recentPattern: [true, false],
      successRateText: '96.4%',
      totalCallsText: '128',
      ttftText: '0.42 s',
      latencyText: '1.80 s',
      tpsText: '12.3',
      totalTokensText: '18.4K',
      usageText: 'I 16.2K · O 1.9K · C 3.1K',
      costText: '$0.0860',
      cacheText: '19.1%',
    }, t);
    expect(hints.status).toContain('HTTP');
    expect(hints.status).toContain('1/2');
    expect(hints.health).toContain('128');
    expect(hints.speed).toContain('TPS');
    expect(hints.usage).toContain('I 16.2K');
    expect(hints.cost).toContain('缓存命中率');
  });

  it('names both requested and mapped models in the tooltip', () => {
    const hints = buildEventHints({
      model: 'g5',
      mappedModel: 'gpt-5',
      intensity: 'xhigh',
      tier: 'priority',
      failed: false,
      protocolLabel: 'HTTP',
      recentPattern: [],
      successRateText: '100%',
      totalCallsText: '1',
      ttftText: '0.1 s',
      latencyText: '1 s',
      tpsText: '1',
      totalTokensText: '10',
      usageText: 'I 8 · O 2 · C 0',
      costText: '$0.01',
      cacheText: '0%',
    }, t);
    expect(hints.model).toContain('请求 g5');
    expect(hints.model).toContain('映射后 gpt-5');
  });
});
