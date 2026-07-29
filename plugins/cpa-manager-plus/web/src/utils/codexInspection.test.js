import { describe, expect, it } from 'vitest';
import {
  createConfigFromDraft,
  normalizeInspectionTargetTypes,
  resolveServerCodexConfig,
  toDraft,
} from './codexInspection.js';

describe('multi-provider inspection settings', () => {
  it('upgrades legacy targetType to an ordered provider list', () => {
    expect(resolveServerCodexConfig({ targetType: 'xai' }).targetTypes).toEqual(['xai']);
    expect(normalizeInspectionTargetTypes('xai+codex')).toEqual(['codex', 'xai']);
  });

  it('round-trips Codex + xAI settings including optional inference', () => {
    const draft = toDraft({ targetTypes: ['codex', 'xai'], xaiInferenceEnabled: true });
    const config = createConfigFromDraft(draft);

    expect(config.targetTypes).toEqual(['codex', 'xai']);
    expect(config.targetType).toBe('codex');
    expect(config.xaiInferenceEnabled).toBe(true);
    expect(config.xaiInferenceModel).toBe('grok-4.5');
  });
});
