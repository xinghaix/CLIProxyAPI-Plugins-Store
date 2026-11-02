<template>
  <section class="monitoring-page window-keeper-page">
    <!-- ====== Unified Command & Global Policy Bar ====== -->
    <div class="card dash-command-bar wk-command-bar">
      <div class="dash-command-top">
        <div class="window-keeper-status-info">
          <span :class="['status-badge', settings.enabled ? 'good' : 'idle']">
            <i aria-hidden="true"></i>{{ settings.enabled ? t('common.enabled') : t('common.disabled') }}
          </span>
          <button
            type="button"
            :class="['toggle-switch', { on: settings.enabled }]"
            :disabled="saving || loading || !ready"
            :aria-label="t('common.enabled')"
            @click="toggleGlobalSwitch"
          >
            <span class="toggle-knob"></span>
          </button>
          <span class="muted small-text">
            {{ settings.model || 'gpt-5.4' }} · {{ formatEffort(settings.effort) }}
            <template v-if="settings.service_tier"> · {{ settings.service_tier }}</template>
            · {{ settings.window_mode === 'always' ? t('windowKeeper.policy.windowModes.alwaysShort') : t('windowKeeper.ext.autoResetModeShort') }}
            · {{ settings.poll_seconds || 20 }}s
          </span>
        </div>

        <div class="dash-command-right">
          <button
            type="button"
            class="btn"
            :disabled="loading || actionPending || !ready || !accounts.length"
            @click="probeAllAccounts"
          >
            {{ batchProbing ? t('windowKeeper.ext.batchProbing') : t('windowKeeper.ext.batchProbe') }}
          </button>
          <button
            type="button"
            :class="['btn', 'dash-filter-toggle', { active: policyOpen }]"
            @click="policyOpen = !policyOpen"
          >
            {{ policyOpen ? t('windowKeeper.policy.collapse') : t('windowKeeper.policy.title') }}
          </button>
          <button type="button" class="btn primary" :disabled="loading || !ready" @click="refreshAll">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
        </div>
      </div>

      <!-- Global Policy Form Drawer -->
      <form v-if="policyOpen" class="window-keeper-policy-form" @submit.prevent="savePolicy">
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.model') }}</span>
            <input v-model.trim="form.model" class="control" required />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.effort') }}</span>
            <select v-model="form.effort" class="control">
              <option value="none">{{ t('windowKeeper.policy.efforts.none') }}</option>
              <option v-for="opt in ['minimal', 'low', 'medium', 'high', 'xhigh', 'max']" :key="opt" :value="opt">{{ opt }}</option>
            </select>
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.windowMode') }}</span>
            <select v-model="form.window_mode" class="control">
              <option value="auto">{{ t('windowKeeper.policy.windowModes.auto') }}</option>
              <option value="always">{{ t('windowKeeper.policy.windowModes.always') }}</option>
            </select>
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.serviceTier') }}</span>
            <select v-model="form.service_tier" class="control">
              <option value="">{{ t('common.none') }}</option>
              <option value="default">default</option>
              <option value="standard">standard</option>
              <option value="flex">flex</option>
            </select>
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.includeAdditional') }}</span>
            <select v-model="form.include_additional" class="control">
              <option value="fill_gaps">{{ t('windowKeeper.policy.additionalModes.fill_gaps') }}</option>
              <option value="all">{{ t('windowKeeper.policy.additionalModes.all') }}</option>
              <option value="none">{{ t('windowKeeper.policy.additionalModes.none') }}</option>
            </select>
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.pollInterval') }}</span>
            <input v-model.number="form.poll_seconds" type="number" min="15" max="300" class="control" required />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.skew') }}</span>
            <input v-model.number="form.skew_seconds" type="number" min="0" max="60" class="control" required />
          </label>
          <label class="config-field" style="grid-column: 1 / -1">
            <span class="config-field-label">{{ t('windowKeeper.policy.prompt') }}</span>
            <input v-model.trim="form.prompt" class="control" required />
          </label>
        </div>
        <div class="config-actions-bar">
          <button type="submit" class="btn primary" :disabled="saving || !ready">{{ t('windowKeeper.policy.save') }}</button>
        </div>
      </form>
    </div>

    <div v-if="!hasManagementKey" class="notice warn">{{ t('windowKeeper.missingKey') }}</div>
    <div v-if="noticeMessage" class="notice">{{ noticeMessage }}</div>
    <div v-if="error" class="notice error">{{ error }}</div>

    <!-- ====== 4-Card High-Density KPI Strip ====== -->
    <section class="dash-kpi-strip">
      <article class="dash-kpi-card clickable" @click="accountFilter = 'all'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('windowKeeper.table.account') }}</span>
          <span class="dash-kpi-chip">{{ t('windowKeeper.ext.healthySub', { count: healthyAccountCount }) }}</span>
        </div>
        <div class="dash-kpi-value">{{ accounts.length }}</div>
        <div class="dash-kpi-foot">
          <span class="good-text">{{ t('windowKeeper.ext.activeSub', { count: activeKeeperAccountCount }) }}</span>
          <span class="dash-kpi-sep">·</span>
          <span>{{ t('windowKeeper.ext.disabledOverrideSub', { count: disabledOverrideCount }) }}</span>
        </div>
      </article>

      <article class="dash-kpi-card clickable" @click="accountFilter = 'active'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('windowKeeper.ext.anchoredCardTitle') }}</span>
          <span class="dash-kpi-chip">{{ t('windowKeeper.ext.anchoredSub', { count: anchoredWindowCount }) }}</span>
        </div>
        <div class="dash-kpi-value good-text">{{ clearOrAnchoredAccountCount }}</div>
        <div class="dash-kpi-foot">
          <span>{{ t('windowKeeper.ext.anchoredFoot') }}</span>
        </div>
      </article>

      <article class="dash-kpi-card clickable" @click="accountFilter = blockedOrDueAccountCount > 0 ? 'blocked' : 'all'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('windowKeeper.ext.blockedCardTitle') }}</span>
          <span class="dash-kpi-chip">{{ t('windowKeeper.ext.dueSub', { count: dueWindowCount }) }}</span>
        </div>
        <div :class="['dash-kpi-value', blockedOrDueAccountCount > 0 ? 'warn-text' : '']">
          {{ blockedOrDueAccountCount }}
        </div>
        <div class="dash-kpi-foot">
          <span>{{ t('windowKeeper.ext.blockedFoot') }}</span>
        </div>
      </article>

      <article class="dash-kpi-card clickable" @click="accountFilter = pausedAccountCount > 0 ? 'paused' : 'all'">
        <div class="dash-kpi-head">
          <span class="dash-kpi-label">{{ t('windowKeeper.attempts.title') }}</span>
          <span class="dash-kpi-chip">{{ t('windowKeeper.ext.pausedSub', { count: pausedAccountCount }) }}</span>
        </div>
        <div class="dash-kpi-value">{{ attempts.length }}</div>
        <div class="dash-kpi-foot">
          <span class="good-text">{{ t('windowKeeper.ext.succeededSub', { count: succeededAttemptCount }) }}</span>
          <span class="dash-kpi-sep">·</span>
          <span :class="failedAttemptCount > 0 ? 'bad-text' : 'muted'">{{ t('windowKeeper.ext.failedSub', { count: failedAttemptCount }) }}</span>
        </div>
      </article>
    </section>

    <!-- ====== Main Master-Detail Layout ====== -->
    <div class="window-keeper-layout wk-responsive-layout">
      <!-- Left: Account Window Cards -->
      <div class="card wk-accounts-shell">
        <div class="mp-toolbar">
          <div class="segmented-control mp-filter-pills">
            <button
              v-for="tab in accountFilterTabs"
              :key="tab.key"
              type="button"
              :class="['segment-btn', { active: accountFilter === tab.key }]"
              @click="accountFilter = tab.key"
            >
              {{ tab.label }}
              <span class="segment-count">{{ tab.count }}</span>
            </button>
          </div>
          <input
            v-model.trim="accountSearch"
            class="control compact mp-search-input"
            :placeholder="t('windowKeeper.ext.searchAccounts')"
          />
        </div>

        <div v-if="filteredAccounts.length === 0" class="empty">
          {{ accounts.length === 0 ? t('windowKeeper.noAccounts') : t('common.noData') }}
        </div>

        <div v-else class="wk-account-cards">
          <article
            v-for="acc in filteredAccounts"
            :key="acc.auth_id"
            :class="['wk-account-card', 'clickable', { selected: selectedAuthId === acc.auth_id }]"
            @click="selectAccount(acc)"
          >
            <div class="wk-acc-top">
              <div class="wk-acc-identity">
                <strong class="wk-acc-email" :title="acc.email || acc.display_name || acc.auth_id">
                  {{ acc.email || acc.display_name || acc.auth_id }}
                </strong>
                <span v-if="acc.plan_type" class="plan-pill">{{ acc.plan_type }}</span>
                <span :class="['status-badge', accountEffectiveTone(acc)]">
                  <i aria-hidden="true"></i>{{ accountEffectiveLabel(acc) }}
                </span>
              </div>

              <div class="wk-acc-quick-actions" @click.stop>
                <button
                  type="button"
                  class="btn btn-xs"
                  :disabled="actionPending || !ready"
                  @click="probeAccount(acc.auth_id)"
                >
                  {{ t('windowKeeper.actions.probe') }}
                </button>
                <button
                  type="button"
                  class="btn btn-xs primary"
                  :disabled="actionPending || !ready || acc.disabled || acc.unavailable"
                  @click="activateAccount(acc.auth_id)"
                >
                  {{ t('windowKeeper.actions.activate') }}
                </button>
                <button
                  v-if="acc.pause_reason"
                  type="button"
                  class="btn btn-xs"
                  :disabled="actionPending || !ready"
                  @click="resumeAccount(acc.auth_id)"
                >
                  {{ t('windowKeeper.actions.resume') }}
                </button>
              </div>
            </div>

            <!-- Windows Progress Bars -->
            <div v-if="visibleWindows(acc.windows).length" class="wk-win-bars">
              <div
                v-for="w in visibleWindows(acc.windows)"
                :key="w.limit_id + '|' + w.slot"
                class="wk-win-row"
              >
                <div class="wk-win-head">
                  <div class="wk-win-title">
                    <strong>{{ formatKind(w.kind, w.period_seconds) }}</strong>
                    <span :class="['wk-phase-tag', phaseClass(w.phase)]">{{ formatPhaseLabel(w.phase) }}</span>
                  </div>
                  <div class="wk-win-meta mono-cell">
                    <span>已用 <b>{{ Math.round(Number(w.used_percent || 0)) }}%</b> ({{ formatWindowRemaining(w) }})</span>
                    <span v-if="w.ends_at" class="muted"> · 重置 {{ formatShortReset(w.ends_at) }}</span>
                  </div>
                </div>
                <div class="wk-progress-track">
                  <i
                    :class="['wk-progress-fill', progressColorClass(w.used_percent, w.phase)]"
                    :style="{ width: Math.min(100, Math.max(2, Math.round(Number(w.used_percent || 0)))) + '%' }"
                  ></i>
                </div>
              </div>
            </div>
            <div v-else class="wk-no-win muted small-text">
              尚未读取窗口快照 — 点击右侧「{{ t('windowKeeper.actions.probe') }}」立即获取 5h / 周限额窗口
            </div>

            <div class="wk-acc-foot muted small-text">
              <span>
                策略: <b>{{ formatOverrideSummary(acc) }}</b>
              </span>
              <span v-if="acc.not_before && !isZeroTime(acc.not_before)">
                · 下次检查: {{ formatTimeOnly(acc.not_before) }}
              </span>
              <span v-if="acc.shape_mismatch" class="warn-text">
                · 窗口形态提示: {{ acc.shape_mismatch }}
              </span>
            </div>
          </article>
        </div>
      </div>

      <!-- Right: Selected Account Override, Window Clocks & Attempt History -->
      <div class="window-keeper-side">
        <DataCard
          v-if="selectedAccount"
          :title="selectedAccount.email || selectedAccount.display_name || selectedAccount.auth_id"
          :subtitle="selectedAccount.plan_type ? selectedAccount.plan_type.toUpperCase() : ''"
        >
          <div class="window-keeper-detail">
            <!-- 1. Per-Account Policy Override Box -->
            <div class="wk-override-box">
              <div class="wk-override-head">
                <strong>{{ t('windowKeeper.ext.overrideTitle') }}</strong>
                <span class="muted small-text">{{ t('windowKeeper.ext.overrideSub') }}</span>
              </div>

              <div class="wk-override-controls">
                <div class="segmented-control dash-mini-seg">
                  <button
                    v-for="opt in overrideModeOptions"
                    :key="opt.value"
                    type="button"
                    :class="['segment-btn', { active: overrideDraft.enabled === opt.value }]"
                    @click="overrideDraft.enabled = opt.value"
                  >
                    {{ opt.label }}
                  </button>
                </div>

                <div class="wk-override-model-row">
                  <input
                    v-model.trim="overrideDraft.model"
                    class="control compact"
                    :placeholder="t('windowKeeper.ext.overrideModelPlaceholder', { model: settings.model || 'gpt-5.4' })"
                  />
                  <button
                    type="button"
                    class="btn btn-xs primary"
                    :disabled="actionPending || !ready || !overrideDirty"
                    @click="saveAccountOverride(selectedAccount.auth_id)"
                  >
                    {{ t('windowKeeper.ext.saveOverride') }}
                  </button>
                </div>
              </div>
            </div>

            <!-- 2. Detailed Window Clocks -->
            <div v-if="visibleWindows(selectedAccount.windows).length > 0" class="window-cards-stack">
              <div v-for="w in visibleWindows(selectedAccount.windows)" :key="w.limit_id + '|' + w.slot" class="window-detail-card">
                <div class="window-detail-header">
                  <strong>{{ formatKind(w.kind, w.period_seconds) }} <span v-if="w.limit_id !== 'codex'" class="muted">({{ w.limit_id }})</span></strong>
                  <span :class="['window-chip', phaseClass(w.phase)]">{{ formatPhaseLabel(w.phase) }} · {{ formatWindowRemaining(w) }}</span>
                </div>
                <div class="window-detail-meta">
                  <div>
                    <span class="muted">{{ t('windowKeeper.windows.startsAt') }}:</span>
                    <span class="mono-cell">{{ formatTime(w.starts_at) }}</span>
                  </div>
                  <div>
                    <span class="muted">{{ t('windowKeeper.windows.endsAt') }}:</span>
                    <span class="mono-cell">{{ formatTime(w.ends_at) }}</span>
                  </div>
                  <div v-if="w.time_source">
                    <span class="muted">{{ t('windowKeeper.windows.timeSource') }}:</span>
                    <span>{{ w.time_source }}</span>
                  </div>
                  <div v-if="!w.gating">
                    <span class="muted">{{ t('windowKeeper.windows.nonGating') }}</span>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="empty">{{ t('common.noData') }}</div>

            <!-- 3. Actions Bar -->
            <div class="config-actions-bar">
              <button
                type="button"
                class="btn"
                :disabled="actionPending || !ready"
                @click="probeAccount(selectedAccount.auth_id)"
              >
                {{ t('windowKeeper.actions.probe') }}
              </button>
              <button
                type="button"
                class="btn primary"
                :disabled="actionPending || !ready || selectedAccount.disabled || selectedAccount.unavailable"
                @click="activateAccount(selectedAccount.auth_id)"
              >
                {{ t('windowKeeper.actions.activate') }}
              </button>
              <button
                v-if="selectedAccount.pause_reason"
                type="button"
                class="btn"
                :disabled="actionPending || !ready"
                @click="resumeAccount(selectedAccount.auth_id)"
              >
                {{ t('windowKeeper.actions.resume') }}
              </button>
            </div>
          </div>
        </DataCard>
        <DataCard v-else :title="t('windowKeeper.table.windows')">
          <div class="empty">{{ t('windowKeeper.selectAccount') }}</div>
        </DataCard>

        <!-- Recent Attempts Card -->
        <DataCard :title="t('windowKeeper.attempts.title')">
          <div v-if="attempts.length > 0 && selectedAccount" class="wk-attempts-filter">
            <div class="segmented-control dash-mini-seg">
              <button
                type="button"
                :class="['segment-btn', { active: attemptScope === 'all' }]"
                @click="attemptScope = 'all'"
              >
                {{ t('windowKeeper.ext.allAccountsAttempts', { count: attempts.length }) }}
              </button>
              <button
                type="button"
                :class="['segment-btn', { active: attemptScope === 'selected' }]"
                @click="attemptScope = 'selected'"
              >
                {{ t('windowKeeper.ext.selectedAccountAttempts', { count: selectedAccountAttempts.length }) }}
              </button>
            </div>
          </div>

          <div v-if="displayedAttempts.length === 0" class="empty">{{ t('windowKeeper.attempts.empty') }}</div>
          <div v-else class="attempts-list">
            <div v-for="att in displayedAttempts" :key="att.id" class="attempt-item">
              <div class="attempt-main">
                <span :class="['status-badge', att.status === 'succeeded' ? 'good' : att.status === 'failed' ? 'bad' : 'warn']">
                  <i aria-hidden="true"></i>{{ att.status }}
                </span>
                <strong class="attempt-account">{{ findEmail(att.account_id) }}</strong>
                <span class="muted small-text mono-cell">{{ formatTime(att.started_at) }}</span>
              </div>
              <div v-if="att.error_kind || att.http_status || att.output_excerpt || attemptHasHttpDetail(att)" class="attempt-sub">
                <span v-if="att.error_kind || att.http_status" class="bad-text small-text mono-cell">
                  {{ attemptErrorMeta(att) }}
                </span>
                <span v-if="att.output_excerpt" class="muted small-text mono-cell">{{ formatAttemptText(att.output_excerpt) }}</span>
                <details v-if="attemptHasHttpDetail(att)" class="attempt-http-details">
                  <summary class="small-text muted">{{ t('windowKeeper.attempts.detail') }}</summary>
                  <div v-if="att.req_headers" class="attempt-http-block">
                    <div class="small-text muted">{{ t('windowKeeper.attempts.reqHeaders') }}</div>
                    <pre class="mono-cell">{{ formatAttemptText(att.req_headers) }}</pre>
                  </div>
                  <div v-if="att.req_body" class="attempt-http-block">
                    <div class="small-text muted">{{ t('windowKeeper.attempts.reqBody') }}</div>
                    <pre class="mono-cell">{{ formatAttemptText(att.req_body) }}</pre>
                  </div>
                  <div v-if="att.resp_headers" class="attempt-http-block">
                    <div class="small-text muted">{{ t('windowKeeper.attempts.respHeaders') }}</div>
                    <pre class="mono-cell">{{ formatAttemptText(att.resp_headers) }}</pre>
                  </div>
                  <div v-if="att.resp_body" class="attempt-http-block">
                    <div class="small-text muted">{{ t('windowKeeper.attempts.respBody') }}</div>
                    <pre class="mono-cell">{{ formatAttemptText(att.resp_body) }}</pre>
                  </div>
                </details>
              </div>
            </div>
          </div>
        </DataCard>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import DataCard from './DataCard.vue';
