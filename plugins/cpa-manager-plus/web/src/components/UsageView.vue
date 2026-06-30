<template>
  <section class="monitoring-page">
    <!-- Filter bar -->
    <div class="card filter-card usage-filterbar">
      <div class="filterbar-row">
        <select v-model="filters.timeRange" class="control compact" @change="refresh(true)">
          <option value="24h">24 小时</option>
          <option value="today">今天</option>
          <option value="yesterday">昨天</option>
          <option value="7d">最近 7 天</option>
          <option value="30d">最近 30 天</option>
          <option value="custom">自定义</option>
        </select>
        <select v-model="filters.granularity" class="control compact" @change="refresh(true)">
          <option value="auto">自动粒度</option>
          <option value="hour">小时</option>
          <option value="day">天</option>
        </select>
        <input v-model.trim="filters.searchQuery" class="control wide" placeholder="搜索模型 / 账号 / API Key / 路径" @keyup.enter="refresh(true)" />
        <select v-model="filters.status" class="control compact" @change="refresh(true)">
          <option value="all">全部状态</option>
          <option value="success">仅成功</option>
          <option value="failed">仅失败</option>
        </select>
        <select v-model="filters.provider" class="control compact" @change="refresh(true)">
          <option value="all">全部 Provider</option>
          <option v-for="p in providerOptions" :key="p" :value="p">{{ p }}</option>
        </select>
      </div>
      <div class="filterbar-row">
        <select v-model="filters.model" class="control compact" @change="refresh(true)">
          <option value="all">全部模型</option>
          <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
        </select>
        <select v-model="filters.authFile" class="control compact" @change="refresh(true)">
          <option value="all">全部凭据</option>
          <option v-for="f in authFileOptions" :key="f" :value="f">{{ f }}</option>
        </select>
        <select v-model="filters.minLatencyMs" class="control compact" @change="refresh(true)">
          <option value="all">全部延迟</option>
          <option value="3000">> 3s</option>
          <option value="10000">> 10s</option>
          <option value="30000">> 30s</option>
        </select>
        <select v-model="filters.cacheStatus" class="control compact" @change="refresh(true)">
          <option value="all">全部缓存</option>
          <option value="hit">缓存命中</option>
          <option value="miss">缓存未命中</option>
        </select>
      </div>
      <div v-if="filters.timeRange === 'custom'" class="filterbar-row">
        <label>开始 <input v-model="customStartInput" type="datetime-local" class="control" /></label>
        <label>结束 <input v-model="customEndInput" type="datetime-local" class="control" /></label>
        <button class="btn primary" @click="applyCustomRange">应用</button>
      </div>
    </div>

    <section v-if="error" class="notice error">{{ error }}</section>

    <!-- KPI cards (always visible) -->
    <MetricGrid :cards="summaryCards" />

    <!-- Tab bar -->
    <div class="monitor-tabs card">
      <button v-for="tab in tabs" :key="tab.key" :class="['tab', {active: activeTab === tab.key}]" @click="activeTab = tab.key">{{ tab.label }}</button>
    </div>

    <!-- Overview tab -->
    <div v-if="activeTab === 'overview'" class="usage-tab-content">
      <DataCard title="时间线" :subtitle="granularityLabel">
        <div class="timeline-bars" v-if="timelineRows.length">
          <div v-for="point in timelineRows" :key="point.bucket_ms" class="timeline-row"
            :class="{selected: selectedBucketMs === point.bucket_ms}"
            @click="selectBucket(point)">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: barWidth(point.calls)}"></i></div>
            <span class="timeline-value">{{ fmtInt(point.calls) }}</span>
            <span class="timeline-sub">{{ fmtCompact(point.tokens) }} tok · {{ fmtMoney(point.cost) }}</span>
          </div>
        </div>
        <div v-else class="empty">暂无时间线数据</div>
      </DataCard>

      <div class="split">
        <DataCard title="模型排名" subtitle="Top 8">
          <SimpleTable :rows="topModels" :columns="rankColumns" />
        </DataCard>
        <DataCard title="API Key 排名" subtitle="Top 8">
          <SimpleTable :rows="topApiKeys" :columns="apiKeyRankColumns" />
        </DataCard>
      </div>

      <DataCard v-if="anomalyRows.length" title="异常点" :subtitle="`${anomalyRows.length} 个`">
        <SimpleTable :rows="anomalyRows" :columns="anomalyColumns" />
      </DataCard>
    </div>

    <!-- Trends tab -->
    <div v-if="activeTab === 'trends'" class="usage-tab-content">
      <DataCard title="趋势" :subtitle="trendMetricLabel">
        <div class="trend-controls">
          <button v-for="m in trendMetrics" :key="m.key" :class="['tab', {active: trendMetric === m.key}]" @click="trendMetric = m.key">{{ m.label }}</button>
        </div>
        <div class="timeline-bars" v-if="timelineRows.length">
          <div v-for="point in timelineRows" :key="point.bucket_ms" class="timeline-row">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: trendBarWidth(point)}"></i></div>
            <span class="timeline-value">{{ formatTrendValue(point) }}</span>
          </div>
        </div>
        <div v-else class="empty">暂无趋势数据</div>
      </DataCard>
    </div>

    <!-- Models tab -->
    <div v-if="activeTab === 'models'" class="usage-tab-content">
      <DataCard title="模型维度" subtitle="model_stats / model_share">
        <SimpleTable :rows="modelRows" :columns="modelColumns" />
      </DataCard>
    </div>

    <!-- API Keys tab -->
    <div v-if="activeTab === 'apiKeys'" class="usage-tab-content">
      <DataCard title="API Key 维度" subtitle="api_key_stats">
        <SimpleTable :rows="apiKeyRows" :columns="apiKeyColumns" />
      </DataCard>
    </div>

    <!-- Credentials tab -->
    <div v-if="activeTab === 'credentials'" class="usage-tab-content">
      <DataCard title="凭据维度" subtitle="credential_stats / credential_timeline">
        <SimpleTable :rows="credentialRows" :columns="credentialColumns" />
      </DataCard>
    </div>

    <!-- Heatmap tab -->
    <div v-if="activeTab === 'heatmap'" class="usage-tab-content">
      <DataCard title="热力图" subtitle="7×24 请求分布">
        <div class="heatmap-controls">
          <select v-model="heatmapMetric" class="control compact">
            <option value="requestCount">请求数</option>
            <option value="totalTokens">Token 数</option>
            <option value="estimatedCost">费用</option>
            <option value="failureRate">失败率</option>
          </select>
          <select v-model="heatmapScaleMode" class="control compact">
            <option value="absolute">绝对值</option>
            <option value="byWeekday">按星期归一</option>
            <option value="byHour">按小时归一</option>
          </select>
        </div>
        <div v-if="heatmapRows.length" class="heatmap-grid-wrap">
          <table class="heatmap-table">
            <thead>
              <tr>
                <th></th>
                <th v-for="h in 24" :key="h">{{ h - 1 }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, wi) in heatmapRows" :key="wi">
                <td class="heatmap-day-label">{{ weekdayLabel(wi) }}</td>
                <td v-for="(cell, hi) in row" :key="hi"
                  class="heatmap-cell"
                  :style="heatmapCellStyle(cell)"
                  :title="heatmapCellTitle(wi, hi, cell)"
                  @click="selectHeatmapCell(wi, hi, cell)"
                ></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty">暂无热力图数据</div>
      </DataCard>
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

