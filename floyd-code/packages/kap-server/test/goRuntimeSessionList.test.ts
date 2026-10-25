import { createServer, type ServerResponse } from 'node:http';
import { mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, expect, test } from 'vitest';
import { startGoServer } from '../src/goRuntime/server';

const cleanups: Array<() => Promise<unknown>> = [];
afterEach(async () => { for (const close of cleanups.splice(0).reverse()) await close(); });

async function fixture() {
  const dir = await realpath(await mkdtemp(join(tmpdir(), 'mega-sessions-')));
  cleanups.push(() => rm(dir, { recursive: true, force: true }));
  await mkdir(join(dir, 'data'));
  const socketPath = join(dir, 'go.sock');
  const workspace = { id: 'workspace-go', path: dir, data_dir: join(dir, 'data') };
  const info = { is_ready: true, is_busy: false, model_cfg: { provider: 'example', model: 'example-model' }, model: { id: 'example-model', name: 'Example Model', context_window: 4096 } };
  const makeSession = (id: string, parent: string, archived = false) => ({ id, title: id, parent_session_id: parent, is_branch: Boolean(parent), archived, archived_at: archived ? 1720000002 : 0, metadata: {}, created_at: 1720000000, updated_at: 1720000001, is_busy: false, message_count: 0, prompt_tokens: 0, completion_tokens: 0, cost: 0 });
  const sessions = [makeSession('root-session', ''), makeSession('fork-session', 'root-session'), makeSession('branch-session', 'root-session'), makeSession('archived-fork', 'root-session', true)];
  const config = { providers: { example: { type: 'openai-compat', models: [{ id: 'example-model', name: 'Example Model', context_window: 4096 }] } }, models: { large: { provider: 'example', model: 'example-model' } }, options: {}, mcp: {} };
  const requests: Array<{ method: string; path: string; body: unknown }> = [];
  const streams = new Set<ServerResponse>();
  const go = createServer(async (req, res) => {
    const path = new URL(req.url!, 'http://go').pathname;
    let input = '';
    for await (const chunk of req) input += chunk;
    const body = input ? JSON.parse(input) : undefined;
    requests.push({ method: req.method!, path, body });
    const send = (data: unknown) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(data)); };
    if (path.endsWith('/events')) { res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.flushHeaders(); streams.add(res); res.on('close', () => streams.delete(res)); return; }
    if (path === '/v1/health') return send({ status: 'ok' });
    if (path === '/v1/version') return send({ version: 'canonical-test', build_id: 'session-contracts' });
    if (path === '/v1/handoff/web') return send({ registered: true });
    if (path === '/v1/workspaces') return send(req.method === 'POST' ? workspace : [workspace]);
    if (path.endsWith('/agent') && req.method === 'GET') return send(info);
    if (path.endsWith('/interactions')) return send({ permissions: [], questions: [] });
    if (path.endsWith('/permissions/mode')) return send({ mode: 'manual' });
    if (path.endsWith('/config/plugin-reload')) return send({});
    if (path.endsWith('/config')) return send(config);
    if (path.endsWith('/mcp/states')) return send({});
    if (path.endsWith('/sessions') && req.method === 'GET') return send(sessions);
    const archive = /\/sessions\/([^/]+)\/archive$/.exec(path);
    if (archive && req.method === 'POST') { const session = sessions.find((item) => item.id === archive[1]); if (session) { session.archived = body.archived; session.archived_at = body.archived ? 1720000002 : 0; return send(session); } }
    const sessionMatch = /\/sessions\/([^/]+)$/.exec(path);
    if (sessionMatch) { const index = sessions.findIndex((item) => item.id === sessionMatch[1]); if (index >= 0) { if (req.method === 'DELETE') { sessions.splice(index, 1); return send({}); } return send(sessions[index]); } }
    if (path.includes('/clients/')) return send({});
    res.writeHead(404); send({ message: `Fixture has no ${req.method} ${path}` });
  });
  await new Promise<void>((resolve) => go.listen(socketPath, resolve));
  cleanups.push(async () => { for (const stream of streams) stream.end(); await new Promise<void>((resolve) => go.close(() => resolve())); });
  const web = await startGoServer({ host: '127.0.0.1', port: 0, homeDir: dir, disableAuth: true, hostIdentity: { name: 'example', version: 'test' } as never, env: { MEGAFLOYD_GO_HOST: `unix://${socketPath}`, MEGAFLOYD_GO_CWD: dir, MEGAFLOYD_GO_DATA_DIR: join(dir, 'data') } });
  cleanups.push(() => web.close());
  return { web, sessions, requests };
}

