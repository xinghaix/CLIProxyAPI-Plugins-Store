import { describe, expect, it } from 'vitest';
import { buildUsageIOC, cacheTokenCount } from './usageBreakdown.js';

describe('buildUsageIOC', () => {
  it('always shows I O C on the second line', () => {
    expect(buildUsageIOC({ input_tokens: 1200, output_tokens: 80, cached_tokens: 0 }, String)).toBe('I 1200 · O 80 · C 0');
  });

  it('uses the larger of cached tokens and cache read/create', () => {
    expect(cacheTokenCount({ cached_tokens: 10, cache_read_tokens: 30, cache_creation_tokens: 20 })).toBe(50);
    expect(buildUsageIOC({ input_tokens: 100, output_tokens: 20, cache_read_tokens: 30, cache_creation_tokens: 20 }, String)).toBe('I 100 · O 20 · C 50');
  });
});
