<template>
  <section class="monitoring-page">
    <!-- ====== Today overview ====== -->
    <section class="dashboard-zone">
      <div class="dashboard-zone-head">
        <h2 class="dashboard-zone-title">{{ t('dashboard.todayOverview') }}</h2>
        <span class="muted small-text">{{ t('dashboard.todayOverviewSub') }}</span>
      </div>
    <MetricGrid :cards="dashboardKpi" />

    <!-- ====== Traffic overview ====== -->
    <DataCard :title="t('dashboard.trafficTitle')" :subtitle="t('dashboard.trafficSubtitle')">
      <div class="timeline-bars" v-if="trafficTimeline.length">
        <div v-for="point in trafficTimeline" :key="point.bucket_ms || point.label" class="timeline-row">
          <span class="timeline-label">{{ formatTimelineLabel(point) }}</span>
          <div class="timeline-track"><i :style="{width: trafficBarWidth(point)}"></i></div>
          <span class="timeline-value">{{ fmtInt(point.calls || point.requests || 0) }}</span>
          <span class="timeline-sub">{{ fmtCompact(point.tokens || 0) }} tok</span>
        </div>
      </div>
      <div v-else class="empty">{{ t('dashboard.empty.traffic') }}</div>
    </DataCard>

    <!-- ====== Model cost rank + Health alerts ====== -->
    <div class="split">
      <DataCard :title="t('dashboard.modelCostRank')" :subtitle="t('dashboard.modelCostRankSub')">
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
        <div v-else class="empty">{{ dashLoading ? t('common.loading') : t('dashboard.empty.rank') }}</div>
      </DataCard>
      <DataCard :title="t('dashboard.healthAlerts')" :subtitle="t('dashboard.healthAlertsSub')">
        <div v-if="recentFailures.length" class="failure-list">
          <div v-for="fail in recentFailures" :key="fail.event_hash || fail.timestamp_ms" class="failure-item">
            <span :class="['failure-severity', fail.severity || 'bad']">{{ fail.fail_status_code || 'ERR' }}</span>
            <div>
              <div class="failure-model">{{ fail.model || EMPTY_VALUE }}</div>
              <div class="failure-summary muted small-text">{{ maskSummary(fail.fail_summary) }}</div>
            </div>
            <span class="failure-time small-text">{{ formatTime(fail.timestamp_ms) }}</span>
          </div>
        </div>
        <div v-else-if="channelHealth.length" class="channel-health-list">
          <div v-for="ch in channelHealth" :key="ch.channel || ch.provider" class="channel-health-item">
            <span class="channel-name">{{ ch.channel || ch.provider || EMPTY_VALUE }}</span>
            <span :class="['channel-status', ch.health === 'good' ? 'good-text' : ch.health === 'warn' ? 'warn-text' : 'bad-text']">{{ ch.success_rate != null ? fmtPct(ch.success_rate) : EMPTY_VALUE }}</span>
            <span class="muted small-text">{{ t('dashboard.channelCalls', { count: ch.calls || 0 }) }}</span>
          </div>
        </div>
        <div v-else class="empty">{{ dashLoading ? t('common.loading') : t('dashboard.empty.alerts') }}</div>
      </DataCard>
    </div>

    <!-- ====== Token mix ====== -->
    <DataCard v-if="tokenMix.length" :title="t('dashboard.tokenMix')" :subtitle="t('dashboard.tokenMixSub')">
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
    <DataCard v-if="configSummary.length" :title="t('dashboard.configSummary')" :subtitle="t('dashboard.configSummarySub')">
      <div class="config-summary-grid">
        <div v-for="item in configSummary" :key="item.label" class="config-summary-item">
          <span class="config-summary-label">{{ item.label }}</span>
          <span :class="['config-summary-value', item.on ? 'good-text' : item.off ? 'muted' : '']">{{ item.value }}</span>
        </div>
      </div>
    </DataCard>
    </section>

    <!-- ====== Usage analytics ====== -->
    <section class="dashboard-zone dashboard-zone-analytics">
      <div class="dashboard-zone-head">
        <h2 class="dashboard-zone-title">{{ t('dashboard.usageAnalytics') }}</h2>
        <span class="muted small-text">{{ t('dashboard.usageAnalyticsSub') }}</span>
      </div>

    <!-- Analytics filter bar -->
    <div class="card filter-card usage-filterbar">
      <div class="filterbar-row">
        <select v-model="filters.timeRange" class="control compact" @change="refreshAnalytics(true)">
          <option value="24h">{{ t('dashboard.filters.h24') }}</option>
          <option value="today">{{ t('dashboard.filters.today') }}</option>
          <option value="yesterday">{{ t('dashboard.filters.yesterday') }}</option>
          <option value="7d">{{ t('dashboard.filters.d7') }}</option>
          <option value="30d">{{ t('dashboard.filters.d30') }}</option>
          <option value="custom">{{ t('dashboard.filters.custom') }}</option>
        </select>
        <select v-model="filters.granularity" class="control compact" @change="refreshAnalytics(true)">
          <option value="auto">{{ t('dashboard.filters.autoGranularity') }}</option>
          <option value="hour">{{ t('dashboard.filters.hour') }}</option>
          <option value="day">{{ t('dashboard.filters.day') }}</option>
        </select>
        <input v-model.trim="filters.searchQuery" class="control wide" :placeholder="t('dashboard.filters.searchPlaceholder')" @keyup.enter="refreshAnalytics(true)" />
        <select v-model="filters.status" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('monitoring.filters.allStatuses') }}</option>
          <option value="success">{{ t('monitoring.filters.successOnly') }}</option>
          <option value="failed">{{ t('monitoring.filters.failedOnly') }}</option>
        </select>
        <select v-model="filters.provider" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('monitoring.filters.allProviders') }}</option>
          <option v-for="p in providerOptions" :key="p" :value="p">{{ p }}</option>
        </select>
      </div>
      <div class="filterbar-row">
        <select v-model="filters.model" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('monitoring.filters.allModels') }}</option>
          <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
        </select>
        <select v-model="filters.authFile" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('dashboard.filters.allCredentials') }}</option>
          <option v-for="f in authFileOptions" :key="f" :value="f">{{ f }}</option>
        </select>
        <select v-model="filters.minLatencyMs" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('dashboard.filters.allLatency') }}</option>
          <option value="3000">&gt; 3s</option>
          <option value="10000">&gt; 10s</option>
          <option value="30000">&gt; 30s</option>
        </select>
        <select v-model="filters.cacheStatus" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('dashboard.filters.allCache') }}</option>
          <option value="hit">{{ t('dashboard.filters.cacheHit') }}</option>
          <option value="miss">{{ t('dashboard.filters.cacheMiss') }}</option>
        </select>
      </div>
      <div v-if="filters.timeRange === 'custom'" class="filterbar-row">
        <label>{{ t('monitoring.customStart') }} <input v-model="customStartInput" type="datetime-local" class="control" /></label>
        <label>{{ t('monitoring.customEnd') }} <input v-model="customEndInput" type="datetime-local" class="control" /></label>
        <button class="btn primary" @click="refreshAnalytics(true)">{{ t('common.apply') }}</button>
      </div>
    </div>

    <section v-if="analyticsError" class="notice error">{{ analyticsError }}</section>

    <!-- Analytics KPI -->
    <MetricGrid :cards="analyticsKpi" />

    <!-- Analytics tabs -->
    <div class="monitor-tabs card">
      <button v-for="tab in analyticsTabs" :key="tab.key" :class="['tab', {active: analyticsTab === tab.key}]" @click="analyticsTab = tab.key">{{ tab.label }}</button>
    </div>

    <!-- Overview -->
    <div v-if="analyticsTab === 'overview'" class="usage-tab-content">
      <DataCard :title="t('dashboard.cards.timeline')" :subtitle="granularityLabel">
        <div class="timeline-bars" v-if="timelineRows.length">
          <div v-for="point in timelineRows" :key="point.bucket_ms" class="timeline-row" :class="{selected: selectedBucketMs === point.bucket_ms}" @click="selectBucket(point)">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: barWidth(point.calls)}"></i></div>
            <span class="timeline-value">{{ fmtInt(point.calls) }}</span>
            <span class="timeline-sub">{{ fmtCompact(point.tokens) }} tok · {{ fmtMoney(point.cost) }}</span>
          </div>
        </div>
        <div v-else class="empty">{{ t('dashboard.empty.timeline') }}</div>
      </DataCard>
      <div class="split">
        <DataCard :title="t('dashboard.cards.modelRank')" :subtitle="t('dashboard.cards.topN', { n: 8 })">
          <SimpleTable :rows="topModels" :columns="rankColumns" selectable :selected-id="selectedModelId" @select="row => selectedModelId = row.id || row.model" />
        </DataCard>
        <DataCard :title="t('dashboard.cards.apiKeyRank')" :subtitle="t('dashboard.cards.topN', { n: 8 })">
          <SimpleTable :rows="topApiKeys" :columns="apiKeyRankColumns" selectable :selected-id="selectedApiKeyHash" @select="row => { selectedApiKeyHash = row.api_key_hash || row.id; loadSelectedApiKeyTimeline(); }" />
        </DataCard>
      </div>
      <DataCard v-if="selectedBucketMs && drilldownRows.length" :title="t('dashboard.cards.drilldown')" :subtitle="t('dashboard.cards.drilldownSub')">
        <SimpleTable :rows="drilldownRows" :columns="drilldownColumns" />
      </DataCard>
      <DataCard v-if="anomalyRows.length" :title="t('dashboard.cards.riskWindows')" :subtitle="t('dashboard.cards.riskWindowsSub', { count: anomalyRows.length })">
        <SimpleTable :rows="anomalyRows" :columns="anomalyColumns" />
      </DataCard>
    </div>

    <!-- Trends -->
    <div v-if="analyticsTab === 'trends'" class="usage-tab-content">
      <DataCard :title="t('dashboard.cards.trends')" :subtitle="trendMetricLabel">
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
        <div v-else class="empty">{{ t('dashboard.empty.trends') }}</div>
      </DataCard>
      <DataCard v-if="selectedBucketMs && drilldownRows.length" :title="t('dashboard.cards.drilldown')" :subtitle="t('dashboard.cards.drilldownSub')">
        <SimpleTable :rows="drilldownRows" :columns="drilldownColumns" />
      </DataCard>
    </div>

    <!-- Models -->
    <div v-if="analyticsTab === 'models'" class="usage-tab-content">
      <DataCard :title="t('dashboard.cards.models')" :subtitle="t('dashboard.cards.modelsSub')">
        <SimpleTable :rows="modelRows" :columns="modelColumns" selectable :selected-id="selectedModelId" @select="row => selectedModelId = row.id || row.model" />
      </DataCard>
      <DataCard v-if="selectedModel" :title="t('dashboard.cards.modelDetail')" :subtitle="selectedModel.model || EMPTY_VALUE">
        <div class="detail-card-head-action"><button class="btn" @click="openMonitoringForModel(selectedModel)">{{ t('dashboard.actions.viewRequestDetail') }}</button></div>
        <DetailGrid :items="buildModelDetail(selectedModel)" />
      </DataCard>
    </div>

    <!-- API Keys -->
    <div v-if="analyticsTab === 'apiKeys'" class="usage-tab-content">
      <DataCard :title="t('dashboard.cards.apiKeys')" :subtitle="t('dashboard.cards.apiKeysSub')">
        <SimpleTable :rows="apiKeyRows" :columns="apiKeyColumns" selectable :selected-id="selectedApiKeyHash" @select="row => { selectedApiKeyHash = row.api_key_hash || row.id; loadSelectedApiKeyTimeline(); }" />
      </DataCard>
      <DataCard v-if="selectedApiKeyTimeline.length" :title="t('dashboard.cards.apiKeyTrend')" :subtitle="selectedApiKey?.api_key_hash || selectedApiKey?.id || EMPTY_VALUE">
        <div class="timeline-bars">
          <div v-for="point in selectedApiKeyTimeline" :key="point.bucket_ms" class="timeline-row">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: trendBarWidth(point)}"></i></div>
            <span class="timeline-value">{{ formatTrendValue(point) }}</span>
          </div>
        </div>
      </DataCard>
      <DataCard v-if="selectedApiKey" :title="t('dashboard.cards.apiKeyDetail')" :subtitle="selectedApiKey.api_key_hash || selectedApiKey.id || EMPTY_VALUE">
        <div class="detail-card-head-action"><button class="btn" @click="openMonitoringForApiKey(selectedApiKey)">{{ t('dashboard.actions.viewRequestDetail') }}</button></div>
        <DetailGrid :items="buildApiKeyDetail(selectedApiKey)" />
      </DataCard>
    </div>

    <!-- Credentials -->
    <div v-if="analyticsTab === 'credentials'" class="usage-tab-content">
      <DataCard :title="t('dashboard.cards.credentials')" :subtitle="t('dashboard.cards.credentialsSub')">
        <SimpleTable :rows="credentialRows" :columns="credentialColumns" selectable :selected-id="selectedCredentialId" @select="row => selectedCredentialId = row.id || row.auth_file || row.authFile" />
      </DataCard>
      <DataCard v-if="selectedCredentialTimelineRows.length" :title="t('dashboard.cards.credentialTrend')" :subtitle="selectedCredential?.auth_file || selectedCredential?.authFile || selectedCredential?.id || EMPTY_VALUE">
        <div class="timeline-bars">
          <div v-for="point in selectedCredentialTimelineRows" :key="point.bucket_ms" class="timeline-row">
            <span class="timeline-label">{{ point.label }}</span>
            <div class="timeline-track"><i :style="{width: credentialTrendBarWidth(point)}"></i></div>
            <span class="timeline-value">{{ fmtCompact(point.calls) }}</span>
            <span class="timeline-sub">{{ fmtCompact(point.total_tokens) }} tok · {{ fmtMoney(point.cost) }}</span>
          </div>
        </div>
      </DataCard>
      <DataCard v-if="selectedCredential" :title="t('dashboard.cards.credentialDetail')" :subtitle="selectedCredential.auth_file || selectedCredential.authFile || selectedCredential.id || EMPTY_VALUE">
        <div class="detail-card-head-action"><button class="btn" @click="openMonitoringForCredential(selectedCredential)">{{ t('dashboard.actions.viewRequestDetail') }}</button></div>
        <DetailGrid :items="buildCredentialDetail(selectedCredential)" />
      </DataCard>
    </div>

    <!-- Heatmap -->
    <div v-if="analyticsTab === 'heatmap'" class="usage-tab-content">
      <DataCard :title="t('dashboard.cards.heatmap')" :subtitle="t('dashboard.cards.heatmapSub')">
        <div class="heatmap-controls">
          <select v-model="heatmapMetric" class="control compact">
            <option value="requestCount">{{ t('dashboard.metrics.requestCount') }}</option>
            <option value="totalTokens">{{ t('dashboard.metrics.totalTokens') }}</option>
            <option value="estimatedCost">{{ t('dashboard.metrics.estimatedCost') }}</option>
            <option value="failureRate">{{ t('dashboard.metrics.failureRate') }}</option>
          </select>
          <select v-model="heatmapScaleMode" class="control compact">
            <option value="absolute">{{ t('dashboard.heatmapScale.absolute') }}</option>
            <option value="byWeekday">{{ t('dashboard.heatmapScale.byWeekday') }}</option>
            <option value="byHour">{{ t('dashboard.heatmapScale.byHour') }}</option>
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
        <div v-else class="empty">{{ t('dashboard.empty.heatmap') }}</div>
      </DataCard>
      <DataCard v-if="selectedHeatmapCell && selectedHeatmapCell.cell" :title="t('dashboard.cards.heatmapDetail')" :subtitle="`${weekdayLabel(selectedHeatmapCell.weekday)} ${selectedHeatmapCell.hour}:00`">
        <DetailGrid :items="buildHeatmapDetail(selectedHeatmapCell.cell)" />
        <div class="split" style="margin-top:12px">
          <DataCard v-if="selectedHeatmapCell.cell.model_contributors?.length" :title="t('dashboard.cards.modelContributors')" :subtitle="t('dashboard.cards.top')">
            <SimpleTable :rows="selectedHeatmapCell.cell.model_contributors" :columns="heatContributorColumns" />
          </DataCard>
          <DataCard v-if="selectedHeatmapCell.cell.api_key_contributors?.length" :title="t('dashboard.cards.apiKeyContributors')" :subtitle="t('dashboard.cards.top')">
            <SimpleTable :rows="selectedHeatmapCell.cell.api_key_contributors" :columns="heatContributorColumns" />
          </DataCard>
        </div>
        <DataCard v-if="selectedHeatmapCell.cell.provider_contributors?.length" :title="t('dashboard.cards.providerContributors')" :subtitle="t('dashboard.cards.top')">
          <SimpleTable :rows="selectedHeatmapCell.cell.provider_contributors" :columns="heatContributorColumns" />
        </DataCard>
      </DataCard>
    </div>
    </section>
  </section>
