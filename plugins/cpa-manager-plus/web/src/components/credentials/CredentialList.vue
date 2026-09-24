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

    <MetricGrid :cards="kpiCards" />

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
            <td>
              <strong>{{ row.maskedEmail || row.displayName || EMPTY_VALUE }}</strong>
              <div class="muted small-text">{{ row.fileName || EMPTY_VALUE }}</div>
              <div v-if="row.providerChip?.tag" class="provider-cell">
                <span :class="['provider-chip', row.providerChip.chip]">{{ row.providerChip.tag }}</span>
              </div>
            </td>
            <td>{{ row.planLabel || EMPTY_VALUE }}</td>
            <td>
              <span :class="['status-chip', row.availabilityTone]">{{ row.availabilityLabel }}</span>
              <div v-if="row.priority != null" class="muted small-text">{{ t('monitoring.credentials.priority', { value: row.priority }) }}</div>
            </td>
            <td>
              <div class="muted small-text">{{ row.lastRequestLabel }}</div>
              <SparklineBars :values="row.sparkValues" :title="t('monitoring.credentials.columns.recent')" />
            </td>
            <td>
              <div class="cred-hist">
                <span>{{ fmtCompact(row.history?.requests) }}</span>
                <span>{{ formatCost(row.history) }}</span>
                <span>{{ fmtCompact(row.history?.tokens) }}</span>
                <span :class="successClass(row.history?.successRate)">{{ fmtPct(row.history?.successRate) }}</span>
              </div>
            </td>
            <td>
              <div v-if="row.primaryQuota" class="quota-row">
                <div class="quota-row-header">
                  <span>{{ row.primaryQuota.label }}</span>
                  <b>{{ Math.round(row.primaryQuota.remainingPercent ?? 0) }}%</b>
                </div>
                <div class="quota-bar" :class="quotaTone(row.primaryQuota.remainingPercent)">
                  <span :style="{ width: `${clamp(row.primaryQuota.remainingPercent)}%` }"></span>
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
  { label: t('monitoring.credentials.kpi.total'), value: formatInt(props.kpi.total || 0), sub: t('monitoring.credentials.kpi.totalSub') },
  { label: t('monitoring.credentials.kpi.available'), value: formatInt(props.kpi.available || 0), sub: t('monitoring.credentials.kpi.availableSub') },
  { label: t('monitoring.credentials.kpi.attention'), value: formatInt(props.kpi.attention || 0), sub: t('monitoring.credentials.kpi.attentionSub') },
  { label: t('monitoring.credentials.kpi.quotaRisk'), value: formatInt(props.kpi.quotaRisk || 0), sub: t('monitoring.credentials.kpi.quotaRiskSub') },
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
  const n = Number(remaining);
  if (!Number.isFinite(n)) return '';
  if (n <= 10) return 'danger';
  if (n <= 35) return 'warn';
  return 'ok';
}
function clamp(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return 0;
  return Math.max(0, Math.min(100, n));
}
</script>
