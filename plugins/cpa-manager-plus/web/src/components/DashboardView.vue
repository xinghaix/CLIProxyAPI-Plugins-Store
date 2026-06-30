<template>
  <section class="monitoring-page">
    <!-- Connection header -->
    <div class="card dashboard-header">
      <div class="dashboard-header-left">
        <div class="connection-status">
          <span :class="['status-dot', health.state !== 'err' && health.state === 'ok' ? 'connected' : 'disconnected']"></span>
          <span class="status-label">{{ health.state === 'ok' ? 'Manager 可达' : health.text || '未检测' }}</span>
        </div>
      </div>
      <div class="dashboard-header-right">
        <span class="dashboard-time">{{ currentTime }}</span>
        <button class="btn" @click="refresh(true)" :disabled="loading">{{ loading ? '刷新中…' : '刷新' }}</button>
      </div>
    </div>

    <!-- Version & collector status -->
    <div class="card dashboard-version-card">
      <div class="version-grid">
        <div><span>插件版本</span><strong>{{ pluginVersion }}</strong></div>
        <div><span>Manager Base URL</span><strong>{{ managerBase || '—' }}</strong></div>
        <div><span>CPA Base</span><strong>{{ cpaBase || '—' }}</strong></div>
        <div><span>Collector</span><strong :class="collectorStatus ? 'good-text' : 'muted'">{{ collectorStatusText }}</strong></div>
      </div>
    </div>

    <!-- Today's usage metrics -->
    <MetricGrid :cards="usageCards" />

    <!-- Traffic overview -->
    <DataCard title="流量概览" subtitle="30 分钟趋势">
      <div class="timeline-bars" v-if="trafficTimeline.length">
        <div v-for="point in trafficTimeline" :key="point.bucket_ms || point.label" class="timeline-row">
          <span class="timeline-label">{{ formatTimelineLabel(point) }}</span>
          <div class="timeline-track"><i :style="{width: trafficBarWidth(point)}"></i></div>
          <span class="timeline-value">{{ fmtInt(point.calls || point.requests || 0) }}</span>
          <span class="timeline-sub">{{ fmtCompact(point.tokens || 0) }} tok</span>
        </div>
      </div>
      <div v-else class="empty">暂无流量数据</div>
    </DataCard>

    <div class="split">
      <!-- Model cost rank -->
      <DataCard title="模型费用排名" subtitle="今日 Top 5">
        <div v-if="modelCostRank.length" class="rank-list">
          <div v-for="(model, idx) in modelCostRank" :key="model.model" class="rank-item">
            <div class="rank-index">{{ idx + 1 }}</div>
            <div class="rank-info">
              <div class="rank-model-name">{{ model.model }}</div>
              <div class="rank-track">
                <div class="rank-bar" :style="{width: `${(model.cost_share || 0) * 100}%`}"></div>
              </div>
            </div>
            <div class="rank-value">
              <div class="rank-cost">{{ fmtMoney(model.cost) }}</div>
              <div class="rank-share">{{ ((model.cost_share || 0) * 100).toFixed(1) }}%</div>
            </div>
          </div>
        </div>
        <div v-else-if="topModels.length" class="rank-list">
          <div v-for="(model, idx) in topModels" :key="model.model" class="rank-item">
            <div class="rank-index">{{ idx + 1 }}</div>
            <div class="rank-info">
              <div class="rank-model-name">{{ model.model }}</div>
              <div class="rank-track">
                <div class="rank-bar" :style="{width: `${model.tokens / Math.max(...topModels.map(m => m.tokens), 1) * 100}%`}"></div>
              </div>
            </div>
            <div class="rank-value">
              <div class="rank-cost">{{ fmtMoney(model.cost) }}</div>
              <div class="rank-share">{{ fmtCompact(model.tokens) }} tok</div>
            </div>
          </div>
        </div>
        <div v-else class="empty">{{ loading ? '加载中…' : '暂无排名数据' }}</div>
      </DataCard>

      <!-- Health alerts -->
      <DataCard title="健康告警" subtitle="近期失败">
        <div v-if="recentFailures.length" class="failure-list">
          <div v-for="fail in recentFailures" :key="fail.event_hash || fail.timestamp_ms" class="failure-item">
            <span :class="['failure-severity', fail.severity || 'bad']">{{ fail.fail_status_code || 'ERR' }}</span>
            <div>
              <div class="failure-model">{{ fail.model || '—' }}</div>
              <div class="failure-summary muted small-text">{{ maskSummary(fail.fail_summary) }}</div>
            </div>
            <span class="failure-time small-text">{{ formatTime(fail.timestamp_ms) }}</span>
          </div>
        </div>
        <div v-else-if="channelHealth.length" class="channel-health-list">
          <div v-for="ch in channelHealth" :key="ch.channel || ch.provider" class="channel-health-item">
            <span class="channel-name">{{ ch.channel || ch.provider || '—' }}</span>
            <span :class="['channel-status', ch.health === 'good' ? 'good-text' : ch.health === 'warn' ? 'warn-text' : 'bad-text']">
              {{ ch.success_rate != null ? fmtPct(ch.success_rate) : '—' }}
            </span>
            <span class="muted small-text">{{ ch.calls || 0 }} 调用</span>
          </div>
        </div>
        <div v-else class="empty">{{ loading ? '加载中…' : '暂无告警' }}</div>
      </DataCard>
    </div>

    <!-- Token mix -->
    <DataCard v-if="tokenMix.length" title="Token 构成" subtitle="输入 / 输出 / 缓存">
      <div class="token-mix-bar">
        <div v-for="seg in tokenMix" :key="seg.label"
          class="token-mix-seg"
          :style="{width: `${(seg.share || 0) * 100}%`, '--mix-color':(seg.color) }">
          <span>{{ seg.label }} {{ ((seg.share || 0) * 100).toFixed(0) }}%</span>
        </div>
      </div>
      <div class="token-mix-legend">
        <div v-for="seg in tokenMix" :key="seg.label" class="token-mix-legend-item">
          <span class="mix-dot" :style="{background: seg.color}"></span>
          <span>{{ seg.label }}: {{ fmtCompact(seg.tokens || 0) }} ({{ ((seg.share || 0) * 100).toFixed(1) }}%)</span>
        </div>
      </div>
    </DataCard>

    <!-- Quick stats -->
    <MetricGrid :cards="quickStats" />

    <!-- Config summary -->
    <DataCard v-if="configSummary.length" title="当前配置摘要" subtitle="config.yaml">
      <div class="config-summary-grid">
        <div v-for="item in configSummary" :key="item.label" class="config-summary-item">
          <span class="config-summary-label">{{ item.label }}</span>
          <span :class="['config-summary-value', item.on ? 'good-text' : item.off ? 'muted' : '']">{{ item.value }}</span>
        </div>
      </div>
    </DataCard>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
  pluginVersion: { type: String, default: '' },
});

