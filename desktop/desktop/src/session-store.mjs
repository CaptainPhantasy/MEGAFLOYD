import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';

const SAFE_ID = /^[A-Za-z0-9._:-]{1,128}$/;
const SESSION_NOT_FOUND = 40401;

/**
 * The desktop may inspect the canonical workspace database when resolving a
 * session link. Parent harness stores are never discovered automatically.
 */
export function chassisDbCandidates({ workspace, dataDir } = {}) {
  if (dataDir) return [path.resolve(dataDir, 'floyd.db')];
  return [];
}

export function findChassisDb(candidates) {
  for (const file of candidates) {
    try {
      if (fs.statSync(file).isFile()) return file;
    } catch {
      // Try the next candidate.
    }
  }
  return null;
}

export function lookupChassisSession({ dbPath, id, exec = execFileSync } = {}) {
  if (!SAFE_ID.test(id)) {
    return { status: 'unknown', error: 'session id is not a plain identifier' };
  }
  if (!dbPath) {
    return { status: 'unknown', error: 'no chassis database found' };
  }
  try {
    if (!fs.statSync(dbPath).isFile()) {
      return { status: 'unknown', error: `not a file: ${dbPath}` };
    }
  } catch {
    return { status: 'unknown', error: `no chassis database at ${dbPath}` };
  }

  // The engine prefixes ids with `session_`; the chassis stores the UUID.
  // Try both forms so a deep link from either surface can find its mirror.
  const candidates = id.startsWith('session_') ? [id, id.slice('session_'.length)] : [id];
  for (const candidate of candidates) {
    if (!SAFE_ID.test(candidate)) continue;
    const escaped = candidate.replace(/'/g, "''");
    const sql = `SELECT id, title, message_count, created_at, updated_at FROM sessions WHERE id = '${escaped}' LIMIT 1;`;
    try {
      const out = exec('sqlite3', ['-ifexists', '-json', dbPath, sql], { encoding: 'utf8' });
      const rows = out.trim().length > 0 ? JSON.parse(out) : [];
      if (rows.length > 0) return { status: 'found', session: rows[0], matchedId: candidate };
    } catch (error) {
      return { status: 'unknown', error: error?.message ?? String(error), dbPath };
    }
  }
  return { status: 'missing', dbPath };
}

export async function lookupEngineSession({ port, token, id, fetchImpl = fetch } = {}) {
  if (!SAFE_ID.test(id)) {
    return { status: 'unknown', error: 'session id is not a plain identifier' };
  }
  if (!port) {
    return { status: 'unknown', error: 'engine port unavailable' };
  }

  const candidates = id.startsWith('session_') ? [id] : [id, `session_${id}`];
  for (const candidate of candidates) {
    try {
      const response = await fetchImpl(
        `http://127.0.0.1:${port}/api/v1/sessions/${encodeURIComponent(candidate)}`,
        {
          headers: token ? { Authorization: `Bearer ${token}` } : {},
          signal: AbortSignal.timeout(3000),
        },
      );
      const body = await response.json().catch(() => null);
      if (body?.code === 0 && body.data) {
        return { status: 'found', session: body.data, matchedId: candidate };
      }
      if (body?.code === SESSION_NOT_FOUND || response.status === 404) {
        continue;
      }
      if (response.status === 401) {
        return { status: 'unknown', error: 'engine rejected the desktop token' };
      }
      return {
        status: 'unknown',
        error: body?.msg ?? `session lookup returned HTTP ${response.status}`,
      };
    } catch (error) {
      return { status: 'unknown', error: error?.message ?? String(error) };
    }
  }
  return { status: 'missing' };
}

export function resolveSessionOwnership({ engine, chassis }) {
  if (engine === 'found' || chassis === 'found') {
    if (engine === 'found' && chassis === 'found') return 'both';
    return engine === 'found' ? 'engine' : 'chassis';
  }
  if (engine === 'unknown' || chassis === 'unknown') return 'unknown';
  return 'none';
}

export function buildSessionNotice({ id, owner, engine, chassis, dbPath }) {
  if (owner === 'chassis') {
    return {
      kind: 'session-store-mismatch',
      store: 'chassis',
      sessionId: id,
      title: 'Session belongs to the CLI store',
      message:
        `Session ${id} exists in the MegaFloyd chassis store` +
        `${dbPath ? ` (${dbPath})` : ''}, not in the web engine. Opening it here would ` +
        `show an empty transcript. Resume it in the terminal with:\n\nmegafloyd -s ${id}`,
    };
  }
  if (owner === 'none') {
    return {
      kind: 'session-not-found',
      store: 'none',
      sessionId: id,
      title: 'Session not found',
      message:
        `Session ${id} was not found in the web engine or the chassis store` +
        `${dbPath ? ` (${dbPath})` : ''}.`,
    };
  }
  const reasons = [engine?.error, chassis?.error].filter(Boolean).join('; ');
  return {
    kind: 'session-store-unknown',
    store: 'unknown',
    sessionId: id,
    title: 'Session ownership unknown',
    message:
      `Could not verify which store owns session ${id}` +
      `${reasons ? `: ${reasons}` : ''}. Refusing to open a possibly empty transcript.`,
  };
}
