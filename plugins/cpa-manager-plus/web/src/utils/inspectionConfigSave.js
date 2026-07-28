/** Build the minimal PUT body for saving Codex inspection settings. */

export function buildInspectionConfigSaveBody(codexInspection) {
  if (!codexInspection || typeof codexInspection !== 'object') {
    throw new Error('codexInspection is required');
  }
  return {
    config: {
      codexInspection,
    },
  };
}

export function hasRedactedManagementKey(body) {
  const roots = [body, body?.config].filter(Boolean);
  return roots.some((root) => {
    const conn = root.cpaConnection;
    if (!conn || typeof conn !== 'object') return false;
    if ('managementKey' in conn && typeof conn.managementKey !== 'string') return true;
    if ('hasManagementKey' in conn) return true;
    return false;
  });
}
