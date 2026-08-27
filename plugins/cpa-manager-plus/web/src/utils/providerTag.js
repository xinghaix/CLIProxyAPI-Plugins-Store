const OAUTH_LABELS = {
  xai: 'XAI',
  grok: 'XAI',
  openai: 'OPENAI',
  chatgpt: 'CODEX',
  codex: 'CODEX',
  gemini: 'GEMINI',
  google: 'GEMINI',
  claude: 'CLAUDE',
  anthropic: 'CLAUDE',
  antigravity: 'ANTIGRAVITY',
  vertex: 'VERTEX',
};

const API_TYPES = [
  'openai-compatible',
  'openai',
  'anthropic',
  'claude',
  'gemini',
  'google',
  'xai',
  'grok',
  'azure',
  'vertex',
  'codex',
];

export function isOAuthAuthType(authType) {
  const type = String(authType || '').trim().toLowerCase();
  return type === 'oauth' || type === 'oauth2';
}

function oauthKind(provider) {
  const raw = String(provider || '').trim().toLowerCase();
  if (!raw) return '';
  return raw.split(/[./:\s]/)[0];
}

function parseApiProvider(provider) {
  const raw = String(provider || '').trim();
  const lower = raw.toLowerCase();
  const types = [...API_TYPES].sort((a, b) => b.length - a.length);
  for (const type of types) {
    if (lower === type) {
      return { tag: type.toUpperCase(), name: '' };
    }
    if (lower.startsWith(`${type}-`) || lower.startsWith(`${type}.`)) {
      return { tag: type.toUpperCase(), name: raw.slice(type.length + 1) };
    }
  }
  return { tag: 'API', name: raw };
}

export function providerChip(provider, authType) {
  const name = String(provider || '').trim();
  if (!name || name === '—') {
    return { kind: '', tag: '', name: '', showName: false };
  }
  if (isOAuthAuthType(authType)) {
    const kind = oauthKind(name);
    const tag = OAUTH_LABELS[kind] || name.toUpperCase();
    return { kind: OAUTH_LABELS[kind] ? kind : 'oauth', tag, name: '', showName: false };
  }
  const parsed = parseApiProvider(name);
  return {
    kind: 'api',
    tag: parsed.tag,
    name: parsed.name,
    showName: Boolean(parsed.name),
  };
}

export function chipClass(kind) {
  if (kind === 'xai' || kind === 'grok') return 'is-xai';
  if (kind === 'codex' || kind === 'openai' || kind === 'chatgpt') return 'is-codex';
  if (kind === 'api') return 'is-api';
  return 'is-oauth';
}
