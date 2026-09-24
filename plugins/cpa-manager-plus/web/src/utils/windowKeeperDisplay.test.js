import { describe, expect, it } from 'vitest';
import { formatAttemptText, formatWindowRemainingText, remainingPercentFromUsed } from './windowKeeperDisplay.js';

describe('remainingPercentFromUsed', () => {
  it('converts used to remaining', () => {
    expect(remainingPercentFromUsed(2)).toBe(98);
    expect(remainingPercentFromUsed(67)).toBe(33);
    expect(remainingPercentFromUsed(0)).toBe(100);
    expect(remainingPercentFromUsed(100)).toBe(0);
  });

  it('clamps and rejects invalid', () => {
    expect(remainingPercentFromUsed(-5)).toBe(100);
    expect(remainingPercentFromUsed(150)).toBe(0);
    expect(remainingPercentFromUsed(undefined)).toBeNull();
    expect(remainingPercentFromUsed('x')).toBeNull();
  });
});

describe('formatWindowRemainingText', () => {
  it('formats Chinese remaining badge', () => {
    expect(formatWindowRemainingText('5小时限额', 2, '剩余')).toBe('5小时限额剩余 98%');
    expect(formatWindowRemainingText('周限额', 67, '剩余')).toBe('周限额剩余 33%');
  });

  it('formats English remaining badge', () => {
    expect(formatWindowRemainingText('5h', 2, ' remaining')).toBe('5h remaining 98%');
  });
});

describe('formatAttemptText', () => {
  it('unescapes HTML entities in attempt JSON bodies', () => {
    expect(formatAttemptText('{&#34;model&#34;:&#34;gpt-5.4&#34;}')).toBe('{"model":"gpt-5.4"}');
    expect(formatAttemptText('{"model":"gpt-5.4"}')).toBe('{"model":"gpt-5.4"}');
  });

  it('handles empty values', () => {
    expect(formatAttemptText('')).toBe('');
    expect(formatAttemptText(null)).toBe('');
  });
});
