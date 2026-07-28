import { describe, expect, it } from 'vitest';
import {
  DEFAULT_SYNC_SETTINGS,
  buildConfirmBody,
  buildFilterCounts,
  buildManualPriceEntry,
  buildManualPutBody,
  buildModelPriceRows,
  buildSyncModels,
  clampIntervalHours,
  extractPricesMap,
  extractUsageModels,
  filterModelPriceRows,
  formatMoneyPer1M,
  formatSourceLabel,
  normalizeCandidates,
  normalizeSourceResults,
  normalizeSyncResult,
  normalizeSyncSettings,
  normalizeSyncStatus,
  sourceBadgeClass,
  summarizeLastResult,
  validateIntervalHours,
} from './priceSync.js';

describe('extractPricesMap', () => {
  it('reads nested prices and normalizes fields', () => {
    const map = extractPricesMap({
      prices: {
        'gpt-4o': {
          Prompt: 2.5,
          completion: 10,
          Source: 'litellm',
          SourceModelId: 'gpt-4o',
        },
      },
    });
    expect(map['gpt-4o'].prompt).toBe(2.5);
    expect(map['gpt-4o'].completion).toBe(10);
    expect(map['gpt-4o'].source).toBe('litellm');
    expect(map['gpt-4o'].sourceModelId).toBe('gpt-4o');
  });

  it('returns empty on bad body', () => {
    expect(extractPricesMap(null)).toEqual({});
    expect(extractPricesMap([])).toEqual({});
  });
});

describe('extractUsageModels / buildSyncModels', () => {
  it('collects model names from summary objects or strings', () => {
    expect(extractUsageModels({ models: [{ model: 'a' }, { Model: 'b' }, 'c'] })).toEqual([
      'a',
      'b',
      'c',
    ]);
  });

  it('unions prices and usage models', () => {
    expect(buildSyncModels({ z: {}, a: {} }, ['m', 'a'])).toEqual(['a', 'm', 'z']);
  });
});

describe('settings', () => {
  it('defaults and clamps interval', () => {
    expect(normalizeSyncSettings(null)).toEqual(DEFAULT_SYNC_SETTINGS);
    expect(normalizeSyncSettings({ enabled: true, intervalHours: 24, protectManual: false })).toEqual({
      enabled: true,
      intervalHours: 24,
      protectManual: false,
    });
    expect(clampIntervalHours(0)).toBe(6);
    expect(clampIntervalHours(999)).toBe(168);
    expect(validateIntervalHours(12).ok).toBe(true);
    expect(validateIntervalHours(1).ok).toBe(false);
    expect(validateIntervalHours(6).ok).toBe(true);
  });
});

describe('normalizeSyncResult / status', () => {
  it('normalizes plan-style RunResult', () => {
    const result = normalizeSyncResult({
      trigger: 'manual',
      targetCount: 3,
      applied: 1,
      protectedSkipped: 1,
      unmatched: 1,
      sources: [
        { source: 'litellm', ok: true, matched: 2, applied: 1, durationMs: 120 },
        { source: 'openrouter', ok: false, error: 'timeout' },
      ],
      candidates: [
        {
          localModel: 'gpt-mini',
          source: 'openrouter',
          sourceModelId: 'openai/gpt-mini',
          prompt: 0.1,
          completion: 0.2,
          reason: 'fuzzy',
          score: 0.7,
        },
      ],
    });
    expect(result.applied).toBe(1);
    expect(result.sources).toHaveLength(2);
    expect(result.sources[1].ok).toBe(false);
    expect(result.candidates[0].localModel).toBe('gpt-mini');
    expect(result.candidateCount).toBe(1);
  });

  it('normalizes backend pricesync.Result shape', () => {
    const result = normalizeSyncResult({
      source: 'multi',
      sources: ['litellm', 'openrouter'],
      imported: 4,
      skipped: 1,
      protectedManual: 2,
      sourceResults: [
        { source: 'litellm', models: 200, skipped: 3 },
        { source: 'openrouter', models: 0, error: 'timeout' },
      ],
      candidates: [
        {
          model: 'gpt-mini',
          candidates: [
            {
              source: 'openrouter',
              sourceModelId: 'openai/gpt-mini',
              score: 0.8,
              reason: 'fuzzy',
              price: { prompt: 0.1, completion: 0.2, source: 'openrouter', sourceModelId: 'openai/gpt-mini' },
            },
          ],
        },
      ],
      unmatched: ['x'],
    });
    expect(result.applied).toBe(4);
    expect(result.protectedSkipped).toBe(2);
    expect(result.sources[0].modelCount).toBe(200);
    expect(result.sources[1].ok).toBe(false);
    expect(result.candidates[0].localModel).toBe('gpt-mini');
    expect(result.candidates[0].sourceModelId).toBe('openai/gpt-mini');
    expect(result.unmatched).toBe(1);
  });

  it('normalizes legacy candidate sets and sourceResults', () => {
    const result = normalizeSyncResult({
      imported: 2,
      sourceResults: [{ source: 'litellm', models: 100, skipped: 1 }],
      candidates: [
        {
          model: 'x',
          candidates: [
            {
              sourceModelId: 'src/x',
              price: { prompt: 1, completion: 2, source: 'litellm' },
              score: 0.9,
            },
          ],
        },
      ],
      unmatched: ['y'],
    });
    expect(result.applied).toBe(2);
    expect(result.sources[0].modelCount).toBe(100);
    expect(result.candidates[0].sourceModelId).toBe('src/x');
    expect(result.unmatched).toBe(1);
    expect(result.unmatchedModels).toEqual(['y']);
  });

  it('normalizes status with last or lastResult', () => {
    const a = normalizeSyncStatus({
      lastSyncAtMs: 100,
      lastSuccessAtMs: 100,
      lastError: '',
      lastResult: { applied: 2, targetCount: 5, candidates: [] },
    });
    expect(a.lastSyncAtMs).toBe(100);
    expect(a.lastResult.applied).toBe(2);

    const b = normalizeSyncStatus({
      running: true,
      settings: { enabled: true, intervalHours: 6 },
      last: { finishedAtMs: 200, applied: 1, sources: [] },
    });
    expect(b.running).toBe(true);
    expect(b.settings.enabled).toBe(true);
    expect(b.lastSyncAtMs).toBe(200);
  });
});

