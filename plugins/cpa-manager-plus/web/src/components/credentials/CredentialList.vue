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

    <div v-if="rows.length" class="table-wrap monitor-table cred-table">
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
            @click="$emit('select', row)"
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
                    <b>{{ remainingText(qw.remainingPercent) }}</b>
                  </div>
                  <div class="quota-bar" :class="quotaTone(qw.remainingPercent)">
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
    <div v-else class="empty">{{ loading ? t('common.loading') : t('monitoring.credentials.empty') }}</div>
  </div>
</template>

<script setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import MetricGrid from '../MetricGrid.vue';
import SparklineBars from './SparklineBars.vue';
import { EMPTY_VALUE, formatInt } from '../../utils/localeFormat.js';
import { clampPercent, quotaBarTone } from '../../utils/credentialPresentation.js';

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
  formatCompact: { type: Function, required: true },
  formatPercent: { type: Function, required: true },
  formatCostText: { type: Function, required: true },
});

defineEmits(['select', 'update:providerFilter', 'update:statusFilter', 'update:search']);

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
function remainingText(remaining) {
  if (!Number.isFinite(Number(remaining))) return EMPTY_VALUE;
  return `${Math.round(Number(remaining))}%`;
}
</script>
