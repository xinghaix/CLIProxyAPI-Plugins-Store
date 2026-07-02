<template>
  <section class="monitoring-page">
    <!-- ====== 今日概览（Plus 仪表盘） ====== -->
    <section class="dashboard-zone">
      <div class="dashboard-zone-head">
        <h2 class="dashboard-zone-title">今日概览</h2>
        <span class="muted small-text">dashboard/summary · 今日 0 点至今</span>
      </div>
    <MetricGrid :cards="dashboardKpi" />

    <!-- ====== Traffic overview (dashboard) ====== -->
    <DataCard title="近 30 分钟流量" subtitle="滚动窗口">
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

    <!-- ====== Model cost rank + Health alerts ====== -->
    <div class="split">
      <DataCard title="模型费用排名" subtitle="今日 Top 5">
        <div v-if="modelCostRank.length" class="rank-list">
          <div v-for="(model, idx) in modelCostRank" :key="model.model" class="rank-item">
            <div class="rank-index">{{ idx + 1 }}</div>
            <div class="rank-info">
              <div class="rank-model-name">{{ model.model }}</div>
              <div class="rank-track"><div class="rank-bar" :style="{width: `${(model.cost_share || 0) * 100}%`}"></div></div>
            </div>
            <div class="rank-value">
              <div class="rank-cost">{{ fmtMoney(model.cost) }}</div>
              <div class="rank-share">{{ ((model.cost_share || 0) * 100).toFixed(1) }}%</div>
            </div>
          </div>
        </div>
        <div v-else class="empty">{{ dashLoading ? '加载中…' : '暂无排名数据' }}</div>
      </DataCard>
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
            <span :class="['channel-status', ch.health === 'good' ? 'good-text' : ch.health === 'warn' ? 'warn-text' : 'bad-text']">{{ ch.success_rate != null ? fmtPct(ch.success_rate) : '—' }}</span>
            <span class="muted small-text">{{ ch.calls || 0 }} 调用</span>
          </div>
        </div>
        <div v-else class="empty">{{ dashLoading ? '加载中…' : '暂无告警' }}</div>
      </DataCard>
    </div>

    <!-- ====== Token mix ====== -->
    <DataCard v-if="tokenMix.length" title="Token 构成" subtitle="输入 / 输出 / 缓存">
      <div class="token-mix-bar">
        <div v-for="seg in tokenMix" :key="seg.label" class="token-mix-seg" :style="{width: `${(seg.share || 0) * 100}%`, '--mix-color':(seg.color) }">
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

    <!-- ====== Quick stats ====== -->
    <div class="dashboard-bento-grid">
      <button v-for="item in quickStats" :key="item.key" class="dashboard-bento-card" @click="openTab(item.tab)">
        <div class="dashboard-bento-top">
          <span class="dashboard-bento-label">{{ item.label }}</span>
          <span class="dashboard-bento-arrow">→</span>
        </div>
        <div class="dashboard-bento-value">{{ item.value }}</div>
        <div class="dashboard-bento-sub muted small-text">{{ item.sub }}</div>
      </button>
    </div>

    <!-- ====== Config summary ====== -->
    <DataCard v-if="configSummary.length" title="CPA 配置摘要" subtitle="来自 Manager 快照">
      <div class="config-summary-grid">
        <div v-for="item in configSummary" :key="item.label" class="config-summary-item">
          <span class="config-summary-label">{{ item.label }}</span>
          <span :class="['config-summary-value', item.on ? 'good-text' : item.off ? 'muted' : '']">{{ item.value }}</span>
        </div>
      </div>
    </DataCard>
    </section>

    <!-- ====== 用量分析（Plus 用量分析，可筛选时间范围） ====== -->
    <section class="dashboard-zone dashboard-zone-analytics">
      <div class="dashboard-zone-head">
        <h2 class="dashboard-zone-title">用量分析</h2>
        <span class="muted small-text">monitoring/analytics</span>
      </div>

    <!-- Analytics filter bar -->
    <div class="card filter-card usage-filterbar">
      <div class="filterbar-row">
        <select v-model="filters.timeRange" class="control compact" @change="refreshAnalytics(true)">
          <option value="24h">24 小时</option><option value="today">今天</option><option value="yesterday">昨天</option>
          <option value="7d">最近 7 天</option><option value="30d">最近 30 天</option><option value="custom">自定义</option>
        </select>
        <select v-model="filters.granularity" class="control compact" @change="refreshAnalytics(true)">
          <option value="auto">自动粒度</option><option value="hour">小时</option><option value="day">天</option>
        </select>
        <input v-model.trim="filters.searchQuery" class="control wide" placeholder="搜索模型 / 账号 / API Key / 路径" @keyup.enter="refreshAnalytics(true)" />
        <select v-model="filters.status" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">全部状态</option><option value="success">仅成功</option><option value="failed">仅失败</option>
        </select>
        <select v-model="filters.provider" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">全部 Provider</option>
          <option v-for="p in providerOptions" :key="p" :value="p">{{ p }}</option>
        </select>
      </div>
      <div class="filterbar-row">
        <select v-model="filters.model" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">全部模型</option>
          <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
        </select>
        <select v-model="filters.authFile" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">全部凭据</option>
          <option v-for="f in authFileOptions" :key="f" :value="f">{{ f }}</option>
        </select>
        <select v-model="filters.minLatencyMs" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">全部延迟</option><option value="3000">&gt; 3s</option><option value="10000">&gt; 10s</option><option value="30000">&gt; 30s</option>
        </select>
        <select v-model="filters.cacheStatus" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">全部缓存</option><option value="hit">缓存命中</option><option value="miss">缓存未命中</option>
        </select>
      </div>
      <div v-if="filters.timeRange === 'custom'" class="filterbar-row">
        <label>开始 <input v-model="customStartInput" type="datetime-local" class="control" /></label>
        <label>结束 <input v-model="customEndInput" type="datetime-local" class="control" /></label>
        <button class="btn primary" @click="refreshAnalytics(true)">应用</button>
      </div>
    </div>

    <section v-if="analyticsError" class="notice error">{{ analyticsError }}</section>

    <!-- Analytics KPI（所选时间范围） -->
    <MetricGrid :cards="analyticsKpi" />

    <!-- Analytics tabs -->
    <div class="monitor-tabs card">
      <button v-for="tab in analyticsTabs" :key="tab.key" :class="['tab', {active: analyticsTab === tab.key}]" @click="analyticsTab = tab.key">{{ tab.label }}</button>
    </div>

    <!-- Overview -->
    <div v-if="analyticsTab === 'overview'" class="usage-tab-content">
      <DataCard title="时间线" :subtitle="granularityLabel">
        <div class="timeline-bars" v-if="timelineRows.length">
          <div v-for="point in timelineRows" :key="point.bucket_ms" class="timeline-row" :class="{selected: selectedBucketMs === point.bucket_ms}" @click="selectBucket(point)">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: barWidth(point.calls)}"></i></div>
            <span class="timeline-value">{{ fmtInt(point.calls) }}</span>
            <span class="timeline-sub">{{ fmtCompact(point.tokens) }} tok · {{ fmtMoney(point.cost) }}</span>
          </div>
        </div>
        <div v-else class="empty">暂无时间线数据</div>
      </DataCard>
      <div class="split">
        <DataCard title="模型排名" subtitle="Top 8"><SimpleTable :rows="topModels" :columns="rankColumns" selectable :selected-id="selectedModelId" @select="row => selectedModelId = row.id || row.model" /></DataCard>
        <DataCard title="API Key 排名" subtitle="Top 8"><SimpleTable :rows="topApiKeys" :columns="apiKeyRankColumns" selectable :selected-id="selectedApiKeyHash" @select="row => { selectedApiKeyHash = row.api_key_hash || row.id; loadSelectedApiKeyTimeline(); }" /></DataCard>
      </div>
      <DataCard v-if="selectedBucketMs && drilldownRows.length" title="钻取预览" subtitle="选中时间桶"><SimpleTable :rows="drilldownRows" :columns="drilldownColumns" /></DataCard>
      <DataCard v-if="anomalyRows.length" title="风险时段" :subtitle="`${anomalyRows.length} 个`"><SimpleTable :rows="anomalyRows" :columns="anomalyColumns" /></DataCard>
    </div>

    <!-- Trends -->
    <div v-if="analyticsTab === 'trends'" class="usage-tab-content">
      <DataCard title="趋势" :subtitle="trendMetricLabel">
        <div class="trend-controls">
          <button v-for="m in trendMetrics" :key="m.key" :class="['tab', {active: trendMetric === m.key}]" @click="trendMetric = m.key">{{ m.label }}</button>
        </div>
        <div class="timeline-bars" v-if="timelineRows.length">
          <div v-for="point in timelineRows" :key="point.bucket_ms" class="timeline-row" :class="{selected: selectedBucketMs === point.bucket_ms}" @click="selectBucket(point)">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: trendBarWidth(point)}"></i></div>
            <span class="timeline-value">{{ formatTrendValue(point) }}</span>
          </div>
        </div>
        <div v-else class="empty">暂无趋势数据</div>
      </DataCard>
      <DataCard v-if="selectedBucketMs && drilldownRows.length" title="钻取预览" subtitle="选中时间桶"><SimpleTable :rows="drilldownRows" :columns="drilldownColumns" /></DataCard>
    </div>

    <!-- Models -->
    <div v-if="analyticsTab === 'models'" class="usage-tab-content">
      <DataCard title="模型维度" subtitle="model_stats / model_share"><SimpleTable :rows="modelRows" :columns="modelColumns" selectable :selected-id="selectedModelId" @select="row => selectedModelId = row.id || row.model" /></DataCard>
      <DataCard v-if="selectedModel" title="模型详情" :subtitle="selectedModel.model || '—'">
        <div class="detail-card-head-action"><button class="btn" @click="openMonitoringForModel(selectedModel)">查看请求详情</button></div>
        <DetailGrid :items="buildModelDetail(selectedModel)" />
      </DataCard>
    </div>

    <!-- API Keys -->
    <div v-if="analyticsTab === 'apiKeys'" class="usage-tab-content">
      <DataCard title="API Key 维度" subtitle="api_key_stats"><SimpleTable :rows="apiKeyRows" :columns="apiKeyColumns" selectable :selected-id="selectedApiKeyHash" @select="row => { selectedApiKeyHash = row.api_key_hash || row.id; loadSelectedApiKeyTimeline(); }" /></DataCard>
      <DataCard v-if="selectedApiKeyTimeline.length" title="选中 API Key 趋势" :subtitle="selectedApiKey?.api_key_hash || selectedApiKey?.id || '—'">
        <div class="timeline-bars">
          <div v-for="point in selectedApiKeyTimeline" :key="point.bucket_ms" class="timeline-row">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: trendBarWidth(point)}"></i></div>
            <span class="timeline-value">{{ formatTrendValue(point) }}</span>
          </div>
        </div>
      </DataCard>
      <DataCard v-if="selectedApiKey" title="API Key 详情" :subtitle="selectedApiKey.api_key_hash || selectedApiKey.id || '—'">
        <div class="detail-card-head-action"><button class="btn" @click="openMonitoringForApiKey(selectedApiKey)">查看请求详情</button></div>
        <DetailGrid :items="buildApiKeyDetail(selectedApiKey)" />
      </DataCard>
    </div>

    <!-- Credentials -->
    <div v-if="analyticsTab === 'credentials'" class="usage-tab-content">
      <DataCard title="凭据维度" subtitle="credential_stats / credential_timeline"><SimpleTable :rows="credentialRows" :columns="credentialColumns" selectable :selected-id="selectedCredentialId" @select="row => selectedCredentialId = row.id || row.auth_file || row.authFile" /></DataCard>
      <DataCard v-if="selectedCredentialTimelineRows.length" title="凭据趋势" :subtitle="selectedCredential?.auth_file || selectedCredential?.authFile || selectedCredential?.id || '—'">
        <div class="timeline-bars">
          <div v-for="point in selectedCredentialTimelineRows" :key="point.bucket_ms" class="timeline-row">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: credentialTrendBarWidth(point)}"></i></div>
            <span class="timeline-value">{{ fmtCompact(point.calls) }}</span>
            <span class="timeline-sub">{{ fmtCompact(point.total_tokens) }} tok · {{ fmtMoney(point.cost) }}</span>
          </div>
        </div>
      </DataCard>
      <DataCard v-if="selectedCredential" title="账号凭据详情" :subtitle="selectedCredential.auth_file || selectedCredential.authFile || selectedCredential.id || '—'">
        <div class="detail-card-head-action"><button class="btn" @click="openMonitoringForCredential(selectedCredential)">查看请求详情</button></div>
        <DetailGrid :items="buildCredentialDetail(selectedCredential)" />
      </DataCard>
    </div>

    <!-- Heatmap -->
    <div v-if="analyticsTab === 'heatmap'" class="usage-tab-content">
      <DataCard title="热力图" subtitle="7×24 请求分布">
        <div class="heatmap-controls">
          <select v-model="heatmapMetric" class="control compact">
            <option value="requestCount">请求数</option><option value="totalTokens">Token 数</option><option value="estimatedCost">费用</option><option value="failureRate">失败率</option>
          </select>
          <select v-model="heatmapScaleMode" class="control compact">
            <option value="absolute">绝对值</option><option value="byWeekday">按星期归一</option><option value="byHour">按小时归一</option>
          </select>
        </div>
        <div v-if="heatmapRows.length" class="heatmap-grid-wrap">
          <table class="heatmap-table">
            <thead><tr><th></th><th v-for="h in 24" :key="h">{{ h - 1 }}</th></tr></thead>
            <tbody>
              <tr v-for="(row, wi) in heatmapRows" :key="wi">
                <td class="heatmap-day-label">{{ weekdayLabel(wi) }}</td>
                <td v-for="(cell, hi) in row" :key="hi" class="heatmap-cell" :style="heatmapCellStyle(cell)" :title="heatmapCellTitle(wi, hi, cell)" @click="selectHeatmapCell(wi, hi, cell)"></td>
              </tr>
            </tbody>
          </table>
        </div>
        <div v-else class="empty">暂无热力图数据</div>
      </DataCard>
      <DataCard v-if="selectedHeatmapCell && selectedHeatmapCell.cell" title="热力图详情" :subtitle="`${weekdayLabel(selectedHeatmapCell.weekday)} ${selectedHeatmapCell.hour}:00`">
        <DetailGrid :items="buildHeatmapDetail(selectedHeatmapCell.cell)" />
        <div class="split" style="margin-top:12px">
          <DataCard v-if="selectedHeatmapCell.cell.model_contributors?.length" title="模型贡献者" subtitle="Top"><SimpleTable :rows="selectedHeatmapCell.cell.model_contributors" :columns="heatContributorColumns" /></DataCard>
          <DataCard v-if="selectedHeatmapCell.cell.api_key_contributors?.length" title="API Key 贡献者" subtitle="Top"><SimpleTable :rows="selectedHeatmapCell.cell.api_key_contributors" :columns="heatContributorColumns" /></DataCard>
        </div>
        <DataCard v-if="selectedHeatmapCell.cell.provider_contributors?.length" title="Provider 贡献者" subtitle="Top"><SimpleTable :rows="selectedHeatmapCell.cell.provider_contributors" :columns="heatContributorColumns" /></DataCard>
      </DataCard>
    </div>
    </section>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const DAY_MS = 86400000;
