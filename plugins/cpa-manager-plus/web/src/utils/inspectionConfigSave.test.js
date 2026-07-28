import { describe, expect, it } from 'vitest';
import { createConfigFromDraft, toDraft } from './codexInspection.js';
import { buildInspectionConfigSaveBody, hasRedactedManagementKey } from './inspectionConfigSave.js';

describe('buildInspectionConfigSaveBody', () => {
  it('sends only codexInspection under config envelope', () => {
    const codexInspection = createConfigFromDraft(toDraft(null));
    const body = buildInspectionConfigSaveBody(codexInspection);
    expect(body).toEqual({ config: { codexInspection } });
    expect(body).not.toHaveProperty('cpaConnection');
    expect(body.config).not.toHaveProperty('cpaConnection');
    expect(body.config).not.toHaveProperty('dataDir');
    expect(hasRedactedManagementKey(body)).toBe(false);
  });

  it('detects redacted managementKey echo payloads', () => {
    expect(
      hasRedactedManagementKey({
        cpaConnection: { managementKey: false, hasManagementKey: true },
        codexInspection: {},
      }),
    ).toBe(true);
  });
});
