import { describe, expect, it } from 'vitest';
import { setI18nLocale } from '../i18n/index.js';
import { PROXY, HEALTH, accountActionPath, formatHealthText, num, formatCell } from './data.js';

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
