import { describe, expect, it } from 'vitest';
import { chipClass, isCodexOAuth, isOAuthAuthType, providerChip } from './providerTag.js';

describe('providerChip', () => {
  it('maps official oauth credential providers', () => {
    expect(isOAuthAuthType('oauth')).toBe(true);
    expect(providerChip('kimi', 'oauth')).toMatchObject({ tag: 'KIMI', chip: 'is-kimi', showName: false });
    expect(providerChip('codex', 'oauth2')).toMatchObject({ tag: 'CODEX', chip: 'is-codex' });
    expect(providerChip('claude', 'oauth')).toMatchObject({ tag: 'CLAUDE', chip: 'is-claude' });
    expect(providerChip('antigravity', 'oauth')).toMatchObject({ tag: 'ANTIGRAVITY', chip: 'is-antigravity' });
    expect(providerChip('xai', 'oauth')).toMatchObject({ tag: 'XAI', chip: 'is-xai' });
    expect(providerChip('vertex', 'oauth')).toMatchObject({ tag: 'VERTEX', chip: 'is-vertex' });
    expect(providerChip('devin', 'oauth')).toMatchObject({ tag: 'DEVIN', chip: 'is-devin' });
    expect(providerChip('cursor', 'oauth')).toMatchObject({ tag: 'CURSOR', chip: 'is-cursor' });
    expect(providerChip('kimi-code', 'oauth')).toMatchObject({ tag: 'KIMI-CODE', chip: 'is-kimi-code' });
    expect(providerChip('kimi.ai', 'oauth')).toMatchObject({ tag: 'KIMI-CODE', chip: 'is-kimi-code' });
    expect(providerChip('cursor-oauth', 'oauth')).toMatchObject({ tag: 'CURSOR', chip: 'is-cursor' });
    expect(providerChip('meta', 'oauth')).toMatchObject({ tag: 'META', chip: 'is-meta' });
    expect(providerChip('gemini-cli', 'oauth')).toMatchObject({ tag: 'GEMINI', chip: 'is-gemini' });
  });

  it('keeps known OAuth providers on distinct capsule classes', () => {
    const providers = ['xai', 'devin', 'cursor', 'kimi', 'kimi-code', 'meta', 'codex', 'claude', 'antigravity', 'vertex', 'gemini', 'gemini-interactions'];
    const chips = providers.map(provider => providerChip(provider, 'oauth').chip);
    expect(new Set(chips).size).toBe(providers.length);
  });

  it('maps official API providers without inventing a host name', () => {
    expect(providerChip('kimi', 'apikey')).toMatchObject({ tag: 'KIMI', showName: false, chip: 'is-kimi' });
    expect(providerChip('gemini', 'apikey')).toMatchObject({ tag: 'GEMINI', showName: false });
    expect(providerChip('gemini-interactions', 'apikey')).toMatchObject({ tag: 'INTERACTIONS', showName: false, chip: 'is-interactions' });
    expect(providerChip('codex', 'apikey')).toMatchObject({ tag: 'CODEX' });
    expect(providerChip('xai', 'apikey')).toMatchObject({ tag: 'XAI' });
    expect(providerChip('claude', 'apikey')).toMatchObject({ tag: 'CLAUDE' });
    expect(providerChip('vertex', 'apikey')).toMatchObject({ tag: 'VERTEX' });
    expect(providerChip('openai-compatibility', 'apikey')).toMatchObject({ tag: 'OPENAI-COMPATIBLE', showName: false });
  });

  it('splits openai-compatible hostnames into type tag and site name', () => {
    expect(providerChip('openai-compatible-wzw.pp.ua', 'apikey')).toEqual({
      kind: 'openai-compatible',
      tag: 'OPENAI-COMPATIBLE',
      name: 'wzw.pp.ua',
      showName: true,
      chip: 'is-api',
    });
  });

  it('does not treat gemini-interactions as a gemini host suffix', () => {
    const chip = providerChip('gemini-interactions', 'apikey');
    expect(chip.tag).toBe('INTERACTIONS');
    expect(chip.name).toBe('');
  });

  it('returns empty when provider is missing', () => {
    expect(providerChip('', 'apikey').tag).toBe('');
    expect(providerChip('—', 'oauth').tag).toBe('');
    expect(chipClass('xai')).toBe('is-xai');
  });

  it('identifies codex oauth accounts accurately', () => {
    expect(isCodexOAuth({ provider: 'codex', auth_type: 'oauth' })).toBe(true);
    expect(isCodexOAuth({ auth_provider_snapshot: 'chatgpt', auth_type: 'oauth2' })).toBe(true);
    expect(isCodexOAuth({ provider: 'antigravity', auth_type: 'oauth' })).toBe(false);
    expect(isCodexOAuth({ provider: 'codex', auth_type: 'apikey' })).toBe(false);
    expect(isCodexOAuth({ provider: 'openai', auth_type: 'oauth' })).toBe(false);
    expect(isCodexOAuth(null)).toBe(false);
  });
});
