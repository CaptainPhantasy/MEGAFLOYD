import fs from 'node:fs';
import path from 'node:path';
import { requestHandoff, sessionFromURL } from './navigation.mjs';

export const EXPECTED_ENGINE_MAJOR = 2;

export const REQUIRED_CAPABILITIES = [
  'websocket',
  'file_upload',
  'fs_query',
  'mcp',
  'tasks',
  'terminal',
];

/** Read the local ready URL emitted by the Go handoff. */
export function handoffUrlFromLine(line, port) {
  const match = /^(?:Web UI|Local):\s+(http:\/\/127\.0\.0\.1:\d+\/\S+)/.exec(line.trim());
  if (!match) return null;
  try {
    const url = new URL(match[1]);
    if (port !== undefined && Number(url.port) !== port) return null;
    return url.toString();
  } catch {
    return null;
  }
}

/** A fragment token is optional for the shared Go runtime. */
export function tokenFromUrl(url) {
  try {
    const parsed = new URL(url);
    const fragment = parsed.hash.startsWith('#') ? parsed.hash.slice(1) : parsed.hash;
    return new URLSearchParams(fragment).get('token');
  } catch {
    return null;
  }
}

/** Platform REST replies are `{ code, msg, data, request_id }`. */
export function parseEnvelope(payload) {
  if (!payload || typeof payload !== 'object') {
    return { ok: false, error: 'meta response is not an object' };
  }
  if (payload.code !== 0) {
    return { ok: false, error: `engine rejected meta: ${payload.msg || payload.code}` };
  }
  if (!payload.data || typeof payload.data !== 'object') {
    return { ok: false, error: 'meta response has no data' };
  }
  return { ok: true, data: payload.data };
}

/**
 * Interpret `/api/v1/meta` for the desktop.
 *
 * The pinned platform reports no `protocol_version`, so the engine version's
 * major is the fallback. A declared `protocol_version` wins when a newer
 * engine adds one. Unknown fields and capabilities are warnings, not failures
 * (spec C4 forward compatibility); a major mismatch refuses the UI.
 */
export function interpretMeta(data, { expectedMajor = EXPECTED_ENGINE_MAJOR } = {}) {
  const base = {
    ok: false,
    engineVersion: null,
    protocolMajor: null,
    platformCommit: null,
    capabilities: {},
    warnings: [],
    error: null,
  };
  if (!data || typeof data !== 'object') {
    return { ...base, error: 'meta data is not an object' };
  }

  const engineVersion = typeof data.server_version === 'string' ? data.server_version : null;
  if (!engineVersion) {
    return { ...base, error: 'engine did not report server_version' };
  }

  const declared = data.protocol_version ?? data.protocolVersion ?? null;
  const majorSource = declared ?? engineVersion;
  const protocolMajor = Number.parseInt(String(majorSource).replace(/^v/i, '').split('.')[0], 10);
  if (!Number.isInteger(protocolMajor)) {
    return { ...base, engineVersion, error: `unparseable protocol version: ${majorSource}` };
  }

  const capabilities =
    data.capabilities && typeof data.capabilities === 'object' ? data.capabilities : {};
  const warnings = [];

  const known = new Set(REQUIRED_CAPABILITIES);
  const extra = Object.keys(capabilities).filter((name) => !known.has(name));
  if (extra.length > 0) warnings.push(`unknown engine capabilities: ${extra.join(', ')}`);

  const missing = REQUIRED_CAPABILITIES.filter((name) => capabilities[name] !== true);
  if (missing.length > 0) warnings.push(`engine does not advertise: ${missing.join(', ')}`);

  const failedFeatures = Array.isArray(data.features)
    ? data.features.filter((feature) => feature?.state === 'Failed').map((feature) => feature?.name)
    : [];
  if (failedFeatures.length > 0) {
    warnings.push(`failed engine features: ${failedFeatures.filter(Boolean).join(', ')}`);
  }

  const platformCommit =
    typeof data.platform_commit === 'string' && data.platform_commit.length > 0
      ? data.platform_commit
      : null;

  const result = {
    ...base,
    engineVersion,
    protocolMajor,
    platformCommit,
    capabilities,
    warnings,
  };

  if (protocolMajor !== expectedMajor) {
    return {
      ...result,
      error: `Engine protocol major ${protocolMajor} is not supported (this desktop requires ${expectedMajor}).`,
    };
  }
  return { ...result, ok: true };
}

/** Fetch and interpret meta; used by the engine after the ready line. */
export async function fetchMeta({
  port,
  token,
  workspace,
  dataDir,
  homeDir,
  fetchImpl = fetch,
  attempts = 3,
  retryDelayMs = 250,
}) {
  let lastError = null;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      const response = await fetchImpl(`http://127.0.0.1:${port}/api/v1/meta`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        signal: AbortSignal.timeout(3000),
      });
      if (response.status === 401) {
        return { ok: false, error: 'engine rejected the desktop token' };
      }
      if (!response.ok) {
        return { ok: false, error: `meta returned HTTP ${response.status}` };
      }
      const envelope = parseEnvelope(await response.json());
      if (!envelope.ok) return envelope;
      const meta = interpretMeta(envelope.data);
      if (!meta.ok || workspace === undefined) return meta;
      const runtime = await fetchRuntime({ port, token, workspace, dataDir, homeDir, fetchImpl, attempts, retryDelayMs });
      return runtime.ok
        ? { ...meta, runtime: runtime.data }
        : { ...meta, ok: false, error: runtime.error };
    } catch (error) {
      lastError = error;
      if (attempt + 1 < attempts) {
        await new Promise((resolve) => setTimeout(resolve, retryDelayMs));
      }
    }
  }
  return { ok: false, error: `meta probe failed: ${lastError?.message ?? 'unknown error'}` };
}


