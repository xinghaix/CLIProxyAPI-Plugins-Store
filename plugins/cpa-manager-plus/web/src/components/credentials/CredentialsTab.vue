<template>
  <div class="credentials-tab">
    <p class="muted small-text cred-scope-note">{{ t('monitoring.credentials.scopeNote') }}</p>
    <section v-if="error" class="notice error">{{ error }}</section>

    <CredentialList
      :rows="filteredRows"
      :kpi="kpi"
      :provider-chips="providerChips"
      :provider-filter="providerFilter"
      :status-filter="statusFilter"
      :search="search"
      :selected-row-key="selectedRowKey"
      :enriching-row-key="enrichingRowKey"
      :loading="loading"
      :total-count="rows.length"
      :has-active-filters="hasActiveFilters"
      :format-compact="fmtCompact"
      :format-percent="fmtPct"
      :format-cost-text="formatCostText"
      @select="openDrawer"
      @update:provider-filter="providerFilter = $event"
      @update:status-filter="statusFilter = $event"
      @update:search="search = $event"
    />

    <CredentialQuotaDrawer
      :open="drawerOpen"
      :credential="selectedRow"
      :history="selectedRow?.history || null"
      :history-from-ms="historyFromMs"
      :history-to-ms="historyToMs"
      :window-cards="selectedWindowCards"
      :probing="drawerProbing"
      :action-notice="drawerNotice"
      :time-zone="analyticsTimeZone"
      :format-compact="fmtCompact"
      :format-percent="fmtPct"
      :format-cost-text="formatCostText"
      @close="closeDrawer"
      @refresh-quota="refreshSelectedQuota"
    />
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import CredentialList from './CredentialList.vue';
import CredentialQuotaDrawer from './CredentialQuotaDrawer.vue';
import { isOAuthAuthType, providerChip } from '../../utils/providerTag.js';
import { EMPTY_VALUE, formatCompactDateTime, formatDateTime } from '../../utils/localeFormat.js';
import { formatQuotaResetRelative } from '../../utils/quotaDisplay.js';
import {
  getOrCreateQuotaRequest,
  getQuotaCacheEntry,
  quotaCacheKey,
  setQuotaCacheEntry,
} from '../../utils/quotaCache.js';
import {
  buildAccountWindowUsageTargets,
  buildCredentialHistoryTarget,
  resolveWindowUsagePresentation,
  usageItemToMetrics,
} from '../../utils/quotaWindowRanges.js';
import {
  buildRecentStatusSlots,
  formatCompactNumber,
  formatCredentialCost,
  formatSuccessRate,
  formatWindowRange,
  groupRecentEventsByCredential,
  isProbeFailure,
  maskEmail,
  planLabelFrom,
  probeFailureMessage,
  resolveAvailability,
  shortWindowLabel,
} from '../../utils/credentialPresentation.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});
const emit = defineEmits(['count']);

const { t, locale } = useI18n();

const loading = ref(false);
const error = ref('');
const rows = ref([]);
const usageByRequestKey = ref(new Map());
const recentByRowKey = ref(new Map());
const providerFilter = ref('all');
const statusFilter = ref('all');
const search = ref('');
const selectedRowKey = ref('');
const drawerOpen = ref(false);
const drawerProbing = ref(false);
const drawerNotice = ref('');
const enrichingRowKey = ref('');
const focusReturnEl = ref(null);
const historyFromMs = ref(0);
const historyToMs = ref(0);
const nowMs = ref(Date.now());
const analyticsTimeZone = ref('');

const selectedRow = computed(() => rows.value.find((r) => r.rowKey === selectedRowKey.value) || null);

const providerChips = computed(() => {
  const counts = new Map();
  for (const row of rows.value) {
    const key = row.provider || 'unknown';
    counts.set(key, (counts.get(key) || 0) + 1);
  }
  return [...counts.entries()]
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .map(([key, count]) => ({
      key,
      count,
      label: providerChip(key, 'oauth').tag || key,
    }));
});

const filteredRows = computed(() => {
  const q = search.value.trim().toLowerCase();
  return rows.value.filter((row) => {
    if (providerFilter.value !== 'all' && row.provider !== providerFilter.value) return false;
    if (statusFilter.value !== 'all' && row.statusBucket !== statusFilter.value) return false;
    if (!q) return true;
    const hay = [row.displayName, row.email, row.fileName, row.note, row.authIndex, row.provider, row.planLabel]
      .map((v) => String(v || '').toLowerCase())
      .join(' ');
    return hay.includes(q);
  });
});

const kpi = computed(() => {
  const list = filteredRows.value;
  return {
    total: list.length,
    available: list.filter((r) => r.statusBucket === 'available').length,
    attention: list.filter((r) => r.statusBucket === 'attention').length,
    quotaRisk: list.filter((r) => r.statusBucket === 'quota_risk').length,
  };
});

