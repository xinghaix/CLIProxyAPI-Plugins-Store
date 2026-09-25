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
  'quota_exhausted',
  'quota_threshold',
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
  if (lower.includes('five') || lower.includes('5h') || lower.includes('5-hour') || lower === 'five_hour') {
    return '5h';
  }
  if (lower.includes('week')) return 'Weekly';
  if (lower.includes('month')) return 'Monthly';
  if (lower.includes('day') || lower.includes('daily')) return 'Daily';
  if (raw.length <= 12) return raw;
  return raw.slice(0, 10);
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
  const risky = windows.find((w) => Number.isFinite(w.remainingPercent) && w.remainingPercent <= 10);
  if (risky) {
    return {
      label: t('monitoring.credentials.availability.exhausted', { window: shortWindowLabel(risky.label, risky.kind) }),
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

/** Build up to `slotCount` status slots (oldest→newest) from recent events. */
export function buildRecentStatusSlots(events, slotCount = 8) {
  const count = Math.max(1, Math.min(12, Number(slotCount) || 8));
  const list = Array.isArray(events) ? events.slice(0, count) : [];
  const statuses = list
    .slice()
    .reverse()
    .map((ev) => {
      if (!ev) return null;
      return ev.failed ? 'fail' : 'ok';
    });
  while (statuses.length < count) statuses.unshift(null);
  return statuses.slice(-count);
}

/** Group analytics events by auth_index / source / auth_file for recent status. */
export function groupRecentEventsByCredential(events, credentials) {
  const byKey = new Map();
  const indexKeys = new Map();
  for (const cred of credentials || []) {
    const keys = [
      cred.authIndex,
      cred.rowKey,
      cred.fileName,
      cred.authId,
      cred.source,
    ]
      .map((v) => String(v || '').trim().toLowerCase())
      .filter(Boolean);
    for (const key of keys) indexKeys.set(key, cred.rowKey);
    byKey.set(cred.rowKey, []);
  }
  const sorted = [...(events || [])].sort(
    (a, b) => Number(b.timestamp_ms || 0) - Number(a.timestamp_ms || 0)
  );
  for (const ev of sorted) {
    const candidates = [ev.auth_index, ev.source, ev.auth_file, ev.auth_file_snapshot, ev.auth_id]
      .map((v) => String(v || '').trim().toLowerCase())
      .filter(Boolean);
    let rowKey = null;
    for (const c of candidates) {
      if (indexKeys.has(c)) {
        rowKey = indexKeys.get(c);
        break;
      }
    }
    if (!rowKey) continue;
    const bucket = byKey.get(rowKey);
    if (bucket && bucket.length < 12) bucket.push(ev);
  }
  return byKey;
}

export function formatWindowRange(fromMs, toMs, formatFn) {
  if (!fromMs || !toMs || fromMs >= toMs) return '';
  return `${formatFn(fromMs)} — ${formatFn(toMs)}`;
}

export function planLabelFrom(probe, cred) {
  return (
    probe?.planType ||
    probe?.quotaMetadata?.planType ||
    probe?.plan ||
    cred?.planType ||
    cred?.metadata?.planType ||
    ''
  );
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
