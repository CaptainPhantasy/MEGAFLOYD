/**
 * Region profiles for the mainland-China and global deployment slots, plus
 * the resolver that decides which region a client belongs to.
 *
 * A region is a bundle of endpoints (OAuth host, managed API base URL, CDN,
 * site, telemetry). No deployment is baked in: every endpoint resolves to the
 * empty string until that region's env overrides configure it, so an
 * unconfigured client never reaches a host at all. On top of the shared
 * `FLOYD_CODE_OAUTH_HOST` / `FLOYD_OAUTH_HOST` / `FLOYD_CODE_BASE_URL`, each
 * region reads its own CDN / site / telemetry overrides — the `FLOYD_CODE_*`
 * names for mainland-cn and the `FLOYD_CODE_GLOBAL_*` (alias `FLOYD_GLOBAL_*`)
 * names for global. The OAuth client_id is shared across regions and stays in
 * `./constants`.
 *
 * Resolution order (first match wins):
 *   1. env override (`FLOYD_CODE_OAUTH_HOST` / `FLOYD_OAUTH_HOST`, or the
 *      global slot's own overrides)
 *   2. persisted login (the `oauthHost` stored in config.toml's oauth ref)
 *   3. persisted default-slot login (the oauth ref's key equals
 *      `FLOYD_CODE_OAUTH_KEY` — a mainland-China login persists no
 *      `oauthHost`, so the default slot's presence is an explicit-mainland-cn
 *      signal that outranks the marker)
 *   4. install-channel marker file (`<home>/region`, written by install
 *      scripts; consultable only before the first login)
 *   5. default 'mainland-cn'
 *
 * Steps 1 and 2 name a region by comparing the observed host against the
 * legacy vendor hosts below. That list is recognition only — a string
 * comparison, never a request target; every endpoint the client actually
 * contacts comes from the env overrides.
 */

import { readFileSync } from 'node:fs';
import { homedir } from 'node:os';
import { join } from 'node:path';

import { z } from 'zod';

import { DEFAULT_FLOYD_CODE_OAUTH_HOST } from './constants';
import { DEFAULT_FLOYD_CODE_BASE_URL } from './managed-usage';
import { FLOYD_CODE_OAUTH_KEY } from './managed-floyd-code';

export type FloydRegion = 'mainland-cn' | 'global';

/** Zod schema for the wire/domain contract; parses to {@link FloydRegion}. */
export const floydRegionSchema = z.enum(['mainland-cn', 'global']);

export interface FloydRegionProfile {
  /** OAuth host the device flow talks to (authorize/token derive from it). */
  readonly oauthHost: string;
  /** Managed API base (`/coding/v1`): usages, userinfo, models, feedback... */
  readonly baseUrl: string;
  /** Update/install/plugin-marketplace root. */
  readonly cdnBase: string;
  /** Official site root (docs, console, signup, upgrade pages). */
  readonly siteBase: string;
  readonly telemetryEndpoint: string;
}

const FLOYD_REGIONS: readonly FloydRegion[] = ['mainland-cn', 'global'];

type FloydRegionEndpoint = keyof FloydRegionProfile;

/** Env overrides per region endpoint; the first defined non-empty name wins. */
const REGION_ENDPOINT_ENV: Record<FloydRegion, Record<FloydRegionEndpoint, readonly string[]>> = {
  'mainland-cn': {
    oauthHost: ['FLOYD_CODE_OAUTH_HOST', 'FLOYD_OAUTH_HOST'],
    baseUrl: ['FLOYD_CODE_BASE_URL'],
    cdnBase: ['FLOYD_CODE_CDN_BASE'],
    siteBase: ['FLOYD_CODE_SITE_BASE'],
    telemetryEndpoint: ['FLOYD_CODE_TELEMETRY_ENDPOINT'],
  },
  global: {
    oauthHost: ['FLOYD_CODE_GLOBAL_OAUTH_HOST', 'FLOYD_GLOBAL_OAUTH_HOST'],
    baseUrl: ['FLOYD_CODE_GLOBAL_BASE_URL'],
    cdnBase: ['FLOYD_CODE_GLOBAL_CDN_BASE'],
    siteBase: ['FLOYD_CODE_GLOBAL_SITE_BASE'],
    telemetryEndpoint: ['FLOYD_CODE_GLOBAL_TELEMETRY_ENDPOINT'],
  },
};

