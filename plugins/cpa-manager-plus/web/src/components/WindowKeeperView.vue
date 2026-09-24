<template>
  <section class="monitoring-page window-keeper-page">
    <div class="card window-keeper-status-card">
      <div class="window-keeper-status-bar">
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
            <template v-if="settings.window_mode === 'always'"> · {{ t('windowKeeper.policy.windowModes.alwaysShort') }}</template>
          </span>
        </div>
        <div class="config-actions-bar" style="padding:0">
          <button class="btn" @click="refreshAll" :disabled="loading || !ready">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
          <button class="btn" @click="policyOpen = !policyOpen">
            {{ policyOpen ? t('windowKeeper.policy.collapse') : t('windowKeeper.policy.adjust') }}
          </button>
        </div>
      </div>

      <!-- Policy Form Drawer -->
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
            <span class="config-field-label">{{ t('windowKeeper.policy.prompt') }}</span>
            <input v-model="form.prompt" class="control" required maxlength="500" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.pollInterval') }}</span>
            <input v-model.number="form.poll_seconds" type="number" min="5" max="600" class="control" />
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ t('windowKeeper.policy.skew') }}</span>
            <input v-model.number="form.skew_seconds" type="number" min="0" max="120" class="control" />
          </label>
        </div>
        <div class="config-actions-bar" style="padding-top:12px">
          <button class="btn primary" type="submit" :disabled="saving">
            {{ saving ? t('common.loading') : t('windowKeeper.policy.save') }}
          </button>
        </div>
      </form>
    </div>

    <!-- Alert banner if management key missing -->
    <section v-if="!hasManagementKey" class="notice warn" style="margin-bottom:12px">
      {{ t('windowKeeper.missingKey') }}
    </section>
    <section v-if="noticeMessage" class="notice config-save-ok" style="margin-bottom:12px">
      {{ noticeMessage }}
    </section>
    <section v-if="error" class="notice error" style="margin-bottom:12px">
      {{ error }}
    </section>

    <!-- Main Content: Ledger + Detail -->
    <div class="window-keeper-layout">
      <!-- Left: Accounts Table -->
      <DataCard :title="t('windowKeeper.table.account')" :subtitle="t('windowKeeper.subtitle')">
        <div class="table-shell" style="border:none">
          <table class="data-table">
            <thead>
              <tr>
                <th>{{ t('windowKeeper.table.account') }}</th>
                <th>{{ t('windowKeeper.table.windows') }}</th>
                <th>{{ t('windowKeeper.table.comparison') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="acc in accounts"
                :key="acc.auth_id"
                :class="['table-row-clickable', { selected: selectedAuthId === acc.auth_id }]"
                @click="selectedAuthId = acc.auth_id"
              >
                <td>
                  <strong>{{ acc.email || acc.name || acc.auth_id }}</strong>
                  <div class="sub muted small-text">
                    {{ acc.plan_type || '—' }}
                    <span v-if="acc.pause_reason" class="status-badge error" style="margin-left:6px;font-size:11px">
                      {{ acc.pause_reason }}
                    </span>
                  </div>
                </td>
                <td>
                  <div class="window-pills-row">
                    <span
                      v-for="w in activeWindows(acc)"
                      :key="w.limit_id + w.slot + w.period_seconds"
                      :class="['window-pill', { warn: isWindowBlocking(w), dim: !w.gating }]"
                    >
                      {{ formatKindLabel(w.kind) }} {{ isWindowBlocking(w) ? t('windowKeeper.windows.full') : Math.round(w.used_percent) + '%' }}
                    </span>
                  </div>
                </td>
                <td class="muted small-text">
                  <span v-if="acc.shape_mismatch" class="status-badge warn" style="font-size:11px">
                    {{ acc.shape_mismatch }}
                  </span>
                  <span v-else>{{ acc.plan_group || '—' }}</span>
                </td>
              </tr>
              <tr v-if="!accounts.length">
                <td colspan="3" class="muted small-text" style="text-align:center;padding:24px">
                  {{ t('windowKeeper.noAccounts') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </DataCard>

      <!-- Right: Selected Account Inspector -->
      <div class="window-keeper-detail-col">
        <DataCard v-if="selectedAccount" :title="selectedAccount.email || selectedAccount.auth_id" :subtitle="selectedAccount.plan_type || ''">
          <div class="account-decision-box">
            <div class="spread-row">
              <strong>{{ selectedAccount.name || selectedAccount.auth_id }}</strong>
              <span v-if="selectedAccount.pause_reason" class="status-badge error">
                {{ selectedAccount.pause_reason }}
              </span>
            </div>
            <span v-if="selectedAccount.shape_mismatch" class="muted small-text">
              {{ selectedAccount.shape_mismatch }}
            </span>
          </div>

          <!-- Window Timing Cards -->
          <div class="window-timing-cards">
            <article
              v-for="w in activeWindows(selectedAccount)"
              :key="w.limit_id + w.slot + w.period_seconds"
              :class="['card', 'timing-box', { blocking: isWindowBlocking(w) }]"
            >
              <div class="spread-row">
                <strong>{{ formatKindLabel(w.kind) }}</strong>
                <span :class="['status-badge', isWindowBlocking(w) ? 'warn' : 'good']">
                  {{ w.phase }} {{ w.gating ? '' : '· ' + t('windowKeeper.windows.nonGating') }}
                </span>
              </div>
              <div class="times-grid">
                <div class="time-item">
                  <span class="muted small-text">{{ t('windowKeeper.windows.startsAt') }}</span>
                  <b class="mono">{{ formatTime(w.starts_at) }}</b>
                </div>
                <div class="time-item">
                  <span class="muted small-text">{{ t('windowKeeper.windows.endsAt') }}</span>
                  <b class="mono">{{ formatTime(w.ends_at) }}</b>
                </div>
              </div>
              <div class="muted small-text" style="margin-top:4px">
                {{ t('windowKeeper.windows.timeSource') }}: {{ w.time_source || '—' }} · {{ Math.round(w.used_percent) }}%
              </div>
            </article>
          </div>

          <!-- Action Buttons -->
          <div class="account-actions-row">
            <button
              class="btn"
              :disabled="actionPending || !ready"
              @click="probeAccount(selectedAccount.auth_id)"
            >
              {{ t('windowKeeper.actions.probe') }}
            </button>
            <button
              class="btn primary"
              :disabled="actionPending || !ready || !settings.enabled || !!selectedAccount.pause_reason"
              @click="activateAccount(selectedAccount.auth_id)"
            >
              {{ t('windowKeeper.actions.activate') }}
            </button>
            <button
              v-if="selectedAccount.pause_reason"
              class="btn"
              :disabled="actionPending || !ready"
              @click="resumeAccount(selectedAccount.auth_id)"
            >
              {{ t('windowKeeper.actions.resume') }}
            </button>
          </div>

          <!-- Recent Attempts -->
          <div class="attempts-section">
            <h4>{{ t('windowKeeper.attempts.title') }}</h4>
            <div v-if="accountAttempts.length" class="attempts-list">
              <div
                v-for="att in accountAttempts"
                :key="att.id"
                class="attempt-item"
              >
                <div class="spread-row">
                  <span :class="['status-badge', att.status === 'succeeded' ? 'good' : 'error']">
                    {{ att.status }}
                  </span>
                  <span class="muted small-text mono">{{ formatTime(att.started_at) }}</span>
                </div>
                <div v-if="att.output_excerpt" class="muted small-text mono excerpt">
                  {{ att.output_excerpt }}
                </div>
              </div>
            </div>
            <p v-else class="muted small-text">{{ t('windowKeeper.attempts.empty') }}</p>
          </div>
        </DataCard>
        <div v-else class="card muted small-text" style="text-align:center;padding:48px">
          {{ t('windowKeeper.selectAccount') }}
        </div>
      </div>
    </div>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import DataCard from './DataCard.vue';

const props = defineProps({
  ready: { type: Boolean, default: false },
  proxyCall: { type: Function, required: true },
});

const { t } = useI18n();

const loading = ref(false);
const saving = ref(false);
const actionPending = ref(false);
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
  prompt: 'Reply with exactly OK.',
  include_additional: 'fill_gaps',
  poll_seconds: 20,
  skew_seconds: 3,
});

const form = reactive({
  model: 'gpt-5.4',
  effort: 'none',
  window_mode: 'auto',
  service_tier: '',
  prompt: 'Reply with exactly OK.',
  include_additional: 'fill_gaps',
  poll_seconds: 20,
  skew_seconds: 3,
});

const accounts = ref([]);
const attempts = ref([]);
const selectedAuthId = ref('');

const selectedAccount = computed(() => {
  return accounts.value.find(a => a.auth_id === selectedAuthId.value) || null;
});

const accountAttempts = computed(() => {
  if (!selectedAccount.value) return [];
  const id = selectedAccount.value.auth_id;
  return attempts.value.filter(a => a.account_id === id).slice(0, 8);
});

function activeWindows(account) {
  if (!account || !account.windows) return [];
  return account.windows.filter(w => !w.absent);
}

function isWindowBlocking(window) {
  return window.phase === 'blocked' || window.used_percent >= 100;
}

function formatKindLabel(kind) {
  switch (kind) {
    case 'five_hour': return t('windowKeeper.windows.five_hour');
    case 'weekly': return t('windowKeeper.windows.weekly');
    case 'monthly': return t('windowKeeper.windows.monthly');
    default: return kind || t('windowKeeper.windows.custom');
  }
}

function formatEffort(effort) {
  if (!effort || effort === 'none') {
    return 'none (' + t('windowKeeper.policy.efforts.noneShort') + ')';
  }
  return effort;
}

function formatTime(val) {
  if (!val || String(val).startsWith('0001')) return '—';
  const d = new Date(val);
  if (isNaN(d.getTime())) return '—';
  const pad = n => String(n).padStart(2, '0');
  return pad(d.getMonth() + 1) + '-' + pad(d.getDate()) + ' ' + pad(d.getHours()) + ':' + pad(d.getMinutes());
}

async function refreshAll() {
  if (!props.ready) return;
  loading.value = true;
  error.value = '';
  try {
    const sResp = await props.proxyCall({ method: 'GET', path: '/v0/management/window-keeper/settings' });
    if (sResp?.settings) {
      Object.assign(settings, sResp.settings);
      Object.assign(form, sResp.settings);
    }
    hasManagementKey.value = !!sResp?.management_key_set;

    const aResp = await props.proxyCall({ method: 'GET', path: '/v0/management/window-keeper/accounts' });
    accounts.value = aResp?.accounts || [];
    if (!selectedAuthId.value && accounts.value.length > 0) {
      selectedAuthId.value = accounts.value[0].auth_id;
    }

    const attResp = await props.proxyCall({ method: 'GET', path: '/v0/management/window-keeper/attempts' });
    attempts.value = attResp?.attempts || [];
  } catch (err) {
    error.value = err.message || String(err);
  } finally {
    loading.value = false;
  }
}

async function toggleGlobalSwitch() {
  const next = !settings.enabled;
  saving.value = true;
  error.value = '';
  try {
    const payload = { ...settings, enabled: next };
    await props.proxyCall({ method: 'PUT', path: '/v0/management/window-keeper/settings', body: payload });
    settings.enabled = next;
    form.enabled = next;
    noticeMessage.value = t('windowKeeper.policy.saved');
    setTimeout(() => { noticeMessage.value = ''; }, 3000);
  } catch (err) {
    error.value = err.message || String(err);
  } finally {
    saving.value = false;
  }
}

async function savePolicy() {
  saving.value = true;
  error.value = '';
  try {
    const payload = { ...settings, ...form };
    const res = await props.proxyCall({ method: 'PUT', path: '/v0/management/window-keeper/settings', body: payload });
    if (res?.settings) {
      Object.assign(settings, res.settings);
      Object.assign(form, res.settings);
    }
    policyOpen.value = false;
    noticeMessage.value = t('windowKeeper.policy.saved');
    setTimeout(() => { noticeMessage.value = ''; }, 3000);
    await refreshAll();
  } catch (err) {
    error.value = err.message || String(err);
  } finally {
    saving.value = false;
  }
}

async function probeAccount(authId) {
  if (!authId) return;
  actionPending.value = true;
  error.value = '';
  try {
    await props.proxyCall({ method: 'POST', path: '/v0/management/window-keeper/accounts/' + authId + '/probe' });
    noticeMessage.value = t('windowKeeper.actions.probed');
    setTimeout(() => { noticeMessage.value = ''; }, 3000);
    await refreshAll();
  } catch (err) {
    error.value = err.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

async function activateAccount(authId) {
  if (!authId) return;
  actionPending.value = true;
  error.value = '';
  try {
    await props.proxyCall({ method: 'POST', path: '/v0/management/window-keeper/accounts/' + authId + '/activate' });
    noticeMessage.value = t('windowKeeper.actions.activated');
    setTimeout(() => { noticeMessage.value = ''; }, 3000);
    await refreshAll();
  } catch (err) {
    error.value = err.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

async function resumeAccount(authId) {
  if (!authId) return;
  actionPending.value = true;
  error.value = '';
  try {
    await props.proxyCall({ method: 'POST', path: '/v0/management/window-keeper/accounts/' + authId + '/resume' });
    noticeMessage.value = t('windowKeeper.actions.resumed');
    setTimeout(() => { noticeMessage.value = ''; }, 3000);
    await refreshAll();
  } catch (err) {
    error.value = err.message || String(err);
  } finally {
    actionPending.value = false;
  }
}

onMounted(() => {
  if (props.ready) {
    refreshAll();
  }
});
</script>

<style scoped>
.window-keeper-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.window-keeper-status-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  flex-wrap: wrap;
  gap: 12px;
}

.window-keeper-status-info {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.window-keeper-policy-form {
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid var(--cpa-rule);
}

.window-keeper-layout {
  display: grid;
  grid-template-columns: minmax(420px, 1.3fr) minmax(320px, 0.9fr);
  gap: 16px;
  align-items: start;
}

.table-row-clickable {
  cursor: pointer;
}
.table-row-clickable:hover {
  background: var(--cpa-surface-muted);
}
.table-row-clickable.selected {
  background: color-mix(in srgb, var(--cpa-success) 12%, var(--cpa-surface));
}

.window-pills-row {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.window-pill {
  font-size: 11px;
  padding: 2px 6px;
  border: 1px solid var(--cpa-rule);
  background: var(--cpa-surface);
  border-radius: 4px;
}

.window-pill.warn {
  color: var(--cpa-error);
  border-color: var(--cpa-error);
  background: color-mix(in srgb, var(--cpa-error) 10%, var(--cpa-surface));
}

.window-pill.dim {
  opacity: 0.55;
}

.window-keeper-detail-col {
  display: flex;
  flex-direction: column;
  gap: 12px;
  position: sticky;
  top: 16px;
}

.account-decision-box {
  padding: 10px;
  border: 1px solid var(--cpa-rule);
  background: var(--cpa-surface-muted);
  border-radius: 4px;
  display: grid;
  gap: 4px;
  margin-bottom: 12px;
}

.spread-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.window-timing-cards {
  display: grid;
  gap: 10px;
  margin-bottom: 16px;
}

.timing-box {
  padding: 10px;
  border: 1px solid var(--cpa-rule);
  border-radius: 4px;
}
.timing-box.blocking {
  border-color: var(--cpa-error);
}

.times-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-top: 8px;
}

.time-item {
  background: var(--cpa-surface-muted);
  padding: 6px 8px;
  border-radius: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.account-actions-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--cpa-rule);
  margin-bottom: 16px;
}

.attempts-section h4 {
  margin: 0 0 8px;
  font-size: 13px;
}

.attempts-list {
  display: grid;
  gap: 8px;
}

.attempt-item {
  padding: 8px;
  background: var(--cpa-surface-muted);
  border-radius: 4px;
  display: grid;
  gap: 4px;
}

.excerpt {
  font-size: 11px;
  white-space: pre-wrap;
  word-break: break-all;
}

@media (max-width: 960px) {
  .window-keeper-layout {
    grid-template-columns: 1fr;
  }
  .window-keeper-detail-col {
    position: static;
  }
}
</style>