</template>

<script setup>
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import DataCard from './DataCard.vue';
import MetricGrid from './MetricGrid.vue';
import { localeRef } from '../localeBridge.js';
import {
  EMPTY_VALUE,
  formatBucketDateTime,
  formatDateTime,
  formatInt,
  formatShortTime,
  formatTime,
  formatWeekdayIndex,
} from '../utils/localeFormat.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t } = useI18n();

const DAY_MS = 86400000;
const HOUR_MS = 3600000;

// ===== Dashboard state =====
const dashData = ref(null);
const dashLoading = ref(false);
const dSummary = computed(() => dashData.value?.today || {});
const rolling = computed(() => dashData.value?.rolling_30m || {});
const modelCostRank = computed(() => dashData.value?.model_cost_rank || []);
const trafficTimeline = computed(() => [...(dashData.value?.traffic_timeline || [])].sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0)));
const recentFailures = computed(() => dashData.value?.recent_failures || []);
const channelHealth = computed(() => dashData.value?.channel_health || []);
const tokenMix = computed(() => dashData.value?.token_mix || []);

const dashboardKpi = computed(() => {
  const s = dSummary.value;
  const r = rolling.value;
  return [
    { label: t('dashboard.kpi.todayRequests'), value: fmtInt(s.total_calls), sub: t('dashboard.kpi.failuresSub', { count: fmtInt(s.failure_calls) }) },
    { label: t('dashboard.kpi.rpm'), value: fmtCompact(r.rpm), sub: t('dashboard.kpi.callsSub', { count: fmtInt(r.total_calls) }) },
    { label: t('dashboard.kpi.tpm'), value: fmtCompact(r.tpm), sub: t('dashboard.kpi.tokensSub', { value: fmtCompact(r.total_tokens) }) },
    { label: t('dashboard.kpi.todaySpend'), value: fmtMoney(s.total_cost), sub: t('dashboard.kpi.tokensSub', { value: fmtCompact(s.total_tokens) }) },
    { label: t('dashboard.kpi.successRate'), value: fmtPct(s.success_rate), sub: t('dashboard.kpi.successOfTotal', { success: fmtInt(s.success_calls), total: fmtInt(s.total_calls) }) },
    { label: t('dashboard.kpi.avgLatency'), value: fmtDuration(s.average_latency_ms), sub: t('dashboard.kpi.zeroTokenSub', { count: fmtInt(s.zero_token_calls) }) },
  ];
});

