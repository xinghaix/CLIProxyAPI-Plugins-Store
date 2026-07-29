export function shortHash(value) {
  const text = String(value || '').trim();
  return text.length > 14 ? `${text.slice(0, 7)}…${text.slice(-5)}` : (text || '—');
}

export function looksLikeApiKey(value) {
  const text = String(value || '').trim();
  return text.length > 8 && /^(?:sk[-_]|rk[-_]|pk[-_]|xai-|AIza|key[-_])/i.test(text);
}

function headTailMask(text, head, tail, separator = '••••') {
  if (text.length <= head + tail) return '••••';
  return `${text.slice(0, head)}${separator}${text.slice(-tail)}`;
}

export function maskSecretSummary(value) {
  const text = String(value || '').trim();
  if (!text) return '—';
  if (text.length <= 8) return '••••';
  if (text.length <= 14) return headTailMask(text, 2, 2);
  if (text.length <= 24) return headTailMask(text, 3, 4);
  return headTailMask(text, 6, 4, '…');
}

export function isSensitiveSource(value, authType) {
  const text = String(value || '').trim().toLowerCase();
  const type = String(authType || '').trim().toLowerCase();
  return !!text && text !== '—' && text !== 'unknown' && type !== 'oauth' && type !== 'oauth2';
}

export function maskApiKey(value) {
  return maskSecretSummary(value);
}

export function eventApiKeyDisplay(value, expanded) {
  const text = String(value || '').trim();
  if (!text) return '—';
  return expanded ? text : maskSecretSummary(text);
}
