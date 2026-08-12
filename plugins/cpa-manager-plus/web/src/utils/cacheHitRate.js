export function computeCacheHitRate(row) {
  const totalTokens = Math.max(Number(row?.total_tokens ?? 0), 0);
  const inputTokens = Math.max(Number(row?.input_tokens ?? 0), 0);
  const outputTokens = Math.max(Number(row?.output_tokens ?? 0), 0);
  const cachedTokens = Math.max(Number(row?.cached_tokens ?? 0), 0);
  const cacheReadTokens = Math.max(Number(row?.cache_read_tokens ?? 0), 0);
  const cacheCreationTokens = Math.max(Number(row?.cache_creation_tokens ?? 0), 0);
  const hasUsage = totalTokens > 0 || inputTokens > 0 || outputTokens > 0
    || cachedTokens > 0 || cacheReadTokens > 0 || cacheCreationTokens > 0;
  if (!hasUsage) return null;

  const explicitRaw = row?.cache_hit_rate;
  const explicit = Number(explicitRaw);
  if (explicitRaw != null && explicitRaw !== '' && Number.isFinite(explicit)) {
    return Math.min(Math.max(explicit, 0), 1);
  }
  const denominator = Math.max(inputTokens, cachedTokens) + cacheReadTokens + cacheCreationTokens;
  return denominator > 0 ? Math.min((cachedTokens + cacheReadTokens) / denominator, 1) : null;
}

export function formatCacheHitRate(value, formatPercent, emptyValue = '—') {
  return value == null ? emptyValue : formatPercent(value);
}