import { formatAttemptText, formatWindowRemainingText } from '../utils/windowKeeperDisplay.js';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t } = useI18n();

const loading = ref(false);
const saving = ref(false);
const actionPending = ref(false);
const batchProbing = ref(false);
const policyOpen = ref(false);
const noticeMessage = ref('');
const error = ref('');
const hasManagementKey = ref(true);

const settings = reactive({
  enabled: false,
  model: 'gpt-5.4',
  effort: 'none',
  window_mode: 'auto',
  service_tier: '',
  prompt: 'Reply with OK only.',
  include_additional: 'fill_gaps',
  poll_seconds: 20,
  skew_seconds: 3,
});

const form = reactive({ ...settings });
const accounts = ref([]);
const attempts = ref([]);
const selectedAuthId = ref('');
const accountFilter = ref('all');
const accountSearch = ref('');
const attemptScope = ref('all');

const overrideDraft = reactive({
  enabled: 'inherit',
  model: '',
});

const overrideModeOptions = computed(() => [
  { value: 'inherit', label: t('windowKeeper.ext.overrideInherit') },
  { value: 'on', label: t('windowKeeper.ext.overrideOn') },
  { value: 'off', label: t('windowKeeper.ext.overrideOff') },
]);

function parseAccountOverride(acc) {
  if (!acc?.override_json) return { enabled: 'inherit', model: '' };
  try {
    const parsed = typeof acc.override_json === 'string' ? JSON.parse(acc.override_json) : acc.override_json;
    const en = ['inherit', 'on', 'off'].includes(parsed?.enabled) ? parsed.enabled : 'inherit';
    return { enabled: en, model: String(parsed?.model || '').trim() };
  } catch {
    return { enabled: 'inherit', model: '' };
  }
}

