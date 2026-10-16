/**
 * Process-wide region cache for the CLI/TUI.
 *
 * Region picks which deployment slot (`mainland-cn` / `global`) the client's
 * off-session endpoints read their configuration from — each slot has its own
 * env overrides — covering the CDN (updates, plugins, tips), site links, and
 * telemetry. No slot ships a host, so an unconfigured client reaches nothing.
 * The OAuth login flow itself does NOT read this — it takes explicit hosts;
 * this cache is for everything derived afterwards.
 *
 * Resolution lives in `@legacy-ai/floyd-code-oauth` (see `resolveFloydRegion`);
 * this module only adds the one thing that package deliberately does not own:
 * reading the persisted login's oauth ref (credential key + `oauthHost`) out
 * of config.toml, synchronously, via the SDK's safe config reader. First call
 * wins; `refreshFloydRegion` re-resolves after login/logout rewrote the oauth
 * ref.
 */

import { loadRuntimeConfigSafe, resolveConfigPath } from '@legacy-ai/floyd-code-sdk';
import {
  FLOYD_CODE_OAUTH_KEY,
  FLOYD_REGION_PROFILES,
  resolveFloydRegion,
  type FloydRegion,
  type FloydRegionProfile,
} from '@legacy-ai/floyd-code-oauth';

// Same value as DEFAULT_OAUTH_PROVIDER_NAME in '#/constant/app' — inlined here
// to keep the import one-directional (constant/app derives URLs from this
// module, so this module must not import back from it).
const MANAGED_FLOYD_CODE_PROVIDER_KEY = 'managed:floyd-code';

/** Platform-selector value for the global OAuth login entry. */
export const FLOYD_CODE_GLOBAL_PLATFORM_VALUE = 'floyd-code-global';

let cached: FloydRegion | undefined;

export interface PersistedFloydOAuthRef {
  readonly key: string;
  readonly oauthHost?: string;
}

/** The oauth ref persisted by a previous login, if any. */
export function persistedFloydOAuthRef(): PersistedFloydOAuthRef | undefined {
  const result = loadRuntimeConfigSafe(resolveConfigPath({}));
  // `providers` is always present on a real config load; the `?.` guards
  // hosts/tests that hand us a partial config shape.
  const oauth = result.config.providers?.[MANAGED_FLOYD_CODE_PROVIDER_KEY]?.oauth;
  if (oauth === undefined) return undefined;
  return { key: oauth.key, oauthHost: oauth.oauthHost };
}

/** Region for a no-flag `floyd login` / `floyd acp --login`: a fresh install
    follows the resolved region (env/marker/default); the default slot (only
    ever a mainland-cn login) re-pins the profile explicitly; a scoped slot —
    a global login, or a custom env persisted with only FLOYD_CODE_BASE_URL and
    no oauthHost — keeps its configured hosts (`undefined`). */
export function regionForBareLogin(ref: PersistedFloydOAuthRef | undefined): FloydRegion | undefined {
  if (ref === undefined) return currentFloydRegion();
  return ref.key === FLOYD_CODE_OAUTH_KEY ? 'mainland-cn' : undefined;
}

export function currentFloydRegion(): FloydRegion {
  if (cached === undefined) {
    const persisted = persistedFloydOAuthRef();
    cached = resolveFloydRegion({
      configuredOAuthHost: persisted?.oauthHost,
      configuredOAuthKey: persisted?.key,
      readMarker: process.env['FLOYD_CODE_REGION_MARKER'] !== 'off',
    });
  }
  return cached;
}

export function currentFloydProfile(): FloydRegionProfile {
  return FLOYD_REGION_PROFILES[currentFloydRegion()];
}

/** Drop the cache and re-resolve. Call after login/logout rewrote config. */
export function refreshFloydRegion(): FloydRegion {
  cached = undefined;
  return currentFloydRegion();
}
