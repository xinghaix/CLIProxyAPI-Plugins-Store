/**
 * Compatible with Cli-Proxy-API-Management-Center secure storage
 * (src/utils/encryption.ts + services/storage/secureStorage.ts).
 *
 * Not a security boundary — reverses host+UA salted XOR obfuscation so plugin
 * pages can reuse CPA MC "remember password" session keys from localStorage.
 */

export const MC_STORAGE_KEY_AUTH = 'cli-proxy-auth';

const ENC_PREFIX = 'enc::v1::';
const SECRET_SALT = 'cli-proxy-api-webui::secure-storage';

let cachedKeyBytes = null;

function encodeText(text) {
  return new TextEncoder().encode(text);
}

function decodeText(bytes) {
  return new TextDecoder().decode(bytes);
}

function getKeyBytes() {
  if (cachedKeyBytes) return cachedKeyBytes;
  try {
    const host = window.location.host;
    const ua = navigator.userAgent;
    cachedKeyBytes = encodeText(`${SECRET_SALT}|${host}|${ua}`);
  } catch {
    cachedKeyBytes = encodeText(SECRET_SALT);
  }
  return cachedKeyBytes;
}

function xorBytes(data, keyBytes) {
  const result = new Uint8Array(data.length);
  for (let i = 0; i < data.length; i++) {
    result[i] = data[i] ^ keyBytes[i % keyBytes.length];
  }
  return result;
}

function fromBase64(base64) {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

export function isObfuscated(value) {
  return typeof value === 'string' && value.startsWith(ENC_PREFIX);
}

/** @param {string} payload */
export function deobfuscateData(payload) {
  if (!payload || !payload.startsWith(ENC_PREFIX)) {
    return payload;
  }
  try {
    const encodedBody = payload.slice(ENC_PREFIX.length);
    const encrypted = fromBase64(encodedBody);
    const decrypted = xorBytes(encrypted, getKeyBytes());
    return decodeText(decrypted);
  } catch {
    return payload;
  }
}

/**
 * MC obfuscatedStorage.getItem equivalent for a raw localStorage entry.
 * @param {string | null} raw
 * @returns {unknown | null}
 */
export function parseObfuscatedStorageValue(raw) {
  if (raw == null || raw === '') return null;
  try {
    const decrypted = deobfuscateData(raw);
    return JSON.parse(decrypted);
  } catch {
    try {
      if (isObfuscated(raw)) {
        return deobfuscateData(raw);
      }
      return raw;
    } catch {
      return null;
    }
  }
}

/**
 * Extract plaintext management key from parsed MC auth persist object.
 * @param {unknown} parsed
 * @returns {string}
 */
export function extractManagementKeyFromAuthBlob(parsed) {
  if (!parsed || typeof parsed !== 'object') return '';

  /** @type {Record<string, unknown>} */
  const root = /** @type {Record<string, unknown>} */ (parsed);
  const nested =
    root.state && typeof root.state === 'object'
      ? /** @type {Record<string, unknown>} */ (root.state)
      : root;

  const candidates = [
    nested.managementKey,
    root.managementKey,
    nested.management_key,
    root.management_key,
  ];

  for (const candidate of candidates) {
    const key = normalizeManagementKeyValue(candidate);
    if (key) return key;
  }
  return '';
}

/**
 * @param {unknown} value
 * @returns {string}
 */
function normalizeManagementKeyValue(value) {
  if (typeof value !== 'string') return '';
  let key = value.trim();
  if (!key) return '';
  if (isObfuscated(key)) {
    key = deobfuscateData(key).trim();
  }
  if (!key || isObfuscated(key)) return '';
  return key;
}

/**
 * Read CPA management key from MC localStorage (cli-proxy-auth + legacy keys).
 * @param {Storage} storage
 * @returns {string}
 */
export function readManagementKeyFromMCStorage(storage) {
  if (!storage) return '';

  const authRaw = storage.getItem(MC_STORAGE_KEY_AUTH);
  if (authRaw) {
    const parsed = parseObfuscatedStorageValue(authRaw);
    const fromAuth = extractManagementKeyFromAuthBlob(parsed);
    if (fromAuth) return fromAuth;
  }

  for (const legacyKey of ['managementKey', 'management_key']) {
    const raw = storage.getItem(legacyKey);
    if (!raw) continue;
    const parsed = parseObfuscatedStorageValue(raw);
    if (typeof parsed === 'string') {
      const key = normalizeManagementKeyValue(parsed);
      if (key) return key;
    }
    if (parsed && typeof parsed === 'object') {
      const key = extractManagementKeyFromAuthBlob(parsed);
      if (key) return key;
    }
    const key = normalizeManagementKeyValue(raw);
    if (key) return key;
  }

  return '';
}