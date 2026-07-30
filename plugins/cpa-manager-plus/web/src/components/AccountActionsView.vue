<template>
  <section class="monitoring-page account-actions-page">
    <section class="card auto-ban-panel">
      <div class="section-title auto-ban-title-row">
        <div>
          <h2>{{ t('autoBan.title') }}</h2>
          <p class="muted small-text">{{ t('autoBan.subtitle') }}</p>
        </div>
        <button class="btn" type="button" @click="refreshAll" :disabled="loading">
          {{ loading ? t('common.loading') : t('autoBan.refresh') }}
        </button>
      </div>

      <section class="auto-ban-settings" :aria-label="t('autoBan.settings')">
        <label class="config-field config-field-toggle">
          <span class="config-field-label">{{ t('autoBan.enabled') }}</span>
          <button type="button" :class="['toggle-switch', { on: settings.enabled }]" :aria-pressed="settings.enabled" @click="settings.enabled = !settings.enabled"><span class="toggle-knob"></span></button>
        </label>
        <label class="config-field config-field-toggle">
          <span class="config-field-label">{{ t('autoBan.dryRun') }}</span>
          <button type="button" :class="['toggle-switch', { on: settings.dryRun }]" :aria-pressed="settings.dryRun" @click="settings.dryRun = !settings.dryRun"><span class="toggle-knob"></span></button>
        </label>
        <label class="config-field config-field-toggle">
          <span class="config-field-label">{{ t('autoBan.sourceUsage') }}</span>
          <button type="button" :class="['toggle-switch', { on: settings.sources.usage }]" :aria-pressed="settings.sources.usage" @click="settings.sources.usage = !settings.sources.usage"><span class="toggle-knob"></span></button>
        </label>
        <label class="config-field config-field-toggle">
          <span class="config-field-label">{{ t('autoBan.sourceInspection') }}</span>
          <button type="button" :class="['toggle-switch', { on: settings.sources.inspection }]" :aria-pressed="settings.sources.inspection" @click="settings.sources.inspection = !settings.sources.inspection"><span class="toggle-knob"></span></button>
        </label>
        <label class="config-field">
          <span class="config-field-label">{{ t('autoBan.codexCooldownHours') }}</span>
          <input v-model.number="settings.defaultCodexCooldownHours" type="number" min="1" max="720" class="control" />
        </label>
        <label class="config-field">
          <span class="config-field-label">{{ t('autoBan.schedulerSeconds') }}</span>
          <input v-model.number="settings.schedulerIntervalSeconds" type="number" min="5" max="3600" class="control" />
        </label>
        <button class="btn primary auto-ban-save-settings" type="button" :disabled="savingSettings" @click="saveSettings">{{ t('autoBan.saveSettings') }}</button>
      </section>

      <div class="auto-ban-warning muted small-text">{{ t('autoBan.warnings.hostCooldown') }}</div>
    </section>

    <section v-if="error" class="notice error">{{ error }}</section>

    <DataCard :title="t('autoBan.rules')" :subtitle="`${rules.length}`">
      <div class="auto-ban-toolbar">
        <span class="muted small-text">{{ t('autoBan.warnings.delete') }}</span>
        <button class="btn primary" type="button" @click="openRuleEditor()">{{ t('autoBan.addRule') }}</button>
      </div>
      <div v-if="rules.length" class="table-wrap monitor-table">
        <table>
          <thead><tr><th>{{ t('autoBan.provider') }}</th><th>{{ t('autoBan.statusCodes') }}</th><th>{{ t('autoBan.threshold') }}</th><th>{{ t('autoBan.action') }}</th><th>{{ t('autoBan.cooldownHours') }}</th><th>{{ t('autoBan.manual') }}</th></tr></thead>
          <tbody>
            <tr v-for="rule in rules" :key="rule.id">
              <td><strong>{{ rule.name }}</strong><div class="muted small-text">{{ rule.providerScope }} · {{ rule.accountKind }}</div></td>
              <td>{{ (rule.matchStatusCodes || []).join(', ') || '—' }}<div class="muted small-text">{{ (rule.matchErrorKinds || []).join(', ') }}</div></td>
              <td>{{ rule.thresholdMode === 'total' ? t('autoBan.total') : t('autoBan.consecutive') }} · {{ rule.thresholdCount }}</td>
              <td><span :class="['status-badge', rule.action === 'delete' ? 'bad' : rule.action === 'review' ? 'warn' : 'good']">{{ actionLabel(rule.action) }}</span></td>
              <td>{{ rule.cooldownMs ? `${Math.round(rule.cooldownMs / 3600000)}h` : '—' }}</td>
              <td><div class="config-actions-bar action-row"><button class="btn" type="button" @click="openRuleEditor(rule)">{{ t('autoBan.editRule') }}</button><button class="btn danger" type="button" @click="requestDeleteRule(rule)">{{ t('autoBan.deleteRule') }}</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div v-else class="empty">{{ t('autoBan.noRules') }}</div>
    </DataCard>

    <DataCard :title="t('autoBan.accounts')" :subtitle="`${autoAccounts.length}`">
      <div class="card filter-card auto-ban-filters">
        <select v-model="autoStateFilter" class="control compact" @change="loadAutoBanAccounts"><option value="">{{ t('common.all') }}</option><option v-for="state in AUTO_BAN_STATES" :key="state" :value="state">{{ stateLabel(state) }}</option></select>
        <input v-model.trim="autoSearch" class="control wide" :placeholder="t('accountActions.searchPlaceholder')" @keyup.enter="loadAutoBanAccounts" />
        <button class="btn" type="button" @click="loadAutoBanAccounts">{{ t('autoBan.refresh') }}</button>
      </div>
      <div v-if="autoAccounts.length" class="table-wrap monitor-table">
        <table>
          <thead><tr><th>{{ t('accountActions.columns.account') }}</th><th>{{ t('autoBan.provider') }}</th><th>{{ t('autoBan.state') }}</th><th>{{ t('autoBan.lastCode') }}</th><th>{{ t('autoBan.hits') }}</th><th>{{ t('autoBan.cooldown') }}</th><th>{{ t('autoBan.manual') }}</th></tr></thead>
          <tbody>
            <template v-for="account in autoAccounts" :key="account.id">
              <tr :class="{ selected: selectedAccount?.id === account.id }">
                <td><strong>{{ account.displayName || account.fileName || account.accountKey }}</strong><div class="muted small-text">{{ account.fileName || account.accountKey }}</div></td>
                <td>{{ account.provider }}<div class="muted small-text">{{ capabilityLabel(account.accountKind, account.capabilityFlags) }}</div></td>
                <td><span :class="['status-badge', stateTone(account.state)]">{{ stateLabel(account.state) }}</span></td>
                <td>{{ account.lastStatusCode || '—' }}<div class="muted small-text">{{ account.lastErrorKind || '' }}</div></td>
                <td>{{ account.consecutiveHits }} / {{ account.totalHits }}</td>
                <td>{{ formatCooldownUntil(account.cooldownUntilMs) || '—' }}</td>
                <td>
                  <div class="config-actions-bar action-row">
                    <button class="btn" type="button" @click="loadAccountDetail(account.id)">{{ t('autoBan.detail') }}</button>
                    <button v-if="account.state === 'cooling' || account.state === 'disabled'" class="btn primary" type="button" @click="requestAccountAction(account, 'unban')">{{ t('autoBan.unban') }}</button>
                    <button v-if="!account.manualHold" class="btn" type="button" @click="requestAccountAction(account, 'hold')">{{ t('autoBan.hold') }}</button>
                    <button v-else class="btn" type="button" @click="requestAccountAction(account, 'release')">{{ t('autoBan.release') }}</button>
                    <button class="btn danger" type="button" @click="requestAccountAction(account, 'delete')">{{ t('autoBan.delete') }}</button>
                  </div>
                </td>
              </tr>
              <tr v-if="selectedAccount?.id === account.id" class="auto-ban-history-row"><td colspan="7"><div class="auto-ban-history"><strong>{{ t('autoBan.history') }}</strong><div v-if="selectedHistory.length" class="history-list"><div v-for="entry in selectedHistory" :key="entry.id" class="history-entry"><span>{{ formatDateTime(entry.createdAtMs) }}</span><span>{{ entry.eventType }}</span><span>{{ entry.statusCode || '' }}</span><span>{{ entry.message }}</span></div></div><div v-else class="muted small-text">{{ t('common.noData') }}</div><div class="config-actions-bar action-row"><button class="btn" type="button" @click="requestAccountAction(account, 'disable')">{{ t('autoBan.disable') }}</button><button class="btn" type="button" @click="requestAccountAction(account, 'enable')">{{ t('autoBan.enable') }}</button><button class="btn" type="button" @click="requestAccountAction(account, 'reset_counters')">{{ t('autoBan.resetCounters') }}</button></div></div></td></tr>
            </template>
          </tbody>
        </table>
      </div>
      <div v-else class="empty">{{ t('autoBan.noAccounts') }}</div>
    </DataCard>

    <DataCard :title="t('accountActions.title')" :subtitle="cardSubtitle">
      <div class="card filter-card auto-ban-filters">
        <select v-model="filter" class="control compact" @change="loadCandidates"><option value="pending">{{ t('accountActions.filter.pending') }}</option><option value="all">{{ t('accountActions.filter.all') }}</option><option value="ignored">{{ t('accountActions.filter.ignored') }}</option><option value="resolved">{{ t('accountActions.filter.resolved') }}</option><option value="deleted">{{ t('accountActions.filter.deleted') }}</option></select>
        <input v-model.trim="search" class="control wide" :placeholder="t('accountActions.searchPlaceholder')" @keyup.enter="loadCandidates" />
        <button class="btn primary" type="button" @click="loadCandidates" :disabled="loading">{{ loading ? t('common.loading') : t('common.refresh') }}</button>
      </div>
      <div class="table-wrap monitor-table">
        <table><thead><tr><th>{{ t('accountActions.columns.account') }}</th><th>{{ t('accountActions.columns.provider') }}</th><th>{{ t('accountActions.columns.authFile') }}</th><th>{{ t('accountActions.columns.errorKind') }}</th><th>{{ t('accountActions.columns.errorCode') }}</th><th>{{ t('accountActions.columns.status') }}</th><th>{{ t('accountActions.columns.actions') }}</th></tr></thead>
          <tbody><tr v-for="item in visibleItems" :key="item.id" :class="{ selected: actingId === item.id }"><td><div>{{ item.account_snapshot || item.accountSnapshot || EMPTY_VALUE }}</div><div class="muted small-text">{{ item.auth_label || item.authLabel || '' }}</div></td><td>{{ item.provider || EMPTY_VALUE }}</td><td>{{ item.auth_file_name || item.authFileName || EMPTY_VALUE }}</td><td>{{ item.error_kind || item.errorKind || item.header_error_kind || item.headerErrorKind || EMPTY_VALUE }}</td><td>{{ item.error_code || item.errorCode || item.header_error_code || item.headerErrorCode || EMPTY_VALUE }}</td><td><span :class="['status-badge', item.status === 'pending' ? 'bad' : 'good']">{{ statusLabel(item.status) }}</span></td><td><div class="config-actions-bar action-row"><button v-if="item.status === 'pending'" class="btn primary" type="button" @click="actCandidate(item.id, 'enable')" :disabled="busy">{{ t('accountActions.actions.enable') }}</button><button v-if="item.status === 'pending'" class="btn" type="button" @click="actCandidate(item.id, 'ignore')" :disabled="busy">{{ t('accountActions.actions.ignore') }}</button><button v-if="item.status === 'pending'" class="btn danger" type="button" @click="actCandidate(item.id, 'delete')" :disabled="busy">{{ t('accountActions.actions.delete') }}</button><button v-if="item.status === 'pending'" class="btn" type="button" @click="actCandidate(item.id, 'resolve')" :disabled="busy">{{ t('accountActions.actions.resolve') }}</button></div></td></tr></tbody>
        </table>
      </div>
    </DataCard>

    <div v-if="ruleEditorOpen" class="drawer-backdrop" @click.self="closeRuleEditor">
      <div class="modal-dialog card drawer auto-ban-rule-drawer" role="dialog" aria-modal="true" :aria-labelledby="ruleEditorTitleId">
        <div class="drawer-head"><h2 :id="ruleEditorTitleId">{{ ruleDraftState.id ? t('autoBan.editRule') : t('autoBan.addRule') }}</h2><button class="btn" type="button" @click="closeRuleEditor">{{ t('common.close') }}</button></div>
        <div class="config-form-grid">
          <label class="config-field config-field-wide"><span class="config-field-label">{{ t('autoBan.editRule') }}</span><input v-model.trim="ruleDraftState.name" class="control" /></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.provider') }}</span><select v-model="ruleDraftState.providerScope" class="control"><option value="codex">Codex</option><option value="xai">xAI</option><option value="custom">Custom</option><option value="*">*</option></select></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.accountKind') }}</span><select v-model="ruleDraftState.accountKind" class="control"><option value="oauth_auth_file">{{ t('autoBan.capability.oauth_auth_file') }}</option><option value="custom_provider">{{ t('autoBan.capability.custom_provider') }}</option><option value="any">{{ t('common.all') }}</option></select></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.statusCodes') }}</span><input v-model.trim="ruleDraftState.statusCodes" class="control" placeholder="429, 401" /></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.errorKinds') }}</span><input v-model.trim="ruleDraftState.errorKinds" class="control" placeholder="rate_limited" /></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.thresholdMode') }}</span><select v-model="ruleDraftState.thresholdMode" class="control"><option value="consecutive">{{ t('autoBan.consecutive') }}</option><option value="total">{{ t('autoBan.total') }}</option></select></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.threshold') }}</span><input v-model.number="ruleDraftState.thresholdCount" type="number" min="1" class="control" /></label>
          <label v-if="ruleDraftState.thresholdMode === 'total'" class="config-field"><span class="config-field-label">{{ t('autoBan.windowMinutes') }}</span><input v-model.number="ruleDraftState.windowMinutes" type="number" min="1" class="control" /></label>
          <label class="config-field"><span class="config-field-label">{{ t('autoBan.action') }}</span><select v-model="ruleDraftState.action" class="control"><option value="review">{{ actionLabel('review') }}</option><option value="disable">{{ actionLabel('disable') }}</option><option value="cooldown_enable">{{ actionLabel('cooldown_enable') }}</option><option value="delete">{{ actionLabel('delete') }}</option></select></label>
          <label v-if="ruleDraftState.action === 'cooldown_enable'" class="config-field"><span class="config-field-label">{{ t('autoBan.cooldownHours') }}</span><input v-model.number="ruleDraftState.cooldownHours" type="number" min="1" max="720" class="control" /></label>
          <label v-if="ruleDraftState.action === 'cooldown_enable'" class="config-field"><span class="config-field-label">{{ t('autoBan.cooldownSource') }}</span><select v-model="ruleDraftState.cooldownSource" class="control"><option value="header_or_default">{{ t('autoBan.headerOrDefault') }}</option><option value="fixed">{{ t('autoBan.fixed') }}</option><option value="header_only">{{ t('autoBan.headerOnly') }}</option></select></label>
          <label v-if="ruleDraftState.action === 'delete'" class="config-field"><span class="config-field-label">{{ t('autoBan.dailyCap') }}</span><input v-model.number="ruleDraftState.maxActionsPerDay" type="number" min="1" class="control" /></label>
          <label class="config-field config-field-toggle"><span class="config-field-label">{{ t('autoBan.respectHostCooldown') }}</span><button type="button" :class="['toggle-switch', { on: ruleDraftState.respectHostCooldown }]" :aria-pressed="ruleDraftState.respectHostCooldown" @click="ruleDraftState.respectHostCooldown = !ruleDraftState.respectHostCooldown"><span class="toggle-knob"></span></button></label>
        </div>
        <div v-if="ruleError" class="notice error">{{ ruleError }}</div>
        <div class="config-actions-bar"><button class="btn" type="button" @click="closeRuleEditor">{{ t('common.cancel') }}</button><button class="btn primary" type="button" :disabled="savingRule" @click="saveRule">{{ t('autoBan.saveRule') }}</button></div>
      </div>
    </div>

    <ConfirmModal :open="Boolean(confirmRequest)" :title="confirmTitle" :message="confirmMessage" :confirm-label="t('autoBan.confirmAction')" :variant="confirmRequest?.danger ? 'danger' : 'primary'" @confirm="confirmPendingAction" @cancel="confirmRequest = null" />
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import ConfirmModal from './ConfirmModal.vue';
import DataCard from './DataCard.vue';
import { accountActionPath } from '../utils/data.js';
import { EMPTY_VALUE, formatDateTime } from '../utils/localeFormat.js';
import { AUTO_BAN_STATES, autoBanPath, formatCooldownUntil, normalizeAutoBanRule, normalizeAutoBanSettings, ruleDraft, validateAutoBanRule } from '../utils/autoBan.js';

