/** Convert used_percent (0–100) to remaining percent for window-keeper badges. */
export function remainingPercentFromUsed(usedPercent) {
  const used = Number(usedPercent);
  if (!Number.isFinite(used)) return null;
  return Math.max(0, Math.min(100, Math.round(100 - used)));
}

/**
 * Badge/card text: "{label}剩余 {percent}%" / "{label} remaining {percent}%".
 * When blocking (full), callers should show the full label instead.
 */
export function formatWindowRemainingText(label, usedPercent, remainingWord = '剩余') {
  const remaining = remainingPercentFromUsed(usedPercent);
  if (remaining == null) return String(label || '');
  const word = remainingWord == null ? '' : String(remainingWord);
  // Chinese locales use no space before the word: 5小时限额剩余 98%
  // English uses a leading space in remainingWord: " remaining"
  return `${label || ''}${word} ${remaining}%`;
}
