<template>
  <section class="monitoring-page">
    <div class="card filter-card monitoring-filterbar">
      <div class="filterbar-title">
        <div class="eyebrow">{{ t('monitoring.eyebrow') }}</div>
        <h2>{{ t('monitoring.title') }}</h2>
      </div>
      <div class="filterbar-controls primary-filters">
        <select v-model="timeRange" class="control compact">
          <option value="today">{{ t('monitoring.timeRange.today') }}</option>
          <option value="7d">{{ t('monitoring.timeRange.d7') }}</option>
          <option value="14d">{{ t('monitoring.timeRange.d14') }}</option>
          <option value="30d">{{ t('monitoring.timeRange.d30') }}</option>
          <option value="all">{{ t('monitoring.timeRange.all') }}</option>
          <option value="custom">{{ t('monitoring.timeRange.custom') }}</option>
        </select>
        <select v-model.number="autoRefreshMs" class="control compact">
          <option :value="0">{{ t('monitoring.autoRefresh.off') }}</option>
          <option :value="5000">{{ t('monitoring.autoRefresh.seconds', { n: 5 }) }}</option>
          <option :value="15000">{{ t('monitoring.autoRefresh.seconds', { n: 15 }) }}</option>
          <option :value="30000">{{ t('monitoring.autoRefresh.seconds', { n: 30 }) }}</option>
          <option :value="60000">{{ t('monitoring.autoRefresh.seconds', { n: 60 }) }}</option>
        </select>
        <input v-model.trim="searchQuery" class="control wide"
               :placeholder="t('monitoring.searchPlaceholder')" @keyup.enter="refresh(true)"/>
      </div>
      <div class="filterbar-actions">
        <button class="btn primary" @click="refresh(true)" :disabled="loading || !ready">{{
            loading ? t('common.loading') : t('common.refresh')
          }}
        </button>
        <button class="btn" @click="exportEventsCsv" :disabled="!eventRows.length">{{ t('monitoring.exportCsv') }}</button>
        <button class="btn" @click="resetFilters">{{ t('monitoring.reset') }}</button>
      </div>
      <div class="filterbar-controls secondary-filters">
        <select v-model="filters.status" class="control compact">
          <option value="all">{{ t('monitoring.filters.allStatuses') }}</option>
          <option value="success">{{ t('monitoring.filters.successOnly') }}</option>
          <option value="failed">{{ t('monitoring.filters.failedOnly') }}</option>
        </select>
        <select v-model="filters.provider" class="control compact">
          <option value="all">{{ t('monitoring.filters.allProviders') }}</option>
          <option v-for="item in optionProviders" :key="item" :value="item">{{ item }}</option>
        </select>
        <select v-model="filters.model" class="control compact">
          <option value="all">{{ t('monitoring.filters.allModels') }}</option>
          <option v-for="item in optionModels" :key="item" :value="item">{{ item }}</option>
        </select>
        <select v-model="filters.account" class="control compact">
          <option value="all">{{ t('monitoring.filters.allAccounts') }}</option>
          <option v-for="item in optionAccounts" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
        <select v-model="filters.apiKeyHash" class="control compact">
          <option value="all">{{ t('monitoring.filters.allApiKeys') }}</option>
          <option v-for="item in optionApiKeys" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
      </div>
    </div>

    <div v-if="timeRange === 'custom'" class="card filter-card custom-range-bar">
      <label>{{ t('monitoring.customStart') }} <input v-model="customStart" type="datetime-local" class="control"/></label>
      <label>{{ t('monitoring.customEnd') }} <input v-model="customEnd" type="datetime-local" class="control"/></label>
      <button class="btn" @click="refresh(true)">{{ t('common.apply') }}</button>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>
    <section v-if="!ready" class="notice">{{ t('monitoring.missingKey') }}</section>

    <MetricGrid :cards="summaryCards"/>

    <div class="monitor-tabs card">
      <button v-for="tab in dataTabs" :key="tab.key" :class="['tab', {active: activeDataTab === tab.key}]"
              @click="activeDataTab = tab.key">{{ tab.label }} <span>{{ tab.count }}</span></button>
    </div>

    <DataCard v-if="activeDataTab === 'timeline'" :title="t('monitoring.cards.timeline')">
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
      <div v-else class="empty">{{ t('monitoring.empty.timeline') }}</div>
    </DataCard>

    <DataCard v-if="activeDataTab === 'events'" :title="t('monitoring.cards.events')">
      <div class="table-wrap monitor-table event-stream-table">
        <table>
          <thead>
          <tr>
            <th>{{ t('monitoring.eventColumns.sourceApiKey') }}</th>
            <th>{{ t('monitoring.eventColumns.model') }}</th>
            <th>{{ t('monitoring.eventColumns.intensity') }}</th>
            <th>{{ t('monitoring.eventColumns.recentStatus') }}</th>
            <th>{{ t('monitoring.eventColumns.requestStatus') }}</th>
            <th>{{ t('monitoring.eventColumns.successRate') }}</th>
            <th>{{ t('monitoring.eventColumns.totalCalls') }}</th>
            <th>{{ t('monitoring.eventColumns.tps') }}</th>
            <th>{{ t('monitoring.eventColumns.ttftLatency') }}</th>
            <th>{{ t('monitoring.eventColumns.time') }}</th>
            <th>{{ t('monitoring.eventColumns.usage') }}</th>
            <th>{{ t('monitoring.eventColumns.cost') }}</th>
          </tr>
          </thead>
          <tbody>
          <tr v-for="row in pagedEvents" :key="row.id" @click="selectedEvent = row.raw" class="clickable">
            <td>
              <strong v-if="row.sourceIsApiKey" class="sensitive-value">
                {{ eventApiKeyDisplay(row.sourceName, isEventKeyExpanded(row, 'source')) }}
                <button
                  type="button"
                  class="sensitive-value-toggle"
                  :aria-expanded="isEventKeyExpanded(row, 'source')"
                  :aria-label="isEventKeyExpanded(row, 'source') ? t('monitoring.labels.collapseApiKey') : t('monitoring.labels.expandApiKey')"
                  @click.stop="toggleEventKey(row, 'source')"
                >{{ isEventKeyExpanded(row, 'source') ? t('monitoring.labels.collapse') : t('monitoring.labels.expand') }}</button>
              </strong>
              <strong v-else>{{ row.sourceName }}</strong>
              <div class="muted small-text">{{ t('monitoring.labels.provider', { value: row.provider }) }}</div>
            </td>
            <td>
              <strong>{{ row.model }}</strong>
              <div v-if="row.resolvedModel && row.resolvedModel !== row.model" class="muted small-text">
                {{ row.resolvedModel }}
              </div>
            </td>
            <td>
              <strong :class="{'blue-text': row.intensity !== '-'}">{{ row.intensity }}</strong>
              <div class="muted small-text">{{ t('monitoring.labels.level', { value: row.tier }) }}</div>
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
                  <i></i>{{ t('monitoring.labels.failed') }}
                </span>
              <span v-else class="status-badge good"><i></i>{{ t('monitoring.labels.success') }}</span>
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
          <button class="failure-tooltip-copy" @click.stop="copyFailureText" :title="t('monitoring.labels.copy')">⎘</button>
          <div v-if="failureTooltip.row?.failStatusCode" class="failure-tooltip-status">HTTP
            {{ failureTooltip.row.failStatusCode }}
          </div>
          <div v-if="failureTooltip.row?.failSummary" class="failure-tooltip-body">
            {{ decodeHtmlEntities(failureTooltip.row.failSummary) }}
          </div>
        </div>
      </Teleport>
    </DataCard>

    <DataCard v-if="activeDataTab === 'accounts'" :title="t('monitoring.cards.accounts')" :subtitle="t('monitoring.cards.accountsSubtitle')">
      <div v-if="accountApiKeyRows.length" class="table-wrap monitor-table account-api-key-table">
        <table>
          <thead>
          <tr>
            <th>{{ t('monitoring.accountColumns.accountApiKey') }}</th>
            <th>{{ t('monitoring.accountColumns.provider') }}</th>
            <th>{{ t('monitoring.accountColumns.requests') }}</th>
            <th>{{ t('monitoring.accountColumns.successRate') }}</th>
            <th>{{ t('monitoring.accountColumns.token') }}</th>
            <th>{{ t('monitoring.accountColumns.cost') }}</th>
            <th>{{ t('monitoring.accountColumns.latency') }}</th>
            <th>{{ t('monitoring.accountColumns.lastSeen') }}</th>
            <th>{{ t('monitoring.accountColumns.actions') }}</th>
          </tr>
          </thead>
          <tbody>
          <tr v-for="row in accountApiKeyRows" :key="row.id" :class="['clickable', { 'selected-row': row.id === selectedAccountId }]"
              @click="selectAccountAPIKey(row)">
            <td>
              <strong v-if="isAccountSourceApiKey(row)" class="sensitive-value">
                {{ eventApiKeyDisplay(accountSource(row), isAccountSourceExpanded(row)) }}
                <button
                  type="button"
                  class="sensitive-value-toggle"
                  :aria-expanded="isAccountSourceExpanded(row)"
                  :aria-label="isAccountSourceExpanded(row) ? t('monitoring.labels.collapseApiKey') : t('monitoring.labels.expandApiKey')"
                  @click.stop="toggleAccountSource(row)"
                >{{ isAccountSourceExpanded(row) ? t('monitoring.labels.collapse') : t('monitoring.labels.expand') }}</button>
              </strong>
              <strong v-else>{{ accountSource(row) }}</strong>
            </td>
            <td>{{ row.auth_provider_snapshot || EMPTY_VALUE }}</td>
            <td>{{ fmtInt(row.calls) }}</td>
            <td><strong :class="successRateClass(row.success_rate)">{{ fmtPct(row.success_rate) }}</strong></td>
            <td>{{ fmtCompact(row.total_tokens) }}</td>
            <td>{{ fmtMoney(row.cost) }}</td>
            <td>{{ fmtDuration(row.average_latency_ms) }}</td>
            <td>{{ formatDateTime(row.last_seen_ms) }}</td>
            <td><button type="button" class="btn btn-xs" @click.stop="filterAccountAPIKey(row)">{{ t('monitoring.labels.filter') }}</button></td>
          </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty">{{ t('monitoring.empty.accounts') }}</div>
    </DataCard>

    <div v-if="activeDataTab === 'accounts' && selectedAccount" style="margin-top:16px">
      <DataCard :title="t('monitoring.cards.sourceDetail')" :subtitle="accountDetailSubtitle(selectedAccount)">
        <DetailGrid :items="buildAccountDetail(selectedAccount)"/>
      </DataCard>
    </div>

    <DataCard v-if="activeDataTab === 'models'" :title="t('monitoring.cards.models')" :subtitle="t('monitoring.cards.modelsSubtitle')">
      <SimpleTable :rows="modelRows" :columns="modelColumns" @select="setModelFilter"/>
    </DataCard>

    <div v-if="selectedEvent" class="modal-backdrop" @click.self="selectedEvent = null">
      <div class="modal-dialog card">
        <div class="modal-head">
          <div><h2>{{ t('monitoring.cards.requestDetail') }}</h2>
            <p class="muted">{{ formatDateTime(selectedEvent.timestamp_ms) }} ·
              {{ selectedEvent.event_hash || selectedEvent.request_id || EMPTY_VALUE }}</p></div>
          <button class="btn" @click="selectedEvent = null">{{ t('common.close') }}</button>
        </div>
        <MetricGrid :cards="eventDetailCards"/>
        <div class="detail-grid">
          <div><h3>{{ t('monitoring.labels.basic') }}</h3>
            <pre>{{ pretty(eventBaseDetail) }}</pre>
          </div>
          <div><h3>{{ t('monitoring.labels.responseMetadata') }}</h3>
            <pre>{{ pretty(selectedEvent.response_metadata || {}) }}</pre>
          </div>
          <div><h3>{{ t('monitoring.labels.errorQuotaTrace') }}</h3>
            <pre>{{ pretty(eventHeaderDetail) }}</pre>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import {computed, defineComponent, h, onBeforeUnmount, onMounted, ref, watch} from 'vue';