const props = defineProps({ ready: { type: Boolean, default: false }, proxyCall: { type: Function, required: true } });
const { t } = useI18n();

const settings = ref(normalizeAutoBanSettings());
const rules = ref([]);
const autoAccounts = ref([]);
const selectedAccount = ref(null);
const selectedHistory = ref([]);
const autoStateFilter = ref('');
const autoSearch = ref('');
const loading = ref(false);
const savingSettings = ref(false);
const savingRule = ref(false);
const busy = ref(false);
const error = ref('');
const filter = ref('pending');
const search = ref('');
const items = ref([]);
const actingId = ref(null);
const ruleEditorOpen = ref(false);
const ruleDraftState = ref(ruleDraft());
const ruleError = ref('');
const confirmRequest = ref(null);
const ruleEditorTitleId = 'auto-ban-rule-editor';

const filterLabel = computed(() => t({ pending: 'accountActions.filter.pending', all: 'accountActions.filter.all', ignored: 'accountActions.filter.ignored', resolved: 'accountActions.filter.resolved', deleted: 'accountActions.filter.deleted' }[filter.value] || 'accountActions.filter.all'));
const cardSubtitle = computed(() => t('accountActions.subtitle', { filter: filterLabel.value, count: visibleItems.value.length }));
const visibleItems = computed(() => {
  const term = search.value.trim().toLowerCase();
  if (!term) return items.value;
  return items.value.filter(item => [item.account_snapshot, item.accountSnapshot, item.auth_label, item.authLabel, item.provider, item.auth_file_name, item.authFileName, item.error_kind, item.errorKind, item.error_code, item.errorCode].some(value => value && String(value).toLowerCase().includes(term)));
});

