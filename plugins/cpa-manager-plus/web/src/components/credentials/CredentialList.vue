<template>
  <div class="cred-list">
    <div class="cred-toolbar">
      <div class="cred-toolbar-top">
        <div class="cred-provider-chips segmented-control" role="group" :aria-label="t('monitoring.credentials.columns.credential')">
          <button
            type="button"
            :class="['segment-btn', { active: providerFilter === 'all' }]"
            @click="$emit('update:providerFilter', 'all')"
          >
            {{ t('monitoring.credentials.filters.all') }}
            <span class="segment-count">{{ totalCount }}</span>
          </button>
          <button
            v-for="chip in providerChips"
            :key="chip.key"
            type="button"
            :class="['segment-btn', { active: providerFilter === chip.key }]"
            @click="$emit('update:providerFilter', chip.key)"
          >
            {{ chip.label }}
            <span class="segment-count">{{ chip.count }}</span>
          </button>
        </div>

        <div class="cred-toolbar-actions">
          <button
            class="btn cred-probe-all-btn"
            type="button"
            :disabled="loading || batchProbing || !ready || !rows.length"
            @click="$emit('probe-all')"
          >
            <span v-if="batchProbing" class="cred-enriching-spinner" aria-hidden="true"></span>
            {{ batchProbing
              ? t('monitoring.credentials.probingProgress', { done: batchProgress?.done || 0, total: batchProgress?.total || rows.length })
              : t('monitoring.credentials.probeAll') }}
          </button>
          <button
            class="btn primary cred-filters-refresh"
            type="button"
            :disabled="loading || !ready"
            @click="$emit('refresh')"
          >{{ loading ? t('common.loading') : t('common.refresh') }}</button>
        </div>
      </div>

      <div class="cred-filters">
        <input
          class="control wide"
          :value="search"
          :placeholder="t('monitoring.credentials.searchPlaceholder')"
          @input="$emit('update:search', $event.target.value)"
        />
        <select class="control compact" :value="statusFilter" @change="$emit('update:statusFilter', $event.target.value)">
          <option value="all">{{ t('monitoring.credentials.filters.allStatuses') }}</option>
          <option value="available">{{ t('monitoring.credentials.filters.available') }}</option>
          <option value="attention">{{ t('monitoring.credentials.filters.attention') }}</option>
          <option value="quota_risk">{{ t('monitoring.credentials.filters.quotaRisk') }}</option>
          <option value="disabled">{{ t('monitoring.credentials.filters.disabled') }}</option>
        </select>
        <select class="control compact" :value="sortBy" @change="$emit('update:sortBy', $event.target.value)">
          <option value="default">{{ t('monitoring.credentials.sort.default') }}</option>
          <option value="quota_asc">{{ t('monitoring.credentials.sort.quotaAsc') }}</option>
          <option value="cost_desc">{{ t('monitoring.credentials.sort.costDesc') }}</option>
          <option value="requests_desc">{{ t('monitoring.credentials.sort.requestsDesc') }}</option>
          <option value="recent_desc">{{ t('monitoring.credentials.sort.recentDesc') }}</option>
        </select>
      </div>
    </div>

    <section class="metric-grid cred-kpi-grid" role="group" :aria-label="t('monitoring.credentials.kpi.total')">
      <button
        v-for="card in kpiCards"
        :key="card.key"
        type="button"
        class="metric cred-kpi-btn"
        :class="[
          card.accent ? `metric-accent-${card.accent}` : '',
          { active: statusFilter === card.filterValue && card.filterValue !== 'all' },
        ]"
        :aria-pressed="statusFilter === card.filterValue"
        @click="toggleKpiFilter(card.filterValue)"
      >
        <div class="metric-label">{{ card.label }}</div>
        <div class="metric-value">{{ card.value }}</div>
        <div v-if="card.sub" class="metric-sub">{{ card.sub }}</div>
      </button>
    </section>

    <div v-if="loading && !rows.length" class="cred-state cred-state-loading" role="status">
      <div class="cred-card-grid" aria-hidden="true">
        <div v-for="n in 6" :key="n" class="cred-skeleton-card"></div>
      </div>
      <span class="muted">{{ t('common.loading') }}</span>
    </div>

    <div v-else-if="rows.length" class="cred-card-grid" role="list">
      <article
        v-for="row in rows"
        :key="row.rowKey"
        class="cred-card clickable"
        :class="{
          'selected-row': row.rowKey === selectedRowKey,
          'cred-row-enriching': row.rowKey === enrichingRowKey,
          'cred-card-disabled': row.disabled,
        }"
        :data-tone="row.availabilityTone || 'ok'"
        :data-cred-row-key="row.rowKey"
        role="button"
        tabindex="0"
        :aria-pressed="row.rowKey === selectedRowKey"
        :aria-busy="row.rowKey === enrichingRowKey ? 'true' : undefined"
        @click="$emit('select', row, $event)"
        @keydown="onRowKeydown($event, row)"
      >
        <!-- Header: Badges + Status + Quick Refresh, then Identity + Recent Sparkline -->
        <header class="cred-card-header">
          <div class="cred-card-badges">
            <div class="cred-card-badges-left">
              <span v-if="row.providerChip?.tag" :class="['provider-chip', row.providerChip.chip]">{{ row.providerChip.tag }}</span>
              <span v-if="row.planLabel" class="cred-plan-badge">{{ row.planLabel }}</span>
              <span v-if="row.priority != null" class="cred-priority-badge" :title="t('monitoring.credentials.priority', { value: row.priority })">
                P{{ row.priority }}
              </span>
            </div>
            <div class="cred-card-badges-right">
              <span :class="['status-chip', 'cred-avail', row.availabilityTone]">
                <i aria-hidden="true"></i>{{ row.availabilityLabel }}
              </span>
              <button
                type="button"
                class="cred-card-refresh-btn"
                :class="{ spinning: row.rowKey === enrichingRowKey }"
                :disabled="row.rowKey === enrichingRowKey"
                :title="t('monitoring.credentials.drawer.refreshQuota')"
                :aria-label="t('monitoring.credentials.drawer.refreshQuota')"
                @click.stop="$emit('refresh-row', row)"
              >
                <svg viewBox="0 0 16 16" width="13" height="13" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true">
                  <path d="M13.5 8a5.5 5.5 0 1 1-1.35-3.6M13.5 2.5v2.4h-2.4" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>
              </button>
            </div>
          </div>

          <div class="cred-card-main">
            <div class="cred-card-identity">
              <strong class="cred-identity-title" :title="row.email || row.displayName || row.fileName">
                {{ row.maskedEmail || row.displayName || EMPTY_VALUE }}
              </strong>
              <div class="muted small-text cred-identity-file" :title="row.path || row.fileName">
                <span>{{ row.fileName || EMPTY_VALUE }}</span>
                <template v-if="row.note"> · <span>{{ row.note }}</span></template>
              </div>
            </div>
            <div class="cred-card-recent" :title="t('monitoring.credentials.columns.recent')">
              <span class="cred-recent-time muted">{{ row.lastRequestLabel }}</span>
              <SparklineBars
                :statuses="row.recentStatuses"
                :title="t('monitoring.credentials.columns.recent')"
              />
            </div>
          </div>

          <button
            v-if="row.probeFailureSummary"
            type="button"
            class="cred-card-alert"
            :title="row.probeFailureSummary"
            @click.stop="onProbeFailCta($event, row)"
          >
            <span class="cred-card-alert-dot" aria-hidden="true"></span>
            <span class="cred-card-alert-text">{{ row.probeFailureSummary }}</span>
          </button>
        </header>

        <!-- Body: Aggregated Quota Windows -->
        <section class="cred-card-quota-section">
          <div v-if="row.quotaDisplays?.length" class="cred-quota-stack">
            <div
              v-for="qw in row.quotaDisplays"
              :key="qw.key"
              class="quota-row"
              :title="qw.title"
            >
              <div class="quota-row-header">
                <div class="quota-row-left">
                  <span class="quota-row-label">{{ qw.shortLabel }}</span>
                  <span v-if="qw.usageLine" class="quota-usage-line muted">{{ qw.usageLine }}</span>
                </div>
                <b class="quota-row-pct" :class="{ 'cred-rem-depleted': isDepleted(qw.remainingPercent) }">
                  {{ remainingText(qw.remainingPercent) }}
                </b>
              </div>
              <div class="quota-bar" :class="[quotaTone(qw.remainingPercent), { depleted: isDepleted(qw.remainingPercent) }]">
                <span :style="{ width: `${clamp(qw.remainingPercent)}%` }"></span>
              </div>
            </div>
            <div v-if="row.extraQuotaCount > 0" class="cred-extra-windows muted small-text">
              {{ t('monitoring.credentials.moreWindows', { count: row.extraQuotaCount }) }}
            </div>
          </div>

          <div v-else-if="row.probeFailed" class="cred-quota-placeholder warn">
            <button
              type="button"
              class="cred-probe-cta"
              @click.stop="onProbeFailCta($event, row)"
            >{{ t('monitoring.credentials.probeFailQuota') }}</button>
          </div>

          <div v-else class="cred-quota-placeholder">
            <span class="muted small-text">{{ t('monitoring.credentials.noQuotaYet') }}</span>
            <button
              type="button"
              class="cred-probe-inline-btn"
              :disabled="row.rowKey === enrichingRowKey"
              @click.stop="$emit('refresh-row', row)"
            >
              {{ row.rowKey === enrichingRowKey ? t('monitoring.credentials.enriching') : t('monitoring.credentials.probeQuota') }}
            </button>
          </div>
        </section>

        <!-- Footer: 4-Column Historical Usage Strip -->
        <footer class="cred-card-footer">
          <div class="cred-stat-cell" :title="t('monitoring.credentials.drawer.requests')">
            <span class="cred-stat-label">{{ t('monitoring.credentials.histLabels.requests') }}</span>
            <strong class="cred-stat-value">{{ hasHistoryUsage(row) ? fmtCompact(row.history?.requests) : EMPTY_VALUE }}</strong>
          </div>
          <div class="cred-stat-cell" :title="t('monitoring.credentials.drawer.tokens')">
            <span class="cred-stat-label">{{ t('monitoring.credentials.histLabels.tokens') }}</span>
            <strong class="cred-stat-value">{{ hasHistoryUsage(row) ? fmtCompact(row.history?.tokens) : EMPTY_VALUE }}</strong>
          </div>
          <div class="cred-stat-cell" :title="t('monitoring.credentials.drawer.estCost')">
            <span class="cred-stat-label">{{ t('monitoring.credentials.histLabels.cost') }}</span>
            <strong class="cred-stat-value">{{ hasHistoryUsage(row) ? formatCost(row.history) : EMPTY_VALUE }}</strong>
          </div>
          <div class="cred-stat-cell" :title="t('monitoring.credentials.drawer.successRate')">
            <span class="cred-stat-label">{{ t('monitoring.credentials.histLabels.success') }}</span>
            <strong class="cred-stat-value" :class="hasHistoryUsage(row) ? successClass(row.history?.successRate) : ''">
              {{ hasHistoryUsage(row) ? fmtPct(row.history?.successRate) : EMPTY_VALUE }}
            </strong>
          </div>
        </footer>
      </article>
    </div>

    <div v-else class="cred-state empty" role="status">
      {{ hasActiveFilters ? t('monitoring.credentials.emptyFiltered') : t('monitoring.credentials.empty') }}
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import SparklineBars from './SparklineBars.vue';
import { EMPTY_VALUE, formatInt } from '../../utils/localeFormat.js';
import { clampPercent, formatRemainingPercent, quotaBarTone } from '../../utils/credentialPresentation.js';

