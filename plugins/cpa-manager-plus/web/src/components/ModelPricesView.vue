<template>
  <section class="monitoring-page model-prices-page">
    <div class="card inspection-status-card model-prices-status-card">
      <div class="inspection-status-bar">
        <div class="inspection-status-info">
          <span :class="['status-badge', syncing || status.running ? '' : lastStatusTone]">
            <i aria-hidden="true"></i>{{ syncStatusText }}
          </span>
          <span :class="['status-badge', settings.enabled ? 'good' : '']">
            <i aria-hidden="true"></i>{{ settings.enabled ? t('modelPrices.autoSyncOn') : t('modelPrices.autoSyncOff') }}
          </span>
          <span class="muted small-text">
            {{ t('modelPrices.syncModels', { count: syncModelCount }) }}
            <template v-if="status.lastSyncAtMs"> · {{ t('modelPrices.lastSync', { time: formatTimestamp(status.lastSyncAtMs, localeRef) }) }}</template>
            <template v-if="lastResult?.trigger"> · {{ triggerLabel(lastResult.trigger) }}</template>
          </span>
        </div>
        <div class="config-actions-bar" style="padding:0">
          <button class="btn" type="button" @click="refresh(true)" :disabled="loading || !ready">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
          <button class="btn" type="button" @click="addManualPrice" :disabled="!ready">{{ t('modelPrices.addManual') }}</button>
          <button class="btn primary" type="button" @click="runSync" :disabled="!ready || syncing || status.running">
            {{ syncing || status.running ? t('modelPrices.syncing') : t('modelPrices.syncNow') }}
          </button>
        </div>
      </div>

      <details class="inspection-info-note">
        <summary>{{ t('modelPrices.infoTitle') }}</summary>
        <ul class="inspection-info-list">
          <li><strong>{{ t('modelPrices.info.scope') }}</strong>: {{ t('modelPrices.info.scopeText') }}</li>
          <li><strong>{{ t('modelPrices.info.dualSource') }}</strong>: {{ t('modelPrices.info.dualSourceText') }}</li>
          <li><strong>{{ t('modelPrices.info.manualProtect') }}</strong>: {{ t('modelPrices.info.manualProtectText') }}</li>
          <li><strong>{{ t('modelPrices.info.autoSync') }}</strong>: {{ t('modelPrices.info.autoSyncText') }}</li>
        </ul>
      </details>

      <div class="model-prices-settings">
        <div class="section-title">
          <h2>{{ t('modelPrices.settingsTitle') }}</h2>
          <button
            type="button"
            class="btn primary"
            :disabled="!ready || settingsSaving || !settingsDirty"
            @click="saveSettings"
          >
            {{ settingsSaving ? t('modelPrices.savingSettings') : t('modelPrices.saveSettings') }}
          </button>
        </div>
        <div class="config-form-grid model-prices-settings-grid">
          <label class="config-field config-field-toggle">
            <span class="config-field-label">{{ t('modelPrices.enableAutoSync') }}</span>
            <button
              type="button"
              :class="['toggle-switch', { on: settingsDraft.enabled }]"
              :disabled="!ready || settingsSaving || !syncApiAvailable.settings"
              @click="settingsDraft.enabled = !settingsDraft.enabled"
            >
              <span class="toggle-knob"></span>
            </button>
            <small class="muted">{{ settingsDraft.enabled ? t('common.enabled') : t('modelPrices.defaultOff') }}</small>
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.intervalHours') }}</span>
            <input
              v-model.number="settingsDraft.intervalHours"
              type="number"
              min="6"
              max="168"
              step="1"
              class="control"
              :disabled="!ready || settingsSaving || !syncApiAvailable.settings"
            />
            <small class="muted">{{ t('modelPrices.intervalHint', { min: MIN_INTERVAL_HOURS, max: MAX_INTERVAL_HOURS, default: DEFAULT_SYNC_SETTINGS.intervalHours }) }}</small>
          </label>
          <label class="config-field config-field-toggle">
            <span class="config-field-label">{{ t('modelPrices.protectManual') }}</span>
            <button
              type="button"
              :class="['toggle-switch', { on: true }]"
              disabled
              :title="t('modelPrices.protectManualTitle')"
            >
              <span class="toggle-knob"></span>
            </button>
            <small class="muted">{{ t('modelPrices.protectManualHint') }}</small>
          </label>
        </div>
        <p v-if="!syncApiAvailable.settings" class="muted small-text">
          {{ t('modelPrices.settingsApiUnavailable') }}
        </p>
        <p v-if="settingsMessage" class="notice config-save-ok" style="margin-top:8px">{{ settingsMessage }}</p>
      </div>

      <div class="inspection-summary-shell">
        <MetricGrid class="inspection-summary-grid" :cards="summaryCards" />
      </div>

      <div v-if="sourceResults.length" class="model-prices-source-results">
        <div
          v-for="src in sourceResults"
          :key="src.source"
          :class="['model-prices-source-pill', src.ok ? 'ok' : 'err']"
          :title="src.error || undefined"
        >
          <strong>{{ formatSourceLabel(src.source) }}</strong>
          <span v-if="src.ok">
            {{ t('modelPrices.sourceOk', { models: src.modelCount, matched: src.matched, applied: src.applied }) }}
            <template v-if="src.skipped"> · {{ t('modelPrices.sourceSkipped', { count: src.skipped }) }}</template>
            <template v-if="src.durationMs"> · {{ formatDurationMs(src.durationMs) }}</template>
          </span>
          <span v-else>{{ src.error ? t('modelPrices.sourceFailedWithError', { error: src.error }) : t('modelPrices.sourceFailed') }}</span>
        </div>
      </div>
      <p v-if="status.lastError || lastResult?.error" class="notice error" style="margin-top:10px">
        {{ status.lastError || lastResult.error }}
      </p>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>
    <section v-if="!ready" class="notice">{{ t('modelPrices.missingKey') }}</section>
    <section v-if="syncNotice" class="notice">{{ syncNotice }}</section>

    <DataCard
      v-if="pendingCandidates.length"
      :title="t('modelPrices.candidatesTitle')"
      :subtitle="t('modelPrices.candidatesSubtitle', { count: pendingCandidates.length })"
    >
      <div class="table-wrap monitor-table">
        <table>
          <thead>
            <tr>
              <th style="width:36px">
                <input
                  type="checkbox"
                  :checked="allCandidatesSelected"
                  @change="toggleSelectAllCandidates"
                />
              </th>
              <th>{{ t('modelPrices.columns.localModel') }}</th>
              <th>{{ t('modelPrices.columns.source') }}</th>
              <th>{{ t('modelPrices.columns.sourceModel') }}</th>
              <th>{{ t('modelPrices.columns.prompt') }}</th>
              <th>{{ t('modelPrices.columns.completion') }}</th>
              <th>{{ t('modelPrices.columns.reason') }}</th>
              <th>{{ t('modelPrices.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(c, idx) in pendingCandidates" :key="candidateKey(c, idx)">
              <td>
                <input
                  type="checkbox"
                  :checked="selectedCandidateKeys.has(candidateKey(c, idx))"
                  @change="toggleCandidate(c, idx)"
                />
              </td>
              <td><strong>{{ c.localModel }}</strong></td>
              <td>
                <span :class="['source-badge', sourceBadgeClass(c.source)]">{{ formatSourceLabel(c.source) }}</span>
              </td>
              <td class="small-text">{{ c.sourceModelId || EMPTY_VALUE }}</td>
              <td>{{ formatMoneyPer1M(c.prompt) }}</td>
              <td>{{ formatMoneyPer1M(c.completion) }}</td>
              <td class="small-text">
                {{ c.reason || 'fuzzy' }}
                <template v-if="c.score"> · {{ Math.round(c.score * 100) }}%</template>
              </td>
              <td>
                <button class="btn primary" type="button" :disabled="confirming" @click="confirmOne(c)">
                  {{ t('modelPrices.confirm') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="config-actions-bar" style="padding-top:10px">
        <button class="btn primary" type="button" :disabled="!selectedCandidateList.length || confirming" @click="confirmSelected">
          {{ confirming ? t('modelPrices.confirming') : t('modelPrices.confirmSelected', { count: selectedCandidateList.length }) }}
        </button>
        <button class="btn" type="button" :disabled="!pendingCandidates.length" @click="dismissCandidates">
          {{ t('modelPrices.dismissCandidates') }}
        </button>
      </div>
    </DataCard>

    <DataCard :title="t('modelPrices.tableTitle')" :subtitle="tableSubtitle">
      <div class="model-prices-toolbar">
        <div class="segmented-control model-prices-filters">
          <button
            v-for="f in filters"
            :key="f"
            type="button"
            :class="['segment-btn', { active: filter === f }]"
            @click="filter = f"
          >
            {{ filterLabel(f) }}
            <span class="segment-count">{{ filterCounts[f] }}</span>
          </button>
        </div>
        <input
          v-model.trim="search"
          class="control wide"
          :placeholder="t('modelPrices.searchPlaceholder')"
        />
      </div>

      <div v-if="!visibleRows.length" class="empty">
        {{ emptyTableText }}
      </div>
      <div v-else class="table-wrap monitor-table">
        <table>
          <thead>
            <tr>
              <th>{{ t('modelPrices.columns.model') }}</th>
              <th>{{ t('modelPrices.columns.promptPer1M') }}</th>
              <th>{{ t('modelPrices.columns.completionPer1M') }}</th>
              <th>{{ t('modelPrices.columns.cachePer1M') }}</th>
              <th>{{ t('modelPrices.columns.cacheRead') }}</th>
              <th>{{ t('modelPrices.columns.cacheCreation') }}</th>
              <th>{{ t('modelPrices.columns.source') }}</th>
              <th>{{ t('modelPrices.columns.sourceModel') }}</th>
              <th>{{ t('modelPrices.columns.updatedAt') }}</th>
              <th>{{ t('modelPrices.columns.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in visibleRows"
              :key="row.model"
              class="clickable"
              @click="openEdit(row)"
            >
              <td>
                <div><strong>{{ row.model }}</strong></div>
                <div v-if="!row.hasPrice && row.candidateCount" class="muted small-text">{{ t('modelPrices.needsConfirm') }}</div>
                <div v-else-if="!row.hasPrice" class="muted small-text">{{ t('modelPrices.notSet') }}</div>
              </td>
              <td>{{ row.hasPrice ? formatMoneyPer1M(row.price.prompt) : EMPTY_VALUE }}</td>
              <td>{{ row.hasPrice ? formatMoneyPer1M(row.price.completion) : EMPTY_VALUE }}</td>
              <td>{{ row.hasPrice ? formatMoneyPer1M(row.price.cache) : EMPTY_VALUE }}</td>
              <td>{{ row.hasPrice ? formatMoneyPer1M(row.price.cacheRead) : EMPTY_VALUE }}</td>
              <td>{{ row.hasPrice ? formatMoneyPer1M(row.price.cacheCreation) : EMPTY_VALUE }}</td>
              <td @click.stop>
                <span v-if="row.hasPrice" :class="['source-badge', sourceBadgeClass(row.price.source)]">
                  {{ formatSourceLabel(row.price.source) }}
                </span>
                <span v-else class="muted">{{ EMPTY_VALUE }}</span>
              </td>
              <td class="small-text">{{ row.price?.sourceModelId || EMPTY_VALUE }}</td>
              <td class="small-text">{{ formatTimestamp(row.price?.updatedAtMs, localeRef) }}</td>
              <td @click.stop>
                <button class="btn" type="button" :disabled="deleting" @click="openEdit(row)">
                  {{ row.hasPrice ? t('modelPrices.edit') : t('modelPrices.add') }}
                </button>
                <button
                  v-if="row.hasPrice"
                  class="btn danger"
                  type="button"
                  :disabled="saving || deleting"
                  @click="confirmDeletePrice(row.model)"
                >{{ t('common.delete') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </DataCard>

    <div v-if="editingModel" class="drawer-backdrop" @click.self="editingModel = null">
      <div class="modal-dialog card drawer">
        <div class="drawer-head">
          <div>
            <h2>{{ editingModel.isNew ? t('modelPrices.drawerNew') : t('modelPrices.drawerEdit') }}</h2>
            <p class="muted">{{ t('modelPrices.drawerSub', { model: editingModel.model || t('modelPrices.newModel') }) }}</p>
          </div>
          <button class="btn" type="button" @click="editingModel = null">{{ t('common.close') }}</button>
        </div>
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.fields.modelName') }}</span>
            <input v-model.trim="editingModel.model" class="control" :disabled="!editingModel.isNew" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.fields.prompt') }}</span>
            <input v-model.number="editingModel.prompt" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.fields.completion') }}</span>
            <input v-model.number="editingModel.completion" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.fields.cache') }}</span>
            <input v-model.number="editingModel.cache" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.fields.cacheRead') }}</span>
            <input v-model.number="editingModel.cacheRead" type="number" step="0.0001" min="0" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('modelPrices.fields.cacheCreation') }}</span>
            <input v-model.number="editingModel.cacheCreation" type="number" step="0.0001" min="0" class="control" />
          </label>
        </div>
        <p class="muted small-text">{{ t('modelPrices.manualNote') }}</p>
        <div class="config-actions-bar">
          <button class="btn primary" type="button" @click="savePrice" :disabled="saving || deleting">
            {{ saving ? t('modelPrices.saving') : t('common.save') }}
          </button>
          <button
            v-if="!editingModel.isNew"
            class="btn danger"
            type="button"
            :disabled="saving || deleting"
            @click="confirmDeletePrice(editingModel.model)"
          >{{ deleting ? t('modelPrices.deleting') : t('modelPrices.deletePrice') }}</button>
          <button class="btn" type="button" :disabled="saving || deleting" @click="editingModel = null">{{ t('common.cancel') }}</button>
        </div>
      </div>
    </div>

    <ConfirmModal
      :open="confirmOpen"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-label="confirmOkLabel"
      :cancel-label="confirmCancelLabel"
      :variant="confirmVariant"
      @confirm="finishConfirm(true)"
      @cancel="finishConfirm(false)"
    />
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import ConfirmModal from './ConfirmModal.vue';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';
import { localeRef } from '../localeBridge.js';
import { EMPTY_VALUE } from '../utils/localeFormat.js';
import {
  DEFAULT_SYNC_SETTINGS,
  MAX_INTERVAL_HOURS,
  MIN_INTERVAL_HOURS,
  PRICE_FILTERS,
  buildConfirmBody,
  buildDeletePriceRequest,
  buildFilterCounts,
  buildManualPriceEntry,
  buildManualPutBody,
  buildModelPriceRows,
  buildSyncModels,
  extractPricesMap,
  extractUsageModels,
  filterModelPriceRows,
  formatDurationMs,
  formatMoneyPer1M,
  formatSourceLabel,
  formatTimestamp,
  normalizeSyncResult,
  normalizeSyncSettings,
  normalizeSyncStatus,
  softProxyCall,
  sourceBadgeClass,
  filterCandidatesWithExistingPrices,
  summarizeLastResult,
  validateIntervalHours,
} from '../utils/priceSync.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t } = useI18n();

const prices = ref({});
const usageModels = ref([]);
const loading = ref(false);
const saving = ref(false);
const deleting = ref(false);
const syncing = ref(false);
const confirming = ref(false);
const settingsSaving = ref(false);
const error = ref('');
const syncNotice = ref('');
const settingsMessage = ref('');
const editingModel = ref(null);
const filter = ref('all');
const search = ref('');
const filters = PRICE_FILTERS;
const confirmOpen = ref(false);
const confirmTitle = ref('');
const confirmMessage = ref('');
const confirmOkLabel = ref('');
const confirmCancelLabel = ref('');
const confirmVariant = ref('primary');
let confirmResolve = null;

const settings = ref({ ...DEFAULT_SYNC_SETTINGS });
const settingsDraft = reactive({ ...DEFAULT_SYNC_SETTINGS });
const status = ref(normalizeSyncStatus(null));
const lastResult = ref(null);
const pendingCandidates = ref([]);
const selectedCandidateKeys = ref(new Set());
const syncApiAvailable = reactive({
  usageSummary: true,
  sync: true,
  settings: true,
  status: true,
  confirm: true,
});

let pollTimer = null;

const syncModelCount = computed(() => buildSyncModels(prices.value, usageModels.value).length);

const allRows = computed(() =>
  buildModelPriceRows(prices.value, usageModels.value, pendingCandidates.value),
);

const filterCounts = computed(() => buildFilterCounts(allRows.value));

const visibleRows = computed(() =>
  filterModelPriceRows(allRows.value, filter.value, search.value).slice(0, 500),
);

const tableSubtitle = computed(() => {
  const total = allRows.value.length;
  const shown = visibleRows.value.length;
  if (shown === total) return t('modelPrices.tableSubtitleAll', { count: total });
  return t('modelPrices.tableSubtitlePartial', { shown, total });
});

const emptyTableText = computed(() => {
  if (!allRows.value.length) {
    return t('modelPrices.emptyNone');
  }
  return t('modelPrices.emptyFilter');
});

const summary = computed(() => summarizeLastResult(lastResult.value, syncModelCount.value));

const summaryCards = computed(() => [
  { label: t('modelPrices.summary.syncTargets'), value: summary.value.targetCount, sub: t('modelPrices.summary.syncTargetsSub') },
  { label: t('modelPrices.summary.autoUpdated'), value: summary.value.applied, sub: t('modelPrices.summary.autoUpdatedSub') },
  { label: t('modelPrices.summary.protectedSkipped'), value: summary.value.protectedSkipped, sub: t('modelPrices.summary.protectedSkippedSub') },
  { label: t('modelPrices.summary.pendingCandidates'), value: pendingCandidates.value.length || summary.value.candidateCount, sub: t('modelPrices.summary.pendingCandidatesSub') },
  { label: t('modelPrices.summary.unmatched'), value: summary.value.unmatched, sub: t('modelPrices.summary.unmatchedSub') },
]);

const sourceResults = computed(() => lastResult.value?.sources || []);

const syncStatusText = computed(() => {
  if (syncing.value || status.value.running) return t('modelPrices.status.running');
  if (status.value.lastError || lastResult.value?.error) return t('modelPrices.status.error');
  if (status.value.lastSuccessAtMs || lastResult.value?.finishedAtMs) return t('modelPrices.status.success');
  return t('modelPrices.status.never');
});

const lastStatusTone = computed(() => {
  if (status.value.lastError || lastResult.value?.error) return 'bad';
  if (status.value.lastSuccessAtMs || status.value.lastSyncAtMs) return 'good';
  return '';
});

const settingsDirty = computed(() => {
  return (
    settingsDraft.enabled !== settings.value.enabled
    || Number(settingsDraft.intervalHours) !== Number(settings.value.intervalHours)
  );
});

const selectedCandidateList = computed(() => {
  return pendingCandidates.value.filter((c, idx) =>
    selectedCandidateKeys.value.has(candidateKey(c, idx)),
  );
});

const allCandidatesSelected = computed(() => {
  if (!pendingCandidates.value.length) return false;
  return pendingCandidates.value.every((c, idx) =>
    selectedCandidateKeys.value.has(candidateKey(c, idx)),
  );
});

watch(
  () => props.ready,
  (ready) => {
    if (ready) refresh(true);
  },
);

onMounted(() => {
  if (props.ready) refresh(true);
});

onBeforeUnmount(() => {
  stopPolling();
});

function showConfirm({
  title,
  message = '',
  confirmLabel,
  cancelLabel,
  variant = 'primary',
} = {}) {
  confirmTitle.value = title || t('common.confirm');
  confirmMessage.value = message;
  confirmOkLabel.value = confirmLabel || t('common.confirm');
  confirmCancelLabel.value = cancelLabel || t('common.cancel');
  confirmVariant.value = variant;
  confirmOpen.value = true;
  return new Promise((resolve) => {
    confirmResolve = resolve;
  });
}

function finishConfirm(ok) {
  confirmOpen.value = false;
  const resolve = confirmResolve;
  confirmResolve = null;
  resolve?.(ok);
}

function candidateKey(c, idx) {
  return `${c.localModel}::${c.source}::${c.sourceModelId}::${idx}`;
}

function filterLabel(f) {
  switch (f) {
    case 'missing':
      return t('modelPrices.filters.missing');
    case 'saved':
      return t('modelPrices.filters.saved');
    case 'candidates':
      return t('modelPrices.filters.candidates');
    default:
      return t('modelPrices.filters.all');
  }
}

function triggerLabel(trigger) {
  if (trigger === 'auto') return t('modelPrices.trigger.auto');
  if (trigger === 'manual') return t('modelPrices.trigger.manual');
  return trigger || '';
}

function applySettings(next) {
  const normalized = normalizeSyncSettings(next);
  settings.value = normalized;
  settingsDraft.enabled = normalized.enabled;
  settingsDraft.intervalHours = normalized.intervalHours;
  settingsDraft.protectManual = normalized.protectManual;
}

function applyStatus(body, pricesMap = prices.value) {
  const normalized = normalizeSyncStatus(body);
  status.value = normalized;
  if (normalized.settings && (body?.settings || body?.Settings)) {
    applySettings(normalized.settings);
  }
  if (normalized.lastResult) {
    const filtered = filterCandidatesWithExistingPrices(
      normalized.lastResult.candidates || [],
      pricesMap,
    );
    lastResult.value = {
      ...normalized.lastResult,
      candidates: filtered,
      candidateCount: filtered.length,
    };
    pendingCandidates.value = filtered;
    selectedCandidateKeys.value = new Set();
  }
  if (normalized.running) startPolling();
  else stopPolling();
}

function applySyncResult(body, pricesMap = prices.value) {
  const result = normalizeSyncResult(body);
  if (!result) return;
  const filtered = filterCandidatesWithExistingPrices(result.candidates || [], pricesMap);
  const next = {
    ...result,
    candidates: filtered,
    candidateCount: filtered.length,
  };
  lastResult.value = next;
  pendingCandidates.value = filtered;
  selectedCandidateKeys.value = new Set();
  status.value = {
    ...status.value,
    running: false,
    lastSyncAtMs: result.finishedAtMs || result.startedAtMs || Date.now(),
    lastSuccessAtMs: result.error ? status.value.lastSuccessAtMs : (result.finishedAtMs || Date.now()),
    lastError: result.error || '',
    lastResult: next,
  };
}

async function refresh(force = false) {
  if (!props.ready) return;
  if (loading.value && !force) return;
  loading.value = true;
  error.value = '';
  syncNotice.value = '';
  try {
    const [pricesRes, usageRes, statusRes, settingsRes] = await Promise.all([
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices' }),
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices/usage-summary' }),
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices/sync-status' }),
      softProxyCall(props.proxyCall, { method: 'GET', path: '/v0/management/model-prices/sync-settings' }),
    ]);

    // Apply prices first so status candidate filtering sees current map.
    let pricesMap = prices.value;
    if (!pricesRes.ok) {
      error.value = pricesRes.error || t('modelPrices.loadFailed');
    } else {
      pricesMap = extractPricesMap(pricesRes.data);
      prices.value = pricesMap;
    }

    if (usageRes.ok) {
      syncApiAvailable.usageSummary = true;
      usageModels.value = extractUsageModels(usageRes.data);
    } else {
      syncApiAvailable.usageSummary = !usageRes.missing ? true : false;
      usageModels.value = [];
      if (!usageRes.missing) {
        // non-404 failure: soft notice only
        syncNotice.value = t('modelPrices.usageUnavailable', { error: usageRes.error });
      }
    }

    if (statusRes.ok) {
      syncApiAvailable.status = true;
      applyStatus(statusRes.data, pricesMap);
    } else {
      syncApiAvailable.status = !statusRes.missing;
      if (!statusRes.missing) {
        syncNotice.value = syncNotice.value || t('modelPrices.statusUnavailable', { error: statusRes.error });
      }
    }

    if (settingsRes.ok) {
      syncApiAvailable.settings = true;
      applySettings(settingsRes.data);
    } else if (statusRes.ok && (statusRes.data?.settings || statusRes.data?.Settings)) {
      syncApiAvailable.settings = true;
      // already applied via status
    } else {
      syncApiAvailable.settings = !settingsRes.missing;
    }
  } finally {
    loading.value = false;
  }
}

async function runSync() {
  if (!props.ready || syncing.value || status.value.running) return;
  error.value = '';
  syncNotice.value = '';
  const models = buildSyncModels(prices.value, usageModels.value);
  if (!models.length) {
    syncNotice.value = t('modelPrices.noModelsToSync');
    return;
  }
  syncing.value = true;
  try {
    const res = await softProxyCall(props.proxyCall, {
      method: 'POST',
      path: '/v0/management/model-prices/sync',
      body: { models },
    });
    if (!res.ok) {
      syncApiAvailable.sync = !res.missing;
      if (/409|already running|进行中/i.test(res.error || '')) {
        syncNotice.value = t('modelPrices.syncInProgress');
        status.value = { ...status.value, running: true };
        startPolling();
        return;
      }
      if (res.missing) {
        error.value = t('modelPrices.syncApiMissing');
      } else {
        error.value = res.error || t('modelPrices.syncFailed');
      }
      return;
    }
    syncApiAvailable.sync = true;
    // Refresh prices first so candidate filter sees exact matches just written.
    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    let pricesMap = prices.value;
    if (pricesRes.ok) {
      pricesMap = extractPricesMap(pricesRes.data);
      prices.value = pricesMap;
    }
    applySyncResult(res.data, pricesMap);

    const applied = lastResult.value?.applied || 0;
    const cand = pendingCandidates.value.length;
    const unmatched = lastResult.value?.unmatched || 0;
    if (lastResult.value?.error && applied === 0) {
      error.value = lastResult.value.error;
    } else {
      syncNotice.value = t('modelPrices.syncDone', { applied, candidates: cand, unmatched });
    }
  } finally {
    syncing.value = false;
  }
}

function startPolling() {
  stopPolling();
  pollTimer = setInterval(async () => {
    if (!props.ready) return;
    const res = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices/sync-status',
    });
    if (!res.ok) return;
    const prevRunning = status.value.running;
    applyStatus(res.data);
    if (prevRunning && !status.value.running) {
      const pricesRes = await softProxyCall(props.proxyCall, {
        method: 'GET',
        path: '/v0/management/model-prices',
      });
      if (pricesRes.ok) prices.value = extractPricesMap(pricesRes.data);
      stopPolling();
    }
  }, 2500);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function saveSettings() {
  if (!props.ready || settingsSaving.value) return;
  settingsMessage.value = '';
  error.value = '';
  const check = validateIntervalHours(settingsDraft.intervalHours);
  if (!check.ok) {
    error.value = check.error;
    return;
  }
  settingsDraft.intervalHours = check.value;
  settingsSaving.value = true;
  try {
    const body = {
      enabled: Boolean(settingsDraft.enabled),
      intervalHours: check.value,
      // Backend always forces protectManual=true.
      protectManual: true,
    };
    const res = await softProxyCall(props.proxyCall, {
      method: 'PUT',
      path: '/v0/management/model-prices/sync-settings',
      body,
    });
    if (!res.ok) {
      if (res.missing) {
        error.value = t('modelPrices.settingsApiMissing');
        syncApiAvailable.settings = false;
      } else {
        error.value = res.error || t('modelPrices.settingsSaveFailed');
      }
      return;
    }
    applySettings(res.data || body);
    settingsMessage.value = t('modelPrices.settingsSaved');
  } finally {
    settingsSaving.value = false;
  }
}

function addManualPrice() {
  editingModel.value = {
    isNew: true,
    model: '',
    prompt: 0,
    completion: 0,
    cache: 0,
    cacheRead: 0,
    cacheCreation: 0,
  };
}

function openEdit(row) {
  if (!row) return;
  if (row.hasPrice && row.price) {
    editingModel.value = {
      isNew: false,
      model: row.model,
      prompt: row.price.prompt,
      completion: row.price.completion,
      cache: row.price.cache,
      cacheRead: row.price.cacheRead,
      cacheCreation: row.price.cacheCreation,
    };
    return;
  }
  editingModel.value = {
    isNew: true,
    model: row.model || '',
    prompt: 0,
    completion: 0,
    cache: 0,
    cacheRead: 0,
    cacheCreation: 0,
  };
}

async function savePrice() {
  const putBody = buildManualPutBody(editingModel.value);
  const entry = buildManualPriceEntry(editingModel.value);
  if (!putBody || !entry) {
    error.value = t('modelPrices.modelNameRequired');
    return;
  }
  saving.value = true;
  error.value = '';
  try {
    // Single-model upsert only — backend merges and preserves other models.
    await props.proxyCall({
      method: 'PUT',
      path: '/v0/management/model-prices',
      body: putBody,
    });
    editingModel.value = null;
    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    if (pricesRes.ok) {
      prices.value = extractPricesMap(pricesRes.data);
      // If backend dropped source, still show manual locally for the edited model.
      if (prices.value[entry.model] && !prices.value[entry.model].source) {
        prices.value[entry.model] = {
          ...prices.value[entry.model],
          source: 'manual',
          sourceModelId: '',
          syncedAtMs: 0,
        };
      }
    } else {
      prices.value = { ...prices.value, [entry.model]: entry.price };
    }
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    saving.value = false;
  }
}

async function confirmDeletePrice(model) {
  const value = String(model || '').trim();
  if (!value || deleting.value) return;
  const ok = await showConfirm({
    title: t('modelPrices.deleteTitle'),
    message: t('modelPrices.deleteMessage', { model: value }),
    confirmLabel: t('common.delete'),
    variant: 'danger',
  });
  if (!ok) return;
  await deletePrice(value);
}

async function deletePrice(model) {
  const request = buildDeletePriceRequest(model);
  if (!request || deleting.value) return;
  deleting.value = true;
  error.value = '';
  syncNotice.value = '';
  try {
    const result = await softProxyCall(props.proxyCall, request);
    if (!result.ok) {
      error.value = result.missing
        ? t('modelPrices.deleteApiMissing')
        : result.error || t('modelPrices.deleteFailed');
      return;
    }
    if (editingModel.value?.model === model) editingModel.value = null;

    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    let pricesMap = prices.value;
    if (pricesRes.ok) {
      pricesMap = extractPricesMap(pricesRes.data);
      prices.value = pricesMap;
    } else {
      pricesMap = { ...prices.value };
      delete pricesMap[model];
      prices.value = pricesMap;
    }

    const statusRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices/sync-status',
    });
    if (statusRes.ok) {
      applyStatus(statusRes.data, pricesMap);
    } else {
      pendingCandidates.value = filterCandidatesWithExistingPrices(pendingCandidates.value, pricesMap);
      if (lastResult.value) {
        lastResult.value = {
          ...lastResult.value,
          candidates: pendingCandidates.value,
          candidateCount: pendingCandidates.value.length,
        };
      }
    }
    syncNotice.value = result.data?.deleted === false
      ? t('modelPrices.deleteAlreadyGone', { model })
      : t('modelPrices.deleteSuccess', { model });
  } finally {
    deleting.value = false;
  }
}

function toggleCandidate(c, idx) {
  const key = candidateKey(c, idx);
  const next = new Set(selectedCandidateKeys.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  selectedCandidateKeys.value = next;
}

function toggleSelectAllCandidates(ev) {
  if (ev?.target?.checked) {
    selectedCandidateKeys.value = new Set(
      pendingCandidates.value.map((c, idx) => candidateKey(c, idx)),
    );
  } else {
    selectedCandidateKeys.value = new Set();
  }
}

async function dismissCandidates() {
  const models = pendingCandidates.value.map((c) => c.localModel).filter(Boolean);
  pendingCandidates.value = [];
  selectedCandidateKeys.value = new Set();
  if (lastResult.value) {
    lastResult.value = {
      ...lastResult.value,
      candidates: [],
      candidateCount: 0,
    };
  }
  const res = await softProxyCall(props.proxyCall, {
    method: 'POST',
    path: '/v0/management/model-prices/sync-dismiss',
    body: { models },
  });
  if (res.ok && res.data?.status) {
    applyStatus(res.data.status, prices.value);
  } else if (!res.ok && !res.missing) {
    // Soft failure: local clear already applied; notice only.
    syncNotice.value = syncNotice.value || t('modelPrices.dismissNotPersisted', { error: res.error });
  }
}

async function confirmOne(candidate) {
  await confirmCandidates([candidate]);
}

async function confirmSelected() {
  await confirmCandidates(selectedCandidateList.value);
}

async function confirmCandidates(list) {
  if (!list.length || confirming.value) return;
  confirming.value = true;
  error.value = '';
  try {
    let applied = 0;
    for (const candidate of list) {
      const body = buildConfirmBody(candidate);
      if (!body) {
        error.value = t('modelPrices.candidateMissing');
        continue;
      }
      const res = await softProxyCall(props.proxyCall, {
        method: 'POST',
        path: '/v0/management/model-prices/sync-confirm',
        body,
      });
      if (!res.ok) {
        if (res.missing) syncApiAvailable.confirm = false;
        error.value = res.error || t('modelPrices.confirmFailed');
        continue;
      }
      applied += 1;
      // optimistic local update
      prices.value = {
        ...prices.value,
        [body.model]: {
          prompt: body.price.prompt,
          completion: body.price.completion,
          cache: body.price.cache,
          cacheRead: body.price.cacheRead,
          cacheCreation: body.price.cacheCreation,
          source: body.price.source || 'sync',
          sourceModelId: body.price.sourceModelId || '',
          updatedAtMs: Date.now(),
          syncedAtMs: Date.now(),
        },
      };
      if (res.data?.status) {
        applyStatus(res.data.status, prices.value);
      }
    }

    const confirmedModels = new Set(list.map((c) => c.localModel));
    pendingCandidates.value = filterCandidatesWithExistingPrices(
      pendingCandidates.value.filter((c) => !confirmedModels.has(c.localModel)),
      prices.value,
    );
    selectedCandidateKeys.value = new Set();
    if (lastResult.value) {
      lastResult.value = {
        ...lastResult.value,
        candidates: pendingCandidates.value,
        candidateCount: pendingCandidates.value.length,
      };
    }
    syncNotice.value = applied ? t('modelPrices.confirmed', { count: applied }) : syncNotice.value;

    const pricesRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices',
    });
    let pricesMap = prices.value;
    if (pricesRes.ok) {
      pricesMap = extractPricesMap(pricesRes.data);
      prices.value = pricesMap;
    }

    const statusRes = await softProxyCall(props.proxyCall, {
      method: 'GET',
      path: '/v0/management/model-prices/sync-status',
    });
    if (statusRes.ok) {
      applyStatus(statusRes.data, pricesMap);
    } else {
      pendingCandidates.value = filterCandidatesWithExistingPrices(pendingCandidates.value, pricesMap);
      if (lastResult.value) {
        lastResult.value = {
          ...lastResult.value,
          candidates: pendingCandidates.value,
          candidateCount: pendingCandidates.value.length,
        };
      }
    }
  } finally {
    confirming.value = false;
  }
}

defineExpose({ refresh });
</script>

<style scoped>
.model-prices-status-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.model-prices-settings {
  border-top: 1px solid var(--cpa-border);
  padding-top: 12px;
}
.model-prices-settings .section-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
.model-prices-settings .section-title h2 {
  margin: 0;
  font-size: 15px;
}
.model-prices-settings-grid {
  margin-top: 0;
}
.model-prices-source-results {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.model-prices-source-pill {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: baseline;
  border-radius: 999px;
  padding: 6px 10px;
  font-size: 12px;
  border: 1px solid var(--cpa-border);
  background: var(--cpa-surface-muted);
  color: var(--cpa-text-secondary);
}
.model-prices-source-pill.ok {
  border-color: color-mix(in srgb, var(--cpa-success) 35%, var(--cpa-border));
  background: var(--cpa-success-soft);
  color: var(--cpa-success);
}
.model-prices-source-pill.err {
  border-color: color-mix(in srgb, var(--cpa-error) 35%, var(--cpa-border));
  background: var(--cpa-error-soft);
  color: var(--cpa-error);
}
.model-prices-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  margin-bottom: 12px;
}
.model-prices-filters {
  flex: 1 1 auto;
}
.source-badge {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 700;
  border: 1px solid var(--cpa-border);
  background: var(--cpa-surface-muted);
  color: var(--cpa-text-secondary);
  white-space: nowrap;
}
.source-badge.source-manual {
  color: var(--cpa-warning);
  background: var(--cpa-warning-soft);
  border-color: color-mix(in srgb, var(--cpa-warning) 30%, var(--cpa-border));
}
.source-badge.source-litellm {
  color: var(--cpa-primary);
  background: color-mix(in srgb, var(--cpa-primary) 12%, var(--cpa-surface));
  border-color: color-mix(in srgb, var(--cpa-primary) 30%, var(--cpa-border));
}
.source-badge.source-openrouter {
  color: var(--cpa-success);
  background: var(--cpa-success-soft);
  border-color: color-mix(in srgb, var(--cpa-success) 30%, var(--cpa-border));
}
.clickable {
  cursor: pointer;
}
.clickable:hover {
  background: color-mix(in srgb, var(--cpa-primary) 6%, transparent);
}
@media (max-width: 720px) {
  .model-prices-toolbar {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
