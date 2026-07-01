<template>
  <section class="monitoring-page inspection-page">
    <div class="card filter-card inspection-status-card">
      <div class="inspection-status-bar">
        <div class="inspection-status-info">
          <span :class="['status-badge', toneClass(runTone)]">
            <i aria-hidden="true"></i>{{ runStatusText }}
          </span>
          <span :class="['status-badge', selectedConfig.enabled ? 'good' : '']">
            <i aria-hidden="true"></i>{{ selectedConfig.enabled ? '定时已启用' : '定时已关闭' }}
          </span>
          <span class="muted small-text">
            最近完成：{{ lastRunTime }}
            <template v-if="activeRun?.finishedAtMs"> · {{ formatDuration(activeRun) }}</template>
          </span>
        </div>
        <div class="config-actions-bar" style="padding:0">
          <button class="btn" @click="refreshAll(false)" :disabled="loading || !ready">{{ loading ? '加载中…' : '刷新' }}</button>
          <button class="btn primary" @click="confirmRunNow" :disabled="!ready || running">{{ running ? '提交中…' : '立即巡检' }}</button>
        </div>
      </div>

      <details class="inspection-info-note">
        <summary>服务端巡检说明</summary>
        <ul class="inspection-info-list">
          <li><strong>后台 Worker</strong>：定时任务由 Manager Server 执行，无需保持本页打开。</li>
          <li><strong>时间基准</strong>：定时时间点以 Manager Server 所在时区为准（可在配置中指定）。</li>
          <li><strong>自动刷新</strong>：启用定时或存在运行中批次时，每 30 秒静默刷新列表。</li>
        </ul>
      </details>

      <div class="inspection-config-overview">
        <div class="section-title">
          <h2>巡检配置</h2>
          <button type="button" class="btn" @click="openConfigDrawer()">编辑配置</button>
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

      <MetricGrid :cards="summaryCards" />
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>
    <section v-if="!ready" class="notice">缺少 CPA management key，无法访问插件代理。</section>

    <div class="inspection-detail-grid">
      <DataCard title="巡检历史" subtitle="选择批次查看结果与日志">
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
              <span v-if="run.deleteCount" class="pill pill-delete">删 {{ run.deleteCount }}</span>
              <span v-if="run.disableCount" class="pill pill-disable">禁 {{ run.disableCount }}</span>
              <span v-if="run.enableCount" class="pill pill-enable">启 {{ run.enableCount }}</span>
              <span v-if="run.reauthCount" class="pill pill-reauth">登 {{ run.reauthCount }}</span>
            </div>
          </button>
        </div>
        <div v-else class="empty">暂无巡检记录，点击「立即巡检」开始第一次检查。</div>
      </DataCard>

      <div class="inspection-detail-panels">
        <div v-if="detail?.run?.error" class="notice error" role="alert">{{ detail.run.error }}</div>

        <DataCard
          title="巡检结果"
          :subtitle="resultsSubtitle"
        >
          <template v-if="detail">
            <div class="inspection-results-toolbar">
              <div class="segment-group">
                <span class="segment-label">处理状态</span>
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
                <span class="segment-label">建议动作</span>
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
              <button
                class="btn danger"
                :disabled="!canExecuteBulk || executingAll"
                @click="confirmExecuteBulk"
              >
                {{ executingAll ? '执行中…' : `执行建议操作 (${executableResults.length})` }}
              </button>
            </div>

            <div class="table-wrap monitor-table">
              <table>
                <thead>
                  <tr>
                    <th>账号</th>
                    <th>凭据文件</th>
                    <th>建议动作</th>
                    <th>判定原因</th>
                    <th>额度</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in pagination.pageItems" :key="row.id">
                    <td>
                      <strong>{{ row.displayAccount || '—' }}</strong>
                      <div class="muted small-text">{{ row.provider }}</div>
                    </td>
                    <td class="small-text">{{ row.fileName }}</td>
                    <td>
                      <span :class="['action-pill', `action-${row.action}`]">{{ formatActionLabel(row.action) }}</span>
                    </td>
                    <td class="small-text">
                      {{ row.actionReason || '—' }}
                      <div v-if="formatActionStatusLabel(row)" class="muted">{{ formatActionStatusLabel(row) }}</div>
                    </td>
                    <td class="small-text">
                      <template v-if="row.usedPercent != null">{{ row.usedPercent }}%</template>
                      <template v-else>—</template>
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
                      <span v-else-if="row.action === 'reauth'" class="muted small-text">请在 CPA 管理台重新认证</span>
                      <span v-else-if="row.action === 'keep'" class="muted small-text">无需处理</span>
                      <span v-else class="muted small-text">需确认</span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div v-if="!filteredResults.length" class="empty">当前筛选下无结果。</div>
            <div v-if="pagination.count" class="pager">
              <span>{{ pagination.startItem }}–{{ pagination.endItem }} / {{ pagination.count }}</span>
              <select v-model.number="resultPageSize" class="control compact">
                <option v-for="n in pageSizes" :key="n" :value="n">{{ n }} 条/页</option>
              </select>
              <button class="btn" :disabled="pagination.currentPage <= 1" @click="resultPage--">上一页</button>
              <button class="btn" :disabled="pagination.currentPage >= pagination.totalPages" @click="resultPage++">下一页</button>
            </div>
          </template>
          <div v-else class="empty">选择左侧批次或开始新的巡检。</div>
        </DataCard>

        <DataCard title="巡检日志" subtitle="按时间顺序记录探测与执行过程">
          <div class="log-toolbar">
            <select v-model="logLevelFilter" class="control compact">
              <option value="all">全部 ({{ logs.length }})</option>
              <option value="info">信息</option>
              <option value="success">成功</option>
              <option value="warning">警告</option>
              <option value="error">错误</option>
            </select>
            <button class="btn" type="button" @click="copyLogs" :disabled="!logs.length">复制</button>
            <button class="btn" type="button" @click="logsCollapsed = !logsCollapsed">{{ logsCollapsed ? '展开' : '折叠' }}</button>
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
            <div v-if="!filteredLogs.length" class="empty">暂无日志。</div>
          </div>
          <div v-else class="muted small-text" style="padding:8px 0">已折叠 {{ logs.length }} 条日志</div>
        </DataCard>
      </div>
    </div>

    <div v-if="configDrawerOpen" class="drawer-backdrop" @click.self="closeConfigDrawer">
      <div class="card drawer inspection-drawer" role="dialog" aria-labelledby="inspection-config-title">
        <div class="drawer-head">
          <div>
            <h2 id="inspection-config-title">服务端巡检配置</h2>
            <p class="muted small-text">保存后由 Manager Server 应用，影响定时任务与默认探测参数。</p>
          </div>
          <button type="button" class="btn" @click="closeConfigDrawer">关闭</button>
        </div>

        <div v-if="configFieldErrors._form" class="notice error">{{ configFieldErrors._form }}</div>

        <section class="inspection-config-section">
          <h3>调度</h3>
          <label class="config-field config-field-toggle">
            <span class="config-field-label">启用定时巡检</span>
            <button type="button" :class="['toggle-switch', { on: draft.enabled }]" @click="draft.enabled = !draft.enabled">
              <span class="toggle-knob"></span>
            </button>
          </label>
          <div class="segmented-control schedule-mode">
            <button type="button" :class="['segment-btn', { active: draft.scheduleMode === 'interval' }]" @click="draft.scheduleMode = 'interval'">固定间隔</button>
            <button type="button" :class="['segment-btn', { active: draft.scheduleMode === 'time_points' }]" @click="draft.scheduleMode = 'time_points'">每日时间点</button>
          </div>
          <label v-if="draft.scheduleMode === 'interval'" class="config-field">
            <span class="config-field-label">间隔（分钟）</span>
            <input v-model="draft.intervalMinutes" type="number" min="1" class="control" />
            <small v-if="configFieldErrors.intervalMinutes" class="bad-text">{{ configFieldErrors.intervalMinutes }}</small>
          </label>
          <template v-else>
            <label class="config-field">
              <span class="config-field-label">时间点</span>
              <input v-model="draft.timePoints" class="control" placeholder="09:00, 13:30, 22:00" />
              <small v-if="configFieldErrors.timePoints" class="bad-text">{{ configFieldErrors.timePoints }}</small>
            </label>
            <label class="config-field">
              <span class="config-field-label">时区</span>
              <select v-model="draft.timeZone" class="control">
                <option value="">服务器默认</option>
                <option v-for="tz in timeZones" :key="tz" :value="tz">{{ tz }}</option>
              </select>
            </label>
          </template>
        </section>

        <section class="inspection-config-section">
          <h3>探测规则</h3>
          <div class="config-form-grid">
            <label class="config-field">
              <span class="config-field-label">额度阈值 (%)</span>
              <input v-model="draft.usedPercentThreshold" type="number" min="0" max="100" step="0.1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">抽样数量 (0=全部)</span>
              <input v-model="draft.sampleSize" type="number" min="0" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">自动处置</span>
              <select v-model="draft.autoActionMode" class="control">
                <option value="none">不自动执行</option>
                <option value="enable">自动启用</option>
                <option value="disable">自动禁用</option>
                <option value="delete">自动删除</option>
              </select>
            </label>
          </div>
        </section>

        <details class="inspection-config-section">
          <summary>高级参数</summary>
          <div class="config-form-grid" style="margin-top:12px">
            <label class="config-field">
              <span class="config-field-label">目标类型</span>
              <input v-model="draft.targetType" class="control" />
              <small v-if="configFieldErrors.targetType" class="bad-text">{{ configFieldErrors.targetType }}</small>
            </label>
            <label class="config-field">
              <span class="config-field-label">探测并发</span>
              <input v-model="draft.workers" type="number" min="1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">删除并发</span>
              <input v-model="draft.deleteWorkers" type="number" min="1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">超时 (ms)</span>
              <input v-model="draft.timeout" type="number" min="1" class="control" />
            </label>
            <label class="config-field">
              <span class="config-field-label">重试次数</span>
              <input v-model="draft.retries" type="number" min="0" class="control" />
            </label>
            <label class="config-field config-field-wide">
              <span class="config-field-label">User-Agent</span>
              <input v-model="draft.userAgent" class="control" />
            </label>
          </div>
        </details>

        <div class="config-actions-bar">
          <span v-if="configDirty" class="warn-text small-text">有未保存的更改</span>
          <span v-else class="muted small-text">已与服务器同步</span>
          <button class="btn" :disabled="saving || !configDirty" @click="discardConfig">放弃</button>
          <button class="btn primary" :disabled="saving || !configDirty || !managerConfig" @click="saveConfig">
            {{ saving ? '保存中…' : '保存并应用' }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
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
  getRunStatusLabel,
  getRunTone,
  isActionableResult,
  resolveServerCodexConfig,
  toDraft,
  validateInspectionConfigFields,
} from '../utils/codexInspection.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

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

