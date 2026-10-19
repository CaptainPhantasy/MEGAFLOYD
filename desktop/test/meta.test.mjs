import assert from 'node:assert/strict';
import http from 'node:http';
import test from 'node:test';

import {
  EXPECTED_ENGINE_MAJOR,
  fetchMeta,
  interpretMeta,
  parseEnvelope,
  tokenFromUrl,
} from '../src/meta.mjs';

const META = {
  server_version: '2.0.2',
  capabilities: {
    websocket: true,
    file_upload: true,
    fs_query: true,
    mcp: true,
    tasks: true,
    terminal: true,
  },
  server_id: 'test-server',
  started_at: '2026-10-19T12:00:00.000Z',
  open_in_apps: [],
  dangerous_bypass_auth: false,
  backend: 'v2',
  experimental_flags: {},
  features: [{ name: 'plan', state: 'Active', meta: {} }],
};

test('accepts the pinned engine contract', () => {
  const result = interpretMeta(META);
  assert.equal(result.ok, true);
  assert.equal(result.engineVersion, '2.0.2');
  assert.equal(result.protocolMajor, EXPECTED_ENGINE_MAJOR);
  assert.equal(result.warnings.length, 0);
});

test('refuses a different engine major', () => {
  const result = interpretMeta({ ...META, server_version: '3.0.0' });
  assert.equal(result.ok, false);
  assert.equal(result.protocolMajor, 3);
  assert.match(result.error, /major 3 is not supported/);
});

test('a declared protocol_version wins over the server version', () => {
  const result = interpretMeta({ ...META, protocol_version: '3.1' });
  assert.equal(result.ok, false);
  assert.equal(result.protocolMajor, 3);
});

test('unknown capabilities warn but do not fail', () => {
  const result = interpretMeta({
    ...META,
    capabilities: { ...META.capabilities, future_capability: true },
  });
  assert.equal(result.ok, true);
  assert.match(result.warnings.join(' '), /unknown engine capabilities: future_capability/);
});

test('advertised capability gaps warn but do not fail', () => {
  const result = interpretMeta({
    ...META,
    capabilities: { ...META.capabilities, terminal: false },
  });
  assert.equal(result.ok, true);
  assert.match(result.warnings.join(' '), /does not advertise: terminal/);
});

test('failed engine features warn', () => {
  const result = interpretMeta({
    ...META,
    features: [{ name: 'tower', state: 'Failed', meta: {} }],
  });
  assert.equal(result.ok, true);
  assert.match(result.warnings.join(' '), /failed engine features: tower/);
});

test('missing server_version is an error', () => {
  const result = interpretMeta({ capabilities: {} });
  assert.equal(result.ok, false);
  assert.match(result.error, /server_version/);
});

test('non-success envelopes are rejected', () => {
  const result = parseEnvelope({ code: 40101, msg: 'Unauthorized', data: null, request_id: 'x' });
  assert.equal(result.ok, false);
  assert.match(result.error, /Unauthorized/);
});

test('tokenFromUrl reads the handoff fragment only', () => {
  assert.equal(
    tokenFromUrl('http://127.0.0.1:58627/sessions/#token=test-token'),
    'test-token',
  );
  assert.equal(tokenFromUrl('not a url'), null);
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

test('fetchMeta authenticates and interprets a real HTTP reply', async () => {
  let authorization = null;
  await withServer(
    (req, res) => {
      authorization = req.headers.authorization;
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ code: 0, msg: 'success', data: META, request_id: 'x' }));
    },
    async (port) => {
      const result = await fetchMeta({ port, token: 'test-token', attempts: 1 });
      assert.equal(result.ok, true);
      assert.equal(result.engineVersion, '2.0.2');
    },
  );
  assert.equal(authorization, 'Bearer test-token');
});

test('fetchMeta refuses when the engine rejects the token', async () => {
  await withServer(
    (_req, res) => {
      res.writeHead(401, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ code: 40101, msg: 'Unauthorized', data: null, request_id: 'x' }));
    },
    async (port) => {
      const result = await fetchMeta({ port, token: 'bad', attempts: 1 });
      assert.equal(result.ok, false);
      assert.match(result.error, /rejected the desktop token/);
    },
  );
});

test('fetchMeta refuses a newer major over HTTP', async () => {
  await withServer(
    (_req, res) => {
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(
        JSON.stringify({
          code: 0,
          msg: 'success',
          data: { ...META, server_version: '3.0.0' },
          request_id: 'x',
        }),
      );
    },
    async (port) => {
      const result = await fetchMeta({ port, token: 'test-token', attempts: 1 });
      assert.equal(result.ok, false);
      assert.match(result.error, /major 3/);
    },
  );
});