const selectedAccount = computed(() => {
  return accounts.value.find(a => a.auth_id === selectedAuthId.value) || null;
});

watch(selectedAccount, (acc) => {
  const ov = parseAccountOverride(acc);
  overrideDraft.enabled = ov.enabled;
  overrideDraft.model = ov.model;
}, { immediate: true });

const overrideDirty = computed(() => {
  if (!selectedAccount.value) return false;
  const ov = parseAccountOverride(selectedAccount.value);
  return ov.enabled !== overrideDraft.enabled || ov.model !== overrideDraft.model.trim();
});

function isAccountKeeperActive(acc) {
  if (!acc || acc.disabled || acc.unavailable || acc.pause_reason) return false;
  const ov = parseAccountOverride(acc);
  if (ov.enabled === 'on') return true;
  if (ov.enabled === 'off') return false;
  return Boolean(settings.enabled);
}

function isAccountBlockedOrDue(acc) {
  return visibleWindows(acc?.windows).some(w => w.phase === 'blocked' || w.phase === 'due');
}

const healthyAccountCount = computed(() => accounts.value.filter(a => !a.disabled && !a.unavailable).length);
const activeKeeperAccountCount = computed(() => accounts.value.filter(isAccountKeeperActive).length);
const disabledOverrideCount = computed(() => accounts.value.filter(a => parseAccountOverride(a).enabled === 'off').length);
const pausedAccountCount = computed(() => accounts.value.filter(a => Boolean(a.pause_reason) || a.disabled || a.unavailable).length);
const blockedOrDueAccountCount = computed(() => accounts.value.filter(isAccountBlockedOrDue).length);
const clearOrAnchoredAccountCount = computed(() =>
  accounts.value.filter(a => visibleWindows(a.windows).some(w => w.phase === 'anchored' || w.phase === 'clear' || w.phase === 'baseline')).length,
);
const anchoredWindowCount = computed(() =>
  accounts.value.reduce((sum, a) => sum + visibleWindows(a.windows).filter(w => w.phase === 'anchored').length, 0),
);
const dueWindowCount = computed(() =>
  accounts.value.reduce((sum, a) => sum + visibleWindows(a.windows).filter(w => w.phase === 'due').length, 0),
);
const succeededAttemptCount = computed(() => attempts.value.filter(a => a.status === 'succeeded').length);
const failedAttemptCount = computed(() => attempts.value.filter(a => a.status === 'failed').length);