let pollTimer = null;
let refreshInFlight = false;

const handlingFilters = HANDLING_FILTERS;
const actionFilters = ACTION_FILTERS;
const pageSizes = RESULT_PAGE_SIZES;

const selectedConfig = computed(() => resolveServerCodexConfig(managerConfig.value?.codexInspection));
const savedScheduleLabel = computed(() => formatSchedule(selectedConfig.value));
const configOverview = computed(() =>
  buildConfigOverviewItems(managerConfig.value?.codexInspection, savedScheduleLabel.value)
);

const activeRun = computed(() => detail.value?.run ?? runs.value[0] ?? null);
const runTone = computed(() => getRunTone(activeRun.value?.status));
const runStatusText = computed(() => getRunStatusLabel(activeRun.value?.status));

const lastRunTime = computed(() => {
  const ms = activeRun.value?.finishedAtMs;
  if (!ms) return '—';
  return new Date(ms).toLocaleTimeString('zh-CN', { hour12: false });
});

const summaryCards = computed(() => {
  const r = activeRun.value;
  const blank = '—';
  const cfg = selectedConfig.value;
  return [
    { label: '巡检集合', value: r ? r.probeSetCount : blank, sub: r ? `总文件 ${r.totalFiles}` : '' },
    { label: '本次探测', value: r ? r.sampledCount : blank, sub: formatTrigger(r) },
    { label: '建议删除', value: r ? r.deleteCount : blank, sub: r ? `待处理动作 ${actionTotal.value}` : '' },
    { label: '建议禁用', value: r ? r.disableCount : blank, sub: `阈值 ${cfg.usedPercentThreshold}%` },
    { label: '建议启用', value: r ? r.enableCount : blank, sub: r ? `保留 ${r.keepCount ?? 0}` : '' },
    { label: '需重新登录', value: r ? (r.reauthCount ?? 0) : blank, sub: '需在管理台处理' },
  ];
});

