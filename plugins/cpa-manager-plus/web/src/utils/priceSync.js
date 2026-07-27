/** Default auto-sync settings for model prices. */
export const DEFAULT_SYNC_SETTINGS = Object.freeze({
  enabled: false,
  intervalHours: 12,
  protectManual: true,
});

export const PRICE_FILTERS = Object.freeze(['all', 'missing', 'saved', 'candidates']);

// Backend enforces 6–168 hours (default 12).
export const MIN_INTERVAL_HOURS = 6;
export const MAX_INTERVAL_HOURS = 168;

/** Normalize a raw price object from API (camelCase or PascalCase). */
export function normalizePrice(raw) {
  if (!raw || typeof raw !== 'object') {
    return {
      prompt: 0,
      completion: 0,
      cache: 0,
      cacheRead: 0,
      cacheCreation: 0,
      source: '',
      sourceModelId: '',
      updatedAtMs: 0,
    };
  }
  return {
    prompt: Number(raw.prompt ?? raw.Prompt ?? 0) || 0,
    completion: Number(raw.completion ?? raw.Completion ?? 0) || 0,
    cache: Number(raw.cache ?? raw.Cache ?? 0) || 0,
    cacheRead: Number(raw.cacheRead ?? raw.CacheRead ?? 0) || 0,
    cacheCreation: Number(raw.cacheCreation ?? raw.CacheCreation ?? 0) || 0,
    source: String(raw.source ?? raw.Source ?? ''),
    sourceModelId: String(raw.sourceModelId ?? raw.SourceModelId ?? ''),
    updatedAtMs: Number(raw.updatedAtMs ?? raw.UpdatedAtMs ?? raw.syncedAtMs ?? raw.SyncedAtMs ?? 0) || 0,
  };
}

/** Extract prices map from GET /model-prices body. */
export function extractPricesMap(body) {
  if (!body || typeof body !== 'object') return {};
  const prices = body.prices ?? body.Prices ?? body;
  if (!prices || typeof prices !== 'object' || Array.isArray(prices)) return {};
  const out = {};
  for (const [model, raw] of Object.entries(prices)) {
    if (!model || typeof raw !== 'object') continue;
    // Skip accidental meta keys if body itself was used.
    if (model === 'prices' || model === 'ok' || model === 'error') continue;
    out[model] = normalizePrice(raw);
  }
  return out;
}

/** Extract model names from usage-summary (graceful when API missing). */
export function extractUsageModels(body) {
  if (!body) return [];
  const list = body.models ?? body.Models ?? body.items ?? body.Items;
  if (!Array.isArray(list)) return [];
  const names = [];
  for (const item of list) {
    if (typeof item === 'string' && item.trim()) {
      names.push(item.trim());
      continue;
    }
    if (item && typeof item === 'object') {
      const model = item.model ?? item.Model ?? item.name ?? item.Name;
      if (typeof model === 'string' && model.trim()) names.push(model.trim());
    }
  }
  return names;
}

/** Build sorted unique model list for sync payload. */
export function buildSyncModels(prices, usageModels = []) {
  const set = new Set();
  for (const model of Object.keys(prices || {})) {
    if (model) set.add(model);
  }
  for (const model of usageModels || []) {
    if (model) set.add(model);
  }
  return Array.from(set).sort((a, b) => a.localeCompare(b));
}

/** Normalize GET/PUT sync-settings body. */
export function normalizeSyncSettings(body) {
  const base = { ...DEFAULT_SYNC_SETTINGS };
  if (!body || typeof body !== 'object') return base;
  const src = body.settings && typeof body.settings === 'object' ? body.settings : body;
  if (typeof src.enabled === 'boolean') base.enabled = src.enabled;
  else if (typeof src.Enabled === 'boolean') base.enabled = src.Enabled;

  const hours = Number(src.intervalHours ?? src.IntervalHours ?? src.interval_hours);
  if (Number.isFinite(hours)) base.intervalHours = clampIntervalHours(hours);

  if (typeof src.protectManual === 'boolean') base.protectManual = src.protectManual;
  else if (typeof src.ProtectManual === 'boolean') base.protectManual = src.ProtectManual;
  else if (typeof src.protect_manual === 'boolean') base.protectManual = src.protect_manual;

  return base;
}

export function clampIntervalHours(value) {
  const n = Math.round(Number(value));
  if (!Number.isFinite(n)) return DEFAULT_SYNC_SETTINGS.intervalHours;
  return Math.min(MAX_INTERVAL_HOURS, Math.max(MIN_INTERVAL_HOURS, n));
}

export function validateIntervalHours(value) {
  const n = Number(value);
  if (!Number.isFinite(n)) return { ok: false, error: '间隔必须是数字' };
  if (n < MIN_INTERVAL_HOURS || n > MAX_INTERVAL_HOURS) {
    return { ok: false, error: `间隔须在 ${MIN_INTERVAL_HOURS}–${MAX_INTERVAL_HOURS} 小时` };
  }
  return { ok: true, value: clampIntervalHours(n) };
}

