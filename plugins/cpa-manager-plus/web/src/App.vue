<template>
  <main class="page">
    <section class="toolbar">
      <nav class="tabs" aria-label="CPA Manager Plus tabs">
        <button v-for="tab in tabs" :key="tab.key" :class="['tab', {active: activeTab === tab.key}]"
                @click="selectTab(tab.key)">{{ tab.label }}
        </button>
      </nav>
    </section>

    <section v-if="authNotice" class="notice">{{ authNotice }}</section>
    <section v-if="activeError" class="notice error">{{ activeError }}</section>

    <section class="panel" v-if="activeTab === 'dashboard'">
      <DashboardView ref="dashboardView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall"/>
    </section>

    <section class="panel" v-if="activeTab === 'monitoring'">
      <MonitoringView ref="monitoringView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall"/>
    </section>

    <section class="panel" v-if="activeTab === 'inspection'">
      <InspectionView ref="inspectionView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall"/>
    </section>

    <section class="panel" v-if="activeTab === 'config'">
      <DataCard :title="$t('config.credentials.title')" :subtitle="$t('config.credentials.subtitle')">
        <p class="muted">{{ $t('config.credentials.description') }}</p>
        <div class="keybar">
          <input v-model.trim="cpaKeyInput" type="password" autocomplete="off"
                 :placeholder="$t('config.credentials.keyPlaceholder')" @keyup.enter="saveCPAKey"/>
          <button class="btn primary" @click="saveCPAKey">{{ $t('config.credentials.saveAndCheck') }}</button>
          <button class="btn" @click="checkHealth" :disabled="loading">{{ $t('config.credentials.checkRuntime') }}</button>
          <button class="btn danger" @click="clearCPAKey">{{ $t('common.clear') }}</button>
        </div>
        <div class="config-health-row">
          <div :class="['health-pill', health.state]"><span class="dot"></span><span>{{ health.text }}</span></div>
        </div>
      </DataCard>

      <DataCard :title="$t('language.title')" :subtitle="$t('language.subtitle')">
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">{{ $t('language.label') }}</span>
            <select v-model="languageSelection" class="control">
              <option value="follow">{{ $t('language.followCPA') }}</option>
              <option value="en">English</option>
              <option value="zh-CN">简体中文</option>
              <option value="zh-TW">繁體中文（台灣）</option>
              <option value="ru">Русский</option>
            </select>
            <small class="muted">{{ languageStatus }}</small>
          </label>
        </div>
      </DataCard>

      <DataCard :title="$t('config.collector.title')" :subtitle="$t('config.collector.subtitle')">
        <div class="config-form-grid">
          <label class="config-field config-field-toggle">
            <span class="config-field-label">{{ $t('config.collector.label') }}</span>
            <button :class="['toggle-switch', {on: mgrMonitoringEnabled}]"
                    @click="mgrMonitoringEnabled = !mgrMonitoringEnabled"
                    :disabled="mgrSaving || !mgrConfigLoaded">
              <span class="toggle-knob"></span>
            </button>
            <small class="muted">{{ mgrMonitoringEnabled ? $t('common.enabled') : $t('common.disabled') }}</small>
          </label>
        </div>
        <p class="muted small-text" style="margin-top:8px">{{ $t('config.collector.description') }}</p>
      </DataCard>

      <DataCard :title="$t('config.accountAuthorization.title')" :subtitle="$t('config.accountAuthorization.subtitle')">
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">{{ $t('config.accountAuthorization.baseUrl') }}</span>
            <input v-model.trim="mgrCPABaseInput" class="control" placeholder="http://127.0.0.1:8317"
                   :disabled="mgrSaving || !mgrConfigLoaded"/>
            <small class="muted">{{ $t('config.accountAuthorization.currentBinding', {value: mgrBoundCPABase || $t('config.accountAuthorization.unbound')}) }}</small>
          </label>
          <label class="config-field">
            <span class="config-field-label">{{ $t('config.accountAuthorization.managementKey') }}</span>
            <div class="keybar">
              <input v-model.trim="mgrCPAKeyInput" :type="mgrCPAKeyVisible ? 'text' : 'password'"
                     autocomplete="new-password" :placeholder="$t('config.accountAuthorization.keyPlaceholder')"
                     :disabled="mgrSaving || !mgrConfigLoaded"/>
              <button class="btn" @click="mgrCPAKeyVisible = !mgrCPAKeyVisible"
                      :disabled="mgrSaving || !mgrConfigLoaded">
                {{ mgrCPAKeyVisible ? $t('common.hide') : $t('common.show') }}
              </button>
              <button class="btn" @click="mgrCPAKeyInput = ''; mgrCPAKeyVisible = false"
                      :disabled="mgrSaving || !mgrCPAKeyInput">{{ $t('common.clear') }}
              </button>
            </div>
            <small class="muted">{{ mgrHasBoundKey ? $t('config.accountAuthorization.bound') : $t('config.accountAuthorization.unbound') }}</small>
          </label>
        </div>
        <p class="muted small-text" style="margin-top:8px">{{ $t('config.accountAuthorization.description') }}</p>
      </DataCard>

      <DataCard :title="$t('config.runtime.title')" :subtitle="$t('config.runtime.subtitle')">
        <div class="config-meta-grid">
          <div><span>{{ $t('config.runtime.source') }}</span><strong>{{ mgrConfigSourceLabel }}</strong></div>
          <div><span>{{ $t('config.runtime.dataDir') }}</span><strong>{{ mgrLoadedConfig?.dataDir || '—' }}</strong></div>
          <div><span>{{ $t('config.runtime.queueCapacity') }}</span><strong>{{ mgrLoadedConfig?.collector?.queueCapacity ?? '—' }}</strong></div>
          <div><span>{{ $t('config.runtime.batchSize') }}</span><strong>{{ mgrLoadedConfig?.collector?.batchSize ?? '—' }}</strong></div>
        </div>
        <p class="muted small-text" style="margin-top:8px">{{ $t('config.runtime.description') }}</p>
      </DataCard>

      <div class="config-save-block">
        <p v-if="!mgrConfigLoaded && resolvedCPAKey" class="muted small-text">{{ $t('config.save.loading') }}</p>
        <p v-else-if="mgrConfigLoaded && !mgrDirty" class="muted small-text">{{ $t('config.save.clean') }}</p>
        <p v-if="configSaveMessage" class="notice config-save-ok">{{ configSaveMessage }}</p>
        <div class="config-actions-bar">
          <button class="btn primary" @click="saveManagerConfig" :disabled="mgrSaving || !mgrConfigLoaded || !mgrDirty">
            {{ mgrSaving ? $t('common.loading') : $t('config.save.savePlugin') }}
          </button>
          <button class="btn" @click="loadConfig" :disabled="mgrSaving || !resolvedCPAKey">{{ $t('config.save.reload') }}</button>
        </div>
      </div>
    </section>

    <section class="panel" v-if="activeTab === 'model-prices'">
      <ModelPricesView ref="modelPricesView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall"/>
    </section>
    <section class="panel" v-if="activeTab === 'account-actions'">
      <AccountActionsView ref="accountActionsView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall"/>
    </section>
  </main>
