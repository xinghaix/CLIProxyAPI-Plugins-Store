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
  if (value == null || value === '') return null;
  const n = Number(value);
  if (!Number.isFinite(n)) return null;
  return Math.max(0, Math.min(100, n));
}

function inferKind(window) {
  const haystack = [window?.kind, window?.id, window?.bucketLabel, window?.label, window?.window]
    .filter(Boolean)
    .join(' ')
    .toLowerCase();
  if (haystack.includes('five') || haystack.includes('5h') || haystack.includes('5-hour') || haystack.includes('5 hour') || haystack.includes('5 小时')) return 'five_hour';
  if (haystack.includes('week') || haystack.includes('seven_day') || haystack.includes('seven-day') || haystack.includes('7-day') || haystack.includes('周限额')) return 'weekly';
  if (haystack.includes('month') || haystack.includes('月限额')) return 'monthly';
  if (haystack.includes('day') || haystack.includes('daily') || haystack.includes('日限额')) return 'daily';
  const raw = String(window?.kind || window?.id || window?.label || '').toLowerCase();
  return raw || 'unknown';
}

function resolveAntigravityBucketMeta(window, provider = '') {
  const isAg = String(provider || '').toLowerCase() === 'antigravity' || window?.kind === 'antigravity_bucket' || Boolean(window?.groupId || window?.groupLabel);
  if (!isAg) {
    return {
      groupId: window?.groupId ? String(window.groupId) : '',
      groupLabel: window?.groupLabel ? String(window.groupLabel) : '',
      bucketLabel: window?.bucketLabel ? String(window.bucketLabel) : '',
    };
  }
  const rawId = String(window?.id || window?.key || '').toLowerCase();
  let groupId = String(window?.groupId || '').trim();
  let groupLabel = String(window?.groupLabel || '').trim();
  if (!groupId) {
    if (rawId.startsWith('gemini-models')) groupId = 'gemini-models';
    else if (rawId.startsWith('claude-and-gpt-models') || rawId.startsWith('claude-gpt-models')) groupId = 'claude-and-gpt-models';
  }
  const normGroup = (groupLabel || groupId).toLowerCase().replace(/[\s_]+/g, '-');
  if (normGroup === 'gemini-models' || normGroup === 'group-gemini-models') {
    groupLabel = 'Gemini 模型';
  } else if (normGroup === 'claude-and-gpt-models' || normGroup === 'claude-gpt-models' || normGroup === 'group-claude-gpt-models') {
    groupLabel = 'Claude 和 GPT 模型';
  }
  const rawBucket = String(window?.bucketLabel || window?.label || window?.window || '').trim();
  const lowerBucket = rawBucket.toLowerCase();
  let bucketLabel = rawBucket;
  if (lowerBucket.includes('five') || lowerBucket.includes('5h') || lowerBucket.includes('5-hour') || lowerBucket.includes('5 hour') || lowerBucket.includes('5 小时')) {
    bucketLabel = '5 小时限额';
  } else if (lowerBucket.includes('week') || lowerBucket.includes('周限额')) {
    bucketLabel = '周限额';
  } else if (lowerBucket.includes('day') || lowerBucket.includes('daily') || lowerBucket.includes('日限额')) {
    bucketLabel = '日限额';
  } else if (lowerBucket.includes('month') || lowerBucket.includes('月限额')) {
    bucketLabel = '月限额';
  }
  return { groupId, groupLabel, bucketLabel };
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
export function normalizeCredentialQuotaWindow(window, index = 0, nowMs = Date.now(), provider = '') {
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
  const rawScope = typeof window?.modelScope === 'string' ? window.modelScope.trim() : '';
  const resolvedScope = rawScope === 'model' ? 'model' : 'account';
  const { groupId, groupLabel, bucketLabel } = resolveAntigravityBucketMeta(window, provider);
  let baseKey = String(window?.id || window?.key || `${kind}-${index}`);
  if (groupId && !baseKey.toLowerCase().startsWith(`${groupId.toLowerCase()}-`)) {
    baseKey = `${groupId}-${baseKey}`;
  }
  let resolvedLabel = window?.label || bucketLabel || window?.id || kind;
  if (groupLabel && bucketLabel && !String(resolvedLabel).includes(groupLabel)) {
    resolvedLabel = `${groupLabel} · ${bucketLabel}`;
  }
  return {
    key: baseKey,
    providerWindowId: String(window?.id || window?.key || baseKey),
    label: resolvedLabel,
    groupId,
    groupLabel,
    bucketLabel: bucketLabel || resolvedLabel,
    models: Array.isArray(window?.models) ? window.models : [],
    kind,
    windowMode,
    usedPercent,
    remainingPercent,
    durationSeconds,
    resetAtMs,
    cycleStartMs,
    cycleEndMs,
    stale,
    sampledAtMs: nowMs,
    modelScope: resolvedScope,
    modelScopeModels: Array.isArray(window?.modelScopeModels) ? window.modelScopeModels : [],
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
      { period: 'current', fromMs: Math.round(nowMs - durationMs), toMs: Math.round(nowMs) },
      { period: 'previous_equal_range', fromMs: Math.round(nowMs - 2 * durationMs), toMs: Math.round(nowMs - durationMs) },
    ].filter((r) => r.fromMs > 0 && r.fromMs < r.toMs);
  }

  if (definition.windowMode !== 'fixed' && definition.windowMode !== 'calendar') return [];
  if (definition.stale || definition.cycleStartMs == null || definition.cycleEndMs == null || definition.cycleEndMs <= nowMs) return [];

  const currentEnd = Math.round(Math.min(nowMs, definition.cycleEndMs));
  const cycleStart = Math.round(definition.cycleStartMs);
  const ranges = [];
  const currentFrom = cycleStart < currentEnd
    ? cycleStart
    : Math.max(1, currentEnd - 1);
  if (currentFrom < currentEnd) {
    ranges.push({ period: 'current', fromMs: currentFrom, toMs: currentEnd });
  }
  ranges.push({
    period: 'previous',
    fromMs: Math.round(cycleStart - durationMs),
    toMs: cycleStart,
  });
  return ranges.filter((r) => Number.isFinite(r.fromMs) && Number.isFinite(r.toMs) && r.fromMs > 0 && r.fromMs < r.toMs);
}

