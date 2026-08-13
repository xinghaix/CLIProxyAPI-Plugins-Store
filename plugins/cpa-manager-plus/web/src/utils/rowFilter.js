export function rowIdentity(row = {}, fallback = '') {
  return String(row.id || row.model || row.api_key_hash || fallback || '');
}

export function canApplySelectedFilter(selectedId, row) {
  const id = rowIdentity(row);
  return Boolean(selectedId) && id !== '' && String(selectedId) === id;
}