const quickStats = computed(() => {
  const s = dSummary.value;
  return [
    { key: 'config', label: t('dashboard.quick.apiKeys'), value: s.api_keys ?? EMPTY_VALUE, sub: t('dashboard.quick.apiKeysSub'), tab: 'config' },
    { key: 'inspection', label: t('dashboard.quick.oauth'), value: s.auth_files ?? EMPTY_VALUE, sub: t('dashboard.quick.oauthSub'), tab: 'inspection' },
    { key: 'monitoring', label: t('dashboard.quick.monitoring'), value: recentFailures.value.length, sub: t('dashboard.quick.monitoringSub'), tab: 'monitoring' },
    { key: 'model-prices', label: t('dashboard.quick.modelPrices'), value: t('dashboard.quick.modelPricesValue'), sub: t('dashboard.quick.modelPricesSub'), tab: 'model-prices' },
  ];
});

const configSummary = computed(() => {
  const c = dashData.value?.config_summary;
  if (!c) return [];
  const onOff = (on) => (on ? t('common.enabled') : t('common.disabled'));
  return [
    { label: t('dashboard.config.debug'), value: onOff(c.debug), on: c.debug, off: !c.debug },
    { label: t('dashboard.config.loggingToFile'), value: onOff(c.logging_to_file), on: c.logging_to_file, off: !c.logging_to_file },
    { label: t('dashboard.config.requestRetry'), value: String(c.request_retry ?? 0) },
    { label: t('dashboard.config.wsAuth'), value: onOff(c.ws_auth), on: c.ws_auth, off: !c.ws_auth },
    { label: t('dashboard.config.routingStrategy'), value: c.routing_strategy || EMPTY_VALUE },
    ...(c.proxy_url ? [{ label: t('dashboard.config.proxyUrl'), value: c.proxy_url }] : []),
  ];
});