export function credentialQuotaWindowsFromProbe(probeResult, nowMs = Date.now(), provider = '') {
  const resolvedProvider = provider || probeResult?.provider || probeResult?.quotaMetadata?.provider || '';
  const rawWindows = Array.isArray(probeResult?.quotaWindows)
    ? probeResult.quotaWindows
    : (Array.isArray(probeResult?.quotaMetadata?.windows) ? probeResult.quotaMetadata.windows : []);
  return rawWindows
    .map((window, index) => normalizeCredentialQuotaWindow(window, index, nowMs, resolvedProvider))
    .filter((window) => isIntervalWindow(window) || window.resetAtMs != null || window.usedPercent != null);
}

/**
 * Build account-window-usage targets for a credential from its probe windows.
 * Does NOT use Monitoring's global from/to.
 */
export function buildAccountWindowUsageTargets(credential, probeResult, nowMs = Date.now()) {
  const windows = credentialQuotaWindowsFromProbe(probeResult, nowMs, credential?.provider || '');
  const targets = [];
  for (const definition of windows) {
    if (!hasKnownModelScope(definition)) continue;
    const ranges = buildQuotaUsageRanges(definition, nowMs);
    for (const range of ranges) {
      const requestKey = quotaUsageRequestKey(credential.rowKey, definition, range);
      targets.push({
        request_key: requestKey,
        row_key: credential.rowKey,
        window_key: definition.kind && definition.kind !== 'unknown' ? definition.kind : definition.key,
        provider_window_id: definition.providerWindowId,
        period: range.period,
        from_ms: range.fromMs,
        to_ms: range.toMs,
        auth_index: credential.authIndex || '',
        auth_id: credential.authId || '',
        auth_file_snapshot: credential.fileName || '',
        model_scope_models: definition.modelScopeModels,
        auth_provider_snapshot: credential.provider || '',
        account_snapshot: credential.displayName || credential.email || credential.authIndex || '',
        source: credential.source || '',
        definition,
      });
    }
  }
  return { windows, targets };
}

