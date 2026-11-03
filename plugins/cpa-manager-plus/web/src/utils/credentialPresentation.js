/**
 * Presentation helpers for the OAuth credentials tab.
 * Visual/wording patterns adapted from CPA-Manager-Plus Accounts list + Quota drawer (MIT, Seakee).
 */

import { EMPTY_VALUE } from './localeFormat.js';

const COMPACT_UNITS = [
  { threshold: 1e9, suffix: 'B' },
  { threshold: 1e6, suffix: 'M' },
  { threshold: 1e3, suffix: 'K' },
];

/** CPA auth-manager statuses that mean the file is loaded / ready (not a failure). */
const HEALTHY_AUTH_STATUSES = new Set(['available', 'ok', 'enabled', 'active', 'ready', 'healthy']);

/** Probe actions that indicate the credential needs attention after enrichment. */
const ATTENTION_PROBE_ACTIONS = new Set(['review', 'reauth', 'disable']);

/** Probe errorKind values that should never look "healthy active". */
const FAILURE_PROBE_KINDS = new Set([
  'auth_invalid',
  'needs_review',
  'probe_failed',
  'network',
  'timeout',
  'missing_auth_index',
  'unsupported_provider',
]);

export function formatCompactNumber(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  const abs = Math.abs(n);
  for (const unit of COMPACT_UNITS) {
    if (abs >= unit.threshold) {
      const scaled = n / unit.threshold;
      const digits = Math.abs(scaled) >= 100 ? 0 : 1;
      return `${scaled.toFixed(digits)}${unit.suffix}`;
    }
  }
  if (Number.isInteger(n)) return String(n);
  return n.toFixed(Math.abs(n) >= 10 ? 1 : 2);
}

/** Compact USD like $205.88 / $1.2K. Never invents $0 for incomplete cost. */
export function formatCompactUsd(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  const abs = Math.abs(n);
  if (abs >= 1e3) {
    const scaled = n / 1e3;
    return `$${scaled.toFixed(Math.abs(scaled) >= 100 ? 0 : 1)}K`;
  }
  return `$${n.toFixed(2)}`;
}

/**
 * Cost text that refuses to show bare $0 when pricing is incomplete.
 * metrics: { cost, costComplete, unpricedCalls }
 */
export function formatCredentialCost(metrics, unavailableLabel = EMPTY_VALUE) {
  if (!metrics) return EMPTY_VALUE;
  const incomplete = metrics.costComplete === false || (metrics.unpricedCalls || 0) > 0;
  const cost = Number(metrics.cost);
  if (incomplete) {
    if (!Number.isFinite(cost) || cost <= 0) return unavailableLabel;
    return `~${formatCompactUsd(cost)}*`;
  }
  if (!Number.isFinite(cost)) return EMPTY_VALUE;
  return formatCompactUsd(cost);
}

export function formatSuccessRate(value, digits = 2) {
  const n = Number(value);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  const ratio = n > 1 ? n / 100 : n;
  const pct = ratio * 100;
  const d = Number.isInteger(digits) ? Math.max(0, Math.min(3, digits)) : 2;
  return `${pct.toFixed(d)}%`;
}

export function clampPercent(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return 0;
  return Math.max(0, Math.min(100, n));
}

export function quotaBarTone(remaining) {
  const n = Number(remaining);
  if (!Number.isFinite(n)) return 'neutral';
  if (n <= 0) return 'danger';
  if (n < 20) return 'danger';
  if (n < 35) return 'warn';
  return 'ok';
}

export function shortWindowLabel(label, kind) {
  const raw = String(label || kind || '').trim();
  const lower = raw.toLowerCase();
  let groupPrefix = '';
  if (lower.includes('gemini')) {
    groupPrefix = 'Gemini';
  } else if (lower.includes('claude') || lower.includes('gpt')) {
    groupPrefix = 'Claude/GPT';
  }
  let period = '';
  if (lower.includes('five') || lower.includes('5h') || lower.includes('5-hour') || lower.includes('5 hour') || lower.includes('5 小时') || lower === 'five_hour') {
    period = '5h';
  } else if (lower.includes('week') || lower.includes('周限额')) {
    period = raw.includes('周') ? '周限额' : 'Weekly';
  } else if (lower.includes('month') || lower.includes('月限额')) {
    period = raw.includes('月') ? '月限额' : 'Monthly';
  } else if (lower.includes('day') || lower.includes('daily') || lower.includes('日限额')) {
    period = raw.includes('日') ? '日限额' : 'Daily';
  }
  if (groupPrefix && period) return `${groupPrefix} · ${period}`;
  if (period) return period;
  if (raw.length <= 14) return raw;
  return raw.slice(0, 12);
}

