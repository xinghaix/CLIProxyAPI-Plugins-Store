/** Server-side Codex inspection helpers (ported from CPA-Manager-Plus presentation layer). */

import { translate } from '../i18n/index.js';
import { EMPTY_VALUE, formatDateTime } from './localeFormat.js';

export const RESULT_PAGE_SIZES = [20, 50, 100];

export const ACTION_FILTERS = ['all', 'reauth', 'delete', 'disable', 'enable', 'review', 'keep'];
export const HANDLING_FILTERS = ['all', 'pending', 'no_action'];

export const DEFAULT_SERVER_CONFIG = {
  enabled: false,
  schedule: { mode: 'interval', intervalMinutes: 60, timePoints: [], timeZone: '' },
  targetTypes: ['codex'],
  targetType: 'codex',
  workers: 4,
  deleteWorkers: 4,
  timeout: 15000,
  retries: 0,
  userAgent: 'codex_cli_rs/0.76.0 (Debian 13.0.0; x86_64) WindowsTerminal',
  xaiInferenceUserAgent: 'xai-grok-workspace/0.2.101',
  xaiInferenceEnabled: false,
  xaiInferenceModel: 'grok-4.5',
  xaiInferencePrompt: 'Reply with exactly OK.',
  usedPercentThreshold: 100,
  sampleSize: 0,
  autoActionMode: 'none',
  autoRecoverEnabled: false,
};

export function formatTimestamp(ms, locale) {
  return formatDateTime(ms, locale);
}

export function formatActionLabel(action) {
  const map = {
    delete: 'inspection.actions.delete',
    disable: 'inspection.actions.disable',
    enable: 'inspection.actions.enable',
    reauth: 'inspection.actions.reauth',
    review: 'inspection.actions.review',
    keep: 'inspection.actions.keep',
  };
  if (map[action]) return translate(map[action]);
  return action || EMPTY_VALUE;
}

export function formatAutoActionModeLabel(mode) {
  const map = {
    none: 'inspection.autoAction.none',
    enable: 'inspection.autoAction.enable',
    disable: 'inspection.autoAction.disable',
    delete: 'inspection.autoAction.delete',
  };
  if (map[mode]) return translate(map[mode]);
  return mode || EMPTY_VALUE;
}

export function getRunStatusLabel(status) {
  const map = {
    completed: 'inspection.runStatus.completed',
    failed: 'inspection.runStatus.failed',
    cancelled: 'inspection.runStatus.cancelled',
    interrupted: 'inspection.runStatus.interrupted',
    running: 'inspection.runStatus.running',
  };
  if (map[status]) return translate(map[status]);
  return translate('inspection.runStatus.idle');
}

export function getRunTone(status) {
  if (status === 'completed') return 'good';
  if (status === 'failed' || status === 'interrupted') return 'bad';
  if (status === 'running' || status === 'cancelled') return 'info';
  return 'idle';
}

export function getInspectionResultsEmptyText(run, rawCount = 0, filteredCount = 0) {
  if (rawCount > 0 && filteredCount === 0) return translate('inspection.empty.filtered');
  if (run?.status !== 'completed') return translate('inspection.empty.none');

  const totalFiles = Number(run.totalFiles) || 0;
  if (totalFiles === 0) return translate('inspection.empty.noCredentials');

  const probeSetCount = Number(run.probeSetCount) || 0;
  if (probeSetCount === 0) {
    return translate('inspection.empty.providerFiltered', { count: totalFiles });
  }

  const sampledCount = Number(run.sampledCount) || 0;
  if (sampledCount === 0) return translate('inspection.empty.noSample');
  return translate('inspection.empty.none');
}

export function formatTrigger(run) {
  if (!run) return EMPTY_VALUE;
  return run.triggerType === 'scheduled'
    ? translate('inspection.trigger.scheduled')
    : translate('inspection.trigger.manual');
}

export function formatDuration(run) {
  if (!run?.startedAtMs || !run.finishedAtMs) return EMPTY_VALUE;
  const seconds = Math.max(0, Math.round((run.finishedAtMs - run.startedAtMs) / 1000));
  return translate('inspection.durationSeconds', { seconds });
}

