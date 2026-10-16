import { FLOYD_REGION_PROFILES } from '@legacy-ai/floyd-code-oauth';
import type { PluginSummary } from '@legacy-ai/floyd-code-sdk';

export const OFFICIAL_BADGE = 'official';
export const CURATED_BADGE = 'curated';
export const THIRD_PARTY_BADGE = 'third-party';

export type PluginTrustLabel = 'official' | 'curated' | 'third-party';

// Trusted plugin hosts are the CDN bases the region profiles actually
// configure (code.<domain>), plus their matching content CDNs (cdn.<domain>).
// A profile that ships no endpoint contributes no host, so nothing is trusted
// by default and every bare zip URL stays third-party until a deployment is
// pointed at a CDN the project controls. The legacy hosts stay recognised
// purely so plugins already installed from them keep their provenance and
// update notices — this is a string match, no request is ever made to them.
const LEGACY_CODE_CDN_HOSTS = ['code.floyd.com', 'code.floyd.ai'];
const CODE_CDN_HOSTS = new Set([
  ...Object.values(FLOYD_REGION_PROFILES)
    .map((profile) => hostFromUrl(profile.cdnBase))
    .filter((host): host is string => host !== undefined),
  ...LEGACY_CODE_CDN_HOSTS,
]);
const CONTENT_CDN_HOSTS = new Set(
  [...CODE_CDN_HOSTS].map((host) => host.replace(/^code\./, 'cdn.')),
);

/**
 * Human-readable provenance label for a plugin, suitable for inline display
 * in `/plugins` overviews and lists.
 *
 * - github source → `github <owner>/<repo>@<ref>`
 * - zip-url with parseable URL → `via <host[:port]>`
 * - everything else → raw source kind (`local-path`, `zip-url`)
 */
export function formatPluginSourceLabel(plugin: PluginSummary): string {
  if (plugin.source === 'github' && plugin.github !== undefined) {
    return `github ${plugin.github.owner}/${plugin.github.repo}@${plugin.github.ref.value}`;
  }
  if (plugin.source === 'zip-url' && plugin.originalSource !== undefined) {
    const host = hostFromUrl(plugin.originalSource);
    if (host !== undefined) return `via ${host}`;
  }
  return plugin.source;
}

/**
 * Returns one of three trust labels for a plugin. Only Floyd-hosted plugin zip
 * paths receive official or curated badges. Everything else is third-party.
 */
export function pluginTrustLabel(plugin: PluginSummary): PluginTrustLabel {
  if (plugin.source !== 'zip-url' || plugin.originalSource === undefined) {
    return 'third-party';
  }
  try {
    const url = new URL(plugin.originalSource);
    if (isOfficialPluginUrl(url)) {
      return 'official';
    }
    if (
      url.protocol === 'https:' &&
      CODE_CDN_HOSTS.has(url.hostname) &&
      url.pathname.startsWith('/floyd-code/plugins/curated/')
    ) {
      return 'curated';
    }
    return 'third-party';
  } catch {
    return 'third-party';
  }
}

/**
 * Returns true only for install sources that are unambiguously Floyd-built
 * official plugins — an https URL under the official Floyd CDN plugin path.
 * Everything else (local paths, GitHub repos, curated or third-party URLs)
 * is treated as unofficial and should be confirmed before install.
 */
export function isOfficialPluginSource(source: string): boolean {
  const trimmed = source.trim();
  if (!trimmed.startsWith('https://')) return false;
  try {
    return isOfficialPluginUrl(new URL(trimmed));
  } catch {
    return false;
  }
}

/**
 * Returns true when an installed plugin provably came from a trusted official
 * source — a zip download under the official CDN plugin path. Local paths,
 * GitHub repos, and third-party URLs do not qualify, even when their manifest
 * id matches an official plugin.
 */
export function isOfficialPluginInstall(plugin: PluginSummary): boolean {
  return (
    plugin.source === 'zip-url' &&
    plugin.originalSource !== undefined &&
    isOfficialPluginSource(plugin.originalSource)
  );
}

function isOfficialPluginUrl(url: URL): boolean {
  if (url.protocol !== 'https:') return false;
  return (
    (CODE_CDN_HOSTS.has(url.hostname) &&
      url.pathname.startsWith('/floyd-code/plugins/official/')) ||
    (CONTENT_CDN_HOSTS.has(url.hostname) &&
      (url.pathname.startsWith('/floyd-computer-use/') ||
        url.pathname.startsWith('/floyd-computer-use-windows/')))
  );
}

function hostFromUrl(raw: string): string | undefined {
  try {
    const url = new URL(raw);
    if (url.port.length > 0) return `${url.hostname}:${url.port}`;
    return url.hostname;
  } catch {
    return undefined;
  }
}
