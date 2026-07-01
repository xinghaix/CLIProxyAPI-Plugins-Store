export const PROXY = '/v0/management/cpa-manager-plus/proxy';
export const HEALTH = '/v0/management/cpa-manager-plus/health';
export const SESSION_KEY = 'cpa_manager_plus_mgmt_key';
export const LEGACY_SESSION_KEY = 'cpa_mgmt_key';

export function readCPAAuthStoreKey(){
  // 1. Try iframe's own localStorage (same-origin as parent CPA host)
  try{
    const raw = localStorage.getItem('cli-proxy-auth');
    if(raw){
      const parsed = JSON.parse(raw);
      const st = parsed && parsed.state ? parsed.state : parsed;
      const key = (st && st.managementKey) || '';
      if(typeof key === 'string' && key && !key.startsWith('enc::v1::')) return key.trim();
    }
  }catch(_){ }

  // 2. Try parent window's localStorage (same-origin iframe shares it,
  //    but the store name or key nesting may differ between versions)
  try{
    const pRaw = window.parent.localStorage.getItem('cli-proxy-auth');
    if(pRaw){
      const parsed = JSON.parse(pRaw);
      const st = parsed && parsed.state ? parsed.state : parsed;
      const key = (st && st.managementKey) || '';
      if(typeof key === 'string' && key && !key.startsWith('enc::v1::')) return key.trim();
    }
  }catch(_){ }

  // 3. Try reading from parent window's runtime state (zustand store).
  //    CPA management center may expose the auth store on window object.
  try{
    const parentStore = window.parent.__CPA_AUTH_STORE__ || window.parent.cpaAuthStore;
    if(parentStore && typeof parentStore === 'object'){
      const key = parentStore.getState ? parentStore.getState().managementKey : parentStore.managementKey;
      if(typeof key === 'string' && key && !key.startsWith('enc::v1::')) return key.trim();
    }
  }catch(_){ }

  return '';
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