const props = defineProps({
  rows: { type: Array, default: () => [] },
  kpi: { type: Object, default: () => ({}) },
  providerChips: { type: Array, default: () => [] },
  providerFilter: { type: String, default: 'all' },
  statusFilter: { type: String, default: 'all' },
  sortBy: { type: String, default: 'default' },
  search: { type: String, default: '' },
  selectedRowKey: { type: String, default: '' },
  enrichingRowKey: { type: String, default: '' },
  batchProbing: { type: Boolean, default: false },
  batchProgress: { type: Object, default: () => ({ done: 0, total: 0 }) },
  loading: { type: Boolean, default: false },
  ready: { type: Boolean, default: false },
  totalCount: { type: Number, default: 0 },
  hasActiveFilters: { type: Boolean, default: false },
  formatCompact: { type: Function, required: true },
  formatPercent: { type: Function, required: true },
  formatCostText: { type: Function, required: true },
});

const emit = defineEmits([
  'select',
  'refresh',
  'refresh-row',
  'probe-all',
  'update:providerFilter',
  'update:statusFilter',
  'update:sortBy',
  'update:search',
]);

const { t } = useI18n();

const kpiCards = computed(() => {
  const filtered = Boolean(props.kpi.filtered);
  const filterSub = t('monitoring.credentials.kpi.filteredSub');
  return [
    {
      key: 'total',
      filterValue: 'all',
      label: t('monitoring.credentials.kpi.total'),
      value: formatInt(props.kpi.total || 0),
      sub: t('monitoring.credentials.kpi.totalSub'),
      accent: 'blue',
    },
    {
      key: 'available',
      filterValue: 'available',
      label: t('monitoring.credentials.kpi.available'),
      value: formatInt(props.kpi.available || 0),
      sub: filtered ? filterSub : t('monitoring.credentials.kpi.availableSub'),
      accent: 'green',
    },
    {
      key: 'attention',
      filterValue: 'attention',
      label: t('monitoring.credentials.kpi.attention'),
      value: formatInt(props.kpi.attention || 0),
      sub: filtered ? filterSub : t('monitoring.credentials.kpi.attentionSub'),
      accent: 'red',
    },
    {
      key: 'quotaRisk',
      filterValue: 'quota_risk',
      label: t('monitoring.credentials.kpi.quotaRisk'),
      value: formatInt(props.kpi.quotaRisk || 0),
      sub: filtered ? filterSub : t('monitoring.credentials.kpi.quotaRiskSub'),
      accent: 'amber',
    },
  ];
});

