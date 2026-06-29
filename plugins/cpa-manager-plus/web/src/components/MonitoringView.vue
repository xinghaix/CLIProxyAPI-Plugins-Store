<template>
  <section class="monitoring-page">
    <div class="monitoring-head card glass">
      <div>
        <div class="eyebrow">REQUEST MONITORING</div>
        <h2>请求监控</h2>
        <p class="muted">迁移自 CPA-Manager-Plus：筛选、摘要、维度分析、事件流、详情和导出均通过插件代理访问 Plus Manager Server。</p>
      </div>
      <div class="monitoring-actions">
        <select v-model="timeRange" class="control">
          <option value="today">今天</option>
          <option value="7d">最近 7 天</option>
          <option value="14d">最近 14 天</option>
          <option value="30d">最近 30 天</option>
          <option value="all">全部</option>
          <option value="custom">自定义</option>
        </select>
        <select v-model.number="autoRefreshMs" class="control">
          <option :value="0">不自动刷新</option>
          <option :value="5000">5 秒</option>
          <option :value="15000">15 秒</option>
          <option :value="30000">30 秒</option>
          <option :value="60000">60 秒</option>
        </select>
        <button class="btn primary" @click="refresh(true)" :disabled="loading || !ready">{{ loading ? '加载中…' : '刷新监控' }}</button>
        <button class="btn" @click="exportEventsCsv" :disabled="!eventRows.length">导出 CSV</button>
      </div>
    </div>

    <div v-if="timeRange === 'custom'" class="card filter-card">
      <label>开始 <input v-model="customStart" type="datetime-local" class="control" /></label>
      <label>结束 <input v-model="customEnd" type="datetime-local" class="control" /></label>
      <button class="btn" @click="refresh(true)">应用自定义时间</button>
    </div>

    <div class="card filter-card">
      <input v-model.trim="searchQuery" class="control wide" placeholder="全文搜索：模型 / 账号 / API Key / 路径 / trace / 错误" @keyup.enter="refresh(true)" />
      <select v-model="filters.status" class="control">
        <option value="all">全部状态</option>
        <option value="success">仅成功</option>
        <option value="failed">仅失败</option>
      </select>
      <select v-model="filters.provider" class="control">
        <option value="all">全部 Provider</option>
        <option v-for="item in optionProviders" :key="item" :value="item">{{ item }}</option>
      </select>
      <select v-model="filters.model" class="control">
        <option value="all">全部模型</option>
        <option v-for="item in optionModels" :key="item" :value="item">{{ item }}</option>
      </select>
      <select v-model="filters.account" class="control">
        <option value="all">全部账号</option>
        <option v-for="item in optionAccounts" :key="item.value" :value="item.value">{{ item.label }}</option>
      </select>
      <select v-model="filters.apiKeyHash" class="control">
        <option value="all">全部 API Key</option>
        <option v-for="item in optionApiKeys" :key="item.value" :value="item.value">{{ item.label }}</option>
      </select>
      <input v-model.trim="filters.projectId" class="control" placeholder="Project ID" @keyup.enter="refresh(true)" />
      <input v-model.trim="filters.requestType" class="control" placeholder="请求类型 / path" @keyup.enter="refresh(true)" />
      <input v-model.number="filters.minLatencyMs" class="control small" type="number" min="0" placeholder="最低延迟 ms" @keyup.enter="refresh(true)" />
      <input v-model.trim="filters.cacheStatus" class="control small" placeholder="缓存状态" @keyup.enter="refresh(true)" />
      <input v-model.trim="filters.headerTraceId" class="control wide" placeholder="Trace ID" @keyup.enter="refresh(true)" />
      <button class="btn" @click="resetFilters">重置筛选</button>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>
    <section v-if="!ready" class="notice">缺少 CPA management key，无法访问插件代理。</section>

    <MetricGrid :cards="summaryCards" />

    <div class="card chart-card">
      <div class="section-title"><h2>时间线</h2><span>{{ data?.granularity || 'auto' }} · {{ formatDateTime(data?.generated_at_ms) }}</span></div>
      <div class="timeline-bars" v-if="timelineRows.length">
        <div v-for="point in timelineRows" :key="point.label + point.bucket_ms" class="timeline-row">
          <span class="timeline-label">{{ point.label }}</span>
          <div class="timeline-track"><i :style="{width: barWidth(point.calls || point.requests || 0)}"></i></div>
          <span class="timeline-value">{{ fmtInt(point.calls || point.requests || 0) }}</span>
          <span class="timeline-sub">{{ fmtInt(point.tokens || point.total_tokens || 0) }} tok</span>
        </div>
      </div>
      <div v-else class="empty">暂无时间线数据</div>
    </div>

    <div class="monitor-tabs card">
      <button v-for="tab in dataTabs" :key="tab.key" :class="['tab', {active: activeDataTab === tab.key}]" @click="activeDataTab = tab.key">{{ tab.label }} <span>{{ tab.count }}</span></button>
    </div>

    <DataCard v-if="activeDataTab === 'events'" title="实时事件" :subtitle="eventsSubtitle">
      <div class="table-wrap monitor-table">
        <table>
          <thead><tr><th>时间</th><th>状态</th><th>模型</th><th>账号 / Key</th><th>路径</th><th>Token</th><th>延迟</th><th>错误 / Trace</th></tr></thead>
          <tbody>
            <tr v-for="row in pagedEvents" :key="row.event_hash || row.request_id || row.__id" @click="selectedEvent = row" class="clickable">
              <td>{{ formatDateTime(row.timestamp_ms) }}</td>
              <td><span :class="['status-badge', row.failed ? 'bad' : 'good']">{{ row.failed ? '失败' : '成功' }}</span></td>
              <td><strong>{{ row.model || '—' }}</strong><div class="muted small-text">{{ row.resolved_model || row.auth_provider_snapshot || '' }}</div></td>
              <td>{{ row.account_snapshot || row.auth_label_snapshot || row.auth_index || '—' }}<div class="muted small-text">{{ shortHash(row.api_key_hash) }}</div></td>
              <td><code>{{ row.method || '' }} {{ row.path || row.endpoint || '—' }}</code></td>
              <td>{{ fmtInt(row.total_tokens) }}</td>
              <td>{{ fmtMs(row.latency_ms) }}</td>
              <td>{{ row.fail_summary || row.header_error_kind || row.header_error_code || row.header_trace_id || '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <PaginationBar :page="eventPage" :page-size="eventPageSize" :total="eventRows.length" @page="eventPage = $event" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'accounts'" title="账号维度" subtitle="account_stats">
      <SimpleTable :rows="accountRows" :columns="accountColumns" @select="setAccountFilter" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'apiKeys'" title="API Key 维度" subtitle="api_key_stats">
      <SimpleTable :rows="apiKeyRows" :columns="apiKeyColumns" @select="setApiKeyFilter" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'models'" title="模型维度" subtitle="model_stats / model_share">
      <SimpleTable :rows="modelRows" :columns="modelColumns" @select="setModelFilter" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'channels'" title="渠道维度" subtitle="channel_share">
      <SimpleTable :rows="channelRows" :columns="channelColumns" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'failures'" title="失败来源" subtitle="failure_sources / recent_failures">
      <SimpleTable :rows="failureRows" :columns="failureColumns" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'tasks'" title="任务桶" subtitle="task_buckets">
      <SimpleTable :rows="taskRows" :columns="taskColumns" />
    </DataCard>

    <DataCard v-if="activeDataTab === 'raw'" title="原始响应" subtitle="debug">
      <pre>{{ pretty(data) }}</pre>
    </DataCard>

    <div v-if="selectedEvent" class="drawer-backdrop" @click.self="selectedEvent = null">
      <aside class="drawer card">
        <div class="drawer-head">
          <div><h2>请求详情</h2><p class="muted">{{ formatDateTime(selectedEvent.timestamp_ms) }} · {{ selectedEvent.event_hash || selectedEvent.request_id || '—' }}</p></div>
          <button class="btn" @click="selectedEvent = null">关闭</button>
        </div>
        <MetricGrid :cards="eventDetailCards" />
        <div class="detail-grid">
          <div><h3>基础</h3><pre>{{ pretty(eventBaseDetail) }}</pre></div>
          <div><h3>响应 Metadata</h3><pre>{{ pretty(selectedEvent.response_metadata || {}) }}</pre></div>
          <div><h3>错误 / Quota / Trace</h3><pre>{{ pretty(eventHeaderDetail) }}</pre></div>
        </div>
      </aside>
    </div>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const data = ref(null);
const loading = ref(false);
const error = ref('');
const timeRange = ref('today');
const customStart = ref(toLocalInput(startOfTodayMs()));
const customEnd = ref(toLocalInput(Date.now()));
const searchQuery = ref('');
const autoRefreshMs = ref(0);
const activeDataTab = ref('events');
const selectedEvent = ref(null);
const eventPage = ref(1);
const eventPageSize = ref(50);
const filters = ref(defaultFilters());
let timer = null;

const dataTabs = computed(() => [
  {key:'events', label:'事件流', count:eventRows.value.length},
  {key:'accounts', label:'账号', count:accountRows.value.length},
  {key:'apiKeys', label:'API Key', count:apiKeyRows.value.length},
  {key:'models', label:'模型', count:modelRows.value.length},
  {key:'channels', label:'渠道', count:channelRows.value.length},
  {key:'failures', label:'失败', count:failureRows.value.length},
  {key:'tasks', label:'任务桶', count:taskRows.value.length},
  {key:'raw', label:'原始', count:data.value ? 1 : 0},
]);

const summary = computed(() => data.value?.summary || {});
const summaryCards = computed(() => [
  {label:'总请求', value: summary.value.total_calls ?? 0, sub:`成功 ${fmtInt(summary.value.success_calls)} / 失败 ${fmtInt(summary.value.failure_calls)}`},
  {label:'成功率', value: fmtPct(summary.value.success_rate), sub:`任务成功 ${fmtPct(summary.value.approx_task_success_rate)}`},
  {label:'总 Token', value: summary.value.total_tokens ?? 0, sub:`输入 ${fmtInt(summary.value.input_tokens)} / 输出 ${fmtInt(summary.value.output_tokens)}`},
  {label:'费用', value: fmtMoney(summary.value.total_cost), sub:`单次 ${fmtMoney(summary.value.average_cost_per_call)}`},
  {label:'平均延迟', value: fmtMs(summary.value.average_latency_ms), sub:`P95 ${fmtMs(summary.value.p95_latency_ms)}`},
  {label:'吞吐', value: `${fmtInt(summary.value.rpm_30m)} RPM`, sub:`${fmtInt(summary.value.tpm_30m)} TPM`},
  {label:'零 Token', value: summary.value.zero_token_calls ?? 0, sub:(summary.value.zero_token_models || []).slice(0,3).join(', ')},
  {label:'近似任务', value: summary.value.approx_tasks ?? 0, sub:`失败 ${fmtInt(summary.value.approx_task_failures)}`},
]);

const eventRows = computed(() => (data.value?.events?.items || []).map((row, idx) => ({...row, __id: idx})));
const pagedEvents = computed(() => pageRows(eventRows.value, eventPage.value, eventPageSize.value));
const eventsSubtitle = computed(() => `events_page · 已载入 ${eventRows.value.length} / ${data.value?.events?.total_count ?? '未知'} · has_more=${Boolean(data.value?.events?.has_more)}`);
const timelineRows = computed(() => data.value?.timeline || []);
const maxTimelineCalls = computed(() => Math.max(1, ...timelineRows.value.map(p => Number(p.calls || p.requests || 0))));
const modelRows = computed(() => data.value?.model_stats || data.value?.model_share || []);
const channelRows = computed(() => data.value?.channel_share || []);
const accountRows = computed(() => data.value?.account_stats || []);
const apiKeyRows = computed(() => data.value?.api_key_stats || []);
const failureRows = computed(() => [...(data.value?.failure_sources || []), ...(data.value?.recent_failures || [])]);
const taskRows = computed(() => data.value?.task_buckets || []);

const optionModels = computed(() => unique([...(data.value?.filter_options?.model_stats || []).map(x => x.model), ...modelRows.value.map(x => x.model)]));
const optionProviders = computed(() => unique([...(data.value?.filter_options?.providers || []), ...eventRows.value.map(x => x.auth_provider_snapshot)]));
const optionAccounts = computed(() => uniqueObjects(accountRows.value.map(row => ({value: row.id || row.account_snapshot || row.account || '', label: row.account_snapshot || row.auth_label_snapshot || row.id || row.account || ''}))));
const optionApiKeys = computed(() => uniqueObjects(apiKeyRows.value.map(row => ({value: row.api_key_hash || row.id || '', label: `${shortHash(row.api_key_hash || row.id)} · ${row.account_snapshot || row.auth_label_snapshot || ''}`}))));

const accountColumns = [
  ['account_snapshot','账号'], ['auth_label_snapshot','标签'], ['auth_provider_snapshot','Provider'], ['calls','请求'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'], ['average_latency_ms','延迟','ms'], ['last_seen_ms','最后出现','time']
];
const apiKeyColumns = [
  ['api_key_hash','API Key','hash'], ['account_snapshot','账号'], ['auth_label_snapshot','标签'], ['calls','请求'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'], ['last_seen_ms','最后出现','time']
];
const modelColumns = [
  ['model','模型'], ['calls','请求'], ['success_calls','成功'], ['failure_calls','失败'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money']
];
const channelColumns = [
  ['auth_index','Auth'], ['source','Source'], ['account_snapshot','账号'], ['auth_provider_snapshot','Provider'], ['calls','请求'], ['success','成功'], ['failure','失败'], ['tokens','Token','int'], ['cost','费用','money'], ['average_latency_ms','延迟','ms']
];
const failureColumns = [
  ['timestamp_ms','时间','time'], ['source','Source'], ['source_hash','Source Hash','hash'], ['auth_index','Auth'], ['model','模型'], ['calls','请求'], ['failure','失败'], ['fail_summary','错误'], ['header_error_kind','错误类型'], ['last_seen_ms','最后出现','time']
];
const taskColumns = [
  ['bucket_key','任务'], ['source','Source'], ['auth_index','Auth'], ['total','请求'], ['success','成功'], ['failure','失败'], ['models','模型'], ['total_tokens','Token','int'], ['average_latency_ms','延迟','ms'], ['last_ms','结束','time']
];

const eventDetailCards = computed(() => selectedEvent.value ? [
  {label:'状态', value:selectedEvent.value.failed ? '失败' : '成功'},
  {label:'Token', value:selectedEvent.value.total_tokens ?? 0},
  {label:'延迟', value:fmtMs(selectedEvent.value.latency_ms)},
  {label:'费用', value:fmtMoney(selectedEvent.value.cost || selectedEvent.value.total_cost)},
] : []);
const eventBaseDetail = computed(() => selectedEvent.value ? pickObject(selectedEvent.value, ['request_id','event_hash','timestamp_ms','model','resolved_model','endpoint','method','path','auth_index','source','source_hash','api_key_hash','account_snapshot','auth_label_snapshot','auth_provider_snapshot','auth_project_id_snapshot','input_tokens','output_tokens','cached_tokens','cache_read_tokens','cache_creation_tokens','reasoning_tokens','total_tokens','latency_ms','ttft_ms','failed','fail_status_code','fail_summary']) : {});
const eventHeaderDetail = computed(() => selectedEvent.value ? pickObject(selectedEvent.value, ['header_quota_recover_at_ms','header_quota_used_percent','header_quota_plan_type','header_error_kind','header_error_code','header_trace_id']) : {});

watch([timeRange, searchQuery, filters], () => { eventPage.value = 1; }, {deep:true});
watch(autoRefreshMs, setupTimer);
watch(() => props.ready, (ready) => { if(ready && !data.value) refresh(true); });
onMounted(() => { if(props.ready) refresh(true); });
onBeforeUnmount(() => clearTimer());

async function refresh(force=false){
  if(!props.ready) return;
  if(loading.value && !force) return;
  loading.value = true;
  error.value = '';
  try{
    const request = buildAnalyticsRequest();
    data.value = await props.proxyCall({method:'POST', path:'/v0/management/monitoring/analytics', body:request});
    selectedEvent.value = null;
    setupTimer();
  }catch(e){
    error.value = e.message || String(e);
  }finally{
    loading.value = false;
  }
}
function buildAnalyticsRequest(){
  const {fromMs, toMs} = resolveRange();
  const f = {};
  if(filters.value.model !== 'all') f.models = [filters.value.model];
  if(filters.value.provider !== 'all') f.providers = [filters.value.provider];
  if(filters.value.account !== 'all') f.accounts = [filters.value.account];
  if(filters.value.apiKeyHash !== 'all') f.api_key_hashes = [filters.value.apiKeyHash];
  if(filters.value.projectId) f.project_ids = [filters.value.projectId];
  if(filters.value.requestType) f.request_types = [filters.value.requestType];
  if(filters.value.status === 'success') f.include_failed = false;
  if(filters.value.status === 'failed') f.failed_only = true;
  if(Number(filters.value.minLatencyMs) > 0) f.min_latency_ms = Number(filters.value.minLatencyMs);
  if(filters.value.cacheStatus) f.cache_status = filters.value.cacheStatus;
  if(filters.value.headerTraceId) f.header_trace_ids = [filters.value.headerTraceId];
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
  if(searchQuery.value) request.search_query = searchQuery.value;
  if(Object.keys(f).length) request.filters = f;
  return request;
}
function resolveRange(){
  const now = Date.now();
  if(timeRange.value === 'today') return {fromMs:startOfTodayMs(), toMs:now};
  if(timeRange.value === '7d') return {fromMs:now - 7*86400000, toMs:now};
  if(timeRange.value === '14d') return {fromMs:now - 14*86400000, toMs:now};
  if(timeRange.value === '30d') return {fromMs:now - 30*86400000, toMs:now};
  if(timeRange.value === 'custom') return {fromMs:Date.parse(customStart.value), toMs:Date.parse(customEnd.value)};
  return {fromMs:0, toMs:now};
}
function resetFilters(){ filters.value = defaultFilters(); searchQuery.value = ''; refresh(true); }
function setAccountFilter(row){ filters.value.account = row.id || row.account_snapshot || row.account || 'all'; refresh(true); }
function setApiKeyFilter(row){ filters.value.apiKeyHash = row.api_key_hash || row.id || 'all'; refresh(true); }
function setModelFilter(row){ filters.value.model = row.model || 'all'; refresh(true); }
function setupTimer(){ clearTimer(); if(autoRefreshMs.value > 0) timer = window.setInterval(() => refresh(false), autoRefreshMs.value); }
function clearTimer(){ if(timer) window.clearInterval(timer); timer = null; }
function exportEventsCsv(){
  const cols = ['timestamp_ms','failed','model','auth_index','account_snapshot','api_key_hash','method','path','total_tokens','latency_ms','fail_status_code','fail_summary','header_trace_id'];
  const csv = [cols.join(','), ...eventRows.value.map(row => cols.map(c => csvCell(row[c])).join(','))].join('\n');
  const blob = new Blob([csv], {type:'text/csv;charset=utf-8'});
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `monitoring-events-${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}
function barWidth(value){ return `${Math.max(2, Math.round((Number(value || 0) / maxTimelineCalls.value) * 100))}%`; }
function pretty(v){ return JSON.stringify(v ?? {}, null, 2); }
function defaultFilters(){ return {status:'all', provider:'all', model:'all', account:'all', apiKeyHash:'all', projectId:'', requestType:'', minLatencyMs:'', cacheStatus:'', headerTraceId:''}; }
function startOfTodayMs(){ const d = new Date(); d.setHours(0,0,0,0); return d.getTime(); }
function toLocalInput(ms){ const d = new Date(ms); const pad = n => String(n).padStart(2,'0'); return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`; }
function shouldUseHour(fromMs, toMs){ return toMs - fromMs <= 48*3600000; }
function pageRows(rows, page, size){ return rows.slice((page-1)*size, page*size); }
function unique(values){ return Array.from(new Set(values.map(v => String(v || '').trim()).filter(Boolean))).sort(); }
function uniqueObjects(items){ const seen = new Set(); return items.filter(item => item.value && !seen.has(item.value) && seen.add(item.value)); }
function fmtInt(v){ const n = Number(v || 0); return Number.isFinite(n) ? new Intl.NumberFormat('zh-CN').format(n) : '—'; }
function fmtPct(v){ if(v == null || Number.isNaN(Number(v))) return '—'; const n = Number(v); return `${(n <= 1 ? n*100 : n).toFixed(1)}%`; }
function fmtMoney(v){ if(v == null || Number.isNaN(Number(v))) return '—'; return '$' + Number(v).toFixed(4); }
function fmtMs(v){ if(v == null || Number.isNaN(Number(v))) return '—'; return `${Math.round(Number(v))} ms`; }
function formatDateTime(ms){ if(!ms) return '—'; return new Date(Number(ms)).toLocaleString('zh-CN', {hour12:false}); }
function shortHash(v){ const s = String(v || '').trim(); return s.length > 14 ? `${s.slice(0,7)}…${s.slice(-5)}` : (s || '—'); }
function pickObject(obj, keys){ return Object.fromEntries(keys.map(k => [k, obj?.[k]]).filter(([,v]) => v !== undefined)); }
function csvCell(v){ const s = v == null ? '' : String(v); return /[",\n]/.test(s) ? `"${s.replaceAll('"','""')}"` : s; }

defineExpose({ refresh });

const SimpleTable = defineComponent({
  props: { rows:{type:Array, default:()=>[]}, columns:{type:Array, default:()=>[]} },
  emits: ['select'],
  setup(props, {emit}){
    return () => {
      if(!props.rows.length) return h('div', {class:'empty'}, '暂无数据');
      const head = h('thead', h('tr', props.columns.map(col => h('th', col[1]))));
      const body = h('tbody', props.rows.slice(0, 250).map((row, idx) =>
        h('tr', {class:'clickable', key:idx, onClick:()=>emit('select', row)},
          props.columns.map(col => h('td', renderCell(row[col[0]], col[2])))
        )
      ));
      return h('div', {class:'table-wrap monitor-table'}, h('table', [head, body]));
    };
  }
});
const PaginationBar = defineComponent({
  props: { page:Number, pageSize:Number, total:Number },
  emits: ['page'],
  setup(props, {emit}){
    return () => {
      const pages = Math.max(1, Math.ceil((props.total || 0) / (props.pageSize || 50)));
      return h('div', {class:'pager'}, [
        h('span', `第 ${props.page} / ${pages} 页 · ${props.total || 0} 条`),
        h('button', {class:'btn', disabled:props.page <= 1, onClick:()=>emit('page', props.page - 1)}, '上一页'),
        h('button', {class:'btn', disabled:props.page >= pages, onClick:()=>emit('page', props.page + 1)}, '下一页'),
      ]);
    };
  }
});
function renderCell(v, type){
  if(type === 'pct') return fmtPct(v);
  if(type === 'money') return fmtMoney(v);
  if(type === 'ms') return fmtMs(v);
  if(type === 'time') return formatDateTime(v);
  if(type === 'int') return fmtInt(v);
  if(type === 'hash') return shortHash(v);
  if(Array.isArray(v)) return v.join(', ');
  if(v && typeof v === 'object') return JSON.stringify(v);
  return v == null || v === '' ? '—' : String(v);
}
</script>