onMounted(() => { if (props.ready) refreshAll(); });

async function refreshAll() {
  if (!props.ready) return;
  loading.value = true;
  error.value = '';
  try { await Promise.all([loadSettings(), loadRules(), loadAutoBanAccounts(), loadCandidates()]); } catch (e) { error.value = e.message || String(e); } finally { loading.value = false; }
}
async function loadSettings() { settings.value = normalizeAutoBanSettings(await props.proxyCall({ method: 'GET', path: autoBanPath('settings') })); }
async function loadRules() { const response = await props.proxyCall({ method: 'GET', path: autoBanPath('rules') }); rules.value = response?.items || []; }
async function loadAutoBanAccounts() { const query = new URLSearchParams({ limit: '200' }); if (autoStateFilter.value) query.set('state', autoStateFilter.value); if (autoSearch.value) query.set('q', autoSearch.value); const response = await props.proxyCall({ method: 'GET', path: autoBanPath('accounts'), query: query.toString() }); autoAccounts.value = response?.items || []; }
async function loadCandidates() { const qs = filter.value === 'all' ? 'limit=200' : `status=${encodeURIComponent(filter.value)}&limit=200`; const response = await props.proxyCall({ method: 'GET', path: '/v0/management/account-action-candidates', query: qs }); items.value = response?.items || []; }
async function saveSettings() { savingSettings.value = true; error.value = ''; try { settings.value = normalizeAutoBanSettings(await props.proxyCall({ method: 'PUT', path: autoBanPath('settings'), body: settings.value })); } catch (e) { error.value = e.message || String(e); } finally { savingSettings.value = false; } }
function openRuleEditor(rule) { ruleDraftState.value = ruleDraft(rule); ruleError.value = ''; ruleEditorOpen.value = true; }
function closeRuleEditor() { ruleEditorOpen.value = false; ruleError.value = ''; }
async function saveRule() { const rule = normalizeAutoBanRule(ruleDraftState.value); const errors = validateAutoBanRule(rule); if (Object.keys(errors).length) { ruleError.value = errors.maxActionsPerDay ? t('autoBan.deleteNeedsCap') : errors.match ? t('autoBan.matchRequired') : t('common.error'); return; } savingRule.value = true; try { const method = rule.id ? 'PATCH' : 'POST'; const path = rule.id ? autoBanPath(`rules/${rule.id}`) : autoBanPath('rules'); await props.proxyCall({ method, path, body: rule }); await loadRules(); closeRuleEditor(); } catch (e) { ruleError.value = e.message || String(e); } finally { savingRule.value = false; } }
function requestDeleteRule(rule) { confirmRequest.value = { type: 'delete-rule', rule, danger: true }; }
async function loadAccountDetail(id) { try { const response = await props.proxyCall({ method: 'GET', path: autoBanPath(`accounts/${id}`) }); selectedAccount.value = response?.account || null; selectedHistory.value = response?.history || []; } catch (e) { error.value = e.message || String(e); } }
function requestAccountAction(account, action) { confirmRequest.value = { type: 'account-action', account, action, danger: action === 'delete' }; }
async function confirmPendingAction() { const request = confirmRequest.value; confirmRequest.value = null; if (!request) return; try { if (request.type === 'delete-rule') { await props.proxyCall({ method: 'DELETE', path: autoBanPath(`rules/${request.rule.id}`) }); await loadRules(); return; } await props.proxyCall({ method: 'POST', path: autoBanPath(`accounts/${request.account.id}/actions`), body: { action: request.action } }); await loadAutoBanAccounts(); if (selectedAccount.value?.id === request.account.id) await loadAccountDetail(request.account.id); } catch (e) { error.value = e.message || String(e); } }
async function actCandidate(id, action) { busy.value = true; actingId.value = id; try { await props.proxyCall({ method: action === 'delete' ? 'DELETE' : 'POST', path: accountActionPath(id, action) }); await loadCandidates(); } catch (e) { error.value = e.message || String(e); } finally { busy.value = false; actingId.value = null; } }
function actionLabel(action) { return t(`autoBan.actionLabels.${action}`); }
function stateLabel(state) { return t(`autoBan.stateLabels.${state}`); }
function stateTone(state) { return ['disabled', 'deleted'].includes(state) ? 'bad' : ['flagged', 'held', 'cooling', 'pending_action'].includes(state) ? 'warn' : 'good'; }
function capabilityLabel(kind, flags) { return flags ? t(`autoBan.capability.${kind}`) : t('autoBan.capability.unavailable'); }
function statusLabel(status) { return t({ pending: 'accountActions.status.pending', ignored: 'accountActions.status.ignored', resolved: 'accountActions.status.resolved', deleted: 'accountActions.status.deleted' }[status] || 'common.unknown'); }
const confirmTitle = computed(() => confirmRequest.value?.type === 'delete-rule' ? t('autoBan.deleteRule') : confirmRequest.value?.action === 'delete' ? t('autoBan.confirmDelete') : t('autoBan.confirmAction'));
const confirmMessage = computed(() => confirmRequest.value?.type === 'delete-rule' ? t('autoBan.ruleDeleteConfirm') : '');
defineExpose({ refresh: refreshAll });
</script>