/**
 * True when enrichment / quota probe failed or needs manual review —
 * must not present as healthy "active".
 */
export function isProbeFailure(probe) {
  if (!probe || typeof probe !== 'object') return false;
  if (probe.error) return true;
  const action = String(probe.action || '').toLowerCase();
  const kind = String(probe.errorKind || '').toLowerCase();
  // A successful quota read may request disable; that is not a transport failure.
  if (kind === 'quota_threshold' || kind === 'quota_exhausted') return false;
  if (FAILURE_PROBE_KINDS.has(kind)) return true;
  if (ATTENTION_PROBE_ACTIONS.has(action)) return true;
  // Soft: actionReason without healthy keep + no usable windows.
  if (action && action !== 'keep' && !hasQuotaWindows(probe)) return true;
  return false;
}

function hasQuotaWindows(probe) {
  const windows = probe?.quotaWindows;
  if (Array.isArray(windows)) return windows.length > 0;
  if (windows && typeof windows === 'object') return Object.keys(windows).length > 0;
  return false;
}

/** Human-readable probe failure detail for notices (never empty when isProbeFailure). */
export function probeFailureMessage(probe) {
  if (!probe) return '';
  return String(
    probe.error
    || probe.errorDetail
    || probe.actionReason
    || probe.actionError
    || ''
  ).trim();
}

/**
 * Localize raw CPA auth status codes for zh-CN (and other) UI chrome.
 * Provider codes (CODEX/XAI) stay as-is elsewhere; this is for status chips/fields.
 */
export function localizeAuthStatus(status, t) {
  const raw = String(status || '').trim();
  if (!raw) return '';
  const key = raw.toLowerCase();
  const known = {
    active: 'monitoring.credentials.status.active',
    available: 'monitoring.credentials.status.available',
    ok: 'monitoring.credentials.status.ok',
    enabled: 'monitoring.credentials.status.enabled',
    disabled: 'monitoring.authCard.disabled',
    error: 'monitoring.credentials.status.error',
    unavailable: 'monitoring.credentials.status.unavailable',
    expired: 'monitoring.credentials.status.expired',
    pending: 'monitoring.credentials.status.pending',
  };
  const i18nKey = known[key];
  if (i18nKey) {
    const label = t(i18nKey);
    // vue-i18n returns the key itself when missing — fall back to attention wording.
    if (label && label !== i18nKey) return label;
  }
  // Never surface raw English codes in localized UI.
  return t('monitoring.credentials.availability.attention');
}

/**
 * Availability presentation. Cooldown when a window is exhausted (remaining <= 0).
 * Probe / enrichment failure maps to attention (需关注 / 探测失败) — never raw "active".
 */
export function resolveAvailability(cred, probe, t) {
  if (cred?.disabled) {
    return { label: t('monitoring.authCard.disabled'), tone: 'off', bucket: 'disabled', severity: 'disabled' };
  }

  // Probe/enrichment failure must drive availability away from looking healthy.
  if (isProbeFailure(probe)) {
    return {
      label: t('monitoring.credentials.availability.probeFailed'),
      tone: 'warn',
      bucket: 'attention',
      severity: 'warning',
    };
  }

  const windows = cred?.quotaWindows || [];
  const exhausted = windows.find((w) => Number.isFinite(w.remainingPercent) && w.remainingPercent <= 0);
  if (exhausted) {
    const short = shortWindowLabel(exhausted.label, exhausted.kind);
    return {
      label: t('monitoring.credentials.availability.cooldown', { window: short }),
      tone: 'cooldown',
      // Cooldown (window exhausted / waiting reset) counts under attention (需关注),
      // not quota_risk — quota_risk is low remaining / near limit only.
      bucket: 'attention',
      severity: 'cooldown',
    };
  }
  // Low remaining (≤10%) is quota risk — not "exhausted". Exhausted (≤0) already maps to cooldown above.
  const risky = windows.find((w) => Number.isFinite(w.remainingPercent) && w.remainingPercent <= 10);
  if (risky) {
    return {
      label: t('monitoring.credentials.availability.critical', { window: shortWindowLabel(risky.label, risky.kind) }),
      tone: 'warn',
      bucket: 'quota_risk',
      severity: 'critical',
    };
  }
  const low = windows.find((w) => Number.isFinite(w.remainingPercent) && w.remainingPercent <= 35);
  if (low) {
    return {
      label: t('monitoring.credentials.availability.low', { window: shortWindowLabel(low.label, low.kind) }),
      tone: 'warn',
      bucket: 'quota_risk',
      severity: 'warning',
    };
  }

  const status = String(cred?.status || probe?.status || '').toLowerCase().trim();
  if (status && !HEALTHY_AUTH_STATUSES.has(status)) {
    return {
      label: localizeAuthStatus(cred?.status || status, t),
      tone: 'warn',
      bucket: 'attention',
      severity: 'warning',
    };
  }
  if (cred?.unavailable || cred?.statusMessage) {
    return {
      label: cred.statusMessage || t('monitoring.credentials.availability.attention'),
      tone: 'warn',
      bucket: 'attention',
      severity: 'warning',
    };
  }
  if (!probe && !HEALTHY_AUTH_STATUSES.has(status) && !windows.length) {
    return { label: t('monitoring.credentials.status.pending'), tone: 'warn', bucket: 'attention', severity: 'warning' };
  }
  return {
    label: t('monitoring.credentials.availability.available'),
    tone: 'ok',
    bucket: 'available',
    severity: 'ok',
  };
}

