import { gunzipSync } from 'node:zlib';

let cachedRoleAdditional: string | undefined;

export function inheritedRoleAdditional(): string {
  if (cachedRoleAdditional !== undefined) return cachedRoleAdditional;
  cachedRoleAdditional = '';
  const raw = process.env['FLOYD_ROLE_ADDITIONAL'];
  if (raw === undefined || !raw.startsWith('gz1:')) return cachedRoleAdditional;
  try {
    cachedRoleAdditional = gunzipSync(Buffer.from(raw.slice(4), 'base64')).toString('utf8').trim();
  } catch {
    cachedRoleAdditional = '';
  }
  return cachedRoleAdditional;
}

export function redundantWithInheritedStandard(text: string): boolean {
  const inherited = inheritedRoleAdditional();
  const trimmed = text.trim();
  if (trimmed.length === 0 || inherited.length === 0) return false;
  return inherited.includes(trimmed);
}