const data = ref(null);
const loading = ref(false);
const currentTime = ref('');
const pluginVersion = props.pluginVersion || '';
let timer = null;

const summary = computed(() => data.value?.today || {});
const rolling = computed(() => data.value?.rolling_30m || {});
const topModels = computed(() => data.value?.top_models_today || []);
const modelCostRank = computed(() => data.value?.model_cost_rank || []);
const trafficTimeline = computed(() => data.value?.traffic_timeline || []);
const recentFailures = computed(() => data.value?.recent_failures || []);
const channelHealth = computed(() => data.value?.channel_health || []);
const tokenMix = computed(() => data.value?.token_mix || []);
const managerBase = computed(() => data.value?.window?.manager_base_url || '');
const cpaBase = computed(() => data.value?.window?.cpa_base_url || '');
const collectorStatus = computed(() => data.value?.collector || null);
const collectorStatusText = computed(() => {
  if(!collectorStatus.value) return '未检测';
  const c = collectorStatus.value;
  if(c.dead_letters > 0) return `${c.dead_letters} 死信`;
  if(c.events != null) return `${fmtInt(c.events)} 事件`;
  return c.collector || '活跃';
});

const usageCards = computed(() => {
  const s = summary.value;
  const r = rolling.value;
  return [
    {label:'今日请求', value: fmtInt(s.total_calls), sub:`失败 ${fmtInt(s.failure_calls)}`},
    {label:'RPM (30min)', value: fmtCompact(r.rpm), sub:`${fmtInt(r.total_calls)} 调用`},
    {label:'TPM (30min)', value: fmtCompact(r.tpm), sub:`${fmtCompact(r.total_tokens)} tok`},
    {label:'今日花费', value: fmtMoney(s.total_cost), sub:`${fmtCompact(s.total_tokens)} tok`},
    {label:'成功率', value: fmtPct(s.success_rate), sub:`${fmtInt(s.success_calls)} / ${fmtInt(s.total_calls)}`},
    {label:'平均延迟', value: fmtDuration(s.average_latency_ms), sub:`零Token ${fmtInt(s.zero_token_calls)}`},
  ];
});

