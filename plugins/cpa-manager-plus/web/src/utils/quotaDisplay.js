export function normalizeQuotaWindows(result) {
  const windows = Array.isArray(result?.quotaWindows) ? result.quotaWindows : [];
  return windows.map((window, index) => {
    const rawUsedPercent = window?.usedPercent;
    const parsedUsedPercent = rawUsedPercent == null ? NaN : Number(rawUsedPercent);
    const parsedRemaining = Number(window?.remaining);
    return {
      id: window?.id || `window-${index}`,
      label: window?.label || window?.id || '',
      hasUsedPercent: Number.isFinite(parsedUsedPercent),
      usedPercent: Number.isFinite(parsedUsedPercent) ? parsedUsedPercent : 0,
      remaining: Number.isFinite(parsedRemaining) ? parsedRemaining : null,
      remainingText: typeof window?.remaining === 'string' ? window.remaining.trim() : '',
      resetText: window?.resetAt ? String(window.resetAt) : '',
    };
  }).filter(window => window.hasUsedPercent || window.remaining !== null || window.remainingText || window.resetText);
}
