import { getLocale } from '../localeBridge.js';

export const EMPTY_VALUE = '—';

const numberFormatCache = new Map();
const dateTimeFormatCache = new Map();

function activeLocale(locale) {
  return locale || getLocale() || 'en';
}

function getNumberFormatter(locale, options = {}) {
  const loc = activeLocale(locale);
  const key = `${loc}|${JSON.stringify(options)}`;
  let fmt = numberFormatCache.get(key);
  if (!fmt) {
    fmt = new Intl.NumberFormat(loc, options);
    if (numberFormatCache.size > 64) numberFormatCache.clear();
    numberFormatCache.set(key, fmt);
  }
  return fmt;
}

function getDateTimeFormatter(locale, options = {}) {
  const loc = activeLocale(locale);
  const key = `${loc}|${JSON.stringify(options)}`;
  let fmt = dateTimeFormatCache.get(key);
  if (!fmt) {
    fmt = new Intl.DateTimeFormat(loc, options);
    if (dateTimeFormatCache.size > 64) dateTimeFormatCache.clear();
    dateTimeFormatCache.set(key, fmt);
  }
  return fmt;
}

export function formatNumber(value, options = {}, locale) {
  if (value == null || value === '') return EMPTY_VALUE;
  const number = Number(value);
  if (!Number.isFinite(number)) return String(value);
  return getNumberFormatter(locale, options).format(number);
}

export function formatInt(value, locale) {
  return formatNumber(value, { maximumFractionDigits: 0 }, locale);
}

export function formatDateTime(value, locale, timeZone) {
  if (!value) return EMPTY_VALUE;
  const date = new Date(Number(value));
  const options = { hour12: false };
  if (timeZone) options.timeZone = timeZone;
  try {
    return getDateTimeFormatter(locale, options).format(date);
  } catch {
    return getDateTimeFormatter(locale, { hour12: false }).format(date);
  }
}

/** Compact month/day hour:minute, optionally in a plugin timezone. */
export function formatCompactDateTime(value, locale, timeZone) {
  if (!value && value !== 0) return EMPTY_VALUE;
  const date = new Date(Number(value));
  const baseOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  };
  const options = timeZone ? { ...baseOptions, timeZone } : baseOptions;
  try {
    return getDateTimeFormatter(locale, options).format(date);
  } catch {
    return getDateTimeFormatter(locale, baseOptions).format(date);
  }
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
  return getDateTimeFormatter(locale, { weekday: 'short' }).format(new Date(Number(value)));
}

/** Short clock time (hour:minute) for timeline labels. */
export function formatShortTime(value, locale) {
  if (!value && value !== 0) return EMPTY_VALUE;
  return getDateTimeFormatter(locale, {
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(Number(value)));
}

/** Compact bucket label: month/day hour:minute. */
export function formatBucketDateTime(value, locale) {
  if (!value && value !== 0) return EMPTY_VALUE;
  return getDateTimeFormatter(locale, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(Number(value)));
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
  return getDateTimeFormatter(locale, { weekday: 'short', timeZone: 'UTC' }).format(date);
}
