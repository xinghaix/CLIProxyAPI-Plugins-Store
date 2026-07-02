<template>
  <section class="monitoring-page">
    <div class="card filter-card monitoring-filterbar">
      <div class="filterbar-title">
        <div class="eyebrow">REQUEST MONITORING</div>
        <h2>请求监控</h2>
      </div>
      <div class="filterbar-controls primary-filters">
        <select v-model="timeRange" class="control compact">
          <option value="today">今天</option>
          <option value="7d">最近 7 天</option>
          <option value="14d">最近 14 天</option>
          <option value="30d">最近 30 天</option>
          <option value="all">全部</option>
          <option value="custom">自定义</option>
        </select>
        <select v-model.number="autoRefreshMs" class="control compact">
          <option :value="0">不自动刷新</option>
          <option :value="5000">5 秒</option>
          <option :value="15000">15 秒</option>
          <option :value="30000">30 秒</option>
          <option :value="60000">60 秒</option>
        </select>
        <input v-model.trim="searchQuery" class="control wide"
               placeholder="全文搜索：模型 / 账号 / API Key / 路径 / trace / 错误" @keyup.enter="refresh(true)"/>
      </div>
      <div class="filterbar-actions">
        <button class="btn primary" @click="refresh(true)" :disabled="loading || !ready">{{
            loading ? '加载中…' : '刷新'
          }}
        </button>
        <button class="btn" @click="exportEventsCsv" :disabled="!eventRows.length">导出 CSV</button>
        <button class="btn" @click="resetFilters">重置</button>
      </div>
      <div class="filterbar-controls secondary-filters">
        <select v-model="filters.status" class="control compact">
          <option value="all">全部状态</option>
          <option value="success">仅成功</option>
          <option value="failed">仅失败</option>
        </select>
        <select v-model="filters.provider" class="control compact">
          <option value="all">全部 Provider</option>
          <option v-for="item in optionProviders" :key="item" :value="item">{{ item }}</option>
        </select>
        <select v-model="filters.model" class="control compact">
          <option value="all">全部模型</option>
          <option v-for="item in optionModels" :key="item" :value="item">{{ item }}</option>
        </select>
        <select v-model="filters.account" class="control compact">
          <option value="all">全部账号</option>
          <option v-for="item in optionAccounts" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
        <select v-model="filters.apiKeyHash" class="control compact">
          <option value="all">全部 API Key</option>
          <option v-for="item in optionApiKeys" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
      </div>
    </div>

    <div v-if="timeRange === 'custom'" class="card filter-card custom-range-bar">
      <label>开始 <input v-model="customStart" type="datetime-local" class="control"/></label>
      <label>结束 <input v-model="customEnd" type="datetime-local" class="control"/></label>
      <button class="btn" @click="refresh(true)">应用</button>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>
    <section v-if="!ready" class="notice">缺少 CPA management key，无法访问插件代理。</section>

    <MetricGrid :cards="summaryCards"/>

    <div class="monitor-tabs card">
      <button v-for="tab in dataTabs" :key="tab.key" :class="['tab', {active: activeDataTab === tab.key}]"
              @click="activeDataTab = tab.key">{{ tab.label }} <span>{{ tab.count }}</span></button>
    </div>

    <DataCard v-if="activeDataTab === 'timeline'" title="时间线">
      <div class="section-title"><span>{{ data?.granularity || 'auto' }} · {{
          formatDateTime(data?.generated_at_ms)
        }}</span></div>
      <div class="timeline-bars" v-if="timelineRows.length">
        <div v-for="point in timelineRows" :key="point.label + point.bucket_ms" class="timeline-row">
          <span class="timeline-label">{{ point.label }}</span>
          <div class="timeline-track"><i :style="{width: barWidth(point.calls || point.requests || 0)}"></i></div>
          <span class="timeline-value">{{ fmtInt(point.calls || point.requests || 0) }}</span>
          <span class="timeline-sub">{{ fmtInt(point.tokens || point.total_tokens || 0) }} tok</span>
        </div>
      </div>
      <div v-else class="empty">暂无时间线数据</div>
    </DataCard>

    <DataCard v-if="activeDataTab === 'events'" title="事件流">
      <div class="table-wrap monitor-table event-stream-table">
        <table>
          <thead>
          <tr>
            <th>来源 / API KEY</th>
            <th>模型</th>
            <th>强度</th>
            <th>最近状态</th>
            <th>请求状态</th>
            <th>成功率</th>
            <th>总调用</th>
            <th>TPS</th>
            <th>首字/耗时</th>
            <th>时间</th>
            <th>本次用量</th>
            <th>本次花费</th>
          </tr>
          </thead>
          <tbody>
          <tr v-for="row in pagedEvents" :key="row.id" @click="selectedEvent = row.raw" class="clickable">
            <td>
              <strong>{{ row.sourceName }}</strong>
              <div class="muted small-text">提供方: {{ row.provider }}</div>
              <div class="muted small-text">API Key: {{ row.apiKeyMasked }}</div>
            </td>
            <td>
              <strong>{{ row.model }}</strong>
              <div v-if="row.resolvedModel && row.resolvedModel !== row.model" class="muted small-text">
                {{ row.resolvedModel }}
              </div>
            </td>
            <td>
              <strong :class="{'blue-text': row.intensity !== '-'}">{{ row.intensity }}</strong>
              <div class="muted small-text">等级: {{ row.tier }}</div>
            </td>
            <td>
              <div class="recent-status" aria-hidden="true">
                <span v-for="(success, idx) in row.recentPattern" :key="idx"
                      :class="['pattern-bar', success ? 'good' : 'bad']"></span>
              </div>
            </td>
            <td>
                <span v-if="row.failed" class="status-badge bad failure-trigger" tabindex="0"
                      @click.stop="toggleFailureTooltip($event, row)" @mouseenter="showFailureTooltip($event, row)"
                      @mouseleave="hideFailureTooltip">
                  <i></i>失败
                </span>
              <span v-else class="status-badge good"><i></i>成功</span>
            </td>
            <td><strong :class="successRateClass(row.successRate)">{{ fmtPct(row.successRate) }}</strong></td>
            <td>{{ fmtInt(row.totalCalls) }}</td>
            <td>{{ fmtTps(row.tps) }}</td>
            <td>
              <div :class="latencyClass(row.ttftMs)">{{ fmtSeconds(row.ttftMs) }}</div>
              <div :class="latencyClass(row.latencyMs)">{{ fmtSeconds(row.latencyMs) }}</div>
            </td>
            <td>
              <div>{{ formatDate(row.timestampMs) }}</div>
              <div>{{ formatTime(row.timestampMs) }}</div>
            </td>
            <td>
              <strong>{{ fmtCompact(row.totalTokens) }}</strong>
              <div class="muted small-text usage-breakdown">{{ row.usageText }}</div>
            </td>
            <td><strong>{{ fmtMoney(row.cost) }}</strong></td>
          </tr>
          </tbody>
        </table>
      </div>
      <PaginationBar :page="eventPage" :page-size="eventPageSize" :total="eventTableRows.length"
                     @page="eventPage = $event"/>
      <Teleport to="body">
        <div v-if="failureTooltip.visible" class="failure-tooltip-popover" :style="failureTooltip.style"
             @mouseenter="keepFailureTooltip" @mouseleave="hideFailureTooltip">
          <button class="failure-tooltip-copy" @click.stop="copyFailureText" title="复制">⎘</button>
          <div v-if="failureTooltip.row?.failStatusCode" class="failure-tooltip-status">HTTP
            {{ failureTooltip.row.failStatusCode }}
          </div>
          <div v-if="failureTooltip.row?.failSummary" class="failure-tooltip-body">
            {{ decodeHtmlEntities(failureTooltip.row.failSummary) }}
          </div>
        </div>
      </Teleport>
    </DataCard>

    <DataCard v-if="activeDataTab === 'accounts'" title="账号汇总" subtitle="account_stats / api_key_stats">
      <div class="split">
        <div>
          <h3 style="margin:0 0 10px; font-size:14px; color:var(--cpa-text-secondary)">账号维度</h3>
          <SimpleTable :rows="accountRows" :columns="accountColumns" selectable :selected-id="selectedAccountId"
                       @select="row => selectedAccountId = row.id || row.account_snapshot || row.account || ''"/>
        </div>
        <div>
          <h3 style="margin:0 0 10px; font-size:14px; color:var(--cpa-text-secondary)">API Key 维度</h3>
          <SimpleTable :rows="apiKeyRows" :columns="apiKeyColumns" @select="setApiKeyFilter"/>
        </div>
      </div>
    </DataCard>

    <div v-if="activeDataTab === 'accounts' && selectedAccount" style="margin-top:16px">
      <DataCard title="账号详情"
                :subtitle="selectedAccount.account_snapshot || selectedAccount.auth_label_snapshot || selectedAccount.id || '—'">
        <DetailGrid :items="buildAccountDetail(selectedAccount)"/>
      </DataCard>
    </div>

    <DataCard v-if="activeDataTab === 'models'" title="模型维度" subtitle="model_stats / model_share">
      <SimpleTable :rows="modelRows" :columns="modelColumns" @select="setModelFilter"/>
    </DataCard>

    <div v-if="selectedEvent" class="modal-backdrop" @click.self="selectedEvent = null">
      <div class="modal-dialog card">
        <div class="modal-head">
          <div><h2>请求详情</h2>
            <p class="muted">{{ formatDateTime(selectedEvent.timestamp_ms) }} ·
              {{ selectedEvent.event_hash || selectedEvent.request_id || '—' }}</p></div>
          <button class="btn" @click="selectedEvent = null">关闭</button>
        </div>
        <MetricGrid :cards="eventDetailCards"/>
        <div class="detail-grid">
          <div><h3>基础</h3>
            <pre>{{ pretty(eventBaseDetail) }}</pre>
          </div>
          <div><h3>响应 Metadata</h3>
            <pre>{{ pretty(selectedEvent.response_metadata || {}) }}</pre>
          </div>
          <div><h3>错误 / Quota / Trace</h3>
            <pre>{{ pretty(eventHeaderDetail) }}</pre>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import {computed, defineComponent, h, onBeforeUnmount, onMounted, ref, watch} from 'vue';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';

