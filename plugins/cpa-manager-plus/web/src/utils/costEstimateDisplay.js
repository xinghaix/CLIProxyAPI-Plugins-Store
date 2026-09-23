export const COST_ESTIMATE_NOTE_KEYS = {
  missing_model: 'missingModel',
  missing_price: 'missingPrice',
  fast_rate_unavailable: 'fastUnavailable',
  long_context_rate_unavailable: 'longContextUnavailable',
  cached_input_rate_unavailable: 'cachedInputUnavailable',
  cache_write_rate_unavailable: 'cacheWriteUnavailable',
  service_tier_unpriced: 'tierUnavailable',
  invalid_token_counts: 'invalidEstimate',
  invalid_flat_rates: 'invalidEstimate',
  invalid_estimate: 'invalidEstimate',
  token_breakdown_exceeds_input: 'invalidEstimate',
  zero_flat_rate: 'zeroFlatRate',
  service_tier_assumed_standard: 'assumedStandard',
};

export function eventCostAmount(row) {
  const estimate = row?.cost_estimate;
  if (!estimate || estimate.status !== 'estimated') return null;
  const amount = Number(estimate.amount);
  return Number.isFinite(amount) && amount >= 0 ? amount : null;
}

export function eventCostMeta(row, t) {
  const estimate = row?.cost_estimate;
  if (!estimate || estimate.status !== 'estimated') return t('monitoring.costEstimate.estimateUnavailable');
  if (estimate.schedule_id === 'model-price-flat' || estimate.context_tier === 'flat') {
    const responseTier = String(row?.response_service_tier || '').trim();
    const requestedTier = String(row?.service_tier || '').trim();
    const evidence = [
      responseTier ? t('monitoring.costEstimate.actualTier', {tier: responseTier}) : '',
      requestedTier ? t('monitoring.costEstimate.requestTier', {tier: requestedTier}) : '',
    ].filter(Boolean);
    return [t('monitoring.costEstimate.customPrice'), ...evidence, t('monitoring.costEstimate.customTierNote')].filter(Boolean).join(' · ');
  }
  const tier = estimate.service_tier === 'fast' ? t('monitoring.costEstimate.fast') : t('monitoring.costEstimate.standard');
  const context = estimate.context_tier === 'long' ? t('monitoring.costEstimate.longContext') : t('monitoring.costEstimate.shortContext');
  const requestedTier = String(row?.service_tier || '').trim();
  const request = requestedTier ? t('monitoring.costEstimate.requestTier', {tier: requestedTier}) : '';
  let source;
  if (estimate.tier_source === 'response') {
    source = [t('monitoring.costEstimate.sourceActual'), request].filter(Boolean).join(' · ');
  } else if (estimate.tier_source === 'assumed-standard') {
    source = [t('monitoring.costEstimate.assumedStandard'), request].filter(Boolean).join(' · ');
  } else {
    source = request || t('monitoring.costEstimate.sourceRequested');
  }
  return `${tier} · ${context} · ${source}`;
}

export function eventReasoningMeta(row, t, formatCompact) {
  const effort = String(row?.reasoning_effort || '').trim();
  const reasoningTokens = Number(row?.reasoning_tokens);
  if (!effort && !(Number.isFinite(reasoningTokens) && reasoningTokens > 0)) return '';
  const details = [];
  if (effort) details.push(t('monitoring.costEstimate.requestedEffort', {effort}));
  if (Number.isFinite(reasoningTokens) && reasoningTokens > 0) {
    details.push(t('monitoring.costEstimate.reasoningTokens', {count: formatCompact(reasoningTokens)}));
  }
  details.push(t('monitoring.costEstimate.reasoningCostBasis'));
  return details.join(' · ');
}

export function eventCostNote(note, t) {
  const key = COST_ESTIMATE_NOTE_KEYS[note];
  return key ? t(`monitoring.costEstimate.${key}`) : t('monitoring.costEstimate.estimateUnavailable');
}

export function eventCostTooltip(row, t) {
  const estimate = row?.cost_estimate;
  if (!estimate || estimate.status !== 'estimated') {
    return [t('monitoring.costEstimate.estimateUnavailable'), eventCostNote(estimate?.note, t)].filter(Boolean).join(' · ');
  }
  return eventCostMeta(row, t);
}

export function aggregateCostCoverage(row, t, formatInt) {
  return t('monitoring.costEstimate.coverage', {priced: formatInt(row?.priced_calls), unpriced: formatInt(row?.unpriced_calls)});
}

export function aggregateCostText(row, formatMoney, emptyValue) {
  return Number(row?.priced_calls || 0) > 0 ? formatMoney(row?.cost) : emptyValue;
}
