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
      <DataCard title="访问凭据" subtitle="仅浏览器缓存">
        <p class="muted">浏览器访问 CPA Manager Plus / 插件 API 所需的 CPA
          <code>management key</code>（仅保存在本页 sessionStorage，与下方「账号处置授权」不同）。</p>
        <div class="keybar">
          <input v-model.trim="cpaKeyInput" type="password" autocomplete="off"
                 placeholder="CPA management key（当前会话临时保存）" @keyup.enter="saveCPAKey"/>
          <button class="btn primary" @click="saveCPAKey">保存并检测</button>
          <button class="btn" @click="checkHealth" :disabled="loading">检测 Runtime</button>
          <button class="btn danger" @click="clearCPAKey">清除</button>
        </div>
        <div class="config-health-row">
          <div :class="['health-pill', health.state]"><span class="dot"></span><span>{{ health.text }}</span></div>
        </div>
      </DataCard>

      <DataCard title="本地用量采集" subtitle="写入本地 SQLite">
        <div class="config-form-grid">
          <label class="config-field config-field-toggle">
            <span class="config-field-label">记录请求用量</span>
            <button :class="['toggle-switch', {on: mgrMonitoringEnabled}]"
                    @click="mgrMonitoringEnabled = !mgrMonitoringEnabled"
                    :disabled="mgrSaving || !mgrConfigLoaded">
              <span class="toggle-knob"></span>
            </button>
            <small class="muted">{{ mgrMonitoringEnabled ? '已启用' : '已关闭' }}</small>
          </label>
        </div>
        <p class="muted small-text" style="margin-top:8px">CPA Runtime 收到的用量记录会直接写入本地 SQLite，
          用于统计、模型价格与账号巡检。关闭后不会记录新的用量。</p>
      </DataCard>

      <DataCard title="账号处置授权" subtitle="仅用于启用、禁用或删除认证文件">
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">CPA 管理 API 地址</span>
            <input v-model.trim="mgrCPABaseInput" class="control" placeholder="http://127.0.0.1:8317"
                   :disabled="mgrSaving || !mgrConfigLoaded"/>
            <small class="muted">当前绑定: {{ mgrBoundCPABase || '未绑定' }}</small>
          </label>
          <label class="config-field">
            <span class="config-field-label">CPA 管理密钥</span>
            <div class="keybar">
              <input v-model.trim="mgrCPAKeyInput" :type="mgrCPAKeyVisible ? 'text' : 'password'"
                     autocomplete="new-password" placeholder="留空保持不变"
                     :disabled="mgrSaving || !mgrConfigLoaded"/>
              <button class="btn" @click="mgrCPAKeyVisible = !mgrCPAKeyVisible"
                      :disabled="mgrSaving || !mgrConfigLoaded">
                {{ mgrCPAKeyVisible ? '隐藏' : '显示' }}
              </button>
              <button class="btn" @click="mgrCPAKeyInput = ''; mgrCPAKeyVisible = false"
                      :disabled="mgrSaving || !mgrCPAKeyInput">清除
              </button>
            </div>
            <small class="muted">{{ mgrHasBoundKey ? '已绑定密钥（留空不修改）' : '未绑定密钥' }}</small>
          </label>
        </div>
        <p class="muted small-text" style="margin-top:8px">真实账号巡检的 provider 探测，以及启用、禁用或删除认证文件的处置都依赖此配置（加密保存在本地 SQLite）。
          本地请求监控和模型价格同步不依赖此配置；与上方浏览器访问密钥用途不同。</p>
      </DataCard>

      <DataCard title="运行时信息" subtitle="只读">
        <div class="config-meta-grid">
          <div><span>配置来源</span><strong>{{ mgrConfigSourceLabel }}</strong></div>
          <div><span>数据目录</span><strong>{{ mgrLoadedConfig?.dataDir || '—' }}</strong></div>
          <div><span>队列容量</span><strong>{{ mgrLoadedConfig?.collector?.queueCapacity ?? '—' }}</strong></div>
          <div><span>写入批量</span><strong>{{ mgrLoadedConfig?.collector?.batchSize ?? '—' }}</strong></div>
        </div>
        <p class="muted small-text" style="margin-top:8px">队列容量和写入批量在插件启动时读取；如需调整，请修改插件 YAML 后重启 CPA。</p>
      </DataCard>

      <div class="config-save-block">
        <p v-if="!mgrConfigLoaded && resolvedCPAKey" class="muted small-text">正在加载插件配置…</p>
        <p v-else-if="mgrConfigLoaded && !mgrDirty" class="muted small-text">当前与本地 Runtime 配置一致，修改本地用量采集或
          账号处置授权后可保存。</p>
        <p v-if="configSaveMessage" class="notice config-save-ok">{{ configSaveMessage }}</p>
        <div class="config-actions-bar">
          <button class="btn primary" @click="saveManagerConfig" :disabled="mgrSaving || !mgrConfigLoaded || !mgrDirty">
            {{ mgrSaving ? '保存中…' : '保存插件配置' }}
          </button>
          <button class="btn" @click="loadConfig" :disabled="mgrSaving || !resolvedCPAKey">重新加载</button>
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
import {computed, onBeforeUnmount, onMounted, reactive, ref} from 'vue';
import DataCard from './components/DataCard.vue';
import MonitoringView from './components/MonitoringView.vue';
import DashboardView from './components/DashboardView.vue';
import ModelPricesView from './components/ModelPricesView.vue';
import AccountActionsView from './components/AccountActionsView.vue';
import InspectionView from './components/InspectionView.vue';
import {formatHealthText, HEALTH, LEGACY_SESSION_KEY, PROXY, readCPAAuthStoreKey, SESSION_KEY} from './utils/data.js';
import {buildManagerConfigSaveBody} from './utils/managerConfigSave.js';
import {initThemeBridge} from './themeBridge.js';