const props = defineProps({
  ready: {type: Boolean, default: false},
  proxyCall: {type: Function, required: true},
});

const data = ref(null);
const modelPrices = ref({});
const loading = ref(false);
const error = ref('');
const timeRange = ref('today');
const customStart = ref(toLocalInput(startOfTodayMs()));
const customEnd = ref(toLocalInput(Date.now()));
const searchQuery = ref('');
const autoRefreshMs = ref(5000);
const activeDataTab = ref('events');
const selectedEvent = ref(null);
const eventPage = ref(1);
const eventPageSize = ref(50);
const selectedAccountId = ref('');
const filters = ref(defaultFilters());
const failureTooltip = ref({visible: false, row: null, style: {}});
let failureHideTimer = null;
let timer = null;

const dataTabs = computed(() => [
  {key: 'events', label: '事件流', count: eventRows.value.length},
  {key: 'accounts', label: '账号汇总', count: accountRows.value.length + apiKeyRows.value.length},
  {key: 'models', label: '模型', count: modelRows.value.length},
  {key: 'timeline', label: '时间线', count: timelineRows.value.length},
]);

const summary = computed(() => data.value?.summary || {});
const eventRows = computed(() => (data.value?.events?.items || []).map((row, idx) => ({...row, __id: idx})));
// loadedAllEvents removed — no longer needed after KPI card cleanup
const hasPrices = computed(() => Object.keys(modelPrices.value).length > 0);
const summaryCards = computed(() => {
  const s = summary.value;
  const totalCacheTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const cacheHitTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0);
  const inputSideTokens = Math.max(Number(s.input_tokens ?? 0), Number(s.cached_tokens ?? 0)) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const cacheHitRate = inputSideTokens > 0 ? cacheHitTokens / inputSideTokens : 0;
  const tokenMix = (n) => s.total_tokens > 0 ? `${fmtPct(n / s.total_tokens)}` : '—';
  return [
    {label: '总调用', value: fmtInt(s.total_calls), sub: `${accountCount.value} 账号`},
    {label: '调用成功率', value: fmtPct(s.success_rate), sub: fmtDuration(s.average_latency_ms)},
    {label: '失败总数', value: fmtInt(s.failure_calls), sub: `${failedGroupCount.value} 监控组`},
    {
      label: '预估花费',
      value: hasPrices.value ? fmtMoney(s.total_cost) : '--',
      sub: hasPrices.value ? '已配置单价模型' : '未配置单价'
    },
    {label: '总 Tokens', value: fmtCompact(s.total_tokens), sub: `推理 ${fmtCompact(s.reasoning_tokens)}`},
    {label: '输入 Tokens', value: fmtCompact(s.input_tokens), sub: `占比 ${tokenMix(Number(s.input_tokens ?? 0))}`},
    {label: '输出 Tokens', value: fmtCompact(s.output_tokens), sub: `占比 ${tokenMix(Number(s.output_tokens ?? 0))}`},
    {label: '缓存 Tokens', value: fmtCompact(totalCacheTokens), sub: `命中率 ${fmtPct(cacheHitRate)}`},
  ];
});
const eventGroupMap = computed(() => buildEventGroupMap(eventRows.value));
const failedGroupCount = computed(() => {
  let count = 0;
  for (const group of eventGroupMap.value.values()) {
    if ((group.failureCalls ?? 0) > 0) count++;
  }
  return count;
});
const accountCount = computed(() => accountRows.value.length);
const eventTableRows = computed(() => eventRows.value.map(row => buildEventTableRow(row, eventGroupMap.value)));
const pagedEvents = computed(() => pageRows(eventTableRows.value, eventPage.value, eventPageSize.value));
// eventsSubtitle removed — debug paging info no longer shown in card header
const timelineRows = computed(() => [...(data.value?.timeline || [])].sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0)));
const maxTimelineCalls = computed(() => Math.max(1, ...timelineRows.value.map(p => Number(p.calls || p.requests || 0))));
const modelRows = computed(() => data.value?.model_stats || data.value?.model_share || []);
const channelRows = computed(() => data.value?.channel_share || []);
const accountRows = computed(() => data.value?.account_stats || []);
const apiKeyRows = computed(() => data.value?.api_key_stats || []);
const failureRows = computed(() => [...(data.value?.failure_sources || []), ...(data.value?.recent_failures || [])]);
const taskRows = computed(() => data.value?.task_buckets || []);

