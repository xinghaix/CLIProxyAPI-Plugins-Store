import { createI18n } from 'vue-i18n';
import en from './messages/en.js';
import ru from './messages/ru.js';
import zhCN from './messages/zh-CN.js';
import zhTW from './messages/zh-TW.js';

export const SUPPORTED_LOCALES = ['en', 'zh-CN', 'zh-TW', 'ru'];
export const DEFAULT_LOCALE = 'en';

export const i18n = createI18n({
  legacy: false,
  locale: DEFAULT_LOCALE,
  fallbackLocale: DEFAULT_LOCALE,
  globalInjection: true,
  missingWarn: false,
  fallbackWarn: false,
  messages: {
    en,
    'zh-CN': zhCN,
    'zh-TW': zhTW,
    ru,
  },
});

export function setI18nLocale(locale) {
  const next = SUPPORTED_LOCALES.includes(locale) ? locale : DEFAULT_LOCALE;
  i18n.global.locale.value = next;
  return next;
}

export function translate(key, values) {
  return i18n.global.t(key, values);
}