export function maskEmail(value) {
  const email = String(value || '').trim();
  const at = email.indexOf('@');
  if (at < 1) return email;
  const local = email.slice(0, at);
  const domain = email.slice(at);
  if (local.length <= 3) return `${local[0] || ''}***${domain}`;
  return `${local.slice(0, 3)}***${domain}`;
}

export function formatWindowRange(fromMs, toMs, formatFn) {
  if (!fromMs || !toMs || fromMs >= toMs) return '';
  return `${formatFn(fromMs)} — ${formatFn(toMs)}`;
}

export function formatPlanType(value, provider = '') {
  const raw = String(value || '').trim();
  if (!raw) return '';
  const lower = raw.toLowerCase();
  const prov = String(provider || '').trim().toLowerCase();
  if (prov === 'codex' && lower === 'pro') return 'Pro 200';
  const labels = {
    plan_max20: 'Max 20x',
    plan_max5: 'Max 5x',
    plan_max: 'Max',
    plan_pro: 'Pro',
    plan_team: 'Team',
    plan_free: 'Free',
    plan_plus: 'Plus',
    promax: 'Pro 500',
    pro_max: 'Pro 500',
    'pro-max': 'Pro 500',
    prolite: 'Pro 100',
    pro_lite: 'Pro 100',
    'pro-lite': 'Pro 100',
    pro: 'Pro',
    plus: 'Plus',
    team: 'Team',
    business: 'Business',
    self_serve_business_prolite: 'Business Premium',
    edu: 'Edu',
    education: 'Edu',
    edu_plus: 'Edu Plus',
    edu_pro: 'Edu Pro',
    ent26: 'Enterprise',
    enterprise: 'Enterprise',
    enterprise_cbp_automation: 'Enterprise',
    enterprise_cbp_usage_based: 'Enterprise',
    go: 'Go',
    free: 'Free',
    free_workspace: 'Free',
    guest: 'Free',
    k12: 'K12',
    ultra: 'Ultra',
    'g1-ultra-tier': 'Ultra',
    'ultra-lite': 'Ultra Lite',
    ultralite: 'Ultra Lite',
    'g1-ultra-lite-tier': 'Ultra Lite',
    'g1-pro-tier': 'Pro',
    'standard-tier': 'Pro',
    'free-tier': 'Free',
    'legacy-tier': 'Free',
    supergrok: 'SuperGrok',
    'supergrok heavy': 'SuperGrok Heavy',
    supergrok_heavy: 'SuperGrok Heavy',
    'supergrok-heavy': 'SuperGrok Heavy',
    paid: 'Paid',
    allegro: 'Allegro',
    allegretto: 'Allegretto',
    moderato: 'Moderato',
    andante: 'Andante',
  };
  return labels[lower] || raw;
}

export function planLabelFrom(probe, cred) {
  const raw =
    probe?.planType ||
    probe?.quotaMetadata?.planType ||
    probe?.quotaMetadata?.planName ||
    probe?.plan ||
    cred?.planType ||
    cred?.planLabel ||
    cred?.metadata?.planType ||
    '';
  const provider = probe?.provider || probe?.quotaMetadata?.provider || cred?.provider || cred?.type || '';
  return formatPlanType(raw, provider);
}

/**
 * Remaining % label for list/drawer. When rem ≤ 0 show depleted copy (not bare "0%").
 */
export function formatRemainingPercent(remaining, depletedLabel = 'Exhausted') {
  if (remaining == null || remaining === '') return EMPTY_VALUE;
  const n = Number(remaining);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  if (n <= 0) return depletedLabel;
  return `${Math.round(n)}%`;
}