const accountFilterTabs = computed(() => [
  { key: 'all', label: t('windowKeeper.ext.filterAll'), count: accounts.value.length },
  { key: 'active', label: t('windowKeeper.ext.filterActive'), count: activeKeeperAccountCount.value },
  { key: 'blocked', label: t('windowKeeper.ext.filterBlocked'), count: blockedOrDueAccountCount.value },
  { key: 'paused', label: t('windowKeeper.ext.filterPaused'), count: pausedAccountCount.value },
]);

const filteredAccounts = computed(() => {
  const q = accountSearch.value.trim().toLowerCase();
  return accounts.value.filter(acc => {
    if (accountFilter.value === 'active' && !isAccountKeeperActive(acc)) return false;
    if (accountFilter.value === 'blocked' && !isAccountBlockedOrDue(acc)) return false;
    if (accountFilter.value === 'paused' && !(acc.pause_reason || acc.disabled || acc.unavailable)) return false;
    if (!q) return true;
    return (
      String(acc.email || '').toLowerCase().includes(q)
      || String(acc.display_name || '').toLowerCase().includes(q)
      || String(acc.auth_id || '').toLowerCase().includes(q)
      || String(acc.plan_type || '').toLowerCase().includes(q)
    );
  });
});

const selectedAccountAttempts = computed(() => {
  if (!selectedAccount.value) return [];
  return attempts.value.filter(a => a.account_id === selectedAccount.value.auth_id);
});