const hasActiveFilters = computed(() => (
  providerFilter.value !== 'all'
  || statusFilter.value !== 'all'
  || Boolean(search.value.trim())
));

const selectedWindowCards = computed(() => {
  const row = selectedRow.value;
  if (!row?.quotaWindows?.length) return [];
  return row.quotaWindows.map((definition) => {
    const presentation = resolveWindowUsagePresentation(definition, usageByRequestKey.value, row.rowKey);
    const fmt = (ms) => formatCompactDateTime(ms, locale.value, analyticsTimeZone.value || undefined);
    return {
      key: definition.key,
      kind: definition.kind,
      label: definition.label,
      remainingPercent: definition.remainingPercent,
      usedPercent: definition.usedPercent,
      resetLabel: formatResetShort(definition.resetAtMs),
      previous: presentation.previous,
      current: presentation.current,
      forecast: presentation.forecast,
      previousPeriod: presentation.previousPeriod,
      previousRangeLabel: formatWindowRange(presentation.previousFromMs, presentation.previousToMs, fmt),
      currentRangeLabel: formatWindowRange(presentation.currentFromMs, presentation.currentToMs, fmt),
    };
  });
});

function fmtCompact(value) {
  return formatCompactNumber(value);
}

function fmtPct(value) {
  return formatSuccessRate(value);
}

function formatCostText(metrics) {
  return formatCredentialCost(metrics, t('monitoring.costEstimate.estimateUnavailable'));
}

function formatReset(resetAtMs) {
  if (!resetAtMs) return '';
  const absolute = formatDateTime(resetAtMs, locale.value, analyticsTimeZone.value || undefined);
  const relative = formatQuotaResetRelative(resetAtMs, nowMs.value, locale.value);
  return relative ? `${absolute} · ${relative}` : absolute;
}

function formatResetShort(resetAtMs) {
  if (!resetAtMs) return '';
  return formatCompactDateTime(resetAtMs, locale.value, analyticsTimeZone.value || undefined);
}

function buildQuotaDisplays(row, usageMap) {
  const windows = row.quotaWindows || [];
  return windows.slice(0, 3).map((definition) => {
    const presentation = resolveWindowUsagePresentation(definition, usageMap, row.rowKey);
    const shortLabel = shortWindowLabel(definition.label, definition.kind);
    const current = presentation.current;
    const forecast = presentation.forecast;
    const parts = [];
    if (current && (current.cost > 0 || current.tokens > 0 || current.costComplete === false)) {
      parts.push(`${formatCostText(current)} / ${fmtCompact(current.tokens)}`);
    }
    if (forecast && (forecast.cost > 0 || forecast.tokens > 0 || forecast.costComplete === false || (forecast.unpricedCalls || 0) > 0)) {
      // Use cost helper so incomplete / unpriced never shows bare $0 (tilde only when complete).
      const fcCost = formatCostText(forecast);
      const fcShown = (fcCost === t('monitoring.costEstimate.estimateUnavailable') || fcCost.startsWith('~') || fcCost === '—')
        ? fcCost
        : `~${fcCost}`;
      parts.push(`${fcShown} / ${fmtCompact(forecast.tokens)}`);
    }
    const resetRel = formatQuotaResetRelative(definition.resetAtMs, nowMs.value, locale.value);
    return {
      key: definition.key,
      shortLabel,
      remainingPercent: definition.remainingPercent,
      usageLine: parts.join(' · ') || (resetRel || ''),
      title: [
        `${definition.label}: ${Math.round(definition.remainingPercent ?? 0)}%`,
        resetRel,
        parts.join(' · '),
      ].filter(Boolean).join(' · '),
    };
  });
}

async function loadCredentials() {
  if (!props.ready) return;
  loading.value = true;
  error.value = '';
  try {
    const resp = await props.proxyCall({
      method: 'GET',
      path: '/v0/management/monitoring/oauth-credentials',
    });
    const items = Array.isArray(resp?.items) ? resp.items : [];
    const oauthItems = items.filter((item) => isOAuthAuthType(item.authType));
    analyticsTimeZone.value = String(resp?.time_zone || '').trim();
    nowMs.value = Date.now();
    historyToMs.value = nowMs.value;
    historyFromMs.value = Math.max(1, nowMs.value - 90 * 24 * 3600 * 1000);

    const baseRows = oauthItems.map((item) => {
      const chip = providerChip(item.provider, item.authType);
      return {
        ...item,
        rowKey: item.rowKey || item.authIndex || item.fileName,
        maskedEmail: maskEmail(item.email || item.displayName),
        providerChip: chip,
        planLabel: '',
        availabilityLabel: t('monitoring.credentials.availability.available'),
        availabilityTone: 'ok',
        statusBucket: item.disabled ? 'disabled' : 'available',
        lastRequestLabel: EMPTY_VALUE,
        recentStatuses: Array.from({ length: 8 }, () => null),
        sparkValues: [],
        history: null,
        primaryQuota: null,
        quotaWindows: [],
        quotaDisplays: [],
        probe: null,
      };
    });
    rows.value = baseRows;
    emit('count', baseRows.length);

    await enrichRows(baseRows);
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    loading.value = false;
  }
}

