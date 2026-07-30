<template>
  <section class="monitoring-page inspection-page">
    <div class="card inspection-status-card">
      <div class="inspection-status-bar">
        <div class="inspection-status-info">
          <span :class="['status-badge', toneClass(runTone)]">
            <i aria-hidden="true"></i>{{ runStatusText }}
          </span>
          <span :class="['status-badge', selectedConfig.enabled ? 'good' : '']">
            <i aria-hidden="true"></i>{{ selectedConfig.enabled ? t('inspection.scheduleOn') : t('inspection.scheduleOff') }}
          </span>
          <span class="muted small-text">
            {{ t('inspection.lastCompleted', { time: lastRunTime }) }}
            <template v-if="activeRun?.finishedAtMs"> · {{ formatDuration(activeRun) }}</template>
          </span>
        </div>
        <div class="config-actions-bar" style="padding:0">
          <button class="btn" @click="refreshAll(false)" :disabled="loading || !ready">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
          <button v-if="hasRunningRun" class="btn danger" @click="confirmCancelRun" :disabled="!ready || running">
            {{ t('inspection.cancelRun') }}
          </button>
          <button class="btn primary" @click="confirmRunNow" :disabled="!ready || running || hasRunningRun">
            {{ running ? t('inspection.submitting') : t('inspection.runNow') }}
          </button>
        </div>
      </div>

      <details class="inspection-info-note">
        <summary>{{ t('inspection.info.title') }}</summary>
        <ul class="inspection-info-list">
          <li><strong>{{ t('inspection.info.runtimeLabel') }}</strong>：{{ t('inspection.info.runtimeBody') }}</li>
          <li><strong>{{ t('inspection.info.timezoneLabel') }}</strong>：{{ t('inspection.info.timezoneBody') }}</li>
          <li><strong>{{ t('inspection.info.autoRefreshLabel') }}</strong>：{{ t('inspection.info.autoRefreshBody') }}</li>
        </ul>
      </details>

      <div class="inspection-config-overview">
        <div class="section-title">
          <h2>{{ t('inspection.configSectionTitle') }}</h2>
          <button type="button" class="btn" @click="openConfigDrawer()">{{ t('inspection.editConfig') }}</button>
        </div>
        <div class="config-summary-grid">
          <button
            v-for="item in configOverview"
            :key="item.key"
            type="button"
            class="config-overview-chip"
            @click="openConfigDrawer(item.field)"
          >
            <span class="config-summary-label">{{ item.label }}</span>
            <strong class="config-summary-value">{{ item.value }}</strong>
          </button>
        </div>
      </div>

      <div v-if="autoBanSettings" class="inspection-auto-ban-note">
        <strong>{{ t('autoBan.title') }}</strong>
        <span :class="['status-badge', autoBanSettings.enabled ? 'good' : 'idle']">{{ autoBanSettings.enabled ? t('common.enabled') : t('common.disabled') }}</span>
        <span class="muted small-text">{{ t('autoBan.subtitle') }}</span>
      </div>

      <div class="inspection-summary-shell">
        <MetricGrid class="inspection-summary-grid" :cards="summaryCards" />
      </div>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>
    <section v-if="!ready" class="notice">{{ t('inspection.missingKey') }}</section>

    <div class="inspection-detail-grid">
      <DataCard :title="t('inspection.history.title')" :subtitle="t('inspection.history.subtitle')">
        <div v-if="runs.length" class="run-history-list" role="tablist">
          <button
            v-for="run in runs"
            :key="run.id"
            type="button"
            role="tab"
            :aria-selected="run.id === selectedRunId"
            :class="['run-history-card', { active: run.id === selectedRunId }]"
            @click="selectRun(run.id)"
          >
            <div class="run-history-head">
              <span :class="['status-badge', toneClass(getRunTone(run.status))]">
                <i></i>{{ getRunStatusLabel(run.status) }}
              </span>
              <span class="muted small-text">#{{ run.id }}</span>
            </div>
            <div class="muted small-text">{{ formatTimestamp(run.startedAtMs) }} · {{ formatTrigger(run) }}</div>
            <div class="run-pills">
              <span v-if="run.deleteCount" class="pill pill-delete">{{ t('inspection.history.pillDelete', { count: run.deleteCount }) }}</span>
              <span v-if="run.disableCount" class="pill pill-disable">{{ t('inspection.history.pillDisable', { count: run.disableCount }) }}</span>
              <span v-if="run.enableCount" class="pill pill-enable">{{ t('inspection.history.pillEnable', { count: run.enableCount }) }}</span>
              <span v-if="run.reauthCount" class="pill pill-reauth">{{ t('inspection.history.pillReauth', { count: run.reauthCount }) }}</span>
            </div>
          </button>
        </div>
        <div v-else class="empty">{{ t('inspection.history.empty') }}</div>
      </DataCard>

      <div class="inspection-detail-panels">
        <div v-if="detail?.run?.error" class="notice error" role="alert">{{ detail.run.error }}</div>

        <DataCard
          :title="t('inspection.results.title')"
          :subtitle="resultsSubtitle"
        >
          <template v-if="detail">
            <div class="inspection-results-toolbar">
              <div class="inspection-filter-row">
                <div class="segment-group">
                  <span class="segment-label">{{ t('inspection.results.handlingLabel') }}</span>
                  <div class="segmented-control">
                    <button
                      v-for="f in handlingFilters"
                      :key="f"
                      type="button"
                      :class="['segment-btn', { active: handlingFilter === f }]"
                      @click="handlingFilter = f"
                    >
                      {{ handlingLabel(f) }} <span class="segment-count">{{ handlingCounts[f] }}</span>
                    </button>
                  </div>
                </div>
                <div class="segment-group">
                  <span class="segment-label">{{ t('inspection.results.actionLabel') }}</span>
                  <div class="segmented-control">
                    <button
                      v-for="f in actionFilters"
                      :key="f"
                      type="button"
                      :class="['segment-btn', { active: actionFilter === f }]"
                      @click="actionFilter = f"
                    >
                      {{ actionLabel(f) }} <span class="segment-count">{{ actionCounts[f] }}</span>
                    </button>
                  </div>
                </div>
              </div>
              <div class="inspection-results-actions">
                <button
                  class="btn danger"
                  :disabled="!canExecuteBulk || executingAll"
                  @click="confirmExecuteBulk"
                >
                  {{ executingAll ? t('inspection.results.executing') : t('inspection.results.executeBulk', { count: executableResults.length }) }}
                </button>
              </div>
            </div>

            <div class="table-wrap monitor-table">
              <table>
                <thead>
                  <tr>
                    <th>{{ t('inspection.columns.account') }}</th>
                    <th>{{ t('inspection.columns.credentialFile') }}</th>
                    <th>{{ t('inspection.columns.action') }}</th>
                    <th>{{ t('inspection.columns.reason') }}</th>
                    <th>{{ t('inspection.columns.quota') }}</th>
                    <th>{{ t('inspection.columns.operations') }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in pagination.pageItems" :key="row.id">
                    <td>
                      <strong>{{ row.displayAccount || EMPTY_VALUE }}</strong>
                      <div class="muted small-text">{{ row.provider }}</div>
                    </td>
                    <td class="small-text">{{ row.fileName }}</td>
                    <td>
                      <span :class="['action-pill', `action-${row.action}`]">{{ formatActionLabel(row.action) }}</span>
                    </td>
                    <td class="small-text">
                      {{ row.actionReason || EMPTY_VALUE }}
                      <div v-if="row.errorKind" class="muted">{{ row.errorKind }}<template v-if="row.statusCode"> · HTTP {{ row.statusCode }}</template></div>
                      <div v-if="formatActionStatusLabel(row)" class="muted">{{ formatActionStatusLabel(row) }}</div>
                    </td>
                    <td class="small-text">
                      <template v-if="row.usedPercent != null">{{ row.usedPercent }}%</template>
                      <template v-else>{{ EMPTY_VALUE }}</template>
                    </td>
                    <td>
                      <button
                        v-if="canonicalIds.has(row.id)"
                        :class="['btn', row.action === 'delete' ? 'danger' : '', 'btn-xs']"
                        :disabled="!canExecuteActions || executingIds.has(row.id)"
                        @click="confirmExecuteSingle(row)"
                      >
                        {{ executingIds.has(row.id) ? '…' : formatActionLabel(row.action) }}
                      </button>
                      <template v-else-if="isAcknowledgeableResult(row)">
                        <div v-if="row.action === 'reauth'" class="muted small-text">{{ t('inspection.row.reauthHint') }}</div>
                        <button
                          class="btn btn-xs"
                          :disabled="!canExecuteActions || actionInFlight || executingIds.has(row.id)"
                          @click="confirmAcknowledge(row)"
                        >
                          {{ executingIds.has(row.id) ? '…' : t('inspection.row.acknowledge') }}
                        </button>
                      </template>
                      <span v-else-if="row.actionStatus === 'acknowledged'" class="muted small-text">{{ t('inspection.row.acknowledged') }}</span>
                      <span v-else-if="row.action === 'reauth'" class="muted small-text">{{ t('inspection.row.reauthHint') }}</span>
                      <span v-else-if="row.action === 'review'" class="muted small-text">{{ t('inspection.row.reviewHint') }}</span>
                      <span v-else-if="row.action === 'keep'" class="muted small-text">{{ t('inspection.row.keepHint') }}</span>
                      <span v-else class="muted small-text">{{ t('inspection.row.confirmNeeded') }}</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="!filteredResults.length" class="empty">{{ resultsEmptyText }}</div>
            <div v-if="pagination.count" class="pager">
              <span>{{ pagination.startItem }}–{{ pagination.endItem }} / {{ pagination.count }}</span>
              <select v-model.number="resultPageSize" class="control compact">
                <option v-for="n in pageSizes" :key="n" :value="n">{{ t('inspection.pager.perPage', { n }) }}</option>
              </select>
              <button class="btn" :disabled="pagination.currentPage <= 1" @click="resultPage--">{{ t('inspection.pager.prev') }}</button>
              <button class="btn" :disabled="pagination.currentPage >= pagination.totalPages" @click="resultPage++">{{ t('inspection.pager.next') }}</button>
            </div>
          </template>
          <div v-else class="empty">{{ t('inspection.results.emptySelect') }}</div>
        </DataCard>

        <DataCard :title="t('inspection.logs.title')" :subtitle="t('inspection.logs.subtitle')">
          <div class="log-toolbar">
            <select v-model="logLevelFilter" class="control compact">
              <option value="all">{{ t('inspection.logs.all', { count: logs.length }) }}</option>
              <option value="info">{{ t('inspection.logs.info') }}</option>
              <option value="success">{{ t('inspection.logs.success') }}</option>
              <option value="warning">{{ t('inspection.logs.warning') }}</option>
              <option value="error">{{ t('inspection.logs.error') }}</option>
            </select>
            <button class="btn" type="button" @click="copyLogs" :disabled="!logs.length">{{ t('inspection.logs.copy') }}</button>
            <button class="btn" type="button" @click="logsCollapsed = !logsCollapsed">
              {{ logsCollapsed ? t('inspection.logs.expand') : t('inspection.logs.collapse') }}
            </button>
          </div>
          <div v-if="!logsCollapsed" class="log-list">
            <div
              v-for="entry in filteredLogs"
              :key="entry.id"
              :class="['log-row', `log-${entry.level}`]"
            >
              <span class="log-time">{{ formatTimestamp(entry.createdAtMs) }}</span>
              <span class="log-message">
                {{ entry.message }}
                <small v-if="entry.detail" class="muted">{{ logDetailText(entry.detail) }}</small>
              </span>
            </div>
            <div v-if="!filteredLogs.length" class="empty">{{ t('inspection.logs.empty') }}</div>
          </div>
          <div v-else class="muted small-text" style="padding:8px 0">{{ t('inspection.logs.collapsed', { count: logs.length }) }}</div>
        </DataCard>
      </div>
    </div>

    <div v-if="configDrawerOpen" class="drawer-backdrop" @click.self="closeConfigDrawer">
      <div class="modal-dialog card drawer inspection-drawer" role="dialog" aria-labelledby="inspection-config-title">
        <div class="drawer-head">
          <div>
            <h2 id="inspection-config-title">{{ t('inspection.drawer.title') }}</h2>
            <p class="muted small-text">{{ t('inspection.drawer.subtitle') }}</p>
          </div>
          <button type="button" class="btn" @click="closeConfigDrawer">{{ t('common.close') }}</button>
        </div>

        <div v-if="configFieldErrors._form" class="notice error">{{ configFieldErrors._form }}</div>

        <section class="inspection-config-section">
          <h3>{{ t('inspection.drawer.schedule') }}</h3>
          <label class="config-field config-field-toggle">
            <span class="config-field-label">{{ t('inspection.drawer.enableSchedule') }}</span>
            <button type="button" :class="['toggle-switch', { on: draft.enabled }]" @click="draft.enabled = !draft.enabled">
              <span class="toggle-knob"></span>
            </button>
          </label>
          <div class="segmented-control schedule-mode">
            <button type="button" :class="['segment-btn', { active: draft.scheduleMode === 'interval' }]" @click="draft.scheduleMode = 'interval'">{{ t('inspection.drawer.intervalMode') }}</button>
            <button type="button" :class="['segment-btn', { active: draft.scheduleMode === 'time_points' }]" @click="draft.scheduleMode = 'time_points'">{{ t('inspection.drawer.timePointsMode') }}</button>
          </div>
          <label v-if="draft.scheduleMode === 'interval'" class="config-field">
            <span class="config-field-label">{{ t('inspection.drawer.intervalMinutes') }}</span>
            <input v-model="draft.intervalMinutes" type="number" min="1" class="control" />
            <small v-if="configFieldErrors.intervalMinutes" class="bad-text">{{ configFieldErrors.intervalMinutes }}</small>
          </label>
          <template v-else>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.timePoints') }}</span>
              <input v-model="draft.timePoints" class="control" placeholder="09:00, 13:30, 22:00" />
              <small v-if="configFieldErrors.timePoints" class="bad-text">{{ configFieldErrors.timePoints }}</small>
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.timeZone') }}</span>
              <select v-model="draft.timeZone" class="control">
                <option value="">{{ t('inspection.drawer.serverDefault') }}</option>
                <option v-for="tz in timeZones" :key="tz" :value="tz">{{ tz }}</option>
              </select>
            </label>
          </template>
        </section>

        <section class="inspection-config-section">
          <h3>{{ t('inspection.drawer.probeRules') }}</h3>
          <div class="config-form-grid">
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.usedPercentThreshold') }}</span>
              <input v-model="draft.usedPercentThreshold" type="number" min="0" max="100" step="0.1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.sampleSize') }}</span>
              <input v-model="draft.sampleSize" type="number" min="0" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.autoAction') }}</span>
              <select v-model="draft.autoActionMode" class="control">
                <option value="none">{{ t('inspection.autoAction.none') }}</option>
                <option value="enable">{{ t('inspection.autoAction.enable') }}</option>
                <option value="disable">{{ t('inspection.autoAction.disable') }}</option>
                <option value="delete">{{ t('inspection.autoAction.delete') }}</option>
              </select>
            </label>
            <label class="config-field config-field-toggle">
              <span class="config-field-label">{{ t('inspection.drawer.autoRecover') }}</span>
              <button type="button" :class="['toggle-switch', { on: draft.autoRecoverEnabled }]" @click="draft.autoRecoverEnabled = !draft.autoRecoverEnabled">
                <span class="toggle-knob"></span>
              </button>
            </label>
          </div>
        </section>

        <details class="inspection-config-section">
          <summary>{{ t('inspection.drawer.advanced') }}</summary>
          <div class="config-form-grid" style="margin-top:12px">
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.targetTypes') }}</span>
              <select v-model="draft.targetTypes" class="control">
                <option value="codex">Codex</option>
                <option value="xai">xAI</option>
                <option value="codex+xai">Codex + xAI</option>
              </select>
              <small v-if="configFieldErrors.targetTypes" class="bad-text">{{ configFieldErrors.targetTypes }}</small>
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.workers') }}</span>
              <input v-model="draft.workers" type="number" min="1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.deleteWorkers') }}</span>
              <input v-model="draft.deleteWorkers" type="number" min="1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.timeout') }}</span>
              <input v-model="draft.timeout" type="number" min="1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">{{ t('inspection.drawer.retries') }}</span>
              <input v-model="draft.retries" type="number" min="0" class="control" />
            </label>
            <label class="config-field config-field-wide">
              <span class="config-field-label">{{ t('inspection.drawer.userAgent') }}</span>
              <input v-model="draft.userAgent" class="control" />
            </label>
            <template v-if="draft.targetTypes.includes('xai')">
              <label class="config-field config-field-toggle config-field-wide">
                <span class="config-field-label">{{ t('inspection.drawer.xaiInference') }}</span>
                <button type="button" :class="['toggle-switch', { on: draft.xaiInferenceEnabled }]" @click="draft.xaiInferenceEnabled = !draft.xaiInferenceEnabled">
                  <span class="toggle-knob"></span>
                </button>
              </label>
              <template v-if="draft.xaiInferenceEnabled">
                <label class="config-field">
                  <span class="config-field-label">{{ t('inspection.drawer.xaiModel') }}</span>
                  <input v-model="draft.xaiInferenceModel" class="control" />
                </label>
                <label class="config-field config-field-wide">
                  <span class="config-field-label">{{ t('inspection.drawer.xaiUserAgent') }}</span>
                  <input v-model="draft.xaiInferenceUserAgent" class="control" />
                </label>
                <label class="config-field config-field-wide">
                  <span class="config-field-label">{{ t('inspection.drawer.xaiPrompt') }}</span>
                  <input v-model="draft.xaiInferencePrompt" class="control" />
                </label>
              </template>
            </template>
          </div>
        </details>

        <div class="config-actions-bar">
          <span v-if="configDirty" class="warn-text small-text">{{ t('inspection.drawer.dirty') }}</span>
          <span v-else class="muted small-text">{{ t('inspection.drawer.clean') }}</span>
          <button class="btn" :disabled="saving || !configDirty" @click="discardConfig">{{ t('inspection.drawer.discard') }}</button>
          <button class="btn primary" :disabled="saving || !configDirty || !managerConfig" @click="saveConfig">
            {{ saving ? t('inspection.drawer.saving') : t('inspection.drawer.saveApply') }}
          </button>
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
import {
  ACTION_FILTERS,
  HANDLING_FILTERS,
  RESULT_PAGE_SIZES,
  buildConfigOverviewItems,
  buildPagination,
  configsEquivalent,
  countActions,
  countHandlingStates,
  createConfigFromDraft,
  filterInspectionResults,
  formatActionLabel,
  formatActionStatusLabel,
  formatDuration,
  formatSchedule,
  formatTimestamp,
  formatTrigger,
  getCanonicalActionIds,
  getInspectionResultsEmptyText,
  getRunStatusLabel,
  getRunTone,
  isAcknowledgeableResult,
  isActionableResult,
  resolveServerCodexConfig,
  toDraft,
  validateInspectionConfigFields,
} from '../utils/codexInspection.js';
import { buildInspectionConfigSaveBody } from '../utils/inspectionConfigSave.js';
import { EMPTY_VALUE, formatTime } from '../utils/localeFormat.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t, locale } = useI18n();

const RUNS_LIMIT = 30;
const COMMON_TZ = ['UTC', 'Asia/Shanghai', 'Asia/Tokyo', 'Europe/London', 'America/New_York', 'America/Los_Angeles'];

const loading = ref(false);
const running = ref(false);
const saving = ref(false);
const error = ref('');
const managerConfig = ref(null);
const draft = reactive(toDraft(null));
const runs = ref([]);
const detail = ref(null);
const selectedRunId = ref(null);
const handlingFilter = ref('all');
const actionFilter = ref('all');
const resultPage = ref(1);
const resultPageSize = ref(RESULT_PAGE_SIZES[0]);
const logLevelFilter = ref('all');
const logsCollapsed = ref(false);
const executingIds = ref(new Set());
const executingAll = ref(false);
const configDrawerOpen = ref(false);
const configFocusField = ref(null);
const actionInFlight = ref(false);

const confirmOpen = ref(false);
const confirmTitle = ref('');
const confirmMessage = ref('');
const confirmOkLabel = ref('');
const confirmCancelLabel = ref('');
const confirmVariant = ref('primary');
let confirmResolve = null;

function showConfirm({
  title = '',
  message = '',
  confirmLabel = '',
  cancelLabel = '',
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

let pollTimer = null;
let refreshInFlight = false;

const handlingFilters = HANDLING_FILTERS;
const actionFilters = ACTION_FILTERS;
const pageSizes = RESULT_PAGE_SIZES;

// Recompute overview/labels when locale changes so codexInspection translate() stays in sync.
const localeTick = computed(() => locale.value);

const selectedConfig = computed(() => resolveServerCodexConfig(managerConfig.value?.codexInspection));
const savedScheduleLabel = computed(() => {
  void localeTick.value;
  return formatSchedule(selectedConfig.value);
});
const configOverview = computed(() => {
  void localeTick.value;
  return buildConfigOverviewItems(managerConfig.value?.codexInspection, savedScheduleLabel.value);
});
const autoBanSettings = computed(() => managerConfig.value?.autoBan || null);

const activeRun = computed(() => detail.value?.run ?? runs.value[0] ?? null);
const runTone = computed(() => getRunTone(activeRun.value?.status));
const runStatusText = computed(() => {
  void localeTick.value;
  return getRunStatusLabel(activeRun.value?.status);
});

const lastRunTime = computed(() => {
  const ms = activeRun.value?.finishedAtMs;
  if (!ms) return EMPTY_VALUE;
  return formatTime(ms);
});

const actionTotal = computed(() => {
  const r = activeRun.value;
  if (!r) return 0;
  return (r.deleteCount || 0) + (r.disableCount || 0) + (r.enableCount || 0) + (r.reauthCount || 0);
});

const summaryCards = computed(() => {
  void localeTick.value;
  const r = activeRun.value;
  const blank = EMPTY_VALUE;
  const cfg = selectedConfig.value;
  return [
    {
      label: t('inspection.summary.probeSet'),
      value: r ? r.probeSetCount : blank,
      sub: r ? t('inspection.summary.totalFiles', { count: r.totalFiles }) : '',
    },
    {
      label: t('inspection.summary.sampled'),
      value: r ? r.sampledCount : blank,
      sub: formatTrigger(r),
    },
    {
      label: t('inspection.summary.suggestDelete'),
      value: r ? r.deleteCount : blank,
      sub: r ? t('inspection.summary.pendingActions', { count: actionTotal.value }) : '',
    },
    {
      label: t('inspection.summary.suggestDisable'),
      value: r ? r.disableCount : blank,
      sub: t('inspection.summary.threshold', { value: cfg.usedPercentThreshold }),
    },
    {
      label: t('inspection.summary.suggestEnable'),
      value: r ? r.enableCount : blank,
      sub: r ? t('inspection.summary.keep', { count: r.keepCount ?? 0 }) : '',
    },
    {
      label: t('inspection.summary.reauth'),
      value: r ? (r.reauthCount ?? 0) : blank,
      sub: t('inspection.summary.reauthSub'),
    },
  ];
});

const resultRows = computed(() => detail.value?.results ?? []);
const handlingCounts = computed(() => countHandlingStates(resultRows.value));
const actionCounts = computed(() => countActions(resultRows.value));
const filteredResults = computed(() =>
  filterInspectionResults(resultRows.value, handlingFilter.value, actionFilter.value)
);
const resultsEmptyText = computed(() => {
  void localeTick.value;
  return getInspectionResultsEmptyText(detail.value?.run, resultRows.value.length, filteredResults.value.length);
});
const pagination = computed(() => buildPagination(filteredResults.value, resultPage.value, resultPageSize.value));
const canonicalIds = computed(() => getCanonicalActionIds(resultRows.value));
const executableResults = computed(() => resultRows.value.filter(isActionableResult).filter((i) => canonicalIds.value.has(i.id)));
const canExecuteActions = computed(() => detail.value?.run?.status === 'completed');
const canExecuteBulk = computed(() => canExecuteActions.value && executableResults.value.length > 0 && !actionInFlight.value);
const hasRunningRun = computed(
  () => runs.value.some((r) => r.status === 'running') || detail.value?.run?.status === 'running'
);

const logs = computed(() => detail.value?.logs ?? []);
const filteredLogs = computed(() => {
  if (logLevelFilter.value === 'all') return logs.value;
  return logs.value.filter((e) => e.level === logLevelFilter.value);
});

const resultsSubtitle = computed(() => {
  void localeTick.value;
  const r = detail.value?.run;
  if (!r) return t('inspection.results.subtitleDefault');
  const time = r.finishedAtMs ? formatTimestamp(r.finishedAtMs) : EMPTY_VALUE;
  return t('inspection.results.subtitleRun', { trigger: formatTrigger(r), time });
});

const normalizedDraftConfig = computed(() => {
  const c = createConfigFromDraft(draft);
  return c ? resolveServerCodexConfig(c) : null;
});

const configDirty = computed(() => {
  if (!managerConfig.value?.codexInspection || !normalizedDraftConfig.value) return false;
  return !configsEquivalent(managerConfig.value.codexInspection, normalizedDraftConfig.value) ||
    Boolean(draft.enabled) !== Boolean(selectedConfig.value.enabled);
});

const configFieldErrors = computed(() => {
  void localeTick.value;
  return validateInspectionConfigFields(draft);
});

const timeZones = computed(() => {
  const set = new Set(COMMON_TZ);
  if (draft.timeZone) set.add(draft.timeZone);
  try {
    const browser = Intl.DateTimeFormat().resolvedOptions().timeZone;
    if (browser) set.add(browser);
  } catch { /* ignore */ }
  return [...set];
});

function toneClass(tone) {
  if (tone === 'good') return 'good';
  if (tone === 'bad') return 'bad';
  if (tone === 'info') return 'warn';
  return '';
}

function handlingLabel(f) {
  if (f === 'all') return t('common.all');
  if (f === 'pending') return t('inspection.handling.pending');
  if (f === 'no_action') return t('inspection.handling.noAction');
  return f;
}

function actionLabel(f) {
  if (f === 'all') return t('common.all');
  return formatActionLabel(f);
}

function logDetailText(detailValue) {
  if (typeof detailValue === 'string') return detailValue;
  try {
    return JSON.stringify(detailValue);
  } catch {
    return String(detailValue);
  }
}

async function loadManagerConfig() {
  const resp = await props.proxyCall({ method: 'GET', path: '/usage-service/config' });
  managerConfig.value = resp?.config || resp;
  Object.assign(draft, toDraft(managerConfig.value?.codexInspection));
}

async function loadRunDetail(id) {
  const d = await props.proxyCall({ method: 'GET', path: `/v0/management/codex-inspection/runs/${id}` });
  detail.value = d;
  selectedRunId.value = d?.run?.id ?? id;
}

async function loadRunsList() {
  const resp = await props.proxyCall({
    method: 'GET',
    path: '/v0/management/codex-inspection/runs',
    query: `limit=${RUNS_LIMIT}`,
  });
  runs.value = resp?.items || [];
}

async function refreshAll(silent = false) {
  if (!props.ready || refreshInFlight) return;
  refreshInFlight = true;
  if (!silent) {
    loading.value = true;
    error.value = '';
  }
  try {
    await loadManagerConfig();
    await loadRunsList();
    const valid = selectedRunId.value != null && runs.value.some((r) => r.id === selectedRunId.value);
    if (valid) {
      const watchingRunning = detail.value?.run?.status === 'running';
      if (!silent || !detail.value || watchingRunning) {
        await loadRunDetail(selectedRunId.value);
      }
    } else {
      const first = runs.value[0]?.id;
      if (first) await loadRunDetail(first);
      else {
        detail.value = null;
        selectedRunId.value = null;
      }
    }
  } catch (e) {
    if (!silent) error.value = e.message || String(e);
  } finally {
    if (!silent) loading.value = false;
    refreshInFlight = false;
  }
}

async function selectRun(id) {
  if (id === selectedRunId.value) return;
  try {
    await loadRunDetail(id);
    resultPage.value = 1;
  } catch (e) {
    error.value = e.message || String(e);
  }
}

async function confirmRunNow() {
  const ok = await showConfirm({
    title: t('inspection.confirm.runNowTitle'),
    message: t('inspection.confirm.runNowMessage'),
    confirmLabel: t('inspection.confirm.runNowOk'),
  });
  if (!ok) return;
  void runNow();
}

async function confirmCancelRun() {
  if (!activeRun.value?.id) return;
  const ok = await showConfirm({
    title: t('inspection.confirm.cancelTitle'),
    message: t('inspection.confirm.cancelMessage'),
    confirmLabel: t('inspection.confirm.cancelOk'),
    variant: 'danger',
  });
  if (!ok) return;
  running.value = true;
  try {
    await props.proxyCall({ method: 'POST', path: `/v0/management/codex-inspection/runs/${activeRun.value.id}/cancel` });
    await refreshAll(true);
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    running.value = false;
  }
}

async function runNow() {
  running.value = true;
  error.value = '';
  try {
    const d = await props.proxyCall({ method: 'POST', path: '/v0/management/codex-inspection/run' });
    detail.value = d;
    selectedRunId.value = d?.run?.id ?? null;
    await loadRunsList();
  } catch (e) {
    error.value = e.message || String(e);
    await refreshAll(true);
  } finally {
    running.value = false;
  }
}

async function confirmAcknowledge(row) {
  const ok = await showConfirm({
    title: t('inspection.confirm.acknowledgeTitle'),
    message: t('inspection.confirm.acknowledgeMessage', { account: row.displayAccount }),
    confirmLabel: t('inspection.confirm.acknowledgeOk'),
  });
  if (!ok) return;
  void acknowledgeResult(row.id);
}

async function acknowledgeResult(resultID) {
  if (!detail.value?.run?.id || !resultID) return;
  actionInFlight.value = true;
  executingIds.value = new Set([resultID]);
  try {
    const resp = await props.proxyCall({
      method: 'POST',
      path: `/v0/management/codex-inspection/runs/${detail.value.run.id}/acknowledge`,
      body: { resultIds: [resultID] },
    });
    detail.value = resp?.detail ?? resp;
    await loadRunsList();
    const failed = (resp?.outcomes || []).filter((outcome) => !outcome.success);
    if (failed.length) {
      error.value = t('inspection.errors.acknowledgeFailed');
    }
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    actionInFlight.value = false;
    executingIds.value = new Set();
  }
}

async function confirmExecuteSingle(row) {
  const label = formatActionLabel(row.action);
  const ok = await showConfirm({
    title: t('inspection.confirm.executeTitle'),
    message: t('inspection.confirm.executeMessage', { account: row.displayAccount, action: label }),
    confirmLabel: label,
    variant: row.action === 'delete' ? 'danger' : 'primary',
  });
  if (!ok) return;
  void executeActions([row.id]);
}

async function confirmExecuteBulk() {
  const targets = executableResults.value;
  const del = targets.filter((item) => item.action === 'delete').length;
  const dis = targets.filter((item) => item.action === 'disable').length;
  const en = targets.filter((item) => item.action === 'enable').length;
  const ok = await showConfirm({
    title: t('inspection.confirm.bulkTitle'),
    message: t('inspection.confirm.bulkMessage', {
      total: targets.length,
      delete: del,
      disable: dis,
      enable: en,
    }),
    confirmLabel: t('inspection.confirm.bulkOk'),
    variant: del > 0 ? 'danger' : 'primary',
  });
  if (!ok) return;
  void executeActions(targets.map((item) => item.id), true);
}

async function executeActions(resultIds, bulk = false) {
  if (!detail.value?.run?.id || !resultIds.length) return;
  actionInFlight.value = true;
  executingIds.value = new Set(resultIds);
  if (bulk) executingAll.value = true;
  try {
    const resp = await props.proxyCall({
      method: 'POST',
      path: `/v0/management/codex-inspection/runs/${detail.value.run.id}/actions`,
      body: { resultIds },
    });
    detail.value = resp?.detail ?? resp;
    await loadRunsList();
    const outcomes = resp?.outcomes || [];
    const failed = outcomes.filter((o) => !o.success);
    if (failed.length) {
      error.value = t('inspection.errors.partialFailed', { failed: failed.length, total: outcomes.length });
    }
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    actionInFlight.value = false;
    executingIds.value = new Set();
    executingAll.value = false;
  }
}

function openConfigDrawer(field) {
  configFocusField.value = field || null;
  configDrawerOpen.value = true;
}

async function closeConfigDrawer() {
  if (configDirty.value) {
    const ok = await showConfirm({
      title: t('inspection.confirm.discardTitle'),
      message: t('inspection.confirm.discardMessage'),
      confirmLabel: t('inspection.confirm.discardOk'),
      variant: 'danger',
    });
    if (!ok) return;
  }
  configDrawerOpen.value = false;
}

function discardConfig() {
  Object.assign(draft, toDraft(managerConfig.value?.codexInspection));
}

async function saveConfig() {
  const codexInspection = createConfigFromDraft(draft);
  if (!codexInspection) {
    error.value = t('inspection.errors.configInvalid');
    return;
  }
  if (!managerConfig.value) return;
  saving.value = true;
  try {
    // Only send writable inspection fields — never echo redacted connection metadata.
    const body = buildInspectionConfigSaveBody(codexInspection);
    const resp = await props.proxyCall({
      method: 'PUT',
      path: '/usage-service/config',
      body,
    });
    managerConfig.value = resp?.config || { ...managerConfig.value, codexInspection };
    Object.assign(draft, toDraft(managerConfig.value?.codexInspection));
    configDrawerOpen.value = false;
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    saving.value = false;
  }
}

async function copyLogs() {
  const lines = logs.value.map((e) => {
    const ts = new Date(e.createdAtMs).toISOString();
    const det = e.detail ? ` ${logDetailText(e.detail)}` : '';
    return `[${ts}] [${e.level}] ${e.message}${det}`;
  });
  try {
    await navigator.clipboard.writeText(lines.join('\n'));
  } catch {
    error.value = t('inspection.errors.copyLogsFailed');
  }
}

function setupPoll() {
  clearPoll();
  if (!props.ready) return;
  if (!selectedConfig.value.enabled && !hasRunningRun.value) return;
  pollTimer = window.setInterval(() => {
    if (saving.value || running.value || actionInFlight.value) return;
    void refreshAll(true);
  }, 30_000);
}

function clearPoll() {
  if (pollTimer) {
    window.clearInterval(pollTimer);
    pollTimer = null;
  }
}

watch([() => selectedConfig.value.enabled, hasRunningRun], () => setupPoll());
watch([handlingFilter, actionFilter, () => detail.value?.run?.id], () => {
  resultPage.value = 1;
});
watch(resultPageSize, () => {
  resultPage.value = 1;
});

onMounted(() => {
  if (props.ready) void refreshAll(false).then(setupPoll);
});

watch(
  () => props.ready,
  (v) => {
    if (v) void refreshAll(false).then(setupPoll);
    else clearPoll();
  }
);

onBeforeUnmount(clearPoll);

defineExpose({ refresh: (force) => refreshAll(!force) });
</script>
