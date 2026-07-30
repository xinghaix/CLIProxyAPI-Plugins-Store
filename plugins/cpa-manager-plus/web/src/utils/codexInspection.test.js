import { describe, expect, it } from 'vitest';
import { setI18nLocale } from '../i18n/index.js';
import {
  createConfigFromDraft,
  formatActionLabel,
  formatActionStatusLabel,
  formatDuration,
  formatSchedule,
  formatTimestamp,
  formatTrigger,
  getCanonicalActionIds,
  getInspectionResultsEmptyText,
  getRunStatusLabel,
  isAcknowledgeableResult,
  isActionableResult,
  isNeedsHandling,
  normalizeActionStatus,
  normalizeInspectionTargetTypes,
  resolveServerCodexConfig,
  toDraft,
  validateInspectionConfigFields,
} from './codexInspection.js';

describe('multi-provider inspection settings', () => {
  it('upgrades legacy targetType to an ordered provider list', () => {
    expect(resolveServerCodexConfig({ targetType: 'xai' }).targetTypes).toEqual(['xai']);
    expect(normalizeInspectionTargetTypes('xai+codex')).toEqual(['codex', 'xai']);
  });

  it('round-trips Codex + xAI settings including optional inference', () => {
    const draft = toDraft({ targetTypes: ['codex', 'xai'], xaiInferenceEnabled: true });
    const config = createConfigFromDraft(draft);

    expect(config.targetTypes).toEqual(['codex', 'xai']);
    expect(config.targetType).toBe('codex');
    expect(config.xaiInferenceEnabled).toBe(true);
    expect(config.xaiInferenceModel).toBe('grok-4.5');
  });
});

describe('inspection result empty states', () => {
  it('explains that no credential was found in English by default', () => {
    setI18nLocale('en');
    expect(getInspectionResultsEmptyText({ status: 'completed', totalFiles: 0 }, 0, 0))
      .toBe('No inspectable credentials found; no accounts were probed this run.');
  });

  it('explains provider filtering before sampling in zh-CN', () => {
    setI18nLocale('zh-CN');
    expect(getInspectionResultsEmptyText({ status: 'completed', totalFiles: 3, probeSetCount: 0 }, 0, 0))
      .toBe('主机发现 3 个凭据文件，但目标类型过滤后没有可探测账号。请检查巡检提供商。');
  });

  it('keeps the filter-specific empty state when raw results exist', () => {
    setI18nLocale('en');
    expect(getInspectionResultsEmptyText({ status: 'completed', totalFiles: 1, probeSetCount: 1, sampledCount: 1 }, 1, 0))
      .toBe('No results match the current filters.');
  });
});

describe('manual inspection acknowledgements', () => {
  it('only allows pending manual review and reauthentication results', () => {
    expect(isAcknowledgeableResult({ id: 1, action: 'review', actionStatus: 'pending' })).toBe(true);
    expect(isAcknowledgeableResult({ id: 2, action: 'reauth', actionStatus: 'failed' })).toBe(true);
    expect(isAcknowledgeableResult({ id: 3, action: 'review', actionStatus: 'acknowledged' })).toBe(false);
    expect(isAcknowledgeableResult({ id: 4, action: 'disable', actionStatus: 'pending' })).toBe(false);
  });

  it('keeps manual acknowledgement out of CPA action execution', () => {
    const review = { id: 1, fileName: 'review.json', action: 'review', actionStatus: 'pending' };
    const disable = { id: 2, fileName: 'disable.json', action: 'disable', actionStatus: 'pending' };
    expect(isActionableResult(review)).toBe(false);
    expect(getCanonicalActionIds([review, disable])).toEqual(new Set([2]));
  });

  it('labels and filters acknowledged results as handled', () => {
    setI18nLocale('zh-CN');
    const acknowledged = { id: 1, action: 'review', actionStatus: 'acknowledged', executedAction: 'acknowledge' };
    expect(normalizeActionStatus(acknowledged)).toBe('acknowledged');
    expect(formatActionStatusLabel(acknowledged)).toBe('已标记处理');
    expect(isNeedsHandling(acknowledged)).toBe(false);
  });
});

describe('inspection labels and schedule formatting', () => {
  it('localizes action, status, trigger, and duration labels', () => {
    setI18nLocale('en');
    expect(formatActionLabel('reauth')).toBe('Re-login');
    expect(getRunStatusLabel('running')).toBe('Running');
    expect(formatTrigger({ triggerType: 'scheduled' })).toBe('Scheduled');
    expect(formatDuration({ startedAtMs: 1000, finishedAtMs: 4000 })).toBe('3 s');

    setI18nLocale('ru');
    expect(formatActionLabel('delete')).toBe('Удалить');
    expect(getRunStatusLabel('completed')).toBe('Завершено');
  });

  it('formats schedules with locale strings and keeps timezone raw', () => {
    setI18nLocale('zh-CN');
    expect(formatSchedule({ schedule: { mode: 'interval', intervalMinutes: 30 } })).toBe('每 30 分钟');
    expect(
      formatSchedule({
        schedule: { mode: 'time_points', timePoints: ['09:00', '22:00'], timeZone: 'Asia/Shanghai' },
      }),
    ).toBe('每日 09:00, 22:00 (Asia/Shanghai)');
  });

  it('uses localeFormat for timestamps', () => {
    setI18nLocale('en');
    const text = formatTimestamp(Date.UTC(2026, 0, 2, 3, 4, 5), 'en');
    expect(text).not.toBe('—');
    expect(formatTimestamp(0)).toBe('—');
  });

  it('localizes validation errors', () => {
    setI18nLocale('en');
    const errors = validateInspectionConfigFields({
      targetTypes: '',
      workers: '0',
      deleteWorkers: '1',
      timeout: '1',
      retries: '0',
      sampleSize: '0',
      usedPercentThreshold: '10',
      scheduleMode: 'interval',
      intervalMinutes: '1',
    });
    expect(errors.targetTypes).toMatch(/provider/i);
    expect(errors.workers).toMatch(/integer/i);
  });
});