export function formatSchedule(config) {
  if (!config) return EMPTY_VALUE;
  const sch = config.schedule || {};
  if (sch.mode === 'time_points') {
    const pts = (sch.timePoints || []).join(', ');
    const tz = (sch.timeZone || '').trim();
    const base = translate('inspection.schedule.daily', { points: pts || EMPTY_VALUE });
    return tz ? `${base} (${tz})` : base;
  }
  return translate('inspection.schedule.interval', { minutes: sch.intervalMinutes || 60 });
}

export function normalizeInspectionTargetTypes(value, legacyTargetType = '') {
  const values = Array.isArray(value) ? value : String(value || legacyTargetType).split(/[,+\s]+/);
  const selected = new Set(values.map((item) => String(item).trim().toLowerCase()));
  return ['codex', 'xai'].filter((provider) => selected.has(provider));
}

export function resolveServerCodexConfig(raw) {
  const c = raw || {};
  const schedule = c.schedule || {};
  // Accept nested schedule and legacy flat scheduleMode/intervalMinutes from older GET payloads.
  const flatMode = c.scheduleMode === 'time_points' || c.scheduleMode === 'interval' ? c.scheduleMode : '';
  const mode =
    schedule.mode === 'time_points' || schedule.mode === 'interval'
      ? schedule.mode
      : flatMode ||
        (schedule.timePoints?.length
          ? 'time_points'
          : DEFAULT_SERVER_CONFIG.schedule.mode);
  const intervalMinutes =
    schedule.intervalMinutes > 0
      ? schedule.intervalMinutes
      : c.intervalMinutes > 0
        ? c.intervalMinutes
        : DEFAULT_SERVER_CONFIG.schedule.intervalMinutes;
  const targetTypes = normalizeInspectionTargetTypes(c.targetTypes, c.targetType);
  return {
    ...DEFAULT_SERVER_CONFIG,
    ...c,
    enabled: Boolean(c.enabled),
    schedule: {
      mode,
      intervalMinutes,
      timePoints: schedule.timePoints || [],
      timeZone: typeof schedule.timeZone === 'string' ? schedule.timeZone : '',
    },
    targetTypes: targetTypes.length ? targetTypes : [...DEFAULT_SERVER_CONFIG.targetTypes],
    targetType: (targetTypes[0] || DEFAULT_SERVER_CONFIG.targetType),
    workers: c.workers > 0 ? c.workers : DEFAULT_SERVER_CONFIG.workers,
    deleteWorkers: c.deleteWorkers > 0 ? c.deleteWorkers : DEFAULT_SERVER_CONFIG.deleteWorkers,
    timeout: c.timeout > 0 ? c.timeout : DEFAULT_SERVER_CONFIG.timeout,
    retries: c.retries !== undefined && c.retries >= 0 ? c.retries : DEFAULT_SERVER_CONFIG.retries,
    userAgent: c.userAgent || DEFAULT_SERVER_CONFIG.userAgent,
    xaiInferenceUserAgent: c.xaiInferenceUserAgent || DEFAULT_SERVER_CONFIG.xaiInferenceUserAgent,
    xaiInferenceEnabled: Boolean(c.xaiInferenceEnabled),
    xaiInferenceModel: c.xaiInferenceModel || DEFAULT_SERVER_CONFIG.xaiInferenceModel,
    xaiInferencePrompt: c.xaiInferencePrompt || DEFAULT_SERVER_CONFIG.xaiInferencePrompt,
    autoRecoverEnabled: Boolean(c.autoRecoverEnabled),
    usedPercentThreshold:
      c.usedPercentThreshold !== undefined ? c.usedPercentThreshold : DEFAULT_SERVER_CONFIG.usedPercentThreshold,
    sampleSize: c.sampleSize !== undefined && c.sampleSize >= 0 ? c.sampleSize : DEFAULT_SERVER_CONFIG.sampleSize,
    autoActionMode: c.autoActionMode || DEFAULT_SERVER_CONFIG.autoActionMode,
  };
}

