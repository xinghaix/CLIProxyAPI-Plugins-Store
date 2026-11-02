import { afterEach, describe, expect, it } from 'vitest';
import { setI18nLocale } from '../i18n/index.js';
import { PROXY, HEALTH, accountActionPath, formatHealthText, num, formatCell, readCPAAuthStoreKey } from './data.js';
import {
  MC_STORAGE_KEY_AUTH,
  deobfuscateData,
  isCurrentObfuscated,
  isObfuscated,
  readManagementKeyFromMCStorage,
} from './mcAuthStorage.js';

describe('API constants', () => {
  it('uses /api gateway instead of /proxy', () => {
    expect(PROXY).toBe('/v0/management/cpa-manager-plus/api');
    expect(PROXY).not.toContain('/proxy');
    expect(HEALTH).toBe('/v0/management/cpa-manager-plus/health');
  });
});

describe('accountActionPath', () => {
  it('builds enable/ignore/resolve paths', () => {
    expect(accountActionPath('abc', 'enable')).toBe(
      '/v0/management/account-action-candidates/abc/enable',
    );
    expect(accountActionPath('abc', 'ignore')).toBe(
      '/v0/management/account-action-candidates/abc/ignore',
    );
    expect(accountActionPath('abc', 'resolve')).toBe(
      '/v0/management/account-action-candidates/abc/resolve',
    );
  });

  it('uses auth-file for delete', () => {
    expect(accountActionPath('id-1', 'delete')).toBe(
      '/v0/management/account-action-candidates/id-1/auth-file',
    );
  });

  it('encodes id and rejects unknown actions', () => {
    expect(accountActionPath('a/b', 'enable')).toBe(
      '/v0/management/account-action-candidates/a%2Fb/enable',
    );
    expect(() => accountActionPath('x', 'nope')).toThrow(/unknown account action/);
  });
});

describe('formatHealthText', () => {
  it('formats local runtime ok body in English by default', () => {
    setI18nLocale('en');
    const text = formatHealthText({
      ok: true,
      mode: 'local',
      data_dir: '/tmp/cpa-manager-plus',
      db_ok: true,
    });
    expect(text).toContain('Local Runtime is healthy');
    expect(text).toContain('local');
    expect(text).toContain('/tmp/cpa-manager-plus');
  });

  it('follows zh-CN health copy and keeps raw backend errors', () => {
    setI18nLocale('zh-CN');
    const text = formatHealthText({
      ok: true,
      mode: 'local',
      data_dir: '/tmp/cpa-manager-plus',
      db_ok: false,
    });
    expect(text).toContain('本地 Runtime 正常');
    expect(text).toContain('DB 异常');
    expect(formatHealthText({ ok: false, error: 'db locked' })).toBe('db locked');
    expect(formatHealthText(null)).toContain('未知');
  });

  it('uses Russian unhealthy wording', () => {
    setI18nLocale('ru');
    expect(formatHealthText({ ok: false })).toContain('неисправен');
  });
});

describe('num / formatCell', () => {
  it('formats numbers with the active locale and keeps empty markers', () => {
    setI18nLocale('en');
    expect(num(null)).toBe('—');
    expect(num(1234.5)).toBe('1,234.5');
    expect(formatCell(null)).toBe('—');
    expect(formatCell({ a: 1 })).toBe('{"a":1}');
  });
});

// 模拟 Management Center 的 XOR 混淆逻辑以验证 v2 与旧版 v1 解混淆兼容性
function makeObfuscatedPayload(rawText, { version = 'v2', host = '127.0.0.1:8317', ua = 'TestUA/1.0' } = {}) {
  const salt = 'cli-proxy-api-webui::secure-storage';
  const keyText = version === 'v1' ? `${salt}|${host}|${ua}` : `${salt}|${host}`;
  const keyBytes = new TextEncoder().encode(keyText);
  const dataBytes = new TextEncoder().encode(rawText);
  const out = new Uint8Array(dataBytes.length);
  for (let i = 0; i < dataBytes.length; i++) {
    out[i] = dataBytes[i] ^ keyBytes[i % keyBytes.length];
  }
  let binary = '';
  for (let i = 0; i < out.length; i++) {
    binary += String.fromCharCode(out[i]);
  }
  return `enc::${version}::${btoa(binary)}`;
}