const optionModels = computed(() => unique([...(data.value?.filter_options?.model_stats || []).map(x => x.model), ...modelRows.value.map(x => x.model)]));
const optionProviders = computed(() => unique([...(data.value?.filter_options?.providers || []), ...eventRows.value.map(x => x.auth_provider_snapshot)]));
const optionAccounts = computed(() => uniqueObjects(accountRows.value.map(row => ({
  value: row.id || row.account_snapshot || row.account || '',
  label: row.account_snapshot || row.auth_label_snapshot || row.id || row.account || ''
}))));
const optionApiKeys = computed(() => uniqueObjects(apiKeyRows.value.map(row => ({
  value: row.api_key_hash || row.id || '',
  label: `${shortHash(row.api_key_hash || row.id)} · ${row.account_snapshot || row.auth_label_snapshot || ''}`
}))));

const accountColumns = [
  ['account_snapshot', '账号'], ['auth_label_snapshot', '标签'], ['auth_provider_snapshot', 'Provider'], ['calls', '请求'], ['success_rate', '成功率', 'pct'], ['total_tokens', 'Token', 'int'], ['cost', '费用', 'money'], ['average_latency_ms', '延迟', 'ms'], ['last_seen_ms', '最后出现', 'time']
];
const apiKeyColumns = [
  ['api_key_hash', 'API Key', 'hash'], ['account_snapshot', '账号'], ['auth_label_snapshot', '标签'], ['calls', '请求'], ['success_rate', '成功率', 'pct'], ['total_tokens', 'Token', 'int'], ['cost', '费用', 'money'], ['last_seen_ms', '最后出现', 'time']
];
const modelColumns = [
  ['model', '模型'], ['calls', '请求'], ['success_calls', '成功'], ['failure_calls', '失败'], ['success_rate', '成功率', 'pct'], ['total_tokens', 'Token', 'int'], ['cost', '费用', 'money']
];
const channelColumns = [
  ['auth_index', 'Auth'], ['source', 'Source'], ['account_snapshot', '账号'], ['auth_provider_snapshot', 'Provider'], ['calls', '请求'], ['success', '成功'], ['failure', '失败'], ['tokens', 'Token', 'int'], ['cost', '费用', 'money'], ['average_latency_ms', '延迟', 'ms']
];
const failureColumns = [
  ['timestamp_ms', '时间', 'time'], ['source', 'Source'], ['source_hash', 'Source Hash', 'hash'], ['auth_index', 'Auth'], ['model', '模型'], ['calls', '请求'], ['failure', '失败'], ['fail_summary', '错误'], ['header_error_kind', '错误类型'], ['last_seen_ms', '最后出现', 'time']
];
const taskColumns = [
  ['bucket_key', '任务'], ['source', 'Source'], ['auth_index', 'Auth'], ['total', '请求'], ['success', '成功'], ['failure', '失败'], ['models', '模型'], ['total_tokens', 'Token', 'int'], ['average_latency_ms', '延迟', 'ms'], ['last_ms', '结束', 'time']
];

