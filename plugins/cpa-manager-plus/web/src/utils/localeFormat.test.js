import { describe, expect, it } from 'vitest';
import {
  formatBucketDateTime,
  formatCompactDateTime,
  formatDateTime,
  formatInt,
  formatNumber,
  formatShortTime,
  formatWeekday,
  formatWeekdayIndex,
} from './localeFormat.js';

describe('locale formatters', () => {
  it('formats numbers with the supplied locale', () => {
    expect(formatInt(1234567, 'en')).toBe('1,234,567');
    expect(formatNumber(1234.5, { maximumFractionDigits: 1 }, 'ru')).toContain('1');
  });

  it('formats weekdays with the supplied locale and preserves empty values', () => {
    expect(formatWeekday(Date.UTC(2026, 0, 5), 'en')).toMatch(/Mon/i);
    expect(formatNumber(null, {}, 'en')).toBe('—');
  });

  it('formats weekday indices and short times', () => {
    expect(formatWeekdayIndex(0, 'en')).toMatch(/Sun/i);
    expect(formatWeekdayIndex(1, 'en')).toMatch(/Mon/i);
    expect(formatWeekdayIndex(99, 'en')).toBe('—');
    expect(formatShortTime(Date.UTC(2026, 0, 5, 14, 30), 'en')).toMatch(/14:30|2:30/);
    expect(formatBucketDateTime(Date.UTC(2026, 0, 5, 14, 30), 'en')).toMatch(/01|1/);
  });

  it('formats datetimes with an explicit timezone when provided', () => {
    const ms = Date.UTC(2026, 0, 5, 6, 30, 0);
    const shanghai = formatCompactDateTime(ms, 'en', 'Asia/Shanghai');
    expect(shanghai).toMatch(/01\/05|1\/5/);
    expect(shanghai).toMatch(/14:30/);
    expect(formatDateTime(ms, 'en', 'UTC')).toMatch(/2026/);
  });
});
