import { describe, expect, it } from 'vitest';
import {
  DEFAULT_CPA_MANAGEMENT_BASE_URL,
  buildManagerConfigSaveBody,
  resolveCPAManagementBaseURL,
} from './managerConfigSave.js';

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

describe('resolveCPAManagementBaseURL', () => {
  it('keeps an explicit Base URL', () => {
    expect(
      resolveCPAManagementBaseURL({
        cpaBaseURL: 'http://localhost:8317',
        managementKey: 'secret',
        hasManagementKey: false,
      })
    ).toBe('http://localhost:8317');
  });

  it('defaults when saving a new key with empty Base URL', () => {
    expect(
      resolveCPAManagementBaseURL({
        cpaBaseURL: '',
        managementKey: 'secret',
        hasManagementKey: false,
      })
    ).toBe(DEFAULT_CPA_MANAGEMENT_BASE_URL);
  });

  it('defaults when a key is already bound and Base URL is empty', () => {
    expect(
      resolveCPAManagementBaseURL({
        cpaBaseURL: '  ',
        managementKey: '',
        hasManagementKey: true,
      })
    ).toBe(DEFAULT_CPA_MANAGEMENT_BASE_URL);
  });

  it('leaves Base URL empty when no key is present', () => {
    expect(
      resolveCPAManagementBaseURL({
        cpaBaseURL: '',
        managementKey: '',
        hasManagementKey: false,
      })
    ).toBe('');
  });
});

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

  it('defaults Base URL when saving a key with an empty address', () => {
    const body = buildManagerConfigSaveBody({
      currentConfig: {
        cpaConnection: { cpaBaseUrl: '', hasManagementKey: false },
        collector: { enabled: true },
      },
      cpaBaseURL: '',
      managementKey: 'new-secret',
      monitoringEnabled: true,
    });

    expect(body).toEqual({
      config: {
        cpaConnection: {
          cpaBaseUrl: DEFAULT_CPA_MANAGEMENT_BASE_URL,
          managementKey: 'new-secret',
        },
      },
    });
  });

  it('heals empty Base URL when a management key is already bound', () => {
    const body = buildManagerConfigSaveBody({
      currentConfig: {
        cpaConnection: { cpaBaseUrl: '', hasManagementKey: true },
        collector: { enabled: true },
      },
      cpaBaseURL: '',
      managementKey: '',
      monitoringEnabled: false,
    });

    expect(body.config.cpaConnection).toEqual({
      cpaBaseUrl: DEFAULT_CPA_MANAGEMENT_BASE_URL,
    });
    expect(body.config.collector).toEqual({ enabled: false });
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
