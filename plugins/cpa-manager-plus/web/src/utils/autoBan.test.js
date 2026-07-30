import { describe, expect, it } from 'vitest';
import { normalizeAutoBanRule, normalizeAutoBanSettings, parseStatusCodes, ruleDraft, validateAutoBanRule } from './autoBan.js';

describe('autoBan helpers', () => {
  it('normalizes safe settings defaults', () => {
    const settings = normalizeAutoBanSettings({ enabled: true, sources: { usage: false }, defaultCodexCooldownHours: 6 });
    expect(settings.enabled).toBe(true);
    expect(settings.sources).toEqual({ usage: false, inspection: true });
    expect(settings.defaultCodexCooldownHours).toBe(6);
    expect(settings.schedulerIntervalSeconds).toBe(30);
  });

  it('parses unique HTTP status codes', () => {
    expect(parseStatusCodes('429, 401；429 bad 600')).toEqual([429, 401]);
  });

  it('requires a daily cap for automatic deletion', () => {
    const rule = normalizeAutoBanRule({ ...ruleDraft(), name: 'delete invalid', providerScope: 'codex', statusCodes: '401', action: 'delete' });
    expect(validateAutoBanRule(rule)).toMatchObject({ maxActionsPerDay: 'delete_cap' });
    rule.maxActionsPerDay = 2;
    expect(validateAutoBanRule(rule)).toEqual({});
  });

  it('uses a fixed cooldown duration when configured', () => {
    const rule = normalizeAutoBanRule({ ...ruleDraft(), name: 'rate limit', providerScope: 'codex', statusCodes: '429', action: 'cooldown_enable', cooldownSource: 'fixed', cooldownHours: 5 });
    expect(rule.cooldownMs).toBe(18_000_000);
    expect(validateAutoBanRule(rule)).toEqual({});
  });
});
