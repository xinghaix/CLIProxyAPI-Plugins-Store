const PROVIDERS = [
  { keys: ['gemini-interactions'], kind: 'gemini-interactions', tag: 'INTERACTIONS', chip: 'is-interactions' },
  { keys: ['openai-compatible', 'openai-compatibility'], kind: 'openai-compatible', tag: 'OPENAI-COMPATIBLE', chip: 'is-api', splitName: true },
  { keys: ['gemini-cli', 'gemini', 'google'], kind: 'gemini', tag: 'GEMINI', chip: 'is-gemini' },
  { keys: ['antigravity'], kind: 'antigravity', tag: 'ANTIGRAVITY', chip: 'is-antigravity' },
  { keys: ['anthropic', 'claude'], kind: 'claude', tag: 'CLAUDE', chip: 'is-claude' },
  { keys: ['chatgpt', 'codex'], kind: 'codex', tag: 'CODEX', chip: 'is-codex' },
  { keys: ['devin', 'devin-oauth'], kind: 'devin', tag: 'DEVIN', chip: 'is-devin' },
  { keys: ['cursor', 'cursor-oauth'], kind: 'cursor', tag: 'CURSOR', chip: 'is-cursor' },
  { keys: ['vertex'], kind: 'vertex', tag: 'VERTEX', chip: 'is-vertex' },
  { keys: ['kimi-code', 'kimi-ai', 'kimi.ai', 'kimi-international'], kind: 'kimi-code', tag: 'KIMI-CODE', chip: 'is-kimi-code' },
  { keys: ['kimi', 'kimi.com'], kind: 'kimi', tag: 'KIMI', chip: 'is-kimi' },
  { keys: ['meta', 'muse', 'meta-oauth', 'muse-meta'], kind: 'meta', tag: 'META', chip: 'is-meta' },
  { keys: ['xai', 'grok'], kind: 'xai', tag: 'XAI', chip: 'is-xai' },
  { keys: ['openai'], kind: 'openai', tag: 'OPENAI', chip: 'is-openai' },
  { keys: ['azure'], kind: 'azure', tag: 'AZURE', chip: 'is-api' },
];

const BY_KEY = new Map();
for (const provider of PROVIDERS) {
  for (const key of provider.keys) {
    BY_KEY.set(key, provider);
  }
}

export function isOAuthAuthType(authType) {
  const type = String(authType || '').trim().toLowerCase();
  return type === 'oauth' || type === 'oauth2';
}

export function isCodexOAuth(providerOrRow, authType) {
  if (!providerOrRow) return false;
  let provider = providerOrRow;
  let type = authType;
  if (typeof providerOrRow === 'object') {
    provider = providerOrRow.auth_provider_snapshot || providerOrRow.provider || providerOrRow.raw?.auth_provider_snapshot || providerOrRow.raw?.provider;
    type = providerOrRow.auth_type || providerOrRow.raw?.auth_type;
  }
  if (!isOAuthAuthType(type)) return false;
  const chip = providerChip(provider, type);
  return chip?.kind === 'codex';
}

function matchProvider(raw) {
  const lower = String(raw || '').trim().toLowerCase();
  if (!lower) return null;
  const exact = BY_KEY.get(lower);
  if (exact) return { spec: exact, name: '' };
  const keys = [...BY_KEY.keys()].sort((a, b) => b.length - a.length);
  for (const key of keys) {
    if (lower.startsWith(`${key}-`) || lower.startsWith(`${key}.`)) {
      const spec = BY_KEY.get(key);
      return { spec, name: String(raw).trim().slice(key.length + 1) };
    }
  }
  return null;
}

export function providerChip(provider, authType) {
  const raw = String(provider || '').trim();
  if (!raw || raw === '—') {
    return { kind: '', tag: '', name: '', showName: false, chip: '' };
  }
  const matched = matchProvider(raw);
  if (isOAuthAuthType(authType)) {
    if (matched) {
      return { kind: matched.spec.kind, tag: matched.spec.tag, name: '', showName: false, chip: matched.spec.chip };
    }
    return { kind: 'oauth', tag: raw.toUpperCase(), name: '', showName: false, chip: 'is-oauth' };
  }
  if (matched) {
    const name = matched.spec.splitName ? matched.name : (matched.name || '');
    const showName = Boolean(matched.spec.splitName && name);
    return {
      kind: matched.spec.kind,
      tag: matched.spec.tag,
      name: showName ? name : '',
      showName,
      chip: matched.spec.chip,
    };
  }
  return { kind: 'api', tag: 'API', name: raw, showName: true, chip: 'is-api' };
}

export function chipClass(kindOrChip) {
  const value = String(kindOrChip || '').trim();
  if (value.startsWith('is-')) return value;
  const matched = BY_KEY.get(value) || PROVIDERS.find(provider => provider.kind === value);
  return matched?.chip || (value === 'api' ? 'is-api' : 'is-oauth');
}
