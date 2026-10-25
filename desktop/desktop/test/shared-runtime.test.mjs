import assert from 'node:assert/strict';
import fs from 'node:fs';
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';

import { fetchMeta, handoffUrlFromLine } from '../src/meta.mjs';
import { lookupEngineSession } from '../src/session-store.mjs';

const META = {
  server_version: '2.0.2',
  backend: 'v1',
  capabilities: { websocket: true, file_upload: false, fs_query: true, mcp: true, tasks: false, terminal: true },
};

async function withServer(handler, run) {
  const server = http.createServer(handler);
  await new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));
  try {
    await run(server.address().port);
  } finally {
    await new Promise((resolve) => server.close(resolve));
  }
}

function reply(res, data) {
  res.writeHead(200, { 'content-type': 'application/json' });
  res.end(JSON.stringify({ code: 0, msg: 'success', data, request_id: 'test' }));
}

test('shared Go handoff accepts tokenless Web UI and Local URLs on the chosen port', () => {
  const url = 'http://127.0.0.1:58627/sessions/';
  assert.equal(handoffUrlFromLine('Web UI: ' + url, 58627), url);
  assert.equal(handoffUrlFromLine('Local:  ' + url, 58627), url);
  assert.equal(handoffUrlFromLine('Web UI: ' + url, 58628), null);
  assert.equal(handoffUrlFromLine('Web UI: https://example.com/sessions/', 58627), null);
  assert.equal(handoffUrlFromLine('Unrelated: ' + url, 58627), null);
});

test('desktop tokenless handshake proves Go engine, canonical workspace and data directory', async (t) => {
  const workspace = fs.mkdtempSync(path.join(os.tmpdir(), 'megafloyd-runtime-'));
  t.after(() => fs.rmSync(workspace, { recursive: true, force: true }));
  const dataDir = path.join(workspace, '.megafloyd');
  fs.mkdirSync(dataDir);
  let runtime = {
    engine: 'go',
    host: 'localhost:12345',
    cwd: fs.realpathSync(workspace),
    data_dir: fs.realpathSync(dataDir),
    workspace_id: 'canonical-workspace',
  };
  const valid = { ...runtime };
  const requests = [];
  await withServer(
    (req, res) => {
      requests.push({ url: req.url, authorization: req.headers.authorization });
      if (req.url === '/api/v1/meta') reply(res, META);
      else if (req.url === '/api/v1/runtime') reply(res, runtime);
      else { res.writeHead(404); res.end(); }
    },
    async (port) => {
      const result = await fetchMeta({ port, workspace, attempts: 1 });
      assert.equal(result.ok, true);
      assert.equal(result.runtime.workspace_id, 'canonical-workspace');
      assert.deepEqual(requests.map((request) => request.url), ['/api/v1/meta', '/api/v1/runtime']);
      assert.ok(requests.every((request) => request.authorization === undefined));
      for (const mismatch of [
        { engine: 'typescript' },
        { cwd: path.join(workspace, 'other-project') },
        { data_dir: path.join(workspace, '.floyd') },
        { workspace_id: '' },
      ]) {
        runtime = { ...valid, ...mismatch };
        const refused = await fetchMeta({ port, workspace, attempts: 1 });
        assert.equal(refused.ok, false, JSON.stringify(mismatch));
        assert.match(refused.error, /Expected the Go runtime/);
      }
    },
  );
});

test('desktop deep link resolves a shared Go session without a token', async () => {
  const id = '11111111-1111-4111-8111-111111111111';
  let authorization;
  await withServer(
    (req, res) => {
      authorization = req.headers.authorization;
      assert.equal(req.url, '/api/v1/sessions/' + id);
      reply(res, { id, title: 'Canonical Go session' });
    },
    async (port) => {
      const result = await lookupEngineSession({ port, id });
      assert.equal(result.status, 'found');
      assert.equal(result.matchedId, id);
    },
  );
  assert.equal(authorization, undefined);
});