// ===== Analytics state =====
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

const analyticsTabs = computed(() => [
  { key: 'overview', label: t('dashboard.tabs.overview') },
  { key: 'trends', label: t('dashboard.tabs.trends') },
  { key: 'models', label: t('dashboard.tabs.models') },
  { key: 'apiKeys', label: t('dashboard.tabs.apiKeys') },
  { key: 'credentials', label: t('dashboard.tabs.credentials') },
  { key: 'heatmap', label: t('dashboard.tabs.heatmap') },
]);

const trendMetrics = computed(() => [
  { key: 'requestCount', label: t('dashboard.metrics.requestCount') },
  { key: 'totalTokens', label: t('dashboard.metrics.totalTokens') },
  { key: 'estimatedCost', label: t('dashboard.metrics.estimatedCost') },
]);

const aSummary = computed(() => analyticsData.value?.summary || {});
const timelineRows = computed(() => [...(analyticsData.value?.timeline || [])].sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0)));
const modelRows = computed(() => analyticsData.value?.model_stats || analyticsData.value?.model_share || []);
const apiKeyRows = computed(() => analyticsData.value?.api_key_stats || []);
const credentialRows = computed(() => analyticsData.value?.credential_stats || []);
const heatmapRaw = computed(() => analyticsData.value?.heatmap || []);
const anomalyRows = computed(() => analyticsData.value?.anomaly_points || []);
const filterOptions = computed(() => analyticsData.value?.filter_options || {});

const modelOptions = computed(() => unique([...(filterOptions.value.model_stats || []).map(x => x.model), ...modelRows.value.map(x => x.model)]));
const providerOptions = computed(() => unique([...(filterOptions.value.providers || []), ...modelRows.value.map(x => x.provider || x.auth_provider_snapshot), ...apiKeyRows.value.map(x => x.provider || x.auth_provider_snapshot)]));
const authFileOptions = computed(() => unique([...(filterOptions.value.auth_files || []), ...credentialRows.value.map(x => x.auth_file || x.authFile || x.source)]));

const granularityLabel = computed(() => {
  const g = analyticsData.value?.granularity || 'auto';
  if (g === 'hour') return t('dashboard.filters.hour');
  if (g === 'day') return t('dashboard.filters.day');
  if (g === 'auto') return t('dashboard.filters.autoGranularity');
  return String(g);
});

const trendMetricLabel = computed(() => trendMetrics.value.find(m => m.key === trendMetric.value)?.label || '');