test('canonical session lists retain native forks and branches in flat grouped and archived views', async () => {
  const { web } = await fixture();
  const read = async (url: string) => {
    const response = await web.app.inject({ method: 'GET', url });
    expect(response.statusCode, response.body).toBe(200);
    return response.json().data;
  };
  const expected = ['branch-session', 'fork-session', 'root-session'];
  const flat = await read('/api/v2/sessions');
  expect(flat.items.map((item: { id: string }) => item.id).sort()).toEqual(expected);
  expect(flat.total).toBe(3);
  const grouped = await read('/api/v2/sessions?view=by_workspace');
  expect(grouped.groups).toHaveLength(1);
  expect(grouped.groups[0].sessions.map((item: { id: string }) => item.id).sort()).toEqual(expected);
  expect(grouped.groups[0].total).toBe(3);
  expect((await read('/api/v2/sessions?meta.archived=true&fields=id,archived')).items).toEqual([{ id: 'archived-fork', archived: true }]);
  expect((await read('/api/v2/sessions?meta.archived=all')).total).toBe(4);
  expect((await read('/api/v1/sessions')).items.map((item: { id: string }) => item.id).sort()).toEqual(expected);
  expect((await read('/api/v1/sessions?archived_only=true')).items.map((item: { id: string }) => item.id)).toEqual(['archived-fork']);
});

test('canonical batch session actions accept the single-colon browser URLs', async () => {
  const { web, sessions, requests } = await fixture();
  const post = async (action: string, ids: string[]) => {
    const response = await web.app.inject({ method: 'POST', url: `/api/v2/sessions:${action}`, payload: { ids } });
    expect(response.statusCode, response.body).toBe(200);
    expect(response.json().data).toEqual({ results: ids.map((id) => ({ id, ok: true })), succeeded: ids.length, failed: 0 });
  };
  const ids = ['root-session', 'fork-session'];
  await post('archive', ids);
  expect(sessions.filter((session) => ids.includes(session.id)).every((session) => session.archived)).toBe(true);
  await post('restore', ids);
  expect(sessions.filter((session) => ids.includes(session.id)).every((session) => !session.archived)).toBe(true);
  await post('delete', ['branch-session']);
  expect(sessions.some((session) => session.id === 'branch-session')).toBe(false);
  expect(requests.filter((request) => request.path.endsWith('/archive')).map(({ method, path, body }) => ({ method, path, body }))).toEqual([
    { method: 'POST', path: '/v1/workspaces/workspace-go/sessions/root-session/archive', body: { archived: true } },
    { method: 'POST', path: '/v1/workspaces/workspace-go/sessions/fork-session/archive', body: { archived: true } },
    { method: 'POST', path: '/v1/workspaces/workspace-go/sessions/root-session/archive', body: { archived: false } },
    { method: 'POST', path: '/v1/workspaces/workspace-go/sessions/fork-session/archive', body: { archived: false } },
  ]);
  expect(requests.some((request) => request.method === 'DELETE' && request.path === '/v1/workspaces/workspace-go/sessions/branch-session')).toBe(true);
  expect(requests.some((request) => request.method === 'POST' && request.path.endsWith('/agent'))).toBe(false);
  expect((await web.app.inject({ method: 'POST', url: '/api/v2/sessions::archive', payload: { ids } })).statusCode).toBe(404);
});