</template>

<script setup>
import {computed, onBeforeUnmount, onMounted, reactive, ref, watch} from 'vue';
import {useI18n} from 'vue-i18n';
import DataCard from './components/DataCard.vue';
import MonitoringView from './components/MonitoringView.vue';
import DashboardView from './components/DashboardView.vue';
import ModelPricesView from './components/ModelPricesView.vue';
import AccountActionsView from './components/AccountActionsView.vue';
import InspectionView from './components/InspectionView.vue';
import {formatHealthText, HEALTH, LEGACY_SESSION_KEY, PROXY, readCPAAuthStoreKey, SESSION_KEY} from './utils/data.js';
import {buildManagerConfigSaveBody} from './utils/managerConfigSave.js';
import {initThemeBridge} from './themeBridge.js';
import {
  clearManualLocaleOverride,
  destroyLocaleBridge,
  localeModeRef,
  localeRef,
  setManualLocale,
} from './localeBridge.js';

initThemeBridge();

const {t} = useI18n();
const localeKeys = {en: 'language.English', 'zh-CN': 'language.zhCN', 'zh-TW': 'language.zhTW', ru: 'language.Russian'};
const tabs = computed(() => [
  {key: 'dashboard', label: t('tabs.dashboard')},
  {key: 'monitoring', label: t('tabs.monitoring')},
  {key: 'model-prices', label: t('tabs.modelPrices')},
  {key: 'account-actions', label: t('tabs.accountActions')},
  {key: 'inspection', label: t('tabs.inspection')},
  {key: 'config', label: t('tabs.config')},
]);
const activeTab = ref('dashboard');
const languageSelection = computed({
  get: () => localeModeRef.value === 'follow' ? 'follow' : localeRef.value,
  set: (value) => {
    if (value === 'follow') clearManualLocaleOverride();
    else setManualLocale(value);
  },
});
const currentLanguageLabel = computed(() => t(localeKeys[localeRef.value] || 'language.English'));
const languageStatus = computed(() => localeModeRef.value === 'follow'
  ? t('language.following')
  : t('language.effective', {language: currentLanguageLabel.value}));