async function enrichRows(baseRows) {
  const historyTargets = [];
  const windowTargets = [];
  const nextUsage = new Map(usageByRequestKey.value);
  try {

  // Probe sequentially but cache-coalesced; keep list visible with per-row enriching indicator.
  for (const row of baseRows) {
    enrichingRowKey.value = row.rowKey;
    const probe = await probeCredential(row);
    row.probe = probe;
    const { windows, targets } = buildAccountWindowUsageTargets(row, probe || {}, nowMs.value);
    row.quotaWindows = windows;
    row.planLabel = planLabelFrom(probe, row);
    const availability = resolveAvailability(row, probe, t);
    row.availabilityLabel = availability.label;
    row.availabilityTone = availability.tone;
    row.statusBucket = availability.bucket;
    row.primaryQuota = windows[0]
      ? { label: windows[0].label, remainingPercent: windows[0].remainingPercent }
      : null;
    windowTargets.push(...targets.map(({ definition, ...target }) => target));
    historyTargets.push(buildCredentialHistoryTarget(row, nowMs.value, 90));
  }
  enrichingRowKey.value = '';

  const batch = [...historyTargets, ...windowTargets];
  if (batch.length) {
    for (let i = 0; i < batch.length; i += 80) {
      const chunk = batch.slice(i, i + 80);
      try {
        const resp = await props.proxyCall({
          method: 'POST',
          path: '/v0/management/monitoring/account-window-usage',
          body: { windows: chunk },
        });
        for (const item of resp?.items || []) {
          if (item?.request_key) nextUsage.set(item.request_key, item);
        }
      } catch (err) {
        console.warn('account-window-usage failed', err);
        error.value = t('monitoring.credentials.usageFailed', { error: err?.message || String(err) });
      }
    }
  }

  usageByRequestKey.value = nextUsage;

  // One analytics pass for recent request status bars (credential-scoped grouping).
  try {
    const lookbackMs = nowMs.value - 14 * 24 * 3600 * 1000;
    const analytics = await props.proxyCall({
      method: 'POST',
      path: '/v0/management/monitoring/analytics',
      body: {
        from_ms: Math.max(1, lookbackMs),
        to_ms: nowMs.value,
        now_ms: nowMs.value,
        time_zone: analyticsTimeZone.value || undefined,
        include: {
          events_page: { limit: 2500 },
          summary: false,
          granularity: 'day',
        },
      },
    });
    const events = analytics?.events?.items || [];
    recentByRowKey.value = groupRecentEventsByCredential(events, baseRows);
  } catch (err) {
    console.warn('recent events for credentials failed', err);
    recentByRowKey.value = new Map();
    // Soft: keep list usable; prefer not to overwrite a harder usage failure.
    if (!error.value) {
      error.value = t('monitoring.credentials.recentFailed', { error: err?.message || String(err) });
    }
  }

  const enriched = baseRows.map((row) => {
    const historyItem = nextUsage.get(`${row.rowKey}\0history\0current`);
    const history = usageItemToMetrics(historyItem);
    const lastSeen = history?.lastSeenMs;
    const recentEvents = recentByRowKey.value.get(row.rowKey) || [];
    const recentStatuses = buildRecentStatusSlots(recentEvents, 8);
    if (!lastSeen && recentEvents[0]?.timestamp_ms) {
      // fall through to recent event time below
    }
    const lastMs = lastSeen || recentEvents[0]?.timestamp_ms || null;
    return {
      ...row,
      history,
      lastRequestLabel: lastMs
        ? formatCompactDateTime(lastMs, locale.value, analyticsTimeZone.value || undefined)
        : EMPTY_VALUE,
      recentStatuses,
      quotaDisplays: buildQuotaDisplays(row, nextUsage),
    };
  });

  // Prefer history first_seen for drawer stats range when available.
  const firstSeens = enriched
    .map((r) => {
      const item = nextUsage.get(`${r.rowKey}\0history\0current`);
      return item?.matched ? Number(item.from_ms) : null;
    })
    .filter((v) => Number.isFinite(v) && v > 0);
  if (firstSeens.length) {
    historyFromMs.value = Math.min(...firstSeens);
  }

  rows.value = enriched;
  emit('count', enriched.length);
  } finally {
    enrichingRowKey.value = '';
  }
}

