import type { OAuthFlowConfig } from './types';

export const DEFAULT_FLOYD_CODE_OAUTH_HOST = '';

/**
 * Fail-closed text for a device-code request with no OAuth host: naming the
 * knobs to set beats letting `/api/oauth/device_authorization` resolve against
 * an empty base and surface as an opaque transport error. Login is optional —
 * a provider under `[providers.*]` in config.toml needs no OAuth at all.
 */
export const FLOYD_CODE_OAUTH_HOST_UNCONFIGURED_MESSAGE =
  'No OAuth host is configured, so device-code login cannot start. Set FLOYD_CODE_OAUTH_HOST to your OAuth server, or for the "global" region slot set FLOYD_CODE_GLOBAL_OAUTH_HOST and FLOYD_CODE_GLOBAL_BASE_URL and run floyd login --region global. Login is optional: a provider under [providers.*] in config.toml needs no OAuth at all.';

/** Node-side env override lookup, resolved through `globalThis` so the module
    stays loadable — and typecheckable — in browser bundles that have no
    `process` global (browser consumers of the ./device entry get the empty
    default unless they inject the override themselves). */
function envOverride(key: string): string | undefined {
  const proc = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process;
  return proc?.env?.[key];
}

export const FLOYD_CODE_FLOW_CONFIG: OAuthFlowConfig = {
  name: 'floyd-code',
  oauthHost:
    envOverride('FLOYD_CODE_OAUTH_HOST') ??
    envOverride('FLOYD_OAUTH_HOST') ??
    DEFAULT_FLOYD_CODE_OAUTH_HOST,
  clientId: '17e5f671-d194-4dfb-9706-5516cb48c098',
};