export function toDraft(config) {
  const r = resolveServerCodexConfig(config);
  return {
    enabled: r.enabled,
    scheduleMode: r.schedule.mode,
    intervalMinutes: String(r.schedule.intervalMinutes),
    timePoints: (r.schedule.timePoints || []).join(', '),
    timeZone: r.schedule.timeZone || '',
    targetTypes: r.targetTypes.join('+'),
    targetType: r.targetType,
    workers: String(r.workers),
    deleteWorkers: String(r.deleteWorkers),
    timeout: String(r.timeout),
    retries: String(r.retries),
    userAgent: r.userAgent,
    xaiInferenceUserAgent: r.xaiInferenceUserAgent,
    xaiInferenceEnabled: r.xaiInferenceEnabled,
    xaiInferenceModel: r.xaiInferenceModel,
    xaiInferencePrompt: r.xaiInferencePrompt,
    usedPercentThreshold: String(r.usedPercentThreshold),
    sampleSize: String(r.sampleSize),
    autoActionMode: r.autoActionMode,
    autoRecoverEnabled: r.autoRecoverEnabled,
  };
}

function normalizeTimePoint(value) {
  const match = String(value).trim().match(/^(\d{1,2}):(\d{1,2})$/);
  if (!match) return null;
  const hour = Number(match[1]);
  const minute = Number(match[2]);
  if (!Number.isInteger(hour) || !Number.isInteger(minute)) return null;
  if (hour < 0 || hour > 23 || minute < 0 || minute > 59) return null;
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
}

export function parseTimePoints(raw) {
  return Array.from(
    new Set(
      String(raw || '')
        .split(/[\s,;，；]+/)
        .map((v) => v.trim())
        .filter(Boolean)
        .map(normalizeTimePoint)
        .filter(Boolean)
    )
  ).sort();
}

export function validateInspectionConfigFields(draft) {
  const errors = {};
  if (!normalizeInspectionTargetTypes(draft.targetTypes, draft.targetType).length) {
    errors.targetTypes = translate('inspection.validation.targetTypes');
  }
  const checkInt = (field, min, labelKey) => {
    const parsed = Number(String(draft[field] ?? '').trim());
    if (!Number.isFinite(parsed) || !Number.isInteger(parsed) || parsed < min) {
      errors[field] = translate('inspection.validation.intMin', {
        label: translate(labelKey),
        min,
      });
    }
  };
  checkInt('workers', 1, 'inspection.validation.workers');
  checkInt('deleteWorkers', 1, 'inspection.validation.deleteWorkers');
  checkInt('timeout', 1, 'inspection.validation.timeout');
  checkInt('retries', 0, 'inspection.validation.retries');
  checkInt('sampleSize', 0, 'inspection.validation.sampleSize');
  const threshold = Number(String(draft.usedPercentThreshold ?? '').trim());
  if (!Number.isFinite(threshold) || threshold < 0 || threshold > 100) {
    errors.usedPercentThreshold = translate('inspection.validation.usedPercentThreshold');
  }
  if (draft.scheduleMode === 'interval') {
    const iv = Number(String(draft.intervalMinutes ?? '').trim());
    if (!Number.isFinite(iv) || !Number.isInteger(iv) || iv < 1) {
      errors.intervalMinutes = translate('inspection.validation.intervalMinutes');
    }
  }
  if (draft.scheduleMode === 'time_points') {
    const tokens = String(draft.timePoints || '')
      .split(/[\s,;，；]+/)
      .map((v) => v.trim())
      .filter(Boolean);
    const invalid = tokens.some((t) => !normalizeTimePoint(t));
    const points = parseTimePoints(draft.timePoints);
    if (invalid || points.length === 0) errors.timePoints = translate('inspection.validation.timePoints');
  }
  return errors;
}

