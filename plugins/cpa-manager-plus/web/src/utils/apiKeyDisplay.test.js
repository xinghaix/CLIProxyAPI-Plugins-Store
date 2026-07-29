import { describe, expect, it } from 'vitest';
import { eventApiKeyDisplay, looksLikeApiKey, maskApiKey, shortHash } from './apiKeyDisplay.js';

describe('API Key display helpers', () => {
  it('recognizes the API Key prefixes used by event sources', () => {
    expect(looksLikeApiKey('sk-example-secret-value')).toBe(true);
    expect(looksLikeApiKey('xai-example-secret-value')).toBe(true);
    expect(looksLikeApiKey('account@example.com')).toBe(false);
  });

  it('masks API Keys and shortens hashes', () => {
    expect(maskApiKey('sk-example-secret-value')).toBe('sk-••••alue');
    expect(shortHash('0123456789abcdef')).toBe('0123456…bcdef');
  });

  it('only reveals the full value after explicit expansion', () => {
    expect(eventApiKeyDisplay('sk-example-secret-value', false)).toBe('••••');
    expect(eventApiKeyDisplay('sk-example-secret-value', true)).toBe('sk-example-secret-value');
    expect(eventApiKeyDisplay('', false)).toBe('—');
  });
});
