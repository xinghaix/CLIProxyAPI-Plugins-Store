/** Default CPA management API Base URL (same as windowkeeper default). */
export const DEFAULT_CPA_MANAGEMENT_BASE_URL = 'http://127.0.0.1:8317';

/**
 * Resolve the Base URL to persist when a management key is present.
 * Empty address + key (new or already bound) defaults to the local CPA management port
 * so key-only saves cannot leave BaseURL empty.
 */
export function resolveCPAManagementBaseURL({ cpaBaseURL, managementKey, hasManagementKey }) {
  const baseURL = String(cpaBaseURL || '').trim();
  if (baseURL) return baseURL;
  const key = String(managementKey || '').trim();
  if (key || hasManagementKey) return DEFAULT_CPA_MANAGEMENT_BASE_URL;
  return '';
}

/** Build the minimal PUT body for local runtime configuration changes. */
export function buildManagerConfigSaveBody({ currentConfig, cpaBaseURL, managementKey, monitoringEnabled }) {
  const current = currentConfig || {};
  const connection = current.cpaConnection || {};
  const collector = current.collector || {};
  const next = {};
  const key = String(managementKey || '').trim();
  const baseURL = resolveCPAManagementBaseURL({
    cpaBaseURL,
    managementKey: key,
    hasManagementKey: Boolean(connection.hasManagementKey),
  });

  if (baseURL !== (connection.cpaBaseUrl || '') || key) {
    next.cpaConnection = { cpaBaseUrl: baseURL };
    // managementKey is write-only: never replay a redacted GET value.
    if (key) next.cpaConnection.managementKey = key;
  }
  if (Boolean(monitoringEnabled) !== (collector.enabled !== false)) {
    next.collector = { enabled: Boolean(monitoringEnabled) };
  }

  return { config: next };
}
