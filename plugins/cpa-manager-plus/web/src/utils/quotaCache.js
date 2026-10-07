import { isProbeFailure } from './credentialPresentation.js';

export const QUOTA_PROBE_COOLDOWN_MS = 5 * 60 * 1000;
export const QUOTA_ERROR_COOLDOWN_MS = 60 * 1000;

const cache = new Map();
const inflight = new Map();

function normalize(value) {
  return String(value ?? '').trim().toLowerCase();
}

export function quotaCacheKey(row) {
  const provider = normalize(row?.auth_provider_snapshot || row?.provider);
  const authType = normalize(row?.auth_type);
  const authID = normalize(row?.auth_id);
  if (authID) return ['id', provider, authType, authID].join('|');
  const composite = [
    provider,
    authType,
    normalize(row?.auth_index),
    normalize(row?.file_name),
    normalize(row?.source),
  ];
  if (composite.some(Boolean)) return ['composite', ...composite].join('|');
  return ['row', normalize(row?.id)].join('|');
}

export function getQuotaCacheEntry(key, nowMs = Date.now()) {
  const entry = cache.get(key);
  if (!entry || nowMs >= entry.nextRequestAt) {
    cache.delete(key);
    return null;
  }
  return entry;
}

export function setQuotaCacheEntry(key, result, cooldownMs = QUOTA_PROBE_COOLDOWN_MS, fetchedAt = Date.now()) {
  const numericCooldown = Number(cooldownMs);
  const safeCooldown = Number.isFinite(numericCooldown) ? Math.max(0, numericCooldown) : QUOTA_PROBE_COOLDOWN_MS;
  const numericFetchedAt = Number(fetchedAt);
  const safeFetchedAt = Number.isFinite(numericFetchedAt) ? numericFetchedAt : Date.now();
  const entry = {
    result,
    fetchedAt: safeFetchedAt,
    nextRequestAt: safeFetchedAt + safeCooldown,
  };
  cache.set(key, entry);
  return entry;
}

export function getOrCreateQuotaRequest(key, factory) {
  const existing = inflight.get(key);
  if (existing) return existing;
  const request = Promise.resolve()
    .then(factory)
    // Both pages resolve the same wrapper, regardless of which factory won.
    .then(value => {
      const response = value && Object.hasOwn(value, 'result') ? value : { result: value };
      return {
        result: response.result,
        cooldownMs: isProbeFailure(response.result) ? QUOTA_ERROR_COOLDOWN_MS : (response.cooldownMs ?? QUOTA_PROBE_COOLDOWN_MS),
      };
    })
    .finally(() => {
      if (inflight.get(key) === request) inflight.delete(key);
    });
  inflight.set(key, request);
  return request;
}

export function clearQuotaCache(key) {
  cache.delete(key);
}

export function clearQuotaCacheForRow(row) {
  clearQuotaCache(quotaCacheKey(row));
}

export function clearAllQuotaCache() {
  cache.clear();
}
