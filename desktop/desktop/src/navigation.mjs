import fs from 'node:fs';
import path from 'node:path';

export function normalizeNavigation(value) {
  if (!value || typeof value.cwd !== 'string' || !path.isAbsolute(value.cwd)) return null;
  const result = { cwd: path.resolve(value.cwd) };
  for (const field of ['host', 'session_id']) {
    if (typeof value[field] === 'string' && value[field] && !/[\x00-\x1f]/.test(value[field])) result[field] = value[field];
  }
  if (typeof value.data_dir === 'string' && path.isAbsolute(value.data_dir)) result.data_dir = path.resolve(value.data_dir);
  return result;
}

export function readNavigation(file) {
  try {
    const value = normalizeNavigation(JSON.parse(fs.readFileSync(file, 'utf8')));
    if (!value) throw new Error('Saved desktop navigation has no absolute workspace');
    return value;
  } catch (error) {
    if (error.code === 'ENOENT') return null;
    throw error;
  }
}

export function writeNavigation(file, value) {
  const normalized = normalizeNavigation(value);
  if (!normalized) throw new Error('Cannot remember a workspace without an absolute directory');
  fs.mkdirSync(path.dirname(file), { recursive: true, mode: 0o700 });
  const temp = `${file}.${process.pid}.tmp`;
  fs.writeFileSync(temp, `${JSON.stringify(normalized, null, 2)}\n`, { mode: 0o600 });
  fs.renameSync(temp, file);
  return normalized;
}

export function initialNavigation(saved, env, defaultWorkspace) {
  const explicit = env.MEGAFLOYD_WORKSPACE ? path.resolve(env.MEGAFLOYD_WORKSPACE) : null;
  const base = saved && (!explicit || explicit === saved.cwd) ? saved : { cwd: explicit || defaultWorkspace };
  return normalizeNavigation({ ...base, cwd: explicit || base.cwd, host: env.MEGAFLOYD_GO_HOST || undefined, data_dir: env.MEGAFLOYD_GO_DATA_DIR || base.data_dir });
}

export function parseDesktopRoute(rawUrl) {
  try {
    const url = new URL(rawUrl);
    if (url.protocol !== 'megafloyd:') return null;
    const kind = url.hostname.toLowerCase();
    const id = kind === 'session' ? decodeURIComponent(url.pathname.replace(/^\/+/, '').split('/')[0]) : undefined;
    if (kind !== 'workspace' && (kind !== 'session' || !id)) return { kind: 'home' };
    const cwd = url.searchParams.get('cwd') || url.searchParams.get('path');
    if (cwd && !path.isAbsolute(cwd)) throw new Error('Workspace must be an absolute directory');
    return { kind, session_id: id, cwd: cwd ? path.resolve(cwd) : undefined, host: url.searchParams.get('host') || undefined, data_dir: url.searchParams.get('data_dir') || undefined, home: url.searchParams.get('home') || undefined };
  } catch {
    return null;
  }
}

export function sessionFromURL(rawUrl) {
  try {
    const match = new URL(rawUrl).pathname.match(/^\/sessions\/([^/]+)\/?$/);
    return match ? decodeURIComponent(match[1]) : null;
  } catch {
    return null;
  }
}

export function sessionURL(baseURL, sessionID) {
  const url = new URL(baseURL);
  if (sessionID) url.pathname = `/sessions/${encodeURIComponent(sessionID)}`;
  return url.toString();
}

export async function requestHandoff(baseURL, sessionID, { target, fetchImpl = fetch } = {}) {
  const base = new URL(baseURL);
  const token = new URLSearchParams(base.hash.slice(1)).get('token');
  const endpoint = new URL(sessionID ? `/api/v1/sessions/${encodeURIComponent(sessionID)}/handoff` : '/api/v1/handoff', base);
  const headers = token ? { Authorization: `Bearer ${token}` } : {};
  if (target) headers['Content-Type'] = 'application/json';
  const response = await fetchImpl(endpoint, { method: target ? 'POST' : 'GET', headers, body: target ? JSON.stringify({ target }) : undefined, signal: AbortSignal.timeout(10_000) });
  const result = await response.json();
  if (!response.ok || result.code !== 0 || !result.data) throw new Error(result.message || result.msg || result.error || `Handoff failed (${response.status})`);
  if (!normalizeNavigation(result.data)) throw new Error('Handoff returned no workspace');
  return result.data;
}
