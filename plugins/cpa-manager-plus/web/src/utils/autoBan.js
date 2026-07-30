export const AUTO_BAN_ACTIONS = ['review', 'disable', 'delete', 'cooldown_enable'];
export const AUTO_BAN_STATES = ['idle', 'flagged', 'pending_action', 'disabled', 'cooling', 'enabling', 'held', 'deleted'];

export const DEFAULT_AUTO_BAN_SETTINGS = {
  enabled: false,
  sources: { usage: true, inspection: true },
  schedulerIntervalSeconds: 30,
  defaultCodexCooldownHours: 5,
  historyRetentionDays: 90,
  dryRun: false,
};

export function autoBanPath(segment = '') {
  return `/v0/management/auto-ban${segment ? `/${String(segment).replace(/^\/+/, '')}` : ''}`;
}

export function normalizeAutoBanSettings(raw) {
  const value = raw || {};
  const sources = value.sources || {};
  return {
    ...DEFAULT_AUTO_BAN_SETTINGS,
    ...value,
    enabled: Boolean(value.enabled),
    sources: {
      usage: sources.usage !== false,
      inspection: sources.inspection !== false,
    },
    schedulerIntervalSeconds: positiveInt(value.schedulerIntervalSeconds, DEFAULT_AUTO_BAN_SETTINGS.schedulerIntervalSeconds),
    defaultCodexCooldownHours: positiveInt(value.defaultCodexCooldownHours, DEFAULT_AUTO_BAN_SETTINGS.defaultCodexCooldownHours),
    historyRetentionDays: positiveInt(value.historyRetentionDays, DEFAULT_AUTO_BAN_SETTINGS.historyRetentionDays),
    dryRun: Boolean(value.dryRun),
  };
}

export function ruleDraft(rule = {}) {
  return {
    id: rule.id || 0,
    enabled: rule.enabled !== false,
    priority: positiveInt(rule.priority, 100),
    name: String(rule.name || ''),
    providerScope: String(rule.providerScope || 'codex'),
    accountKind: String(rule.accountKind || 'oauth_auth_file'),
    statusCodes: Array.isArray(rule.matchStatusCodes) ? rule.matchStatusCodes.join(', ') : String(rule.statusCodes || ''),
    errorKinds: Array.isArray(rule.matchErrorKinds) ? rule.matchErrorKinds.join(', ') : String(rule.errorKinds || ''),
    thresholdMode: rule.thresholdMode === 'total' ? 'total' : 'consecutive',
    thresholdCount: positiveInt(rule.thresholdCount, 1),
    windowMinutes: rule.windowMs ? Math.max(1, Math.round(Number(rule.windowMs) / 60_000)) : '',
    action: AUTO_BAN_ACTIONS.includes(rule.action) ? rule.action : 'review',
    cooldownHours: rule.cooldownMs ? Math.max(1, Math.round(Number(rule.cooldownMs) / 3_600_000)) : '',
    cooldownSource: rule.cooldownSource || 'header_or_default',
    respectHostCooldown: Boolean(rule.respectHostCooldown),
    maxActionsPerDay: rule.maxActionsPerDay || '',
  };
}

export function normalizeAutoBanRule(draft) {
  const statusCodes = parseStatusCodes(draft.statusCodes);
  const errorKinds = parseTokens(draft.errorKinds);
  const rule = {
    id: Number(draft.id) || 0,
    enabled: Boolean(draft.enabled),
    priority: positiveInt(draft.priority, 100),
    name: String(draft.name || '').trim(),
    providerScope: String(draft.providerScope || '').trim().toLowerCase(),
    accountKind: String(draft.accountKind || 'any').trim(),
    matchStatusCodes: statusCodes,
    matchErrorKinds: errorKinds,
    matchBodySubstrings: [],
    sourceMask: 3,
    thresholdMode: draft.thresholdMode === 'total' ? 'total' : 'consecutive',
    thresholdCount: positiveInt(draft.thresholdCount, 1),
    windowMs: nullableDuration(draft.windowMinutes, 60_000),
    successResetsConsecutive: true,
    action: String(draft.action || 'review'),
    cooldownMs: nullableDuration(draft.cooldownHours, 3_600_000),
    cooldownSource: String(draft.cooldownSource || 'header_or_default'),
    respectHostCooldown: Boolean(draft.respectHostCooldown),
    maxActionsPerDay: nullablePositiveInt(draft.maxActionsPerDay),
  };
  return rule;
}

export function validateAutoBanRule(rule) {
  const errors = {};
  if (!rule.name) errors.name = 'name';
  if (!rule.providerScope) errors.providerScope = 'provider';
  if (!AUTO_BAN_ACTIONS.includes(rule.action)) errors.action = 'action';
  if (!['consecutive', 'total'].includes(rule.thresholdMode) || rule.thresholdCount < 1) errors.threshold = 'threshold';
  if (!rule.matchStatusCodes.length && !rule.matchErrorKinds.length && !rule.matchBodySubstrings.length) errors.match = 'match';
  if (rule.action === 'cooldown_enable' && rule.cooldownSource === 'fixed' && !rule.cooldownMs) errors.cooldown = 'cooldown';
  if (rule.action === 'delete' && !rule.maxActionsPerDay) errors.maxActionsPerDay = 'delete_cap';
  return errors;
}

export function parseStatusCodes(value) {
  return [...new Set(String(value || '').split(/[\s,，；;]+/).map(Number).filter(code => Number.isInteger(code) && code >= 100 && code <= 599))];
}

export function parseTokens(value) {
  return [...new Set(String(value || '').split(/[\s,，；;]+/).map(item => item.trim()).filter(Boolean))];
}

export function formatCooldownUntil(ms, now = Date.now()) {
  const value = Number(ms);
  if (!Number.isFinite(value) || value <= 0) return '';
  const seconds = Math.max(0, Math.ceil((value - now) / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainSeconds = seconds % 60;
  if (hours) return `${hours}h ${minutes}m`;
  if (minutes) return `${minutes}m`;
  return `${remainSeconds}s`;
}

function positiveInt(value, fallback) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function nullablePositiveInt(value) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
}

function nullableDuration(value, multiplier) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.round(parsed * multiplier) : null;
}
