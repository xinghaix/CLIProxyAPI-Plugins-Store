/**
 * Derive account-window-usage ranges from a credential's own quota probe windows.
 * Independent of Monitoring's global time range. Adapted from CPA-Manager-Plus
 * accountQuotaWindowDefinitions.ts (MIT, Seakee).
 */

import { quotaResetTimestamp } from './quotaDisplay.js';
import { estimateWindowUsage } from './estimateWindowUsage.js';

const KIND_DURATION_SECONDS = {
  five_hour: 5 * 3600,
  '5h': 5 * 3600,
  '5-hour': 5 * 3600,
  daily: 24 * 3600,
  weekly: 7 * 24 * 3600,
  monthly: 30 * 24 * 3600,
};

function clampPercent(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Math.max(0, Math.min(100, n));
}

function inferKind(window) {
  const raw = String(window?.kind || window?.id || window?.label || '').toLowerCase();
  if (raw.includes('five') || raw.includes('5h') || raw.includes('5-hour') || raw.includes('5 hour')) return 'five_hour';
  if (raw.includes('week')) return 'weekly';
  if (raw.includes('month')) return 'monthly';
  if (raw.includes('day') || raw.includes('daily')) return 'daily';
  return raw || 'unknown';
}

function durationSecondsFor(window) {
  const periodHours = Number(window?.periodHours ?? window?.period_hours);
  if (Number.isFinite(periodHours) && periodHours > 0) return periodHours * 3600;
  const limitSeconds = Number(window?.limitWindowSeconds ?? window?.limit_window_seconds);
  if (Number.isFinite(limitSeconds) && limitSeconds > 0) return limitSeconds;
  const kind = inferKind(window);
  return KIND_DURATION_SECONDS[kind] || null;
}

function isIntervalWindow(window) {
  const used = Number(window?.usedPercent);
  const remaining = Number(window?.remainingPercent);
  const hasProgress = Number.isFinite(used) || Number.isFinite(remaining);
  const duration = durationSecondsFor(window);
  return hasProgress && duration != null && duration > 0;
}

/**
 * Normalize a probe quota window into a display + range-ready definition.
 */
export function normalizeCredentialQuotaWindow(window, index = 0, nowMs = Date.now()) {
  const kind = inferKind(window);
  const usedPercent = clampPercent(window?.usedPercent);
  let remainingPercent = clampPercent(window?.remainingPercent);
  if (remainingPercent == null && usedPercent != null) remainingPercent = clampPercent(100 - usedPercent);
  const durationSeconds = durationSecondsFor(window);
  const resetAtMs = quotaResetTimestamp(window?.resetAt ?? window?.reset_at ?? window?.resetText);
  let cycleEndMs = resetAtMs;
  let cycleStartMs = null;
  if (cycleEndMs != null && durationSeconds != null) {
    cycleStartMs = cycleEndMs - durationSeconds * 1000;
  }
  const windowMode = cycleStartMs != null && cycleEndMs != null ? 'fixed' : durationSeconds != null ? 'rolling' : 'unknown';
  const stale = windowMode === 'fixed' && cycleEndMs != null && cycleEndMs <= nowMs;
  return {
    key: String(window?.id || window?.key || `${kind}-${index}`),
    providerWindowId: String(window?.id || window?.key || `${kind}-${index}`),
    label: window?.label || window?.id || kind,
    kind,
    windowMode,
    usedPercent,
    remainingPercent,
    durationSeconds,
    resetAtMs,
    cycleStartMs,
    cycleEndMs,
    stale,
    raw: window,
  };
}

/**
 * Build previous/current usage ranges for one quota window definition.
 * Returns [] when boundaries cannot be derived — callers must not invent ranges.
 */
export function buildQuotaUsageRanges(definition, nowMs = Date.now()) {
  const durationSeconds = definition?.durationSeconds ?? 0;
  const durationMs = durationSeconds * 1000;
  if (!Number.isFinite(durationSeconds) || durationMs <= 0) return [];

  if (definition.windowMode === 'rolling') {
    return [
      { period: 'current', fromMs: nowMs - durationMs, toMs: nowMs },
      { period: 'previous_equal_range', fromMs: nowMs - 2 * durationMs, toMs: nowMs - durationMs },
    ].filter((r) => r.fromMs > 0 && r.fromMs < r.toMs);
  }

  if (definition.windowMode !== 'fixed' && definition.windowMode !== 'calendar') return [];
  if (definition.stale || definition.cycleStartMs == null || definition.cycleEndMs == null) return [];

  const currentEnd = Math.min(nowMs, definition.cycleEndMs);
  const ranges = [];
  if (definition.cycleStartMs < currentEnd) {
    ranges.push({ period: 'current', fromMs: definition.cycleStartMs, toMs: currentEnd });
  }
  ranges.push({
    period: 'previous',
    fromMs: definition.cycleStartMs - durationMs,
    toMs: definition.cycleStartMs,
  });
  return ranges.filter((r) => Number.isFinite(r.fromMs) && Number.isFinite(r.toMs) && r.fromMs > 0 && r.fromMs < r.toMs);
}