import {useI18n} from 'vue-i18n';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';
import { eventApiKeyDisplay, isSensitiveSource, maskSecretSummary, shortHash } from '../utils/apiKeyDisplay.js';
import { EMPTY_VALUE, formatDate, formatDateTime, formatInt, formatTime } from '../utils/localeFormat.js';

const props = defineProps({
  ready: {type: Boolean, default: false},
  proxyCall: {type: Function, required: true},
});

const {t} = useI18n();

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
const expandedEventKeys = ref(new Set());
const expandedAccountSources = ref(new Set());
const filters = ref(defaultFilters());
const failureTooltip = ref({visible: false, row: null, style: {}});
let failureHideTimer = null;
let timer = null;

const dataTabs = computed(() => [
  {key: 'events', label: t('monitoring.tabs.events'), count: eventRows.value.length},
  {key: 'accounts', label: t('monitoring.tabs.accounts'), count: accountApiKeyRows.value.length},
  {key: 'models', label: t('monitoring.tabs.models'), count: modelRows.value.length},
  {key: 'timeline', label: t('monitoring.tabs.timeline'), count: timelineRows.value.length},
]);

const summary = computed(() => data.value?.summary || {});
const eventRows = computed(() => (data.value?.events?.items || []).map((row, idx) => ({...row, __id: idx})));
const hasPrices = computed(() => Object.keys(modelPrices.value).length > 0);
const summaryCards = computed(() => {
  const s = summary.value;
  const totalCacheTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const cacheHitTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0);
  const inputSideTokens = Math.max(Number(s.input_tokens ?? 0), Number(s.cached_tokens ?? 0)) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const cacheHitRate = inputSideTokens > 0 ? cacheHitTokens / inputSideTokens : 0;
  const tokenMix = (n) => s.total_tokens > 0 ? `${fmtPct(n / s.total_tokens)}` : EMPTY_VALUE;
  return [
    {label: t('monitoring.kpi.totalCalls'), value: fmtInt(s.total_calls), sub: t('monitoring.kpi.accountsSub', {count: accountCount.value})},
    {label: t('monitoring.kpi.successRate'), value: fmtPct(s.success_rate), sub: fmtDuration(s.average_latency_ms)},
    {label: t('monitoring.kpi.failureTotal'), value: fmtInt(s.failure_calls), sub: t('monitoring.kpi.monitorGroupsSub', {count: failedGroupCount.value})},
    {
      label: t('monitoring.kpi.estimatedCost'),
      value: hasPrices.value ? fmtMoney(s.total_cost) : '--',
      sub: hasPrices.value ? t('monitoring.kpi.pricesConfigured') : t('monitoring.kpi.pricesMissing')
    },
    {label: t('monitoring.kpi.totalTokens'), value: fmtCompact(s.total_tokens), sub: t('monitoring.kpi.reasoningSub', {value: fmtCompact(s.reasoning_tokens)})},
    {label: t('monitoring.kpi.inputTokens'), value: fmtCompact(s.input_tokens), sub: t('monitoring.kpi.shareSub', {value: tokenMix(Number(s.input_tokens ?? 0))})},
    {label: t('monitoring.kpi.outputTokens'), value: fmtCompact(s.output_tokens), sub: t('monitoring.kpi.shareSub', {value: tokenMix(Number(s.output_tokens ?? 0))})},
    {label: t('monitoring.kpi.cacheTokens'), value: fmtCompact(totalCacheTokens), sub: t('monitoring.kpi.hitRateSub', {value: fmtPct(cacheHitRate)})},
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
const timelineRows = computed(() => [...(data.value?.timeline || [])].sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0)));
const maxTimelineCalls = computed(() => Math.max(1, ...timelineRows.value.map(p => Number(p.calls || p.requests || 0))));
const modelRows = computed(() => data.value?.model_stats || data.value?.model_share || []);
const channelRows = computed(() => data.value?.channel_share || []);
const accountRows = computed(() => data.value?.account_stats || []);
const apiKeyRows = computed(() => data.value?.api_key_stats || []);
const accountApiKeyRows = computed(() => data.value?.account_api_key_stats || []);
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