const displayedAttempts = computed(() => {
  const list = (attemptScope.value === 'selected' && selectedAccount.value)
    ? selectedAccountAttempts.value
    : attempts.value;
  return list.slice(0, 20);
});

async function callApi(method, path, body) {
  return await props.proxyCall({ method, path, body });
}

async function loadSettings() {
  const res = await callApi('GET', '/v0/management/window-keeper/settings');
  if (res && res.settings) {
    Object.assign(settings, res.settings);
    Object.assign(form, res.settings);
  }
  if (res && typeof res.management_key_set === 'boolean') {
    hasManagementKey.value = res.management_key_set;
  }
}

async function loadAccounts() {
  const res = await callApi('GET', '/v0/management/window-keeper/accounts');
  accounts.value = (res && Array.isArray(res.accounts)) ? res.accounts : [];
  if (!selectedAuthId.value && accounts.value.length > 0) {
    selectedAuthId.value = accounts.value[0].auth_id;
  } else if (selectedAuthId.value && !accounts.value.some(a => a.auth_id === selectedAuthId.value)) {
    selectedAuthId.value = accounts.value[0]?.auth_id || '';
  }
}

async function loadAttempts() {
  const res = await callApi('GET', '/v0/management/window-keeper/attempts');
  attempts.value = (res && Array.isArray(res.attempts)) ? res.attempts : [];
}

