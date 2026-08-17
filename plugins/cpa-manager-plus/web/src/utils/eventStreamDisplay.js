export function recentPatternSummary(pattern = []) {
  const list = Array.isArray(pattern) ? pattern : [];
  return { ok: list.filter(Boolean).length, total: list.length };
}

export function buildModelMeta({ intensity, tier, resolvedModel, model } = {}, t) {
  const parts = [];
  if (intensity && intensity !== '-') {
    parts.push(t('monitoring.eventMeta.intensity', { value: intensity }));
  }
  if (tier && tier !== 'default') {
    parts.push(t('monitoring.labels.level', { value: tier }));
  }
  if (resolvedModel && resolvedModel !== model) parts.push(resolvedModel);
  return parts.join(' · ');
}

export function formatCallsSub(calls, t) {
  return t('monitoring.eventMeta.calls', { value: calls });
}

export function formatTpsSub(tps, t) {
  return t('monitoring.eventMeta.tps', { value: tps });
}

export function formatCacheSub(rate, t) {
  return t('monitoring.eventMeta.cache', { value: rate });
}

export function buildEventHints(row, t) {
  const recent = recentPatternSummary(row.recentPattern);
  return {
    model: t('monitoring.eventHints.model', {
      model: row.model,
      intensity: row.intensity && row.intensity !== '-' ? row.intensity : t('monitoring.eventMeta.none'),
      tier: row.tier || '',
    }),
    status: t('monitoring.eventHints.status', {
      status: row.failed ? t('monitoring.labels.failed') : t('monitoring.labels.success'),
      protocol: row.protocolLabel,
      ok: recent.ok,
      total: recent.total,
    }),
    health: t('monitoring.eventHints.health', { rate: row.successRateText, calls: row.totalCallsText }),
    speed: t('monitoring.eventHints.speed', {
      ttft: row.ttftText,
      latency: row.latencyText,
      tps: row.tps == null ? row.tpsText : `${row.tpsText} tok/s`,
    }),
    usage: t('monitoring.eventHints.usage', { total: row.totalTokensText, breakdown: row.usageText }),
    cost: t('monitoring.eventHints.cost', { cost: row.costText, cache: row.cacheText }),
  };
}