const modelColumns = computed(() => [
  ['model', t('monitoring.modelColumns.model')],
  ['calls', t('monitoring.modelColumns.requests')],
  ['success_calls', t('monitoring.modelColumns.success')],
  ['failure_calls', t('monitoring.modelColumns.failure')],
  ['success_rate', t('monitoring.modelColumns.successRate'), 'pct'],
  ['total_tokens', t('monitoring.modelColumns.token'), 'int'],
  ['cost', t('monitoring.modelColumns.cost'), 'money'],
]);

const eventDetailCards = computed(() => selectedEvent.value ? [
  {label: t('monitoring.labels.status'), value: selectedEvent.value.failed ? t('monitoring.labels.failed') : t('monitoring.labels.success')},
  {label: t('monitoring.labels.token'), value: selectedEvent.value.total_tokens ?? 0},
  {label: t('monitoring.labels.latency'), value: fmtMs(selectedEvent.value.latency_ms)},
  {label: t('monitoring.labels.cost'), value: fmtMoney(calculateEventCost(selectedEvent.value, modelPrices.value))},
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

function eventKeyId(row, field) {
  return `${row.id}:${field}`;
}

function isEventKeyExpanded(row, field) {
  return expandedEventKeys.value.has(eventKeyId(row, field));
}

function toggleEventKey(row, field) {
  const key = eventKeyId(row, field);
  const next = new Set(expandedEventKeys.value);
  if (next.has(key)) {
    next.delete(key);
  } else {
    next.add(key);
  }
  expandedEventKeys.value = next;
}

function accountSource(row) {
  return String(row?.source || '').trim() || EMPTY_VALUE;
}

function accountSourceKey(row) {
  return row?.id || accountSource(row);
}

function isAccountSourceApiKey(row) {
  return isSensitiveSource(accountSource(row), row?.auth_type);
}

function accountDetailSubtitle(row) {
  const source = accountSource(row);
  return isSensitiveSource(source, row?.auth_type) ? maskSecretSummary(source) : source;
}

function isAccountSourceExpanded(row) {
  return expandedAccountSources.value.has(accountSourceKey(row));
}

function toggleAccountSource(row) {
  const key = accountSourceKey(row);
  const next = new Set(expandedAccountSources.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  expandedAccountSources.value = next;
}

function selectAccountAPIKey(row) {
  selectedAccountId.value = row.id || '';
}

function filterAccountAPIKey(row) {
  filters.value.account = 'all';
  filters.value.apiKeyHash = 'all';
  const source = accountSource(row);
  searchQuery.value = source === EMPTY_VALUE || source === 'unknown' ? '' : source;
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
  map._slidingWindow = new Map();
  const sw = new Map();
  for (const event of sortedAsc) {
    const key = eventGroupKey(event);
    const prev = sw.get(key) ?? {total: 0, success: 0, pattern: []};
    const statsIncluded = event.failed === true || Number(event.input_tokens || 0) > 0 || Number(event.output_tokens || 0) > 0;
    const requestCount = prev.total + (statsIncluded ? 1 : 0);
    const successCount = prev.success + (statsIncluded && !event.failed ? 1 : 0);
    const pattern = [...prev.pattern, !event.failed].slice(-10);
    sw.set(key, {total: requestCount, success: successCount, pattern});
    if (!map._slidingWindow.has(key)) map._slidingWindow.set(key, new Map());
    map._slidingWindow.get(key).set(event.event_hash || event.request_id || `${event.timestamp_ms}-${event.__id}`, {
      requestCount,
      successRate: requestCount > 0 ? successCount / requestCount : 1,
      recentPattern: pattern,
    });
  }
  return map;
}

function buildEventTableRow(row, groupMap) {
  const key = eventGroupKey(row);
  const eventId = row.event_hash || row.request_id || `${row.timestamp_ms}-${row.__id}`;
  const sliding = groupMap._slidingWindow?.get(key)?.get(eventId);
  const latencyMs = numberOrNull(row.latency_ms);
  const outputTokens = Number(row.output_tokens || 0);
  const sourceName = String(row.source || '').trim() || EMPTY_VALUE;
  return {
    id: eventId,
    raw: row,
    sourceName,
    sourceIsApiKey: isSensitiveSource(sourceName, row.auth_type),
    provider: row.auth_provider_snapshot || row.provider || EMPTY_VALUE,
    apiKeyHash: row.api_key_hash || EMPTY_VALUE,
    model: row.model || EMPTY_VALUE,
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
  const source = String(row.source || '').trim() || 'unknown';
  const provider = row.auth_provider_snapshot || row.provider || '';
  const model = row.model || '';
  return [source, provider, model].join('::');
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
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  return formatInt(n);
}

function fmtPct(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  const n = Number(v);
  return `${(n <= 1 ? n * 100 : n).toFixed(1)}%`;
}

function fmtMoney(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  return '$' + Number(v).toFixed(4);
}

function fmtMs(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  return `${Math.round(Number(v))} ms`;
}

function fmtDuration(v) {
  const n = Number(v);
  if (v == null || !Number.isFinite(n)) return EMPTY_VALUE;
  if (n < 1000) return `${Math.round(n)} ms`;
  const sec = n / 1000;
  if (sec < 60) return `${sec.toFixed(sec < 10 ? 1 : 0)} s`;
  const min = Math.floor(sec / 60);
  const rem = Math.round(sec % 60);
  return `${min}m ${rem}s`;
}

function fmtSeconds(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  return `${(Number(v) / 1000).toFixed(Number(v) >= 10000 ? 1 : 2)} s`;
}

function fmtTps(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  return Number(v).toFixed(Number(v) >= 10 ? 0 : 1);
}

function fmtCompact(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  if (Math.abs(n) >= 1000000) return `${(n / 1000000).toFixed(1)}M`;
  if (Math.abs(n) >= 1000) return `${(n / 1000).toFixed(1)}K`;
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
    const {t: ti18n} = useI18n();
    return () => {
      if (!props.rows.length) return h('div', {class: 'empty'}, ti18n('common.noData'));
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
    const {t: ti18n} = useI18n();
    return () => h('div', {class: 'config-meta-grid'}, props.items.map((item, idx) => {
      const value = item.sensitive
        ? h('strong', {class: 'sensitive-value sensitive-value-block'}, [
            h('code', {class: 'sensitive-value-content'}, item.value),
            h('button', {
              type: 'button',
              class: 'sensitive-value-toggle',
              'aria-expanded': item.expanded,
              'aria-label': item.expanded ? ti18n('monitoring.labels.collapseApiKey') : ti18n('monitoring.labels.expandApiKey'),
              onClick: event => {
                event.stopPropagation();
                item.onToggle?.();
              },
            }, item.expanded ? ti18n('monitoring.labels.collapse') : ti18n('monitoring.labels.expand')),
          ])
        : h('strong', {class: 'config-meta-value'}, item.value);
      return h('div', {key: idx, class: item.wide ? 'config-field-wide' : ''}, [h('span', item.label), value]);
    }));
  }
});
const selectedAccount = computed(() => accountApiKeyRows.value.find(r => r.id === selectedAccountId.value) || null);

function buildAccountDetail(row) {
  if (!row) return [];
  const source = accountSource(row);
  const sensitive = isSensitiveSource(source, row.auth_type);
  const expanded = isAccountSourceExpanded(row);
  return [
    {
      label: t('monitoring.labels.source'),
      value: sensitive ? eventApiKeyDisplay(source, expanded) : source,
      wide: true,
      sensitive,
      expanded,
      onToggle: sensitive ? () => toggleAccountSource(row) : null,
    },
    {label: t('monitoring.accountColumns.provider'), value: row.auth_provider_snapshot || EMPTY_VALUE, wide: true},
    {label: t('monitoring.labels.requests'), value: fmtInt(row.calls)},
    {label: t('monitoring.labels.successRate'), value: fmtPct(row.success_rate)},
    {label: t('monitoring.labels.token'), value: fmtCompact(row.total_tokens)},
    {label: t('monitoring.labels.cost'), value: fmtMoney(row.cost)},
    {label: t('monitoring.labels.latency'), value: fmtMs(row.average_latency_ms)},
    {
      label: t('monitoring.labels.lastSeen'),
      value: formatDateTime(row.last_seen_ms)
    },
    {
      label: t('monitoring.labels.planType'),
      value: (row.plan_type || row.planType || EMPTY_VALUE)
    },
  ];
}

const PaginationBar = defineComponent({
  props: {page: Number, pageSize: Number, total: Number},
  emits: ['page'],
  setup(props, {emit}) {
    const {t: ti18n} = useI18n();
    return () => {
      const pages = Math.max(1, Math.ceil((props.total || 0) / (props.pageSize || 50)));
      return h('div', {class: 'pager'}, [
        h('span', ti18n('monitoring.pagination.summary', {page: props.page, pages, total: props.total || 0})),
        h('button', {class: 'btn', disabled: props.page <= 1, onClick: () => emit('page', props.page - 1)}, ti18n('monitoring.pagination.prev')),
        h('button', {
          class: 'btn',
          disabled: props.page >= pages,
          onClick: () => emit('page', props.page + 1)
        }, ti18n('monitoring.pagination.next')),
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
  return v == null || v === '' ? EMPTY_VALUE : String(v);
}
</script>
