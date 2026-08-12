import { describe, expect, it } from 'vitest';
import { computeCacheHitRate, formatCacheHitRate } from './cacheHitRate.js';

describe('cache hit rate', () => {
  it('returns null when there are no input/cache tokens', () => {
    expect(computeCacheHitRate({ input_tokens: 0, cached_tokens: 0 })).toBeNull();
    expect(formatCacheHitRate(null, (value) => `${value}%`)).toBe('—');
  });

  it('computes and clamps a populated rate', () => {
    expect(computeCacheHitRate({ input_tokens: 1000, cached_tokens: 200, cache_read_tokens: 300 })).toBeCloseTo(5 / 13);
    expect(computeCacheHitRate({ cache_hit_rate: 1.5, total_tokens: 1 })).toBe(1);
  });

  it('accepts an explicit zero rate when usage exists', () => {
    expect(computeCacheHitRate({ cache_hit_rate: 0, input_tokens: 100 })).toBe(0);
    expect(formatCacheHitRate(0, (value) => `${value}%`)).toBe('0%');
  });

  it('does not turn a zero-usage row with an explicit zero into 0%', () => {
    expect(computeCacheHitRate({ cache_hit_rate: 0, total_tokens: 0 })).toBeNull();
  });
});
