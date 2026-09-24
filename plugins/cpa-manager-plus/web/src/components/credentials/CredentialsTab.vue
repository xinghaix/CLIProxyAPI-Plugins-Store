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
      :loading="loading"
      :total-count="rows.length"
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
import { EMPTY_VALUE, formatDateTime, formatInt } from '../../utils/localeFormat.js';
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
const drawerProbing = ref(false);
const historyFromMs = ref(0);
const historyToMs = ref(0);
const nowMs = ref(Date.now());

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
    const hay = [row.displayName, row.email, row.fileName, row.note, row.authIndex, row.provider]
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

const selectedWindowCards = computed(() => {
  const row = selectedRow.value;
  if (!row?.quotaWindows?.length) return [];
  return row.quotaWindows.map((definition) => {
    const presentation = resolveWindowUsagePresentation(definition, usageByRequestKey.value, row.rowKey);
    return {
      key: definition.key,
      label: definition.label,
      remainingPercent: definition.remainingPercent,
      usedPercent: definition.usedPercent,
      resetLabel: formatReset(definition.resetAtMs),
      previous: presentation.previous,
      current: presentation.current,
      forecast: presentation.forecast,
    };
  });
});

function fmtCompact(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  if (Math.abs(n) >= 1e9) return `${(n / 1e9).toFixed(1)}B`;
  if (Math.abs(n) >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (Math.abs(n) >= 1e3) return `${(n / 1e3).toFixed(1)}K`;
  return formatInt(n);
}

function fmtPct(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  const ratio = n > 1 ? n / 100 : n;
  return `${(ratio * 100).toFixed(ratio * 100 >= 10 ? 1 : 2)}%`;
}

function formatCostText(metrics) {
  if (!metrics) return EMPTY_VALUE;
  if (metrics.costComplete === false || (metrics.unpricedCalls || 0) > 0) {
    if (!Number.isFinite(metrics.cost) || metrics.cost <= 0) {
      return t('monitoring.costEstimate.estimateUnavailable');
    }
    return `~$${metrics.cost.toFixed(2)}*`;
  }
  if (!Number.isFinite(metrics.cost)) return EMPTY_VALUE;
  return `$${metrics.cost.toFixed(2)}`;
}

function formatReset(resetAtMs) {
  if (!resetAtMs) return '';
  const absolute = formatDateTime(resetAtMs, locale.value);
  const relative = formatQuotaResetRelative(resetAtMs, nowMs.value, locale.value);
  return relative ? `${absolute} · ${relative}` : absolute;
}

function maskEmail(value) {
  const email = String(value || '').trim();
  const at = email.indexOf('@');
  if (at < 1) return email;
  const local = email.slice(0, at);
  const domain = email.slice(at);
  if (local.length <= 3) return `${local[0] || ''}***${domain}`;
  return `${local.slice(0, 3)}***${domain}`;
}

function availabilityFor(cred, probe) {
  if (cred.disabled) {
    return { label: t('monitoring.authCard.disabled'), tone: 'off', bucket: 'disabled' };
  }
  const windows = cred.quotaWindows || [];
  const risky = windows.find((w) => Number(w.remainingPercent) <= 10);
  const low = windows.find((w) => Number(w.remainingPercent) <= 35);
  if (risky) {
    return {
      label: t('monitoring.credentials.availability.exhausted', { window: risky.label }),
      tone: 'warn',
      bucket: 'quota_risk',
    };
  }
  if (low) {
    return {
      label: t('monitoring.credentials.availability.low', { window: low.label }),
      tone: 'warn',
      bucket: 'quota_risk',
    };
  }
  const status = String(cred.status || probe?.status || '').toLowerCase();
  if (status && status !== 'available' && status !== 'ok' && status !== 'enabled') {
    return { label: cred.status || status, tone: 'warn', bucket: 'attention' };
  }
  if (cred.unavailable || cred.statusMessage) {
    return { label: cred.statusMessage || t('monitoring.credentials.availability.attention'), tone: 'warn', bucket: 'attention' };
  }
  return { label: t('monitoring.credentials.availability.available'), tone: '', bucket: 'available' };
}

function planLabelFrom(probe, cred) {
  return probe?.planType || probe?.quotaMetadata?.planType || cred.planType || '';
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
        availabilityTone: '',
        statusBucket: item.disabled ? 'disabled' : 'available',
        lastRequestLabel: EMPTY_VALUE,
        sparkValues: [],
        history: null,
        primaryQuota: null,
        quotaWindows: [],
        probe: null,
      };
    });
    rows.value = baseRows;
    emit('count', baseRows.length);

    // Enrich with per-credential probe + window usage (not parent monitoring range).
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

  for (const row of baseRows) {
    const probe = await probeCredential(row);
    row.probe = probe;
    const { windows, targets } = buildAccountWindowUsageTargets(row, probe || {}, nowMs.value);
    row.quotaWindows = windows;
    row.planLabel = planLabelFrom(probe, row);
    const availability = availabilityFor(row, probe);
    row.availabilityLabel = availability.label;
    row.availabilityTone = availability.tone;
    row.statusBucket = availability.bucket;
    row.primaryQuota = windows[0]
      ? { label: windows[0].label, remainingPercent: windows[0].remainingPercent }
      : null;
    windowTargets.push(...targets.map(({ definition, ...target }) => target));
    historyTargets.push(buildCredentialHistoryTarget(row, nowMs.value, 90));
  }

  const batch = [...historyTargets, ...windowTargets];
  if (batch.length) {
    const chunks = [];
    for (let i = 0; i < batch.length; i += 80) chunks.push(batch.slice(i, i + 80));
    for (const chunk of chunks) {
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
        // Keep list visible; drawer can still refresh.
        console.warn('account-window-usage failed', err);
      }
    }
  }

  usageByRequestKey.value = nextUsage;

  const enriched = baseRows.map((row) => {
    const historyItem = nextUsage.get(`${row.rowKey}\0history\0current`);
    const history = usageItemToMetrics(historyItem);
    const lastSeen = history?.lastSeenMs;
    return {
      ...row,
      history,
      lastRequestLabel: lastSeen ? formatDateTime(lastSeen, locale.value) : EMPTY_VALUE,
      // Lightweight placeholder spark from recent history density; real hourly spark optional later.
      sparkValues: sparkFromWindows(row, nextUsage),
    };
  });
  rows.value = enriched;
  emit('count', enriched.length);
}

