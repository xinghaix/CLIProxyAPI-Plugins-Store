<template>
  <div class="credentials-tab">
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
      :ready="ready"
      :total-count="rows.length"
      :has-active-filters="hasActiveFilters"
      :format-compact="fmtCompact"
      :format-percent="fmtPct"
      :format-cost-text="formatCostText"
      @select="(row, event, tab) => openDrawer(row, event, tab)"
      @refresh="refresh"
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
      :initial-tab="drawerInitialTab"
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import CredentialList from './CredentialList.vue';
import CredentialQuotaDrawer from './CredentialQuotaDrawer.vue';
import { isOAuthAuthType, providerChip } from '../../utils/providerTag.js';
import { EMPTY_VALUE, formatCompactDateTime } from '../../utils/localeFormat.js';
import { formatQuotaResetRelative } from '../../utils/quotaDisplay.js';
import {
  QUOTA_ERROR_COOLDOWN_MS,
  getOrCreateQuotaRequest,
  getQuotaCacheEntry,
  quotaCacheKey,
  setQuotaCacheEntry,
} from '../../utils/quotaCache.js';
import {
  buildAccountWindowUsageTargets,
  credentialQuotaWindowsFromProbe,
  resolveWindowUsagePresentation,
} from '../../utils/quotaWindowRanges.js';
import {
  formatCompactNumber,
  formatCredentialCost,
  formatSuccessRate,
  formatWindowRange,
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
const providerFilter = ref('all');
const statusFilter = ref('all');
const search = ref('');
const selectedRowKey = ref('');
const drawerOpen = ref(false);
const drawerInitialTab = ref('quota');
const drawerProbing = ref(false);
const drawerNotice = ref('');
const enrichingRowKey = ref('');
const focusReturnEl = ref(null);
const historyFromMs = ref(0);
const historyToMs = ref(0);
const nowMs = ref(Date.now());
const analyticsTimeZone = ref('');
let loadGeneration = 0;
let quotaGeneration = 0;

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

const hasActiveFilters = computed(() => (
  providerFilter.value !== 'all'
  || statusFilter.value !== 'all'
  || Boolean(search.value.trim())
));

const kpi = computed(() => {
  const all = rows.value;
  const list = filteredRows.value;
  const filtered = hasActiveFilters.value;
  return {
    total: all.length,
    available: list.filter((r) => r.statusBucket === 'available').length,
    attention: list.filter((r) => r.statusBucket === 'attention').length,
    quotaRisk: list.filter((r) => r.statusBucket === 'quota_risk').length,
    filtered,
  };
});

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
  const generation = ++loadGeneration;
  ++quotaGeneration;
  drawerProbing.value = false;
  enrichingRowKey.value = '';
  loading.value = true;
  error.value = '';
  try {
    const resp = await props.proxyCall({
      method: 'GET',
      path: '/v0/management/monitoring/oauth-credentials',
    });
    if (generation !== loadGeneration) return;
    usageByRequestKey.value = new Map();
    const items = Array.isArray(resp?.items) ? resp.items : [];
    const oauthItems = items.filter((item) => isOAuthAuthType(item.authType));
    analyticsTimeZone.value = String(resp?.time_zone || '').trim();
    nowMs.value = Date.now();
    historyToMs.value = nowMs.value;
    historyFromMs.value = Math.max(1, nowMs.value - 90 * 24 * 3600 * 1000);

    const baseRows = oauthItems.map((item) => {
      const chip = providerChip(item.provider, item.authType);
      const hist = item.history || null;
      const lastMs = hist?.lastSeenMs || null;
      const cachedProbe = getQuotaCacheEntry(credentialQuotaCacheKey(item))?.result || null;
      const quotaWindows = credentialQuotaWindowsFromProbe(cachedProbe || item, nowMs.value);
      const row = {
        ...item,
        rowKey: item.rowKey || item.authIndex || item.fileName,
        maskedEmail: maskEmail(item.email || item.displayName),
        providerChip: chip,
        lastRequestLabel: lastMs
          ? formatCompactDateTime(lastMs, locale.value, analyticsTimeZone.value || undefined)
          : EMPTY_VALUE,
        recentStatuses: Array.isArray(item.recentStatuses) && item.recentStatuses.length === 8
          ? item.recentStatuses
          : Array.from({ length: 8 }, () => null),
        sparkValues: [],
        history: hist,
      };
      applyQuotaState(row, cachedProbe, quotaWindows);
      return row;
    });

    rows.value = baseRows;
    emit('count', baseRows.length);
  } catch (err) {
    if (generation === loadGeneration) error.value = err?.message || String(err);
  } finally {
    if (generation === loadGeneration) loading.value = false;
  }
}

function credentialQuotaCacheKey(row) {
  return quotaCacheKey({
    auth_index: row.authIndex,
    auth_id: row.authId,
    auth_provider_snapshot: row.provider,
    provider: row.provider,
    source: row.source || '',
    file_name: row.fileName,
    auth_type: row.authType,
  });
}

