const HTML_ENTITY_RE = /&(#(?:x[\da-f]+|\d+)|quot|apos|amp|lt|gt|nbsp);/gi;
const HTML_ENTITY_VALUES = {
  amp: '&',
  apos: "'",
  gt: '>',
  lt: '<',
  nbsp: '\u00a0',
  quot: '"',
};

function decodeHtmlEntity(match, entity) {
  const normalized = entity.toLowerCase();
  if (normalized.startsWith('#x')) {
    const codePoint = Number.parseInt(normalized.slice(2), 16);
    return Number.isFinite(codePoint) ? String.fromCodePoint(codePoint) : match;
  }
  if (normalized.startsWith('#')) {
    const codePoint = Number.parseInt(normalized.slice(1), 10);
    return Number.isFinite(codePoint) ? String.fromCodePoint(codePoint) : match;
  }
  return HTML_ENTITY_VALUES[normalized] || match;
}

export function decodeHtmlEntities(value) {
  if (value == null) return '';
  let decoded = String(value);
  if (!decoded || typeof document === 'undefined') {
    return decoded.replace(HTML_ENTITY_RE, decodeHtmlEntity);
  }
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const textarea = document.createElement('textarea');
    textarea.innerHTML = decoded;
    const next = textarea.value;
    if (next === decoded) break;
    decoded = next;
  }
  return decoded;
}

function findQuotaMessage(value) {
  if (typeof value === 'string') return value.trim();
  if (!value || typeof value !== 'object') return '';
  const nested = value.error ?? value;
  if (typeof nested === 'string') return nested.trim();
  if (!nested || typeof nested !== 'object') return '';
  for (const key of ['message', 'detail', 'error', 'description']) {
    const message = findQuotaMessage(nested[key]);
    if (message) return message;
  }
  return '';
}

export function formatQuotaStatusMessage(value) {
  if (value == null || value === '') return '';
  if (typeof value === 'object') {
    return findQuotaMessage(value) || JSON.stringify(value);
  }
  const decoded = decodeHtmlEntities(value).trim();
  if (!decoded) return '';
  try {
    const parsed = JSON.parse(decoded);
    return findQuotaMessage(parsed) || decoded;
  } catch {
    return decoded;
  }
}

export function quotaResetTimestamp(value) {
  if (value == null || value === '') return null;
  const numeric = Number(value);
  if (Number.isFinite(numeric) && numeric > 0) {
    return numeric < 1e11 ? numeric * 1000 : numeric;
  }
  const parsed = Date.parse(String(value));
  return Number.isFinite(parsed) ? parsed : null;
}

export function formatQuotaResetRelative(value, nowMs = Date.now(), locale) {
  const resetAtMs = quotaResetTimestamp(value);
  const currentMs = Number(nowMs);
  if (resetAtMs == null || !Number.isFinite(currentMs)) return '';
  const remainingMs = resetAtMs - currentMs;
  if (remainingMs <= 0) return '';

  let amount;
  let unit;
  if (remainingMs < 60 * 60 * 1000) {
    amount = Math.max(1, Math.ceil(remainingMs / (60 * 1000)));
    unit = 'minute';
  } else if (remainingMs < 24 * 60 * 60 * 1000) {
    amount = Math.max(1, Math.ceil(remainingMs / (60 * 60 * 1000)));
    unit = 'hour';
  } else {
    amount = Math.max(1, Math.ceil(remainingMs / (24 * 60 * 60 * 1000)));
    unit = 'day';
  }

  try {
    return new Intl.RelativeTimeFormat(locale || undefined, {numeric: 'always'}).format(amount, unit);
  } catch {
    return `${amount} ${unit}${amount === 1 ? '' : 's'}`;
  }
}

export function normalizeQuotaWindows(result) {
  const windows = Array.isArray(result?.quotaWindows) ? result.quotaWindows : [];
  return windows.map((window, index) => {
    const rawUsedPercent = window?.usedPercent;
    const parsedUsedPercent = rawUsedPercent == null ? NaN : Number(rawUsedPercent);
    const parsedRemaining = Number(window?.remaining);
    return {
      id: window?.id || `window-${index}`,
      label: window?.label || window?.id || '',
      hasUsedPercent: Number.isFinite(parsedUsedPercent),
      usedPercent: Number.isFinite(parsedUsedPercent) ? parsedUsedPercent : 0,
      remaining: Number.isFinite(parsedRemaining) ? parsedRemaining : null,
      remainingText: typeof window?.remaining === 'string' ? window.remaining.trim() : '',
      resetText: window?.resetAt ? String(window.resetAt) : '',
    };
  }).filter(window => window.hasUsedPercent || window.remaining !== null || window.remainingText || window.resetText);
}
