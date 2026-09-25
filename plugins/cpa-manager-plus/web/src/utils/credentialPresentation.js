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
 * Availability presentation. Cooldown when a window is exhausted (remaining <= 0).
 */
export function resolveAvailability(cred, probe, t) {
  if (cred?.disabled) {
    return { label: t('monitoring.authCard.disabled'), tone: 'off', bucket: 'disabled', severity: 'disabled' };
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
  const status = String(cred?.status || probe?.status || '').toLowerCase();
  if (status && status !== 'available' && status !== 'ok' && status !== 'enabled') {
    return { label: cred.status || status, tone: 'warn', bucket: 'attention', severity: 'warning' };
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