describe('normalizeSourceResults / candidates', () => {
  it('handles object map of sources', () => {
    const list = normalizeSourceResults({ litellm: { ok: true, matched: 3 } });
    expect(list[0].source).toBe('litellm');
    expect(list[0].matched).toBe(3);
  });

  it('returns empty for null candidates', () => {
    expect(normalizeCandidates(null)).toEqual([]);
  });
});

describe('rows / filters', () => {
  it('builds and filters rows', () => {
    const prices = {
      saved: { prompt: 1, completion: 2, source: 'manual', sourceModelId: '' },
    };
    const rows = buildModelPriceRows(prices, ['missing'], [
      { localModel: 'missing', source: 'litellm', sourceModelId: 'm', prompt: 1, completion: 2 },
    ]);
    expect(rows.map((r) => r.model)).toEqual(['missing', 'saved']);
    const counts = buildFilterCounts(rows);
    expect(counts).toEqual({ all: 2, missing: 1, saved: 1, candidates: 1 });
    expect(filterModelPriceRows(rows, 'candidates', '').map((r) => r.model)).toEqual(['missing']);
    expect(filterModelPriceRows(rows, 'all', 'manual')).toHaveLength(1);
  });
});

describe('formatters / builders', () => {
  it('formats money and source labels', () => {
    expect(formatMoneyPer1M(1.2)).toBe('$1.2000');
    expect(formatMoneyPer1M(null)).toBe('—');
    expect(formatSourceLabel('manual')).toBe('手动');
    expect(formatSourceLabel('litellm')).toBe('LiteLLM');
    expect(sourceBadgeClass('manual')).toBe('source-manual');
  });

  it('builds manual entry with source=manual', () => {
    const entry = buildManualPriceEntry({
      model: ' my ',
      prompt: 1,
      completion: -2,
      cache: 'x',
      cacheRead: 0.5,
      cacheCreation: 0,
    });
    expect(entry.model).toBe('my');
    expect(entry.price.source).toBe('manual');
    expect(entry.price.sourceModelId).toBe('');
    expect(entry.price.syncedAtMs).toBe(0);
    expect(entry.price.completion).toBe(0);
    expect(entry.price.cache).toBe(0);
    expect(entry.price.cacheRead).toBe(0.5);
  });

  it('builds single-model PUT body only', () => {
    const body = buildManualPutBody({
      model: 'gpt-x',
      prompt: 1,
      completion: 2,
      cache: 0,
      cacheRead: 0,
      cacheCreation: 0,
    });
    expect(Object.keys(body.prices)).toEqual(['gpt-x']);
    expect(body.prices['gpt-x']).toEqual({
      prompt: 1,
      completion: 2,
      cache: 0,
      cacheRead: 0,
      cacheCreation: 0,
      source: 'manual',
      sourceModelId: '',
      syncedAtMs: 0,
    });
  });

  it('builds confirm body for sync-confirm contract', () => {
    const body = buildConfirmBody({
      localModel: 'm',
      source: 'openrouter',
      sourceModelId: 'openai/m',
      prompt: 1,
      completion: 2,
      cache: 0,
      cacheRead: 0,
      cacheCreation: 0,
    });
    expect(body).toEqual({
      model: 'm',
      price: {
        prompt: 1,
        completion: 2,
        cache: 0,
        cacheRead: 0,
        cacheCreation: 0,
        source: 'openrouter',
        sourceModelId: 'openai/m',
      },
    });
    expect(buildConfirmBody({ localModel: 'm', source: '', sourceModelId: 'x' })).toBeNull();
  });

  it('summarizes last result', () => {
    expect(summarizeLastResult(null, 4).targetCount).toBe(4);
    expect(summarizeLastResult({ applied: 2, protectedSkipped: 1, unmatched: 3, candidateCount: 5 }, 9)).toEqual({
      targetCount: 9,
      applied: 2,
      protectedSkipped: 1,
      candidateCount: 5,
      unmatched: 3,
    });
    expect(
      summarizeLastResult({ targetCount: 3, applied: 1, protectedSkipped: 0, unmatched: 0, candidateCount: 0 }),
    ).toMatchObject({ targetCount: 3, applied: 1 });
  });
});