const loading = ref(false);
const health = reactive({state: '', text: t('auth.notChecked'), messageKey: 'auth.notChecked', response: null});
const cpaKeyInput = ref((sessionStorage.getItem(SESSION_KEY) || '').trim());
const errors = reactive({});
const configData = ref(null);
const dashboardView = ref(null);
const monitoringView = ref(null);
const modelPricesView = ref(null);
const accountActionsView = ref(null);
const inspectionView = ref(null);

// Local runtime config state
const mgrSaving = ref(false);
const mgrCPABaseInput = ref('');
const mgrCPAKeyInput = ref('');
const mgrCPAKeyVisible = ref(false);
const mgrMonitoringEnabled = ref(true);
const mgrBoundCPABase = ref('');
const mgrHasBoundKey = ref(false);
const mgrConfigSource = ref('');
const mgrLoadedConfig = ref(null);
const mgrConfigLoaded = ref(false);
const configSaveMessageKey = ref('');
const configSaveMessage = computed(() => configSaveMessageKey.value ? t(configSaveMessageKey.value) : '');

const mgrConfigSourceLabel = computed(() => {
  if (mgrConfigSource.value === 'plugin') return t('config.runtime.localRuntime');
  if (mgrConfigSource.value === 'env') return t('config.runtime.environment');
  if (mgrConfigSource.value === 'db') return t('config.runtime.database');
  return mgrConfigLoaded.value ? t('config.runtime.localRuntime') : t('config.runtime.notLoaded');
});
const mgrDirty = computed(() => {
  if (!mgrConfigLoaded.value) return false;
  const c = mgrLoadedConfig.value || {};
  const conn = c.cpaConnection || {};
  const col = c.collector || {};
  if (mgrCPABaseInput.value !== (conn.cpaBaseUrl || '')) return true;
  if (mgrCPAKeyInput.value.trim()) return true;
  return mgrMonitoringEnabled.value !== (col.enabled !== false);
});

const resolvedCPAKey = computed(() => {
  const input = (cpaKeyInput.value || '').trim();
  if (input) return input;
  const session = (sessionStorage.getItem(SESSION_KEY) || '').trim();
  if (session) return session;
  const store = readCPAAuthStoreKey();
  if (store) return store;
  return (sessionStorage.getItem(LEGACY_SESSION_KEY) || '').trim();
});
const authNotice = computed(() => resolvedCPAKey.value ? '' : t('auth.notice'));
const activeError = computed(() => errors[activeTab.value] || '');

function authHeaders(json = true) {
  const headers = json ? {
    'Content-Type': 'application/json',
    'Accept': 'application/json'
  } : {'Accept': 'application/json'};
  const key = resolvedCPAKey.value;
  if (!key) return headers;
  const clean = key.replace(/^Bearer\s+/i, '');
  headers.Authorization = 'Bearer ' + clean;
  headers['X-Management-Key'] = clean;
  return headers;
}

