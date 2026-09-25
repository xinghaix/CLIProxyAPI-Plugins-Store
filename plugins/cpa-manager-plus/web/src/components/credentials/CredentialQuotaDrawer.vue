<template>
  <div
    v-if="open && credential"
    class="drawer-backdrop cred-drawer-backdrop"
    @click.self="$emit('close')"
  >
    <aside
      class="modal-dialog card drawer cred-drawer"
      :class="{ 'cred-drawer--dragging': sheetDragging }"
      role="dialog"
      aria-modal="true"
      :aria-label="title"
      tabindex="-1"
      ref="panelRef"
      :style="sheetStyle"
    >
      <div
        class="cred-sheet-handle"
        aria-hidden="true"
        @pointerdown="onSheetPointerDown"
      >
        <span class="cred-sheet-handle-bar"></span>
      </div>
      <div class="drawer-head cred-drawer-head">
        <div class="cred-drawer-head-main">
          <h2 class="cred-drawer-title">{{ title }}</h2>
          <p class="muted small-text cred-drawer-sub">
            <span v-if="credential.planLabel" class="cred-plan-badge">{{ credential.planLabel }}</span>
            <span class="cred-drawer-path-text" :title="credential.path || credential.fileName || ''">{{ credential.path || credential.fileName || EMPTY_VALUE }}</span>
            <button
              v-if="credential.path || credential.fileName"
              type="button"
              class="btn btn-xs cred-copy-btn"
              @click="copyPath"
            >{{ copyLabel }}</button>
          </p>
        </div>
        <button class="btn cred-drawer-close" type="button" @click="$emit('close')">{{ t('common.close') }}</button>
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
        <MetricGrid class="cred-kpi-grid cred-overview-kpi" :cards="overviewCards" />
        <div class="cred-overview-section">
          <div class="cred-drawer-section-title">{{ t('monitoring.credentials.drawer.overviewIdentity') }}</div>
          <div class="detail-grid cred-overview-grid">
            <div><span class="muted">{{ t('monitoring.credentials.drawer.provider') }}</span><strong>{{ credential.provider || EMPTY_VALUE }}</strong></div>
            <div><span class="muted">{{ t('monitoring.credentials.drawer.authType') }}</span><strong>{{ credential.authType || EMPTY_VALUE }}</strong></div>
            <div><span class="muted">{{ t('monitoring.credentials.drawer.status') }}</span><strong>{{ credential.status || credential.availabilityLabel || EMPTY_VALUE }}</strong></div>
            <div><span class="muted">{{ t('monitoring.authCard.priority') }}</span><strong>{{ credential.priority ?? EMPTY_VALUE }}</strong></div>
            <div v-if="credential.weight != null"><span class="muted">{{ t('monitoring.credentials.drawer.weight') }}</span><strong>{{ credential.weight }}</strong></div>
            <div v-if="credential.projectId"><span class="muted">{{ t('monitoring.credentials.drawer.projectId') }}</span><strong>{{ credential.projectId }}</strong></div>
            <div v-if="credential.accountId"><span class="muted">{{ t('monitoring.credentials.drawer.accountId') }}</span><strong>{{ credential.accountId }}</strong></div>
            <div v-if="credential.note"><span class="muted">{{ t('monitoring.authCard.note') }}</span><strong>{{ credential.note }}</strong></div>
            <div v-if="credential.lastRefresh"><span class="muted">{{ t('monitoring.credentials.drawer.lastRefresh') }}</span><strong>{{ credential.lastRefresh }}</strong></div>
            <div v-if="credential.updatedAt"><span class="muted">{{ t('monitoring.credentials.drawer.updatedAt') }}</span><strong>{{ credential.updatedAt }}</strong></div>
          </div>
        </div>
        <div v-if="credential.quotaWindows?.length" class="cred-overview-section">
          <div class="cred-drawer-section-title">{{ t('monitoring.credentials.drawer.standardQuota') }}</div>
          <ul class="cred-overview-windows">
            <li v-for="w in credential.quotaWindows" :key="w.key" class="cred-overview-window">
              <div class="cred-overview-window-head">
                <strong>{{ w.label }}</strong>
                <span :class="{ 'cred-rem-depleted': isDepleted(w.remainingPercent) }">{{ remainingText(w.remainingPercent) }}</span>
                <span class="muted small-text">{{ w.resetAtMs ? formatCompact(w.resetAtMs) : '' }}</span>
              </div>
              <div class="quota-bar" :class="[quotaTone(w.remainingPercent), { depleted: isDepleted(w.remainingPercent) }]">
                <span :style="{ width: `${clamp(w.remainingPercent)}%` }"></span>
              </div>
            </li>
          </ul>
        </div>
      </div>

      <div v-else-if="activeTab === 'quota'" class="cred-drawer-body">
        <div class="cred-drawer-section-title">{{ t('monitoring.credentials.drawer.totalUsage') }}</div>
        <p class="muted small-text">{{ historyRangeLabel }}</p>
        <MetricGrid class="cred-kpi-grid cred-quota-summary" :cards="historyCards" />

        <div class="cred-drawer-section-title">{{ t('monitoring.credentials.drawer.standardQuota') }}</div>
        <div v-if="!windowCards.length" class="empty">{{ probing ? t('monitoring.authCard.querying') : t('monitoring.authCard.noQuota') }}</div>
        <article v-for="card in windowCards" :key="card.key" class="cred-window-card">
          <div class="cred-window-head">
            <div class="cred-window-head-main">
              <span class="cred-window-icon" :data-kind="card.kind || 'unknown'" aria-hidden="true"></span>
              <div>
                <div class="cred-window-title-row">
                  <strong>{{ card.label }}</strong>
                  <span v-if="card.resetLabel" class="cred-reset-badge">{{ t('monitoring.credentials.drawer.resetAt', { time: card.resetLabel }) }}</span>
                </div>
              </div>
            </div>
            <div class="cred-window-remaining" :class="{ 'cred-rem-depleted': isDepleted(card.remainingPercent) }">
              <span>{{ t('monitoring.credentials.drawer.remainingLabel') }}</span>
              <b>{{ remainingText(card.remainingPercent) }}</b>
            </div>
          </div>
          <div class="quota-bar thick" :class="[quotaTone(card.remainingPercent), { depleted: isDepleted(card.remainingPercent) }]">
            <span :style="{ width: `${clamp(card.remainingPercent)}%` }"></span>
          </div>
          <div class="cred-window-cols">
            <div class="cred-window-col">
              <div class="cred-window-col-title">{{ previousTitle(card) }}</div>
              <div class="cred-window-col-sub muted small-text">{{ card.previousRangeLabel || '\u00a0' }}</div>
              <UsageMetrics
                :metrics="card.previous"
                :format-compact="formatCompactNum"
                :format-percent="formatPercent"
                :format-cost-text="formatCostText"
                :labels="usageLabels"
              />
            </div>
            <div class="cred-window-col">
              <div class="cred-window-col-title">{{ t('monitoring.credentials.drawer.current') }}</div>
              <div class="cred-window-col-sub muted small-text">{{ card.currentRangeLabel || '\u00a0' }}</div>
              <UsageMetrics
                :metrics="card.current"
                :format-compact="formatCompactNum"
                :format-percent="formatPercent"
                :format-cost-text="formatCostText"
                :labels="usageLabels"
              />
            </div>
            <div class="cred-window-col cred-window-col-forecast">
              <div class="cred-window-col-title">{{ t('monitoring.credentials.drawer.forecast') }}</div>
              <div class="cred-window-col-sub muted small-text">
                <template v-if="card.forecast?.basis">{{ t(`monitoring.credentials.drawer.forecastBasis.${card.forecast.basis}`) }}</template>
                <template v-else>&nbsp;</template>
              </div>
              <UsageMetrics
                :metrics="card.forecast"
                :format-compact="formatCompactNum"
                :format-percent="formatPercent"
                :format-cost-text="formatCostText"
                :labels="forecastLabels"
                forecast
              />
              <div v-if="card.forecast" class="muted small-text cred-forecast-note">
                {{ t('monitoring.credentials.drawer.forecastSuccessUnavailable') }}
              </div>
            </div>
          </div>
          <p class="muted small-text cred-window-footer">{{ t('monitoring.credentials.drawer.windowFooter') }}</p>
        </article>
      </div>

      <div v-else-if="activeTab === 'settings'" class="cred-drawer-body">
        <div class="detail-grid cred-overview-grid">
          <div><span class="muted">{{ t('monitoring.authCard.priority') }}</span><strong>{{ credential.priority ?? EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.credentials.drawer.weight') }}</span><strong>{{ credential.weight ?? EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.authCard.note') }}</span><strong>{{ credential.note || EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.credentials.drawer.disabled') }}</span><strong>{{ credential.disabled ? t('common.disabled') : t('common.enabled') }}</strong></div>
        </div>
        <div class="cred-stub-panel" role="note">
          <p class="muted small-text">{{ t('monitoring.credentials.drawer.readOnlyHint') }}</p>
        </div>
      </div>

      <div v-else-if="activeTab === 'models'" class="cred-drawer-body">
        <p class="muted small-text">{{ t('monitoring.credentials.drawer.modelsHint') }}</p>
        <div v-if="credential.probe?.models?.length" class="cred-model-list">
          <span v-for="m in credential.probe.models" :key="m" class="chip">{{ m }}</span>
        </div>
        <div v-else class="cred-stub-panel empty" role="status">{{ t('monitoring.credentials.drawer.stubTab') }}</div>
      </div>

      <div v-else class="cred-drawer-body">
        <div class="detail-grid cred-overview-grid">
          <div><span class="muted">{{ t('monitoring.credentials.drawer.status') }}</span><strong>{{ credential.status || EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.credentials.drawer.statusMessage') }}</span><strong>{{ credential.statusMessage || EMPTY_VALUE }}</strong></div>
          <div><span class="muted">{{ t('monitoring.credentials.columns.availability') }}</span><strong>{{ credential.availabilityLabel || EMPTY_VALUE }}</strong></div>
          <div v-if="credential.probe?.actionReason"><span class="muted">{{ t('monitoring.credentials.drawer.probeReason') }}</span><strong>{{ credential.probe.actionReason }}</strong></div>
          <div v-if="credential.probe?.error"><span class="muted">{{ t('monitoring.credentials.drawer.probeError') }}</span><strong>{{ credential.probe.error }}</strong></div>
        </div>
      </div>

      <div v-if="actionNotice" class="notice error cred-drawer-notice">{{ actionNotice }}</div>

      <div class="cred-drawer-footer">
        <button class="btn primary" type="button" :disabled="probing" @click="$emit('refresh-quota')">
          {{ probing ? t('monitoring.authCard.querying') : t('monitoring.credentials.drawer.refreshQuota') }}
        </button>
      </div>
    </aside>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import MetricGrid from '../MetricGrid.vue';
import { EMPTY_VALUE, formatCompactDateTime, formatInt } from '../../utils/localeFormat.js';
import { clampPercent, formatRemainingPercent, quotaBarTone } from '../../utils/credentialPresentation.js';
import { focusInitialIn, trapTabKeydown } from '../../utils/focusTrap.js';

const props = defineProps({
  open: { type: Boolean, default: false },
  credential: { type: Object, default: null },
  history: { type: Object, default: null },
  historyFromMs: { type: Number, default: 0 },
  historyToMs: { type: Number, default: 0 },
  windowCards: { type: Array, default: () => [] },
  probing: { type: Boolean, default: false },
  actionNotice: { type: String, default: '' },
  timeZone: { type: String, default: '' },
  formatCompact: { type: Function, required: true },
  formatPercent: { type: Function, required: true },
  formatCostText: { type: Function, required: true },
});

const emit = defineEmits(['close', 'refresh-quota']);

const { t, locale } = useI18n();
const activeTab = ref('quota');
const copyFeedback = ref('');
const panelRef = ref(null);
const sheetOffsetY = ref(0);
const sheetDragging = ref(false);
let copyTimer = null;
let sheetDragCleanup = null;

const sheetStyle = computed(() => {
  if (!sheetOffsetY.value) return undefined;
  return {
    transform: `translateY(${sheetOffsetY.value}px)`,
    transition: sheetDragging.value ? 'none' : undefined,
  };
});

watch(() => props.credential?.rowKey, () => {
  activeTab.value = 'quota';
  copyFeedback.value = '';
});

watch(() => props.open, (isOpen) => {
  sheetOffsetY.value = 0;
  sheetDragging.value = false;
  if (isOpen) {
    // Accessible default: focus Close first so Esc/close is one Tab away from entry;
    // Tab/Shift+Tab then cycle only inside the panel (see trapTabKeydown).
    requestAnimationFrame(() => focusInitialIn(panelRef.value, '.cred-drawer-close'));
  }
});

function onKeydown(event) {
  if (!props.open) return;
  if (event.key === 'Escape') {
    event.preventDefault();
    emit('close');
    return;
  }
  if (event.key === 'Tab') {
    trapTabKeydown(event, panelRef.value);
  }
}

onMounted(() => {
  window.addEventListener('keydown', onKeydown);
});
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown);
  if (copyTimer) clearTimeout(copyTimer);
  if (sheetDragCleanup) sheetDragCleanup();
});

const tabs = computed(() => [
  { key: 'overview', label: t('monitoring.credentials.drawer.tabs.overview') },
  { key: 'quota', label: t('monitoring.credentials.drawer.tabs.quota') },
  { key: 'settings', label: t('monitoring.credentials.drawer.tabs.settings') },
  { key: 'models', label: t('monitoring.credentials.drawer.tabs.models') },
  { key: 'diagnostics', label: t('monitoring.credentials.drawer.tabs.diagnostics') },
]);

const title = computed(() => props.credential?.maskedEmail || props.credential?.displayName || props.credential?.fileName || EMPTY_VALUE);

const copyLabel = computed(() => (
  copyFeedback.value === 'ok'
    ? t('monitoring.credentials.drawer.copied')
    : copyFeedback.value === 'fail'
      ? t('monitoring.credentials.drawer.copyFailed')
      : t('monitoring.credentials.drawer.copy')
));

const usageLabels = computed(() => ({
  requests: t('monitoring.credentials.drawer.requests'),
  tokens: t('monitoring.credentials.drawer.tokens'),
  cost: t('monitoring.credentials.drawer.estCost'),
  successRate: t('monitoring.credentials.drawer.successRate'),
}));

const forecastLabels = computed(() => ({
  requests: t('monitoring.credentials.drawer.forecastRequests'),
  tokens: t('monitoring.credentials.drawer.forecastTokens'),
  cost: t('monitoring.credentials.drawer.forecastCost'),
}));

const overviewCards = computed(() => [
  { key: 'plan', label: t('monitoring.credentials.columns.plan'), value: props.credential?.planLabel || EMPTY_VALUE, accent: 'blue' },
  { key: 'avail', label: t('monitoring.credentials.columns.availability'), value: props.credential?.availabilityLabel || EMPTY_VALUE, accent: props.credential?.availabilityTone === 'ok' ? 'green' : 'amber' },
  { key: 'calls', label: t('monitoring.kpi.totalCalls'), value: formatInt(props.history?.requests || 0), accent: 'teal' },
  { key: 'cost', label: t('monitoring.kpi.estimatedCost'), value: props.formatCostText(props.history), accent: 'amber' },
]);

const historyCards = computed(() => [
  {
    key: 'req',
    label: t('monitoring.credentials.drawer.requests'),
    value: props.formatCompact(props.history?.requests),
    accent: 'blue',
    iconClass: 'tone-blue',
  },
  {
    key: 'tok',
    label: t('monitoring.credentials.drawer.tokens'),
    value: props.formatCompact(props.history?.tokens),
    accent: 'teal',
    iconClass: 'tone-teal',
  },
  {
    key: 'cost',
    label: t('monitoring.credentials.drawer.estCost'),
    value: props.formatCostText(props.history),
    accent: 'amber',
    iconClass: 'tone-amber',
  },
  {
    key: 'ok',
    label: t('monitoring.credentials.drawer.successRate'),
    value: props.formatPercent(props.history?.successRate),
    accent: 'green',
    iconClass: 'tone-green',
  },
]);

const historyRangeLabel = computed(() => {
  if (!props.historyFromMs || !props.historyToMs) return '';
  return t('monitoring.credentials.drawer.statsRange', {
    from: formatCompact(props.historyFromMs),
    to: formatCompact(props.historyToMs),
  });
});

function formatCompact(ms) {
  return formatCompactDateTime(ms, locale.value, props.timeZone || undefined);
}
function formatCompactNum(value) {
  return props.formatCompact(value);
}
function isDepleted(remaining) {
  const n = Number(remaining);
  return Number.isFinite(n) && n <= 0;
}
function remainingText(remaining) {
  return formatRemainingPercent(remaining, t('monitoring.credentials.depleted'));
}
function clamp(value) {
  return clampPercent(value);
}
function quotaTone(remaining) {
  return quotaBarTone(remaining);
}
function previousTitle(card) {
  if (card.previousPeriod === 'previous_equal_range') {
    return t('monitoring.credentials.drawer.previousEqual');
  }
  return t('monitoring.credentials.drawer.previous');
}
function isMobileSheet() {
  return typeof window !== 'undefined' && window.matchMedia('(max-width: 768px)').matches;
}

function onSheetPointerDown(event) {
  if (!isMobileSheet()) return;
  if (event.button != null && event.button !== 0) return;
  event.preventDefault();
  if (sheetDragCleanup) sheetDragCleanup();
  const startY = event.clientY;
  sheetDragging.value = true;
  sheetOffsetY.value = 0;
  const onMove = (ev) => {
    sheetOffsetY.value = Math.max(0, ev.clientY - startY);
  };
  const onUp = () => {
    if (sheetDragCleanup) sheetDragCleanup();
    sheetDragCleanup = null;
    sheetDragging.value = false;
    if (sheetOffsetY.value > 96) {
      sheetOffsetY.value = 0;
      emit('close');
      return;
    }
    sheetOffsetY.value = 0;
  };
  window.addEventListener('pointermove', onMove);
  window.addEventListener('pointerup', onUp);
  window.addEventListener('pointercancel', onUp);
  sheetDragCleanup = () => {
    window.removeEventListener('pointermove', onMove);
    window.removeEventListener('pointerup', onUp);
    window.removeEventListener('pointercancel', onUp);
  };
}

async function copyPath() {
  const value = props.credential?.path || props.credential?.fileName || '';
  if (!value) return;
  if (!navigator?.clipboard?.writeText) {
    copyFeedback.value = 'fail';
    scheduleCopyReset();
    return;
  }
  try {
    await navigator.clipboard.writeText(value);
    copyFeedback.value = 'ok';
  } catch {
    copyFeedback.value = 'fail';
  }
  scheduleCopyReset();
}
function scheduleCopyReset() {
  if (copyTimer) clearTimeout(copyTimer);
  copyTimer = setTimeout(() => { copyFeedback.value = ''; }, 1600);
}

const UsageMetrics = defineComponent({
  name: 'UsageMetrics',
  props: {
    metrics: { type: Object, default: null },
    formatCompact: Function,
    formatPercent: Function,
    formatCostText: Function,
    labels: { type: Object, default: () => ({}) },
    forecast: { type: Boolean, default: false },
  },
  setup(p) {
    return () => {
      const m = p.metrics;
      if (!m) {
        return h('div', { class: 'muted small-text cred-usage-empty' }, '—');
      }
      const rows = [
        { key: 'requests', tone: 'blue', label: p.labels.requests, value: p.formatCompact(m.requests) },
        { key: 'tokens', tone: 'teal', label: p.labels.tokens, value: p.formatCompact(m.tokens) },
        { key: 'cost', tone: 'amber', label: p.labels.cost, value: p.formatCostText(m) },
      ];
      if (!p.forecast && m.successRate != null && p.labels.successRate) {
        rows.push({
          key: 'success',
          tone: 'green',
          label: p.labels.successRate,
          value: p.formatPercent(m.successRate),
        });
      }
      return h(
        'div',
        { class: 'cred-usage-metrics' },
        rows.map((row) => h('div', { class: 'cred-usage-row', key: row.key }, [
          h('span', { class: ['cred-usage-dot', `tone-${row.tone}`], 'aria-hidden': 'true' }),
          h('span', { class: 'cred-usage-label' }, row.label),
          h('strong', { class: 'cred-usage-value' }, row.value),
        ]))
      );
    };
  },
});
</script>
