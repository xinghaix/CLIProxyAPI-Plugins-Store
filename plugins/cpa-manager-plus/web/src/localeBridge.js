import { shallowRef } from 'vue';
import { DEFAULT_LOCALE, setI18nLocale } from './i18n/index.js';
import {
  clearManualLocale,
  normalizeLocale,
  readManualLocale,
  writeManualLocale,
} from './utils/localePreference.js';

export const localeRef = shallowRef(DEFAULT_LOCALE);
export const hostLocaleRef = shallowRef(null);
export const localeModeRef = shallowRef('follow');

let observer = null;
let initialized = false;

export function parseHostLanguage(value) {
  if (typeof value !== 'string') return null;
  try {
    const parsed = JSON.parse(value);
    if (typeof parsed === 'string') return normalizeLocale(parsed);
    if (parsed && typeof parsed === 'object') {
      return normalizeLocale(parsed.state?.language || parsed.language);
    }
    return null;
  } catch {
    return normalizeLocale(value);
  }
}

function readHostStoredLocale() {
  const storages = [];
  try {
    storages.push(window.localStorage);
  } catch {
    // Local storage is optional.
  }
  try {
    if (window.parent && window.parent !== window) storages.push(window.parent.localStorage);
  } catch {
    // Cross-origin parent cannot be read.
  }
  for (const storage of storages) {
    try {
      const locale = parseHostLanguage(storage?.getItem('cli-proxy-language'));
      if (locale) return locale;
    } catch {
      // Continue with the next fallback.
    }
  }
  return null;
}

export function getHostLocale() {
  try {
    if (window.parent && window.parent !== window) {
      const locale = normalizeLocale(window.parent.document.documentElement.lang);
      if (locale) return locale;
    }
  } catch {
    // Cross-origin parent falls through to the storage fallback.
  }
  return readHostStoredLocale();
}

export function resolveEffectiveLocale(manualLocale, hostLocale) {
  return manualLocale || hostLocale || DEFAULT_LOCALE;
}

function applyLocale(locale) {
  const next = setI18nLocale(locale);
  localeRef.value = next;
  document.documentElement.lang = next;
  window.dispatchEvent(new CustomEvent('cpa-manager-plus:locale-change', { detail: { locale: next } }));
  return next;
}

export function syncLocaleFromParent() {
  hostLocaleRef.value = getHostLocale();
  if (localeModeRef.value === 'follow') return applyLocale(resolveEffectiveLocale(null, hostLocaleRef.value));
  return localeRef.value;
}

export function setManualLocale(locale) {
  const saved = writeManualLocale(locale);
  if (!saved) return localeRef.value;
  localeModeRef.value = 'manual';
  return applyLocale(saved);
}

export function clearManualLocaleOverride() {
  clearManualLocale();
  localeModeRef.value = 'follow';
  return syncLocaleFromParent();
}

export function getLocale() {
  return localeRef.value;
}

export function getLocaleMode() {
  return localeModeRef.value;
}

export function initLocaleBridge() {
  if (initialized) return;
  initialized = true;

  const manual = readManualLocale();
  if (manual) {
    localeModeRef.value = 'manual';
    applyLocale(manual);
  } else {
    syncLocaleFromParent();
  }

  try {
    const parentRoot = window.parent?.document?.documentElement;
    if (parentRoot && window.parent !== window) {
      observer = new MutationObserver(() => syncLocaleFromParent());
      observer.observe(parentRoot, { attributes: true, attributeFilter: ['lang'] });
    }
  } catch {
    // Cross-origin and standalone pages retain the English fallback.
  }
}

export function destroyLocaleBridge() {
  observer?.disconnect();
  observer = null;
  initialized = false;
}