/** The unconfigured profile: no deployment ships with the client. */
const DEFAULT_REGION_PROFILES: Record<FloydRegion, FloydRegionProfile> = {
  'mainland-cn': {
    oauthHost: DEFAULT_FLOYD_CODE_OAUTH_HOST,
    baseUrl: DEFAULT_FLOYD_CODE_BASE_URL,
    cdnBase: '',
    siteBase: '',
    telemetryEndpoint: '',
  },
  global: {
    oauthHost: '',
    baseUrl: '',
    cdnBase: '',
    siteBase: '',
    telemetryEndpoint: '',
  },
};

/**
 * Legacy vendor OAuth hosts, kept for RECOGNITION only: a persisted or
 * configured host naming one of these names the region it belongs to. Never a
 * request target — contacted endpoints come from the env overrides.
 */
const LEGACY_REGION_OAUTH_HOSTS: Record<FloydRegion, readonly string[]> = {
  'mainland-cn': ['https://auth.floyd.com'],
  global: ['https://auth.floyd.ai'],
};

/** Content-CDN override for {@link floydCdnContentUrl}; shared across regions. */
const CONTENT_CDN_BASE_ENV = 'FLOYD_CODE_CONTENT_CDN_BASE';

function envValue(env: NodeJS.ProcessEnv, name: string): string {
  const value = env[name];
  return value !== undefined && value.length > 0 ? value : '';
}

function envEndpoint(
  region: FloydRegion,
  endpoint: FloydRegionEndpoint,
  env: NodeJS.ProcessEnv,
): string {
  for (const name of REGION_ENDPOINT_ENV[region][endpoint]) {
    const value = envValue(env, name);
    if (value.length > 0) return value;
  }
  return '';
}

function regionEndpoint(
  region: FloydRegion,
  endpoint: FloydRegionEndpoint,
  env: NodeJS.ProcessEnv,
): string {
  const configured = envEndpoint(region, endpoint, env);
  return configured.length > 0 ? configured : DEFAULT_REGION_PROFILES[region][endpoint];
}

export function floydRegionProfile(
  region: FloydRegion,
  env: NodeJS.ProcessEnv = process.env,
): FloydRegionProfile {
  return {
    oauthHost: regionEndpoint(region, 'oauthHost', env),
    baseUrl: regionEndpoint(region, 'baseUrl', env),
    cdnBase: regionEndpoint(region, 'cdnBase', env),
    siteBase: regionEndpoint(region, 'siteBase', env),
    telemetryEndpoint: regionEndpoint(region, 'telemetryEndpoint', env),
  };
}

// The record re-reads `process.env` on every property access so consumers that
// read it long after import (region cache, telemetry, marketplace) see the
// current configuration instead of whatever was set at module load.
function liveRegionProfile(region: FloydRegion): FloydRegionProfile {
  return {
    get oauthHost(): string {
      return floydRegionProfile(region).oauthHost;
    },
    get baseUrl(): string {
      return floydRegionProfile(region).baseUrl;
    },
    get cdnBase(): string {
      return floydRegionProfile(region).cdnBase;
    },
    get siteBase(): string {
      return floydRegionProfile(region).siteBase;
    },
    get telemetryEndpoint(): string {
      return floydRegionProfile(region).telemetryEndpoint;
    },
  };
}

export const FLOYD_REGION_PROFILES: Record<FloydRegion, FloydRegionProfile> = {
  'mainland-cn': liveRegionProfile('mainland-cn'),
  global: liveRegionProfile('global'),
};

/**
 * Content-CDN URL builder (tips banner, WebBridge / Computer-Use binaries).
 * No content-CDN host ships with the client, so the result is empty until
 * `FLOYD_CODE_CONTENT_CDN_BASE` is set — callers must treat empty as "no CDN
 * configured" instead of fetching a relative path. Funnel every content URL
 * through here so pointing at a real CDN touches one function.
 */
export function floydCdnContentUrl(path: string): string {
  const base = envValue(process.env, CONTENT_CDN_BASE_ENV).replace(/\/+$/, '');
  return base.length > 0 ? `${base}/${path.replace(/^\/+/, '')}` : '';
}

/**
 * Login hosts for an explicit region choice, or `undefined` when the chosen
 * profile is unconfigured (both hosts empty — an unconfigured region must not
 * persist an empty host) or when the env overrides of the *other* deployment
 * slot are in play: env keeps full control of the endpoints it configures, so
 * a region pick must not smuggle profile hosts past it (requested hosts
 * outrank env in `resolveFloydCodeLoginAuth`).
 *
 * When returned, both hosts are always set.
 */