/** Normalize source result entries from various backend shapes. */
export function normalizeSourceResults(raw) {
  if (!raw) return [];
  if (Array.isArray(raw)) {
    return raw.map(normalizeOneSourceResult).filter(Boolean);
  }
  if (typeof raw === 'object') {
    return Object.entries(raw).map(([source, value]) => {
      if (value && typeof value === 'object') {
        return normalizeOneSourceResult({ source, ...value });
      }
      return normalizeOneSourceResult({ source, error: String(value) });
    }).filter(Boolean);
  }
  return [];
}

function normalizeOneSourceResult(item) {
  if (!item || typeof item !== 'object') return null;
  const source = String(item.source ?? item.Source ?? '');
  if (!source) return null;
  const error = item.error ?? item.Error ?? '';
  const ok = typeof item.ok === 'boolean'
    ? item.ok
    : typeof item.OK === 'boolean'
      ? item.OK
      : !error;
  return {
    source,
    ok,
    error: error ? String(error) : '',
    modelCount: Number(item.modelCount ?? item.ModelCount ?? item.models ?? item.Models ?? 0) || 0,
    matched: Number(item.matched ?? item.Matched ?? 0) || 0,
    applied: Number(item.applied ?? item.Applied ?? 0) || 0,
    skipped: Number(item.skipped ?? item.Skipped ?? 0) || 0,
    httpStatus: Number(item.httpStatus ?? item.HTTPStatus ?? item.http_status ?? 0) || 0,
    durationMs: Number(item.durationMs ?? item.DurationMS ?? item.duration_ms ?? 0) || 0,
  };
}

/** Flatten candidates from plan shape (flat list) or legacy sets. */
export function normalizeCandidates(raw) {
  if (!raw) return [];
  if (!Array.isArray(raw)) return [];
  const out = [];
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue;
    // Legacy set: { model, candidates: [...] }
    if (Array.isArray(item.candidates || item.Candidates)) {
      const localModel = item.model ?? item.Model ?? item.localModel ?? item.LocalModel ?? '';
      for (const c of item.candidates || item.Candidates) {
        const n = normalizeOneCandidate(c, localModel);
        if (n) out.push(n);
      }
      continue;
    }
    const n = normalizeOneCandidate(item, '');
    if (n) out.push(n);
  }
  return out;
}

function normalizeOneCandidate(item, fallbackModel) {
  if (!item || typeof item !== 'object') return null;
  const localModel = String(
    item.localModel ?? item.LocalModel ?? item.model ?? item.Model ?? fallbackModel ?? '',
  ).trim();
  if (!localModel) return null;
  const nested = item.price && typeof item.price === 'object' ? item.price : null;
  const price = normalizePrice(nested || item);
  const source = String(
    item.source ?? item.Source ?? nested?.source ?? nested?.Source ?? price.source ?? '',
  );
  const sourceModelId = String(
    item.sourceModelId ?? item.SourceModelId ?? nested?.sourceModelId ?? nested?.SourceModelId ?? price.sourceModelId ?? '',
  );
  return {
    localModel,
    source: source || 'sync',
    sourceModelId,
    prompt: price.prompt,
    completion: price.completion,
    cache: price.cache,
    cacheRead: price.cacheRead,
    cacheCreation: price.cacheCreation,
    reason: String(item.reason ?? item.Reason ?? ''),
    score: Number(item.score ?? item.Score ?? 0) || 0,
  };
}

/**
 * Normalize sync status from either:
 * - { lastSyncAtMs, lastSuccessAtMs, lastError, lastResult }
 * - { settings, running, last: RunResult }
 */
export function normalizeSyncStatus(body) {
  const empty = {
    running: false,
    lastSyncAtMs: 0,
    lastSuccessAtMs: 0,
    lastError: '',
    lastResult: null,
    settings: { ...DEFAULT_SYNC_SETTINGS },
  };
  if (!body || typeof body !== 'object') return empty;

  if (body.settings) {
    empty.settings = normalizeSyncSettings(body.settings);
  }

  empty.running = Boolean(body.running ?? body.Running);

  const last = body.lastResult ?? body.LastResult ?? body.last ?? body.Last ?? null;
  if (last && typeof last === 'object') {
    empty.lastResult = normalizeSyncResult(last);
    empty.lastSyncAtMs = Number(
      body.lastSyncAtMs
        ?? body.LastSyncAtMs
        ?? last.finishedAtMs
        ?? last.FinishedAtMS
        ?? last.startedAtMs
        ?? last.StartedAtMS
        ?? 0,
    ) || 0;
    empty.lastSuccessAtMs = Number(
      body.lastSuccessAtMs
        ?? body.LastSuccessAtMs
        ?? (!last.error && !last.Error ? (last.finishedAtMs ?? last.FinishedAtMS ?? 0) : 0)
        ?? 0,
    ) || 0;
    empty.lastError = String(body.lastError ?? body.LastError ?? last.error ?? last.Error ?? '');
  } else {
    empty.lastSyncAtMs = Number(body.lastSyncAtMs ?? body.LastSyncAtMs ?? 0) || 0;
    empty.lastSuccessAtMs = Number(body.lastSuccessAtMs ?? body.LastSuccessAtMs ?? 0) || 0;
    empty.lastError = String(body.lastError ?? body.LastError ?? '');
  }

  return empty;
}

