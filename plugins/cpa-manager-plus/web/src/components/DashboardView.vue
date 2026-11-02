<template>
  <section class="monitoring-page dash-unified-page">
    <!-- ====== Unified Command & Quick Filter Bar ====== -->
    <div class="card dash-command-bar">
      <div class="dash-command-top">
        <div class="dash-range-pills segmented-control" role="group" :aria-label="t('dashboard.usageAnalytics')">
          <button
            v-for="opt in timeRangeOptions"
            :key="opt.value"
            type="button"
            :class="['segment-btn', { active: filters.timeRange === opt.value }]"
            @click="setTimeRange(opt.value)"
          >
            {{ opt.label }}
          </button>
        </div>

        <div class="dash-command-right">
          <div class="dash-live-pill" :title="t('dashboard.trafficTitle')">
            <span class="dash-live-dot"></span>
            <span>RPM <b>{{ fmtCompact(rolling.rpm) }}</b></span>
            <span class="dash-live-sep">·</span>
            <span>TPM <b>{{ fmtCompact(rolling.tpm) }}</b></span>
          </div>

          <button
            type="button"
            :class="['btn', 'dash-filter-toggle', { active: showAdvancedFilters || activeFilterCount > 0 }]"
            @click="showAdvancedFilters = !showAdvancedFilters"
          >
            {{ t('monitoring.labels.filter') }}
            <span v-if="activeFilterCount > 0" class="dash-filter-badge">{{ activeFilterCount }}</span>
          </button>

          <button
            type="button"
            class="btn primary"
            :disabled="dashLoading || analyticsLoading || !ready"
            @click="refreshAll"
          >
            {{ (dashLoading || analyticsLoading) ? t('common.loading') : t('common.refresh') }}
          </button>
        </div>
      </div>

      <!-- Custom Time Range Row -->
      <div v-if="filters.timeRange === 'custom'" class="dash-custom-range">
        <label>{{ t('monitoring.customStart') }} <input v-model="customStartInput" type="datetime-local" class="control compact" /></label>
        <label>{{ t('monitoring.customEnd') }} <input v-model="customEndInput" type="datetime-local" class="control compact" /></label>
        <button type="button" class="btn primary" @click="refreshAnalytics(true)">{{ t('common.apply') }}</button>
      </div>

      <!-- Collapsible Advanced Filters -->
      <div v-if="showAdvancedFilters" class="dash-filter-drawer">
        <input
          v-model.trim="filters.searchQuery"
          class="control wide"
          :placeholder="t('dashboard.filters.searchPlaceholder')"
          @keyup.enter="refreshAnalytics(true)"
        />
        <select v-model="filters.provider" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('monitoring.filters.allProviders') }}</option>
          <option v-for="p in providerOptions" :key="p" :value="p">{{ p }}</option>
        </select>
        <select v-model="filters.model" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('monitoring.filters.allModels') }}</option>
          <option v-for="m in modelOptions" :key="m" :value="m">{{ m }}</option>
        </select>
        <select v-model="filters.authFile" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('dashboard.filters.allCredentials') }}</option>
          <option v-for="f in authFileOptions" :key="f" :value="f">{{ f }}</option>
        </select>
        <select v-model="filters.status" class="control compact" @change="refreshAnalytics(true)">
          <option value="all">{{ t('monitoring.filters.allStatuses') }}</option>
          <option value="success">{{ t('monitoring.filters.successOnly') }}</option>
          <option value="failed">{{ t('monitoring.filters.failedOnly') }}</option>
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
        <button v-if="activeFilterCount > 0" type="button" class="btn" @click="resetAnalyticsFilters">
          {{ t('monitoring.reset') }}
        </button>
      </div>
    </div>

    <section v-if="analyticsError" class="notice error">{{ analyticsError }}</section>

    <!-- ====== Unified 4-Card High-Density KPI Matrix ====== -->
    <section class="dash-kpi-strip">
      <!-- 1. Requests & Throughput -->
      <article class="dash-kpi-card">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('dashboard.analyticsKpi.requests') }}</span>
          <span class="dash-kpi-chip">{{ t('dashboard.kpi.todayRequests') }} {{ fmtCompact(dSummary.total_calls) }}</span>
        </div>
        <div class="dash-kpi-value">{{ fmtInt(aSummary.total_calls) }}</div>
        <div class="dash-kpi-foot">
          <span class="good-text">{{ t('monitoring.labels.success') }} {{ fmtCompact(aSummary.success_calls) }}</span>
          <span class="dash-kpi-sep">·</span>
          <span :class="Number(aSummary.failure_calls) > 0 ? 'bad-text' : 'muted'">
            {{ t('monitoring.labels.failed') }} {{ fmtInt(aSummary.failure_calls) }}
          </span>
        </div>
      </article>

      <!-- 2. Success Rate & Latency -->
      <article class="dash-kpi-card">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('dashboard.analyticsKpi.successRate') }}</span>
          <span class="dash-kpi-chip">{{ fmtDuration(aSummary.average_latency_ms ?? dSummary.average_latency_ms) }}</span>
        </div>
        <div :class="['dash-kpi-value', rateToneClass(aSummary.success_rate)]">
          {{ fmtPct(aSummary.success_rate) }}
        </div>
        <div class="dash-kpi-foot">
          <span>{{ t('dashboard.analyticsKpi.riskWindowsSub', { count: anomalyRows.length }) }}</span>
          <span class="dash-kpi-sep">·</span>
          <span>{{ t('dashboard.kpi.zeroTokenSub', { count: fmtInt(dSummary.zero_token_calls) }) }}</span>
        </div>
      </article>

      <!-- 3. Estimated Spend -->
      <article class="dash-kpi-card">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('dashboard.analyticsKpi.spend') }}</span>
          <span class="dash-kpi-chip">{{ t('dashboard.analyticsKpi.avgCostSub', { value: fmtMoney(aSummary.average_cost_per_call) }) }}</span>
        </div>
        <div class="dash-kpi-value dash-kpi-money">{{ fmtMoney(aSummary.total_cost) }}</div>
        <div class="dash-kpi-foot">
          <span>{{ t('dashboard.kpi.todaySpend') }} <b>{{ fmtMoney(dSummary.total_cost) }}</b></span>
          <span class="dash-kpi-sep">·</span>
          <span>{{ t('dashboard.kpi.callsSub', { count: fmtCompact(rolling.total_calls) }) }}</span>
        </div>
      </article>

      <!-- 4. Total Tokens & Embedded Token Mix -->
      <article class="dash-kpi-card">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('dashboard.analyticsKpi.totalTokens') }}</span>
          <span class="dash-kpi-chip">{{ t('monitoring.kpi.hitRateSub', { value: fmtCacheHitRate(computeCacheHitRate(aSummary)) }) }}</span>
        </div>
        <div class="dash-kpi-value">{{ fmtCompact(aSummary.total_tokens) }}</div>
        <div class="dash-mini-mix" v-if="unifiedTokenMix.length">
          <div class="dash-mini-mix-bar">
            <span
              v-for="seg in unifiedTokenMix"
              :key="seg.key"
              :style="{ width: seg.widthPct, background: seg.color }"
              :title="seg.tooltip"
            ></span>
          </div>
          <div class="dash-mini-mix-legend">
            <span v-for="seg in unifiedTokenMix" :key="seg.key">
              <i :style="{ background: seg.color }"></i>{{ seg.label }} {{ fmtCompact(seg.tokens) }}
            </span>
          </div>
        </div>
        <div v-else class="dash-kpi-foot">
          <span>{{ t('dashboard.analyticsKpi.inputTokens') }} {{ fmtCompact(aSummary.input_tokens) }}</span>
          <span class="dash-kpi-sep">·</span>
          <span>{{ t('dashboard.analyticsKpi.outputTokens') }} {{ fmtCompact(aSummary.output_tokens) }}</span>
        </div>
      </article>
    </section>

    <!-- ====== Sub-View Navigation Tabs ====== -->
    <div class="monitor-tabs card dash-subtabs">
      <div class="monitor-tabs-list">
        <button
          v-for="tab in analyticsTabs"
          :key="tab.key"
          type="button"
          :class="['tab', { active: analyticsTab === tab.key }]"
          @click="analyticsTab = tab.key"
        >
          {{ tab.label }}
          <span v-if="tab.count != null" class="segment-count">{{ tab.count }}</span>
        </button>
      </div>
      <span v-if="activeAnalyticsNote" class="monitor-tabs-note">{{ activeAnalyticsNote }}</span>
    </div>

    <!-- ====== VIEW 1: Unified Overview & Interactive Trend ====== -->
    <div v-if="analyticsTab === 'overview' || analyticsTab === 'trends'" class="usage-tab-content dash-overview-stack">
      <!-- Compact Interactive Vertical Bar Chart Card -->
      <div class="card dash-trend-card">
        <div class="dash-card-head">
          <div class="dash-card-head-title">
            <h3>{{ t('dashboard.cards.timeline') }}</h3>
            <span class="muted small-text">{{ granularityLabel }}</span>
          </div>
          <div class="dash-trend-toolbar">
            <div class="segmented-control dash-mini-seg">
              <button
                v-for="m in trendMetrics"
                :key="m.key"
                type="button"
                :class="['segment-btn', { active: trendMetric === m.key }]"
                @click="trendMetric = m.key"
              >
                {{ m.label }}
              </button>
            </div>
            <select v-model="filters.granularity" class="control compact dash-granularity-select" @change="refreshAnalytics(true)">
              <option value="auto">{{ t('dashboard.filters.autoGranularity') }}</option>
              <option value="hour">{{ t('dashboard.filters.hour') }}</option>
              <option value="day">{{ t('dashboard.filters.day') }}</option>
            </select>
          </div>
        </div>

        <div v-if="chartTimelineRows.length" class="dash-vchart-wrap">
          <div class="dash-vchart" role="list">
            <button
              v-for="point in chartTimelineRows"
              :key="point.bucket_ms"
              type="button"
              :class="['dash-vbar-col', { selected: selectedBucketMs === point.bucket_ms, 'has-fail': Number(point.failure_calls || point.failures || 0) > 0 }]"
              :title="formatPointTooltip(point)"
              @click="selectBucket(point)"
            >
              <span class="dash-vbar-val">{{ formatTrendValue(point) }}</span>
              <span class="dash-vbar-track">
                <i class="dash-vbar-fill" :style="{ height: trendBarWidth(point) }"></i>
              </span>
              <span class="dash-vbar-label">{{ shortBucketLabel(point.label) }}</span>
            </button>
          </div>
        </div>
        <div v-else class="empty">{{ t('dashboard.empty.timeline') }}</div>

        <!-- Inline Bucket Drilldown Preview -->
        <div v-if="selectedBucketMs && drilldownRows.length" class="dash-drilldown-box">
          <div class="dash-drilldown-head">
            <strong>{{ t('dashboard.cards.drilldown') }} ({{ drilldownRows.length }})</strong>
            <button type="button" class="btn btn-xs" @click="selectedBucketMs = null">{{ t('common.close') }}</button>
          </div>
          <div class="dash-compact-list">
            <div v-for="(d, idx) in drilldownRows" :key="idx" class="dash-compact-item">
              <div class="dash-compact-main">
                <strong>{{ d.model || EMPTY_VALUE }}</strong>
                <span class="muted small-text">{{ d.provider || EMPTY_VALUE }} · {{ shortHash(d.api_key_hash) }}</span>
              </div>
              <div class="dash-compact-metrics">
                <span>{{ fmtCompact(d.total_tokens) }} tok</span>
                <strong>{{ fmtMoney(d.cost) }}</strong>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 3-Column Bento Grid: Top Models | Top Credentials/API Keys | Health & Alerts -->
      <div class="dash-bento-grid">
        <!-- Col 1: Top Models by Cost / Calls -->
        <div class="card dash-bento-card">
          <div class="dash-card-head">
            <div class="dash-card-head-title">
              <h3>{{ t('dashboard.cards.modelRank') }}</h3>
              <span class="muted small-text">{{ t('dashboard.cards.topN', { n: topModels.length || 6 }) }}</span>
            </div>
            <button v-if="modelRows.length > 6" type="button" class="btn btn-xs" @click="analyticsTab = 'models'">
              {{ t('common.all') }} ({{ modelRows.length }})
            </button>
          </div>

          <div v-if="topModels.length" class="dash-rank-list">
            <div
              v-for="(m, idx) in topModels.slice(0, 6)"
              :key="m.model || idx"
              class="dash-rank-row clickable"
              @click="openMonitoringForModel(m)"
            >
              <span class="dash-rank-idx">{{ idx + 1 }}</span>
              <div class="dash-rank-body">
                <div class="dash-rank-top">
                  <strong class="dash-rank-name" :title="m.model">{{ m.model || EMPTY_VALUE }}</strong>
                  <span class="dash-rank-cost">{{ fmtMoney(m.cost) }}</span>
                </div>
                <div class="dash-rank-bar-track">
                  <i :style="{ width: modelShareWidth(m) }"></i>
                </div>
                <div class="dash-rank-meta muted small-text">
                  <span>{{ fmtInt(m.calls) }} {{ t('dashboard.columns.requests') }}</span>
                  <span>·</span>
                  <span>{{ fmtCompact(m.total_tokens) }} tok</span>
                  <span>·</span>
                  <span :class="rateToneClass(m.success_rate)">{{ fmtPct(m.success_rate) }}</span>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="empty">{{ t('dashboard.empty.rank') }}</div>
        </div>

        <!-- Col 2: Top Credentials / API Keys (Switchable) -->
        <div class="card dash-bento-card">
          <div class="dash-card-head">
            <div class="dash-card-head-title">
              <h3>{{ overviewSourceMode === 'credentials' ? t('dashboard.cards.credentials') : t('dashboard.cards.apiKeyRank') }}</h3>
              <span class="muted small-text">{{ t('dashboard.cards.topN', { n: 6 }) }}</span>
            </div>
            <div class="segmented-control dash-mini-seg">
              <button
                type="button"
                :class="['segment-btn', { active: overviewSourceMode === 'credentials' }]"
                @click="overviewSourceMode = 'credentials'"
              >
                {{ t('dashboard.tabs.credentials') }}
              </button>
              <button
                type="button"
                :class="['segment-btn', { active: overviewSourceMode === 'apiKeys' }]"
                @click="overviewSourceMode = 'apiKeys'"
              >
                {{ t('dashboard.tabs.apiKeys') }}
              </button>
            </div>
          </div>

          <div v-if="overviewSourceMode === 'credentials' && topCredentials.length" class="dash-rank-list">
            <div
              v-for="(c, idx) in topCredentials.slice(0, 6)"
              :key="c.id || c.auth_file || idx"
              class="dash-rank-row clickable"
              @click="openMonitoringForCredential(c)"
            >
              <span class="dash-rank-idx">{{ idx + 1 }}</span>
              <div class="dash-rank-body">
                <div class="dash-rank-top">
                  <strong class="dash-rank-name" :title="c.auth_file || c.authFile || c.id">
                    {{ c.auth_file || c.authFile || c.account_snapshot || c.id || EMPTY_VALUE }}
                  </strong>
                  <span class="dash-rank-cost">{{ fmtMoney(c.cost) }}</span>
                </div>
                <div class="dash-rank-bar-track">
                  <i :style="{ width: credShareWidth(c) }"></i>
                </div>
                <div class="dash-rank-meta muted small-text">
                  <span v-if="c.provider">{{ c.provider }} · </span>
                  <span>{{ fmtInt(c.calls) }} {{ t('dashboard.columns.requests') }}</span>
                  <span>·</span>
                  <span>{{ fmtCompact(c.total_tokens) }} tok</span>
                  <span>·</span>
                  <span :class="rateToneClass(c.success_rate)">{{ fmtPct(c.success_rate) }}</span>
                </div>
              </div>
            </div>
          </div>
          <div v-else-if="overviewSourceMode === 'apiKeys' && topApiKeys.length" class="dash-rank-list">
            <div
              v-for="(k, idx) in topApiKeys.slice(0, 6)"
              :key="k.api_key_hash || k.id || idx"
              class="dash-rank-row clickable"
              @click="openMonitoringForApiKey(k)"
            >
              <span class="dash-rank-idx">{{ idx + 1 }}</span>
              <div class="dash-rank-body">
                <div class="dash-rank-top">
                  <strong class="dash-rank-name">{{ shortHash(k.api_key_hash || k.id) }}</strong>
                  <span class="dash-rank-cost">{{ fmtMoney(k.cost) }}</span>
                </div>
                <div class="dash-rank-bar-track">
                  <i :style="{ width: apiKeyShareWidth(k) }"></i>
                </div>
                <div class="dash-rank-meta muted small-text">
                  <span v-if="k.account_snapshot">{{ k.account_snapshot }} · </span>
                  <span>{{ fmtInt(k.calls) }} {{ t('dashboard.columns.requests') }}</span>
                  <span>·</span>
                  <span>{{ fmtCompact(k.total_tokens) }} tok</span>
                  <span>·</span>
                  <span :class="rateToneClass(k.success_rate)">{{ fmtPct(k.success_rate) }}</span>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="empty">{{ t('common.noData') }}</div>
        </div>

        <!-- Col 3: Channel Health & Recent Alerts -->
        <div class="card dash-bento-card">
          <div class="dash-card-head">
            <div class="dash-card-head-title">
              <h3>{{ t('dashboard.healthAlerts') }}</h3>
              <span class="muted small-text">{{ t('dashboard.healthAlertsSub') }}</span>
            </div>
          </div>

          <!-- Channel Health Chips -->
          <div v-if="channelHealth.length" class="dash-channel-pills">
            <div v-for="ch in channelHealth" :key="ch.channel || ch.provider" class="dash-channel-pill">
              <span class="dash-channel-name">{{ ch.channel || ch.provider || EMPTY_VALUE }}</span>
              <strong :class="rateToneClass(ch.success_rate)">
                {{ ch.success_rate != null ? fmtPct(ch.success_rate) : EMPTY_VALUE }}
              </strong>
              <span class="muted small-text">({{ fmtCompact(ch.calls || 0) }})</span>
            </div>
          </div>

          <!-- Recent Failures -->
          <div v-if="recentFailures.length" class="dash-alert-list">
            <div v-for="fail in recentFailures.slice(0, 5)" :key="fail.event_hash || fail.timestamp_ms" class="dash-alert-item">
              <span :class="['failure-severity', fail.severity || 'bad']">{{ fail.fail_status_code || 'ERR' }}</span>
              <div class="dash-alert-body">
                <div class="dash-alert-top">
                  <strong>{{ fail.model || EMPTY_VALUE }}</strong>
                  <span class="muted small-text">{{ formatTime(fail.timestamp_ms) }}</span>
                </div>
                <div class="dash-alert-msg muted small-text">{{ maskSummary(fail.fail_summary) }}</div>
              </div>
            </div>
          </div>
          <!-- Risk Windows if no recentFailures -->
          <div v-else-if="anomalyRows.length" class="dash-alert-list">
            <div v-for="(anom, idx) in anomalyRows.slice(0, 4)" :key="idx" class="dash-alert-item">
              <span class="failure-severity warn">{{ anom.severity || 'RISK' }}</span>
              <div class="dash-alert-body">
                <div class="dash-alert-top">
                  <strong>{{ anom.label }}</strong>
                  <span class="bad-text small-text">{{ t('dashboard.columns.failureRate') }} {{ fmtPct(anom.failure_rate) }}</span>
                </div>
                <div class="dash-alert-msg muted small-text">
                  {{ fmtInt(anom.calls) }} {{ t('dashboard.columns.requests') }} · {{ fmtMoney(anom.cost) }}
                </div>
              </div>
            </div>
          </div>
          <div v-else-if="!channelHealth.length" class="empty">
            {{ dashLoading ? t('common.loading') : t('dashboard.empty.alerts') }}
          </div>
        </div>
      </div>
    </div>

    <!-- ====== VIEW 2: Models Dimension (Responsive Card Grid) ====== -->
    <div v-if="analyticsTab === 'models'" class="usage-tab-content">
      <div v-if="modelRows.length" class="dash-entity-grid">
        <article
          v-for="(row, idx) in modelRows"
          :key="row.id || row.model || idx"
          :class="['card', 'dash-entity-card', 'clickable', { 'selected-row': (row.id || row.model) === (selectedModel?.id || selectedModel?.model) }]"
          @click="selectedModelId = row.id || row.model"
        >
          <div class="dash-entity-head">
            <div class="dash-entity-title-wrap">
              <strong class="dash-entity-title" :title="row.model">{{ row.model || EMPTY_VALUE }}</strong>
              <span v-if="row.provider" class="dash-kpi-chip">{{ row.provider }}</span>
            </div>
            <button type="button" class="btn btn-xs" @click.stop="openMonitoringForModel(row)">
              {{ t('dashboard.actions.viewRequestDetail') }}
            </button>
          </div>

          <div class="dash-rank-bar-track">
            <i :style="{ width: modelShareWidth(row) }"></i>
          </div>

          <div class="dash-entity-stats">
            <div>
              <span>{{ t('dashboard.columns.requests') }}</span>
              <strong>{{ fmtInt(row.calls) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.successRate') }}</span>
              <strong :class="rateToneClass(row.success_rate)">{{ fmtPct(row.success_rate) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.token') }}</span>
              <strong>{{ fmtCompact(row.total_tokens) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.cost') }}</span>
              <strong>{{ fmtMoney(row.cost) }}</strong>
            </div>
          </div>
          <div class="muted small-text dash-entity-sub">
            {{ buildUsageIOC(row, fmtCompact) }}
          </div>
        </article>
      </div>
      <div v-else class="empty">{{ t('common.noData') }}</div>
    </div>

    <!-- ====== VIEW 3: Credentials Dimension (Card Grid + Trend) ====== -->
    <div v-if="analyticsTab === 'credentials'" class="usage-tab-content dash-overview-stack">
      <div v-if="selectedCredential && selectedCredentialTimelineRows.length" class="card dash-trend-card">
        <div class="dash-card-head">
          <div class="dash-card-head-title">
            <h3>{{ t('dashboard.cards.credentialTrend') }}</h3>
            <span class="muted small-text">{{ selectedCredential.auth_file || selectedCredential.authFile || selectedCredential.id }}</span>
          </div>
          <button type="button" class="btn btn-xs" @click="openMonitoringForCredential(selectedCredential)">
            {{ t('dashboard.actions.viewRequestDetail') }}
          </button>
        </div>
        <div class="dash-vchart-wrap">
          <div class="dash-vchart">
            <div v-for="point in selectedCredentialTimelineRows.slice().reverse()" :key="point.bucket_ms" class="dash-vbar-col">
              <span class="dash-vbar-val">{{ fmtCompact(point.calls) }}</span>
              <span class="dash-vbar-track">
                <i class="dash-vbar-fill" :style="{ height: credentialTrendBarWidth(point) }"></i>
              </span>
              <span class="dash-vbar-label">{{ shortBucketLabel(point.label) }}</span>
            </div>
          </div>
        </div>
      </div>

      <div v-if="credentialRows.length" class="dash-entity-grid">
        <article
          v-for="(row, idx) in credentialRows"
          :key="row.id || row.auth_file || idx"
          :class="['card', 'dash-entity-card', 'clickable', { 'selected-row': (row.id || row.auth_file) === (selectedCredential?.id || selectedCredential?.auth_file) }]"
          @click="selectedCredentialId = row.id || row.auth_file || row.authFile"
        >
          <div class="dash-entity-head">
            <div class="dash-entity-title-wrap">
              <strong class="dash-entity-title" :title="row.auth_file || row.authFile || row.id">
                {{ row.auth_file || row.authFile || row.id || EMPTY_VALUE }}
              </strong>
              <span v-if="row.provider" class="dash-kpi-chip">{{ row.provider }}</span>
            </div>
            <button type="button" class="btn btn-xs" @click.stop="openMonitoringForCredential(row)">
              {{ t('dashboard.actions.viewRequestDetail') }}
            </button>
          </div>
          <div class="dash-entity-stats">
            <div>
              <span>{{ t('dashboard.columns.requests') }}</span>
              <strong>{{ fmtInt(row.calls) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.successRate') }}</span>
              <strong :class="rateToneClass(row.success_rate)">{{ fmtPct(row.success_rate) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.token') }}</span>
              <strong>{{ fmtCompact(row.total_tokens) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.cost') }}</span>
              <strong>{{ fmtMoney(row.cost) }}</strong>
            </div>
          </div>
          <div v-if="row.account_snapshot || row.last_seen_ms" class="muted small-text dash-entity-sub">
            <span v-if="row.account_snapshot">{{ row.account_snapshot }}</span>
            <span v-if="row.account_snapshot && row.last_seen_ms"> · </span>
            <span v-if="row.last_seen_ms">{{ formatDateTime(row.last_seen_ms, localeRef) }}</span>
          </div>
        </article>
      </div>
      <div v-else class="empty">{{ t('common.noData') }}</div>
    </div>

    <!-- ====== VIEW 4: API Keys Dimension (Card Grid + Trend) ====== -->
    <div v-if="analyticsTab === 'apiKeys'" class="usage-tab-content dash-overview-stack">
      <div v-if="selectedApiKey && selectedApiKeyTimeline.length" class="card dash-trend-card">
        <div class="dash-card-head">
          <div class="dash-card-head-title">
            <h3>{{ t('dashboard.cards.apiKeyTrend') }}</h3>
            <span class="muted small-text">{{ shortHash(selectedApiKey.api_key_hash || selectedApiKey.id) }}</span>
          </div>
          <button type="button" class="btn btn-xs" @click="openMonitoringForApiKey(selectedApiKey)">
            {{ t('dashboard.actions.viewRequestDetail') }}
          </button>
        </div>
        <div class="dash-vchart-wrap">
          <div class="dash-vchart">
            <div v-for="point in selectedApiKeyTimeline.slice().reverse()" :key="point.bucket_ms" class="dash-vbar-col">
              <span class="dash-vbar-val">{{ formatTrendValue(point) }}</span>
              <span class="dash-vbar-track">
                <i class="dash-vbar-fill" :style="{ height: trendBarWidth(point) }"></i>
              </span>
              <span class="dash-vbar-label">{{ shortBucketLabel(point.label || formatTimelineLabel(point)) }}</span>
            </div>
          </div>
        </div>
      </div>

      <div v-if="apiKeyRows.length" class="dash-entity-grid">
        <article
          v-for="(row, idx) in apiKeyRows"
          :key="row.api_key_hash || row.id || idx"
          :class="['card', 'dash-entity-card', 'clickable', { 'selected-row': (row.api_key_hash || row.id) === (selectedApiKey?.api_key_hash || selectedApiKey?.id) }]"
          @click="selectedApiKeyHash = row.api_key_hash || row.id; loadSelectedApiKeyTimeline()"
        >
          <div class="dash-entity-head">
            <div class="dash-entity-title-wrap">
              <strong class="dash-entity-title">{{ shortHash(row.api_key_hash || row.id) }}</strong>
              <span v-if="row.provider" class="dash-kpi-chip">{{ row.provider }}</span>
            </div>
            <button type="button" class="btn btn-xs" @click.stop="openMonitoringForApiKey(row)">
              {{ t('dashboard.actions.viewRequestDetail') }}
            </button>
          </div>
          <div class="dash-entity-stats">
            <div>
              <span>{{ t('dashboard.columns.requests') }}</span>
              <strong>{{ fmtInt(row.calls) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.successRate') }}</span>
              <strong :class="rateToneClass(row.success_rate)">{{ fmtPct(row.success_rate) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.token') }}</span>
              <strong>{{ fmtCompact(row.total_tokens) }}</strong>
            </div>
            <div>
              <span>{{ t('dashboard.columns.cost') }}</span>
              <strong>{{ fmtMoney(row.cost) }}</strong>
            </div>
          </div>
          <div v-if="row.account_snapshot || row.last_seen_ms" class="muted small-text dash-entity-sub">
            <span v-if="row.account_snapshot">{{ row.account_snapshot }}</span>
            <span v-if="row.account_snapshot && row.last_seen_ms"> · </span>
            <span v-if="row.last_seen_ms">{{ formatDateTime(row.last_seen_ms, localeRef) }}</span>
          </div>
        </article>
      </div>
      <div v-else class="empty">{{ t('common.noData') }}</div>
    </div>

    <!-- ====== VIEW 5: 7x24 Heatmap ====== -->
    <div v-if="analyticsTab === 'heatmap'" class="usage-tab-content">
      <DataCard>
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
      <DataCard v-if="selectedHeatmapCell && selectedHeatmapCell.cell" :title="t('dashboard.cards.heatmapDetail')" :subtitle="weekdayLabel(selectedHeatmapCell.weekday) + ' ' + selectedHeatmapCell.hour + ':00'">
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
</template>

<script setup>
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import DataCard from './DataCard.vue';
import { computeCacheHitRate, formatCacheHitRate } from '../utils/cacheHitRate.js';
import { buildUsageIOC, cacheTokenCount } from '../utils/usageBreakdown.js';
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
let dashSequence = 0;
const dSummary = computed(() => dashData.value?.today || {});
const rolling = computed(() => dashData.value?.rolling_30m || {});
const modelCostRank = computed(() => dashData.value?.model_cost_rank || []);
const trafficTimeline = computed(() => [...(dashData.value?.traffic_timeline || [])].sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0)));
const recentFailures = computed(() => dashData.value?.recent_failures || []);
const channelHealth = computed(() => dashData.value?.channel_health || []);
const tokenMix = computed(() => dashData.value?.token_mix || []);

// ===== Analytics state =====
const analyticsData = ref(null);
const analyticsLoading = ref(false);
let analyticsSequence = 0;
const analyticsError = ref('');
const analyticsTab = ref('overview');
const overviewSourceMode = ref('credentials');
const showAdvancedFilters = ref(false);
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

const timeRangeOptions = computed(() => [
  { value: 'today', label: t('dashboard.filters.today') },
  { value: '24h', label: t('dashboard.filters.h24') },
  { value: 'yesterday', label: t('dashboard.filters.yesterday') },
  { value: '7d', label: t('dashboard.filters.d7') },
  { value: '30d', label: t('dashboard.filters.d30') },
  { value: 'custom', label: t('dashboard.filters.custom') },
]);

const activeFilterCount = computed(() => {
  const f = filters.value;
  let count = 0;
  if (f.searchQuery) count += 1;
  if (f.provider !== 'all') count += 1;
  if (f.model !== 'all') count += 1;
  if (f.authFile !== 'all') count += 1;
  if (f.status !== 'all') count += 1;
  if (f.minLatencyMs !== 'all') count += 1;
  if (f.cacheStatus !== 'all') count += 1;
  return count;
});

function setTimeRange(value) {
  filters.value.timeRange = value;
  if (value !== 'custom') refreshAnalytics(true);
}

function resetAnalyticsFilters() {
  const currentRange = filters.value.timeRange;
  filters.value = { ...defaultFilters(), timeRange: currentRange };
  refreshAnalytics(true);
}

const modelRows = computed(() => analyticsData.value?.model_stats || analyticsData.value?.model_share || []);
const apiKeyRows = computed(() => analyticsData.value?.api_key_stats || []);
const credentialRows = computed(() => analyticsData.value?.credential_stats || []);

const analyticsTabs = computed(() => [
  { key: 'overview', label: t('dashboard.tabs.overview') },
  { key: 'models', label: t('dashboard.tabs.models'), count: modelRows.value.length || null },
  { key: 'credentials', label: t('dashboard.tabs.credentials'), count: credentialRows.value.length || null },
  { key: 'apiKeys', label: t('dashboard.tabs.apiKeys'), count: apiKeyRows.value.length || null },
  { key: 'heatmap', label: t('dashboard.tabs.heatmap') },
]);

const trendMetrics = computed(() => [
  { key: 'requestCount', label: t('dashboard.metrics.requestCount') },
  { key: 'totalTokens', label: t('dashboard.metrics.totalTokens') },
  { key: 'estimatedCost', label: t('dashboard.metrics.estimatedCost') },
]);

const aSummary = computed(() => analyticsData.value?.summary || dSummary.value || {});
const summaryCacheTokens = computed(() => Number(aSummary.value.cache_tokens ?? cacheTokenCount(aSummary.value)));

const unifiedTokenMix = computed(() => {
  const s = aSummary.value;
  const total = Math.max(Number(s.total_tokens || 0), 0);
  if (!total) return [];
  const input = Math.max(Number(s.input_tokens || 0), 0);
  const output = Math.max(Number(s.output_tokens || 0), 0);
  const cache = Math.max(summaryCacheTokens.value, 0);
  const sumParts = Math.max(input + output + cache, 1);
  return [
    { key: 'in', label: t('dashboard.analyticsKpi.inputTokens'), tokens: input, share: input / sumParts, widthPct: Math.max(3, Math.round((input / sumParts) * 100)) + '%', color: '#2563eb', tooltip: t('dashboard.analyticsKpi.inputTokens') + ': ' + fmtCompact(input) },
    { key: 'cache', label: t('dashboard.analyticsKpi.cacheTokens'), tokens: cache, share: cache / sumParts, widthPct: Math.max(3, Math.round((cache / sumParts) * 100)) + '%', color: '#0d9488', tooltip: t('dashboard.analyticsKpi.cacheTokens') + ': ' + fmtCompact(cache) },
    { key: 'out', label: t('dashboard.analyticsKpi.outputTokens'), tokens: output, share: output / sumParts, widthPct: Math.max(3, Math.round((output / sumParts) * 100)) + '%', color: '#d97706', tooltip: t('dashboard.analyticsKpi.outputTokens') + ': ' + fmtCompact(output) },
  ].filter(seg => seg.tokens > 0);
});

const timelineRows = computed(() => [...(analyticsData.value?.timeline || [])]
  .map((point) => ({
    ...point,
    label: point.label || (point.bucket_ms ? formatBucketDateTime(point.bucket_ms, localeRef.value) : EMPTY_VALUE),
  }))
  .sort((a, b) => Number(b.bucket_ms || 0) - Number(a.bucket_ms || 0)));

const chartTimelineRows = computed(() => {
  const src = timelineRows.value.length ? timelineRows.value : trafficTimeline.value.map(p => ({
    ...p,
    label: formatTimelineLabel(p),
  }));
  return [...src].reverse().slice(-24);
});

const heatmapRaw = computed(() => analyticsData.value?.heatmap || []);
const anomalyRows = computed(() => analyticsData.value?.anomaly_points || []);
const filterOptions = computed(() => analyticsData.value?.filter_options || {});

const modelOptions = computed(() => unique([
  ...(filterOptions.value.model_stats || []).map(x => x.model),
  ...modelRows.value.map(x => x.model),
  filters.value.model !== 'all' ? filters.value.model : '',
]));
const providerOptions = computed(() => unique([
  ...(filterOptions.value.providers || []),
  ...modelRows.value.map(x => x.provider || x.auth_provider_snapshot),
  ...apiKeyRows.value.map(x => x.provider || x.auth_provider_snapshot),
  filters.value.provider !== 'all' ? filters.value.provider : '',
]));
const authFileOptions = computed(() => unique([
  ...(filterOptions.value.auth_files || []),
  ...credentialRows.value.map(x => x.auth_file || x.authFile || x.source),
  filters.value.authFile !== 'all' ? filters.value.authFile : '',
]));

const granularityLabel = computed(() => {
  const g = analyticsData.value?.granularity || 'auto';
  if (g === 'hour') return t('dashboard.filters.hour');
  if (g === 'day') return t('dashboard.filters.day');
  if (g === 'auto') return t('dashboard.filters.autoGranularity');
  return String(g);
});

const trendMetricLabel = computed(() => trendMetrics.value.find(m => m.key === trendMetric.value)?.label || '');
const activeAnalyticsNote = computed(() => {
  if (analyticsTab.value === 'overview' || analyticsTab.value === 'trends') return granularityLabel.value + ' · ' + trendMetricLabel.value;
  if (analyticsTab.value === 'models') return t('dashboard.cards.modelsSub');
  if (analyticsTab.value === 'apiKeys') return t('dashboard.cards.apiKeysSub');
  if (analyticsTab.value === 'credentials') return t('dashboard.cards.credentialsSub');
  if (analyticsTab.value === 'heatmap') return t('dashboard.cards.heatmapSub');
  return '';
});

const topModels = computed(() => {
  if (modelRows.value.length) {
    return [...modelRows.value].sort((a, b) => Number(b.cost ?? b.calls ?? 0) - Number(a.cost ?? a.calls ?? 0)).slice(0, 8);
  }
  return modelCostRank.value.slice(0, 8);
});
const topCredentials = computed(() => [...credentialRows.value].sort((a, b) => Number(b.cost ?? b.calls ?? 0) - Number(a.cost ?? a.calls ?? 0)).slice(0, 8));
const topApiKeys = computed(() => [...apiKeyRows.value].sort((a, b) => Number(b.cost ?? b.calls ?? 0) - Number(a.cost ?? a.calls ?? 0)).slice(0, 8));

const maxModelCostOrCalls = computed(() => Math.max(1e-6, ...topModels.value.map(m => Number(m.cost || m.calls || 0))));
const maxCredCostOrCalls = computed(() => Math.max(1e-6, ...topCredentials.value.map(c => Number(c.cost || c.calls || 0))));
const maxApiKeyCostOrCalls = computed(() => Math.max(1e-6, ...topApiKeys.value.map(k => Number(k.cost || k.calls || 0))));

function modelShareWidth(row) {
  if (row?.cost_share != null) return Math.max(3, Math.round(Number(row.cost_share) * 100)) + '%';
  const v = Number(row?.cost || row?.calls || 0);
  return Math.max(3, Math.round((v / maxModelCostOrCalls.value) * 100)) + '%';
}
function credShareWidth(row) {
  const v = Number(row?.cost || row?.calls || 0);
  return Math.max(3, Math.round((v / maxCredCostOrCalls.value) * 100)) + '%';
}
function apiKeyShareWidth(row) {
  const v = Number(row?.cost || row?.calls || 0);
  return Math.max(3, Math.round((v / maxApiKeyCostOrCalls.value) * 100)) + '%';
}

const selectedModel = computed(() => modelRows.value.find(r => (r.id || r.model) === selectedModelId.value) || modelRows.value[0] || null);
const selectedApiKey = computed(() => apiKeyRows.value.find(r => (r.api_key_hash || r.id) === selectedApiKeyHash.value) || apiKeyRows.value[0] || null);
const selectedCredential = computed(() => credentialRows.value.find(r => (r.id || r.auth_file) === selectedCredentialId.value) || credentialRows.value[0] || null);

const maxTrendValue = computed(() => {
  let max = 1e-6;
  for (const p of chartTimelineRows.value) {
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

async function refreshAll() {
  await Promise.all([refreshDashboard(true), refreshAnalytics(true)]);
}
defineExpose({ refresh: refreshAll });

// ===== Dashboard fetch =====
async function refreshDashboard(force = false) {
  if (!props.ready) return;
  if (dashLoading.value && !force) return;
  const sequence = ++dashSequence;
  dashLoading.value = true;
  try {
    const now = Date.now();
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    const snapshot = await props.proxyCall({
      method: 'GET',
      path: '/v0/management/dashboard/summary',
      query: 'today_start_ms=' + d.getTime() + '&now_ms=' + now + '&top_models=5&recent_failures=5',
    });
    if (sequence === dashSequence) dashData.value = snapshot;
  } catch {
    // Keep previous dashboard snapshot on fetch failure.
  } finally {
    if (sequence === dashSequence) dashLoading.value = false;
  }
}

// ===== Analytics fetch =====
async function refreshAnalytics(force = false) {
  if (!props.ready) return;
  if (analyticsLoading.value && !force) return;
  const sequence = ++analyticsSequence;
  analyticsLoading.value = true;
  analyticsError.value = '';
  try {
    const snapshot = await props.proxyCall({
      method: 'POST',
      path: '/v0/management/monitoring/analytics',
      body: buildAnalyticsRequest(),
    });
    if (sequence !== analyticsSequence) return;
    analyticsData.value = snapshot;
    selectedBucketMs.value = null;
  } catch (e) {
    if (sequence === analyticsSequence) analyticsError.value = e.message || String(e);
  } finally {
    if (sequence === analyticsSequence) analyticsLoading.value = false;
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
  window.dispatchEvent(new CustomEvent('cpa-manager-plus:open-monitoring', { detail: payload }));
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
  if (trendMetric.value === 'requestCount') return Number(point.calls || point.requests || 0);
  if (trendMetric.value === 'totalTokens') return Number(point.tokens || point.total_tokens || 0);
  if (trendMetric.value === 'estimatedCost') return Number(point.cost || 0);
  return 0;
}

function formatTrendValue(point) {
  const v = trendValue(point);
  if (trendMetric.value === 'estimatedCost') return fmtMoney(v);
  return fmtCompact(v);
}

function formatPointTooltip(point) {
  return [
    point.label,
    fmtInt(point.calls || point.requests || 0) + ' ' + t('dashboard.columns.requests'),
    fmtCompact(point.tokens || point.total_tokens || 0) + ' tok',
    fmtMoney(point.cost || 0),
  ].join(' · ');
}

function trendBarWidth(point) {
  return Math.max(4, Math.round((trendValue(point) / maxTrendValue.value) * 100)) + '%';
}

function credentialTrendBarWidth(point) {
  const rows = selectedCredentialTimelineRows.value;
  const max = Math.max(1, ...rows.map(p => Number(p.calls || 0)));
  return Math.max(4, Math.round((Number(point.calls || 0) / max) * 100)) + '%';
}

function shortBucketLabel(label) {
  const s = String(label || '');
  const parts = s.trim().split(/\s+/);
  return parts.length > 1 ? parts[parts.length - 1] : s;
}

function rateToneClass(rate) {
  if (rate == null || Number.isNaN(Number(rate))) return '';
  const n = Number(rate) <= 1 ? Number(rate) * 100 : Number(rate);
  if (n >= 98) return 'good-text';
  if (n < 92) return 'bad-text';
  return 'warn-text';
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
  if (!cell) return {};
  const v = heatmapCellValue(cell);
  const max = heatmapMaxValue();
  let ratio = v / max;
  if (heatmapScaleMode.value === 'byWeekday' || heatmapScaleMode.value === 'byHour') ratio = Math.min(ratio, 1);
  const mix = Math.round(14 + Math.max(0, Math.min(1, ratio)) * 72);
  return { background: 'color-mix(in srgb, var(--cpa-ink) ' + mix + '%, transparent)' };
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

function fmtCacheHitRate(value) {
  return formatCacheHitRate(value, fmtPct);
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
  return d.length > 80 ? d.slice(0, 80) + '…' : d;
}

function fmtInt(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  return formatInt(n, localeRef.value);
}

function fmtPct(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  const n = Number(v);
  return (n <= 1 ? n * 100 : n).toFixed(1) + '%';
}

function fmtMoney(v) {
  if (v == null || Number.isNaN(Number(v))) return EMPTY_VALUE;
  const n = Number(v);
  return Math.abs(n) >= 10 ? '$' + n.toFixed(2) : '$' + n.toFixed(4);
}

function fmtDuration(v) {
  const n = Number(v);
  if (v == null || !Number.isFinite(n)) return EMPTY_VALUE;
  if (n < 1000) return Math.round(n) + ' ms';
  const sec = n / 1000;
  if (sec < 60) return sec.toFixed(sec < 10 ? 1 : 0) + ' s';
  const min = Math.floor(sec / 60);
  const rem = Math.round(sec % 60);
  return min + 'm ' + rem + 's';
}

function fmtCompact(v) {
  const n = Number(v || 0);
  if (!Number.isFinite(n)) return EMPTY_VALUE;
  if (Math.abs(n) >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (Math.abs(n) >= 1e6) return (n / 1e6).toFixed(1) + 'M';
  if (Math.abs(n) >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return String(Math.round(n));
}

function shortHash(v) {
  const s = String(v || '').trim();
  return s.length > 14 ? s.slice(0, 7) + '…' + s.slice(-5) : (s || EMPTY_VALUE);
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
            props.columns.map(c => h('td', renderCell(row[c[0]], c[2], row))),
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

function renderCell(v, type, row) {
  if (type === 'usage') {
    return h('div', {class: 'usage-cell'}, [
      h('strong', fmtCompact(row?.total_tokens)),
      h('div', {class: 'muted small-text usage-breakdown'}, buildUsageIOC(row, fmtCompact)),
    ]);
  }
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