const eventDetailCards = computed(() => selectedEvent.value ? [
  {label: '状态', value: selectedEvent.value.failed ? '失败' : '成功'},
  {label: 'Token', value: selectedEvent.value.total_tokens ?? 0},
  {label: '延迟', value: fmtMs(selectedEvent.value.latency_ms)},
  {label: '费用', value: fmtMoney(calculateEventCost(selectedEvent.value, modelPrices.value))},
] : []);
const eventBaseDetail = computed(() => selectedEvent.value ? decodeDetailObject(pickObject(selectedEvent.value, ['request_id', 'event_hash', 'timestamp_ms', 'model', 'resolved_model', 'endpoint', 'method', 'path', 'auth_index', 'source', 'source_hash', 'api_key_hash', 'account_snapshot', 'auth_label_snapshot', 'auth_provider_snapshot', 'auth_project_id_snapshot', 'input_tokens', 'output_tokens', 'cached_tokens', 'cache_read_tokens', 'cache_creation_tokens', 'reasoning_tokens', 'total_tokens', 'latency_ms', 'ttft_ms', 'failed', 'fail_status_code', 'fail_summary'])) : {});
const eventHeaderDetail = computed(() => selectedEvent.value ? decodeDetailObject(pickObject(selectedEvent.value, ['header_quota_recover_at_ms', 'header_quota_used_percent', 'header_quota_plan_type', 'header_error_kind', 'header_error_code', 'header_trace_id'])) : {});

watch([timeRange, searchQuery, filters], () => {
  eventPage.value = 1;
}, {deep: true});
watch(autoRefreshMs, setupTimer);
watch(() => props.ready, (ready) => {
  if (ready && !data.value) refresh(true);
});
onMounted(() => {
  if (props.ready) refresh(true);
});
onBeforeUnmount(() => clearTimer());

async function refresh(force = false) {
  if (!props.ready) return;
  if (loading.value && !force) return;
  loading.value = true;
  error.value = '';
  try {
    const [analyticsData, pricesData] = await Promise.all([
      props.proxyCall({method: 'POST', path: '/v0/management/monitoring/analytics', body: buildAnalyticsRequest()}),
      loadModelPrices(),
    ]);
    data.value = analyticsData;
    modelPrices.value = pricesData;
    selectedEvent.value = null;
    setupTimer();
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    loading.value = false;
  }
}

async function loadModelPrices() {
  try {
    const resp = await props.proxyCall({method: 'GET', path: '/v0/management/model-prices'});
    return resp?.prices || {};
  } catch {
    return {};
  }
}

function buildAnalyticsRequest() {
  const {fromMs, toMs} = resolveRange();
  const f = {};
  if (filters.value.model !== 'all') f.models = [filters.value.model];
  if (filters.value.provider !== 'all') f.providers = [filters.value.provider];
  if (filters.value.account !== 'all') f.accounts = [filters.value.account];
  if (filters.value.apiKeyHash !== 'all') f.api_key_hashes = [filters.value.apiKeyHash];
  if (filters.value.projectId) f.project_ids = [filters.value.projectId];
  if (filters.value.requestType) f.request_types = [filters.value.requestType];
  if (filters.value.status === 'success') f.include_failed = false;
  if (filters.value.status === 'failed') f.failed_only = true;
  if (Number(filters.value.minLatencyMs) > 0) f.min_latency_ms = Number(filters.value.minLatencyMs);
  if (filters.value.cacheStatus) f.cache_status = filters.value.cacheStatus;
  if (filters.value.headerTraceId) f.header_trace_ids = [filters.value.headerTraceId];
  const request = {
    from_ms: fromMs,
    to_ms: toMs,
    now_ms: Date.now(),
    time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || '',
    include: {
      summary: true,
      summary_comparison: true,
      timeline: true,
      hourly_distribution: true,
      model_share: true,
      channel_share: true,
      model_stats: true,
      failure_sources: true,
      account_stats: true,
      api_key_stats: true,
      filter_options: true,
      heatmap: true,
      anomaly_points: true,
      task_buckets: true,
      recent_failures: 30,
      events_page: {limit: 300},
      granularity: shouldUseHour(fromMs, toMs) ? 'hour' : 'day',
    },
  };
  if (searchQuery.value) request.search_query = searchQuery.value;
  if (Object.keys(f).length) request.filters = f;
  return request;
}

