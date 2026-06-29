<template>
  <main class="page">
    <section class="hero">
      <div class="hero-main">
        <div>
          <div class="eyebrow">CPA PLUGIN · COMPILED VUE APP · MANAGER PLUS DATA</div>
          <h1>CPA Manager Plus</h1>
          <p class="subtitle">数据经 CPA 同源 Management API 到插件，再反向代理到 Plus Manager Server。</p>
        </div>
        <div :class="['health-pill', health.state]"><span class="dot"></span><span>{{ health.text }}</span></div>
      </div>
    </section>

    <section class="toolbar">
      <nav class="tabs" aria-label="CPA Manager Plus tabs">
        <button v-for="tab in tabs" :key="tab.key" :class="['tab', {active: activeTab === tab.key}]" @click="selectTab(tab.key)">{{ tab.label }}</button>
      </nav>
      <div class="row">
        <button class="btn" @click="checkHealth" :disabled="loading">检测 Manager</button>
        <button class="btn primary" @click="refreshActive" :disabled="loading">{{ loading ? '加载中…' : '刷新当前 Tab' }}</button>
      </div>
    </section>

    <section v-if="authNotice" class="notice">{{ authNotice }}</section>
    <section v-if="activeError" class="notice error">{{ activeError }}</section>

    <section class="panel" v-if="activeTab === 'dashboard'">
      <MetricGrid :cards="dashboardCards" />
      <div class="split">
        <DataCard title="今日概览" subtitle="/v0/management/dashboard/summary">
          <DataTable :rows="dashboardRows" :preferred-keys="['key','value']" />
        </DataCard>
        <DataCard title="原始摘要" subtitle="调试视图" glass>
          <pre>{{ pretty(dashboardData) }}</pre>
        </DataCard>
      </div>
    </section>

    <section class="panel" v-if="activeTab === 'usage'">
      <MetricGrid :cards="usageCards" />
      <DataCard title="用量记录" subtitle="/v0/management/usage">
        <DataTable :rows="usageRows" :preferred-keys="['date','day','model','provider','authIndex','requests','tokens','cost','inputTokens','outputTokens']" />
      </DataCard>
      <pre>{{ pretty(usageData) }}</pre>
    </section>

    <section class="panel" v-if="activeTab === 'monitoring'">
      <MetricGrid :cards="monitoringCards" />
      <DataCard title="监控分析" subtitle="/v0/management/monitoring/analytics">
        <DataTable :rows="monitoringRows" :preferred-keys="['time','createdAt','provider','model','authIndex','status','statusCode','error','latencyMs']" />
      </DataCard>
      <pre>{{ pretty(monitoringData) }}</pre>
    </section>

    <section class="panel" v-if="activeTab === 'inspection'">
      <MetricGrid :cards="inspectionCards" />
      <DataCard title="巡检批次" subtitle="/v0/management/codex-inspection/runs">
        <DataTable :rows="inspectionRows" :preferred-keys="['id','status','triggerType','startedAtMs','finishedAtMs','totalFiles','disabledCount','error']" />
      </DataCard>
      <pre>{{ pretty(inspectionData) }}</pre>
    </section>

    <section class="panel" v-if="activeTab === 'config'">
      <DataCard title="访问凭据" subtitle="sessionStorage only">
        <p class="muted">这里输入的是 CPA <code>remote-management.secret-key</code>，用于浏览器访问 CPA 的 <code>/v0/management/*</code>。它不是插件 YAML 里的 Plus <code>management_key</code>。</p>
        <div class="keybar">
          <input v-model.trim="cpaKeyInput" type="password" autocomplete="off" placeholder="CPA management key（当前会话临时保存）" @keyup.enter="saveCPAKey" />
          <button class="btn primary" @click="saveCPAKey">保存并检测</button>
          <button class="btn danger" @click="clearCPAKey">清除</button>
        </div>
      </DataCard>
      <div class="split">
        <DataCard title="Manager 配置" subtitle="/usage-service/config">
          <DataTable :rows="configRows" :preferred-keys="['key','value']" />
        </DataCard>
        <DataCard title="连接信息" subtitle="插件配置" glass>
          <pre>{{ pretty(configData) }}</pre>
        </DataCard>
      </div>
    </section>
  </main>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue';
import DataCard from './components/DataCard.vue';
import DataTable from './components/DataTable.vue';
import MetricGrid from './components/MetricGrid.vue';
import { PROXY, HEALTH, SESSION_KEY, LEGACY_SESSION_KEY, readCPAAuthStoreKey, pick, findArray, formatCell, todayStartQuery } from './utils/data.js';

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
const dashboardData = ref(null);
const usageData = ref(null);
const monitoringData = ref(null);
const inspectionData = ref(null);
const configData = ref(null);

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
const dashboardRows = computed(() => Object.entries(dashboardData.value || {}).map(([key,value]) => ({key, value: formatCell(value)})));
const usageRows = computed(() => findArray(usageData.value));
const monitoringRows = computed(() => findArray(monitoringData.value));
const inspectionRows = computed(() => findArray(inspectionData.value));
const configRows = computed(() => {
  const d = configData.value && configData.value.config ? configData.value.config : (configData.value || {});
  return Object.entries(d).map(([key,value]) => ({key, value: formatCell(value)}));
});
const dashboardCards = computed(() => {
  const d = dashboardData.value || {};
  return [
    {label:'请求量', value: pick(d, ['requests','requestCount','totalRequests','total'])},
    {label:'成功', value: pick(d, ['success','successCount','successfulRequests'])},
    {label:'失败', value: pick(d, ['failed','failedCount','errors','errorCount'])},
    {label:'Token / 用量', value: pick(d, ['tokens','totalTokens','usage','cost'])},
  ];
});
const usageCards = computed(() => {
  const d = usageData.value || {};
  const rows = usageRows.value;
  return [
    {label:'记录数', value: rows.length},
    {label:'总请求', value: pick(d, ['totalRequests','requests','total'])},
    {label:'总 Token', value: pick(d, ['totalTokens','tokens'])},
    {label:'费用', value: pick(d, ['totalCost','cost','amount'])},
  ];
});
const monitoringCards = computed(() => {
  const d = monitoringData.value || {};
  const rows = monitoringRows.value;
  return [
    {label:'事件数', value: rows.length || pick(d, ['events','eventCount','total'])},
    {label:'失败', value: pick(d, ['failed','failedCount','errors','errorCount'])},
    {label:'账号数', value: pick(d, ['accounts','accountCount'])},
    {label:'模型数', value: pick(d, ['models','modelCount'])},
  ];
});
const inspectionCards = computed(() => {
  const rows = inspectionRows.value;
  const last = rows[0] || {};
  return [
    {label:'巡检批次', value: rows.length},
    {label:'最近状态', value: last.status || '—'},
    {label:'禁用', value: pick(last, ['disabledCount','disableCount'])},
    {label:'错误', value: pick(last, ['errorCount','failedCount']) || (last.error ? 1 : 0)},
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
    if(activeTab.value === 'dashboard') await loadDashboard();
    if(activeTab.value === 'usage') await loadUsage();
    if(activeTab.value === 'monitoring') await loadMonitoring();
    if(activeTab.value === 'inspection') await loadInspection();
    if(activeTab.value === 'config') await loadConfig();
  }catch(e){ errors[activeTab.value] = e.message || String(e); }
  finally{ loading.value = false; }
}
async function loadDashboard(){ dashboardData.value = await proxyCall({method:'GET', path:'/v0/management/dashboard/summary', query:todayStartQuery()}); }
async function loadUsage(){ usageData.value = await proxyCall({method:'GET', path:'/v0/management/usage'}); }
async function loadMonitoring(){ monitoringData.value = await proxyCall({method:'GET', path:'/v0/management/monitoring/analytics'}); }
async function loadInspection(){ inspectionData.value = await proxyCall({method:'GET', path:'/v0/management/codex-inspection/runs'}); }
async function loadConfig(){ configData.value = await proxyCall({method:'GET', path:'/usage-service/config'}); }
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

onMounted(() => { checkHealth(); refreshActive(); });
</script>
