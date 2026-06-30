<template>
  <main class="page">
    <section class="toolbar">
      <nav class="tabs" aria-label="CPA Manager Plus tabs">
        <button v-for="tab in tabs" :key="tab.key" :class="['tab', {active: activeTab === tab.key}]" @click="selectTab(tab.key)">{{ tab.label }}</button>
      </nav>
    </section>

    <section v-if="authNotice" class="notice">{{ authNotice }}</section>
    <section v-if="activeError" class="notice error">{{ activeError }}</section>

    <section class="panel" v-if="activeTab === 'dashboard'">
      <DashboardView ref="dashboardView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall" />
    </section>

    <section class="panel" v-if="activeTab === 'monitoring'">
      <MonitoringView ref="monitoringView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall" />
    </section>

    <section class="panel" v-if="activeTab === 'usage'">
      <UsageView ref="usageView" :ready="!!resolvedCPAKey" :proxy-call="proxyCall" />
    </section>

    <section class="panel" v-if="activeTab === 'inspection'">
      <MetricGrid :cards="inspectionCards" />
      <DataCard title="巡检批次" subtitle="/v0/management/codex-inspection/runs">
        <DataTable :rows="inspectionRows" :preferred-keys="['id','status','triggerType','startedAtMs','finishedAtMs','totalFiles','disabledCount','error']" />
      </DataCard>
      <pre>{{ pretty(inspectionData) }}</pre>
    </section>

    <section class="panel" v-if="activeTab === 'config'">
      <DataCard title="访问凭据" subtitle="仅浏览器缓存">
        <p class="muted">这里输入的是 CPA <code>remote-management.secret-key</code>，用于浏览器访问 CPA 的 <code>/v0/management/*</code>。它不是插件 YAML 里的 Plus <code>management_key</code>。</p>
        <div class="keybar">
          <input v-model.trim="cpaKeyInput" type="password" autocomplete="off" placeholder="CPA management key（当前会话临时保存）" @keyup.enter="saveCPAKey" />
          <button class="btn primary" @click="saveCPAKey">保存并检测</button>
          <button class="btn" @click="checkHealth" :disabled="loading">检测 Manager</button>
          <button class="btn danger" @click="clearCPAKey">清除</button>
        </div>
        <div :class="['health-pill', health.state]"><span class="dot"></span><span>{{ health.text }}</span></div>
      </DataCard>

      <DataCard title="CPA 连接配置" subtitle="Manager Server → CPA">
        <div class="config-form-grid">
          <label class="config-field">
            <span class="config-field-label">CPA Base URL</span>
            <input v-model.trim="mgrCPABaseInput" class="control" placeholder="http://127.0.0.1:8317" :disabled="mgrSaving" />
            <small class="muted">当前绑定: {{ mgrBoundCPABase || '未绑定' }}</small>
          </label>
          <label class="config-field">
            <span class="config-field-label">CPA Management Key</span>
            <div class="keybar">
              <input v-model.trim="mgrCPAKeyInput" :type="mgrCPAKeyVisible ? 'text' : 'password'" autocomplete="new-password" placeholder="留空保持不变" :disabled="mgrSaving" />
              <button class="btn" @click="mgrCPAKeyVisible = !mgrCPAKeyVisible" :disabled="mgrSaving">{{ mgrCPAKeyVisible ? '隐藏' : '显示' }}</button>
              <button class="btn" @click="mgrCPAKeyInput = ''; mgrCPAKeyVisible = false" :disabled="mgrSaving || !mgrCPAKeyInput">清除</button>
            </div>
            <small class="muted">{{ mgrHasBoundKey ? '已绑定密钥（留空不修改）' : '未绑定密钥' }}</small>
          </label>
        </div>
        <p class="muted small-text" style="margin-top:8px">修改 CPA 连接可能导致 Manager Server 与 CPA 断开，请确认后保存。</p>
      </DataCard>

      <DataCard title="请求监控配置" subtitle="Collector">
        <div class="config-form-grid">
          <label class="config-field config-field-toggle">
            <span class="config-field-label">请求监控</span>
            <button :class="['toggle-switch', {on: mgrMonitoringEnabled}]" @click="mgrMonitoringEnabled = !mgrMonitoringEnabled" :disabled="mgrSaving || !canConfigureMonitoring">
              <span class="toggle-knob"></span>
            </button>
            <small class="muted">{{ mgrMonitoringEnabled ? '已启用' : '已关闭' }}</small>
          </label>
          <label class="config-field">
            <span class="config-field-label">Collector 模式</span>
            <select v-model="mgrCollectorMode" class="control" :disabled="mgrSaving || !mgrMonitoringEnabled || !canConfigureMonitoring">
              <option value="auto">自动</option>
              <option value="http">HTTP</option>
              <option value="resp">RESP</option>
              <option value="subscribe">Subscribe</option>
            </select>
          </label>
          <label class="config-field">
            <span class="config-field-label">轮询间隔 (ms)</span>
            <input v-model.trim="mgrPollIntervalMs" type="number" min="1" class="control" placeholder="500" :disabled="mgrSaving || !mgrMonitoringEnabled || !canConfigureMonitoring" />
            <small class="muted">须 ≤ CPA retention ({{ mgrRetentionSeconds }}s)</small>
          </label>
          <label class="config-field">
            <span class="config-field-label">批量大小</span>
            <input v-model.trim="mgrBatchSize" type="number" min="1" class="control" placeholder="100" :disabled="mgrSaving || !mgrMonitoringEnabled || !canConfigureMonitoring" />
          </label>
          <label class="config-field">
            <span class="config-field-label">查询限制</span>
            <input v-model.trim="mgrQueryLimit" type="number" min="1" class="control" placeholder="50000" :disabled="mgrSaving || !mgrMonitoringEnabled || !canConfigureMonitoring" />
          </label>
        </div>
        <div v-if="!canConfigureMonitoring" class="notice" style="margin-top:8px">需先填写 CPA Base URL 和 Management Key 才能配置监控。</div>
      </DataCard>

      <div class="config-actions-bar">
        <button class="btn primary" @click="saveManagerConfig" :disabled="mgrSaving || !mgrDirty">
          {{ mgrSaving ? '保存中…' : '保存 Manager 配置' }}
        </button>
        <button class="btn" @click="loadConfig" :disabled="mgrSaving">重新加载</button>
      </div>

      <DataCard title="配置元信息" subtitle="status">
        <div class="config-meta-grid">
          <div><span>配置来源</span><strong>{{ mgrConfigSourceLabel }}</strong></div>
          <div><span>CPA Usage</span><strong>{{ mgrUsageEnabled ? '已启用' : '未启用' }}</strong></div>
          <div><span>CPA Retention</span><strong>{{ mgrRetentionSeconds }}s</strong></div>
        </div>
      </DataCard>
    </section>
  </main>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue';
import DataCard from './components/DataCard.vue';
import DataTable from './components/DataTable.vue';
import MonitoringView from './components/MonitoringView.vue';
import UsageView from './components/UsageView.vue';
import DashboardView from './components/DashboardView.vue';
import { PROXY, HEALTH, SESSION_KEY, LEGACY_SESSION_KEY, readCPAAuthStoreKey } from './utils/data.js';
import { initThemeBridge } from './themeBridge.js';

initThemeBridge();

const tabs = [
  {key:'dashboard', label:'仪表盘'},
  {key:'usage', label:'用量分析'},
  {key:'monitoring', label:'请求监控'},
  {key:'inspection', label:'账号巡检'},
  {key:'config', label:'配置'},
];
const activeTab = ref('dashboard');
const loading = ref(false);
const health = reactive({state:'', text:'未检测 Manager'});
const cpaKeyInput = ref((sessionStorage.getItem(SESSION_KEY) || '').trim());
const errors = reactive({});
const inspectionData = ref(null);
const configData = ref(null);
const dashboardView = ref(null);
const monitoringView = ref(null);
const usageView = ref(null);

// Manager config state
const mgrSaving = ref(false);
const mgrCPABaseInput = ref('');
const mgrCPAKeyInput = ref('');
const mgrCPAKeyVisible = ref(false);
const mgrMonitoringEnabled = ref(true);
const mgrCollectorMode = ref('auto');
const mgrPollIntervalMs = ref('500');
const mgrBatchSize = ref('100');
const mgrQueryLimit = ref('50000');
const mgrBoundCPABase = ref('');
const mgrHasBoundKey = ref(false);
const mgrConfigSource = ref('');
const mgrUsageEnabled = ref(false);
const mgrRetentionSeconds = ref(60);
const mgrLoadedConfig = ref(null);

const mgrConfigSourceLabel = computed(() => {
  if(mgrConfigSource.value === 'env') return '环境变量';
  if(mgrConfigSource.value === 'db') return '数据库';
  return '未配置';
});
const canConfigureMonitoring = computed(() => Boolean(mgrCPABaseInput.value.trim() && (mgrCPAKeyInput.value.trim() || mgrHasBoundKey.value)));
const mgrDirty = computed(() => {
  if(!mgrLoadedConfig.value) return false;
  const c = mgrLoadedConfig.value;
  if(mgrCPABaseInput.value !== (c.cpaConnection?.cpaBaseUrl || '')) return true;
  if(mgrCPAKeyInput.value.trim()) return true;
  if(mgrMonitoringEnabled.value !== (c.collector?.enabled !== false)) return true;
  if(mgrCollectorMode.value !== (c.collector?.collectorMode || 'auto')) return true;
  if(mgrPollIntervalMs.value !== String(c.collector?.pollIntervalMs || 500)) return true;
  if(mgrBatchSize.value !== String(c.collector?.batchSize || 100)) return true;
  if(mgrQueryLimit.value !== String(c.collector?.queryLimit || 50000)) return true;
  return false;
});

const resolvedCPAKey = computed(() => {
  const input = (cpaKeyInput.value || '').trim();
  if(input) return input;
  const session = (sessionStorage.getItem(SESSION_KEY) || '').trim();
  if(session) return session;
  const store = readCPAAuthStoreKey();
  if(store) return store;
  return (sessionStorage.getItem(LEGACY_SESSION_KEY) || '').trim();
});
const authNotice = computed(() => resolvedCPAKey.value ? '' : '未检测到可用的 CPA management key。若 CPA 管理台保存的是 enc::v1:: 加密密钥，插件页无法解密；请在「配置」Tab 临时输入 CPA remote-management.secret-key。本字段只保存在 sessionStorage，不写入插件 YAML。');
const activeError = computed(() => errors[activeTab.value] || '');
const inspectionRows = computed(() => Array.isArray(inspectionData.value) ? inspectionData.value : (inspectionData.value?.items || inspectionData.value?.runs || []));

const inspectionCards = computed(() => {
  const rows = inspectionRows.value;
  const last = rows[0] || {};
  return [
    {label:'巡检批次', value: rows.length},
    {label:'最近状态', value: last.status || '—'},
    {label:'禁用', value: last.disabledCount ?? last.disableCount ?? '—'},
    {label:'错误', value: last.errorCount ?? last.failedCount ?? (last.error ? 1 : 0)},
  ];
});
function pretty(data){ return data == null ? '等待加载' : JSON.stringify(data, null, 2); }
function authHeaders(json=true){
  const headers = json ? {'Content-Type':'application/json','Accept':'application/json'} : {'Accept':'application/json'};
  const key = resolvedCPAKey.value;
  if(!key) return headers;
  const clean = key.replace(/^Bearer\s+/i,'');
  headers.Authorization = 'Bearer ' + clean;
  headers['X-Management-Key'] = clean;
  return headers;
}
async function readJSONResponse(res){
  const text = await res.text();
  if(!text) return null;
  try{ return JSON.parse(text); }catch{ return text; }
}
function formatError(status, body){
  const msg = body && (body.error || body.message) ? (body.error || body.message) : '';
  if(status === 401) return 'CPA 管理鉴权失败：请登录管理台或在配置 Tab 输入 CPA remote-management.secret-key';
  if(status === 403) return '插件代理拒绝：' + (msg || '路径或方法不在允许范围内');
  return msg || ('HTTP ' + status);
}
async function proxyCall(payload){
  if(!resolvedCPAKey.value) throw new Error('missing CPA management key');
  const res = await fetch(PROXY, {method:'POST', headers:authHeaders(true), body:JSON.stringify(payload)});
  const body = await readJSONResponse(res);
  if(!res.ok) throw new Error(formatError(res.status, body));
  return body;
}
async function checkHealth(){
  if(!resolvedCPAKey.value){ health.state = 'err'; health.text = '缺少 CPA management key'; return; }
  health.state = ''; health.text = '检测中…';
  try{
    const res = await fetch(HEALTH, {headers:authHeaders(false)});
    const body = await readJSONResponse(res);
    if(res.ok && body && body.ok){ health.state = 'ok'; health.text = 'Manager 可达 · ' + (body.manager_base_url || ''); }
    else { health.state = 'err'; health.text = formatError(res.status, body); }
  }catch(e){ health.state = 'err'; health.text = e.message || '检测失败'; }
}
function selectTab(tab){ activeTab.value = tab; refreshActive(); }
async function refreshActive(){
  if(loading.value) return;
  loading.value = true;
  errors[activeTab.value] = '';
  try{
    if(activeTab.value === 'dashboard') await (dashboardView.value ? dashboardView.value.refresh(true) : Promise.resolve());
    if(activeTab.value === 'usage') await (usageView.value ? usageView.value.refresh(true) : Promise.resolve());
    if(activeTab.value === 'monitoring') await (monitoringView.value ? monitoringView.value.refresh(true) : Promise.resolve());
    if(activeTab.value === 'inspection') await loadInspection();
    if(activeTab.value === 'config') await loadConfig();
  }catch(e){ errors[activeTab.value] = e.message || String(e); }
  finally{ loading.value = false; }
}
async function loadInspection(){ inspectionData.value = await proxyCall({method:'GET', path:'/v0/management/codex-inspection/runs'}); }
async function loadConfig(){
  const resp = await proxyCall({method:'GET', path:'/usage-service/config'});
  configData.value = resp;
  const cfg = resp?.config || resp || {};
  mgrLoadedConfig.value = cfg;
  mgrConfigSource.value = resp?.source || '';
  mgrCPABaseInput.value = cfg.cpaConnection?.cpaBaseUrl || '';
  mgrBoundCPABase.value = cfg.cpaConnection?.cpaBaseUrl || '';
  mgrHasBoundKey.value = Boolean(cfg.cpaConnection?.managementKey);
  mgrCPAKeyInput.value = '';
  mgrCPAKeyVisible.value = false;
  mgrMonitoringEnabled.value = cfg.collector?.enabled !== false;
  mgrCollectorMode.value = cfg.collector?.collectorMode || 'auto';
  mgrPollIntervalMs.value = String(cfg.collector?.pollIntervalMs || 500);
  mgrBatchSize.value = String(cfg.collector?.batchSize || 100);
  mgrQueryLimit.value = String(cfg.collector?.queryLimit || 50000);
  mgrUsageEnabled.value = Boolean(resp?.cpaUsage?.usageStatisticsEnabled);
  mgrRetentionSeconds.value = resp?.cpaUsage?.redisUsageQueueRetentionSeconds || 60;
}
async function saveManagerConfig(){
  if(!mgrDirty.value) return;
  mgrSaving.value = true;
  try{
    const c = mgrLoadedConfig.value || {};
    const cpaConnection = {...(c.cpaConnection || {}), cpaBaseUrl: mgrCPABaseInput.value.trim()};
    if(mgrCPAKeyInput.value.trim()) cpaConnection.managementKey = mgrCPAKeyInput.value.trim();
    const nextConfig = {
      ...c,
      cpaConnection,
      collector: {
        ...(c.collector || {}),
        enabled: mgrMonitoringEnabled.value,
        collectorMode: mgrCollectorMode.value,
        pollIntervalMs: Number(mgrPollIntervalMs.value) || 500,
        batchSize: Number(mgrBatchSize.value) || 100,
        queryLimit: Number(mgrQueryLimit.value) || 50000,
      },
      externalUsageService: c.externalUsageService || {enabled:false, serviceBase:''},
    };
    await proxyCall({method:'PUT', path:'/usage-service/config', body:nextConfig});
    await loadConfig();
  }catch(e){
    errors.config = e.message || String(e);
  }finally{
    mgrSaving.value = false;
  }
}
function saveCPAKey(){
  const key = (cpaKeyInput.value || '').trim().replace(/^Bearer\s+/i,'');
  if(key) sessionStorage.setItem(SESSION_KEY, key);
  cpaKeyInput.value = key;
  checkHealth();
  refreshActive();
}
function clearCPAKey(){
  cpaKeyInput.value = '';
  sessionStorage.removeItem(SESSION_KEY);
  sessionStorage.removeItem(LEGACY_SESSION_KEY);
  health.state = ''; health.text = '未检测 Manager';
}
function handleOpenMonitoring(){
  activeTab.value = 'monitoring';
  setTimeout(() => { refreshActive(); }, 0);
}

onMounted(() => {
  checkHealth();
  refreshActive();
  window.addEventListener('cpa-manager-plus:open-monitoring', handleOpenMonitoring);
});
onBeforeUnmount(() => {
  window.removeEventListener('cpa-manager-plus:open-monitoring', handleOpenMonitoring);
});
</script>
