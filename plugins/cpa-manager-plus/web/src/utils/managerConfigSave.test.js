import { describe, expect, it } from 'vitest';
import { buildManagerConfigSaveBody } from './managerConfigSave.js';

const currentConfig = {
  cpaConnection: { cpaBaseUrl: 'http://127.0.0.1:8317', hasManagementKey: true },
  collector: {
    enabled: true,
    collectorMode: 'auto',
    pollIntervalMs: 500,
    batchSize: 64,
    queryLimit: 50000,
    tlsSkipVerify: false,
  },
};

describe('buildManagerConfigSaveBody', () => {
  it('sends only the local collection switch', () => {
    const body = buildManagerConfigSaveBody({
      currentConfig,
      cpaBaseURL: 'http://127.0.0.1:8317',
      managementKey: '',
      monitoringEnabled: false,
    });

    expect(body).toEqual({ config: { collector: { enabled: false } } });
    expect(body.config).not.toHaveProperty('externalUsageService');
    expect(body.config.collector).not.toHaveProperty('collectorMode');
    expect(body.config.collector).not.toHaveProperty('batchSize');
  });

  it('sends a newly entered key without replaying redacted connection metadata', () => {
    const body = buildManagerConfigSaveBody({
      currentConfig,
      cpaBaseURL: 'http://localhost:8317',
      managementKey: 'new-secret',
      monitoringEnabled: true,
    });

    expect(body).toEqual({
      config: {
        cpaConnection: {
          cpaBaseUrl: 'http://localhost:8317',
          managementKey: 'new-secret',
        },
      },
    });
  });

  it('omits unchanged settings', () => {
    expect(
      buildManagerConfigSaveBody({
        currentConfig,
        cpaBaseURL: 'http://127.0.0.1:8317',
        managementKey: '',
        monitoringEnabled: true,
      })
    ).toEqual({ config: {} });
  });
});