export function floydRegionLoginHosts(
  region: FloydRegion,
  env: NodeJS.ProcessEnv = process.env,
): { readonly oauthHost: string; readonly baseUrl: string } | undefined {
  const other: FloydRegion = region === 'mainland-cn' ? 'global' : 'mainland-cn';
  if (
    envEndpoint(other, 'oauthHost', env).length > 0 ||
    envEndpoint(other, 'baseUrl', env).length > 0
  ) {
    return undefined;
  }
  const profile = floydRegionProfile(region, env);
  if (profile.oauthHost.length === 0 || profile.baseUrl.length === 0) return undefined;
  return { oauthHost: profile.oauthHost, baseUrl: profile.baseUrl };
}

/**
 * Marker file name under the Floyd home dir. Install scripts write a single
 * line (`mainland-cn` or `global`) here so a fresh client can default to the
 * region matching the channel it was installed from. It is only consulted
 * while the user has never logged in; a persisted login (config.toml) always
 * wins.
 */
export const FLOYD_REGION_MARKER_FILENAME = 'region';

export interface ResolveFloydRegionOptions {
  /** Defaults to `process.env`. */
  readonly env?: NodeJS.ProcessEnv;
  /** The `oauthHost` persisted in config.toml's oauth ref, if any. */
  readonly configuredOAuthHost?: string;
  /**
   * The credential key persisted in config.toml's oauth ref, if any. The
   * default slot ({@link FLOYD_CODE_OAUTH_KEY}) only ever holds a
   * mainland-China login — mainland-cn persists no `oauthHost` — so its
   * presence is an explicit-mainland-cn signal that outranks the
   * install-channel marker.
   */
  readonly configuredOAuthKey?: string;
  /** Floyd home dir; defaults to `FLOYD_CODE_HOME` or `~/.floyd-code`. */
  readonly homeDir?: string;
  /**
   * Set false to skip the install-channel marker (e.g. the desktop app's
   * embedded server, which is not installed through a channel script and
   * leaves the region choice entirely to the login UI).
   */
  readonly readMarker?: boolean;
}

function normalizeHost(value: string): string {
  return value.trim().replace(/\/+$/, '');
}

// Recognition only: the observed host names a region, it is never contacted.
function regionForOAuthHost(oauthHost: string): FloydRegion | undefined {
  const normalized = normalizeHost(oauthHost);
  for (const region of FLOYD_REGIONS) {
    const known = LEGACY_REGION_OAUTH_HOSTS[region].some(
      (host) => normalizeHost(host) === normalized,
    );
    if (known) return region;
  }
  return undefined;
}

function readRegionMarker(homeDir: string): FloydRegion | undefined {
  let raw: string;
  try {
    raw = readFileSync(join(homeDir, FLOYD_REGION_MARKER_FILENAME), 'utf-8');
  } catch {
    return undefined;
  }
  const value = raw.trim();
  return value === 'mainland-cn' || value === 'global' ? value : undefined;
}

// Mirrors `defaultFloydHome` in ./toolkit; keep the two in sync so the marker
// always lands next to the credentials dir it describes.
function defaultHomeDir(env: NodeJS.ProcessEnv): string {
  const override = env['FLOYD_CODE_HOME'];
  if (override !== undefined && override.length > 0) return override;
  return join(homedir(), '.floyd-code');
}

export function resolveFloydRegion(options: ResolveFloydRegionOptions = {}): FloydRegion {
  const env = options.env ?? process.env;
  // A configured env host pins the deployment slot that configures it, with
  // the legacy vendor hosts recognised as slot names. An unknown host means a
  // custom/internal environment: the per-endpoint env overrides keep doing
  // their job regardless of region, so stay on the slot that configured it
  // instead of letting a stale config/marker point CDN links somewhere odd.
  for (const region of FLOYD_REGIONS) {
    const envHost = envEndpoint(region, 'oauthHost', env);
    if (envHost.length === 0) continue;
    return regionForOAuthHost(envHost) ?? region;
  }
  const configured = options.configuredOAuthHost;
  if (configured !== undefined && configured.length > 0) {
    const configuredRegion = regionForOAuthHost(configured);
    if (configuredRegion !== undefined) return configuredRegion;
  }
  if (options.configuredOAuthKey === FLOYD_CODE_OAUTH_KEY) return 'mainland-cn';
  if (options.readMarker !== false) {
    const markerRegion = readRegionMarker(options.homeDir ?? defaultHomeDir(env));
    if (markerRegion !== undefined) return markerRegion;
  }
  return 'mainland-cn';
}
