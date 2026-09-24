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
      <div class="monitor-tabs-list">
        <button v-for="tab in dataTabs" :key="tab.key" :class="['tab', {active: activeDataTab === tab.key}]"
                @click="activeDataTab = tab.key">{{ tab.label }} <span>{{ tab.count }}</span></button>
      </div>
      <span v-if="activeMonitorNote" class="monitor-tabs-note">{{ activeMonitorNote }}</span>
    </div>

    <DataCard v-if="activeDataTab === 'events'">
      <div class="table-wrap monitor-table event-stream-table">
        <table>
          <thead>
          <tr>
            <th :title="t('monitoring.eventHints.sourceHeader')">{{ t('monitoring.eventColumns.sourceApiKey') }}</th>
            <th :title="t('monitoring.eventHints.modelHeader')">{{ t('monitoring.eventColumns.model') }}</th>
            <th :title="t('monitoring.eventHints.effortHeader')">{{ t('monitoring.eventColumns.effort') }}</th>
            <th :title="t('monitoring.eventHints.statusHeader')">{{ t('monitoring.eventColumns.requestStatus') }}</th>
            <th :title="t('monitoring.eventHints.healthHeader')">{{ t('monitoring.eventColumns.health') }}</th>
            <th :title="t('monitoring.eventHints.latencyHeader')">{{ t('monitoring.eventColumns.latency') }}</th>
            <th :title="t('monitoring.eventHints.tpsHeader')">{{ t('monitoring.eventColumns.tps') }}</th>
            <th :title="t('monitoring.eventHints.timeHeader')">{{ t('monitoring.eventColumns.time') }}</th>
            <th :title="t('monitoring.eventHints.usageHeader')">{{ t('monitoring.eventColumns.usage') }}</th>
            <th :title="t('monitoring.eventHints.costHeader')">{{ t('monitoring.eventColumns.cache') }}</th>
            <th :title="t('monitoring.eventHints.costHeader')">{{ t('monitoring.eventColumns.cost') }}</th>
          </tr>
          </thead>
          <tbody>
          <tr v-for="row in pagedEvents" :key="row.id" @click="selectedEvent = row.raw" class="clickable">
            <td class="event-source-cell" :title="t('monitoring.eventHints.sourceHeader')">
              <div class="event-source-identity">
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
              </div>
              <div v-if="row.providerChip.tag" class="provider-cell">
                <span :class="['provider-chip', row.providerChip.chip]">{{ row.providerChip.tag }}</span>
                <span v-if="row.providerChip.showName" class="provider-chip-name">{{ row.providerChip.name }}</span>
              </div>
            </td>
            <td class="event-model-cell">
              <button
                v-if="row.hasModelDetails"
                type="button"
                class="event-model-route"
                :class="{ 'has-response-mismatch': row.responseModelMismatch }"
                :aria-label="row.hints.model"
                :aria-expanded="modelRouteTooltip.visible && modelRouteTooltip.row?.id === row.id"
                :aria-describedby="modelRouteTooltip.visible && modelRouteTooltip.row?.id === row.id ? 'event-model-tooltip' : undefined"
                @keydown.esc.stop="hideModelRouteTooltip(true)"
                @click.stop="toggleModelRouteTooltip($event, row)"
                @mouseenter="showModelRouteTooltip($event, row)"
                @mouseleave="hideModelRouteTooltip"
              >
                <span class="event-model-name">{{ row.model }}</span>
                <span class="event-model-route-icon" aria-hidden="true">
                  <svg viewBox="2 3 10.2 10" width="13" height="13" fill="none">
                    <path d="M2.5 3.5h4.2c2.3 0 2.3 3.5 0 3.5H7M11.5 10.5H7.3c-2.3 0-2.3-3.5 0-3.5H7" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
                    <path d="M9.7 8.7 11.5 10.5 9.7 12.3" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
                  </svg>
                </span>
              </button>
              <span v-else class="event-model-name">{{ row.model }}</span>
            </td>
            <td class="event-effort-cell" :title="row.hints.model">
              <div><span class="event-metric-label">{{ t('monitoring.eventMeta.intensityLabel') }}</span> {{ row.intensityDisplay }}</div>
              <div><span class="event-metric-label">{{ t('monitoring.eventMeta.tierLabel') }}</span> {{ row.tierDisplay }}</div>
            </td>
            <td class="event-status-cell" :title="row.hints.status">
              <div class="event-status-grid">
                <span
                  v-if="row.failed"
                  class="event-status-kicker bad-text failure-trigger"
                  tabindex="0"
                  @click.stop="toggleFailureTooltip($event, row)"
                  @mouseenter="showFailureTooltip($event, row)"
                  @mouseleave="hideFailureTooltip"
                >{{ t('monitoring.labels.failed') }}</span>
                <span v-else class="event-status-kicker good-text">{{ t('monitoring.labels.success') }}</span>
                <div class="event-status-marks">
                  <span :class="['pattern-bar', row.failed ? 'bad' : 'good']"></span>
                  <span :class="['event-protocol-tag', `is-${row.protocol}`]">
                    {{ row.protocolLabel }}
                    <span v-if="row.httpStatus" class="event-protocol-code">{{ row.httpStatus }}</span>
                  </span>
                </div>
                <span class="event-status-kicker muted">{{ t('monitoring.eventMeta.recent') }}</span>
                <div class="event-status-marks">
                  <div class="recent-status" :aria-label="row.hints.status">
                    <span v-for="(success, idx) in row.recentPattern" :key="idx"
                          :class="['pattern-bar', success ? 'good' : 'bad']"></span>
                    <span v-if="!row.recentPattern.length">{{ EMPTY_VALUE }}</span>
                  </div>
                </div>
              </div>
            </td>
            <td class="event-stack-cell" :title="row.hints.health">
              <strong :class="successRateClass(row.successRate)">{{ fmtPct(row.successRate) }}</strong>
              <div class="muted small-text">{{ row.callsSub }}</div>
            </td>
            <td class="event-stack-cell" :title="row.hints.speed">
              <div :class="latencyClass(row.ttftMs)">
                <span class="event-metric-label">{{ t('monitoring.labels.firstToken') }}</span> {{ fmtSeconds(row.ttftMs) }}
              </div>
              <div :class="latencyClass(row.latencyMs)">
                <span class="event-metric-label">{{ t('monitoring.labels.elapsed') }}</span> {{ fmtSeconds(row.latencyMs) }}
              </div>
            </td>
            <td class="event-tps-cell" :title="row.hints.speed">
              <strong v-if="row.tps != null">{{ fmtTps(row.tps) }}</strong>
              <span v-else>{{ EMPTY_VALUE }}</span>
            </td>
            <td :title="t('monitoring.eventHints.timeHeader')">
              <div>{{ formatDate(row.timestampMs) }}</div>
              <div>{{ formatTime(row.timestampMs) }}</div>
            </td>
            <td class="usage-cell" :title="row.hints.usage">
              <strong>{{ fmtCompact(row.totalTokens) }}</strong>
              <div class="muted small-text usage-breakdown">{{ row.usageText }}</div>
            </td>
            <td :title="row.hints.cost"><strong>{{ fmtCacheHitRate(row.cacheHitRate) }}</strong></td>
            <td class="event-cost-cell" :title="row.costTooltip">
              <strong>{{ row.costText }}</strong>
              <span class="cost-estimate-meta">{{ row.costMeta }}</span>
            </td>
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
        <div v-if="modelRouteTooltip.visible" id="event-model-tooltip" role="tooltip" class="event-model-tooltip" :style="modelRouteTooltip.style"
             @mouseenter="keepModelRouteTooltip" @mouseleave="hideModelRouteTooltip">
          <div class="event-model-tooltip-row">
            <span>{{ t('monitoring.eventMeta.requestedModel') }}</span>
            <strong class="event-model-tip-chip is-requested">{{ modelRouteTooltip.row?.model }}</strong>
          </div>
          <div class="event-model-tooltip-row">
            <span>{{ t('monitoring.eventMeta.billedModel') }}</span>
            <strong class="event-model-tip-chip is-actual">{{ modelRouteTooltip.row?.mappedModel }}</strong>
          </div>
          <div v-if="modelRouteTooltip.row?.showResponseModel" class="event-model-tooltip-row">
            <span>{{ t('monitoring.eventMeta.responseModel') }}</span>
            <strong class="event-model-tip-chip is-response" :title="modelRouteTooltip.row.responseModel">{{ modelRouteTooltip.row.responseModel }}</strong>
          </div>
          <div v-if="modelRouteTooltip.row?.showResponseModel && modelRouteTooltip.row.responseModelSource" class="muted small-text">
            {{ t('monitoring.eventMeta.responseModelSource') }}: {{ t('monitoring.eventMeta.responseModelSources.' + modelRouteTooltip.row.responseModelSource) }}
          </div>
          <div v-if="modelRouteTooltip.row?.responseModelConflict" class="event-model-conflict">
            <div>{{ t('monitoring.eventMeta.responseModelConflict') }}</div>
            <div v-if="modelRouteTooltip.row.observedResponseModel" class="event-model-tooltip-row">
              <span>{{ t('monitoring.eventMeta.observedResponseModel') }}</span>
              <strong class="event-model-tip-chip" :title="modelRouteTooltip.row.observedResponseModel">{{ modelRouteTooltip.row.observedResponseModel }}</strong>
            </div>
          </div>
        </div>
      </Teleport>
    </DataCard>

    <DataCard v-if="activeDataTab === 'accounts'">
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
            <td>
              <span v-if="accountProviderChip(row).tag" class="provider-cell">
                <span :class="['provider-chip', accountProviderChip(row).chip]">{{ accountProviderChip(row).tag }}</span>
                <span v-if="accountProviderChip(row).showName" class="provider-chip-name">{{ accountProviderChip(row).name }}</span>
              </span>
              <span v-else>{{ row.auth_provider_snapshot || EMPTY_VALUE }}</span>
            </td>
            <td>{{ fmtInt(row.calls) }}</td>
            <td><strong :class="successRateClass(row.success_rate)">{{ fmtPct(row.success_rate) }}</strong></td>
            <td>{{ fmtCompact(row.total_tokens) }}</td>
            <td :title="aggregateCostCoverage(row)">
              <strong>{{ aggregateCostText(row) }}</strong>
              <small v-if="Number(row.unpriced_calls || 0) > 0" class="cost-aggregate-meta">{{ t('monitoring.costEstimate.unpricedShort', {count: fmtInt(row.unpriced_calls)}) }}</small>
            </td>
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
      <article v-if="isOAuthAuthType(selectedAccount.auth_type)" class="auth-card">
        <div class="auth-card-head">
          <div :class="['auth-card-logo', accountProviderChip(selectedAccount).chip || 'is-oauth']">{{ accountProviderChip(selectedAccount).tag.slice(0, 1) || '•' }}</div>
          <div class="auth-card-title">
            <div class="auth-card-badges">
              <span v-if="accountProviderChip(selectedAccount).tag" :class="['provider-chip', accountProviderChip(selectedAccount).chip]">{{ accountProviderChip(selectedAccount).tag }}</span>
              <span class="auth-type-badge">{{ formatAuthType(accountQuota.authMetadata?.authType || selectedAccount.auth_type) }}</span>
            </div>
            <h3>{{ accountQuota.authMetadata?.email || accountSource(selectedAccount) }}</h3>
            <div class="auth-card-file">{{ accountQuota.fileName || accountQuota.authMetadata?.name || EMPTY_VALUE }}</div>
          </div>
          <span :class="['status-chip', accountQuota.disabled ? 'off' : '']">
            <i></i>{{ accountQuota.disabled ? t('monitoring.authCard.disabled') : t('monitoring.authCard.enabled') }}
          </span>
        </div>
        <div v-if="accountQuota.authMetadata?.statusMessage" class="auth-status-message">
          {{ accountQuota.authMetadata.statusMessage }}
        </div>
        <div class="auth-health">
          <div class="auth-health-row">
            <span>{{ t('monitoring.authCard.health') }}</span>
            <span class="health-counts">
              <span class="good-text">{{ t('monitoring.authCard.successCount', { count: fmtInt(selectedAccount.success_calls) }) }}</span>
              <span class="bad-text">{{ t('monitoring.authCard.failureCount', { count: fmtInt(selectedAccount.failure_calls) }) }}</span>
            </span>
          </div>
          <div class="spark" :aria-label="t('monitoring.authCard.health')">
            <i v-for="(ok, idx) in accountHealthBlocks(selectedAccount)" :key="idx" :class="ok == null ? 'idle' : ok ? 'ok' : 'bad'"></i>
            <span :class="['spark-rate', successRateClass(selectedAccount.success_rate)]">{{ fmtPct(selectedAccount.success_rate) }}</span>
          </div>
        </div>
        <div class="auth-meta">
          {{ fmtCompact(selectedAccount.total_tokens) }} tok · {{ aggregateCostText(selectedAccount) }} · {{ fmtDuration(selectedAccount.average_latency_ms) }} · {{ formatDateTime(selectedAccount.last_seen_ms) }}
        </div>
        <div class="auth-file-meta">
          <span v-if="accountQuota.authMetadata?.authIndex" class="auth-file-meta-item"><b>{{ t('monitoring.authCard.authIndex') }}</b> {{ accountQuota.authMetadata.authIndex }}</span>
          <span v-if="accountQuota.authMetadata?.projectId" class="auth-file-meta-item"><b>{{ t('monitoring.authCard.projectId') }}</b> {{ accountQuota.authMetadata.projectId }}</span>
          <span v-if="accountQuota.authMetadata?.size" class="auth-file-meta-item"><b>{{ t('monitoring.authCard.fileSize') }}</b> {{ formatBytes(accountQuota.authMetadata.size) }}</span>
          <span v-if="accountQuota.authMetadata?.modTime" class="auth-file-meta-item"><b>{{ t('monitoring.authCard.modified') }}</b> {{ formatMetadataDate(accountQuota.authMetadata.modTime) }}</span>
          <span v-if="accountQuota.authMetadata?.priority != null" class="auth-file-meta-item is-priority"><b>{{ t('monitoring.authCard.priority') }}</b> {{ accountQuota.authMetadata.priority }}</span>
          <span v-if="accountQuota.authMetadata?.weight != null" class="auth-file-meta-item is-weight"><b>{{ t('monitoring.authCard.weight') }}</b> {{ accountQuota.authMetadata.weight }}</span>
        </div>
        <div v-if="accountQuota.authMetadata?.note" class="auth-note">
          <span>{{ t('monitoring.authCard.note') }}</span>{{ accountQuota.authMetadata.note }}
        </div>
        <div v-if="hasQuotaSummary" class="auth-plan">
          <div class="auth-summary-grid">
            <div v-if="accountQuota.planType" class="auth-summary-item">
              <span>{{ t('monitoring.authCard.plan') }}</span><b>{{ formatPlanType(accountQuota.planType) }}</b>
            </div>
            <div v-if="accountQuota.subscriptionActiveUntil" class="auth-summary-item">
              <span>{{ t('monitoring.authCard.renewal') }}</span><b>{{ formatMetadataDate(accountQuota.subscriptionActiveUntil) }}</b>
            </div>
            <div v-if="accountQuota.resetCreditsAvailableCount != null" class="auth-summary-item">
              <span>{{ t('monitoring.authCard.resetCredits') }}</span><b>{{ accountQuota.resetCreditsAvailableCount }}</b>
            </div>
            <div v-if="accountQuota.extraUsage?.is_enabled" class="auth-summary-item">
              <span>{{ t('monitoring.authCard.extraUsage') }}</span><b>{{ formatExtraUsage(accountQuota.extraUsage) }}</b>
            </div>
          </div>
          <div v-if="accountQuota.resetCreditsError" class="auth-quota-hint">{{ accountQuota.resetCreditsError }}</div>
          <div v-if="accountQuota.resetCredits.length" class="reset-credit-list">
            <span v-for="(credit, index) in accountQuota.resetCredits" :key="credit.id || index">
              {{ t('monitoring.authCard.resetCreditItem', { index: index + 1, date: formatMetadataDate(credit.expiresAt) }) }}
            </span>
          </div>
          <template v-if="accountQuota.groups.length">
            <div v-for="group in accountQuota.groups" :key="group.id" class="quota-group">
              <div class="quota-group-title">{{ group.label }}<span v-if="group.description">{{ group.description }}</span></div>
              <div v-for="bucket in group.buckets" :key="bucket.id" class="quota-row quota-row-group">
                <div class="quota-row-header">
                  <span>{{ bucket.label }}</span>
                  <div class="quota-meta"><b>{{ Math.round(bucket.remainingPercent) }}%</b><span v-if="bucket.resetAt" class="quota-reset">{{ formatQuotaReset(bucket.resetAt) }}<span v-if="formatQuotaResetRelative(bucket.resetAt)" class="quota-reset-relative"> · {{ formatQuotaResetRelative(bucket.resetAt) }}</span></span></div>
                </div>
                <div class="quota-bar" :class="quotaTone(bucket.remainingPercent)"><span :style="{ width: `${bucket.remainingPercent}%` }"></span></div>
              </div>
            </div>
          </template>
          <template v-else-if="accountQuota.windows.length">
            <div v-for="window in accountQuota.windows" :key="window.id" class="quota-row">
              <div class="quota-row-header">
                <span>{{ quotaWindowLabel(window) }}</span>
                <div class="quota-meta">
                  <b v-if="window.hasRemainingPercent">{{ Math.round(window.remainingPercent) }}%</b>
                  <span v-if="window.usedLabel" class="muted">{{ window.usedLabel }}</span>
                  <span v-if="window.amountLabel" class="quota-amount">{{ window.amountLabel }}</span>
                  <span v-if="window.resetText" class="quota-reset">{{ formatQuotaReset(window.resetText) }}<span v-if="formatQuotaResetRelative(window.resetText)" class="quota-reset-relative"> · {{ formatQuotaResetRelative(window.resetText) }}</span></span>
                </div>
              </div>
              <div v-if="window.hasRemainingPercent" class="quota-bar" :class="quotaTone(window.remainingPercent)"><span :style="{ width: `${window.remainingPercent}%` }"></span></div>
              <div v-if="window.remainingText && !window.hasRemainingPercent" class="muted quota-text">{{ window.remainingText }}</div>
            </div>
          </template>
          <p v-else>{{ accountQuota.message || t('monitoring.authCard.noQuota') }}</p>
        </div>
        <div v-else class="auth-plan auth-plan-empty">
          <div class="auth-plan-kicker">{{ t('monitoring.authCard.quota') }}</div>
          <p>{{ accountQuota.message || t('monitoring.authCard.noQuota') }}</p>
        </div>
        <div class="auth-actions">
          <button class="btn primary" type="button" :disabled="quotaLoading" @click="queryAccountQuota(selectedAccount, {force: true})">
            {{ quotaLoading ? t('monitoring.authCard.querying') : t('monitoring.authCard.queryQuota') }}
          </button>
          <button class="btn" type="button" @click="filterAccountAPIKey(selectedAccount)">{{ t('monitoring.authCard.filterEvents') }}</button>
          <button class="btn" type="button" @click="emit('open-inspection')">{{ t('monitoring.authCard.openInspection') }}</button>
        </div>
      </article>
      <DataCard v-else :title="t('monitoring.cards.sourceDetail')" :subtitle="accountDetailSubtitle(selectedAccount)">
        <DetailGrid :items="buildAccountDetail(selectedAccount)"/>
      </DataCard>
    </div>

    <DataCard v-if="activeDataTab === 'models'">
      <SimpleTable
        :rows="modelRows"
        :columns="modelColumns"
        selectable
        :selected-id="selectedModelId"
        @select="selectModel"
        @filter="applyModelFilter"
      />
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
import { isOAuthAuthType, providerChip } from '../utils/providerTag.js';
import { EMPTY_VALUE, formatDate, formatDateTime, formatInt, formatTime } from '../utils/localeFormat.js';
import { computeCacheHitRate, formatCacheHitRate } from '../utils/cacheHitRate.js';
import { requestProtocol, requestProtocolLabel } from '../utils/requestProtocol.js';
import { buildUsageIOC } from '../utils/usageBreakdown.js';
import { aggregateCostCoverage as formatAggregateCostCoverage, aggregateCostText as formatAggregateCostText, eventCostAmount as formatEventCostAmount, eventCostMeta as formatEventCostMeta, eventCostTooltip as formatEventCostTooltip } from '../utils/costEstimateDisplay.js';
import { canApplySelectedFilter, rowIdentity } from '../utils/rowFilter.js';
import { buildEventHints, buildModelMeta, formatCacheSub, formatCallsSub, formatTpsSub, hasModelRouteDetails, hasResponseModelConflict, hasResponseModelDifference, hasResponseModelMismatch, mappedModelName, requestedModelName, responseModelName, responseModelSource } from '../utils/eventStreamDisplay.js';
import {
  formatQuotaResetRelative as formatQuotaResetRelativeValue,
  formatQuotaStatusMessage,
  normalizeQuotaWindows,
} from '../utils/quotaDisplay.js';
import {
  QUOTA_ERROR_COOLDOWN_MS,
  QUOTA_PROBE_COOLDOWN_MS,
  getOrCreateQuotaRequest,
  getQuotaCacheEntry,
  quotaCacheKey,
  setQuotaCacheEntry,
} from '../utils/quotaCache.js';

