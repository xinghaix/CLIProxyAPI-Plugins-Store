import { describe, expect, it } from 'vitest';
import { PROXY, HEALTH, accountActionPath, formatHealthText } from './data.js';

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
  it('formats local runtime ok body', () => {
    const text = formatHealthText({
      ok: true,
      mode: 'local',
      data_dir: '/tmp/cpa-manager-plus',
      db_ok: true,
    });
    expect(text).toContain('本地 Runtime 正常');
    expect(text).toContain('local');
    expect(text).toContain('/tmp/cpa-manager-plus');
  });

  it('surfaces errors and unknown states', () => {
    expect(formatHealthText({ ok: false, error: 'db locked' })).toBe('db locked');
    expect(formatHealthText(null)).toContain('未知');
  });
});