function sparkFromWindows(row, usageMap) {
  // Derive a simple 12-slot spark from matched window request density when hourly API absent.
  const values = [];
  for (const window of row.quotaWindows || []) {
    const current = usageMap.get(`${row.rowKey}\0${window.providerWindowId}\0current`);
    values.push(Number(current?.total_requests) || 0);
  }
  if (values.length) return values;
  const history = usageMap.get(`${row.rowKey}\0history\0current`);
  const total = Number(history?.total_requests) || 0;
  return Array.from({ length: 12 }, (_, i) => (i === 11 ? total : Math.round(total / 12)));
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

function openDrawer(row) {
  selectedRowKey.value = row.rowKey;
  drawerOpen.value = true;
}

function closeDrawer() {
  drawerOpen.value = false;
}

async function refreshSelectedQuota() {
  const row = selectedRow.value;
  if (!row) return;
  drawerProbing.value = true;
  try {
    nowMs.value = Date.now();
    const probe = await probeCredential(row, { force: true });
    row.probe = probe;
    const { windows, targets } = buildAccountWindowUsageTargets(row, probe || {}, nowMs.value);
    row.quotaWindows = windows;
    row.planLabel = planLabelFrom(probe, row);
    const availability = availabilityFor(row, probe);
    row.availabilityLabel = availability.label;
    row.availabilityTone = availability.tone;
    row.statusBucket = availability.bucket;
    row.primaryQuota = windows[0]
      ? { label: windows[0].label, remainingPercent: windows[0].remainingPercent }
      : null;
    const payloadTargets = targets.map(({ definition, ...target }) => target);
    if (payloadTargets.length) {
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
    }
    // trigger reactivity
    rows.value = rows.value.map((r) => (r.rowKey === row.rowKey ? { ...row } : r));
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
