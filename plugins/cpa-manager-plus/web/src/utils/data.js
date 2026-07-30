import { translate } from '../i18n/index.js';
import { readManagementKeyFromMCStorage } from './mcAuthStorage.js';
import { EMPTY_VALUE, formatNumber } from './localeFormat.js';

// API gateway path (formerly /proxy). proxyCall keeps the internal name and payload shape.
export const PROXY = '/v0/management/cpa-manager-plus/api';
export const HEALTH = '/v0/management/cpa-manager-plus/health';
export const SESSION_KEY = 'cpa_manager_plus_mgmt_key';
export const LEGACY_SESSION_KEY = 'cpa_mgmt_key';

/** Build management path for account-action candidate operations. */
export function accountActionPath(id, action) {
  const base = `/v0/management/account-action-candidates/${encodeURIComponent(id)}`;
  switch (action) {
    case 'enable':
    case 'ignore':
    case 'resolve':
      return `${base}/${action}`;
    case 'delete':
      return `${base}/auth-file`;
    default:
      throw new Error(`unknown account action: ${action}`);
  }
}

/** Format local runtime health response for the config tab pill. */
export function formatHealthText(body) {
  if (!body || typeof body !== 'object') return translate('health.unknown');
  if (body.ok) {
    const mode = body.mode || 'local';
    const parts = [`${translate('health.normal')} · ${mode}`];
    if (body.data_dir) parts.push(String(body.data_dir));
    else if (body.manager_base_url) parts.push(String(body.manager_base_url));
    if (body.db_ok === false) parts.push(translate('health.databaseError'));
    return parts.join(' · ');
  }
  if (body.error) return String(body.error);
  if (body.manager_base_url) return `${translate('health.unhealthy')} · ${body.manager_base_url}`;
  return translate('health.unhealthy');
}

function readManagementKeyFromParentRuntime() {
  try {
    const parent = window.parent;
    if (!parent || parent === window) return '';
    const parentStore = parent.__CPA_AUTH_STORE__ || parent.cpaAuthStore;
    if (!parentStore || typeof parentStore !== 'object') return '';
    const state = typeof parentStore.getState === 'function' ? parentStore.getState() : parentStore;
    const key = state?.managementKey;
    if (typeof key !== 'string') return '';
    const trimmed = key.trim();
    if (!trimmed || trimmed.startsWith('enc::v1::')) return '';
    return trimmed;
  } catch {
    return '';
  }
}

export function readCPAAuthStoreKey() {
  // 1. Same-origin iframe shares parent's localStorage; try current window first.
  try {
    const key = readManagementKeyFromMCStorage(localStorage);
    if (key) return key;
  } catch {
    /* ignore */
  }

  // 2. Parent localStorage (explicit — some embed paths differ).
  try {
    const key = readManagementKeyFromMCStorage(window.parent.localStorage);
    if (key) return key;
  } catch {
    /* cross-origin or blocked */
  }

  // 3. In-memory MC auth store (logged in without "remember password").
  const runtimeKey = readManagementKeyFromParentRuntime();
  if (runtimeKey) return runtimeKey;

  return '';
}

export function num(v) {
  if (v == null || v === '') return EMPTY_VALUE;
  if (typeof v === 'number') return formatNumber(v, { maximumFractionDigits: 2 });
  return String(v);
}

export function pick(obj, keys) {
  if (!obj || typeof obj !== 'object') return undefined;
  for (const key of keys) {
    if (obj[key] != null) return obj[key];
  }
  return undefined;
}

export function findArray(data) {
  if (Array.isArray(data)) return data;
  if (!data || typeof data !== 'object') return [];
  for (const key of ['items', 'rows', 'events', 'runs', 'data', 'records', 'results']) {
    if (Array.isArray(data[key])) return data[key];
  }
  return [];
}

export function formatCell(v) {
  if (v == null) return EMPTY_VALUE;
  if (typeof v === 'object') return JSON.stringify(v);
  return String(v);
}

export function todayStartQuery() {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return 'today_start_ms=' + d.getTime();
}
