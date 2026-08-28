import { describe, expect, it } from 'vitest';
import {
  QUOTA_ERROR_COOLDOWN_MS,
  QUOTA_PROBE_COOLDOWN_MS,
  clearAllQuotaCache,
  getOrCreateQuotaRequest,
  getQuotaCacheEntry,
  quotaCacheKey,
  setQuotaCacheEntry,
} from './quotaCache.js';

describe('quota cache', () => {
  it('uses stable identity fields instead of analytics row ids', () => {
    const first = quotaCacheKey({
      id: 'row-1',
      auth_id: 'AUTH-1',
      auth_provider_snapshot: 'Codex',
      auth_type: 'OAuth',
    });
    const second = quotaCacheKey({
      id: 'row-2',
      auth_id: 'auth-1',
      auth_provider_snapshot: 'codex',
      auth_type: 'oauth',
    });
    expect(first).toBe(second);
  });

  it('stores the next safe request time with each result', () => {
    clearAllQuotaCache();
    const entry = setQuotaCacheEntry('account', { quotaWindows: [] }, QUOTA_PROBE_COOLDOWN_MS, 1000);
    expect(entry).toEqual({
      result: { quotaWindows: [] },
      fetchedAt: 1000,
      nextRequestAt: 1000 + QUOTA_PROBE_COOLDOWN_MS,
    });
    expect(getQuotaCacheEntry('account')).toEqual(entry);
  });

  it('deduplicates concurrent requests for the same key', async () => {
    let calls = 0;
    let resolveRequest;
    const pending = new Promise(resolve => { resolveRequest = resolve; });
    const first = getOrCreateQuotaRequest('same', async () => {
      calls += 1;
      return pending;
    });
    const second = getOrCreateQuotaRequest('same', async () => {
      calls += 1;
      return 'unexpected';
    });

    expect(second).toBe(first);
    expect(calls).toBe(0);
    resolveRequest('done');
    await expect(first).resolves.toBe('done');
    expect(calls).toBe(1);
  });

  it('supports a shorter cooldown for transport errors', () => {
    clearAllQuotaCache();
    const entry = setQuotaCacheEntry('error', { actionReason: 'network error' }, QUOTA_ERROR_COOLDOWN_MS, 5000);
    expect(entry.nextRequestAt).toBe(5000 + QUOTA_ERROR_COOLDOWN_MS);
  });
});