/** Normalize POST /sync response. */
export function normalizeSyncResult(body) {
  if (!body || typeof body !== 'object') return null;
  // Sometimes API wraps: { result: {...} }
  const raw = body.result && typeof body.result === 'object' && !body.sources && !body.Sources
    ? body.result
    : body;

  // Backend Result has sources: string[] plus sourceResults: object[].
  // Prefer detailed sourceResults; only fall back to sources when it looks like result rows.
  const sourceResultsRaw = raw.sourceResults ?? raw.SourceResults;
  const sourcesRaw = raw.sources ?? raw.Sources;
  const sources = normalizeSourceResults(
    Array.isArray(sourceResultsRaw) && sourceResultsRaw.length
      ? sourceResultsRaw
      : Array.isArray(sourcesRaw) && sourcesRaw.length && typeof sourcesRaw[0] === 'string'
        ? []
        : (sourcesRaw ?? sourceResultsRaw),
  );
  const candidates = normalizeCandidates(raw.candidates ?? raw.Candidates);
  const unmatchedRaw = raw.unmatched ?? raw.Unmatched;
  let unmatched = 0;
  let unmatchedModels = [];
  if (Array.isArray(unmatchedRaw)) {
    unmatchedModels = unmatchedRaw.map(String).filter(Boolean);
    unmatched = unmatchedModels.length;
  } else {
    unmatched = Number(unmatchedRaw) || 0;
  }

  return {
    trigger: String(raw.trigger ?? raw.Trigger ?? ''),
    startedAtMs: Number(raw.startedAtMs ?? raw.StartedAtMS ?? 0) || 0,
    finishedAtMs: Number(raw.finishedAtMs ?? raw.FinishedAtMS ?? 0) || 0,
    targetCount: Number(raw.targetCount ?? raw.TargetCount ?? 0) || 0,
    applied: Number(raw.applied ?? raw.Applied ?? raw.imported ?? raw.Imported ?? 0) || 0,
    protectedSkipped: Number(
      raw.protectedSkipped
        ?? raw.ProtectedSkipped
        ?? raw.protectedManual
        ?? raw.ProtectedManual
        ?? 0,
    ) || 0,
    unmatched,
    unmatchedModels,
    candidateCount: Number(raw.candidateCount ?? raw.CandidateCount ?? candidates.length) || candidates.length,
    sources,
    candidates,
    error: String(raw.error ?? raw.Error ?? ''),
  };
}

export function formatMoneyPer1M(value) {
  if (value == null || value === '') return '—';
  const n = Number(value);
  if (!Number.isFinite(n)) return '—';
  return `$${n.toFixed(4)}`;
}

export function formatSourceLabel(source) {
  const s = String(source || '').trim();
  if (!s) return '—';
  if (s.toLowerCase() === 'manual') return '手动';
  if (s.toLowerCase() === 'litellm') return 'LiteLLM';
  if (s.toLowerCase() === 'openrouter') return 'OpenRouter';
  return s;
}

export function sourceBadgeClass(source) {
  const s = String(source || '').toLowerCase();
  if (s === 'manual') return 'source-manual';
  if (s === 'litellm') return 'source-litellm';
  if (s === 'openrouter') return 'source-openrouter';
  if (!s) return 'source-empty';
  return 'source-other';
}

export function formatTimestamp(ms) {
  const n = Number(ms);
  if (!n) return '—';
  try {
    return new Date(n).toLocaleString('zh-CN', { hour12: false });
  } catch {
    return '—';
  }
}

export function formatDurationMs(ms) {
  const n = Number(ms);
  if (!Number.isFinite(n) || n <= 0) return '—';
  if (n < 1000) return `${Math.round(n)}ms`;
  return `${(n / 1000).toFixed(1)}s`;
}

/**
 * Build table rows from prices + usage models + candidates.
 */