const DAY_MS = 86400000;
const HOUR_MS = 3600000;

const data = ref(null);
const loading = ref(false);
const error = ref('');
const activeTab = ref('overview');
const selectedBucketMs = ref(null);
const trendMetric = ref('requestCount');
const heatmapMetric = ref('requestCount');
const heatmapScaleMode = ref('absolute');
const customStartInput = ref('');
const customEndInput = ref('');
const filters = ref(defaultFilters());

const tabs = [
  {key:'overview', label:'概览'},
  {key:'trends', label:'趋势'},
  {key:'models', label:'模型'},
  {key:'apiKeys', label:'API Key'},
  {key:'credentials', label:'凭据'},
  {key:'heatmap', label:'热力图'},
];
const trendMetrics = [
  {key:'requestCount', label:'请求数'},
  {key:'totalTokens', label:'Token 数'},
  {key:'estimatedCost', label:'费用'},
];

const summary = computed(() => data.value?.summary || {});
const timelineRows = computed(() => data.value?.timeline || []);
const modelRows = computed(() => data.value?.model_stats || data.value?.model_share || []);
const apiKeyRows = computed(() => data.value?.api_key_stats || []);
const credentialRows = computed(() => data.value?.credential_stats || []);
const heatmapRaw = computed(() => data.value?.heatmap || []);
const anomalyRows = computed(() => data.value?.anomaly_points || []);
const filterOptions = computed(() => data.value?.filter_options || {});