const props = defineProps({
  ready: {type: Boolean, default: false},
  proxyCall: {type: Function, required: true},
});
const emit = defineEmits(['open-inspection']);
const API_KEY_AUTO_COLLAPSE_MS = 5000;

const {t, locale} = useI18n();

const data = ref(null);
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
const selectedQuotaKey = ref('');
const quotaLoading = ref(false);
const accountQuota = ref(emptyAccountQuota());
const selectedModelId = ref('');
const expandedEventKeys = ref(new Set());
const expandedAccountSources = ref(new Set());
const eventKeyCollapseTimers = new Map();
const accountSourceCollapseTimers = new Map();
const filters = ref(defaultFilters());
const failureTooltip = ref({visible: false, row: null, style: {}});
const modelRouteTooltip = ref({visible: false, row: null, style: {}});
const quotaNowMs = ref(Date.now());
let failureHideTimer = null;
let modelRouteHideTimer = null;
let timer = null;
let quotaClockTimer = null;

const dataTabs = computed(() => [
  {key: 'events', label: t('monitoring.tabs.events'), count: eventRows.value.length, note: ''},
  {key: 'accounts', label: t('monitoring.tabs.accounts'), count: accountApiKeyRows.value.length, note: t('monitoring.cards.accountsSubtitle')},
  {key: 'models', label: t('monitoring.tabs.models'), count: modelRows.value.length, note: t('monitoring.cards.modelsSubtitle')},
]);
const activeMonitorNote = computed(() => dataTabs.value.find((tab) => tab.key === activeDataTab.value)?.note || '');

