<template>
  <div v-if="open && credential" class="drawer-backdrop" @click.self="$emit('close')">
    <aside class="modal-dialog card drawer cred-drawer" role="dialog" :aria-label="title">
      <div class="drawer-head">
        <div>
          <h2>{{ title }}</h2>
          <p class="muted small-text cred-drawer-path">
            <span>{{ credential.path || credential.fileName || EMPTY_VALUE }}</span>
            <button v-if="credential.path || credential.fileName" type="button" class="btn btn-xs" @click="copyPath">{{ t('monitoring.credentials.drawer.copy') }}</button>
          </p>
        </div>
        <button class="btn" type="button" @click="$emit('close')">{{ t('common.close') }}</button>
      </div>

      <div class="monitor-tabs-list cred-drawer-tabs">
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          :class="['tab', { active: activeTab === tab.key }]"
          @click="activeTab = tab.key"
        >{{ tab.label }}</button>
      </div>

      <div v-if="activeTab === 'overview'" class="cred-drawer-body">
        <MetricGrid :cards="overviewCards" />
        <div class="detail-grid">
          <div><span class="muted">{{ t('monitoring.credentials.drawer.provider') }}</span><strong>{{ credential.provider || EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.credentials.drawer.authType') }}</span><strong>{{ credential.authType || EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.credentials.drawer.status') }}</span><strong>{{ credential.status || EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.authCard.priority') }}</span><strong>{{ credential.priority ?? EMPTY_VALUE }}</strong></div>
          <div v-if="credential.note"><span class="muted">{{ t('monitoring.authCard.note') }}</span><strong>{{ credential.note }}</strong></div>
        </div>
      </div>

      <div v-else-if="activeTab === 'quota'" class="cred-drawer-body">
        <div class="cred-drawer-section-title">{{ t('monitoring.credentials.drawer.totalUsage') }}</div>
        <p class="muted small-text">{{ historyRangeLabel }}</p>
        <MetricGrid :cards="historyCards" />

        <div class="cred-drawer-section-title">{{ t('monitoring.credentials.drawer.standardQuota') }}</div>
        <div v-if="!windowCards.length" class="empty">{{ probing ? t('monitoring.authCard.querying') : t('monitoring.authCard.noQuota') }}</div>
        <article v-for="card in windowCards" :key="card.key" class="cred-window-card">
          <div class="cred-window-head">
            <div>
              <strong>{{ card.label }}</strong>
              <span v-if="card.resetLabel" class="muted small-text"> · {{ card.resetLabel }}</span>
            </div>
            <div class="quota-meta">
              <b>{{ t('monitoring.credentials.drawer.remaining', { value: Math.round(card.remainingPercent ?? 0) }) }}</b>
            </div>
          </div>
          <div class="quota-bar" :class="quotaTone(card.remainingPercent)">
            <span :style="{ width: `${usedWidth(card)}%` }"></span>
          </div>
          <div class="cred-window-cols">
            <div>
              <div class="cred-window-col-title">{{ t('monitoring.credentials.drawer.previous') }}</div>
              <UsageMetrics :metrics="card.previous" :format-compact="formatCompact" :format-percent="formatPercent" :format-cost-text="formatCostText" />
            </div>
            <div>
              <div class="cred-window-col-title">{{ t('monitoring.credentials.drawer.current') }}</div>
              <UsageMetrics :metrics="card.current" :format-compact="formatCompact" :format-percent="formatPercent" :format-cost-text="formatCostText" />
            </div>
            <div>
              <div class="cred-window-col-title">{{ t('monitoring.credentials.drawer.forecast') }}</div>
              <UsageMetrics :metrics="card.forecast" :format-compact="formatCompact" :format-percent="formatPercent" :format-cost-text="formatCostText" forecast />
              <div v-if="card.forecast?.basis" class="muted small-text">{{ t(`monitoring.credentials.drawer.forecastBasis.${card.forecast.basis}`) }}</div>
            </div>
          </div>
          <p class="muted small-text">{{ t('monitoring.credentials.drawer.windowFooter') }}</p>
        </article>
      </div>

      <div v-else class="cred-drawer-body">
        <div class="empty">{{ t('monitoring.credentials.drawer.stubTab') }}</div>
      </div>

      <div class="cred-drawer-footer">
        <button class="btn primary" type="button" :disabled="probing" @click="$emit('refresh-quota')">
          {{ probing ? t('monitoring.authCard.querying') : t('monitoring.credentials.drawer.refreshQuota') }}
        </button>
        <!-- Disable intentionally omitted (read-only scope). -->
      </div>
    </aside>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import MetricGrid from '../MetricGrid.vue';
import { EMPTY_VALUE, formatDateTime, formatInt } from '../../utils/localeFormat.js';

const props = defineProps({
  open: { type: Boolean, default: false },
  credential: { type: Object, default: null },
  history: { type: Object, default: null },
  historyFromMs: { type: Number, default: 0 },
  historyToMs: { type: Number, default: 0 },
  windowCards: { type: Array, default: () => [] },
  probing: { type: Boolean, default: false },
  formatCompact: { type: Function, required: true },
  formatPercent: { type: Function, required: true },
  formatCostText: { type: Function, required: true },
});

defineEmits(['close', 'refresh-quota']);

const { t } = useI18n();
const activeTab = ref('quota');

watch(() => props.credential?.rowKey, () => { activeTab.value = 'quota'; });

const tabs = computed(() => [
  { key: 'overview', label: t('monitoring.credentials.drawer.tabs.overview') },
  { key: 'quota', label: t('monitoring.credentials.drawer.tabs.quota') },
  { key: 'settings', label: t('monitoring.credentials.drawer.tabs.settings') },
  { key: 'models', label: t('monitoring.credentials.drawer.tabs.models') },
  { key: 'diagnostics', label: t('monitoring.credentials.drawer.tabs.diagnostics') },
]);

const title = computed(() => props.credential?.maskedEmail || props.credential?.displayName || props.credential?.fileName || EMPTY_VALUE);

const overviewCards = computed(() => [
  { label: t('monitoring.credentials.columns.plan'), value: props.credential?.planLabel || EMPTY_VALUE },
  { label: t('monitoring.credentials.columns.availability'), value: props.credential?.availabilityLabel || EMPTY_VALUE },
  { label: t('monitoring.kpi.totalCalls'), value: formatInt(props.history?.requests || 0) },
  { label: t('monitoring.kpi.estimatedCost'), value: props.formatCostText(props.history) },
]);

const historyCards = computed(() => [
  { label: t('monitoring.credentials.drawer.requests'), value: props.formatCompact(props.history?.requests) },
  { label: t('monitoring.credentials.drawer.tokens'), value: props.formatCompact(props.history?.tokens) },
  { label: t('monitoring.credentials.drawer.estCost'), value: props.formatCostText(props.history) },
  { label: t('monitoring.credentials.drawer.successRate'), value: props.formatPercent(props.history?.successRate) },
]);

const historyRangeLabel = computed(() => {
  if (!props.historyFromMs || !props.historyToMs) return '';
  return t('monitoring.credentials.drawer.statsRange', {
    from: formatDateTime(props.historyFromMs),
    to: formatDateTime(props.historyToMs),
  });
});

function usedWidth(card) {
  const remaining = Number(card?.remainingPercent);
  if (!Number.isFinite(remaining)) return 0;
  return Math.max(0, Math.min(100, 100 - remaining));
}
function quotaTone(remaining) {
  const n = Number(remaining);
  if (!Number.isFinite(n)) return '';
  if (n <= 10) return 'danger';
  if (n <= 35) return 'warn';
  return 'ok';
}
async function copyPath() {
  const value = props.credential?.path || props.credential?.fileName || '';
  if (!value || !navigator?.clipboard) return;
  try { await navigator.clipboard.writeText(value); } catch { /* ignore */ }
}

const UsageMetrics = defineComponent({
  name: 'UsageMetrics',
  props: {
    metrics: { type: Object, default: null },
    formatCompact: Function,
    formatPercent: Function,
    formatCostText: Function,
    forecast: { type: Boolean, default: false },
  },
  setup(p) {
    return () => {
      const m = p.metrics;
      if (!m) return h('div', { class: 'muted small-text' }, '—');
      return h('div', { class: 'cred-usage-metrics' }, [
        h('div', [p.formatCompact(m.requests), ' req']),
        h('div', [p.formatCompact(m.tokens), ' tok']),
        h('div', [p.formatCostText(m)]),
        m.successRate != null && !p.forecast
          ? h('div', [p.formatPercent(m.successRate)])
          : null,
      ].filter(Boolean));
    };
  },
});
</script>