initThemeBridge();

const tabs = [
  {key: 'dashboard', label: '仪表盘'},
  {key: 'monitoring', label: '请求监控'},
  {key: 'model-prices', label: '模型单价'},
  {key: 'account-actions', label: '认证异常'},
  {key: 'inspection', label: '账号巡检'},
  {key: 'config', label: '配置'},
];
const activeTab = ref('dashboard');
const loading = ref(false);
const health = reactive({state: '', text: '未检测 Runtime'});
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
const configSaveMessage = ref('');

const mgrConfigSourceLabel = computed(() => {
  if (mgrConfigSource.value === 'plugin') return '本地 Runtime';
  if (mgrConfigSource.value === 'env') return '环境变量';
  if (mgrConfigSource.value === 'db') return '数据库';
  return mgrConfigLoaded.value ? '本地 Runtime' : '未加载';
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
const authNotice = computed(() => resolvedCPAKey.value ? '' : '未检测到可用的 CPA management key。请在 CPA 管理台登录并勾选「记住密码」，或在「配置」Tab 临时输入 CPA remote-management.secret-key（仅保存在本页 sessionStorage）。');
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
      if (status === 401) return 'CPA 管理鉴权失败：' + code + msg;
      if (status === 403) return '插件 API 拒绝：' + code + msg;
      if (status === 409) return '配置冲突：' + code + msg;
      return code + msg;
    }
  }
  if (typeof body === 'string' && body.trim()) return body.trim();
  if (status === 401) return 'CPA 管理鉴权失败：请登录管理台或在配置 Tab 输入 CPA remote-management.secret-key';
  if (status === 403) return '插件 API 拒绝：路径或方法不在允许范围内';
  return 'HTTP ' + status;
}

async function proxyCall(payload) {
  if (!resolvedCPAKey.value) throw new Error('missing CPA management key');
  const res = await fetch(PROXY, {method: 'POST', headers: authHeaders(true), body: JSON.stringify(payload)});
  const body = await readJSONResponse(res);
  if (!res.ok) throw new Error(formatError(res.status, body));
  return body;
}

async function checkHealth() {
  if (!resolvedCPAKey.value) {
    health.state = 'err';
    health.text = '缺少 CPA management key';
    return;
  }
  health.state = '';
  health.text = '检测中…';
  try {
    const res = await fetch(HEALTH, {headers: authHeaders(false)});
    const body = await readJSONResponse(res);
    if (res.ok && body && body.ok) {
      health.state = 'ok';
      health.text = formatHealthText(body);
    } else {
      health.state = 'err';
      health.text = formatError(res.status, body) || formatHealthText(body);
    }
  } catch (e) {
    health.state = 'err';
    health.text = e.message || '检测失败';
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
  configSaveMessage.value = '';
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
    errors.config = '配置尚未加载完成，请稍后或点击「重新加载」';
    return;
  }
  if (!mgrDirty.value) return;
  mgrSaving.value = true;
  errors.config = '';
  configSaveMessage.value = '';
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
    configSaveMessage.value = '插件配置已保存并应用';
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
  health.text = '未检测 Runtime';
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

onMounted(() => {
  checkHealth();
  refreshActive();
  window.addEventListener('cpa-manager-plus:open-monitoring', handleOpenMonitoring);
  window.addEventListener('cpa-manager-plus:open-tab', handleOpenTab);
});
onBeforeUnmount(() => {
  window.removeEventListener('cpa-manager-plus:open-monitoring', handleOpenMonitoring);
  window.removeEventListener('cpa-manager-plus:open-tab', handleOpenTab);
});
</script>