async function refreshAll() {
  loading.value = true;
  error.value = '';
  try {
    await Promise.all([loadSettings(), loadAccounts(), loadAttempts()]);
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    loading.value = false;
  }
}

async function toggleGlobalSwitch() {
  saving.value = true;
  error.value = '';
  noticeMessage.value = '';
  const nextState = !settings.enabled;
  try {
    const payload = { ...settings, enabled: nextState };
    await callApi('PUT', '/v0/management/window-keeper/settings', payload);
    settings.enabled = nextState;
    form.enabled = nextState;
    await loadAccounts();
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    saving.value = false;
  }
}

async function savePolicy() {
  saving.value = true;
  error.value = '';
  noticeMessage.value = '';
  try {
    const payload = { ...form, enabled: settings.enabled };
    const res = await callApi('PUT', '/v0/management/window-keeper/settings', payload);
    if (res && res.settings) {
      Object.assign(settings, res.settings);
      Object.assign(form, res.settings);
    }
    noticeMessage.value = t('windowKeeper.policy.saved');
    policyOpen.value = false;
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    saving.value = false;
  }
}

async function saveAccountOverride(authId) {
  if (!authId) return;
  actionPending.value = true;
  error.value = '';
  noticeMessage.value = '';
  try {
    await callApi('PUT', '/v0/management/window-keeper/accounts/override', {
      auth_id: authId,
      enabled: overrideDraft.enabled,
      model: overrideDraft.model.trim(),
    });
    await loadAccounts();
    noticeMessage.value = '单账号保活策略已保存';
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

async function probeAccount(authId) {
  actionPending.value = true;
  error.value = '';
  noticeMessage.value = '';
  try {
    await callApi('POST', `/v0/management/window-keeper/accounts/${encodeURIComponent(authId)}/probe`, { auth_id: authId });
    await loadAccounts();
    noticeMessage.value = t('windowKeeper.actions.probed');
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

async function probeAllAccounts() {
  const targets = accounts.value.filter(a => !a.disabled && !a.unavailable);
  if (!targets.length) return;
  batchProbing.value = true;
  actionPending.value = true;
  error.value = '';
  noticeMessage.value = '';
  try {
    let okCount = 0;
    for (const acc of targets) {
      try {
        await callApi('POST', `/v0/management/window-keeper/accounts/${encodeURIComponent(acc.auth_id)}/probe`, { auth_id: acc.auth_id });
        okCount += 1;
      } catch {
        // continue probing remaining accounts
      }
    }
    await loadAccounts();
    noticeMessage.value = `已完成批量读取（成功 ${okCount}/${targets.length} 个账号）`;
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    batchProbing.value = false;
    actionPending.value = false;
  }
}

async function activateAccount(authId) {
  actionPending.value = true;
  error.value = '';
  noticeMessage.value = '';
  try {
    await callApi('POST', `/v0/management/window-keeper/accounts/${encodeURIComponent(authId)}/activate`, { auth_id: authId });
    await Promise.all([loadAccounts(), loadAttempts()]);
    noticeMessage.value = t('windowKeeper.actions.activated');
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

async function resumeAccount(authId) {
  actionPending.value = true;
  error.value = '';
  noticeMessage.value = '';
  try {
    await callApi('POST', `/v0/management/window-keeper/accounts/${encodeURIComponent(authId)}/resume`, { auth_id: authId });
    await loadAccounts();
    noticeMessage.value = t('windowKeeper.actions.resumed');
  } catch (err) {
    error.value = err?.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

function selectAccount(acc) {
  selectedAuthId.value = acc.auth_id;
}

function visibleWindows(windows) {
  if (!Array.isArray(windows)) return [];
  return windows.filter(w => !w.absent);
}

function formatKind(kind, periodSeconds) {
  if (kind === 'five_hour') return t('windowKeeper.windows.five_hour');
  if (kind === 'weekly') return t('windowKeeper.windows.weekly');
  if (kind === 'monthly') return t('windowKeeper.windows.monthly');
  if (periodSeconds) {
    const hours = Math.round(periodSeconds / 3600);
    return hours >= 24 ? `${Math.round(hours / 24)}d` : `${hours}h`;
  }
  return kind || t('windowKeeper.windows.custom');
}

function formatPhaseLabel(phase) {
  switch (phase) {
    case 'anchored': return t('windowKeeper.ext.phaseAnchored');
    case 'clear': return t('windowKeeper.ext.phaseClear');
    case 'baseline': return t('windowKeeper.ext.phaseBaseline');
    case 'due': return t('windowKeeper.ext.phaseDue');
    case 'blocked': return t('windowKeeper.ext.phaseBlocked');
    case 'fixed': return t('windowKeeper.ext.phaseFixed');
    case 'inconclusive': return t('windowKeeper.ext.phaseInconclusive');
    case 'stopped': return t('windowKeeper.ext.phaseStopped');
    default: return phase || '—';
  }
}

function formatEffort(effort) {
  if (!effort || effort === 'none') {
    return t('windowKeeper.policy.efforts.noneShort');
  }
  return effort;
}

function formatWindowRemaining(w) {
  if (!w) return '';
  const label = formatKind(w.kind, w.period_seconds);
  if (w.phase === 'blocked') {
    return `${label} ${t('windowKeeper.windows.full')}`;
  }
  return formatWindowRemainingText(label, w.used_percent, t('windowKeeper.windows.remainingWord'));
}

function attemptErrorMeta(att) {
  const parts = [];
  if (att?.error_kind) parts.push(att.error_kind);
  if (Number(att?.http_status) > 0) parts.push(t('windowKeeper.attempts.httpStatus', { status: att.http_status }));
  return parts.join(' · ');
}

function attemptHasHttpDetail(att) {
  return Boolean(att && (att.req_headers || att.req_body || att.resp_headers || att.resp_body));
}

function phaseClass(phase) {
  switch (phase) {
    case 'blocked': return 'phase-blocked';
    case 'due': return 'phase-due';
    case 'anchored': return 'phase-anchored';
    case 'clear':
    case 'baseline': return 'phase-clear';
    case 'inconclusive':
    case 'stopped': return 'phase-warn';
    default: return 'phase-default';
  }
}

function progressColorClass(usedPercent, phase) {
  const pct = Number(usedPercent || 0);
  if (phase === 'blocked' || pct >= 100) return 'is-danger';
  if (pct >= 80) return 'is-warn';
  if (phase === 'anchored') return 'is-good';
  return 'is-primary';
}

function accountEffectiveTone(acc) {
  if (acc.disabled || acc.unavailable) return 'bad';
  if (acc.pause_reason) return 'warn';
  if (isAccountKeeperActive(acc)) return 'good';
  return 'idle';
}

function accountEffectiveLabel(acc) {
  if (acc.disabled) return t('common.disabled');
  if (acc.unavailable) return '不可用';
  if (acc.pause_reason === 'reauth') return '需重新授权';
  if (acc.pause_reason === 'plan') return '套餐受限暂停';
  if (acc.pause_reason) return acc.pause_reason;
  const ov = parseAccountOverride(acc);
  if (ov.enabled === 'on') return '单账号开启';
  if (ov.enabled === 'off') return '单独关闭';
  return settings.enabled ? '保活监控中' : '待机';
}

function formatOverrideSummary(acc) {
  const ov = parseAccountOverride(acc);
  const modeLabel = ov.enabled === 'on' ? '强制开启' : ov.enabled === 'off' ? '单独关闭' : '跟随全局';
  const modelLabel = ov.model || settings.model || 'gpt-5.4';
  return `${modeLabel} · ${modelLabel}`;
}

function isZeroTime(isoStr) {
  if (!isoStr) return true;
  return String(isoStr).startsWith('0001-01-01') || String(isoStr).startsWith('1970-01-01');
}

function formatTime(isoStr) {
  if (!isoStr || isZeroTime(isoStr)) return '—';
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return '—';
    return d.toLocaleString();
  } catch {
    return String(isoStr);
  }
}

function formatTimeOnly(isoStr) {
  if (!isoStr || isZeroTime(isoStr)) return '—';
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return '—';
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  } catch {
    return String(isoStr);
  }
}

function formatShortReset(isoStr) {
  if (!isoStr || isZeroTime(isoStr)) return '—';
  try {
    const d = new Date(isoStr);
    if (isNaN(d.getTime())) return '—';
    const diffMs = d.getTime() - Date.now();
    if (diffMs > 0 && diffMs < 86400000) {
      const h = Math.floor(diffMs / 3600000);
      const m = Math.floor((diffMs % 3600000) / 60000);
      return h > 0 ? `${h}h ${m}m 后` : `${m}m 后`;
    }
    return d.toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' });
  } catch {
    return String(isoStr);
  }
}

function findEmail(authId) {
  const acc = accounts.value.find(a => a.auth_id === authId);
  return acc ? (acc.email || acc.display_name || acc.auth_id) : authId;
}

watch(
  () => props.ready,
  (ready) => {
    if (ready) refreshAll();
  },
);

onMounted(() => {
  if (props.ready) {
    refreshAll();
  }
});

defineExpose({ refresh: refreshAll });
</script>