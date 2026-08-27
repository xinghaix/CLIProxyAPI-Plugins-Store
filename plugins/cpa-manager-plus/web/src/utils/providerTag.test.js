import { describe, expect, it } from 'vitest';
import { chipClass, isOAuthAuthType, providerChip } from './providerTag.js';

describe('providerChip', () => {
  it('uses a provider tag for oauth accounts', () => {
    expect(isOAuthAuthType('oauth')).toBe(true);
    expect(providerChip('xai', 'oauth')).toEqual({ kind: 'xai', tag: 'XAI', name: '', showName: false });
    expect(providerChip('codex', 'oauth2').tag).toBe('CODEX');
    expect(chipClass('xai')).toBe('is-xai');
  });

  it('splits openai-compatible hostnames into type tag and site name', () => {
    expect(providerChip('openai-compatible-wzw.pp.ua', 'apikey')).toEqual({
      kind: 'api',
      tag: 'OPENAI-COMPATIBLE',
      name: 'wzw.pp.ua',
      showName: true,
    });
  });

  it('returns empty when provider is missing', () => {
    expect(providerChip('', 'apikey').tag).toBe('');
    expect(providerChip('—', 'oauth').tag).toBe('');
  });
});
