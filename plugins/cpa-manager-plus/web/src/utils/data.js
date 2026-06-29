export const PROXY = '/v0/management/cpa-manager-plus/proxy';
export const HEALTH = '/v0/management/cpa-manager-plus/health';
export const SESSION_KEY = 'cpa_manager_plus_mgmt_key';
export const LEGACY_SESSION_KEY = 'cpa_mgmt_key';

export function readCPAAuthStoreKey(){
  try{
    const raw = localStorage.getItem('cli-proxy-auth');
    if(!raw) return '';
    const parsed = JSON.parse(raw);
    const st = parsed && parsed.state ? parsed.state : parsed;
    const key = (st && st.managementKey) || '';
    if(typeof key !== 'string' || !key || key.startsWith('enc::v1::')) return '';
    return key.trim();
  }catch(_){ return ''; }
}

export function num(v){
  if(v == null || v === '') return '—';
  if(typeof v === 'number') return new Intl.NumberFormat('zh-CN', {maximumFractionDigits: 2}).format(v);
  return String(v);
}

export function pick(obj, keys){
  if(!obj || typeof obj !== 'object') return undefined;
  for(const key of keys){ if(obj[key] != null) return obj[key]; }
  return undefined;
}

export function findArray(data){
  if(Array.isArray(data)) return data;
  if(!data || typeof data !== 'object') return [];
  for(const key of ['items','rows','events','runs','data','records','results']){
    if(Array.isArray(data[key])) return data[key];
  }
  return [];
}

export function formatCell(v){
  if(v == null) return '—';
  if(typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

export function todayStartQuery(){
  const d = new Date();
  d.setHours(0,0,0,0);
  return 'today_start_ms=' + d.getTime();
}
