import { describe, expect, it } from 'vitest';
import en from './messages/en.js';
import ru from './messages/ru.js';
import zhCN from './messages/zh-CN.js';
import zhTW from './messages/zh-TW.js';

function keysOf(value, prefix = '') {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return [prefix];
  return Object.entries(value).flatMap(([key, child]) => keysOf(child, prefix ? `${prefix}.${key}` : key));
}

describe('i18n message dictionaries', () => {
  it('keeps every locale aligned with the English source keys', () => {
    const expected = keysOf(en).sort();
    for (const [locale, messages] of Object.entries({ 'zh-CN': zhCN, 'zh-TW': zhTW, ru })) {
      expect(keysOf(messages).sort(), locale).toEqual(expected);
    }
  });
});