function resolveRange() {
  const now = Date.now();
  if (timeRange.value === 'today') return {fromMs: startOfTodayMs(), toMs: now};
  if (timeRange.value === '7d') return {fromMs: now - 7 * 86400000, toMs: now};
  if (timeRange.value === '14d') return {fromMs: now - 14 * 86400000, toMs: now};
  if (timeRange.value === '30d') return {fromMs: now - 30 * 86400000, toMs: now};
  if (timeRange.value === 'custom') return {fromMs: Date.parse(customStart.value), toMs: Date.parse(customEnd.value)};
  return {fromMs: 0, toMs: now};
}

function resetFilters() {
  filters.value = defaultFilters();
  searchQuery.value = '';
  refresh(true);
}

function setAccountFilter(row) {
  filters.value.account = row.id || row.account_snapshot || row.account || 'all';
  refresh(true);
}

function setApiKeyFilter(row) {
  filters.value.apiKeyHash = row.api_key_hash || row.id || 'all';
  refresh(true);
}

function setModelFilter(row) {
  filters.value.model = row.model || 'all';
  refresh(true);
}

function setupTimer() {
  clearTimer();
  if (autoRefreshMs.value > 0) timer = window.setInterval(() => refresh(false), autoRefreshMs.value);
}

function clearTimer() {
  if (timer) window.clearInterval(timer);
  timer = null;
}

function showFailureTooltip(event, row) {
  if (failureHideTimer) {
    clearTimeout(failureHideTimer);
    failureHideTimer = null;
  }
  const el = event.currentTarget;
  const rect = el.getBoundingClientRect();
  const left = Math.max(12, Math.min(rect.left, window.innerWidth - 440 - 12));
  const spaceBelow = window.innerHeight - rect.bottom - 12;
  const placement = spaceBelow >= 200 || spaceBelow >= rect.top ? 'below' : 'above';
  failureTooltip.value = {
    visible: true,
    row,
    style: placement === 'below'
        ? {top: `${rect.bottom + 8}px`, left: `${left}px`, maxWidth: '420px'}
        : {bottom: `${window.innerHeight - rect.top + 8}px`, left: `${left}px`, maxWidth: '420px'},
  };
}

function toggleFailureTooltip(event, row) {
  if (failureTooltip.value.visible && failureTooltip.value.row?.id === row.id) {
    hideFailureTooltip();
  } else {
    showFailureTooltip(event, row);
  }
}

function keepFailureTooltip() {
  if (failureHideTimer) {
    clearTimeout(failureHideTimer);
    failureHideTimer = null;
  }
}

function hideFailureTooltip() {
  if (failureHideTimer) clearTimeout(failureHideTimer);
  failureHideTimer = setTimeout(() => {
    failureTooltip.value.visible = false;
  }, 120);
}

function copyFailureText() {
  const row = failureTooltip.value.row;
  if (!row) return;
  const parts = [];
  if (row.failStatusCode) parts.push(`HTTP ${row.failStatusCode}`);
  if (row.failSummary) parts.push(decodeHtmlEntities(row.failSummary));
  const text = parts.join('\n');
  if (navigator.clipboard) {
    navigator.clipboard.writeText(text).then(() => {
    }).catch(() => {
    });
  }
}

function decodeHtmlEntities(str) {
  if (!str) return '';
  const txt = document.createElement('textarea');
  txt.innerHTML = str;
  return txt.value;
}

function decodeDetailObject(obj) {
  if (!obj || typeof obj !== 'object') return obj;
  const result = {};
  for (const [key, value] of Object.entries(obj)) {
    result[key] = typeof value === 'string' ? decodeHtmlEntities(value) : value;
  }
  return result;
}