function createMemoryStorage(initial = {}) {
  const map = new Map(Object.entries(initial));
  return {
    getItem(k) {
      return map.has(k) ? map.get(k) : null;
    },
    setItem(k, v) {
      map.set(k, String(v));
    },
  };
}

describe('mcAuthStorage & readCPAAuthStoreKey (enc::v2:: + enc::v1::)', () => {
  const origWindow = globalThis.window;
  const origNavigator = globalThis.navigator;
  const origLocalStorage = globalThis.localStorage;

  afterEach(() => {
    if (origWindow === undefined) delete globalThis.window;
    else globalThis.window = origWindow;
    if (origNavigator === undefined) delete globalThis.navigator;
    else Object.defineProperty(globalThis, 'navigator', { value: origNavigator, configurable: true, writable: true });
    if (origLocalStorage === undefined) delete globalThis.localStorage;
    else globalThis.localStorage = origLocalStorage;
  });

  it('decodes enc::v2:: cli-proxy-auth without depending on navigator.userAgent', () => {
    globalThis.window = { location: { host: '127.0.0.1:8317' } };
    Object.defineProperty(globalThis, 'navigator', {
      value: { userAgent: 'ChangedBrowserUA/99.0' },
      configurable: true,
      writable: true,
    });

    const json = JSON.stringify({
      state: {
        apiBase: 'http://127.0.0.1:8317',
        managementKey: 'secret-v2-key',
        rememberPassword: true,
      },
      version: 0,
    });
    const encoded = makeObfuscatedPayload(json, {
      version: 'v2',
      host: '127.0.0.1:8317',
      ua: 'OriginalUA/1.0',
    });

    expect(isCurrentObfuscated(encoded)).toBe(true);
    expect(isObfuscated(encoded)).toBe(true);
    expect(JSON.parse(deobfuscateData(encoded)).state.managementKey).toBe('secret-v2-key');

    const storage = createMemoryStorage({ [MC_STORAGE_KEY_AUTH]: encoded });
    expect(readManagementKeyFromMCStorage(storage)).toBe('secret-v2-key');
  });

  it('retains backward compatibility with legacy enc::v1:: cli-proxy-auth', () => {
    globalThis.window = { location: { host: '127.0.0.1:8317' } };
    Object.defineProperty(globalThis, 'navigator', {
      value: { userAgent: 'LegacyUA/1.0' },
      configurable: true,
      writable: true,
    });

    const json = JSON.stringify({
      state: {
        managementKey: 'secret-v1-key',
      },
    });
    const encoded = makeObfuscatedPayload(json, {
      version: 'v1',
      host: '127.0.0.1:8317',
      ua: 'LegacyUA/1.0',
    });

    expect(isCurrentObfuscated(encoded)).toBe(false);
    expect(isObfuscated(encoded)).toBe(true);

    const storage = createMemoryStorage({ [MC_STORAGE_KEY_AUTH]: encoded });
    expect(readManagementKeyFromMCStorage(storage)).toBe('secret-v1-key');
  });

  it('rejects undecoded enc::v2:: and enc::v1:: strings in parent runtime store', () => {
    const emptyStorage = createMemoryStorage();
    globalThis.localStorage = emptyStorage;

    const parentWindow = {
      localStorage: emptyStorage,
      __CPA_AUTH_STORE__: {
        getState: () => ({ managementKey: 'enc::v2::aaaa' }),
      },
    };
    globalThis.window = {
      location: { host: '127.0.0.1:8317' },
      parent: parentWindow,
    };
    expect(readCPAAuthStoreKey()).toBe('');

    parentWindow.__CPA_AUTH_STORE__ = {
      getState: () => ({ managementKey: 'plain-runtime-key' }),
    };
    expect(readCPAAuthStoreKey()).toBe('plain-runtime-key');
  });
});
