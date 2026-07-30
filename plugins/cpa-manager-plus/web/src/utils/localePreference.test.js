import { describe, expect, it } from 'vitest';
import {
  UI_LANGUAGE_KEY,
  clearManualLocale,
  normalizeLocale,
  readManualLocale,
  writeManualLocale,
} from './localePreference.js';

function createStorage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
  };
}

describe('locale preference', () => {
  it('normalizes CPA and browser locale variants', () => {
    expect(normalizeLocale('en-US')).toBe('en');
    expect(normalizeLocale('zh-CN')).toBe('zh-CN');
    expect(normalizeLocale('zh-cn')).toBe('zh-CN');
    expect(normalizeLocale('zh_CN')).toBe('zh-CN');
    expect(normalizeLocale('zh-Hans')).toBe('zh-CN');
    expect(normalizeLocale('zh-HK')).toBe('zh-TW');
    expect(normalizeLocale('ru_RU')).toBe('ru');
    expect(normalizeLocale('fr-FR')).toBeNull();
  });

  it('persists only supported manual locales', () => {
    const storage = createStorage();
    expect(writeManualLocale('zh-TW', storage)).toBe('zh-TW');
    expect(storage.getItem(UI_LANGUAGE_KEY)).toBe('zh-TW');
    expect(readManualLocale(storage)).toBe('zh-TW');
    expect(writeManualLocale('fr', storage)).toBeNull();
    clearManualLocale(storage);
    expect(readManualLocale(storage)).toBeNull();
  });
});
