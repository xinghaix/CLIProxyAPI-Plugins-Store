import { normalizePrice } from './priceSync.js';

/** Normalize GET /model-prices/source-lookup response. */
export function normalizePriceSourceLookup(body) {
  if (!body || typeof body !== 'object') {
    return { model: '', sources: [] };
  }
  const rawSources = body.sources ?? body.Sources;
  const sources = Array.isArray(rawSources)
    ? rawSources.map((item) => normalizePriceSource(item)).filter(Boolean)
    : [];
  return {
    model: String(body.model ?? body.Model ?? '').trim(),
    sources: sources.sort((a, b) => (
      a.priority - b.priority
      || a.source.localeCompare(b.source)
      || a.sourceModelId.localeCompare(b.sourceModelId)
    )),
  };
}

function normalizePriceSource(raw) {
  if (!raw || typeof raw !== 'object') return null;
  const price = normalizePrice(raw.price ?? raw.Price ?? raw);
  const source = String(raw.source ?? raw.Source ?? price.source ?? '').trim();
  const sourceModelId = String(
    raw.sourceModelId
      ?? raw.SourceModelId
      ?? price.sourceModelId
      ?? '',
  ).trim();
  if (!source || !sourceModelId) return null;
  return {
    source,
    sourceModelId,
    priority: Number(raw.priority ?? raw.Priority ?? 0) || 0,
    available: Array.isArray(raw.available ?? raw.Available)
      ? (raw.available ?? raw.Available).map(String)
      : [],
    ...price,
    source,
    sourceModelId,
  };
}

export function buildPriceSourceLookupRequest(model) {
  const value = String(model || '').trim();
  if (!value) return null;
  return {
    method: 'GET',
    path: '/v0/management/model-prices/source-lookup',
    query: `model=${encodeURIComponent(value)}`,
  };
}

/** Build a manual price payload from a selected public source price. */
export function buildSourcePriceEntry(model, source) {
  const value = String(model || '').trim();
  if (!value || !source?.source || !source?.sourceModelId) return null;
  const price = normalizePrice(source);
  return {
    model: value,
    price: {
      prompt: price.prompt,
      completion: price.completion,
      cache: price.cache,
      cacheRead: price.cacheRead,
      cacheCreation: price.cacheCreation,
      source: 'manual',
      sourceModelId: '',
      syncedAtMs: 0,
    },
    selectedSource: {
      source: source.source,
      sourceModelId: source.sourceModelId,
    },
  };
}