const summary = computed(() => data.value?.summary || {});
const eventRows = computed(() => (data.value?.events?.items || []).map((row, idx) => ({...row, __id: idx})));
const summaryCards = computed(() => {
  const s = summary.value;
  const totalCacheTokens = Number(s.cached_tokens ?? 0) + Number(s.cache_read_tokens ?? 0) + Number(s.cache_creation_tokens ?? 0);
  const cacheHitRate = computeCacheHitRate(s);
  const tokenMix = (n) => s.total_tokens > 0 ? `${fmtPct(n / s.total_tokens)}` : EMPTY_VALUE;
  return [
    {label: t('monitoring.kpi.totalCalls'), value: fmtInt(s.total_calls), sub: t('monitoring.kpi.accountsSub', {count: accountCount.value})},
    {label: t('monitoring.kpi.successRate'), value: fmtPct(s.success_rate), sub: fmtDuration(s.average_latency_ms)},
    {label: t('monitoring.kpi.failureTotal'), value: fmtInt(s.failure_calls), sub: t('monitoring.kpi.monitorGroupsSub', {count: failedGroupCount.value})},
    {
      label: t('monitoring.kpi.estimatedCost'),
      value: Number(s.priced_calls || 0) > 0 ? fmtMoney(s.total_cost) : EMPTY_VALUE,
      sub: t('monitoring.costEstimate.coverage', {priced: fmtInt(s.priced_calls), unpriced: fmtInt(s.unpriced_calls)})
    },
    {label: t('monitoring.kpi.totalTokens'), value: fmtCompact(s.total_tokens), sub: t('monitoring.kpi.reasoningSub', {value: fmtCompact(s.reasoning_tokens)})},
    {label: t('monitoring.kpi.inputTokens'), value: fmtCompact(s.input_tokens), sub: t('monitoring.kpi.shareSub', {value: tokenMix(Number(s.input_tokens ?? 0))})},
    {label: t('monitoring.kpi.outputTokens'), value: fmtCompact(s.output_tokens), sub: t('monitoring.kpi.shareSub', {value: tokenMix(Number(s.output_tokens ?? 0))})},
    {label: t('monitoring.kpi.cacheTokens'), value: fmtCompact(totalCacheTokens), sub: t('monitoring.kpi.hitRateSub', {value: fmtCacheHitRate(cacheHitRate)})},
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
  ['total_tokens', t('monitoring.modelColumns.token'), 'usage'],
  ['cost', t('monitoring.modelColumns.cost'), 'money'],
  ['actions', t('monitoring.accountColumns.actions'), 'filter'],
]);

const eventDetailCards = computed(() => selectedEvent.value ? [
  {label: t('monitoring.labels.status'), value: selectedEvent.value.failed ? t('monitoring.labels.failed') : t('monitoring.labels.success')},
  {label: t('monitoring.labels.token'), value: selectedEvent.value.total_tokens ?? 0},
  {label: t('monitoring.labels.latency'), value: fmtMs(selectedEvent.value.latency_ms)},
  {label: t('monitoring.labels.cost'), value: fmtMoney(eventCostAmount(selectedEvent.value)), sub: eventCostMeta(selectedEvent.value)},
] : []);
const eventBaseDetail = computed(() => selectedEvent.value ? decodeDetailObject(pickObject(selectedEvent.value, ['request_id', 'event_hash', 'timestamp_ms', 'model', 'alias', 'requested_model', 'resolved_model', 'response_model', 'host_response_model', 'observed_response_model', 'response_model_source', 'response_model_conflict', 'endpoint', 'method', 'path', 'protocol', 'executor_type', 'auth_index', 'source', 'source_hash', 'api_key_hash', 'account_snapshot', 'auth_label_snapshot', 'auth_provider_snapshot', 'auth_project_id_snapshot', 'input_tokens', 'output_tokens', 'cached_tokens', 'cache_read_tokens', 'cache_creation_tokens', 'cache_input_mode', 'cache_hit_tokens', 'cache_hit_input_tokens', 'cache_hit_rate', 'reasoning_effort', 'service_tier', 'response_service_tier', 'cost_estimate', 'reasoning_tokens', 'total_tokens', 'latency_ms', 'ttft_ms', 'failed', 'fail_status_code', 'fail_summary'])) : {});
const eventHeaderDetail = computed(() => selectedEvent.value ? decodeDetailObject(pickObject(selectedEvent.value, ['header_quota_recover_at_ms', 'header_quota_used_percent', 'header_quota_plan_type', 'header_error_kind', 'header_error_code', 'header_trace_id'])) : {});

watch(timeRange, () => {
  eventPage.value = 1;
  refresh(true);
});
watch(filters, () => {
  eventPage.value = 1;
  refresh(true);
}, {deep: true});
// Auto-refresh replaces normalized rows; keep open popups bound to live metadata.
// Do not cancel a pending pointer-leave timer just because new data arrived.
watch(pagedEvents, (rows) => {
  if (modelRouteTooltip.value.visible) {
    const row = rows.find(row => row.id === modelRouteTooltip.value.row?.id);
    if (row?.hasModelDetails) modelRouteTooltip.value.row = row;
    else hideModelRouteTooltip(true);
  }
  if (failureTooltip.value.visible) {
    const row = rows.find(row => row.id === failureTooltip.value.row?.id);
    if (row?.failed) failureTooltip.value.row = row;
    else {
      keepFailureTooltip();
      failureTooltip.value.visible = false;
    }
  }
});
watch([activeDataTab, eventPage, eventPageSize], () => {
  hideModelRouteTooltip(true);
  keepFailureTooltip();
  failureTooltip.value.visible = false;
});
watch(autoRefreshMs, setupTimer);
watch(() => props.ready, (ready) => {
  if (!ready) {
    clearTimer();
    return;
  }
  setupTimer();
  if (!data.value) refresh(true);
});
onMounted(() => {
  quotaClockTimer = window.setInterval(() => {
    quotaNowMs.value = Date.now();
  }, 30000);
  if (props.ready) {
    setupTimer();
    refresh(true);
  }
});
onBeforeUnmount(() => {
  clearTimer();
  clearKeyCollapseTimers();
  if (quotaClockTimer) window.clearInterval(quotaClockTimer);
  quotaClockTimer = null;
  if (failureHideTimer) clearTimeout(failureHideTimer);
  if (modelRouteHideTimer) clearTimeout(modelRouteHideTimer);
});

async function refresh(force = false) {
  if (!props.ready) return;
  if (loading.value && !force) return;
  loading.value = true;
  error.value = '';
  try {
    const analyticsData = await props.proxyCall({method: 'POST', path: '/v0/management/monitoring/analytics', body: buildAnalyticsRequest()});
    if (analyticsData && analyticsData.error) {
      error.value = String(analyticsData.error);
    }
    data.value = analyticsData;
  } catch (e) {
    error.value = e.message || String(e);
  } finally {
    loading.value = false;
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
      events_page: {limit: 3000},
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
    clearKeyCollapseTimer(eventKeyCollapseTimers, key);
  } else {
    next.add(key);
    scheduleKeyCollapse(expandedEventKeys, eventKeyCollapseTimers, key);
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
  if (next.has(key)) {
    next.delete(key);
    clearKeyCollapseTimer(accountSourceCollapseTimers, key);
  } else {
    next.add(key);
    scheduleKeyCollapse(expandedAccountSources, accountSourceCollapseTimers, key);
  }
  expandedAccountSources.value = next;
}

function clearKeyCollapseTimer(timers, key) {
  const timer = timers.get(key);
  if (timer) window.clearTimeout(timer);
  timers.delete(key);
}

function scheduleKeyCollapse(expandedKeys, timers, key) {
  clearKeyCollapseTimer(timers, key);
  timers.set(key, window.setTimeout(() => {
    const next = new Set(expandedKeys.value);
    next.delete(key);
    expandedKeys.value = next;
    timers.delete(key);
  }, API_KEY_AUTO_COLLAPSE_MS));
}

function clearKeyCollapseTimers() {
  for (const [key] of eventKeyCollapseTimers) clearKeyCollapseTimer(eventKeyCollapseTimers, key);
  for (const [key] of accountSourceCollapseTimers) clearKeyCollapseTimer(accountSourceCollapseTimers, key);
}

function selectAccountAPIKey(row) {
  selectedAccountId.value = row?.id || '';
  selectedQuotaKey.value = row ? quotaCacheKey(row) : '';
  quotaLoading.value = false;
  accountQuota.value = emptyAccountQuota();
  if (!row || !isOAuthAuthType(row.auth_type)) return;

  const key = selectedQuotaKey.value;
  const cached = getQuotaCacheEntry(key);
  if (cached) {
    applyCachedQuotaResult(key, cached);
    return;
  }
  queryAccountQuota(row);
}

function accountProviderChip(row) {
  return providerChip(row?.auth_provider_snapshot || row?.provider, row?.auth_type);
}

function accountRecentPattern(row) {
  return eventRows.value
    .filter(event => sameAccountIdentity(event, row))
    .slice(0, 20)
    .map(event => !event.failed)
    .reverse();
}

function accountHealthBlocks(row) {
  const pattern = accountRecentPattern(row);
  const idleCount = Math.max(0, 20 - pattern.length);
  return Array.from({length: 20}, (_, index) => index < idleCount ? null : pattern[index - idleCount]);
}

function sameAccountIdentity(event, row) {
  const eventProvider = String(event?.auth_provider_snapshot || event?.provider || '').trim().toLowerCase();
  const rowProvider = String(row?.auth_provider_snapshot || row?.provider || '').trim().toLowerCase();
  if (eventProvider && rowProvider && eventProvider !== rowProvider) return false;
  const eventType = String(event?.auth_type || '').trim().toLowerCase();
  const rowType = String(row?.auth_type || '').trim().toLowerCase();
  if (eventType && rowType && eventType !== rowType) return false;
  if (row?.auth_index) return String(event?.auth_index || '') === String(row.auth_index);
  if (row?.auth_id) return String(event?.auth_file_snapshot || event?.auth_id || '') === String(row.auth_id);
  return String(event?.source || '').trim() === accountSource(row);
}

function matchInspectionResult(results, row) {
  const source = accountSource(row).toLowerCase();
  const provider = String(row?.auth_provider_snapshot || '').trim().toLowerCase();
  const authType = String(row?.auth_type || '').trim().toLowerCase();
  const authIndex = String(row?.auth_index || '').trim();
  const authId = String(row?.auth_id || '').trim();
  const sameIdentity = (item) => {
    if (provider && String(item.provider || '').trim().toLowerCase() !== provider) return false;
    if (authType && item.authType && String(item.authType).trim().toLowerCase() !== authType) return false;
    return true;
  };
  const items = (results || []).filter(sameIdentity);
  return items.find(item => authIndex && String(item.authIndex || '').trim() === authIndex)
    || items.find(item => authId && String(item.authId || '').trim() === authId)
    || items.find(item => String(item.displayAccount || '').trim().toLowerCase() === source)
    || items.find(item => String(item.fileName || '').trim().toLowerCase().includes(source))
    || null;
}

function emptyAccountQuota() {
  return {
    planType: '', fileName: '', disabled: false, authMetadata: null,
    subscriptionActiveUntil: null, resetCreditsAvailableCount: null,
    resetCreditsApplicableAvailableCount: null, resetCredits: [], resetCreditsError: '',
    extraUsage: null, groups: [], windows: [], quotaMode: '', message: '',
  };
}

const hasQuotaSummary = computed(() => {
  const quota = accountQuota.value;
  return Boolean(
    quota.planType || quota.subscriptionActiveUntil || quota.resetCreditsAvailableCount != null ||
    quota.extraUsage || quota.groups.length || quota.windows.length
  );
});

function quotaResultHasData(result) {
  const metadata = result?.quotaMetadata || {};
  const windows = (Array.isArray(result?.quotaWindows) && result.quotaWindows.length > 0)
    || (Array.isArray(metadata.windows) && metadata.windows.length > 0);
  const groups = Array.isArray(metadata.groups) && metadata.groups.some(group => Array.isArray(group?.buckets) && group.buckets.length > 0);
  const extraUsage = (result?.extraUsage && typeof result.extraUsage === 'object')
    || (metadata.extraUsage && typeof metadata.extraUsage === 'object');
  return Boolean(
    result?.planType || metadata.planType || result?.subscriptionActiveUntil || metadata.subscriptionActiveUntil ||
    result?.resetCreditsAvailableCount != null || metadata.resetCreditsAvailableCount != null ||
    (Array.isArray(result?.resetCredits) && result.resetCredits.length > 0) ||
    (Array.isArray(metadata.resetCredits) && metadata.resetCredits.length > 0) ||
    extraUsage || windows || groups
  );
}

function remainingTextFromWindow(window, remaining) {
  if (typeof window.remainingText === 'string' && window.remainingText.trim()) return window.remainingText;
  if (Number.isFinite(remaining)) return t('monitoring.authCard.remaining', { value: remaining });
  return '';
}

function numberOrNullValue(value) {
  const number = Number(value);
  return Number.isFinite(number) ? number : null;
}

function clampPercent(value) {
  const number = numberOrNullValue(value);
  return number == null ? null : Math.max(0, Math.min(100, number));
}

function quotaGroupsFromResult(result) {
  const meta = result?.quotaMetadata || {};
  const groups = Array.isArray(meta.groups) ? meta.groups : [];
  return groups.map((group, groupIndex) => ({
    id: group?.id || `quota-group-${groupIndex}`,
    label: group?.label || t('monitoring.authCard.quota'),
    description: group?.description || '',
    buckets: (Array.isArray(group?.buckets) ? group.buckets : []).map((bucket, bucketIndex) => {
      const fraction = numberOrNullValue(bucket?.remainingFraction);
      const remainingPercent = clampPercent(bucket?.remainingPercent ?? (fraction == null ? null : fraction * 100)) ?? 0;
      return {
        id: bucket?.id || `${groupIndex}-${bucketIndex}`,
        label: bucket?.label || bucket?.id || t('monitoring.authCard.quota'),
        remainingPercent,
        resetAt: bucket?.resetAt || '',
      };
    }),
  })).filter(group => group.buckets.length);
}

function formatQuotaAmount(value) {
  const number = numberOrNullValue(value);
  if (number == null) return '';
  return new Intl.NumberFormat(undefined, {style: 'currency', currency: 'USD'}).format(number / 100);
}

function quotaWindowsFromResult(result) {
  const rawWindows = Array.isArray(result?.quotaWindows)
    ? result.quotaWindows
    : (Array.isArray(result?.quotaMetadata?.windows) ? result.quotaMetadata.windows : []);
  return normalizeQuotaWindows({...(result || {}), quotaWindows: rawWindows}).map((window, index) => {
    const raw = rawWindows[index] || {};
    const remainingPercent = clampPercent(raw.remainingPercent ?? (window.hasUsedPercent ? 100 - window.usedPercent : null));
    const used = numberOrNullValue(raw.used);
    const limit = numberOrNullValue(raw.limit);
    const remaining = numberOrNullValue(raw.remaining);
    const amountWindow = ['monthly', 'on-demand'].includes(String(raw.kind || '').toLowerCase());
    const amountLabel = amountWindow && limit != null
      ? `${formatQuotaAmount(remaining ?? Math.max(0, limit - (used || 0)))} / ${formatQuotaAmount(limit)}`
      : '';
    const usedLabel = raw.kind === 'product' && window.hasUsedPercent
      ? t('monitoring.authCard.usedPercent', {value: Math.round(window.usedPercent)})
      : (used != null && limit != null && !amountWindow ? `${fmtCompact(used)} / ${fmtCompact(limit)}` : '');
    return {
      ...window,
      kind: raw.kind || '',
      used,
      limit,
      hasRemainingPercent: Number.isFinite(remainingPercent),
      remainingPercent: Number.isFinite(remainingPercent) ? remainingPercent : 0,
      amountLabel,
      usedLabel,
      description: raw.description || '',
      remainingText: remainingTextFromWindow(window, window.remaining),
    };
  });
}

function formatAuthType(value) {
  const type = String(value || '').trim().toLowerCase();
  if (type === 'oauth' || type === 'oauth2') return 'OAuth';
  if (type === 'apikey' || type === 'api_key' || type === 'api-key' || type === 'api') return 'API Key';
  return value || EMPTY_VALUE;
}

function formatBytes(value) {
  const bytes = numberOrNullValue(value);
  if (bytes == null) return EMPTY_VALUE;
  if (bytes < 1024) return `${Math.round(bytes)} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function formatMetadataDate(value) {
  if (value == null || value === '') return EMPTY_VALUE;
  let timestamp = numberOrNullValue(value);
  if (timestamp != null) {
    if (timestamp > 0 && timestamp < 1e11) timestamp *= 1000;
  } else {
    timestamp = Date.parse(String(value));
  }
  if (!Number.isFinite(timestamp)) return String(value);
  return new Date(timestamp).toLocaleString(undefined, {hour12: false});
}

function formatQuotaReset(value) {
  if (!value) return '';
  const formatted = formatMetadataDate(value);
  return formatted === EMPTY_VALUE ? String(value) : formatted;
}

function formatQuotaResetRelative(value) {
  return formatQuotaResetRelativeValue(value, quotaNowMs.value, locale.value);
}

function formatPlanType(value) {
  const plan = String(value || '').trim();
  const labels = {
    plan_max: 'Max', plan_pro: 'Pro', plan_team: 'Team', plan_free: 'Free',
    plan_plus: 'Plus', supergrok: 'SuperGrok', 'supergrok heavy': 'SuperGrok Heavy',
    paid: 'Paid', ultra: 'Ultra', 'ultra-lite': 'Ultra Lite', pro: 'Pro', free: 'Free',
  };
  return labels[plan.toLowerCase()] || plan || EMPTY_VALUE;
}

function formatExtraUsage(extra) {
  const used = numberOrNullValue(extra?.used_credits ?? extra?.usedCredits);
  const limit = numberOrNullValue(extra?.monthly_limit ?? extra?.monthlyLimit);
  if (used == null && limit == null) return EMPTY_VALUE;
  const format = value => value == null ? EMPTY_VALUE : new Intl.NumberFormat(undefined, {style: 'currency', currency: 'USD'}).format(value / 100);
  return `${format(used)} / ${format(limit)}`;
}

function quotaTone(remainingPercent) {
  const remaining = numberOrNullValue(remainingPercent);
  if (remaining != null && remaining <= 20) return 'bad';
  if (remaining != null && remaining <= 50) return 'warn';
  return '';
}

function quotaWindowLabel(window) {
  return window?.label || window?.kind || t('monitoring.authCard.quota');
}

function isSelectedQuotaKey(key) {
  return Boolean(key && selectedQuotaKey.value === key);
}

function applyQuotaResultForDisplay(result) {
  applyQuotaResult(result || {});
  if (!hasQuotaSummary.value && !accountQuota.value.message) {
    accountQuota.value.message = t('monitoring.authCard.noQuota');
  }
}

function applyCachedQuotaResult(key, entry) {
  if (!isSelectedQuotaKey(key)) return;
  applyQuotaResultForDisplay(entry?.result || {});
}

function applyQuotaResult(result) {
  const quota = result?.quotaMetadata || {};
  const rawAuthMetadata = result?.authMetadata || {
    id: result?.authId || selectedAccount.value?.auth_id || '',
    authIndex: result?.authIndex || selectedAccount.value?.auth_index || '',
    name: result?.fileName || '',
    email: result?.displayAccount || accountSource(selectedAccount.value),
    provider: result?.provider || selectedAccount.value?.auth_provider_snapshot || '',
    authType: result?.authType || selectedAccount.value?.auth_type || '',
    disabled: Boolean(result?.disabled),
  };
  const authMetadata = {
    ...rawAuthMetadata,
    statusMessage: formatQuotaStatusMessage(rawAuthMetadata?.statusMessage),
  };
  accountQuota.value = {
    ...emptyAccountQuota(),
    planType: result?.planType || quota.planType || '',
    fileName: result?.fileName || authMetadata?.name || '',
    disabled: Boolean(result?.disabled || authMetadata?.disabled),
    authMetadata,
    subscriptionActiveUntil: result?.subscriptionActiveUntil || quota.subscriptionActiveUntil || null,
    resetCreditsAvailableCount: result?.resetCreditsAvailableCount ?? quota.resetCreditsAvailableCount ?? null,
    resetCreditsApplicableAvailableCount: result?.resetCreditsApplicableAvailableCount ?? quota.resetCreditsApplicableAvailableCount ?? null,
    resetCredits: Array.isArray(result?.resetCredits) ? result.resetCredits : (Array.isArray(quota.resetCredits) ? quota.resetCredits : []),
    resetCreditsError: result?.resetCreditsError || quota.resetCreditsError || '',
    extraUsage: result?.extraUsage || quota.extraUsage || null,
    groups: quotaGroupsFromResult(result || {}),
    windows: quotaWindowsFromResult(result || {}),
    quotaMode: quota.mode || '',
    message: formatQuotaStatusMessage(quota.quotaError || result?.errorDetail || result?.actionReason || result?.error || ''),
  };
}

async function loadLatestInspectionResult(row) {
  const runsResp = await props.proxyCall({ method: 'GET', path: '/v0/management/codex-inspection/runs', query: 'limit=8' });
  const runs = Array.isArray(runsResp?.items) ? runsResp.items : [];
  const completed = runs.find(run => run.status === 'completed');
  if (!completed?.id) return null;
  const detail = await props.proxyCall({ method: 'GET', path: `/v0/management/codex-inspection/runs/${completed.id}` });
  return matchInspectionResult(detail?.results || [], row);
}

async function fetchQuotaResult(row) {
  let probed = null;
  try {
    probed = await props.proxyCall({
      method: 'POST',
      path: '/v0/management/account-quota-probe',
      body: {
        authId: row.auth_id || '',
        authIndex: row.auth_index || '',
        authType: row.auth_type || '',
        provider: row.auth_provider_snapshot || '',
        source: accountSource(row),
        fileName: row.file_name || '',
      },
    });

    if (probed && !probed.error && quotaResultHasData(probed)) {
      return {result: probed, cooldownMs: QUOTA_PROBE_COOLDOWN_MS};
    }

    const stored = await loadLatestInspectionResult(row);
    if (stored && quotaResultHasData(stored)) {
      return {result: stored, cooldownMs: QUOTA_PROBE_COOLDOWN_MS};
    }

    if (probed && !probed.error) {
      return {result: probed, cooldownMs: QUOTA_PROBE_COOLDOWN_MS};
    }

    return {
      result: stored || {actionReason: probed?.error || t('monitoring.authCard.noQuota')},
      cooldownMs: QUOTA_PROBE_COOLDOWN_MS,
    };
  } catch (error) {
    return {
      result: {actionReason: error.message || String(error)},
      cooldownMs: QUOTA_ERROR_COOLDOWN_MS,
    };
  }
}

async function queryAccountQuota(row, {force = false} = {}) {
  if (!row || !props.proxyCall) return;
  const key = quotaCacheKey(row);
  const cached = getQuotaCacheEntry(key);
  if (!force && cached) {
    applyCachedQuotaResult(key, cached);
    return cached.result;
  }

  if (isSelectedQuotaKey(key)) quotaLoading.value = true;
  try {
    const response = await getOrCreateQuotaRequest(key, () => fetchQuotaResult(row));
    setQuotaCacheEntry(key, response.result, response.cooldownMs);
    if (isSelectedQuotaKey(key)) applyQuotaResultForDisplay(response.result);
    return response.result;
  } catch (error) {
    const result = {actionReason: error.message || String(error)};
    setQuotaCacheEntry(key, result, QUOTA_ERROR_COOLDOWN_MS);
    if (isSelectedQuotaKey(key)) applyQuotaResultForDisplay(result);
    return result;
  } finally {
    if (isSelectedQuotaKey(key)) quotaLoading.value = false;
  }
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

function selectModel(row) {
  selectedModelId.value = rowIdentity(row);
}

function applyModelFilter(row) {
  if (!canApplySelectedFilter(selectedModelId.value, row)) return;
  setModelFilter(row);
}

function setModelFilter(row) {
  filters.value.model = row.model || 'all';
  refresh(true);
}

function setupTimer() {
  clearTimer();
  if (props.ready && autoRefreshMs.value > 0) timer = window.setInterval(() => refresh(false), autoRefreshMs.value);
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
  hideModelRouteTooltip(true);
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

function showModelRouteTooltip(event, row) {
  if (modelRouteHideTimer) {
    clearTimeout(modelRouteHideTimer);
    modelRouteHideTimer = null;
  }
  keepFailureTooltip();
  failureTooltip.value.visible = false;
  const el = event.currentTarget;
  const rect = el.getBoundingClientRect();
  const left = Math.max(12, Math.min(rect.left, window.innerWidth - 420 - 12));
  const spaceBelow = window.innerHeight - rect.bottom - 12;
  const estimatedHeight = 120 + (row.showResponseModel ? 70 : 0) + (row.responseModelConflict ? 130 : 0);
  const placement = spaceBelow >= estimatedHeight || spaceBelow >= rect.top ? 'below' : 'above';
  modelRouteTooltip.value = {
    visible: true,
    row,
    style: placement === 'below'
      ? {top: `${rect.bottom + 8}px`, left: `${left}px`}
      : {bottom: `${window.innerHeight - rect.top + 8}px`, left: `${left}px`},
  };
}

function toggleModelRouteTooltip(event, row) {
  if (modelRouteTooltip.value.visible && modelRouteTooltip.value.row?.id === row.id) {
    hideModelRouteTooltip(true);
  } else {
    showModelRouteTooltip(event, row);
  }
}

function keepModelRouteTooltip() {
  if (modelRouteHideTimer) {
    clearTimeout(modelRouteHideTimer);
    modelRouteHideTimer = null;
  }
}

function hideModelRouteTooltip(immediate = false) {
  if (modelRouteHideTimer) clearTimeout(modelRouteHideTimer);
  if (immediate === true) {
    modelRouteTooltip.value.visible = false;
    modelRouteHideTimer = null;
    return;
  }
  modelRouteHideTimer = setTimeout(() => {
    modelRouteTooltip.value.visible = false;
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
  const cols = ['timestamp_ms', 'failed', 'protocol', 'executor_type', 'model', 'alias', 'requested_model', 'resolved_model', 'auth_index', 'account_snapshot', 'api_key_hash', 'method', 'path', 'total_tokens', 'cache_hit_tokens', 'cache_hit_input_tokens', 'cache_hit_rate', 'latency_ms', 'fail_status_code', 'fail_summary', 'header_trace_id'];
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
  const built = {
    id: eventId,
    raw: row,
    sourceName,
    sourceIsApiKey: isSensitiveSource(sourceName, row.auth_type),
    provider: row.auth_provider_snapshot || row.provider || EMPTY_VALUE,
    providerChip: providerChip(row.auth_provider_snapshot || row.provider, row.auth_type),
    apiKeyHash: row.api_key_hash || EMPTY_VALUE,
    model: requestedModelName(row) || EMPTY_VALUE,
    alias: String(row.alias || '').trim(),
    mappedModel: mappedModelName(row),
    resolvedModel: mappedModelName(row),
    responseModel: responseModelName(row),
    hostResponseModel: String(row.host_response_model || '').trim(),
    observedResponseModel: String(row.observed_response_model || '').trim(),
    responseModelSource: responseModelSource(row),
    responseModelConflict: hasResponseModelConflict(row),
    showResponseModel: hasResponseModelDifference(row),
    responseModelMismatch: hasResponseModelMismatch({
      model: requestedModelName(row) || EMPTY_VALUE,
      mappedModel: mappedModelName(row),
      responseModel: responseModelName(row),
      ...row,
    }),
    hasModelDetails: hasModelRouteDetails(row),
    intensity: row.reasoning_effort || row.response_service_tier || row.service_tier || '-',
    intensityDisplay: String(row.reasoning_effort || '').trim() || EMPTY_VALUE,
    tier: row.response_service_tier || row.service_tier || (row.reasoning_effort && row.reasoning_effort !== '-' ? 'priority' : 'default'),
    tierDisplay: String(row.response_service_tier || row.service_tier || '').trim() || EMPTY_VALUE,
    recentPattern: (sliding?.recentPattern || []).slice(-5),
    failed: Boolean(row.failed),
    protocol: requestProtocol(row),
    protocolLabel: requestProtocolLabel(row, t),
    httpStatus: numberOrNull(row.http_status_code ?? row.status_code ?? row.response_status_code ?? row.fail_status_code),
    successRate: sliding?.successRate ?? (row.failed ? 0 : 1),
    totalCalls: sliding?.requestCount ?? 1,
    tps: latencyMs && latencyMs > 0 ? outputTokens / (latencyMs / 1000) : null,
    ttftMs: numberOrNull(row.ttft_ms) ?? latencyMs,
    latencyMs,
    timestampMs: row.timestamp_ms,
    totalTokens: Number(row.total_tokens || 0),
    usageText: buildUsageText(row),
    cacheHitRate: computeCacheHitRate(row),
    cost: eventCostAmount(row),
    costMeta: eventCostMeta(row),
    costTooltip: '',
    failStatusCode: numberOrNull(row.fail_status_code),
    failSummary: row.fail_summary || '',
  };
  built.modelMeta = buildModelMeta(built, t);
  built.callsSub = formatCallsSub(fmtInt(built.totalCalls), t);
  built.tpsSub = formatTpsSub(fmtTps(built.tps), t);
  built.cacheSub = formatCacheSub(fmtCacheHitRate(built.cacheHitRate), t);
  built.costText = fmtMoney(built.cost);
  built.hints = buildEventHints({
    ...built,
    successRateText: fmtPct(built.successRate),
    totalCallsText: fmtInt(built.totalCalls),
    ttftText: fmtSeconds(built.ttftMs),
    latencyText: fmtSeconds(built.latencyMs),
    tpsText: fmtTps(built.tps),
    totalTokensText: fmtCompact(built.totalTokens),
    costText: fmtMoney(built.cost),
    cacheText: fmtCacheHitRate(built.cacheHitRate),
  }, t);
  built.costTooltip = eventCostTooltip(row) || built.hints.cost;
  return built;
}

function eventGroupKey(row) {
  const source = String(row.source || '').trim() || 'unknown';
  const provider = row.auth_provider_snapshot || row.provider || '';
  const authType = row.auth_type || '';
  const stableIdentity = row.auth_index || row.auth_file_snapshot || row.api_key_hash || source;
  const model = row.model || '';
  return [source, provider, authType, stableIdentity, model].join('::');
}

function numberOrNull(v) {
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

function buildUsageText(row) {
  return buildUsageIOC(row, fmtCompact);
}

function fmtCacheHitRate(value) {
  return formatCacheHitRate(value, fmtPct, EMPTY_VALUE);
}

function eventCostAmount(row) {
  return formatEventCostAmount(row);
}

function eventCostMeta(row) {
  return formatEventCostMeta(row, t);
}

function eventCostTooltip(row) {
  return formatEventCostTooltip(row, t);
}

function aggregateCostCoverage(row) {
  return formatAggregateCostCoverage(row, t, fmtInt);
}

function aggregateCostText(row) {
  return formatAggregateCostText(row, fmtMoney, EMPTY_VALUE);
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
  const seconds = (Number(v) / 1000).toFixed(Number(v) >= 10000 ? 1 : 2);
  return `${seconds.replace(/\.?0+$/, '')} s`;
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
  emits: ['select', 'filter'],
  setup(props, {emit}) {
    const {t: ti18n} = useI18n();
    return () => {
      if (!props.rows.length) return h('div', {class: 'empty'}, ti18n('common.noData'));
      const head = h('thead', h('tr', props.columns.map(col => h('th', col[1]))));
      const body = h('tbody', props.rows.slice(0, 250).map((row, idx) => {
        const rowId = rowIdentity(row, idx);
        const isSelected = canApplySelectedFilter(props.selectedId, {id: rowId});
        return h('tr', {
              class: props.selectable ? ['clickable', isSelected ? 'selected-row' : ''].filter(Boolean).join(' ') : 'clickable',
              key: idx,
              onClick: () => emit('select', row)
            }, props.columns.map(col => {
              if (col[2] === 'filter') {
                return h('td', h('button', {
                  type: 'button',
                  class: 'btn btn-xs',
                  disabled: !isSelected,
                  onClick: (event) => {
                    event.stopPropagation();
                    if (isSelected) emit('filter', row);
                  },
                }, ti18n('monitoring.labels.filter')));
              }
              return h('td', renderCell(row[col[0]], col[2], row));
            }));
      }));
      return h('div', {class: 'table-wrap monitor-table'}, h('table', [head, body]));
    };
  }
});
function renderProviderValue(chip) {
  if (!chip?.tag) return h('strong', {class: 'config-meta-value'}, EMPTY_VALUE);
  const children = [h('span', {class: ['provider-chip', chip.chip]}, chip.tag)];
  if (chip.showName) children.push(h('span', {class: 'provider-chip-name'}, chip.name));
  return h('span', {class: 'provider-cell detail-provider-cell'}, children);
}

const DetailGrid = defineComponent({
  props: {items: {type: Array, default: () => []}},
  setup(props) {
    const {t: ti18n} = useI18n();
    return () => h('div', {class: 'config-meta-grid'}, props.items.map((item, idx) => {
      const value = item.providerChip
        ? renderProviderValue(item.providerChip)
        : item.sensitive
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
    {label: t('monitoring.accountColumns.provider'), value: row.auth_provider_snapshot || EMPTY_VALUE, providerChip: accountProviderChip(row), wide: true},
    {label: t('monitoring.labels.requests'), value: fmtInt(row.calls)},
    {label: t('monitoring.labels.successRate'), value: fmtPct(row.success_rate)},
    {label: t('monitoring.labels.token'), value: fmtCompact(row.total_tokens)},
    {label: t('monitoring.labels.cost'), value: aggregateCostText(row)},
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

function renderCell(v, type, row) {
  if (type === 'usage') {
    return h('div', {class: 'usage-cell'}, [
      h('strong', fmtCompact(row?.total_tokens)),
      h('div', {class: 'muted small-text usage-breakdown'}, buildUsageIOC(row, fmtCompact)),
    ]);
  }
  if (type === 'pct') return fmtPct(v);
  if (type === 'money') {
    const pricedCalls = Number(row?.priced_calls || 0);
    const unpricedCalls = Number(row?.unpriced_calls || 0);
    const title = `${t('monitoring.costEstimate.apiEquivalent')} · ${aggregateCostCoverage(row)} · ${t('monitoring.costEstimate.subscriptionNote')}`;
    return h('span', {class: 'cost-aggregate-cell', title}, [
      h('strong', pricedCalls > 0 ? fmtMoney(v) : EMPTY_VALUE),
      unpricedCalls > 0 ? h('small', {class: 'cost-aggregate-meta'}, t('monitoring.costEstimate.unpricedShort', {count: fmtInt(unpricedCalls)})) : null,
    ]);
  }
  if (type === 'ms') return fmtMs(v);
  if (type === 'time') return formatDateTime(v);
  if (type === 'int') return fmtInt(v);
  if (type === 'hash') return shortHash(v);
  if (Array.isArray(v)) return v.join(', ');
  if (v && typeof v === 'object') return JSON.stringify(v);
  return v == null || v === '' ? EMPTY_VALUE : String(v);
}
</script>
