import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterAll, beforeAll, describe, expect, it } from 'vitest';

import { startServer, type RunningServer } from '../src';
import { TEST_HOST_IDENTITY } from './helpers/hostIdentity';
import { authHeaders } from './helpers/auth';

const HTTP_METHODS = new Set([
  'get',
  'put',
  'post',
  'delete',
  'options',
  'head',
  'patch',
  'trace',
]);

const META_ENDPOINTS = ['/openapi.json', '/asyncapi.json', '/'];

describe('API surface snapshot', () => {
  let home: string | undefined;
  let server: RunningServer | undefined;

  beforeAll(async () => {
    home = mkdtempSync(join(tmpdir(), 'floyd-server-v2-api-surface-'));
    server = await startServer({
      hostIdentity: TEST_HOST_IDENTITY,
      host: '127.0.0.1',
      port: 0,
      homeDir: home,
      logLevel: 'silent',
      debugEndpoints: true,
    });
  });

  afterAll(async () => {
    if (server !== undefined) {
      try {
        await server.close();
      } catch {
      }
      server = undefined;
    }
    if (home !== undefined) {
      rmSync(home, { recursive: true, force: true });
      home = undefined;
    }
  });

  it('matches the documented v2 route table and meta endpoints', async () => {
    const base = `http://${server!.host}:${server!.port}`;

    const openApiRes = await fetch(`${base}/openapi.json`, { headers: authHeaders(server as RunningServer) } as never);
    expect(openApiRes.status).toBe(200);
    const openApi = (await openApiRes.json()) as {
      paths?: Record<string, Record<string, unknown>>;
    };
    const paths = openApi.paths ?? {};
    expect(Object.keys(paths).length).toBeGreaterThan(0);

    const routes: Array<[string, string]> = [];
    for (const [path, item] of Object.entries(paths)) {
      for (const key of Object.keys(item)) {
        if (HTTP_METHODS.has(key.toLowerCase())) {
          routes.push([key.toUpperCase(), path]);
        }
      }
    }
    routes.sort((a, b) => a[0].localeCompare(b[0]) || a[1].localeCompare(b[1]));

    const meta: Array<[string, string, number]> = [];
    for (const endpoint of META_ENDPOINTS) {
      const res = await fetch(`${base}${endpoint}`, { headers: authHeaders(server as RunningServer) } as never);
      meta.push(['GET', endpoint, res.status]);
    }
    meta.sort((a, b) => a[0].localeCompare(b[0]) || a[1].localeCompare(b[1]) || a[2] - b[2]);

    expect({ routes, meta }).toMatchSnapshot();
  });

  it('serves the documented bulk plugin install route with authentication and validation', async () => {
    const base = `http://${server!.host}:${server!.port}`;
    const endpoint = `${base}/api/v1/plugins/install-all`;
    const unauthorized = await fetch(endpoint, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ source: 'fixture' }),
    });
    expect(unauthorized.status).toBe(401);

    const invalid = await fetch(endpoint, {
      method: 'POST',
      headers: authHeaders(server!, { 'content-type': 'application/json' }),
      body: JSON.stringify({ source: '' }),
    });
    expect(await invalid.json()).toMatchObject({ code: 40001 });

    const source = join(home!, 'fixture-source');
    const ids = ['first-fixture', 'second-fixture'];
    mkdirSync(join(source, '.claude-plugin'), { recursive: true });
    writeFileSync(
      join(source, '.claude-plugin', 'marketplace.json'),
      JSON.stringify({
        name: 'fixture-pack',
        plugins: ids.map((name) => ({ name, source: `./plugins/${name}` })),
      }),
    );
    for (const name of ids) {
      const directory = join(source, 'plugins', name, '.claude-plugin');
      mkdirSync(directory, { recursive: true });
      writeFileSync(join(directory, 'plugin.json'), JSON.stringify({ name, version: '1.0.0' }));
    }

    const installed = await fetch(endpoint, {
      method: 'POST',
      headers: authHeaders(server!, { 'content-type': 'application/json' }),
      body: JSON.stringify({ source }),
    });
    expect(installed.status).toBe(200);
    const result = await installed.json() as {
      code: number;
      data: { installed: Array<{ id: string; enabled: boolean }>; skipped: unknown[] };
    };
    expect(result.code).toBe(0);
    expect(result.data.installed.map(({ id, enabled }) => [id, enabled])).toEqual([
      ['first-fixture', true],
      ['second-fixture', true],
    ]);
    expect(result.data.skipped).toEqual([]);

    const listed = await fetch(`${base}/api/v1/plugins`, { headers: authHeaders(server!) });
    const plugins = await listed.json() as { data: { plugins: Array<{ id: string }> } };
    expect(plugins.data.plugins.map(({ id }) => id).sort()).toEqual(ids);
  });
});