const quickStats = computed(() => {
  const s = summary.value;
  return [
    {label:'管理密钥', value: s.api_keys ?? '—', sub:'CPA 配置'},
    {label:'OAuth 凭据', value: s.auth_files ?? '—', sub:'auth files'},
    {label:'可用模型', value: s.available_models ?? '—', sub:'models'},
  ];
});

const configSummary = computed(() => {
  const c = data.value?.config_summary;
  if(!c) return [];
  return [
    {label:'Debug', value: c.debug ? '启用' : '关闭', on: c.debug, off: !c.debug},
    {label:'日志文件', value: c.logging_to_file ? '启用' : '关闭', on: c.logging_to_file, off: !c.logging_to_file},
    {label:'重试次数', value: String(c.request_retry ?? 0)},
    {label:'WS 认证', value: c.ws_auth ? '启用' : '关闭', on: c.ws_auth, off: !c.ws_auth},
    {label:'路由策略', value: c.routing_strategy || '—'},
    ...(c.proxy_url ? [{label:'Proxy URL', value: c.proxy_url}] : []),
  ];
});

onMounted(() => {
  updateClock();
  timer = setInterval(updateClock, 60000);
  if(props.ready) refresh(true);
});
onBeforeUnmount(() => { if(timer) clearInterval(timer); });

function updateClock(){ currentTime.value = new Date().toLocaleString('zh-CN', {hour12:false}); }

async function refresh(force=false){
  if(!props.ready) return;
  if(loading.value && !force) return;
  loading.value = true;
  try{
    const now = Date.now();
    const d = new Date(); d.setHours(0,0,0,0);
    const todayStart = d.getTime();
    data.value = await props.proxyCall({
      method:'GET',
      path:'/v0/management/dashboard/summary',
      query:`today_start_ms=${todayStart}&now_ms=${now}&top_models=5&recent_failures=5`,
    });
  }catch(e){
    // silent — dashboard tolerates errors
  }finally{
    loading.value = false;
  }
}
defineExpose({ refresh });

const maxTrafficCalls = computed(() => Math.max(1, ...trafficTimeline.value.map(p => Number(p.calls || p.requests || 0))));
function trafficBarWidth(point){ return `${Math.max(2, Math.round((Number(point.calls || point.requests || 0) / maxTrafficCalls.value) * 100))}%`; }
function formatTimelineLabel(point){ if(point.label) return point.label; const d = new Date(point.bucket_ms); return d.toLocaleTimeString('zh-CN', {hour:'2-digit', minute:'2-digit', hour12:false}); }
function formatTime(ms){ if(!ms) return '—'; return new Date(Number(ms)).toLocaleTimeString('zh-CN', {hour12:false}); }
function maskSummary(s){ if(!s) return '—'; return s.length > 80 ? s.slice(0, 80) + '…' : s; }

function fmtInt(v){ const n = Number(v || 0); return Number.isFinite(n) ? new Intl.NumberFormat('zh-CN').format(n) : '—'; }
function fmtPct(v){ if(v == null || Number.isNaN(Number(v))) return '—'; const n = Number(v); return `${(n <= 1 ? n*100 : n).toFixed(1)}%`; }
function fmtMoney(v){ if(v == null || Number.isNaN(Number(v))) return '—'; return '$' + Number(v).toFixed(4); }
function fmtDuration(v){ const n = Number(v); if(v == null || !Number.isFinite(n)) return '—'; if(n < 1000) return `${Math.round(n)} ms`; const sec = n / 1000; if(sec < 60) return `${sec.toFixed(sec < 10 ? 1 : 0)} s`; const min = Math.floor(sec / 60); const rem = Math.round(sec % 60); return `${min}m ${rem}s`; }
function fmtCompact(v){ const n = Number(v || 0); if(!Number.isFinite(n)) return '—'; if(Math.abs(n) >= 1e9) return `${(n/1e9).toFixed(2)}B`; if(Math.abs(n) >= 1e6) return `${(n/1e6).toFixed(1)}M`; if(Math.abs(n) >= 1e3) return `${(n/1e3).toFixed(1)}K`; return String(Math.round(n)); }
</script>