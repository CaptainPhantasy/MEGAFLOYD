import assert from 'node:assert/strict';
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import {
  buildSessionNotice,
  chassisDbCandidates,
  findChassisDb,
  lookupChassisSession,
  lookupEngineSession,
  resolveSessionOwnership,
} from '../src/session-store.mjs';

const SESSION_ID = '11111111-1111-4111-8111-111111111111';

test('ownership resolves every store combination', () => {
  const cases = [
    [{ engine: 'found', chassis: 'found' }, 'both'],
    [{ engine: 'found', chassis: 'missing' }, 'engine'],
    [{ engine: 'found', chassis: 'unknown' }, 'engine'],
    [{ engine: 'missing', chassis: 'found' }, 'chassis'],
    [{ engine: 'unknown', chassis: 'found' }, 'chassis'],
    [{ engine: 'missing', chassis: 'missing' }, 'none'],
    [{ engine: 'unknown', chassis: 'missing' }, 'unknown'],
    [{ engine: 'missing', chassis: 'unknown' }, 'unknown'],
  ];
  for (const [input, expected] of cases) {
    assert.equal(resolveSessionOwnership(input), expected, JSON.stringify(input));
  }
});

test('a chassis-owned session names the store and the resume command', () => {
  const notice = buildSessionNotice({
    id: SESSION_ID,
    owner: 'chassis',
    engine: { status: 'missing' },
    chassis: { status: 'found' },
    dbPath: '/tmp/workspace/.floyd/floyd.db',
  });
  assert.equal(notice.kind, 'session-store-mismatch');
  assert.equal(notice.store, 'chassis');
  assert.match(notice.message, /chassis store/);
  assert.match(notice.message, new RegExp(`megafloyd -s ${SESSION_ID}`));
});

test('an unknown owner refuses instead of opening an empty transcript', () => {
  const notice = buildSessionNotice({
    id: SESSION_ID,
    owner: 'unknown',
    engine: { status: 'unknown', error: 'lookup timed out' },
    chassis: { status: 'unknown', error: 'no chassis database found' },
  });
  assert.equal(notice.kind, 'session-store-unknown');
  assert.match(notice.message, /Refusing to open a possibly empty transcript/);
});

function tempChassisDb() {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'mf-session-'));
  const dbPath = path.join(dir, 'floyd.db');
  execFileSync('sqlite3', [
    dbPath,
    [
      'CREATE TABLE sessions (',
      'id TEXT PRIMARY KEY, parent_session_id TEXT, title TEXT NOT NULL,',
      'message_count INTEGER NOT NULL DEFAULT 0, prompt_tokens INTEGER NOT NULL DEFAULT 0,',
      'completion_tokens INTEGER NOT NULL DEFAULT 0, cost REAL NOT NULL DEFAULT 0.0,',
      'updated_at INTEGER NOT NULL, created_at INTEGER NOT NULL);',
      `INSERT INTO sessions (id, title, message_count, updated_at, created_at) VALUES`,
      `('${SESSION_ID}', 'A chassis session', 4, 1792413000000, 1792412000000);`,
    ].join(' '),
  ]);
  return { dir, dbPath };
}

