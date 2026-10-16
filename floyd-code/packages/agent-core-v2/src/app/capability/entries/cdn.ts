import { floydCdnContentUrl, floydRegionProfile } from '@legacy-ai/floyd-code-oauth';
import type { FloydRegion } from '@legacy-ai/floyd-code-oauth';

export const FLOYD_CODE_CONTENT_CDN_BASE_ENV = 'FLOYD_CODE_CONTENT_CDN_BASE';
export const FLOYD_CODE_CDN_BASE_ENV = 'FLOYD_CODE_CDN_BASE';
export const FLOYD_CODE_GLOBAL_CDN_BASE_ENV = 'FLOYD_CODE_GLOBAL_CDN_BASE';

export function requireContentCdnUrl(path: string, what: string): string {
  const url = floydCdnContentUrl(path);
  if (url.length === 0) {
    throw new Error(
      `${what}: no content CDN is configured. Set ${FLOYD_CODE_CONTENT_CDN_BASE_ENV} to the CDN root URL and retry.`,
    );
  }
  return url;
}

export function requireRegionCdnUrl(region: FloydRegion, path: string, what: string): string {
  const base = floydRegionProfile(region).cdnBase.replace(/\/+$/, '');
  if (base.length === 0) {
    const knob = region === 'global' ? FLOYD_CODE_GLOBAL_CDN_BASE_ENV : FLOYD_CODE_CDN_BASE_ENV;
    throw new Error(
      `${what}: no CDN is configured for the ${region} region. Set ${knob} to the CDN root URL and retry.`,
    );
  }
  return `${base}/${path.replace(/^\/+/, '')}`;
}
