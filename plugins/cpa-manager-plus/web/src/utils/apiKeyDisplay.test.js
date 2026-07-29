import { describe, expect, it } from 'vitest';
import {
  eventApiKeyDisplay,
  isSensitiveSource,
  looksLikeApiKey,
  maskApiKey,
  maskSecretSummary,
  shortHash,
} from './apiKeyDisplay.js';

describe('API Key display helpers', () => {
  it('retains prefix recognition as a compatibility helper', () => {
    expect(looksLikeApiKey('sk-example-secret-value')).toBe(true);
    expect(looksLikeApiKey('xai-example-secret-value')).toBe(true);
    expect(looksLikeApiKey('account@example.com')).toBe(false);
  });

  it('only leaves OAuth and unknown sources unmasked', () => {
    expect(isSensitiveSource('oauth@example.com', 'oauth')).toBe(false);
    expect(isSensitiveSource('oauth@example.com', 'oauth2')).toBe(false);
    expect(isSensitiveSource('unknown', 'apikey')).toBe(false);
    expect(isSensitiveSource('', 'apikey')).toBe(false);
    expect(isSensitiveSource('oauth@example.com', '')).toBe(true);
    expect(isSensitiveSource('vertex-project', 'vertex')).toBe(true);
    expect(isSensitiveSource('sk-example-secret-value', 'apikey')).toBe(true);
  });

  it('masks every length range without depending on a key prefix', () => {
    expect(maskSecretSummary('')).toBe('—');
    expect(maskSecretSummary('sk-12')).toBe('••••');
    expect(maskSecretSummary('abcdefghij')).toBe('ab••••ij');
    expect(maskSecretSummary('abcdefghijklmnop')).toBe('abc••••mnop');
    expect(maskSecretSummary('abcdefghijklmnopqrstuvwxyz')).toBe('abcdef…wxyz');
    expect(maskApiKey('sk-example-secret-value')).toBe('sk-••••alue');
  });

  it('never exposes the complete value in collapsed summaries', () => {
    for (const value of ['a', 'sk-12', 'abcdefghij', 'abcdefghijklmnop', 'abcdefghijklmnopqrstuvwxyz']) {
      expect(maskSecretSummary(value)).not.toBe(value);
    }
  });

  it('only reveals the full value after explicit expansion', () => {
    expect(eventApiKeyDisplay('sk-example-secret-value', false)).toBe('sk-••••alue');
    expect(eventApiKeyDisplay('sk-example-secret-value', true)).toBe('sk-example-secret-value');
    expect(eventApiKeyDisplay('abcd', false)).toBe('••••');
    expect(eventApiKeyDisplay('', false)).toBe('—');
  });

  it('continues shortening API key hashes', () => {
    expect(shortHash('0123456789abcdef')).toBe('0123456…bcdef');
  });
});
