export function shortHash(value) {
  const text = String(value || '').trim();
  return text.length > 14 ? `${text.slice(0, 7)}…${text.slice(-5)}` : (text || '—');
}

export function looksLikeApiKey(value) {
  const text = String(value || '').trim();
  return text.length > 8 && /^(?:sk[-_]|rk[-_]|pk[-_]|xai-|AIza|key[-_])/i.test(text);
}

export function looksLikeEmail(value) {
  const text = String(value || '').trim();
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(text);
}

function headTailMask(text, head, tail, separator = '••••') {
  if (text.length <= head + tail) return '••••';
  return `${text.slice(0, head)}${separator}${text.slice(-tail)}`;
}

function maskEmailSummary(text) {
  const atIndex = text.indexOf('@');
  const local = text.slice(0, atIndex);
  const domain = text.slice(atIndex + 1);
  if (local.length <= 2) {
    return `${local.slice(0, 1)}••••@${domain}`;
  }
  if (local.length <= 5) {
    return `${local.slice(0, 1)}••••${local.slice(-1)}@${domain}`;
  }
  return `${local.slice(0, 2)}••••${local.slice(-2)}@${domain}`;
}

export function maskSecretSummary(value) {
  const text = String(value || '').trim();
  if (!text) return '—';
  if (looksLikeEmail(text)) return maskEmailSummary(text);
  if (text.length <= 8) return '••••';
  if (text.length <= 14) return headTailMask(text, 2, 2);
  if (text.length <= 24) return headTailMask(text, 3, 4);
  return headTailMask(text, 6, 4, '…');
}

export function isSensitiveSource(value, authType) {
  const text = String(value || '').trim().toLowerCase();
  const type = String(authType || '').trim().toLowerCase();
  if (!text || text === '—' || text === 'unknown') return false;
  if (looksLikeEmail(text)) return true;
  return type !== 'oauth' && type !== 'oauth2';
}

export function maskApiKey(value) {
  return maskSecretSummary(value);
}

export function eventApiKeyDisplay(value, expanded) {
  const text = String(value || '').trim();
  if (!text) return '—';
  return expanded ? text : maskSecretSummary(text);
}