export function buildModelPriceRows(prices, usageModels = [], candidates = []) {
  const rowMap = new Map();
  const candidateCountByModel = new Map();
  for (const c of candidates || []) {
    if (!c?.localModel) continue;
    candidateCountByModel.set(c.localModel, (candidateCountByModel.get(c.localModel) || 0) + 1);
  }
  const usageCalls = new Map();
  // usageModels may be plain strings (from extractUsageModels) — no call counts.
  // If original items with calls were needed, callers can pass a Map via prices only.

  const ensure = (model) => {
    if (!model) return null;
    let row = rowMap.get(model);
    if (row) return row;
    const price = prices?.[model];
    row = {
      model,
      hasPrice: Boolean(price),
      price: price || null,
      candidateCount: candidateCountByModel.get(model) || 0,
      calls: usageCalls.get(model) || 0,
    };
    rowMap.set(model, row);
    return row;
  };

  for (const model of Object.keys(prices || {})) ensure(model);
  for (const model of usageModels || []) ensure(model);
  for (const model of candidateCountByModel.keys()) ensure(model);

  return Array.from(rowMap.values()).sort((a, b) => {
    return (
      Number(a.hasPrice) - Number(b.hasPrice)
      || b.candidateCount - a.candidateCount
      || b.calls - a.calls
      || a.model.localeCompare(b.model)
    );
  });
}

export function buildFilterCounts(rows) {
  const counts = { all: rows.length, missing: 0, saved: 0, candidates: 0 };
  for (const row of rows) {
    if (row.hasPrice) counts.saved += 1;
    else counts.missing += 1;
    if (!row.hasPrice && row.candidateCount > 0) counts.candidates += 1;
  }
  return counts;
}

export function filterModelPriceRows(rows, filter, search) {
  const query = String(search || '').trim().toLowerCase();
  return (rows || []).filter((row) => {
    if (filter === 'missing' && row.hasPrice) return false;
    if (filter === 'saved' && !row.hasPrice) return false;
    if (filter === 'candidates' && (row.hasPrice || row.candidateCount === 0)) return false;
    if (!query) return true;
    const hay = [
      row.model,
      row.price?.source,
      row.price?.sourceModelId,
    ]
      .filter(Boolean)
      .join(' ')
      .toLowerCase();
    return hay.includes(query);
  });
}

/** Build manual price payload entry (always source=manual). */
export function buildManualPriceEntry(draft) {
  const model = String(draft?.model || '').trim();
  if (!model) return null;
  return {
    model,
    price: {
      prompt: nonNeg(draft.prompt),
      completion: nonNeg(draft.completion),
      cache: nonNeg(draft.cache),
      cacheRead: nonNeg(draft.cacheRead),
      cacheCreation: nonNeg(draft.cacheCreation),
      source: 'manual',
      sourceModelId: '',
      syncedAtMs: 0,
    },
  };
}

/** Single-model PUT body for manual upsert (does not send full price map). */
export function buildManualPutBody(draft) {
  const entry = buildManualPriceEntry(draft);
  if (!entry) return null;
  return { prices: { [entry.model]: entry.price } };
}

function nonNeg(v) {
  const n = Number(v);
  if (!Number.isFinite(n) || n < 0) return 0;
  return n;
}

/**
 * Build confirm API body for POST .../sync-confirm.
 * Backend contract: { model, price } with price.source + price.sourceModelId required.
 */
export function buildConfirmBody(candidate) {
  if (!candidate?.localModel) return null;
  const source = String(candidate.source || '').trim();
  const sourceModelId = String(candidate.sourceModelId || '').trim();
  if (!source || !sourceModelId) return null;
  return {
    model: candidate.localModel,
    price: {
      prompt: nonNeg(candidate.prompt),
      completion: nonNeg(candidate.completion),
      cache: nonNeg(candidate.cache),
      cacheRead: nonNeg(candidate.cacheRead),
      cacheCreation: nonNeg(candidate.cacheCreation),
      source,
      sourceModelId,
    },
  };
}

/** Summarize last result for metric cards. */
export function summarizeLastResult(result, syncModelCount = 0) {
  if (!result) {
    return {
      targetCount: syncModelCount,
      applied: 0,
      protectedSkipped: 0,
      candidateCount: 0,
      unmatched: 0,
    };
  }
  return {
    targetCount: result.targetCount || syncModelCount,
    applied: result.applied || 0,
    protectedSkipped: result.protectedSkipped || 0,
    candidateCount: result.candidateCount || (result.candidates?.length || 0),
    unmatched: result.unmatched || 0,
  };
}

/** Soft API call helper: returns {ok, data, error, missing}. */
export async function softProxyCall(proxyCall, payload) {
  try {
    const data = await proxyCall(payload);
    return { ok: true, data, error: '', missing: false };
  } catch (e) {
    const msg = e?.message || String(e);
    const missing = /404|not found|unknown path|unsupported|未实现|no route/i.test(msg);
    return { ok: false, data: null, error: msg, missing };
  }
}