const HOUR_MS = 3600000;

// ===== Dashboard state =====
const dashData = ref(null);
const dashLoading = ref(false);
const dSummary = computed(() => dashData.value?.today || {});
const rolling = computed(() => dashData.value?.rolling_30m || {});
const topModelsDash = computed(() => dashData.value?.top_models_today || []);
const modelCostRank = computed(() => dashData.value?.model_cost_rank || []);
const trafficTimeline = computed(() => [...(dashData.value?.traffic_timeline || [])].sort((a,b) => Number(b.bucket_ms||0) - Number(a.bucket_ms||0)));
const recentFailures = computed(() => dashData.value?.recent_failures || []);
const channelHealth = computed(() => dashData.value?.channel_health || []);
const tokenMix = computed(() => dashData.value?.token_mix || []);
const dashboardKpi = computed(() => {
  const s = dSummary.value; const r = rolling.value;
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
  const s = dSummary.value;
  return [
    {key:'config', label:'管理密钥', value: s.api_keys ?? '—', sub:'CPA 配置', tab:'config'},
    {key:'inspection', label:'OAuth 凭据', value: s.auth_files ?? '—', sub:'账号巡检', tab:'inspection'},
    {key:'monitoring', label:'请求监控', value: recentFailures.value.length, sub:'近期失败样本', tab:'monitoring'},
    {key:'model-prices', label:'模型单价', value: '管理', sub:'价格设置', tab:'model-prices'},
  ];
});

