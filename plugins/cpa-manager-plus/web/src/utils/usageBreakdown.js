export function cacheTokenCount(row = {}) {
  return Math.max(
    Number(row.cached_tokens || 0),
    Number(row.cache_read_tokens || 0) + Number(row.cache_creation_tokens || 0),
    0,
  );
}

export function buildUsageIOC(row = {}, format = (value) => String(Math.round(Number(value || 0) || 0))) {
  return `I ${format(row.input_tokens)} · O ${format(row.output_tokens)} · C ${format(cacheTokenCount(row))}`;
}
