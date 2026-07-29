export function shortHash(value) {
  const text = String(value || '').trim();
  return text.length > 14 ? `${text.slice(0, 7)}…${text.slice(-5)}` : (text || '—');
}

export function looksLikeApiKey(value) {
  const text = String(value || '').trim();
  return text.length > 8 && /^(?:sk[-_]|rk[-_]|pk[-_]|xai-|AIza|key[-_])/i.test(text);
}

export function maskApiKey(value) {
  const text = String(value || '').trim();
  if (!text) return '—';
  if (looksLikeApiKey(text)) return `${text.slice(0, 3)}••••${text.slice(-4)}`;
  return shortHash(text);
}

export function eventApiKeyDisplay(value, expanded) {
  const text = String(value || '').trim();
  if (!text) return '—';
  return expanded ? text : '••••';
}
