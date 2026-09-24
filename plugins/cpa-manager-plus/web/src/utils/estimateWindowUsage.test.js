import { describe, expect, it } from 'vitest';
import { estimateWindowUsage } from './estimateWindowUsage.js';

describe('estimateWindowUsage', () => {
  it('projects from provider quota progress', () => {
    expect(
      estimateWindowUsage({
        usedPercent: 1,
        current: { requests: 2, tokens: 6400, cost: 0.02 },
      })
    ).toEqual({ requests: 200, tokens: 640000, cost: 2, basis: 'quota' });
  });

  it('rounds dynamic projections while preserving current minimums', () => {
    expect(
      estimateWindowUsage({
        usedPercent: 7,
        current: { requests: 1, tokens: 6400, cost: 0.06 },
      })
    ).toEqual({ requests: 14, tokens: 91429, cost: 0.86, basis: 'quota' });
  });

  it('uses previous when quota progress unavailable', () => {
    expect(
      estimateWindowUsage({
        usedPercent: 0,
        current: { requests: 1, tokens: 10, cost: 0.01 },
        previous: { requests: 60, tokens: 600000, cost: 6 },
      })
    ).toEqual({ requests: 60, tokens: 600000, cost: 6, basis: 'previous' });
  });

  it('returns null without a usable basis', () => {
    expect(
      estimateWindowUsage({
        usedPercent: null,
        current: { requests: 2, tokens: 6400, cost: 0.02 },
      })
    ).toBeNull();
  });
});
