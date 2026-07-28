/** Build the minimal PUT body for local runtime configuration changes. */
export function buildManagerConfigSaveBody({ currentConfig, cpaBaseURL, managementKey, monitoringEnabled }) {
  const current = currentConfig || {};
  const connection = current.cpaConnection || {};
  const collector = current.collector || {};
  const next = {};
  const baseURL = String(cpaBaseURL || '').trim();
  const key = String(managementKey || '').trim();

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