function exportEventsCsv() {
  const cols = ['timestamp_ms', 'failed', 'model', 'auth_index', 'account_snapshot', 'api_key_hash', 'method', 'path', 'total_tokens', 'latency_ms', 'fail_status_code', 'fail_summary', 'header_trace_id'];
  const csv = [cols.join(','), ...eventRows.value.map(row => cols.map(c => csvCell(row[c])).join(','))].join('\n');
  const blob = new Blob([csv], {type: 'text/csv;charset=utf-8'});
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `monitoring-events-${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

function buildEventGroupMap(events) {
  const sortedAsc = [...(events || [])].sort(
      (a, b) => Number(a.timestamp_ms || 0) - Number(b.timestamp_ms || 0) || String(a.__id).localeCompare(String(b.__id))
  );
  const metricsByStream = new Map();
  const groupsByStream = new Map();
  for (const event of sortedAsc) {
    const key = eventGroupKey(event);
    const prev = metricsByStream.get(key) ?? {total: 0, success: 0, pattern: []};
    const statsIncluded = event.failed === true || Number(event.input_tokens || 0) > 0 || Number(event.output_tokens || 0) > 0;
    const requestCount = prev.total + (statsIncluded ? 1 : 0);
    const successCount = prev.success + (statsIncluded && !event.failed ? 1 : 0);
    const successRate = requestCount > 0 ? successCount / requestCount : 1;
    const pattern = [...prev.pattern, !event.failed].slice(-10);
    metricsByStream.set(key, {total: requestCount, success: successCount, pattern});
    const group = groupsByStream.get(key) ?? {calls: 0, successCalls: 0, failureCalls: 0, events: []};
    group.calls += 1;
    group.successCalls += event.failed ? 0 : 1;
    group.failureCalls += event.failed ? 1 : 0;
    group.events.push(event);
    groupsByStream.set(key, group);
  }
  const map = new Map();
  for (const [key, group] of groupsByStream) {
    group.events.sort((a, b) => Number(b.timestamp_ms || 0) - Number(a.timestamp_ms || 0));
    map.set(key, group);
  }
  // attach per-event sliding-window snapshot for buildEventTableRow
  map._slidingWindow = new Map();
  // re-walk sortedAsc to capture snapshot at each event position
  const sw = new Map();
  for (const event of sortedAsc) {
    const key = eventGroupKey(event);
    const prev = sw.get(key) ?? {total: 0, success: 0, pattern: []};
    const statsIncluded = event.failed === true || Number(event.input_tokens || 0) > 0 || Number(event.output_tokens || 0) > 0;
    const requestCount = prev.total + (statsIncluded ? 1 : 0);
    const successCount = prev.success + (statsIncluded && !event.failed ? 1 : 0);
    const successRate = requestCount > 0 ? successCount / requestCount : 1;
    const pattern = [...prev.pattern, !event.failed].slice(-10);
    sw.set(key, {total: requestCount, success: successCount, pattern});
    if (!map._slidingWindow.has(key)) map._slidingWindow.set(key, new Map());
    map._slidingWindow.get(key).set(event.event_hash || event.request_id || `${event.timestamp_ms}-${event.__id}`, {
      requestCount,
      successRate,
      recentPattern: pattern,
    });
  }
  return map;
}

function buildEventTableRow(row, groupMap) {
  const key = eventGroupKey(row);
  const group = groupMap.get(key) || {};
  const eventId = row.event_hash || row.request_id || `${row.timestamp_ms}-${row.__id}`;
  const sliding = groupMap._slidingWindow?.get(key)?.get(eventId);
  const latencyMs = numberOrNull(row.latency_ms);
  const outputTokens = Number(row.output_tokens || 0);
  return {
    id: eventId,
    raw: row,
    sourceName: row.account_snapshot || row.auth_label_snapshot || row.source || row.auth_index || '—',
    provider: row.auth_provider_snapshot || row.provider || row.source || '—',
    apiKeyMasked: maskApiKey(row.api_key_hash),
    model: row.model || '—',
    resolvedModel: row.resolved_model || '',
    intensity: row.reasoning_effort || row.service_tier || '-',
    tier: row.service_tier || (row.reasoning_effort && row.reasoning_effort !== '-' ? 'priority' : 'default'),
    recentPattern: (sliding?.recentPattern || []).slice(-5),
    failed: Boolean(row.failed),
    successRate: sliding?.successRate ?? (row.failed ? 0 : 1),
    totalCalls: sliding?.requestCount ?? 1,
    tps: latencyMs && latencyMs > 0 ? outputTokens / (latencyMs / 1000) : null,
    ttftMs: numberOrNull(row.ttft_ms) ?? latencyMs,
    latencyMs,
    timestampMs: row.timestamp_ms,
    totalTokens: Number(row.total_tokens || 0),
    usageText: buildUsageText(row),
    cost: calculateEventCost(row, modelPrices.value),
    failStatusCode: numberOrNull(row.fail_status_code),
    failSummary: row.fail_summary || '',
  };
}

function eventGroupKey(row) {
  const account = row.account_snapshot || row.auth_label_snapshot || row.source || '';
  const provider = row.auth_provider_snapshot || row.provider || '';
  const model = row.model || '';
  return [account, provider, model].join('::');
}

function normalizeRate(rate, calls, success) {
  if (rate != null && Number.isFinite(Number(rate))) return Number(rate) > 1 ? Number(rate) / 100 : Number(rate);
  return calls > 0 ? success / calls : null;
}

function numberOrNull(v) {
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function buildUsageText(row) {
  const total = Number(row.total_tokens || 0);
  if (total === 0) return '0';
  const parts = [];
  parts.push(`I ${fmtCompact(row.input_tokens)}`);
  parts.push(`O ${fmtCompact(row.output_tokens)}`);
  if (Number(row.reasoning_tokens || 0) > 0) parts.push(`R ${fmtCompact(row.reasoning_tokens)}`);
  const cached = Number(row.cached_tokens || row.cache_read_tokens || row.cache_creation_tokens || 0);
  if (cached > 0) parts.push(`C ${fmtCompact(cached)}`);
  return parts.join(' · ');
}

const TOKENS_PER_PRICE_UNIT = 1000000;

function calculateEventCost(row, prices) {
  if (!prices || Object.keys(prices).length === 0) return null;
  const model = row.resolved_model || row.model || '';
  const price = prices[model] || prices[row.model || ''];
  if (!price) return null;
  const inputTokens = Math.max(Number(row.input_tokens || 0), 0);
  const outputTokens = Math.max(Number(row.output_tokens || 0), 0);
  const cachedTokens = Math.max(Number(row.cached_tokens || 0), 0);
  const cacheReadTokens = Math.max(Number(row.cache_read_tokens || 0), 0);
  const cacheCreationTokens = Math.max(Number(row.cache_creation_tokens || 0), 0);
  const promptPrice = Number(price.prompt) || 0;
  const completionPrice = Number(price.completion) || 0;
  let standardCost = 0;
  if (cacheReadTokens > 0 || cacheCreationTokens > 0) {
    const cacheReadPrice = Number(price.cacheRead) || Number(price.cache) || 0;
    const cacheCreationPrice = Number(price.cacheCreation) || promptPrice;
    const promptTokens = Math.max(inputTokens - cachedTokens, 0);
    standardCost =
        (promptTokens / TOKENS_PER_PRICE_UNIT) * promptPrice +
        (outputTokens / TOKENS_PER_PRICE_UNIT) * completionPrice +
        (cachedTokens / TOKENS_PER_PRICE_UNIT) * (Number(price.cache) || 0) +
        (cacheReadTokens / TOKENS_PER_PRICE_UNIT) * cacheReadPrice +
        (cacheCreationTokens / TOKENS_PER_PRICE_UNIT) * cacheCreationPrice;
  } else {
    const promptTokens = Math.max(inputTokens - cachedTokens, 0);
    standardCost =
        (promptTokens / TOKENS_PER_PRICE_UNIT) * promptPrice +
        (outputTokens / TOKENS_PER_PRICE_UNIT) * completionPrice +
        (cachedTokens / TOKENS_PER_PRICE_UNIT) * (Number(price.cache) || 0);
  }
  const serviceTier = row.service_tier || '';
  const multiplier = getServiceTierMultiplier(model || row.model, serviceTier);
  const total = standardCost * multiplier;
  return Number.isFinite(total) && total > 0 ? total : 0;
}

function getServiceTierMultiplier(model, tier) {
  if (!tier) return 1;
  const t = String(tier).trim().toLowerCase();
  if (!t || t === 'default' || t === 'standard') return 1;
  const m = String(model || '').toLowerCase();
  if (t === 'priority') {
    if (m.includes('gpt-5.5')) return 2.5;
    if (m.includes('gpt-5.4-mini')) return 2;
    if (m.includes('gpt-5.4')) return 2;
    return 2;
  }
  return 1;
}

function maskApiKey(value) {
  const s = String(value || '').trim();
  if (!s) return '—';
  if (s.startsWith('sk') && s.length > 8) return `${s.slice(0, 2)}******${s.slice(-2)}`;
  return shortHash(s);
}

function barWidth(value) {
  return `${Math.max(2, Math.round((Number(value || 0) / maxTimelineCalls.value) * 100))}%`;
}

function pretty(v) {
  return JSON.stringify(v ?? {}, null, 2);
}

function defaultFilters() {
  return {status: 'all', provider: 'all', model: 'all', account: 'all', apiKeyHash: 'all'};
}

function startOfTodayMs() {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

function toLocalInput(ms) {
  const d = new Date(ms);
  const pad = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function shouldUseHour(fromMs, toMs) {
  return toMs - fromMs <= 48 * 3600000;
}

function pageRows(rows, page, size) {
  return rows.slice((page - 1) * size, page * size);
}

function unique(values) {
  return Array.from(new Set(values.map(v => String(v || '').trim()).filter(Boolean))).sort();
}

function uniqueObjects(items) {
  const seen = new Set();
  return items.filter(item => item.value && !seen.has(item.value) && seen.add(item.value));
}

function fmtInt(v) {
  const n = Number(v || 0);
  return Number.isFinite(n) ? new Intl.NumberFormat('zh-CN').format(n) : '—';
}

function fmtPct(v) {
  if (v == null || Number.isNaN(Number(v))) return '—';
  const n = Number(v);
  return `${(n <= 1 ? n * 100 : n).toFixed(1)}%`;
}

function fmtMoney(v) {
  if (v == null || Number.isNaN(Number(v))) return '—';
  return '$' + Number(v).toFixed(4);
}

function fmtMs(v) {
  if (v == null || Number.isNaN(Number(v))) return '—';
  return `${Math.round(Number(v))} ms`;
}

function fmtDuration(v) {
  const n = Number(v);
  if (v == null || !Number.isFinite(n)) return '—';
  if (n < 1000) return `${Math.round(n)} ms`;
  const sec = n / 1000;
  if (sec < 60) return `${sec.toFixed(sec < 10 ? 1 : 0)} s`;
  const min = Math.floor(sec / 60);
  const rem = Math.round(sec % 60);
  return `${min}m ${rem}s`;
}

function fmtSeconds(v) {
  if (v == null || Number.isNaN(Number(v))) return '—';
  return `${(Number(v) / 1000).toFixed(Number(v) >= 10000 ? 1 : 2)} s`;
}

function fmtTps(v) {
  if (v == null || Number.isNaN(Number(v))) return '—';
  return Number(v).toFixed(Number(v) >= 10 ? 0 : 1);
}

function fmtCompact(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return '—';
  if (Math.abs(n) >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
  if (Math.abs(n) >= 1000) return `${(n / 1000).toFixed(n >= 10000 ? 1 : 1)}K`;
  return fmtInt(n);
}

function successRateClass(v) {
  const n = Number(v);
  if (!Number.isFinite(n)) return '';
  return n >= 0.95 ? 'good-text' : n >= 0.85 ? 'warn-text' : 'bad-text';
}

function latencyTone(v) {
  const n = Number(v);
  if (!Number.isFinite(n)) return 'good';
  return n >= 30000 ? 'bad' : n >= 10000 ? 'warn' : 'good';
}

function latencyClass(v) {
  return `${latencyTone(v)}-text`;
}

function formatDateTime(ms) {
  if (!ms) return '—';
  return new Date(Number(ms)).toLocaleString('zh-CN', {hour12: false});
}

function formatDate(ms) {
  if (!ms) return '—';
  return new Date(Number(ms)).toLocaleDateString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit'
  }).replaceAll('/', '/');
}

function formatTime(ms) {
  if (!ms) return '—';
  return new Date(Number(ms)).toLocaleTimeString('zh-CN', {hour12: false});
}

function shortHash(v) {
  const s = String(v || '').trim();
  return s.length > 14 ? `${s.slice(0, 7)}…${s.slice(-5)}` : (s || '—');
}

function pickObject(obj, keys) {
  return Object.fromEntries(keys.map(k => [k, obj?.[k]]).filter(([, v]) => v !== undefined));
}

function csvCell(v) {
  const s = v == null ? '' : String(v);
  return /[",\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s;
}

defineExpose({refresh});

const SimpleTable = defineComponent({
  props: {
    rows: {type: Array, default: () => []},
    columns: {type: Array, default: () => []},
    selectable: {type: Boolean, default: false},
    selectedId: {type: [String, Number], default: ''}
  },
  emits: ['select'],
  setup(props, {emit}) {
    return () => {
      if (!props.rows.length) return h('div', {class: 'empty'}, '暂无数据');
      const head = h('thead', h('tr', props.columns.map(col => h('th', col[1]))));
      const body = h('tbody', props.rows.slice(0, 250).map((row, idx) => {
        const rowId = row.id || row.model || row.api_key_hash || row.account_snapshot || idx;
        const isSelected = props.selectedId && String(props.selectedId) === String(rowId);
        return h('tr', {
              class: props.selectable ? ['clickable', isSelected ? 'selected-row' : ''].filter(Boolean).join(' ') : 'clickable',
              key: idx,
              onClick: () => emit('select', row)
            }, props.columns.map(col => h('td', renderCell(row[col[0]], col[2])))
        );
      }));
      return h('div', {class: 'table-wrap monitor-table'}, h('table', [head, body]));
    };
  }
});
const DetailGrid = defineComponent({
  props: {items: {type: Array, default: () => []}},
  setup(props) {
    return () => h('div', {class: 'config-meta-grid'}, props.items.map((item, idx) =>
        h('div', {key: idx}, [h('span', item.label), h('strong', item.value)])
    ));
  }
});
const selectedAccount = computed(() => accountRows.value.find(r => (r.id || r.account_snapshot || r.account || '') === selectedAccountId.value) || null);

function buildAccountDetail(row) {
  if (!row) return [];
  return [
    {label: '账号', value: row.account_snapshot || row.auth_label_snapshot || row.id || '—'},
    {label: 'Provider', value: row.auth_provider_snapshot || '—'},
    {label: '标签', value: row.auth_label_snapshot || '—'},
    {label: '请求', value: fmtInt(row.calls)},
    {label: '成功率', value: fmtPct(row.success_rate)},
    {label: 'Token', value: fmtCompact(row.total_tokens)},
    {label: '费用', value: fmtMoney(row.cost)},
    {label: '延迟', value: fmtMs(row.average_latency_ms)},
    {
      label: '最后出现',
      value: row.last_seen_ms ? new Date(Number(row.last_seen_ms)).toLocaleString('zh-CN', {hour12: false}) : '—'
    },
    {
      label: 'Plan Type',
      value: (row.plan_type || row.planType || row.auth_provider_snapshot && (row.plan_type || row.planType) ? (row.plan_type || row.planType) : '—')
    },
  ];
}

const PaginationBar = defineComponent({
  props: {page: Number, pageSize: Number, total: Number},
  emits: ['page'],
  setup(props, {emit}) {
    return () => {
      const pages = Math.max(1, Math.ceil((props.total || 0) / (props.pageSize || 50)));
      return h('div', {class: 'pager'}, [
        h('span', `第 ${props.page} / ${pages} 页 · ${props.total || 0} 条`),
        h('button', {class: 'btn', disabled: props.page <= 1, onClick: () => emit('page', props.page - 1)}, '上一页'),
        h('button', {
          class: 'btn',
          disabled: props.page >= pages,
          onClick: () => emit('page', props.page + 1)
        }, '下一页'),
      ]);
    };
  }
});

function renderCell(v, type) {
  if (type === 'pct') return fmtPct(v);
  if (type === 'money') return fmtMoney(v);
  if (type === 'ms') return fmtMs(v);
  if (type === 'time') return formatDateTime(v);
  if (type === 'int') return fmtInt(v);
  if (type === 'hash') return shortHash(v);
  if (Array.isArray(v)) return v.join(', ');
  if (v && typeof v === 'object') return JSON.stringify(v);
  return v == null || v === '' ? '—' : String(v);
}
</script>
