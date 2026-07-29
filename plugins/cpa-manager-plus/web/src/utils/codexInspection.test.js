import { describe, expect, it } from 'vitest';
import {
  createConfigFromDraft,
  formatActionStatusLabel,
  getCanonicalActionIds,
  getInspectionResultsEmptyText,
  isAcknowledgeableResult,
  isActionableResult,
  isNeedsHandling,
  normalizeActionStatus,
  normalizeInspectionTargetTypes,
  resolveServerCodexConfig,
  toDraft,
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
  it('explains that no credential was found', () => {
    expect(getInspectionResultsEmptyText({ status: 'completed', totalFiles: 0 }, 0, 0))
      .toBe('未发现可巡检凭据；本次未对任何账号执行真实探测。');
  });

  it('explains provider filtering before sampling', () => {
    expect(getInspectionResultsEmptyText({ status: 'completed', totalFiles: 3, probeSetCount: 0 }, 0, 0))
      .toBe('主机发现 3 个凭据文件，但目标类型过滤后没有可探测账号。请检查巡检提供商。');
  });

  it('keeps the filter-specific empty state when raw results exist', () => {
    expect(getInspectionResultsEmptyText({ status: 'completed', totalFiles: 1, probeSetCount: 1, sampledCount: 1 }, 1, 0))
      .toBe('当前筛选下无结果。');
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
    const acknowledged = { id: 1, action: 'review', actionStatus: 'acknowledged', executedAction: 'acknowledge' };
    expect(normalizeActionStatus(acknowledged)).toBe('acknowledged');
    expect(formatActionStatusLabel(acknowledged)).toBe('已标记处理');
    expect(isNeedsHandling(acknowledged)).toBe(false);
  });
});