function toggleKpiFilter(filterValue) {
  if (filterValue === 'all' || props.statusFilter === filterValue) {
    emit('update:statusFilter', 'all');
  } else {
    emit('update:statusFilter', filterValue);
  }
}

function fmtCompact(value) {
  return props.formatCompact(value);
}
function fmtPct(value) {
  return props.formatPercent(value);
}
function formatCost(history) {
  return props.formatCostText(history);
}
function successClass(rate) {
  if (rate == null) return '';
  if (rate >= 0.95) return 'good-text';
  if (rate >= 0.8) return '';
  return 'bad-text';
}
function quotaTone(remaining) {
  return quotaBarTone(remaining);
}
function clamp(value) {
  return clampPercent(value);
}
function isDepleted(remaining) {
  const n = Number(remaining);
  return Number.isFinite(n) && n <= 0;
}
function remainingText(remaining) {
  return formatRemainingPercent(remaining, t('monitoring.credentials.depleted'));
}

function hasHistoryUsage(row) {
  const h = row?.history;
  if (!h) return false;
  const req = Number(h.requests);
  const tok = Number(h.tokens);
  const cost = Number(h.cost);
  if (Number.isFinite(req) && req > 0) return true;
  if (Number.isFinite(tok) && tok > 0) return true;
  if (Number.isFinite(cost) && cost > 0) return true;
  if (h.costComplete === false && (h.unpricedCalls || 0) > 0) return true;
  return false;
}
function onProbeFailCta(event, row) {
  emit('select', row, event, 'overview');
}
function onRowKeydown(event, row) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    emit('select', row, event);
  }
}
</script>