const modelOptions = computed(() => unique([
  ...(filterOptions.value.model_stats || []).map(x => x.model),
  ...modelRows.value.map(x => x.model),
]));
const providerOptions = computed(() => unique([
  ...(filterOptions.value.providers || []),
  ...modelRows.value.map(x => x.provider || x.auth_provider_snapshot),
  ...apiKeyRows.value.map(x => x.provider || x.auth_provider_snapshot),
]));
const authFileOptions = computed(() => unique([
  ...(filterOptions.value.auth_files || []),
  ...credentialRows.value.map(x => x.auth_file || x.authFile || x.source),
]));

const granularityLabel = computed(() => data.value?.granularity || 'auto');
const trendMetricLabel = computed(() => trendMetrics.find(m => m.key === trendMetric.value)?.label || '');

// KPI cards
const summaryCards = computed(() => {
  const s = summary.value;
  const cacheTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const totalTokens = Math.max(Number(s.total_tokens ?? 0), 0);
  const reasoningTokens = Number(s.reasoning_tokens ?? 0);
  const cacheHitRate = computeCacheHitRate(s);
  return [
    {label:'请求数', value: fmtCompact(s.total_calls), sub:`${fmtInt(s.success_calls)} 成功 / ${fmtInt(s.failure_calls)} 失败`},
    {label:'成功率', value: fmtPct(s.success_rate), sub: fmtDuration(s.average_latency_ms)},
    {label:'失败数', value: fmtInt(s.failure_calls), sub: `${anomalyRows.value.length} 异常点`},
    {label:'预估花费', value: fmtMoney(s.total_cost), sub:`单次 ${fmtMoney(s.average_cost_per_call)}`},
    {label:'总 Tokens', value: fmtCompact(s.total_tokens), sub:`推理 ${fmtCompact(reasoningTokens)}`},
    {label:'输入 Tokens', value: fmtCompact(s.input_tokens), sub:`占比 ${fmtPct(totalTokens > 0 ? Number(s.input_tokens ?? 0) / totalTokens : 0)}`},
    {label:'输出 Tokens', value: fmtCompact(s.output_tokens), sub:`占比 ${fmtPct(totalTokens > 0 ? Number(s.output_tokens ?? 0) / totalTokens : 0)}`},
    {label:'缓存 Tokens', value: fmtCompact(cacheTokens), sub:`命中率 ${fmtPct(cacheHitRate)}`},
  ];
});

// Top N rows
const topModels = computed(() => [...modelRows.value].sort((a,b) => Number(b.calls ?? 0) - Number(a.calls ?? 0)).slice(0, 8));
const topApiKeys = computed(() => [...apiKeyRows.value].sort((a,b) => Number(b.calls ?? 0) - Number(a.calls ?? 0)).slice(0, 8));

// Timeline max
const maxTimelineCalls = computed(() => Math.max(1, ...timelineRows.value.map(p => Number(p.calls || 0))));
const maxTrendValue = computed(() => {
  let max = 1;
  for(const p of timelineRows.value){
    const v = trendValue(p);
    if(v > max) max = v;
  }
  return max;
});

// Heatmap: transform raw points into 7x24 grid
const heatmapRows = computed(() => {
  const grid = Array.from({length:7}, () => Array.from({length:24}, () => null));
  for(const point of heatmapRaw.value){
    const wd = Number(point.weekday ?? 0);
    const hr = Number(point.hour ?? 0);
    if(wd >= 0 && wd < 7 && hr >= 0 && hr < 24){
      grid[wd][hr] = point;
    }
  }
  return grid;
});