const configSummary = computed(() => {
  const c = dashData.value?.config_summary;
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

// ===== Analytics state (merged from UsageView) =====
const analyticsData = ref(null);
const analyticsLoading = ref(false);
const analyticsError = ref('');
const analyticsTab = ref('overview');
const selectedBucketMs = ref(null);
const selectedModelId = ref('');
const selectedApiKeyHash = ref('');
const selectedCredentialId = ref('');
const selectedHeatmapCell = ref(null);
const trendMetric = ref('requestCount');
const heatmapMetric = ref('requestCount');
const heatmapScaleMode = ref('absolute');
const selectedApiKeyTimeline = ref([]);
const customStartInput = ref('');
const customEndInput = ref('');
const filters = ref(defaultFilters());

const analyticsTabs = [
  {key:'overview', label:'概览'}, {key:'trends', label:'趋势'}, {key:'models', label:'模型'},
  {key:'apiKeys', label:'API Key'}, {key:'credentials', label:'凭据'}, {key:'heatmap', label:'热力图'},
];
const trendMetrics = [
  {key:'requestCount', label:'请求数'}, {key:'totalTokens', label:'Token 数'}, {key:'estimatedCost', label:'费用'},
];

const aSummary = computed(() => analyticsData.value?.summary || {});
const timelineRows = computed(() => [...(analyticsData.value?.timeline || [])].sort((a,b) => Number(b.bucket_ms||0) - Number(a.bucket_ms||0)));
const modelRows = computed(() => analyticsData.value?.model_stats || analyticsData.value?.model_share || []);
const apiKeyRows = computed(() => analyticsData.value?.api_key_stats || []);
const credentialRows = computed(() => analyticsData.value?.credential_stats || []);
const heatmapRaw = computed(() => analyticsData.value?.heatmap || []);
const anomalyRows = computed(() => analyticsData.value?.anomaly_points || []);
const filterOptions = computed(() => analyticsData.value?.filter_options || {});

const modelOptions = computed(() => unique([...(filterOptions.value.model_stats || []).map(x => x.model), ...modelRows.value.map(x => x.model)]));
const providerOptions = computed(() => unique([...(filterOptions.value.providers || []), ...modelRows.value.map(x => x.provider || x.auth_provider_snapshot), ...apiKeyRows.value.map(x => x.provider || x.auth_provider_snapshot)]));
const authFileOptions = computed(() => unique([...(filterOptions.value.auth_files || []), ...credentialRows.value.map(x => x.auth_file || x.authFile || x.source)]));
const granularityLabel = computed(() => analyticsData.value?.granularity || 'auto');
const trendMetricLabel = computed(() => trendMetrics.find(m => m.key === trendMetric.value)?.label || '');

const analyticsKpi = computed(() => {
  const s = aSummary.value;
  const cacheTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const totalTokens = Math.max(Number(s.total_tokens ?? 0), 0);
  const rangeLabel = filters.value.timeRange === 'today' ? '今日' : filters.value.timeRange === '24h' ? '24h' : filters.value.timeRange;
  return [
    {label:'请求数', value: fmtCompact(s.total_calls), sub:`${rangeLabel} · ${fmtInt(s.success_calls)} 成功 / ${fmtInt(s.failure_calls)} 失败`},
    {label:'成功率', value: fmtPct(s.success_rate), sub: fmtDuration(s.average_latency_ms)},
    {label:'失败 / 异常', value: fmtInt(s.failure_calls), sub: `${anomalyRows.value.length} 风险时段`},
    {label:'花费', value: fmtMoney(s.total_cost), sub:`均次 ${fmtMoney(s.average_cost_per_call)}`},
    {label:'总 Token', value: fmtCompact(s.total_tokens), sub:`推理 ${fmtCompact(s.reasoning_tokens ?? 0)}`},
    {label:'输入 Token', value: fmtCompact(s.input_tokens), sub:`占比 ${fmtPct(totalTokens > 0 ? Number(s.input_tokens ?? 0) / totalTokens : 0)}`},
    {label:'输出 Token', value: fmtCompact(s.output_tokens), sub:`占比 ${fmtPct(totalTokens > 0 ? Number(s.output_tokens ?? 0) / totalTokens : 0)}`},
    {label:'缓存 Token', value: fmtCompact(cacheTokens), sub:`命中率 ${fmtPct(computeCacheHitRate(s))}`},
  ];
});

const topModels = computed(() => [...modelRows.value].sort((a,b) => Number(b.calls ?? 0) - Number(a.calls ?? 0)).slice(0, 8));
const topApiKeys = computed(() => [...apiKeyRows.value].sort((a,b) => Number(b.calls ?? 0) - Number(a.calls ?? 0)).slice(0, 8));
const selectedModel = computed(() => modelRows.value.find(r => (r.id || r.model) === selectedModelId.value) || modelRows.value[0] || null);
const selectedApiKey = computed(() => apiKeyRows.value.find(r => (r.api_key_hash || r.id) === selectedApiKeyHash.value) || apiKeyRows.value[0] || null);
const selectedCredential = computed(() => credentialRows.value.find(r => (r.id || r.auth_file) === selectedCredentialId.value) || credentialRows.value[0] || null);

const maxTimelineCalls = computed(() => Math.max(1, ...timelineRows.value.map(p => Number(p.calls || 0))));
const maxTrendValue = computed(() => { let max = 1; for(const p of timelineRows.value){ const v = trendValue(p); if(v > max) max = v; } return max; });

const heatmapRows = computed(() => {
  const grid = Array.from({length:7}, () => Array.from({length:24}, () => null));
  for(const point of heatmapRaw.value){ const wd = Number(point.weekday ?? 0); const hr = Number(point.hour ?? 0); if(wd >= 0 && wd < 7 && hr >= 0 && hr < 24){ grid[wd][hr] = point; } }
  return grid;
});

const rankColumns = [['model','模型'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money']];
const apiKeyRankColumns = [['api_key_hash','API Key','hash'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money']];
const modelColumns = [['model','模型'], ['provider','Provider'], ['calls','请求','int'], ['success_calls','成功','int'], ['failure_calls','失败','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money']];
const apiKeyColumns = [['api_key_hash','API Key','hash'], ['account_snapshot','账号'], ['provider','Provider'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'], ['last_seen_ms','最后','time']];
const credentialColumns = [['auth_file','凭据文件'], ['provider','Provider'], ['calls','请求','int'], ['success_rate','成功率','pct'], ['total_tokens','Token','int'], ['cost','费用','money'], ['last_seen_ms','最后','time']];
const anomalyColumns = [['label','时间'], ['severity','级别'], ['calls','请求','int'], ['failure_rate','失败率','pct'], ['cost','费用','money'], ['request_change','请求变化','pct']];
const drilldownColumns = [['timestamp_ms','时间','time'], ['model','模型'], ['api_key_hash','API Key','hash'], ['provider','Provider'], ['total_tokens','Token','int'], ['cost','费用','money'], ['failure_rate','失败率','pct']];
const heatContributorColumns = [['label','标签'], ['calls','请求','int'], ['tokens','Token','int'], ['cost','费用','money'], ['failure_rate','失败率','pct'], ['share','占比','pct']];
const drilldownRows = computed(() => analyticsData.value?.drilldown_preview?.items || []);
const selectedCredentialTimelineRows = computed(() => {
  const id = selectedCredential.value?.id || selectedCredential.value?.auth_file || selectedCredential.value?.authFile || '';
  if(!id) return [];
  return (analyticsData.value?.credential_timeline || [])
    .filter(p => (p.id || p.auth_file_snapshot || p.auth_index || p.source_hash || '-') === id)
    .map(p => ({ bucket_ms: p.bucket_ms, label: p.bucket_label || (p.bucket_ms ? new Date(Number(p.bucket_ms)).toLocaleString('zh-CN', {month:'2-digit', day:'2-digit', hour:'2-digit', minute:'2-digit', hour12:false}) : '—'), calls: p.calls, total_tokens: p.total_tokens ?? p.tokens, cost: p.cost }))
    .sort((a,b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0));
});

// ===== Lifecycle =====
onMounted(() => {
  if(props.ready) refreshAll();
});
watch(() => props.ready, (ready) => { if(ready && !dashData.value) refreshAll(); });

function openTab(tab){ window.dispatchEvent(new CustomEvent('cpa-manager-plus:open-tab', { detail: { tab } })); }

async function refreshAll(){
  await Promise.all([refreshDashboard(true), refreshAnalytics(true)]);
}
defineExpose({ refresh: refreshAll });

// ===== Dashboard fetch =====
async function refreshDashboard(force=false){
  if(!props.ready) return;
  if(dashLoading.value && !force) return;
  dashLoading.value = true;
  try{
    const now = Date.now(); const d = new Date(); d.setHours(0,0,0,0);
    dashData.value = await props.proxyCall({method:'GET', path:'/v0/management/dashboard/summary', query:`today_start_ms=${d.getTime()}&now_ms=${now}&top_models=5&recent_failures=5`});
  }catch{ }finally{ dashLoading.value = false; }
}

// ===== Analytics fetch =====
async function refreshAnalytics(force=false){
  if(!props.ready) return;
  if(analyticsLoading.value && !force) return;
  analyticsLoading.value = true;
  analyticsError.value = '';
  try{
    analyticsData.value = await props.proxyCall({method:'POST', path:'/v0/management/monitoring/analytics', body:buildAnalyticsRequest()});
    selectedBucketMs.value = null;
  }catch(e){ analyticsError.value = e.message || String(e); }
  finally{ analyticsLoading.value = false; }
}
function buildAnalyticsRequest(){
  const now = Date.now(); const bounds = getRangeBounds(now); const f = {};
  if(filters.value.model !== 'all') f.models = [filters.value.model];
  if(filters.value.provider !== 'all') f.providers = [filters.value.provider.toLowerCase()];
  if(filters.value.authFile !== 'all') f.auth_files = [filters.value.authFile];
  if(filters.value.status === 'success') f.include_failed = false;
  if(filters.value.status === 'failed') f.failed_only = true;
  if(filters.value.minLatencyMs !== 'all') f.min_latency_ms = Number(filters.value.minLatencyMs);
  if(filters.value.cacheStatus !== 'all') f.cache_status = filters.value.cacheStatus;
  const granularity = resolveGranularity();
  const include = { summary:true, summary_comparison:true, timeline:true, model_stats:true, channel_share:true, api_key_stats:true, credential_stats:true, credential_timeline:true, filter_options:true, heatmap:true, anomaly_points:true, granularity,
    ...(selectedBucketMs.value ? { drilldown_preview: { from_ms: selectedBucketMs.value, to_ms: selectedBucketMs.value + (granularity === 'day' ? DAY_MS : HOUR_MS), limit: 12 } } : {}),
  };
  const request = { from_ms: bounds.fromMs, to_ms: bounds.toMs, now_ms: now, time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || '', include };
  if(filters.value.searchQuery) request.search_query = filters.value.searchQuery;
  if(Object.keys(f).length) request.filters = f;
  return request;
}
function getRangeBounds(now){
  const tr = filters.value.timeRange;
  if(tr === 'custom'){ const s = Date.parse(customStartInput.value); const e = Date.parse(customEndInput.value); if(s && e && s < e) return {fromMs:s, toMs:e}; return {fromMs: now - DAY_MS, toMs: now}; }
  if(tr === '24h') return {fromMs: now - DAY_MS, toMs: now};
  if(tr === 'today'){ const d = new Date(); d.setHours(0,0,0,0); return {fromMs: d.getTime(), toMs: now}; }
  if(tr === 'yesterday'){ const d = new Date(); d.setHours(0,0,0,0); return {fromMs: d.getTime() - DAY_MS, toMs: d.getTime()}; }
  if(tr === '7d') return {fromMs: now - 7*DAY_MS, toMs: now};
  if(tr === '30d') return {fromMs: now - 30*DAY_MS, toMs: now};
  return {fromMs: now - 7*DAY_MS, toMs: now};
}
function resolveGranularity(){ const g = filters.value.granularity; if(g === 'hour' || g === 'day') return g; if(filters.value.timeRange === '30d') return 'day'; return 'hour'; }
function selectBucket(point){ selectedBucketMs.value = selectedBucketMs.value === point?.bucket_ms ? null : point?.bucket_ms ?? null; refreshAnalytics(true); }
function selectHeatmapCell(wi, hi, cell){ if(!cell) return; selectedHeatmapCell.value = {weekday: wi, hour: hi, cell}; }
async function loadSelectedApiKeyTimeline(){
  const hash = selectedApiKeyHash.value; if(!hash) return;
  try{ const now = Date.now(); const bounds = getRangeBounds(now); const f = {};
    if(filters.value.model !== 'all') f.models = [filters.value.model];
    if(filters.value.provider !== 'all') f.providers = [filters.value.provider.toLowerCase()];
    if(filters.value.authFile !== 'all') f.auth_files = [filters.value.authFile];
    if(filters.value.status === 'success') f.include_failed = false;
    if(filters.value.status === 'failed') f.failed_only = true;
    if(filters.value.minLatencyMs !== 'all') f.min_latency_ms = Number(filters.value.minLatencyMs);
    if(filters.value.cacheStatus !== 'all') f.cache_status = filters.value.cacheStatus;
    f.api_key_hashes = [hash];
    const resp = await props.proxyCall({method:'POST', path:'/v0/management/monitoring/analytics', body:{from_ms:bounds.fromMs, to_ms:bounds.toMs, now_ms:now, time_zone:Intl.DateTimeFormat().resolvedOptions().timeZone||'', ...(filters.value.searchQuery?{search_query:filters.value.searchQuery}:{}), filters:f, include:{timeline:true, granularity:resolveGranularity()}}});
    selectedApiKeyTimeline.value = [...(resp?.timeline || [])].sort((a,b) => Number(b.bucket_ms||0) - Number(a.bucket_ms||0));
  }catch{ selectedApiKeyTimeline.value = []; }
}
function buildModelDetail(row){ if(!row) return []; return [{label:'模型',value:row.model||'—'},{label:'Provider',value:row.provider||'—'},{label:'请求',value:fmtInt(row.calls)},{label:'成功率',value:fmtPct(row.success_rate)},{label:'失败',value:fmtInt(row.failure_calls)},{label:'Token',value:fmtCompact(row.total_tokens)},{label:'费用',value:fmtMoney(row.cost)}]; }
function buildApiKeyDetail(row){ if(!row) return []; return [{label:'API Key',value:shortHash(row.api_key_hash||row.id)},{label:'账号',value:row.account_snapshot||'—'},{label:'Provider',value:row.provider||'—'},{label:'请求',value:fmtInt(row.calls)},{label:'成功率',value:fmtPct(row.success_rate)},{label:'Token',value:fmtCompact(row.total_tokens)},{label:'费用',value:fmtMoney(row.cost)},{label:'最后出现',value:row.last_seen_ms?new Date(Number(row.last_seen_ms)).toLocaleString('zh-CN',{hour12:false}):'—'}]; }
function buildCredentialDetail(row){ if(!row) return []; return [{label:'凭据文件',value:row.auth_file||row.authFile||row.id||'—'},{label:'Provider',value:row.provider||'—'},{label:'账号',value:row.account_snapshot||row.account||'—'},{label:'Auth Index',value:row.auth_index||row.authIndex||'—'},{label:'Project ID',value:row.project_id||row.projectId||'—'},{label:'请求',value:fmtInt(row.calls)},{label:'成功率',value:fmtPct(row.success_rate)},{label:'Token',value:fmtCompact(row.total_tokens)},{label:'费用',value:fmtMoney(row.cost)}]; }
function buildHeatmapDetail(cell){ if(!cell) return []; return [{label:'请求',value:fmtInt(cell.calls)},{label:'成功',value:fmtInt(cell.success)},{label:'失败',value:fmtInt(cell.failure)},{label:'Token',value:fmtCompact(cell.tokens)},{label:'费用',value:fmtMoney(cell.cost)},{label:'失败率',value:fmtPct(cell.failure_rate)}]; }
function openMonitoringWithPayload(payload){ try{sessionStorage.setItem('cpa-manager-plus:pending-monitoring-filter',JSON.stringify(payload));}catch{} window.dispatchEvent(new CustomEvent('cpa-manager-plus:open-monitoring')); }
function openMonitoringForModel(row){ openMonitoringWithPayload({model:row?.model||'all'}); }
function openMonitoringForApiKey(row){ openMonitoringWithPayload({apiKeyHash:row?.api_key_hash||row?.id||'all'}); }
function openMonitoringForCredential(row){ openMonitoringWithPayload({authFile:row?.auth_file||row?.authFile||row?.id||'all'}); }
function trendValue(point){ if(trendMetric.value==='requestCount') return Number(point.calls||0); if(trendMetric.value==='totalTokens') return Number(point.tokens||point.total_tokens||0); if(trendMetric.value==='estimatedCost') return Number(point.cost||0); return 0; }
function formatTrendValue(point){ const v = trendValue(point); if(trendMetric.value==='estimatedCost') return fmtMoney(v); return fmtCompact(v); }
function trendBarWidth(point){ return `${Math.max(2, Math.round((trendValue(point) / maxTrendValue.value) * 100))}%`; }
function credentialTrendBarWidth(point){ const rows = selectedCredentialTimelineRows.value; const max = Math.max(1, ...rows.map(p => Number(p.calls||0))); return `${Math.max(2, Math.round((Number(point.calls||0) / max) * 100))}%`; }
function barWidth(value){ return `${Math.max(2, Math.round((Number(value||0) / maxTimelineCalls.value) * 100))}%`; }
function heatmapCellValue(cell){ if(!cell) return 0; if(heatmapMetric.value==='requestCount') return Number(cell.calls||0); if(heatmapMetric.value==='totalTokens') return Number(cell.tokens||0); if(heatmapMetric.value==='estimatedCost') return Number(cell.cost||0); if(heatmapMetric.value==='failureRate') return Number(cell.failure_rate||0); return 0; }
function heatmapMaxValue(){ let max=0; for(const row of heatmapRows.value){ for(const cell of row){ const v=heatmapCellValue(cell); if(v>max) max=v; } } return Math.max(max,0.001); }
function heatmapCellStyle(cell){ if(!cell) return {background:'transparent'}; const v=heatmapCellValue(cell); const max=heatmapMaxValue(); let ratio=v/max; if(heatmapScaleMode.value==='byWeekday'||heatmapScaleMode.value==='byHour') ratio=Math.min(ratio,1); const alpha=Math.max(0.08,ratio); return {background:`color-mix(in srgb, var(--cpa-primary) ${Math.round(alpha*100)}%, transparent)`}; }
function heatmapCellTitle(wi,hi,cell){ if(!cell) return ''; return `${weekdayLabel(wi)} ${hi}:00 — ${fmtInt(cell.calls)} 请求, ${fmtCompact(cell.tokens)} tok, ${fmtMoney(cell.cost)}, 失败率 ${fmtPct(cell.failure_rate)}`; }
function weekdayLabel(idx){ return ['周日','周一','周二','周三','周四','周五','周六'][idx]||''; }
function computeCacheHitRate(s){ const inputTokens=Number(s?.input_tokens??0); const cacheReadTokens=Number(s?.cache_read_tokens??0); const cacheCreationTokens=Number(s?.cache_creation_tokens??0); const cachedTokens=Number(s?.cached_tokens??0); const totalInput=Math.max(inputTokens,cachedTokens)+cacheReadTokens+cacheCreationTokens; const hitTokens=cachedTokens+cacheReadTokens; return totalInput>0?hitTokens/totalInput:0; }
function defaultFilters(){ return {timeRange:'24h',granularity:'auto',model:'all',apiKeyHash:'all',provider:'all',authFile:'all',status:'all',searchQuery:'',minLatencyMs:'all',cacheStatus:'all'}; }
function unique(values){ return Array.from(new Set(values.map(v=>String(v||'').trim()).filter(Boolean))).sort(); }
function formatTimelineLabel(point){ if(point.label) return point.label; const d=new Date(point.bucket_ms); return d.toLocaleTimeString('zh-CN',{hour:'2-digit',minute:'2-digit',hour12:false}); }
function formatTime(ms){ if(!ms) return '—'; return new Date(Number(ms)).toLocaleTimeString('zh-CN',{hour12:false}); }
function maskSummary(s){ if(!s) return '—'; return s.length>80?s.slice(0,80)+'…':s; }
const maxTrafficCalls = computed(() => Math.max(1, ...trafficTimeline.value.map(p => Number(p.calls||p.requests||0))));
function trafficBarWidth(point){ return `${Math.max(2, Math.round((Number(point.calls||point.requests||0) / maxTrafficCalls.value) * 100))}%`; }
function fmtInt(v){ const n=Number(v||0); return Number.isFinite(n)?new Intl.NumberFormat('zh-CN').format(n):'—'; }
function fmtPct(v){ if(v==null||Number.isNaN(Number(v))) return '—'; const n=Number(v); return `${(n<=1?n*100:n).toFixed(1)}%`; }
function fmtMoney(v){ if(v==null||Number.isNaN(Number(v))) return '—'; return '$'+Number(v).toFixed(4); }
function fmtDuration(v){ const n=Number(v); if(v==null||!Number.isFinite(n)) return '—'; if(n<1000) return `${Math.round(n)} ms`; const sec=n/1000; if(sec<60) return `${sec.toFixed(sec<10?1:0)} s`; const min=Math.floor(sec/60); const rem=Math.round(sec%60); return `${min}m ${rem}s`; }
function fmtCompact(v){ const n=Number(v||0); if(!Number.isFinite(n)) return '—'; if(Math.abs(n)>=1e9) return `${(n/1e9).toFixed(2)}B`; if(Math.abs(n)>=1e6) return `${(n/1e6).toFixed(1)}M`; if(Math.abs(n)>=1e3) return `${(n/1e3).toFixed(1)}K`; return String(Math.round(n)); }
function shortHash(v){ const s=String(v||'').trim(); return s.length>14?`${s.slice(0,7)}…${s.slice(-5)}`:(s||'—'); }

const SimpleTable = defineComponent({
  props: { rows:{type:Array,default:()=>[]}, columns:{type:Array,default:()=>[]}, selectable:{type:Boolean,default:false}, selectedId:{type:[String,Number],default:''} },
  emits: ['select'],
  setup(props, {emit}){ return () => { if(!props.rows.length) return h('div',{class:'empty'},'暂无数据'); const head=h('thead',h('tr',props.columns.map(c=>h('th',c[1])))); const body=h('tbody',props.rows.slice(0,50).map((row,idx)=>{ const rowId=row.id||row.model||row.api_key_hash||row.auth_file||row.authFile||idx; const isSelected=props.selectedId&&String(props.selectedId)===String(rowId); return h('tr',props.selectable?{key:idx,class:['clickable',isSelected?'selected-row':''].filter(Boolean).join(' '),onClick:()=>emit('select',row)}:{key:idx},props.columns.map(c=>h('td',renderCell(row[c[0]],c[2])))); })); return h('div',{class:'table-wrap monitor-table'},h('table',[head,body])); }; }
});
const DetailGrid = defineComponent({
  props: { items:{type:Array,default:()=>[]} },
  setup(props){ return () => h('div',{class:'config-meta-grid'},props.items.map((item,idx)=>h('div',{key:idx},[h('span',item.label),h('strong',item.value)]))); }
});
function renderCell(v, type){ if(type==='pct') return fmtPct(v); if(type==='money') return fmtMoney(v); if(type==='ms') return fmtDuration(v); if(type==='time') return v?new Date(Number(v)).toLocaleString('zh-CN',{hour12:false}):'—'; if(type==='int') return fmtInt(v); if(type==='hash') return shortHash(v); if(Array.isArray(v)) return v.join(', '); if(v&&typeof v==='object') return JSON.stringify(v); return v==null||v===''?'—':String(v); }
</script>