export function createConfigFromDraft(draft) {
  const errors = validateInspectionConfigFields(draft);
  if (Object.keys(errors).length) return null;
  const intervalMinutes = Number(String(draft.intervalMinutes).trim());
  const timePoints = parseTimePoints(draft.timePoints);
  const schedule =
    draft.scheduleMode === 'time_points'
      ? { mode: 'time_points', timePoints, intervalMinutes, timeZone: String(draft.timeZone || '').trim() }
      : { mode: 'interval', intervalMinutes, timePoints, timeZone: String(draft.timeZone || '').trim() };
  const targetTypes = normalizeInspectionTargetTypes(draft.targetTypes, draft.targetType);
  return {
    enabled: Boolean(draft.enabled),
    schedule,
    targetTypes,
    targetType: targetTypes[0],
    workers: Number(draft.workers),
    deleteWorkers: Number(draft.deleteWorkers),
    timeout: Number(draft.timeout),
    retries: Number(draft.retries),
    userAgent: String(draft.userAgent).trim(),
    xaiInferenceUserAgent: String(draft.xaiInferenceUserAgent || '').trim(),
    xaiInferenceEnabled: Boolean(draft.xaiInferenceEnabled),
    xaiInferenceModel: String(draft.xaiInferenceModel || '').trim(),
    xaiInferencePrompt: String(draft.xaiInferencePrompt || '').trim(),
    usedPercentThreshold: Number(draft.usedPercentThreshold),
    sampleSize: Number(draft.sampleSize),
    autoActionMode: ['none', 'enable', 'disable', 'delete'].includes(draft.autoActionMode)
      ? draft.autoActionMode
      : 'none',
    autoRecoverEnabled: Boolean(draft.autoRecoverEnabled),
  };
}

export function configsEquivalent(a, b) {
  const pick = (c) =>
    JSON.stringify({
      enabled: c.enabled,
      scheduleMode: c.schedule.mode,
      intervalMinutes: c.schedule.intervalMinutes,
      timePoints: [...(c.schedule.timePoints || [])].sort(),
      timeZone: (c.schedule.timeZone || '').trim(),
      targetTypes: [...(c.targetTypes || [])].sort(),
      workers: c.workers,
      deleteWorkers: c.deleteWorkers,
      timeout: c.timeout,
      retries: c.retries,
      userAgent: (c.userAgent || '').trim(),
      xaiInferenceUserAgent: (c.xaiInferenceUserAgent || '').trim(),
      xaiInferenceEnabled: Boolean(c.xaiInferenceEnabled),
      xaiInferenceModel: (c.xaiInferenceModel || '').trim(),
      xaiInferencePrompt: (c.xaiInferencePrompt || '').trim(),
      usedPercentThreshold: c.usedPercentThreshold,
      sampleSize: c.sampleSize,
      autoActionMode: c.autoActionMode,
      autoRecoverEnabled: Boolean(c.autoRecoverEnabled),
    });
  return pick(resolveServerCodexConfig(a)) === pick(resolveServerCodexConfig(b));
}

export function isServerAction(action) {
  return action === 'delete' || action === 'disable' || action === 'enable';
}

export function normalizeActionStatus(item) {
  const s = item.actionStatus;
  if (['none', 'pending', 'success', 'failed', 'skipped', 'needs_review', 'acknowledged'].includes(s)) return s;
  return isServerAction(item.action) ? 'pending' : 'none';
}

export function isActionableResult(item) {
  const status = normalizeActionStatus(item);
  return item.id > 0 && isServerAction(item.action) && (status === 'pending' || status === 'failed');
}

export function isManualAckAction(action) {
  return action === 'review' || action === 'reauth';
}

export function isAcknowledgeableResult(item) {
  const status = normalizeActionStatus(item);
  return item.id > 0 && isManualAckAction(item.action) && ['pending', 'failed', 'needs_review'].includes(status);
}

export function getCanonicalActionIds(results) {
  const canonical = new Set();
  const fileOrder = [];
  const groups = new Map();
  for (const item of results || []) {
    const fileName = String(item.fileName || '').trim();
    if (!isServerAction(item.action) || !fileName) continue;
    if (!groups.has(fileName)) {
      groups.set(fileName, []);
      fileOrder.push(fileName);
    }
    groups.get(fileName).push(item);
  }
  for (const fileName of fileOrder) {
    const group = groups.get(fileName) || [];
    if (!group.length) continue;
    const action = group[0].action;
    if (group.some((i) => i.action !== action)) continue;
    if (isActionableResult(group[0])) canonical.add(group[0].id);
  }
  return canonical;
}

