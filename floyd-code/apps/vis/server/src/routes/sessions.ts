import { Hono } from 'hono';
import { cp, mkdir, rename, rm } from 'node:fs/promises';
import { homedir } from 'node:os';
import { basename, join } from 'node:path';
import { FLOYD_CODE_HOME } from '../config';
import { isLoopbackBound, revealInOs } from '../lib/reveal';
import { listSessions, readSessionDetail } from '../lib/session-store';

const QUARANTINE_ROOT: string = join(homedir(), '.quarantine');

/** Local-time `<YYYYMMDD>T<HHMMSS>` stamp for a quarantine directory name.
 *  Local rather than UTC so the name matches what a human browsing
 *  ~/.quarantine expects. */
function quarantineStamp(now: Date = new Date()): string {
  const pad = (value: number): string => String(value).padStart(2, '0');
  const date = `${String(now.getFullYear())}${pad(now.getMonth() + 1)}${pad(now.getDate())}`;
  const time = `${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  return `${date}T${time}`;
}

/** Move a session directory into the quarantine root instead of destroying
 *  it, so an accidental delete stays recoverable. Returns the quarantine
 *  path. A cross-device rename cannot succeed, so fall back to copy-then-
 *  remove there. */
async function quarantineSessionDir(dir: string): Promise<string> {
  const dest = join(QUARANTINE_ROOT, `${basename(dir)}.${quarantineStamp()}`);
  await mkdir(QUARANTINE_ROOT, { recursive: true });
  try {
    await rename(dir, dest);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== 'EXDEV') throw error;
    await cp(dir, dest, { recursive: true });
    await rm(dir, { recursive: true, force: true });
  }
  return dest;
}

export function sessionsRoute(home: string = FLOYD_CODE_HOME): Hono {
  const r = new Hono();
  r.get('/', async (c) => {
    const sessions = await listSessions(home);
    return c.json({ sessions });
  });
  r.delete('/:id', async (c) => {
    const id = c.req.param('id');
    const all = await listSessions(home);
    const target = all.find((s) => s.sessionId === id);
    if (!target) return c.json({ error: 'session not found', code: 'NOT_FOUND' }, 404);
    const quarantinedTo = await quarantineSessionDir(target.sessionDir);
    return c.json({ sessionId: id, deleted: true, quarantinedTo });
  });
  // Open the session directory in the OS file manager. The folder is
  // opened on the SERVER host, so the endpoint is refused unless the
  // server is bound to loopback.
  r.post('/:id/reveal', async (c) => {
    const id = c.req.param('id');
    if (!isLoopbackBound(c.env)) {
      return c.json(
        { error: 'reveal requires a loopback-bound vis server', code: 'FORBIDDEN' },
        403,
      );
    }
    const detail = await readSessionDetail(home, id);
    if (!detail) return c.json({ error: 'session not found', code: 'NOT_FOUND' }, 404);
    try {
      await revealInOs(detail.sessionDir);
      return c.json({ sessionId: id, opened: detail.sessionDir });
    } catch (err) {
      return c.json(
        { error: `failed to open: ${(err as Error).message}`, code: 'READ_ERROR' },
        500,
      );
    }
  });
  return r;
}
