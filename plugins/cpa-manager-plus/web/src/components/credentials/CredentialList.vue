<template>
  <div class="cred-list">
    <div class="cred-filters">
      <div class="cred-provider-chips">
        <button
          type="button"
          :class="['chip', { active: providerFilter === 'all' }]"
          @click="$emit('update:providerFilter', 'all')"
        >{{ t('monitoring.credentials.filters.all', { count: totalCount }) }}</button>
        <button
          v-for="chip in providerChips"
          :key="chip.key"
          type="button"
          :class="['chip', { active: providerFilter === chip.key }]"
          @click="$emit('update:providerFilter', chip.key)"
        >{{ chip.label }} ({{ chip.count }})</button>
      </div>
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
    </div>

    <MetricGrid class="cred-kpi-grid" :cards="kpiCards" />

    <div v-if="loading && !rows.length" class="cred-state cred-state-loading" role="status">
      <div class="cred-skeleton-stack" aria-hidden="true">
        <div v-for="n in 3" :key="n" class="cred-skeleton-row"></div>
      </div>
      <span class="muted">{{ t('common.loading') }}</span>
    </div>

    <template v-else-if="rows.length">
      <!-- Desktop table -->
      <div class="table-wrap monitor-table cred-table cred-table-desktop">
        <table>
          <thead>
            <tr>
              <th>{{ t('monitoring.credentials.columns.credential') }}</th>
              <th>{{ t('monitoring.credentials.columns.plan') }}</th>
              <th>{{ t('monitoring.credentials.columns.availability') }}</th>
              <th>{{ t('monitoring.credentials.columns.recent') }}</th>
              <th>{{ t('monitoring.credentials.columns.historical') }}</th>
              <th>{{ t('monitoring.credentials.columns.quota') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in rows"
              :key="row.rowKey"
              class="clickable"
              :class="{ 'selected-row': row.rowKey === selectedRowKey }"
              tabindex="0"
              role="button"
              :aria-pressed="row.rowKey === selectedRowKey"
              @click="$emit('select', row)"
              @keydown="onRowKeydown($event, row)"
            >
              <td class="cred-identity-cell">
                <strong class="cred-identity-title">{{ row.maskedEmail || row.displayName || EMPTY_VALUE }}</strong>
                <div class="muted small-text cred-identity-file" :title="row.path || row.fileName">{{ row.fileName || EMPTY_VALUE }}</div>
                <div v-if="row.providerChip?.tag" class="provider-cell">
                  <span :class="['provider-chip', row.providerChip.chip]">{{ row.providerChip.tag }}</span>
                </div>
              </td>
              <td>
                <span v-if="row.planLabel" class="cred-plan-badge">{{ row.planLabel }}</span>
                <span v-else class="muted">{{ EMPTY_VALUE }}</span>
              </td>
              <td>
                <span :class="['status-chip', 'cred-avail', row.availabilityTone]">
                  <i aria-hidden="true"></i>{{ row.availabilityLabel }}
                </span>
                <div v-if="row.priority != null" class="muted small-text">{{ t('monitoring.credentials.priority', { value: row.priority }) }}</div>
              </td>
              <td class="cred-recent-cell">
                <div class="cred-recent-time muted small-text">{{ row.lastRequestLabel }}</div>
                <SparklineBars
                  :statuses="row.recentStatuses"
                  :title="t('monitoring.credentials.columns.recent')"
                />
              </td>
              <td>
                <div class="cred-hist">
                  <span class="cred-hist-metric cred-hist-req" :title="t('monitoring.credentials.drawer.requests')">
                    <i class="cred-hist-icon" aria-hidden="true"></i>
                    <strong>{{ fmtCompact(row.history?.requests) }}</strong>
                  </span>
                  <span class="cred-hist-metric cred-hist-tok" :title="t('monitoring.credentials.drawer.tokens')">
                    <i class="cred-hist-icon" aria-hidden="true"></i>
                    <strong>{{ fmtCompact(row.history?.tokens) }}</strong>
                  </span>
                  <span class="cred-hist-metric cred-hist-cost" :title="t('monitoring.credentials.drawer.estCost')">
                    <i class="cred-hist-icon" aria-hidden="true"></i>
                    <strong>{{ formatCost(row.history) }}</strong>
                  </span>
                  <span
                    class="cred-hist-metric cred-hist-ok"
                    :class="successClass(row.history?.successRate)"
                    :title="t('monitoring.credentials.drawer.successRate')"
                  >
                    <i class="cred-hist-icon" aria-hidden="true"></i>
                    <strong>{{ fmtPct(row.history?.successRate) }}</strong>
                  </span>
                </div>
              </td>
              <td>
                <div v-if="row.quotaDisplays?.length" class="cred-quota-stack">
                  <div
                    v-for="qw in row.quotaDisplays"
                    :key="qw.key"
                    class="quota-row"
                    :title="qw.title"
                  >
                    <div class="quota-row-header">
                      <span class="quota-row-label">{{ qw.shortLabel }}</span>
                      <b :class="{ 'cred-rem-depleted': isDepleted(qw.remainingPercent) }">{{ remainingText(qw.remainingPercent) }}</b>
                    </div>
                    <div class="quota-bar" :class="[quotaTone(qw.remainingPercent), { depleted: isDepleted(qw.remainingPercent) }]">
                      <span :style="{ width: `${clamp(qw.remainingPercent)}%` }"></span>
                    </div>
                    <div v-if="qw.usageLine" class="quota-usage-line muted small-text">{{ qw.usageLine }}</div>
                  </div>
                </div>
                <div v-else class="muted">{{ EMPTY_VALUE }}</div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Mobile card stack (≤768 via CSS) -->
      <div class="cred-card-list" role="list">
        <article
          v-for="row in rows"
          :key="`card-${row.rowKey}`"
          class="cred-card clickable"
          :class="{ 'selected-row': row.rowKey === selectedRowKey }"
          role="button"
          tabindex="0"
          :aria-pressed="row.rowKey === selectedRowKey"
          @click="$emit('select', row)"
          @keydown="onRowKeydown($event, row)"
        >
          <div class="cred-card-top">
            <div class="cred-card-identity">
              <strong class="cred-identity-title">{{ row.maskedEmail || row.displayName || EMPTY_VALUE }}</strong>
              <div class="muted small-text cred-identity-file" :title="row.path || row.fileName">{{ row.fileName || EMPTY_VALUE }}</div>
              <div class="cred-card-meta">
                <span v-if="row.providerChip?.tag" :class="['provider-chip', row.providerChip.chip]">{{ row.providerChip.tag }}</span>
                <span v-if="row.planLabel" class="cred-plan-badge">{{ row.planLabel }}</span>
              </div>
            </div>
            <span :class="['status-chip', 'cred-avail', row.availabilityTone]">
              <i aria-hidden="true"></i>{{ row.availabilityLabel }}
            </span>
          </div>
          <div class="cred-card-recent">
            <span class="muted small-text">{{ row.lastRequestLabel }}</span>
            <SparklineBars
              :statuses="row.recentStatuses"
              :title="t('monitoring.credentials.columns.recent')"
            />
          </div>
          <div class="cred-hist cred-card-hist">
            <span class="cred-hist-metric cred-hist-req"><i class="cred-hist-icon" aria-hidden="true"></i><strong>{{ fmtCompact(row.history?.requests) }}</strong></span>
            <span class="cred-hist-metric cred-hist-tok"><i class="cred-hist-icon" aria-hidden="true"></i><strong>{{ fmtCompact(row.history?.tokens) }}</strong></span>
            <span class="cred-hist-metric cred-hist-cost"><i class="cred-hist-icon" aria-hidden="true"></i><strong>{{ formatCost(row.history) }}</strong></span>
            <span class="cred-hist-metric cred-hist-ok" :class="successClass(row.history?.successRate)"><i class="cred-hist-icon" aria-hidden="true"></i><strong>{{ fmtPct(row.history?.successRate) }}</strong></span>
          </div>
          <div v-if="row.quotaDisplays?.length" class="cred-quota-stack cred-card-quota">
            <div
              v-for="qw in row.quotaDisplays"
              :key="qw.key"
              class="quota-row"
              :title="qw.title"
            >
              <div class="quota-row-header">
                <span class="quota-row-label">{{ qw.shortLabel }}</span>
                <b :class="{ 'cred-rem-depleted': isDepleted(qw.remainingPercent) }">{{ remainingText(qw.remainingPercent) }}</b>
              </div>
              <div class="quota-bar" :class="[quotaTone(qw.remainingPercent), { depleted: isDepleted(qw.remainingPercent) }]">
                <span :style="{ width: `${clamp(qw.remainingPercent)}%` }"></span>
              </div>
              <div v-if="qw.usageLine" class="quota-usage-line muted small-text">{{ qw.usageLine }}</div>
            </div>
          </div>
          <div v-else class="muted small-text">{{ EMPTY_VALUE }}</div>
        </article>
      </div>
    </template>

    <div v-else class="cred-state empty" role="status">
      {{ hasActiveFilters ? t('monitoring.credentials.emptyFiltered') : t('monitoring.credentials.empty') }}
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import MetricGrid from '../MetricGrid.vue';
import SparklineBars from './SparklineBars.vue';
import { EMPTY_VALUE, formatInt } from '../../utils/localeFormat.js';
import { clampPercent, formatRemainingPercent, quotaBarTone } from '../../utils/credentialPresentation.js';

const props = defineProps({
  rows: { type: Array, default: () => [] },
  kpi: { type: Object, default: () => ({}) },
  providerChips: { type: Array, default: () => [] },
  providerFilter: { type: String, default: 'all' },
  statusFilter: { type: String, default: 'all' },
  search: { type: String, default: '' },
  selectedRowKey: { type: String, default: '' },
  loading: { type: Boolean, default: false },
  totalCount: { type: Number, default: 0 },
  hasActiveFilters: { type: Boolean, default: false },
  formatCompact: { type: Function, required: true },
  formatPercent: { type: Function, required: true },
  formatCostText: { type: Function, required: true },
});

const emit = defineEmits(['select', 'update:providerFilter', 'update:statusFilter', 'update:search']);

const { t } = useI18n();

const kpiCards = computed(() => [
  {
    key: 'total',
    label: t('monitoring.credentials.kpi.total'),
    value: formatInt(props.kpi.total || 0),
    sub: t('monitoring.credentials.kpi.totalSub'),
    accent: 'blue',
  },
  {
    key: 'available',
    label: t('monitoring.credentials.kpi.available'),
    value: formatInt(props.kpi.available || 0),
    sub: t('monitoring.credentials.kpi.availableSub'),
    accent: 'green',
  },
  {
    key: 'attention',
    label: t('monitoring.credentials.kpi.attention'),
    value: formatInt(props.kpi.attention || 0),
    sub: t('monitoring.credentials.kpi.attentionSub'),
    accent: 'red',
  },
  {
    key: 'quotaRisk',
    label: t('monitoring.credentials.kpi.quotaRisk'),
    value: formatInt(props.kpi.quotaRisk || 0),
    sub: t('monitoring.credentials.kpi.quotaRiskSub'),
    accent: 'amber',
  },
]);

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
function onRowKeydown(event, row) {
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault();
    emit('select', row);
  }
}
</script>