async function readJSONResponse(res) {
  const text = await res.text();
  if (!text) return null;
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

function formatError(status, body) {
  if (body && typeof body === 'object') {
    const code = body.code ? `[${body.code}] ` : '';
    const msg = body.error || body.message || body.msg || '';
    if (msg) {
      const message = code + msg;
      if (status === 401) return t('errors.authentication', {message});
      if (status === 403) return t('errors.forbidden', {message});
      if (status === 409) return t('errors.conflict', {message});
      return message;
    }
  }
  if (typeof body === 'string' && body.trim()) return body.trim();
  if (status === 401) return t('errors.authenticationHint');
  if (status === 403) return t('errors.forbiddenHint');
  return t('errors.http', {status});
}

async function proxyCall(payload) {
  if (!resolvedCPAKey.value) throw new Error(t('errors.missingManagementKey'));
  const res = await fetch(PROXY, {method: 'POST', headers: authHeaders(true), body: JSON.stringify(payload)});
  const body = await readJSONResponse(res);
  if (!res.ok) throw new Error(formatError(res.status, body));
  return body;
}

async function checkHealth() {
  if (!resolvedCPAKey.value) {
    health.state = 'err';
    health.response = null;
    health.messageKey = 'auth.missingKey';
    health.text = t(health.messageKey);
    return;
  }
  health.state = '';
  health.response = null;
  health.messageKey = 'common.loading';
  health.text = t(health.messageKey);
  try {
    const res = await fetch(HEALTH, {headers: authHeaders(false)});
    const body = await readJSONResponse(res);
    health.response = {status: res.status, body, ok: Boolean(res.ok && body?.ok)};
    health.messageKey = '';
    if (health.response.ok) {
      health.state = 'ok';
      health.text = formatHealthText(body);
    } else {
      health.state = 'err';
      health.text = formatError(res.status, body) || formatHealthText(body);
    }
  } catch (e) {
    health.state = 'err';
    health.response = null;
    health.messageKey = 'auth.checkFailed';
    health.text = e.message || t(health.messageKey);
  }
}

function selectTab(tab) {
  activeTab.value = tab;
  refreshActive();
}

async function refreshActive() {
  if (loading.value) return;
  loading.value = true;
  errors[activeTab.value] = '';
  try {
    if (activeTab.value === 'dashboard') await (dashboardView.value ? dashboardView.value.refresh(true) : Promise.resolve());
    if (activeTab.value === 'monitoring') await (monitoringView.value ? monitoringView.value.refresh(true) : Promise.resolve());
    if (activeTab.value === 'inspection') await (inspectionView.value ? inspectionView.value.refresh(true) : Promise.resolve());
    if (activeTab.value === 'config') await loadConfig();
    if (activeTab.value === 'model-prices') await (modelPricesView.value ? modelPricesView.value.refresh(true) : Promise.resolve());
    if (activeTab.value === 'account-actions') await (accountActionsView.value ? accountActionsView.value.refresh(true) : Promise.resolve());
  } catch (e) {
    errors[activeTab.value] = e.message || String(e);
  } finally {
    loading.value = false;
  }
}

async function loadConfig() {
  if (!resolvedCPAKey.value) return;
  mgrConfigLoaded.value = false;
  configSaveMessageKey.value = '';
  const resp = await proxyCall({method: 'GET', path: '/usage-service/config'});
  configData.value = resp;
  const cfg = resp?.config || resp || {};
  mgrLoadedConfig.value = cfg;
  mgrConfigSource.value = resp?.source || 'plugin';
  mgrCPABaseInput.value = cfg.cpaConnection?.cpaBaseUrl || '';
  mgrBoundCPABase.value = cfg.cpaConnection?.cpaBaseUrl || '';
  mgrHasBoundKey.value = Boolean(
    cfg.cpaConnection?.hasManagementKey ?? cfg.cpaConnection?.managementKey,
  );
  mgrCPAKeyInput.value = '';
  mgrCPAKeyVisible.value = false;
  mgrMonitoringEnabled.value = cfg.collector?.enabled !== false;
  mgrConfigLoaded.value = true;
}

async function saveManagerConfig() {
  if (!mgrConfigLoaded.value) {
    errors.config = t('config.save.notLoaded');
    return;
  }
  if (!mgrDirty.value) return;
  mgrSaving.value = true;
  errors.config = '';
  configSaveMessageKey.value = '';
  try {
    const body = buildManagerConfigSaveBody({
      currentConfig: mgrLoadedConfig.value || {},
      cpaBaseURL: mgrCPABaseInput.value,
      managementKey: mgrCPAKeyInput.value,
      monitoringEnabled: mgrMonitoringEnabled.value,
    });
    // Local runtime expects outer {"config": ...}; only dirty sections are included.
    await proxyCall({method: 'PUT', path: '/usage-service/config', body});
    await loadConfig();
    configSaveMessageKey.value = 'config.save.saved';
    checkHealth();
  } catch (e) {
    errors.config = e.message || String(e);
  } finally {
    mgrSaving.value = false;
  }
}

function saveCPAKey() {
  const key = (cpaKeyInput.value || '').trim().replace(/^Bearer\s+/i, '');
  if (key) sessionStorage.setItem(SESSION_KEY, key);
  cpaKeyInput.value = key;
  checkHealth();
  refreshActive();
}

function clearCPAKey() {
  cpaKeyInput.value = '';
  sessionStorage.removeItem(SESSION_KEY);
  sessionStorage.removeItem(LEGACY_SESSION_KEY);
  health.state = '';
  health.response = null;
  health.messageKey = 'auth.notChecked';
  health.text = t(health.messageKey);
}

function handleOpenMonitoring() {
  activeTab.value = 'monitoring';
  setTimeout(() => {
    refreshActive();
  }, 0);
}

function handleOpenTab(event) {
  const tab = event?.detail?.tab;
  if (!tab) return;
  activeTab.value = tab;
  setTimeout(() => {
    refreshActive();
  }, 0);
}

watch(localeRef, () => {
  if (health.response) {
    health.text = health.response.ok
      ? formatHealthText(health.response.body)
      : formatError(health.response.status, health.response.body) || formatHealthText(health.response.body);
  } else if (health.messageKey) {
    health.text = t(health.messageKey);
  }
});

onMounted(() => {
  checkHealth();
  refreshActive();
  window.addEventListener('cpa-manager-plus:open-monitoring', handleOpenMonitoring);
  window.addEventListener('cpa-manager-plus:open-tab', handleOpenTab);
});
onBeforeUnmount(() => {
  window.removeEventListener('cpa-manager-plus:open-monitoring', handleOpenMonitoring);
  window.removeEventListener('cpa-manager-plus:open-tab', handleOpenTab);
  destroyLocaleBridge();
});
</script>