// Columns
const rankColumns = [
  ['model','模型'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'],
];
const apiKeyRankColumns = [
  ['api_key_hash','API Key','hash'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'],
];
const modelColumns = [
  ['model','模型'], ['provider','Provider'], ['calls','请求','int'], ['success_calls','成功','int'], ['failure_calls','失败','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'],
];
const apiKeyColumns = [
  ['api_key_hash','API Key','hash'], ['account_snapshot','账号'], ['provider','Provider'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'], ['last_seen_ms','最后','time'],
];
const credentialColumns = [
  ['auth_file','凭据文件'], ['provider','Provider'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'], ['last_seen_ms','最后','time'],
];
const anomalyColumns = [
  ['label','时间'], ['severity','级别'], ['calls','请求','int'], ['failure_rate','失败率','pct'], ['cost','费用','money'], ['request_change','请求变化','pct'],
];

watch(filters, () => {}, {deep:true});

onMounted(() => { if(props.ready) refresh(true); });
onBeforeUnmount(() => {});
watch(() => props.ready, (ready) => { if(ready && !data.value) refresh(true); });

async function refresh(force=false){
  if(!props.ready) return;
  if(loading.value && !force) return;
  loading.value = true;
  error.value = '';
  try{
    const req = buildRequest();
    data.value = await props.proxyCall({method:'POST', path:'/v0/management/monitoring/analytics', body:req});
    selectedBucketMs.value = null;
  }catch(e){
    error.value = e.message || String(e);
  }finally{
    loading.value = false;
  }
}
function buildRequest(){
  const now = Date.now();
  const bounds = getRangeBounds(now);
  const f = {};
  if(filters.value.model !== 'all') f.models = [filters.value.model];
  if(filters.value.provider !== 'all') f.providers = [filters.value.provider.toLowerCase()];
  if(filters.value.authFile !== 'all') f.auth_files = [filters.value.authFile];
  if(filters.value.status === 'success') f.include_failed = false;
  if(filters.value.status === 'failed') f.failed_only = true;
  if(filters.value.minLatencyMs !== 'all') f.min_latency_ms = Number(filters.value.minLatencyMs);
  if(filters.value.cacheStatus !== 'all') f.cache_status = filters.value.cacheStatus;
  const granularity = resolveGranularity();
  const include = {
    summary: true,
    summary_comparison: true,
    timeline: true,
    model_stats: true,
    channel_share: true,
    api_key_stats: true,
    credential_stats: true,
    credential_timeline: true,
    filter_options: true,
    heatmap: true,
    anomaly_points: true,
    granularity,
  };
  const request = {
    from_ms: bounds.fromMs,
    to_ms: bounds.toMs,
    now_ms: now,
    time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || '',
    include,
  };
  if(filters.value.searchQuery) request.search_query = filters.value.searchQuery;
  if(Object.keys(f).length) request.filters = f;
  return request;
}
function getRangeBounds(now){
  const tr = filters.value.timeRange;
  if(tr === 'custom'){
    const start = Date.parse(customStartInput.value);
    const end = Date.parse(customEndInput.value);
    if(start && end && start < end) return {fromMs: start, toMs: end};
    return {fromMs: now - DAY_MS, toMs: now};
  }
  if(tr === '24h') return {fromMs: now - DAY_MS, toMs: now};
  if(tr === 'today'){ const d = new Date(); d.setHours(0,0,0,0); return {fromMs: d.getTime(), toMs: now}; }
  if(tr === 'yesterday'){ const d = new Date(); d.setHours(0,0,0,0); return {fromMs: d.getTime() - DAY_MS, toMs: d.getTime()}; }
  if(tr === '7d') return {fromMs: now - 7*DAY_MS, toMs: now};
  if(tr === '30d') return {fromMs: now - 30*DAY_MS, toMs: now};
  return {fromMs: now - 7*DAY_MS, toMs: now};
}
function resolveGranularity(){
  const g = filters.value.granularity;
  if(g === 'hour' || g === 'day') return g;
  const tr = filters.value.timeRange;
  if(tr === '30d') return 'day';
  return 'hour';
}
function applyCustomRange(){ refresh(true); }
function selectBucket(point){ selectedBucketMs.value = selectedBucketMs.value === point?.bucket_ms ? null : point?.bucket_ms ?? null; }
function selectHeatmapCell(wi, hi, cell){ if(!cell) return; }

function trendValue(point){
  if(trendMetric.value === 'requestCount') return Number(point.calls || 0);
  if(trendMetric.value === 'totalTokens') return Number(point.tokens || point.total_tokens || 0);
  if(trendMetric.value === 'estimatedCost') return Number(point.cost || 0);
  return 0;
}
function formatTrendValue(point){
  const v = trendValue(point);
  if(trendMetric.value === 'estimatedCost') return fmtMoney(v);
  return fmtCompact(v);
}
function trendBarWidth(point){ return `${Math.max(2, Math.round((trendValue(point) / maxTrendValue.value) * 100))}%`; }
function barWidth(value){ return `${Math.max(2, Math.round((Number(value || 0) / maxTimelineCalls.value) * 100))}%`; }

function heatmapCellValue(cell){
  if(!cell) return 0;
  if(heatmapMetric.value === 'requestCount') return Number(cell.calls || 0);
  if(heatmapMetric.value === 'totalTokens') return Number(cell.tokens || 0);
  if(heatmapMetric.value === 'estimatedCost') return Number(cell.cost || 0);
  if(heatmapMetric.value === 'failureRate') return Number(cell.failure_rate || 0);
  return 0;
}
function heatmapMaxValue(){
  let max = 0;
  for(const row of heatmapRows.value){
    for(const cell of row){
      const v = heatmapCellValue(cell);
      if(v > max) max = v;
    }
  }
  return Math.max(max, 0.001);
}
function heatmapCellStyle(cell){
  if(!cell) return {background: 'transparent'};
  const v = heatmapCellValue(cell);
  const max = heatmapMaxValue();
  let ratio = v / max;
  if(heatmapScaleMode.value === 'byWeekday' || heatmapScaleMode.value === 'byHour') ratio = Math.min(ratio, 1);
  const alpha = Math.max(0.08, ratio);
  return {background: `color-mix(in srgb, var(--cpa-primary) ${Math.round(alpha*100)}%, transparent)`};
}
function heatmapCellTitle(wi, hi, cell){
  if(!cell) return '';
  return `${weekdayLabel(wi)} ${hi}:00 — ${fmtInt(cell.calls)} 请求, ${fmtCompact(cell.tokens)} tok, ${fmtMoney(cell.cost)}, 失败率 ${fmtPct(cell.failure_rate)}`;
}
function weekdayLabel(idx){ return ['周日','周一','周二','周三','周四','周五','周六'][idx] || ''; }

function computeCacheHitRate(s){
  const inputTokens = Number(s?.input_tokens ?? 0);
  const cacheReadTokens = Number(s?.cache_read_tokens ?? 0);
  const cacheCreationTokens = Number(s?.cache_creation_tokens ?? 0);
  const cachedTokens = Number(s?.cached_tokens ?? 0);
  const totalInput = Math.max(inputTokens, cachedTokens) + cacheReadTokens + cacheCreationTokens;
  const hitTokens = cachedTokens + cacheReadTokens;
  return totalInput > 0 ? hitTokens / totalInput : 0;
}
function defaultFilters(){ return {timeRange:'24h', granularity:'auto', model:'all', apiKeyHash:'all', provider:'all', authFile:'all', status:'all', searchQuery:'', minLatencyMs:'all', cacheStatus:'all'}; }
function unique(values){ return Array.from(new Set(values.map(v => String(v || '').trim()).filter(Boolean))).sort(); }

function fmtInt(v){ const n = Number(v || 0); return Number.isFinite(n) ? new Intl.NumberFormat('zh-CN').format(n) : '—'; }
function fmtPct(v){ if(v == null || Number.isNaN(Number(v))) return '—'; const n = Number(v); return `${(n <= 1 ? n*100 : n).toFixed(1)}%`; }
function fmtMoney(v){ if(v == null || Number.isNaN(Number(v))) return '—'; return '$' + Number(v).toFixed(4); }
function fmtDuration(v){ const n = Number(v); if(v == null || !Number.isFinite(n)) return '—'; if(n < 1000) return `${Math.round(n)} ms`; const sec = n / 1000; if(sec < 60) return `${sec.toFixed(sec < 10 ? 1 : 0)} s`; const min = Math.floor(sec / 60); const rem = Math.round(sec % 60); return `${min}m ${rem}s`; }
function fmtCompact(v){
  const n = Number(v || 0);
  if(!Number.isFinite(n)) return '—';
  if(Math.abs(n) >= 1e9) return `${(n/1e9).toFixed(2)}B`;
  if(Math.abs(n) >= 1e6) return `${(n/1e6).toFixed(1)}M`;
  if(Math.abs(n) >= 1e3) return `${(n/1e3).toFixed(1)}K`;
  return String(Math.round(n));
}

defineExpose({ refresh });

const SimpleTable = defineComponent({
  props: { rows:{type:Array, default:()=>[]}, columns:{type:Array, default:()=>[]} },
  setup(props){
    return () => {
      if(!props.rows.length) return h('div', {class:'empty'}, '暂无数据');
      const head = h('thead', h('tr', props.columns.map(col => h('th', col[1]))));
      const body = h('tbody', props.rows.slice(0, 50).map((row, idx) =>
        h('tr', {key:idx},
          props.columns.map(col => h('td', renderCell(row[col[0]], col[2])))
        )
      ));
      return h('div', {class:'table-wrap monitor-table'}, h('table', [head, body]));
    };
  }
});
function renderCell(v, type){
  if(type === 'pct') return fmtPct(v);
  if(type === 'money') return fmtMoney(v);
  if(type === 'ms') return fmtDuration(v);
  if(type === 'time') return v ? new Date(Number(v)).toLocaleString('zh-CN', {hour12:false}) : '—';
  if(type === 'int') return fmtInt(v);
  if(type === 'hash') return shortHash(v);
  if(Array.isArray(v)) return v.join(', ');
  if(v && typeof v === 'object') return JSON.stringify(v);
  return v == null || v === '' ? '—' : String(v);
}
function shortHash(v){ const s = String(v || '').trim(); return s.length > 14 ? `${s.slice(0,7)}…${s.slice(-5)}` : (s || '—'); }
</script>
