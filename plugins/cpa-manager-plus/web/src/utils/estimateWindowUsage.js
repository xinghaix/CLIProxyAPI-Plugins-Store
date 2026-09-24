/**
 * Port of CPA-Manager-Plus estimateWindowUsage.ts (MIT, Seakee).
 * Forecast = current * 100/used% when quota progress is usable; else previous.
 * Cost is rounded to cents. Never invent metrics.
 */

const isUsableMetrics = (metrics) =>
  metrics != null &&
  Number.isFinite(metrics.requests) &&
  metrics.requests >= 0 &&
  Number.isFinite(metrics.tokens) &&
  metrics.tokens >= 0 &&
  Number.isFinite(metrics.cost) &&
  metrics.cost >= 0;

const hasUsage = (metrics) =>
  metrics.requests > 0 || metrics.tokens > 0 || metrics.cost > 0;

const roundForecastCost = (value) => {
  const scaled = value * 100;
  return Number.isFinite(scaled) ? Math.round(scaled) / 100 : value;
};

export function estimateWindowUsage(input) {
  const hasCurrentUsage = isUsableMetrics(input?.current) && hasUsage(input.current);
  const usedPercent = input?.usedPercent;
  const hasQuotaProgress =
    usedPercent != null &&
    Number.isFinite(usedPercent) &&
    usedPercent > 0 &&
    usedPercent <= 100;
  if (hasCurrentUsage && hasQuotaProgress) {
    const multiplier = 100 / usedPercent;
    const forecast = {
      requests: Math.max(input.current.requests, Math.round(input.current.requests * multiplier)),
      tokens: Math.max(input.current.tokens, Math.round(input.current.tokens * multiplier)),
      cost: Math.max(input.current.cost, roundForecastCost(input.current.cost * multiplier)),
      basis: 'quota',
    };
    if (
      Number.isFinite(multiplier) &&
      Number.isFinite(forecast.requests) &&
      Number.isFinite(forecast.tokens) &&
      Number.isFinite(forecast.cost)
    ) {
      return forecast;
    }
  }
  if (isUsableMetrics(input?.previous)) {
    return {...input.previous, basis: 'previous'};
  }
  return null;
}