export function usageItemToMetrics(item) {
  if (!item?.matched || item.scope_match_status !== 'complete' || item.sync_status !== 'ready') return null;
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

function hasKnownModelScope(definition) {
  if (!definition?.modelScope || definition.modelScope === 'unknown') return false;
  const models = Array.isArray(definition.modelScopeModels) ? definition.modelScopeModels : [];
  if (models.length) return models.every(model => typeof model === 'string' && model !== '' && model.trim() === model);
  return definition.modelScope === 'account';
}

function encodeKeyPart(value) {
  return encodeURIComponent(String(value ?? '')).replace(/'/g, '%27');
}

export function normalizeUsageRequestKey(rawKey) {
  if (typeof rawKey !== 'string') return '';
  return rawKey
    .replace(/&#34;|&quot;/g, '"')
    .replace(/&#39;|&apos;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');
}

function quotaUsageRequestKey(rowKey, definition, range) {
  // CPA 宿主对 schema_version < 6 的插件管理响应执行 html.EscapeString，会将双引号转义为 &#34;。
  // 因此 request_key 禁止包含 " ' < > & 等会被 HTML 转义的字符，确保前后端原样匹配。
  const models = Array.isArray(definition.modelScopeModels)
    ? definition.modelScopeModels.map(encodeKeyPart).join(',')
    : '';
  const scopeSig = [
    encodeKeyPart(definition.modelScope || ''),
    models,
    encodeKeyPart(definition.usedPercent ?? ''),
    encodeKeyPart(definition.sampledAtMs ?? ''),
  ].join(':');
  return [
    rowKey,
    encodeKeyPart(definition.key || definition.providerWindowId),
    encodeKeyPart(range.period),
    range.fromMs,
    range.toMs,
    scopeSig,
  ].join('\0');
}

export function resolveWindowUsagePresentation(definition, usageByRequestKey, credentialRowKey, nowMs = Date.now()) {
  // Match the exact query snapshot, not just a window name reused next cycle.
  const ranges = hasKnownModelScope(definition) && Number.isFinite(definition.sampledAtMs) && !definition.stale && !(definition.cycleEndMs != null && definition.cycleEndMs <= nowMs)
    ? buildQuotaUsageRanges(definition, definition.sampledAtMs)
    : [];
  const itemFor = period => {
    const range = ranges.find(range => range.period === period);
    if (!range) return null;
    const key = quotaUsageRequestKey(credentialRowKey, definition, range);
    const item = usageByRequestKey.get(key) || usageByRequestKey.get(normalizeUsageRequestKey(key));
    const itemKey = normalizeUsageRequestKey(item?.request_key);
    return (item?.request_key === key || itemKey === normalizeUsageRequestKey(key)) && item?.from_ms === range.fromMs && item?.to_ms === range.toMs ? item : null;
  };
  const currentItem = itemFor('current');
  const previousItem = itemFor('previous') || itemFor('previous_equal_range');
  const current = usageItemToMetrics(currentItem);
  const previous = usageItemToMetrics(previousItem);
  const forecast = estimateWindowUsage({ usedPercent: definition.usedPercent, current, previous });
  return {
    current,
    previous,
    forecast: forecast?.costComplete === false ? null : forecast,
    previousPeriod: previous ? previousItem.period : null,
    currentFromMs: currentItem?.from_ms ?? null,
    currentToMs: currentItem?.to_ms ?? null,
    previousFromMs: previousItem?.from_ms ?? null,
    previousToMs: previousItem?.to_ms ?? null,
  };
}
