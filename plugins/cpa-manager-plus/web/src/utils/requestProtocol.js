export function requestProtocol(row = {}) {
  const explicit = String(row.protocol || row.request_protocol || '').trim().toLowerCase();
  if (explicit === 'websocket' || explicit === 'ws') return 'websocket';
  if (explicit === 'http' || explicit === 'https') return 'http';
  const executor = String(row.executor_type || row.executorType || '').toLowerCase();
  if (executor.includes('websocket')) return 'websocket';
  return 'http';
}

export function requestProtocolLabel(row, translate) {
  const protocol = requestProtocol(row);
  if (typeof translate === 'function') {
    const key = `monitoring.protocol.${protocol}`;
    const label = translate(key);
    if (label && label !== key) return label;
  }
  return protocol === 'websocket' ? 'WebSocket' : 'HTTP';
}