const actionTotal = computed(() => {
  const r = activeRun.value;
  if (!r) return 0;
  return (r.deleteCount || 0) + (r.disableCount || 0) + (r.enableCount || 0) + (r.reauthCount || 0);
});

const resultRows = computed(() => detail.value?.results ?? []);
const handlingCounts = computed(() => countHandlingStates(resultRows.value));
const actionCounts = computed(() => countActions(resultRows.value));
const filteredResults = computed(() =>
  filterInspectionResults(resultRows.value, handlingFilter.value, actionFilter.value)
);
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
  const r = detail.value?.run;
  if (!r) return '汇总账号状态与建议动作';
  const time = r.finishedAtMs ? formatTimestamp(r.finishedAtMs) : '—';
  return `${formatTrigger(r)} · 完成于 ${time}`;
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

const configFieldErrors = computed(() => validateInspectionConfigFields(draft));

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
  return { all: '全部', pending: '待处理', no_action: '无需处理' }[f] || f;
}

function actionLabel(f) {
  const map = {
    all: '全部',
    reauth: '重新登录',
    delete: '删除',
    disable: '禁用',
    enable: '启用',
    keep: '保留',
  };
  return map[f] || f;
}

function logDetailText(detail) {
  if (typeof detail === 'string') return detail;
  try {
    return JSON.stringify(detail);
  } catch {
    return String(detail);
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

function confirmRunNow() {
  if (!window.confirm('将立即在 Manager Server 上启动一次 Codex 账号巡检，是否继续？')) return;
  void runNow();
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

function confirmExecuteSingle(row) {
  const label = formatActionLabel(row.action);
  if (!window.confirm(`确定对账号「${row.displayAccount}」执行「${label}」吗？`)) return;
  void executeActions([row.id]);
}

function confirmExecuteBulk() {
  const targets = executableResults.value;
  const del = targets.filter((t) => t.action === 'delete').length;
  const dis = targets.filter((t) => t.action === 'disable').length;
  const en = targets.filter((t) => t.action === 'enable').length;
  if (!window.confirm(`即将执行 ${targets.length} 项：删除 ${del}、禁用 ${dis}、启用 ${en}。确认继续？`)) return;
  void executeActions(targets.map((t) => t.id), true);
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
    const failed = (resp?.outcomes || []).filter((o) => !o.success);
    if (failed.length) {
      error.value = `部分执行失败：${failed.length}/${(resp?.outcomes || []).length}`;
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

function closeConfigDrawer() {
  if (configDirty.value && !window.confirm('有未保存的更改，确定关闭？')) return;
  configDrawerOpen.value = false;
}

function discardConfig() {
  Object.assign(draft, toDraft(managerConfig.value?.codexInspection));
}

async function saveConfig() {
  const codexInspection = createConfigFromDraft(draft);
  if (!codexInspection) {
    error.value = '配置校验未通过，请检查表单';
    return;
  }
  if (!managerConfig.value) return;
  saving.value = true;
  try {
    const next = { ...managerConfig.value, codexInspection };
    const resp = await props.proxyCall({ method: 'PUT', path: '/usage-service/config', body: next });
    managerConfig.value = resp?.config || next;
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
    error.value = '复制日志失败';
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