test('chassis lookup finds a row and reports a missing row', (t) => {
  let sqlite = true;
  try {
    execFileSync('sqlite3', ['-version'], { stdio: 'ignore' });
  } catch {
    sqlite = false;
  }
  if (!sqlite) return t.skip('sqlite3 not installed');

  const { dir, dbPath } = tempChassisDb();
  try {
    const found = lookupChassisSession({ dbPath, id: SESSION_ID });
    assert.equal(found.status, 'found');
    assert.equal(found.session.title, 'A chassis session');

    const missing = lookupChassisSession({
      dbPath,
      id: '22222222-2222-4222-8222-222222222222',
    });
    assert.equal(missing.status, 'missing');

    const prefixed = lookupChassisSession({ dbPath, id: `session_${SESSION_ID}` });
    assert.equal(prefixed.status, 'found');
    assert.equal(prefixed.matchedId, SESSION_ID);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test('chassis lookup rejects a non-identifier session id', () => {
  const result = lookupChassisSession({ dbPath: '/tmp/nope.db', id: '../etc/passwd' });
  assert.equal(result.status, 'unknown');
});

test('chassis candidates prefer the explicit data dir, then workspace, then home', () => {
  const previous = process.env.MEGAFLOYD_DATA_DIR;
  process.env.MEGAFLOYD_DATA_DIR = '/tmp/mf-data';
  try {
    const candidates = chassisDbCandidates({ workspace: '/work/project', home: '/home/user' });
    assert.deepEqual(candidates, [
      '/tmp/mf-data/floyd.db',
      '/work/project/.floyd/floyd.db',
      '/home/user/.floyd/floyd.db',
    ]);
  } finally {
    if (previous === undefined) delete process.env.MEGAFLOYD_DATA_DIR;
    else process.env.MEGAFLOYD_DATA_DIR = previous;
  }
});

test('findChassisDb skips missing candidates', () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'mf-db-'));
  const dbPath = path.join(dir, 'floyd.db');
  fs.writeFileSync(dbPath, '');
  try {
    assert.equal(findChassisDb(['/tmp/definitely-not-here.db', dbPath]), dbPath);
    assert.equal(findChassisDb(['/tmp/definitely-not-here.db']), null);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

async function withServer(handler, run) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  const { port } = server.address();
  try {
    return await run(port);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

test('engine lookup finds a session and sends the token', async () => {
  let authorization = null;
  await withServer(
    (req, res) => {
      authorization = req.headers.authorization;
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(
        JSON.stringify({
          code: 0,
          msg: 'success',
          data: { id: SESSION_ID, title: 'An engine session' },
          request_id: 'x',
        }),
      );
    },
    async (port) => {
      const result = await lookupEngineSession({ port, token: 'test-token', id: SESSION_ID });
      assert.equal(result.status, 'found');
      assert.equal(result.session.title, 'An engine session');
    },
  );
  assert.equal(authorization, 'Bearer test-token');
});

test('engine lookup retries the session_ prefix for a bare UUID', async () => {
  const paths = [];
  await withServer(
    (req, res) => {
      paths.push(req.url);
      res.writeHead(200, { 'content-type': 'application/json' });
      if (req.url.endsWith(`/session_${SESSION_ID}`)) {
        res.end(
          JSON.stringify({
            code: 0,
            msg: 'success',
            data: { id: `session_${SESSION_ID}` },
            request_id: 'x',
          }),
        );
        return;
      }
      res.end(
        JSON.stringify({
          code: 40401,
          msg: 'session does not exist',
          data: null,
          request_id: 'x',
        }),
      );
    },
    async (port) => {
      const result = await lookupEngineSession({ port, token: 'test-token', id: SESSION_ID });
      assert.equal(result.status, 'found');
      assert.equal(result.matchedId, `session_${SESSION_ID}`);
    },
  );
  assert.equal(paths.length, 2);
});

test('engine lookup reports the platform 40401 code as missing', async () => {
  await withServer(
    (_req, res) => {
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(
        JSON.stringify({
          code: 40401,
          msg: 'session does not exist',
          data: null,
          request_id: 'x',
        }),
      );
    },
    async (port) => {
      const result = await lookupEngineSession({ port, token: 'test-token', id: SESSION_ID });
      assert.equal(result.status, 'missing');
    },
  );
});

test('engine lookup treats a rejected token as unknown, not missing', async () => {
  await withServer(
    (_req, res) => {
      res.writeHead(401, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ code: 40101, msg: 'Unauthorized', data: null, request_id: 'x' }));
    },
    async (port) => {
      const result = await lookupEngineSession({ port, token: 'bad', id: SESSION_ID });
      assert.equal(result.status, 'unknown');
    },
  );
});
