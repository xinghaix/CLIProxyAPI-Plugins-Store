import { describe, expect, it } from 'vitest';
import { parseHostLanguage, resolveEffectiveLocale } from './localeBridge.js';

describe('locale bridge resolution', () => {
  it('uses manual override before the host locale', () => {
    expect(resolveEffectiveLocale('ru', 'zh-CN')).toBe('ru');
    expect(resolveEffectiveLocale(null, 'zh-TW')).toBe('zh-TW');
  });

  it('falls back to English when no locale is available', () => {
    expect(resolveEffectiveLocale(null, null)).toBe('en');
  });

  it('parses host storage values in plain, object, and JSON-string forms', () => {
    expect(parseHostLanguage('zh-CN')).toBe('zh-CN');
    expect(parseHostLanguage('{"state":{"language":"zh-CN"}}')).toBe('zh-CN');
    expect(parseHostLanguage('"zh-CN"')).toBe('zh-CN');
    expect(parseHostLanguage('{"language":"ru-RU"}')).toBe('ru');
  });
});