const analyticsKpi = computed(() => {
  const s = aSummary.value;
  const cacheTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const totalTokens = Math.max(Number(s.total_tokens ?? 0), 0);
  const rangeLabel = filters.value.timeRange === 'today'
    ? t('dashboard.range.today')
    : filters.value.timeRange === '24h'
      ? t('dashboard.range.h24')
      : filters.value.timeRange;
  return [
    {
      label: t('dashboard.analyticsKpi.requests'),
      value: fmtCompact(s.total_calls),
      sub: t('dashboard.analyticsKpi.successFailSub', {
        range: rangeLabel,
        success: fmtInt(s.success_calls),
        failure: fmtInt(s.failure_calls),
      }),
    },
    { label: t('dashboard.analyticsKpi.successRate'), value: fmtPct(s.success_rate), sub: fmtDuration(s.average_latency_ms) },
    {
      label: t('dashboard.analyticsKpi.failureAnomaly'),
      value: fmtInt(s.failure_calls),
      sub: t('dashboard.analyticsKpi.riskWindowsSub', { count: anomalyRows.value.length }),
    },
    { label: t('dashboard.analyticsKpi.spend'), value: fmtMoney(s.total_cost), sub: t('dashboard.analyticsKpi.avgCostSub', { value: fmtMoney(s.average_cost_per_call) }) },
    { label: t('dashboard.analyticsKpi.totalTokens'), value: fmtCompact(s.total_tokens), sub: t('monitoring.kpi.reasoningSub', { value: fmtCompact(s.reasoning_tokens ?? 0) }) },
    {
      label: t('dashboard.analyticsKpi.inputTokens'),
      value: fmtCompact(s.input_tokens),
      sub: t('monitoring.kpi.shareSub', { value: fmtPct(totalTokens > 0 ? Number(s.input_tokens ?? 0) / totalTokens : 0) }),
    },
    {
      label: t('dashboard.analyticsKpi.outputTokens'),
      value: fmtCompact(s.output_tokens),
      sub: t('monitoring.kpi.shareSub', { value: fmtPct(totalTokens > 0 ? Number(s.output_tokens ?? 0) / totalTokens : 0) }),
    },
    {
      label: t('dashboard.analyticsKpi.cacheTokens'),
      value: fmtCompact(cacheTokens),
      sub: t('monitoring.kpi.hitRateSub', { value: fmtPct(computeCacheHitRate(s)) }),
    },
  ];
});

const topModels = computed(() => [...modelRows.value].sort((a, b) => Number(b.calls ?? 0) - Number(a.calls ?? 0)).slice(0, 8));
const topApiKeys = computed(() => [...apiKeyRows.value].sort((a, b) => Number(b.calls ?? 0) - Number(a.calls ?? 0)).slice(0, 8));
const selectedModel = computed(() => modelRows.value.find(r => (r.id || r.model) === selectedModelId.value) || modelRows.value[0] || null);
const selectedApiKey = computed(() => apiKeyRows.value.find(r => (r.api_key_hash || r.id) === selectedApiKeyHash.value) || apiKeyRows.value[0] || null);
const selectedCredential = computed(() => credentialRows.value.find(r => (r.id || r.auth_file) === selectedCredentialId.value) || credentialRows.value[0] || null);

const maxTimelineCalls = computed(() => Math.max(1, ...timelineRows.value.map(p => Number(p.calls || 0))));
const maxTrendValue = computed(() => {
  let max = 1;
  for (const p of timelineRows.value) {
    const v = trendValue(p);
    if (v > max) max = v;
  }
  return max;
});

const heatmapRows = computed(() => {
  const grid = Array.from({ length: 7 }, () => Array.from({ length: 24 }, () => null));
  for (const point of heatmapRaw.value) {
    const wd = Number(point.weekday ?? 0);
    const hr = Number(point.hour ?? 0);
    if (wd >= 0 && wd < 7 && hr >= 0 && hr < 24) grid[wd][hr] = point;
  }
  return grid;
});