async function probeCredential(row, { force = false } = {}) {
  const cacheRow = {
    auth_index: row.authIndex,
    auth_id: row.authId,
    auth_provider_snapshot: row.provider,
    provider: row.provider,
    source: row.fileName,
    file_name: row.fileName,
    auth_type: row.authType,
  };
  const key = quotaCacheKey(cacheRow);
  if (!force) {
    const cached = getQuotaCacheEntry(key);
    if (cached?.result) return cached.result;
  }
  try {
    const result = await getOrCreateQuotaRequest(key, async () => props.proxyCall({
      method: 'POST',
      path: '/v0/management/account-quota-probe',
      body: {
        authId: row.authId || '',
        authIndex: row.authIndex || '',
        authType: row.authType || 'oauth',
        provider: row.provider || '',
        source: row.fileName || '',
        fileName: row.fileName || '',
      },
    }));
    setQuotaCacheEntry(key, result);
    return result;
  } catch (err) {
    const result = { actionReason: err?.message || String(err), error: err?.message || String(err) };
    setQuotaCacheEntry(key, result);
    return result;
  }
}

function openDrawer(row, event) {
  const target = event?.currentTarget;
  focusReturnEl.value = (target && typeof target.focus === 'function')
    ? target
    : (document.activeElement instanceof HTMLElement ? document.activeElement : null);
  selectedRowKey.value = row.rowKey;
  drawerNotice.value = '';
  drawerOpen.value = true;
}

function closeDrawer() {
  drawerOpen.value = false;
  drawerNotice.value = '';
  const el = focusReturnEl.value;
  focusReturnEl.value = null;
  const rowKey = selectedRowKey.value;
  requestAnimationFrame(() => {
    if (el && typeof el.focus === 'function' && el.isConnected) {
      try { el.focus(); return; } catch { /* ignore */ }
    }
    // Fallback: visible row/card matching the closed credential.
    if (!rowKey || typeof document === 'undefined') return;
    const nodes = document.querySelectorAll(`[data-cred-row-key="${CSS.escape(rowKey)}"]`);
    for (const node of nodes) {
      if (!(node instanceof HTMLElement)) continue;
      const style = window.getComputedStyle(node);
      if (style.display === 'none' || style.visibility === 'hidden') continue;
      try { node.focus(); return; } catch { /* ignore */ }
    }
  });
}

async function refreshSelectedQuota() {
  const row = selectedRow.value;
  if (!row) return;
  drawerProbing.value = true;
  drawerNotice.value = '';
  try {
    nowMs.value = Date.now();
    const probe = await probeCredential(row, { force: true });
    row.probe = probe;
    if (isProbeFailure(probe)) {
      const detail = probeFailureMessage(probe) || t('monitoring.credentials.availability.probeFailed');
      drawerNotice.value = t('monitoring.credentials.refreshFailed', { error: detail });
    }
    const { windows, targets } = buildAccountWindowUsageTargets(row, probe || {}, nowMs.value);
    row.quotaWindows = windows;
    row.planLabel = planLabelFrom(probe, row);
    const availability = resolveAvailability(row, probe, t);
    row.availabilityLabel = availability.label;
    row.availabilityTone = availability.tone;
    row.statusBucket = availability.bucket;
    row.primaryQuota = windows[0]
      ? { label: windows[0].label, remainingPercent: windows[0].remainingPercent }
      : null;
    const payloadTargets = targets.map(({ definition, ...target }) => target);
    if (payloadTargets.length) {
      try {
        const resp = await props.proxyCall({
          method: 'POST',
          path: '/v0/management/monitoring/account-window-usage',
          body: { windows: payloadTargets },
        });
        const next = new Map(usageByRequestKey.value);
        for (const item of resp?.items || []) {
          if (item?.request_key) next.set(item.request_key, item);
        }
        usageByRequestKey.value = next;
        row.quotaDisplays = buildQuotaDisplays(row, next);
      } catch (err) {
        drawerNotice.value = t('monitoring.credentials.usageFailed', { error: err?.message || String(err) });
      }
    }
    rows.value = rows.value.map((r) => (r.rowKey === row.rowKey ? { ...row } : r));
  } catch (err) {
    drawerNotice.value = t('monitoring.credentials.refreshFailed', { error: err?.message || String(err) });
  } finally {
    drawerProbing.value = false;
  }
}

watch(() => props.ready, (ready) => {
  if (ready) loadCredentials();
});

onMounted(() => {
  if (props.ready) loadCredentials();
});

defineExpose({
  refresh: loadCredentials,
  filteredCount: () => filteredRows.value.length,
});
</script>
