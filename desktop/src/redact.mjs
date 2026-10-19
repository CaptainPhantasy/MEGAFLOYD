const TOKEN_VALUE = /([#?&;])token=[A-Za-z0-9._~%+\/=-]+/gi;
const TOKEN_LINE = /^(\s*Token:\s*)\S+.*$/gim;
const NON_LOOPBACK_WEB_UI =
  /^(\s*Web UI:\s+)(https?:\/\/(?!127\.0\.0\.1(?::|\/|$)|localhost(?::|\/|$)|\[::1\](?::|\/|$))).*$/gim;

export const REDACTED = '<redacted>';
export const SUPPRESSED_URL = '<non-loopback URL suppressed>';

/**
 * Engine output is logged verbatim only after it passes through here. The raw
 * line still drives readiness parsing; the redacted copy is what reaches disk.
 */
export function redactSecrets(text) {
  return String(text)
    .replace(TOKEN_VALUE, `$1token=${REDACTED}`)
    .replace(TOKEN_LINE, `$1${REDACTED}`)
    .replace(NON_LOOPBACK_WEB_UI, `$1${SUPPRESSED_URL}`);
}

/** Renderer-bound state carries the tokenised URL; strip it before the bridge. */
export function sanitizeState(state) {
  if (!state || typeof state !== 'object') return state;
  const url = typeof state.url === 'string' ? redactSecrets(state.url) : state.url;
  return { ...state, url };
}