const rankColumns = computed(() => [
  ['model', t('dashboard.columns.model')],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['success_rate', t('dashboard.columns.successRate'), 'pct'],
  ['total_tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
]);
const apiKeyRankColumns = computed(() => [
  ['api_key_hash', t('dashboard.columns.apiKey'), 'hash'],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['success_rate', t('dashboard.columns.successRate'), 'pct'],
  ['total_tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
]);
const modelColumns = computed(() => [
  ['model', t('dashboard.columns.model')],
  ['provider', t('dashboard.columns.provider')],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['success_calls', t('dashboard.columns.success'), 'int'],
  ['failure_calls', t('dashboard.columns.failure'), 'int'],
  ['success_rate', t('dashboard.columns.successRate'), 'pct'],
  ['total_tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
]);
const apiKeyColumns = computed(() => [
  ['api_key_hash', t('dashboard.columns.apiKey'), 'hash'],
  ['account_snapshot', t('dashboard.columns.account')],
  ['provider', t('dashboard.columns.provider')],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['success_rate', t('dashboard.columns.successRate'), 'pct'],
  ['total_tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
  ['last_seen_ms', t('dashboard.columns.last'), 'time'],
]);
const credentialColumns = computed(() => [
  ['auth_file', t('dashboard.columns.authFile')],
  ['provider', t('dashboard.columns.provider')],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['success_rate', t('dashboard.columns.successRate'), 'pct'],
  ['total_tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
  ['last_seen_ms', t('dashboard.columns.last'), 'time'],
]);
const anomalyColumns = computed(() => [
  ['label', t('dashboard.columns.time')],
  ['severity', t('dashboard.columns.severity')],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['failure_rate', t('dashboard.columns.failureRate'), 'pct'],
  ['cost', t('dashboard.columns.cost'), 'money'],
  ['request_change', t('dashboard.columns.requestChange'), 'pct'],
]);
const drilldownColumns = computed(() => [
  ['timestamp_ms', t('dashboard.columns.time'), 'time'],
  ['model', t('dashboard.columns.model')],
  ['api_key_hash', t('dashboard.columns.apiKey'), 'hash'],
  ['provider', t('dashboard.columns.provider')],
  ['total_tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
  ['failure_rate', t('dashboard.columns.failureRate'), 'pct'],
]);
const heatContributorColumns = computed(() => [
  ['label', t('dashboard.columns.label')],
  ['calls', t('dashboard.columns.requests'), 'int'],
  ['tokens', t('dashboard.columns.token'), 'int'],
  ['cost', t('dashboard.columns.cost'), 'money'],
  ['failure_rate', t('dashboard.columns.failureRate'), 'pct'],
  ['share', t('dashboard.columns.share'), 'pct'],
]);

const drilldownRows = computed(() => analyticsData.value?.drilldown_preview?.items || []);
const selectedCredentialTimelineRows = computed(() => {
  const id = selectedCredential.value?.id || selectedCredential.value?.auth_file || selectedCredential.value?.authFile || '';
  if (!id) return [];
  return (analyticsData.value?.credential_timeline || [])
    .filter(p => (p.id || p.auth_file_snapshot || p.auth_index || p.source_hash || '-') === id)
    .map(p => ({
      bucket_ms: p.bucket_ms,
      label: p.bucket_label || (p.bucket_ms ? formatBucketDateTime(p.bucket_ms, localeRef.value) : EMPTY_VALUE),
      calls: p.calls,
      total_tokens: p.total_tokens ?? p.tokens,
      cost: p.cost,
    }))
    .sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0));
});

// ===== Lifecycle =====
onMounted(() => {
  if (props.ready) refreshAll();
});
watch(() => props.ready, (ready) => {
  if (ready && !dashData.value) refreshAll();
});

function openTab(tab) {
  window.dispatchEvent(new CustomEvent('cpa-manager-plus:open-tab', { detail: { tab } }));
}

async function refreshAll() {
  await Promise.all([refreshDashboard(true), refreshAnalytics(true)]);
}
defineExpose({ refresh: refreshAll });

// ===== Dashboard fetch =====
async function refreshDashboard(force = false) {
  if (!props.ready) return;
  if (dashLoading.value && !force) return;
  dashLoading.value = true;
  try {
    const now = Date.now();
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    dashData.value = await props.proxyCall({
      method: 'GET',
      path: '/v0/management/dashboard/summary',
      query: `today_start_ms=${d.getTime()}&now_ms=${now}&top_models=5&recent_failures=5`,
    });
  } catch {
    // Keep previous dashboard snapshot on fetch failure.
  } finally {
    dashLoading.value = false;
  }
}

// ===== Analytics fetch =====
async function refreshAnalytics(force = false) {
  if (!props.ready) return;
  if (analyticsLoading.value && !force) return;
  analyticsLoading.value = true;
  analyticsError.value = '';
  try {
    analyticsData.value = await props.proxyCall({
      method: 'POST',
      path: '/v0/management/monitoring/analytics',
      body: buildAnalyticsRequest(),
    });
    selectedBucketMs.value = null;
  } catch (e) {
    analyticsError.value = e.message || String(e);
  } finally {
    analyticsLoading.value = false;
  }
}

function buildAnalyticsRequest() {
  const now = Date.now();
  const bounds = getRangeBounds(now);
  const f = {};
  if (filters.value.model !== 'all') f.models = [filters.value.model];
  if (filters.value.provider !== 'all') f.providers = [filters.value.provider.toLowerCase()];
  if (filters.value.authFile !== 'all') f.auth_files = [filters.value.authFile];
  if (filters.value.status === 'success') f.include_failed = false;
  if (filters.value.status === 'failed') f.failed_only = true;
  if (filters.value.minLatencyMs !== 'all') f.min_latency_ms = Number(filters.value.minLatencyMs);
  if (filters.value.cacheStatus !== 'all') f.cache_status = filters.value.cacheStatus;
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
    ...(selectedBucketMs.value
      ? {
          drilldown_preview: {
            from_ms: selectedBucketMs.value,
            to_ms: selectedBucketMs.value + (granularity === 'day' ? DAY_MS : HOUR_MS),
            limit: 12,
          },
        }
      : {}),
  };
  const request = {
    from_ms: bounds.fromMs,
    to_ms: bounds.toMs,
    now_ms: now,
    time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || '',
    include,
  };
  if (filters.value.searchQuery) request.search_query = filters.value.searchQuery;
  if (Object.keys(f).length) request.filters = f;
  return request;
}

function getRangeBounds(now) {
  const tr = filters.value.timeRange;
  if (tr === 'custom') {
    const s = Date.parse(customStartInput.value);
    const e = Date.parse(customEndInput.value);
    if (s && e && s < e) return { fromMs: s, toMs: e };
    return { fromMs: now - DAY_MS, toMs: now };
  }
  if (tr === '24h') return { fromMs: now - DAY_MS, toMs: now };
  if (tr === 'today') {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return { fromMs: d.getTime(), toMs: now };
  }
  if (tr === 'yesterday') {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return { fromMs: d.getTime() - DAY_MS, toMs: d.getTime() };
  }
  if (tr === '7d') return { fromMs: now - 7 * DAY_MS, toMs: now };
  if (tr === '30d') return { fromMs: now - 30 * DAY_MS, toMs: now };
  return { fromMs: now - 7 * DAY_MS, toMs: now };
}

function resolveGranularity() {
  const g = filters.value.granularity;
  if (g === 'hour' || g === 'day') return g;
  if (filters.value.timeRange === '30d') return 'day';
  return 'hour';
}

function selectBucket(point) {
  selectedBucketMs.value = selectedBucketMs.value === point?.bucket_ms ? null : point?.bucket_ms ?? null;
  refreshAnalytics(true);
}

function selectHeatmapCell(wi, hi, cell) {
  if (!cell) return;
  selectedHeatmapCell.value = { weekday: wi, hour: hi, cell };
}

async function loadSelectedApiKeyTimeline() {
  const hash = selectedApiKeyHash.value;
  if (!hash) return;
  try {
    const now = Date.now();
    const bounds = getRangeBounds(now);
    const f = {};
    if (filters.value.model !== 'all') f.models = [filters.value.model];
    if (filters.value.provider !== 'all') f.providers = [filters.value.provider.toLowerCase()];
    if (filters.value.authFile !== 'all') f.auth_files = [filters.value.authFile];
    if (filters.value.status === 'success') f.include_failed = false;
    if (filters.value.status === 'failed') f.failed_only = true;
    if (filters.value.minLatencyMs !== 'all') f.min_latency_ms = Number(filters.value.minLatencyMs);
    if (filters.value.cacheStatus !== 'all') f.cache_status = filters.value.cacheStatus;
    f.api_key_hashes = [hash];
    const resp = await props.proxyCall({
      method: 'POST',
      path: '/v0/management/monitoring/analytics',
      body: {
        from_ms: bounds.fromMs,
        to_ms: bounds.toMs,
        now_ms: now,
        time_zone: Intl.DateTimeFormat().resolvedOptions().timeZone || '',
        ...(filters.value.searchQuery ? { search_query: filters.value.searchQuery } : {}),
        filters: f,
        include: { timeline: true, granularity: resolveGranularity() },
      },
    });
    selectedApiKeyTimeline.value = [...(resp?.timeline || [])].sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0));
  } catch {
    selectedApiKeyTimeline.value = [];
  }
}

function buildModelDetail(row) {
  if (!row) return [];
  return [
    { label: t('dashboard.labels.model'), value: row.model || EMPTY_VALUE },
    { label: t('dashboard.labels.provider'), value: row.provider || EMPTY_VALUE },
    { label: t('dashboard.labels.requests'), value: fmtInt(row.calls) },
    { label: t('dashboard.labels.successRate'), value: fmtPct(row.success_rate) },
    { label: t('dashboard.labels.failure'), value: fmtInt(row.failure_calls) },
    { label: t('dashboard.labels.token'), value: fmtCompact(row.total_tokens) },
    { label: t('dashboard.labels.cost'), value: fmtMoney(row.cost) },
  ];
}

function buildApiKeyDetail(row) {
  if (!row) return [];
  return [
    { label: t('dashboard.labels.apiKey'), value: shortHash(row.api_key_hash || row.id) },
    { label: t('dashboard.labels.account'), value: row.account_snapshot || EMPTY_VALUE },
    { label: t('dashboard.labels.provider'), value: row.provider || EMPTY_VALUE },
    { label: t('dashboard.labels.requests'), value: fmtInt(row.calls) },
    { label: t('dashboard.labels.successRate'), value: fmtPct(row.success_rate) },
    { label: t('dashboard.labels.token'), value: fmtCompact(row.total_tokens) },
    { label: t('dashboard.labels.cost'), value: fmtMoney(row.cost) },
    { label: t('dashboard.labels.lastSeen'), value: row.last_seen_ms ? formatDateTime(row.last_seen_ms, localeRef.value) : EMPTY_VALUE },
  ];
}

function buildCredentialDetail(row) {
  if (!row) return [];
  return [
    { label: t('dashboard.labels.authFile'), value: row.auth_file || row.authFile || row.id || EMPTY_VALUE },
    { label: t('dashboard.labels.provider'), value: row.provider || EMPTY_VALUE },
    { label: t('dashboard.labels.account'), value: row.account_snapshot || row.account || EMPTY_VALUE },
    { label: t('dashboard.labels.authIndex'), value: row.auth_index || row.authIndex || EMPTY_VALUE },
    { label: t('dashboard.labels.projectId'), value: row.project_id || row.projectId || EMPTY_VALUE },
    { label: t('dashboard.labels.requests'), value: fmtInt(row.calls) },
    { label: t('dashboard.labels.successRate'), value: fmtPct(row.success_rate) },
    { label: t('dashboard.labels.token'), value: fmtCompact(row.total_tokens) },
    { label: t('dashboard.labels.cost'), value: fmtMoney(row.cost) },
  ];
}

function buildHeatmapDetail(cell) {
  if (!cell) return [];
  return [
    { label: t('dashboard.labels.requests'), value: fmtInt(cell.calls) },
    { label: t('dashboard.labels.success'), value: fmtInt(cell.success) },
    { label: t('dashboard.labels.failure'), value: fmtInt(cell.failure) },
    { label: t('dashboard.labels.token'), value: fmtCompact(cell.tokens) },
    { label: t('dashboard.labels.cost'), value: fmtMoney(cell.cost) },
    { label: t('dashboard.labels.failureRate'), value: fmtPct(cell.failure_rate) },
  ];
}

function openMonitoringWithPayload(payload) {
  try {
    sessionStorage.setItem('cpa-manager-plus:pending-monitoring-filter', JSON.stringify(payload));
  } catch {
    // sessionStorage may be unavailable.
  }
  window.dispatchEvent(new CustomEvent('cpa-manager-plus:open-monitoring'));
}

function openMonitoringForModel(row) {
  openMonitoringWithPayload({ model: row?.model || 'all' });
}

function openMonitoringForApiKey(row) {
  openMonitoringWithPayload({ apiKeyHash: row?.api_key_hash || row?.id || 'all' });
}

function openMonitoringForCredential(row) {
  openMonitoringWithPayload({ authFile: row?.auth_file || row?.authFile || row?.id || 'all' });
}

function trendValue(point) {
  if (trendMetric.value === 'requestCount') return Number(point.calls || 0);
  if (trendMetric.value === 'totalTokens') return Number(point.tokens || point.total_tokens || 0);
  if (trendMetric.value === 'estimatedCost') return Number(point.cost || 0);
  return 0;
}

function formatTrendValue(point) {
  const v = trendValue(point);
  if (trendMetric.value === 'estimatedCost') return fmtMoney(v);
  return fmtCompact(v);
}

function trendBarWidth(point) {
  return `${Math.max(2, Math.round((trendValue(point) / maxTrendValue.value) * 100))}%`;
}

function credentialTrendBarWidth(point) {
  const rows = selectedCredentialTimelineRows.value;
  const max = Math.max(1, ...rows.map(p => Number(p.calls || 0)));
  return `${Math.max(2, Math.round((Number(point.calls || 0) / max) * 100))}%`;
}

function barWidth(value) {
  return `${Math.max(2, Math.round((Number(value || 0) / maxTimelineCalls.value) * 100))}%`;
}

function heatmapCellValue(cell) {
  if (!cell) return 0;
  if (heatmapMetric.value === 'requestCount') return Number(cell.calls || 0);
  if (heatmapMetric.value === 'totalTokens') return Number(cell.tokens || 0);
  if (heatmapMetric.value === 'estimatedCost') return Number(cell.cost || 0);
  if (heatmapMetric.value === 'failureRate') return Number(cell.failure_rate || 0);
  return 0;
}

function heatmapMaxValue() {
  let max = 0;
  for (const row of heatmapRows.value) {
    for (const cell of row) {
      const v = heatmapCellValue(cell);
      if (v > max) max = v;
    }
  }
  return Math.max(max, 0.001);
}

function heatmapCellStyle(cell) {
  if (!cell) return { background: 'transparent' };
  const v = heatmapCellValue(cell);
  const max = heatmapMaxValue();
  let ratio = v / max;
  if (heatmapScaleMode.value === 'byWeekday' || heatmapScaleMode.value === 'byHour') ratio = Math.min(ratio, 1);
  const alpha = Math.max(0.08, ratio);
  return { background: `color-mix(in srgb, var(--cpa-primary) ${Math.round(alpha * 100)}%, transparent)` };
}

function heatmapCellTitle(wi, hi, cell) {
  if (!cell) return '';
  return t('dashboard.heatmapCellTitle', {
    weekday: weekdayLabel(wi),
    hour: hi,
    calls: fmtInt(cell.calls),
    tokens: fmtCompact(cell.tokens),
    cost: fmtMoney(cell.cost),
    rate: fmtPct(cell.failure_rate),
  });
}

function weekdayLabel(idx) {
  return formatWeekdayIndex(idx, localeRef.value);
}

function computeCacheHitRate(s) {
  const inputTokens = Number(s?.input_tokens ?? 0);
  const cacheReadTokens = Number(s?.cache_read_tokens ?? 0);
  const cacheCreationTokens = Number(s?.cache_creation_tokens ?? 0);
  const cachedTokens = Number(s?.cached_tokens ?? 0);
  const totalInput = Math.max(inputTokens, cachedTokens) + cacheReadTokens + cacheCreationTokens;
  const hitTokens = cachedTokens + cacheReadTokens;
  return totalInput > 0 ? hitTokens / totalInput : 0;
}

function defaultFilters() {
  return {
    timeRange: '24h',
    granularity: 'auto',
    model: 'all',
    apiKeyHash: 'all',
    provider: 'all',
    authFile: 'all',
    status: 'all',
    searchQuery: '',
    minLatencyMs: 'all',
    cacheStatus: 'all',
  };
}

function unique(values) {
  return Array.from(new Set(values.map(v => String(v || '').trim()).filter(Boolean))).sort();
}

function formatTimelineLabel(point) {
  if (point.label) return point.label;
  return formatShortTime(point.bucket_ms, localeRef.value);
}

function decodeHtmlEntities(str) {
  if (!str) return '';
  const txt = document.createElement('textarea');
  txt.innerHTML = str;
  return txt.value;
}

function maskSummary(s) {
  if (!s) return EMPTY_VALUE;
  const d = decodeHtmlEntities(s);
  return d.length > 80 ? `${d.slice(0, 80)}…` : d;
}

const maxTrafficCalls = computed(() => Math.max(1, ...trafficTimeline.value.map(p => Number(p.calls || p.requests || 0))));

function trafficBarWidth(point) {
  return `${Math.max(2, Math.round((Number(point.calls || point.requests || 0) / maxTrafficCalls.value) * 100))}%`;
}

function fmtInt(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  return formatInt(n, localeRef.value);
}

function fmtPct(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  const n = Number(v);
  return `${(n <= 1 ? n * 100 : n).toFixed(1)}%`;
}

function fmtMoney(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  return `$${Number(v).toFixed(4)}`;
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

function fmtCompact(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  if (Math.abs(n) >= 1e9) return `${(n / 1e9).toFixed(2)}B`;
  if (Math.abs(n) >= 1e6) return `${(n / 1e6).toFixed(1)}M`;
  if (Math.abs(n) >= 1e3) return `${(n / 1e3).toFixed(1)}K`;
  return String(Math.round(n));
}

function shortHash(v) {
  const s = String(v || '').trim();
  return s.length > 14 ? `${s.slice(0, 7)}…${s.slice(-5)}` : (s || EMPTY_VALUE);
}

const SimpleTable = defineComponent({
  props: {
    rows: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    selectable: { type: Boolean, default: false },
    selectedId: { type: [String, Number], default: '' },
  },
  emits: ['select'],
  setup(props, { emit }) {
    const { t: ti18n } = useI18n();
    return () => {
      if (!props.rows.length) return h('div', { class: 'empty' }, ti18n('common.noData'));
      const head = h('thead', h('tr', props.columns.map(c => h('th', c[1]))));
      const body = h(
        'tbody',
        props.rows.slice(0, 50).map((row, idx) => {
          const rowId = row.id || row.model || row.api_key_hash || row.auth_file || row.authFile || idx;
          const isSelected = props.selectedId && String(props.selectedId) === String(rowId);
          return h(
            'tr',
            props.selectable
              ? {
                  key: idx,
                  class: ['clickable', isSelected ? 'selected-row' : ''].filter(Boolean).join(' '),
                  onClick: () => emit('select', row),
                }
              : { key: idx },
            props.columns.map(c => h('td', renderCell(row[c[0]], c[2]))),
          );
        }),
      );
      return h('div', { class: 'table-wrap monitor-table' }, h('table', [head, body]));
    };
  },
});

const DetailGrid = defineComponent({
  props: { items: { type: Array, default: () => [] } },
  setup(props) {
    return () => h(
      'div',
      { class: 'config-meta-grid' },
      props.items.map((item, idx) => h('div', { key: idx }, [h('span', item.label), h('strong', item.value)])),
    );
  },
});

function renderCell(v, type) {
  if (type === 'pct') return fmtPct(v);
  if (type === 'money') return fmtMoney(v);
  if (type === 'ms') return fmtDuration(v);
  if (type === 'time') return formatDateTime(v, localeRef.value);
  if (type === 'int') return fmtInt(v);
  if (type === 'hash') return shortHash(v);
  if (Array.isArray(v)) return v.join(', ');
  if (v && typeof v === 'object') return JSON.stringify(v);
  return v == null || v === '' ? EMPTY_VALUE : String(v);
}
</script>