export function credentialQuotaWindowsFromProbe(probeResult, nowMs = Date.now()) {
  const rawWindows = Array.isArray(probeResult?.quotaWindows)
    ? probeResult.quotaWindows
    : (Array.isArray(probeResult?.quotaMetadata?.windows) ? probeResult.quotaMetadata.windows : []);
  return rawWindows
    .map((window, index) => normalizeCredentialQuotaWindow(window, index, nowMs))
    .filter((window) => isIntervalWindow(window) || window.resetAtMs != null || window.usedPercent != null);
}

/**
 * Build account-window-usage targets for a credential from its probe windows.
 * Does NOT use Monitoring's global from/to.
 */
export function buildAccountWindowUsageTargets(credential, probeResult, nowMs = Date.now()) {
  const windows = credentialQuotaWindowsFromProbe(probeResult, nowMs);
  const targets = [];
  for (const definition of windows) {
    const ranges = buildQuotaUsageRanges(definition, nowMs);
    for (const range of ranges) {
      const requestKey = `${credential.rowKey}\0${definition.providerWindowId}\0${range.period}`;
      targets.push({
        request_key: requestKey,
        row_key: credential.rowKey,
        window_key: definition.key,
        provider_window_id: definition.providerWindowId,
        period: range.period,
        from_ms: range.fromMs,
        to_ms: range.toMs,
        auth_index: credential.authIndex || '',
        auth_file_snapshot: credential.fileName || credential.authId || '',
        auth_provider_snapshot: credential.provider || '',
        account_snapshot: credential.displayName || credential.email || credential.authIndex || '',
        source: credential.fileName || credential.source || '',
        definition,
      });
    }
  }
  return { windows, targets };
}

/** Lifetime / historical range for list column — credential-scoped, not parent picker. */
export function buildCredentialHistoryTarget(credential, nowMs = Date.now(), lookbackDays = 90) {
  const fromMs = nowMs - lookbackDays * 24 * 3600 * 1000;
  return {
    request_key: `${credential.rowKey}\0history\0current`,
    row_key: credential.rowKey,
    window_key: 'history',
    provider_window_id: 'history',
    period: 'current',
    from_ms: Math.max(1, fromMs),
    to_ms: nowMs,
    auth_index: credential.authIndex || '',
    auth_file_snapshot: credential.fileName || credential.authId || '',
    auth_provider_snapshot: credential.provider || '',
    account_snapshot: credential.displayName || credential.email || credential.authIndex || '',
    source: credential.fileName || credential.source || '',
  };
}

export function usageItemToMetrics(item) {
  if (!item?.matched) return null;
  return {
    requests: Number(item.total_requests) || 0,
    tokens: Number(item.total_tokens) || 0,
    cost: Number(item.total_cost) || 0,
    successCalls: Number(item.success_calls) || 0,
    failureCalls: Number(item.failure_calls) || 0,
    successRate: item.success_rate == null ? null : Number(item.success_rate),
    costComplete: item.cost_complete !== false && !(Number(item.unpriced_calls) > 0),
    unpricedCalls: Number(item.unpriced_calls) || 0,
    lastSeenMs: item.last_seen_ms ?? null,
  };
}

export function resolveWindowUsagePresentation(definition, usageByRequestKey, credentialRowKey) {
  const currentKey = `${credentialRowKey}\0${definition.providerWindowId}\0current`;
  const previousKey = `${credentialRowKey}\0${definition.providerWindowId}\0previous`;
  const previousEqualKey = `${credentialRowKey}\0${definition.providerWindowId}\0previous_equal_range`;
  const current = usageItemToMetrics(usageByRequestKey.get(currentKey));
  const previous =
    usageItemToMetrics(usageByRequestKey.get(previousKey)) ||
    usageItemToMetrics(usageByRequestKey.get(previousEqualKey));
  const forecast = estimateWindowUsage({
    usedPercent: definition.usedPercent,
    current,
    previous,
  });
  return { current, previous, forecast };
}