function canonicalPath(value) {
  if (typeof value !== 'string' || value.length === 0) return null;
  try {
    return fs.realpathSync(value);
  } catch {
    return path.resolve(value);
  }
}

/** The default private-store marker; an explicitly configured home takes precedence. */
const CANONICAL_DATA_DIR_NAME = '.megafloyd';

function isCanonicalDataDir(value, homeDir) {
  if (homeDir) {
    const root = canonicalPath(path.join(homeDir, 'data'));
    const candidate = canonicalPath(value);
    const relative = path.relative(root, candidate);
    return relative === '' || (!path.isAbsolute(relative) && relative !== '..' && !relative.startsWith(`..${path.sep}`));
  }
  return path.resolve(value).split(path.sep).includes(CANONICAL_DATA_DIR_NAME);
}

/** Prove the web adapter serves this Go workspace and its own data store. */
export async function fetchRuntime({
  port,
  token,
  workspace,
  dataDir,
  workspaceId,
  homeDir,
  host,
  timeoutMs = 3000,
  fetchImpl = fetch,
  attempts = 3,
  retryDelayMs = 250,
}) {
  let lastError;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      const response = await fetchImpl(`http://127.0.0.1:${port}/api/v1/runtime`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        signal: AbortSignal.timeout(timeoutMs),
      });
      if (!response.ok) return { ok: false, error: `runtime identity returned HTTP ${response.status}` };
      const envelope = parseEnvelope(await response.json());
      if (!envelope.ok) return envelope;
      const runtime = envelope.data;
      // A caller that names a workspace but no store has not said which store to
      // trust, so use the explicitly configured MegaFloyd home or the default
      // native store. A legacy `.floyd` store is never adopted implicitly.
      const dataDirMismatch =
        workspace && typeof runtime.data_dir === 'string'
          ? dataDir
            ? canonicalPath(runtime.data_dir) !== canonicalPath(dataDir)
            : !isCanonicalDataDir(runtime.data_dir, homeDir)
          : false;
      if (
        runtime.engine !== 'go' ||
        typeof runtime.host !== 'string' || runtime.host.length === 0 ||
        typeof runtime.workspace_id !== 'string' || runtime.workspace_id.length === 0 ||
        (workspaceId && runtime.workspace_id !== workspaceId) ||
        (host && runtime.host !== host) ||
        typeof runtime.cwd !== 'string' || !path.isAbsolute(runtime.cwd) ||
        (workspace && canonicalPath(runtime.cwd) !== canonicalPath(workspace)) ||
        typeof runtime.data_dir !== 'string' || !path.isAbsolute(runtime.data_dir) ||
        dataDirMismatch
      ) {
        return { ok: false, error: `Expected the Go runtime for workspace ${workspace} and data ${dataDir}.` };
      }
      return { ok: true, data: runtime };
    } catch (error) {
      lastError = error;
      if (attempt + 1 < attempts) {
        await new Promise((resolve) => setTimeout(resolve, retryDelayMs));
      }
    }
  }
  return { ok: false, error: `runtime probe failed: ${lastError?.message ?? 'unknown error'}` };
}


/** Validate the selected handoff without confusing it with the adapter's primary workspace. */
export async function fetchStartupMeta({ url, workspace, dataDir, host, homeDir, fetchImpl = fetch, attempts = 3, retryDelayMs = 250 }) {
  const port = Number(new URL(url).port);
  const token = tokenFromUrl(url);
  const meta = await fetchMeta({ port, token, fetchImpl, attempts, retryDelayMs });
  if (!meta.ok) return meta;
  const runtime = await fetchRuntime({ port, token, host, fetchImpl, attempts, retryDelayMs });
  if (!runtime.ok) return { ...meta, ok: false, error: runtime.error };
  try {
    const sessionID = sessionFromURL(url);
    const target = await requestHandoff(url, sessionID, { fetchImpl });
    if (canonicalPath(target.cwd) !== canonicalPath(workspace)) throw new Error('Handoff selected a different workspace');
    if (typeof target.data_dir !== 'string' || !path.isAbsolute(target.data_dir) ||
      (dataDir ? canonicalPath(target.data_dir) !== canonicalPath(dataDir) : !isCanonicalDataDir(target.data_dir, homeDir))) {
      throw new Error('Handoff selected a different workspace store');
    }
    if (target.host !== runtime.data.host || (host && target.host !== host)) throw new Error('Handoff selected a different Go host');
    if (sessionID && target.session_id !== sessionID) throw new Error('Handoff selected a different session');
    return { ...meta, runtime: runtime.data, handoff: target };
  } catch (error) {
    return { ...meta, ok: false, error: `handoff probe failed: ${error?.message ?? error}` };
  }
}
