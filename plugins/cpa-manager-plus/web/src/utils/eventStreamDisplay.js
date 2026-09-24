export function recentPatternSummary(pattern = []) {
  const list = Array.isArray(pattern) ? pattern : [];
  return { ok: list.filter(Boolean).length, total: list.length };
}

export function requestedModelName(row = {}) {
  const alias = String(row.alias || row.requested_model || row.requestedModel || '').trim();
  const model = String(row.model || '').trim();
  return alias || model;
}

export function mappedModelName(row = {}) {
  const mapped = String(row.mappedModel || row.resolved_model || row.resolvedModel || '').trim();
  if (mapped) return mapped;
  return String(row.model || '').trim();
}

export function hasModelMapping(row = {}) {
  const requested = requestedModelName(row);
  const mapped = mappedModelName(row);
  return Boolean(requested && mapped && requested !== mapped);
}

// Keep the observed response identity separate from routing and billing.
export function responseModelName(row = {}) {
  return String(row.responseModel || row.response_model || '').trim();
}

export function hasResponseModelDifference(row = {}) {
  const response = responseModelName(row);
  const billed = mappedModelName(row);
  return Boolean(response && billed && response !== billed);
}

export function containsStr(left, right) {
  const a = String(left || '').trim().toLowerCase();
  const b = String(right || '').trim().toLowerCase();
  return Boolean(a && b && (a.includes(b) || b.includes(a)));
}

export function modelNamesContainEachOther(left, right) {
  return containsStr(left, right);
}

// A provider may decorate or normalize a model name while still referring to
// the requested or routed model. Keep those expected variants non-red, but
// retain the response row in the model route popup for visibility.
export function hasResponseModelMismatch(row = {}) {
  const response = responseModelName(row);
  const requested = requestedModelName(row);
  const billed = mappedModelName(row);
  if (!response || !billed || response === billed) return false;

  // 如果 请求模型 跟 响应模型 相互包含，不用显示为红色
  if (containsStr(response, requested)) return false;
  // 如果 路由模型 跟 响应模型 相互包含，不用显示为红色
  if (containsStr(response, billed)) return false;
  // 如果 请求模型 跟 路由模型 相互包含，不用显示为红色
  if (containsStr(requested, billed) && (containsStr(requested, response) || containsStr(billed, response))) {
    return false;
  }
  return true;
}

export function hasResponseModelConflict(row = {}) {
  return (row.responseModelConflict ?? row.response_model_conflict) === true;
}

export function responseModelSource(row = {}) {
  const source = String(row.responseModelSource ?? row.response_model_source ?? '').trim();
  return ['host', 'observer', 'confirmed'].includes(source) ? source : '';
}

export function hasModelRouteDetails(row = {}) {
  return hasModelMapping(row) || hasResponseModelDifference(row) || hasResponseModelConflict(row);
}

export function buildModelMeta({ intensity, tier } = {}, t) {
  const parts = [];
  if (intensity && intensity !== '-') {
    parts.push(t('monitoring.eventMeta.intensity', { value: intensity }));
  }
  if (tier && tier !== 'default') {
    parts.push(t('monitoring.labels.level', { value: tier }));
  }
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
  const requested = requestedModelName(row);
  const mapped = mappedModelName(row);
  const intensity = row.intensity && row.intensity !== '-' ? row.intensity : t('monitoring.eventMeta.none');
  const tier = row.tier || '';
  const modelHint = mapped && mapped !== requested
    ? t('monitoring.eventHints.modelMapped', { model: requested, mapped, intensity, tier })
    : t('monitoring.eventHints.model', { model: requested, intensity, tier });
  return {
    model: hasResponseModelDifference(row)
      ? `${modelHint} · ${t('monitoring.eventMeta.responseModel')}: ${responseModelName(row)}`
      : modelHint,
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