async function probeCredential(row, { force = false } = {}) {
  const key = credentialQuotaCacheKey(row);
  if (!force) {
    const cached = getQuotaCacheEntry(key);
    if (cached?.result) return cached.result;
  }
  try {
    const { result, cooldownMs } = await getOrCreateQuotaRequest(key, async () => props.proxyCall({
      method: 'POST',
      path: '/v0/management/account-quota-probe',
      body: {
        authId: row.authId || '',
        authIndex: row.authIndex || '',
        authType: row.authType || 'oauth',
        provider: row.provider || '',
        source: row.source || '',
        fileName: row.fileName || '',
      },
    }));
    setQuotaCacheEntry(key, result, cooldownMs);
    return result;
  } catch (err) {
    const result = { actionReason: err?.message || String(err), error: err?.message || String(err) };
    setQuotaCacheEntry(key, result, QUOTA_ERROR_COOLDOWN_MS);
    return result;
  }
}

function openDrawer(row, event, preferredTab) {
  const target = event?.currentTarget;
  focusReturnEl.value = (target && typeof target.focus === 'function')
    ? target
    : (document.activeElement instanceof HTMLElement ? document.activeElement : null);
  selectedRowKey.value = row.rowKey;
  drawerNotice.value = '';
  drawerInitialTab.value = preferredTab || (row.probeFailed ? 'overview' : 'quota');
  drawerOpen.value = true;

  refreshSelectedQuota({ force: false });
}

function closeDrawer() {
  ++quotaGeneration;
  drawerProbing.value = false;
  enrichingRowKey.value = '';
  drawerOpen.value = false;
  drawerNotice.value = '';
  const el = focusReturnEl.value;
  focusReturnEl.value = null;
  const rowKey = selectedRowKey.value;
  requestAnimationFrame(() => {
    if (el && typeof el.focus === 'function' && el.isConnected) {
      try { el.focus(); return; } catch { /* ignore */ }
    }
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

function applyQuotaState(row, probe, windows) {
  const availability = resolveAvailability({ ...row, quotaWindows: windows }, probe, t);
  Object.assign(row, {
    probe,
    quotaWindows: windows,
    planLabel: planLabelFrom(probe, row),
    availabilityLabel: availability.label,
    availabilityTone: availability.tone,
    statusBucket: availability.bucket,
    probeFailed: isProbeFailure(probe),
    probeFailureSummary: isProbeFailure(probe) ? (probeFailureMessage(probe) || t('monitoring.credentials.availability.probeFailed')) : '',
    primaryQuota: windows[0] ? { label: windows[0].label, remainingPercent: windows[0].remainingPercent } : null,
  });
  row.quotaDisplays = buildQuotaDisplays(row, usageByRequestKey.value);
}

async function refreshSelectedQuota({ force = true } = {}) {
  const row = selectedRow.value;
  if (!row) return;
  const generation = ++quotaGeneration;
  const inventoryGeneration = loadGeneration;
  const isCurrent = () => generation === quotaGeneration && inventoryGeneration === loadGeneration && selectedRow.value === row;
  drawerProbing.value = true;
  enrichingRowKey.value = row.rowKey;
  drawerNotice.value = '';
  try {
    const inventoryWindows = !force && !row.probe && row.quotaWindows?.some(w => !w.stale && !(w.cycleEndMs != null && w.cycleEndMs <= Date.now()));
    const probe = inventoryWindows
      ? { quotaWindows: row.quotaWindows.map(w => w.raw || w) }
      : await probeCredential(row, { force });
    if (!isCurrent()) return;
    nowMs.value = Date.now();
    const { windows, targets } = buildAccountWindowUsageTargets(row, probe || {}, nowMs.value);
    // A failed retry must not retain totals from the preceding query snapshot.
    usageByRequestKey.value = new Map([...usageByRequestKey.value].filter(([key]) => !key.startsWith(`${row.rowKey}\0`)));
    applyQuotaState(row, inventoryWindows ? null : probe, windows);
    if (row.probeFailed) {
      drawerNotice.value = t('monitoring.credentials.refreshFailed', { error: row.probeFailureSummary });
    }
    const payloadTargets = targets.map(({ definition, ...target }) => target);
    if (payloadTargets.length) {
      try {
        const resp = await props.proxyCall({
          method: 'POST',
          path: '/v0/management/monitoring/account-window-usage',
          body: { windows: payloadTargets },
        });
        if (!isCurrent()) return;
        const expected = new Map(payloadTargets.map(target => [target.request_key, target]));
        const next = new Map(usageByRequestKey.value);
        for (const item of Array.isArray(resp?.items) ? resp.items : []) {
          const target = expected.get(item?.request_key);
          if (target && item.from_ms === target.from_ms && item.to_ms === target.to_ms && item.period === target.period) {
            next.set(item.request_key, item);
          }
        }
        usageByRequestKey.value = next;
        row.quotaDisplays = buildQuotaDisplays(row, next);
      } catch (err) {
        if (isCurrent()) drawerNotice.value = t('monitoring.credentials.usageFailed', { error: err?.message || String(err) });
      }
    }
  } catch (err) {
    if (isCurrent()) drawerNotice.value = t('monitoring.credentials.refreshFailed', { error: err?.message || String(err) });
  } finally {
    if (isCurrent()) {
      drawerProbing.value = false;
      enrichingRowKey.value = '';
    }
  }
}

onBeforeUnmount(() => { ++loadGeneration; ++quotaGeneration; });

watch(() => props.ready, (ready) => {
  if (ready) loadCredentials();
});

onMounted(() => {
  if (props.ready) loadCredentials();
});

async function refresh() {
  await loadCredentials();
}

defineExpose({
  refresh,
  filteredCount: () => filteredRows.value.length,
});
</script>
