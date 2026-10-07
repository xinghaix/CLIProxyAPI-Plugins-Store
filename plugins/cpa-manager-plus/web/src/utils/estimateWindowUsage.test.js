import { describe, expect, it } from 'vitest';
import { estimateWindowUsage } from './estimateWindowUsage.js';

describe('estimateWindowUsage', () => {
  it('projects from provider quota progress', () => {
    expect(
      estimateWindowUsage({
        usedPercent: 1,
        current: { requests: 2, tokens: 6400, cost: 0.02 },
      })
    ).toEqual({ requests: 200, tokens: 640000, cost: 2, basis: 'quota', costComplete: true, unpricedCalls: 0 });
  });

  it('rounds dynamic projections while preserving current minimums', () => {
    expect(
      estimateWindowUsage({
        usedPercent: 7,
        current: { requests: 1, tokens: 6400, cost: 0.06 },
      })
    ).toEqual({ requests: 14, tokens: 91429, cost: 0.86, basis: 'quota', costComplete: true, unpricedCalls: 0 });
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


  it('propagates incomplete pricing onto quota forecasts', () => {
    expect(
      estimateWindowUsage({
        usedPercent: 10,
        current: { requests: 10, tokens: 1000, cost: 0, costComplete: false, unpricedCalls: 4 },
      })
    ).toMatchObject({
      basis: 'quota',
      costComplete: false,
      unpricedCalls: 4,
      cost: 0,
    });
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
