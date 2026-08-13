import { describe, expect, it } from 'vitest';
import {
  buildEventHints,
  buildModelMeta,
  formatCacheSub,
  formatCallsSub,
  formatTpsSub,
  recentPatternSummary,
} from './eventStreamDisplay.js';

function t(key, params = {}) {
  const dict = {
    'monitoring.eventMeta.intensity': '强度 {value}',
    'monitoring.eventMeta.calls': '{value} 次',
    'monitoring.eventMeta.tps': 'TPS {value}',
    'monitoring.eventMeta.cache': '缓存 {value}',
    'monitoring.eventMeta.none': '—',
    'monitoring.labels.level': '等级: {value}',
    'monitoring.labels.failed': '失败',
    'monitoring.labels.success': '成功',
    'monitoring.eventHints.model': '模型 {model} · 强度 {intensity} · 等级 {tier}',
    'monitoring.eventHints.status': '{status} · {protocol} · 最近 {ok}/{total} 成功',
    'monitoring.eventHints.health': '成功率 {rate} · 总调用 {calls}',
    'monitoring.eventHints.speed': '首字 {ttft} · 耗时 {latency} · TPS {tps}',
    'monitoring.eventHints.usage': '总量 {total} · {breakdown}',
    'monitoring.eventHints.cost': '花费 {cost} · 缓存命中率 {cache}',
  };
  return (dict[key] || key).replace(/\{(\w+)\}/g, (_, name) => String(params[name] ?? ''));
}

describe('event stream merged labels', () => {
  it('keeps intensity and tier visible on the model second line', () => {
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
});