export function isNeedsHandling(item) {
  const status = normalizeActionStatus(item);
  if (['success', 'skipped', 'acknowledged'].includes(status)) return false;
  return item.action !== 'keep' || item.statusCode === 401;
}

export function countHandlingStates(items) {
  const pending = (items || []).filter(isNeedsHandling).length;
  return { all: items.length, pending, no_action: items.length - pending };
}

export function countActions(items) {
  const counts = { delete: 0, disable: 0, enable: 0, reauth: 0, review: 0, keep: 0 };
  for (const item of items || []) {
    if (counts[item.action] !== undefined) counts[item.action] += 1;
  }
  return {
    all: items.length,
    reauth: counts.reauth,
    delete: counts.delete,
    disable: counts.disable,
    enable: counts.enable,
    review: counts.review,
    keep: counts.keep,
  };
}

export function filterInspectionResults(items, handlingFilter, actionFilter) {
  let list = items || [];
  if (handlingFilter === 'pending') list = list.filter(isNeedsHandling);
  if (handlingFilter === 'no_action') list = list.filter((i) => !isNeedsHandling(i));
  if (actionFilter !== 'all') list = list.filter((i) => i.action === actionFilter);
  return list;
}

export function buildPagination(items, page, pageSize) {
  const safeSize = Math.max(1, pageSize);
  const totalPages = Math.max(1, Math.ceil(items.length / safeSize));
  const currentPage = Math.min(Math.max(1, page), totalPages);
  const start = (currentPage - 1) * safeSize;
  const end = Math.min(start + safeSize, items.length);
  return {
    currentPage,
    totalPages,
    pageItems: items.slice(start, end),
    startItem: items.length ? start + 1 : 0,
    endItem: end,
    count: items.length,
  };
}

export function buildConfigOverviewItems(config, scheduleLabel) {
  const c = resolveServerCodexConfig(config);
  const sample =
    c.sampleSize > 0 ? String(c.sampleSize) : translate('inspection.overview.allAccounts');
  return [
    {
      key: 'schedule',
      label: translate('inspection.overview.schedule'),
      value: c.enabled ? translate('common.enabled') : translate('common.disabled'),
      field: 'schedule',
    },
    { key: 'trigger', label: translate('inspection.overview.trigger'), value: scheduleLabel, field: 'schedule' },
    {
      key: 'providers',
      label: translate('inspection.overview.providers'),
      value: c.targetTypes.map((item) => (item === 'xai' ? 'xAI' : 'Codex')).join(' + '),
      field: 'targetTypes',
    },
    {
      key: 'threshold',
      label: translate('inspection.overview.threshold'),
      value: `${c.usedPercentThreshold}%`,
      field: 'usedPercentThreshold',
    },
    { key: 'sample', label: translate('inspection.overview.sample'), value: sample, field: 'sampleSize' },
    {
      key: 'auto',
      label: translate('inspection.overview.auto'),
      value: formatAutoActionModeLabel(c.autoActionMode),
      field: 'autoActionMode',
    },
  ];
}

export function formatActionStatusLabel(item) {
  const status = normalizeActionStatus(item);
  const action = formatActionLabel(item.executedAction || item.action);
  if (status === 'success') return translate('inspection.actionStatus.success', { action });
  if (status === 'failed') return translate('inspection.actionStatus.failed');
  if (status === 'skipped') return translate('inspection.actionStatus.skipped');
  if (status === 'needs_review') return translate('inspection.actionStatus.needsReview');
  if (status === 'acknowledged') return translate('inspection.actionStatus.acknowledged');
  if (status === 'pending') return translate('inspection.actionStatus.pending');
  return '';
}

export function resultRowKey(item) {
  return `server-${item.id || item.accountKey}`;
}
