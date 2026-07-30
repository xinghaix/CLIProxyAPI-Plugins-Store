export const UI_LANGUAGE_KEY = 'cpa-manager-plus:ui-language';
export const SUPPORTED_UI_LOCALES = ['en', 'zh-CN', 'zh-TW', 'ru'];

export function normalizeLocale(value) {
  const raw = String(value || '').trim().replace(/_/g, '-');
  if (!raw) return null;
  const lower = raw.toLowerCase();
  const exact = SUPPORTED_UI_LOCALES.find((locale) => locale.toLowerCase() === lower);
  if (exact) return exact;
  if (lower === 'zh' || lower.startsWith('zh-hans') || lower.startsWith('zh-cn')) return 'zh-CN';
  if (lower.startsWith('zh-hant') || lower.startsWith('zh-tw') || lower.startsWith('zh-hk')) return 'zh-TW';
  if (lower.startsWith('en')) return 'en';
  if (lower.startsWith('ru')) return 'ru';
  return null;
}

function getStorage(storage) {
  if (storage) return storage;
  try {
    return globalThis.localStorage;
  } catch {
    return null;
  }
}

export function readManualLocale(storage) {
  try {
    return normalizeLocale(getStorage(storage)?.getItem(UI_LANGUAGE_KEY));
  } catch {
    return null;
  }
}

export function writeManualLocale(locale, storage) {
  const normalized = normalizeLocale(locale);
  if (!normalized) return null;
  try {
    getStorage(storage)?.setItem(UI_LANGUAGE_KEY, normalized);
  } catch {
    // Use the in-memory locale when storage is unavailable.
  }
  return normalized;
}

export function clearManualLocale(storage) {
  try {
    getStorage(storage)?.removeItem(UI_LANGUAGE_KEY);
  } catch {
    // The bridge can still return to follow mode in memory.
  }
}
