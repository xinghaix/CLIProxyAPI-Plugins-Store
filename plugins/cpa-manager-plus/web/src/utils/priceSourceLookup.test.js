import { describe, expect, it } from 'vitest';
import {
  buildPriceSourceLookupRequest,
  buildSourcePriceEntry,
  normalizePriceSourceLookup,
} from './priceSourceLookup.js';

describe('price source lookup', () => {
  it('normalizes selectable source prices', () => {
    const result = normalizePriceSourceLookup({
      model: 'gpt-test',
      sources: [{
        source: 'models.dev:xai',
        sourceModelId: 'xai/gpt-test',
        priority: 10,
        available: ['prompt', 'completion', 'cacheRead'],
        price: { prompt: 2, completion: 6, cacheRead: 0.3 },
      }],
    });
    expect(result.model).toBe('gpt-test');
    expect(result.sources[0]).toMatchObject({
      source: 'models.dev:xai',
      sourceModelId: 'xai/gpt-test',
      priority: 10,
      prompt: 2,
      completion: 6,
      cacheRead: 0.3,
      available: ['prompt', 'completion', 'cacheRead'],
    });
  });

  it('builds encoded lookup requests', () => {
    expect(buildPriceSourceLookupRequest(' acme/claude ')).toEqual({
      method: 'GET',
      path: '/v0/management/model-prices/source-lookup',
      query: 'model=acme%2Fclaude',
    });
    expect(buildPriceSourceLookupRequest('')).toBeNull();
  });

  it('builds a manual replacement entry from a selected source', () => {
    expect(buildSourcePriceEntry('gpt-test', {
      source: 'litellm',
      sourceModelId: 'gpt-test',
      prompt: 2,
      completion: 6,
      cache: 0,
      cacheRead: 0.5,
      cacheCreation: 0,
    })).toEqual({
      model: 'gpt-test',
      price: {
        prompt: 2,
        completion: 6,
        cache: 0,
        cacheRead: 0.5,
        cacheCreation: 0,
        source: 'manual',
        sourceModelId: '',
        syncedAtMs: 0,
      },
      selectedSource: { source: 'litellm', sourceModelId: 'gpt-test' },
    });
    expect(buildSourcePriceEntry('', {})).toBeNull();
  });
});
