import { getLocale } from '../localeBridge.js';

export const EMPTY_VALUE = '—';

function activeLocale(locale) {
  return locale || getLocale() || 'en';
}

export function formatNumber(value, options = {}, locale) {
  if (value == null || value === '') return EMPTY_VALUE;
  const number = Number(value);
  if (!Number.isFinite(number)) return String(value);
  return new Intl.NumberFormat(activeLocale(locale), options).format(number);
}

export function formatInt(value, locale) {
  return formatNumber(value, { maximumFractionDigits: 0 }, locale);
}

export function formatDateTime(value, locale) {
  if (!value) return EMPTY_VALUE;
  return new Date(Number(value)).toLocaleString(activeLocale(locale), { hour12: false });
}

export function formatDate(value, locale) {
  if (!value) return EMPTY_VALUE;
  return new Date(Number(value)).toLocaleDateString(activeLocale(locale));
}

export function formatTime(value, locale) {
  if (!value) return EMPTY_VALUE;
  return new Date(Number(value)).toLocaleTimeString(activeLocale(locale), { hour12: false });
}

export function formatWeekday(value, locale) {
  if (!value) return EMPTY_VALUE;
  return new Intl.DateTimeFormat(activeLocale(locale), { weekday: 'short' }).format(new Date(Number(value)));
}

/** Short clock time (hour:minute) for timeline labels. */
export function formatShortTime(value, locale) {
  if (!value && value !== 0) return EMPTY_VALUE;
  return new Date(Number(value)).toLocaleTimeString(activeLocale(locale), {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

/** Compact bucket label: month/day hour:minute. */
export function formatBucketDateTime(value, locale) {
  if (!value && value !== 0) return EMPTY_VALUE;
  return new Date(Number(value)).toLocaleString(activeLocale(locale), {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
}

/**
 * Weekday short label for heatmap rows.
 * Index 0 = Sunday … 6 = Saturday (matches backend heatmap weekday).
 */
export function formatWeekdayIndex(index, locale) {
  const i = Number(index);
  if (!Number.isInteger(i) || i < 0 || i > 6) return EMPTY_VALUE;
  // 2024-01-07 is a Sunday; pin noon UTC so the weekday is stable across zones.
  const date = new Date(Date.UTC(2024, 0, 7 + i, 12, 0, 0));
  return new Intl.DateTimeFormat(activeLocale(locale), { weekday: 'short', timeZone: 'UTC' }).format(date);
}
