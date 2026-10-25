import { execFileSync } from 'node:child_process';
import { createServer, request, type ServerResponse } from 'node:http';
import { chmod, mkdir, mkdtemp, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { homedir, tmpdir } from 'node:os';
import { join } from 'node:path';
import { once } from 'node:events';
import { afterEach, expect, test } from 'vitest';
import { WebSocket } from 'ws';
import { transcriptResponseSchema } from '@legacy-ai/transcript';
import { startGoServer } from '../src/goRuntime/server';
import { GoRuntime } from '../src/goRuntime/runtime';
import type { GoPermission } from '../src/goRuntime/client';
import { listSkillsResponseSchema } from '../src/protocol/rest-skill';

const cleanups: Array<() => Promise<unknown>> = [];
afterEach(async () => { for (const close of cleanups.splice(0).reverse()) await close(); });

test('canonical native subagents are discovered recursively without treating branches as agents', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'megafloyd-child-sessions-'));
  cleanups.push(() => rm(dir, { recursive: true, force: true }));
  const socket = join(dir, 'go.sock');
  const root = { id: 'root', title: 'Conversation', created_at: 1720000000, parent_session_id: '' };
  const branch = { ...root, id: 'branch', title: 'User branch', parent_session_id: root.id };
  const child = { ...root, id: 'task-child', title: 'Research', parent_session_id: root.id };
  const grandchild = { ...root, id: 'task-grandchild', title: 'Verify', parent_session_id: child.id };
  const foreign = { ...root, id: 'foreign-child', title: 'Other conversation task', parent_session_id: 'other-root' };
  const all = [root, branch, child, grandchild, foreign];
  const server = createServer((req, res) => {
    const path = new URL(req.url!, 'http://fixture').pathname;
    const send = (value: unknown) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(value)); };
    if (path.endsWith('/sessions')) return send([root, branch]);
    if (path.endsWith('/subagents')) return send({ items: [child, grandchild, foreign].filter((item) => item.parent_session_id === path.split('/').at(-2)) });
    if (path.endsWith('/messages')) return send([{ id: 'child-message', session_id: grandchild.id, role: 'assistant', parts: [{ type: 'text', data: { text: 'Verified child result.' } }] }]);
    const session = all.find((item) => path.endsWith(`/sessions/${item.id}`));
    if (session) return send(session);
    if (path.startsWith('/v1/clients/')) return send({});
    res.statusCode = 404; send({ message: 'Unknown native fixture session' });
  });
  await new Promise<void>((resolve) => server.listen(socket, resolve));
  cleanups.push(() => new Promise<void>((resolve) => server.close(() => resolve())));
  const runtime = new GoRuntime(`unix://${socket}`, dir, join(dir, 'data'));
  runtime.workspace = { id: 'fixture-workspace', path: dir, data_dir: join(dir, 'data') };
  cleanups.push(() => runtime.close());
  expect(await runtime.agents(root.id)).toMatchObject([
    { agentId: 'main', type: 'main' },
    { agentId: child.id, type: 'sub', parentAgentId: 'main', label: 'Research' },
    { agentId: grandchild.id, type: 'sub', parentAgentId: child.id, label: 'Verify' },
  ]);
  expect(await runtime.agentSession(root.id, child.id)).toBe(child.id);
  expect(await runtime.messages(await runtime.agentSession(root.id, grandchild.id))).toMatchObject([{ session_id: grandchild.id, parts: [{ data: { text: 'Verified child result.' } }] }]);
  await expect(runtime.agentSession(root.id, foreign.id)).rejects.toMatchObject({ status: 404 });
  await expect(runtime.agentSession(root.id, branch.id)).rejects.toMatchObject({ status: 404 });
  await expect(runtime.agents('missing-root')).rejects.toMatchObject({ status: 404 });
});

test('canonical submitted goals configure before dispatching the actual constrained prompt once', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'megafloyd-submitted-goal-'));
  cleanups.push(() => rm(dir, { recursive: true, force: true }));
  const socket = join(dir, 'go.sock');
  const requests: Array<{ action: string; body: Record<string, unknown>; settings: Record<string, unknown> }> = [];
  const messages: Record<string, unknown>[] = [];
  let settings: Record<string, unknown> = {};
  const server = createServer(async (req, res) => {
    const path = new URL(req.url!, 'http://fixture').pathname;
    let raw = ''; for await (const chunk of req) raw += chunk;
    const body = raw ? JSON.parse(raw) : {};
    const send = (value: unknown) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(value)); };
    if (path.endsWith('/agent') && req.method === 'GET') return send({ is_ready: true, is_busy: false, model_cfg: { provider: 'fixture', model: 'plain' }, model: { id: 'plain', can_reason: false } });
    if (path.endsWith('/sessions/existing-session')) return send({ id: 'existing-session', is_busy: false });
    if (path.endsWith('/messages')) return send(messages);
    if (path.endsWith('/settings') && req.method === 'POST') settings = { ...settings, ...body };
    if (req.method === 'POST') {
      requests.push({ action: path.split('/').at(-1)!, body, settings: { ...settings } });
      if (path.endsWith('/agent')) messages.push({ id: body.run_id, role: 'user', parts: [{ type: 'text', data: { text: body.prompt } }] });
      return send({});
    }
    if (path.startsWith('/v1/clients/')) return send({});
    res.statusCode = 404; send({ message: 'Unexpected goal fixture request' });
  });
  await new Promise<void>((resolve) => server.listen(socket, resolve));
  cleanups.push(() => new Promise<void>((resolve) => server.close(() => resolve())));
  const runtime = new GoRuntime(`unix://${socket}`, dir, join(dir, 'data'));
  runtime.workspace = { id: 'fixture-workspace', path: dir, data_dir: join(dir, 'data') };
  cleanups.push(() => runtime.close());
  for (const [index, goal] of [{ goal_objective: 'Inspect the project' }, { goal_control: 'resume' }].entries()) {
    requests.length = 0;
    await runtime.submit('existing-session', { prompt_id: `goal-prompt-${index}`, ...goal, profile: 'reviewer', disabled_tools: ['edit'], content: [{ type: 'text', text: 'Inspect only; do not edit.' }, { type: 'text', text: 'Report the evidence before any change.' }] });
    expect(requests.map((item) => item.action)).toEqual(['settings', 'goal', 'agent']);
    expect(requests[1]?.body).toMatchObject({ defer_start: true });
    expect(requests[1]?.settings).toEqual({ profile: 'reviewer', disabled_tools: ['edit'] });
    expect(requests[2]?.body).toMatchObject({ session_id: 'existing-session', run_id: `goal-prompt-${index}`, prompt: 'Inspect only; do not edit.\nReport the evidence before any change.' });
    expect(requests.filter((item) => item.action === 'agent')).toHaveLength(1);
  }
  requests.length = 0;
  await runtime.configure({ goal_control: 'resume', profile: 'reviewer' }, 'existing-session');
  expect(requests.map((item) => item.action)).toEqual(['settings', 'goal']);
  expect(requests[1]?.body['defer_start']).not.toBe(true);
});

test('canonical web chat continues the Go session, preserves tools, and recovers approvals on reconnect', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'megafloyd-web-'));
  cleanups.push(() => rm(dir, { recursive: true, force: true }));
  await writeFile(join(dir, 'canonical.txt'), 'The shared Go workspace.\n');
  const historicalRoot = join(dir, 'historical-workspace-not-opened');
  const deletedRoot = join(dir, 'deleted-workspace-not-opened');
  const workspacePreferences = JSON.stringify({ version: 1, workspaces: { primary: { root: dir, name: 'Canonical workspace', created_at: '2024-07-01T00:00:00.000Z', last_opened_at: '2024-07-03T00:00:00.000Z' }, historical: { root: historicalRoot, name: 'Historical workspace', created_at: '2024-07-01T00:00:00.000Z', last_opened_at: '2024-07-02T00:00:00.000Z' }, deleted: { root: deletedRoot, name: 'Deleted workspace', created_at: '2024-07-01T00:00:00.000Z', last_opened_at: '2024-07-02T00:00:00.000Z' } }, deleted_workspace_ids: ['deleted'] });
  await writeFile(join(dir, 'workspaces.json'), workspacePreferences);
  const socketPath = join(dir, 'go.sock');
  const sessionId = '78b31356-5318-4d56-9e03-422358747188';
  const skillInstruction = '# Gateway check\nUse the gateway health tool and report its result.';
  const attachmentBytes = Buffer.from('canonical attachment').toString('base64');
  const session = { archived: false, archived_at: 0, metadata: {} as Record<string, unknown>, id: sessionId, title: 'Terminal conversation', created_at: 1720000000, updated_at: 1720000001, is_busy: false, message_count: 4, prompt_tokens: 10, completion_tokens: 4, cost: 0, parent_session_id: '' };
  const messages = [
    { id: 'user-1', role: 'user', session_id: sessionId, created_at: 1720000000, updated_at: 1720000000, parts: [{ type: 'text', data: { text: 'Check the gateway' } }, { type: 'binary', data: { Path: '/tmp/evidence.txt', MIMEType: 'text/plain', Data: attachmentBytes } }] },
    { id: 'assistant-1', role: 'assistant', session_id: sessionId, created_at: 1720000001, updated_at: 1720000001, parts: [{ type: 'reasoning', data: { thinking: 'Use the health tool.' } }, { type: 'tool_call', data: { id: 'call-1', name: 'mcp_gateway_health', input: '{"scope":"room"}', finished: true } }, { type: 'finish', data: { reason: 'tool_use', time: 1720000001 } }] },
    { id: 'result-1', role: 'tool', session_id: sessionId, created_at: 1720000001, updated_at: 1720000001, parts: [{ type: 'tool_result', data: { tool_call_id: 'call-1', name: 'mcp_gateway_health', content: 'gateway healthy', is_error: false } }] },
    { id: 'assistant-2', role: 'assistant', session_id: sessionId, created_at: 1720000001, updated_at: 1720000001, parts: [{ type: 'text', data: { text: 'The gateway is healthy.' } }, { type: 'finish', data: { reason: 'end_turn', time: 1720000001 } }] },
  ];
  let permissions: GoPermission[] = [{ id: 'permission-1', session_id: sessionId, tool_call_id: 'call-2', tool_name: 'bash', description: 'Read working directory', action: 'execute', params: { command: 'pwd' }, path: dir }];
  let questions: Array<Record<string, unknown>> = [];
  const streams = new Set<ServerResponse>();
  const clientStreams = new Map<string, ServerResponse>();
  const currentSessions = new Map<string, string>();
  const submissions: Record<string, unknown>[] = [];
  const handoffs: Record<string, unknown>[] = [];
  const settingWrites: string[] = [];
  const history: Record<string, unknown>[] = [];
  let subscribed = false;
  let initializations = 0;
  let workspaceAttachments = 0;
  const workspaceCreationRequests: Record<string, unknown>[] = [];
  const nativeWorkspaces = [{ id: 'workspace-go', path: dir, data_dir: join(dir, 'data') }];
  let permissionMode = 'manual';
  const plan = { active: false, id: '', path: '', content: '' };
  const swarm = { active: false, max_parallel: 3 };
  let goal: Record<string, unknown> | null = null;
  const tasks: Array<{ id: string; session_id: string; kind: string; description: string; command?: string; status: string; created_at: string; run_in_background: boolean; output_preview?: string }> = [];
  const queued: Array<{ id: string; run_id: string; prompt: string; created_at: string }> = [];
  const sessions = [session];
  const childMessages = new Map<string, typeof messages>();
  const pluginReloads: Array<Record<string, unknown>> = [];
  const config: Record<string, any> = { providers: { example: { type: 'openai-compat', api_key: 'fixture-secret-never-exposed', models: [{ id: 'example-model', name: 'Example Model', context_window: 4096 }] } }, models: { large: { provider: 'example', model: 'example-model' } }, options: { additional_dirs: [] }, mcp: { gateway: { type: 'http', url: 'https://example.test/mcp' } } };
  const go = createServer(async (req, res) => {
    const url = new URL(req.url!, 'http://go');
    const path = decodeURIComponent(url.pathname);
    const clientId = url.searchParams.get('client_id') ?? '';
    let input = '';
    for await (const chunk of req) input += chunk;
    const body = input ? JSON.parse(input) : {};
    if (req.method === 'POST' && ['/config/model', '/permissions/mode', '/plan', '/swarm'].some((suffix) => path.endsWith(suffix))) settingWrites.push(path);
    const send = (data: unknown) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(data)); };
    if (path.endsWith('/events')) { subscribed = true; clientStreams.set(clientId, res); res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.flushHeaders(); streams.add(res); res.on('close', () => { streams.delete(res); if (clientStreams.get(clientId) === res) { clientStreams.delete(clientId); currentSessions.delete(clientId); } }); return; }
    if (path.endsWith('/current-session')) { if (!clientStreams.has(clientId)) { res.writeHead(409); return send({ message: 'client is not attached' }); } currentSessions.set(clientId, body.session_id); res.end(); return; }
    if (path === '/v1/health') return send({ status: 'ok' });
    if (path === '/v1/handoff/web' && req.method === 'POST') return send({ registered: true });
    if (path === '/v1/version') return send({ version: 'canonical-test', build_id: 'go-canonical' });
    if (path === '/v1/workspaces' && req.method === 'POST') {
      workspaceAttachments++;
      workspaceCreationRequests.push(body);
      const existing = nativeWorkspaces.find((item) => item.path === body.path);
      if (existing) return send(existing);
      const created = { id: 'workspace-external', path: body.path, data_dir: join(dir, 'private-workspaces', 'external-project') };
      nativeWorkspaces.push(created);
      return send(created);
    }
    if (path === '/v1/workspaces') return send(nativeWorkspaces);
    if (path === '/v1/workspaces/workspace-external/sessions') return send([]);
    if (path.endsWith('/agent/init')) { initializations++; expect(body.interactive).toBe(true); return send({}); }
    if (path.endsWith('/skills/read')) { expect(body).toEqual({ skill_id: 'gateway-check' }); return send({ content: Buffer.from(skillInstruction).toString('base64'), result: { name: 'gateway-check' } }); }
    if (path.endsWith('/skills')) return send([{ id: 'gateway-check', name: 'gateway-check', description: 'Check gateway health', source: 'builtin', user_invocable: true }]);
    if (path.endsWith('/handoff')) { if (req.method === 'POST') handoffs.push({ ...body, path }); return send({ cwd: dir, data_dir: join(dir, 'data'), session_id: sessionId, host: `unix://${socketPath}`, web_url: `http://127.0.0.1:58627/sessions/${sessionId}`, desktop_url: `megafloyd://session/${sessionId}?cwd=${encodeURIComponent(dir)}`, terminal_command: `megafloyd --cwd '${dir}' --session '${sessionId}'`, ...(req.method === 'POST' ? { target: body.target, opened: true } : {}) }); }
    if (path.endsWith('/mcp/states')) return send({ gateway: { name: 'gateway', state: 'connected', tool_count: 12 } });
    if (path.endsWith('/tools')) return send([{ name: 'mcp_gateway_health', description: 'Gateway health', input_schema: { type: 'object' } }]);
    if (path.endsWith('/prompts/list')) return send([]);
    if (path.endsWith('/permissions/mode')) { if (req.method === 'POST') permissionMode = body.mode; return send({ mode: permissionMode }); }
    if (path.endsWith('/auth/account')) return send({ provider: 'managed:floyd-code', authenticated: false, status: 'unauthenticated', region: 'global', base_url: 'https://example.test', oauth_host: 'https://example.test' });
    if (path.endsWith('/config/plugin-reload')) { pluginReloads.push(body); return send({}); }
    if (path.endsWith('/config/set') || path.endsWith('/config/remove')) { const fields = (body.key.match(/(?:\\.|[^.])+/g) || []).map((field: string) => field.replace(/\\(.)/g, '$1')); let target = config; for (const field of fields.slice(0, -1)) target = target[field] ??= {}; if (path.endsWith('/remove')) delete target[fields.at(-1)]; else target[fields.at(-1)] = body.value; return send({}); }
    if (path.endsWith('/config/model')) { config['models'][body.model_type] = body.model; return send({}); }
    if (path.endsWith('/agent/update')) return send({});
    if (path.endsWith('/config')) return send(config);
    if (path.endsWith('/workspace/add-dir')) { config['options'].additional_dirs.push(body.path); return send({ project_root: dir, config_path: join(dir, '.megafloyd.json'), additional_dirs: config['options'].additional_dirs, persisted: body.persist !== false }); }
    if (path.endsWith('/plan')) { if (req.method === 'POST') { plan.active = body.action === 'enter' || body.action === 'update'; plan.content = body.content || plan.content; } return send(plan); }
    if (path.endsWith('/swarm')) { if (req.method === 'POST') Object.assign(swarm, body); return send(swarm); }
    if (path.endsWith('/goal')) { if (req.method === 'POST') goal = body.objective ? { objective: body.objective, status: 'active', budget: { turnBudget: 3 }, tokensUsed: 0 } : goal ? { ...goal, status: body.control === 'pause' ? 'paused' : 'active' } : null; return send(goal); }
    if (path.endsWith('/queue')) return send({ items: queued, active: session.is_busy ? { id: 'browser-turn-1', run_id: 'browser-turn-1', user_message_id: 'go-browser-user-1', prompt: 'Continue here.', created_at: '2024-07-03T09:46:42.000Z' } : null, count: queued.length });
    if (path.endsWith('/queue/steer')) { for (const id of body.ids) queued.splice(queued.findIndex((item) => item.id === id), 1); return send({ steered: true, ids: body.ids }); }
    if (path.includes('/queue/')) { const id = path.split('/').at(-1); const item = queued.find((item) => item.id === id); if (req.method === 'DELETE') queued.splice(queued.indexOf(item!), 1); else if (item) item.prompt = body.prompt; return send(item || {}); }
    if (path.endsWith('/tasks')) return send({ items: tasks });
    if (path.includes('/tasks/')) { const action = path.split('/').at(-1); const task = tasks.find((item) => item.id === (action === 'cancel' || action === 'detach' ? path.split('/').at(-2) : action)); if (!task) { res.writeHead(404); return send({ message: 'task missing' }); } if (action === 'cancel') { task.status = 'cancelled'; return send({ cancelled: true }); } if (action === 'detach') { task.run_in_background = true; return send({ detached: true, status: task.status }); } return send(task); }
    if (path.endsWith('/settings')) { if (req.method === 'POST') session.metadata['agent_config'] = { ...session.metadata['agent_config'] as object, ...body }; return send(session.metadata['agent_config'] || {}); }
    if (path.endsWith('/fork') || path.endsWith('/branch')) { const child = { ...session, id: path.endsWith('/fork') ? 'forked-session' : 'branched-session', title: body.title || 'Child', parent_session_id: sessionId, metadata: body.metadata || {}, message_count: path.endsWith('/fork') ? messages.length : 0 }; sessions.push(child); childMessages.set(child.id, path.endsWith('/fork') ? structuredClone(messages) : []); return send(child); }
    if (path.endsWith('/undo')) return send({ session, removed_turns: body.count || 1 });
    if (path.endsWith('/mcp/test')) return send({ success: true, output: 'One tool discovered by the Go fixture.' });
    if (path.endsWith('/mcp/auth-reset')) return send({});
    if (path.endsWith('/agent') && req.method === 'GET') return send({ is_ready: true, is_busy: false, model_cfg: { provider: 'example', model: 'example-model' }, model: { id: 'example-model', name: 'Example Model', context_window: 4096 } });
    if (path.endsWith('/interactions')) { expect(subscribed).toBe(true); return send({ permissions, questions }); }
    if (path.endsWith('/sessions') && req.method === 'GET') return send(sessions);
    if (path.endsWith('/subagents')) return send({ items: [] });
    if (path.endsWith(`/sessions/${sessionId}`)) return send({ ...session, attached_clients: [...currentSessions].filter(([id, selected]) => selected === sessionId && clientStreams.has(id)).length });
    if (path.endsWith('/messages')) return send(childMessages.get(path.split('/').at(-2)!) || messages);
    const child = sessions.find((item) => path.endsWith(`/sessions/${item.id}`));
    if (child) return send(child);
    if (path.endsWith('/history')) return send(history);
    if (path.endsWith('/archive')) { session.archived = body.archived; session.archived_at = body.archived ? 1720000002 : 0; return send(session); }
    if (path.endsWith('/metadata')) { session.metadata = { ...session.metadata, ...body.metadata }; if (body.title) session.title = body.title; return send(session); }
    if (path.endsWith('/file-history/turns')) return send({ turns: [{ turn_id: 0, user_message_id: 'user-1', recorded: history.length > 0 }] });
    if (path.endsWith('/file-history/changes')) { expect(url.searchParams.get('turn_id')).toBe('0'); return send({ changes: history.length ? [{ path: '/workspace/example.txt', status: 'modified', additions: 1, deletions: 0 }] : [], recorded: history.length > 0 }); }
    if (path.endsWith('/file-history/content')) return send({ content: { version: 1, content: url.searchParams.get('phase') === 'start' ? '' : 'A recorded edit.' } });
    if (path.endsWith('/permissions/grant')) { expect(body.permission).toEqual(permissions[0]); expect(body.action).toBe('allow'); permissions = []; return send({ resolved: true }); }
    if (path.endsWith('/questions/answer')) { expect(body).toEqual({ batch_request_id: 'question-1', responses: [{ request_id: 'q1', selected_ids: ['one', 'two'] }] }); questions = []; return send({ resolved: true }); }
    if (path.endsWith('/agent') && req.method === 'POST') {
      submissions.push(body);
      if (body.run_id === 'browser-turn-1') {
        session.is_busy = true;
        messages.push({ id: 'go-browser-user-1', role: 'user', session_id: sessionId, created_at: 1720000002, updated_at: 1720000002, parts: [{ type: 'text', data: { text: body.prompt } }] });
      }
      res.writeHead(202); res.end(); return;
    }
    if (path.includes('/clients/')) { const id = path.slice(path.lastIndexOf('/') + 1); currentSessions.delete(id); clientStreams.get(id)?.end(); res.end(); return; }
    res.writeHead(404); send({ message: `fixture has no ${req.method} ${path}` });
  });
  await new Promise<void>((resolve) => go.listen(socketPath, resolve));
  const attachedClients = () => new Promise<number>((resolve, reject) => {
    const req = request({ socketPath, path: `/v1/workspaces/workspace-go/sessions/${sessionId}` }, (res) => {
      let body = '';
      res.setEncoding('utf8');
      res.on('data', (chunk: string) => { body += chunk; });
      res.on('error', reject);
      res.on('end', () => resolve(JSON.parse(body).attached_clients));
    });
    req.on('error', reject);
    req.end();
  });
  cleanups.push(async () => { for (const res of streams) res.end(); await new Promise<void>((resolve) => go.close(() => resolve())); });
  const web = await startGoServer({ host: '127.0.0.1', port: 0, homeDir: dir, disableAuth: true, hostIdentity: { name: 'example', version: 'test' } as never, env: { MEGAFLOYD_GO_HOST: `unix://${socketPath}`, MEGAFLOYD_GO_CWD: dir, MEGAFLOYD_GO_DATA_DIR: join(dir, 'data') } });
  cleanups.push(() => web.close());
  expect(initializations).toBe(0);
  const api = async (path: string, body?: unknown) => {
    const result = await web.app.inject({ method: body === undefined ? 'GET' : 'POST', url: `/api/v1${path}`, payload: body === undefined ? undefined : JSON.stringify(body), headers: body === undefined ? {} : { 'content-type': 'application/json' } });
    expect(result.statusCode, `${path}: ${result.body}`).toBe(200);
    const envelope = result.json();
    expect(envelope.code).toBe(0);
    return envelope.data;
  };
  expect(await api('/runtime')).toMatchObject({ engine: 'go', data_dir: join(dir, 'data'), workspace_id: 'workspace-go' });
  expect((await web.app.inject({ method: 'GET', url: '/api/v1/company/account' })).statusCode).toBe(409);
  config['options'].company_platform_url = 'https://company.example.test';
  const accountLink = await web.app.inject({ method: 'GET', url: '/api/v1/company/account' });
  expect(accountLink.statusCode).toBe(302);
  expect(accountLink.headers.location).toBe('https://company.example.test/account');
  expect((await api('/meta')).web_title).toBe('MegaFloyd');
  const handoffConfig = JSON.stringify(config);
  await api(`/sessions/${sessionId}/profile`, { agent_config: { thinking: 'off', permission_mode: 'manual', plan_mode: false, swarm_mode: false } });
  expect(settingWrites).toEqual([]);
  const handoffMessages = JSON.stringify(messages);
  expect(await api(`/sessions/${sessionId}/handoff`)).toMatchObject({ cwd: dir, data_dir: join(dir, 'data'), session_id: sessionId, host: `unix://${socketPath}` });
  expect(await api('/handoff')).toMatchObject({ cwd: dir, host: `unix://${socketPath}` });
  const commandCatalog = listSkillsResponseSchema.parse(await api(`/sessions/${sessionId}/skills`));
  for (const target of ['web', 'tui', 'desktop']) {
    expect(commandCatalog.skills).toContainEqual(expect.objectContaining({ name: target, source: 'builtin', type: 'command', disable_model_invocation: true, scopes: ['web'] }));
    const activated = await api(`/sessions/${sessionId}/skills/${target}:activate`, { args: `[${target}](floyd-code://skill/${target})`, metadata: { floyd_code_composer: { version: 1, doc: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'mention', attrs: { kind: 'skill', name: target, path: '' } }, { type: 'text', text: ' ' }] }] }, attachments: [], attachmentOrder: [] }, display_text: `${target} ` } });
    expect(activated).toMatchObject({ activated: true, skill_name: target, status: 'completed', handoff: { target, opened: true, session_id: sessionId, cwd: dir } });
    const completedControl = transcriptResponseSchema.parse(await api(`/sessions/${sessionId}/transcript?agent_id=main`));
    expect(completedControl.items).toContainEqual(expect.objectContaining({ kind: 'marker', marker: 'skill', payload: { origin: { kind: 'skill_activation', trigger: 'user-slash', skillName: target, skillArgs: `[${target}](floyd-code://skill/${target})` }, status: 'completed' } }));
    expect(completedControl.items.filter((item) => item.kind === 'turn')).toHaveLength(1);
    expect(await api(`/sessions/${sessionId}/prompts`, { prompt_id: `handoff-${target}`, model: 'must-not-select', permission_mode: 'yolo', content: [{ type: 'text', text: `/${target}` }] })).toMatchObject({ prompt_id: `handoff-${target}`, status: 'completed', handoff: { target, opened: true } });
  }
  const duplicateHandoff = await api(`/sessions/${sessionId}/prompts`, { prompt_id: 'handoff-web', model: 'must-not-select', permission_mode: 'yolo', content: [{ type: 'text', text: '/web' }] });
  expect(duplicateHandoff.status).toBe('completed');
  expect(handoffs.map((item) => item['target'])).toEqual(['web', 'web', 'tui', 'tui', 'desktop', 'desktop']);
  for (const text of ['/web extra', '/tui\nrun something', '/desktop please']) {
    const invalid = await web.app.inject({ method: 'POST', url: `/api/v1/sessions/${sessionId}/prompts`, payload: { content: [{ type: 'text', text }] } });
    expect(invalid.statusCode, invalid.body).toBe(400);
  }
  for (const args of [undefined, ' \n\t']) {
    expect(await api(`/sessions/${sessionId}/skills/tui:activate`, { args })).toMatchObject({ activated: true, status: 'completed', handoff: { target: 'tui', opened: true } });
  }
  const validHandoffs = handoffs.length;
  for (const body of [
    { args: 'please' },
    { args: '[desktop](floyd-code://skill/desktop)' },
    { args: '[tui](floyd-code://skill/tui) extra' },
    { args: '[tui](floyd-code://skill/tui)', attachments: [{ type: 'text', text: 'attachment' }] },
    { args: '[tui](floyd-code://skill/tui)', metadata: { floyd_code_composer: { attachments: [{}] } } },
  ]) {
    const invalid = await web.app.inject({ method: 'POST', url: `/api/v1/sessions/${sessionId}/skills/tui:activate`, payload: body });
    expect(invalid.statusCode, invalid.body).toBe(400);
  }
  expect(handoffs).toHaveLength(validHandoffs);
  expect(submissions).toEqual([]);
  expect(JSON.stringify(config)).toBe(handoffConfig);
  expect(JSON.stringify(messages)).toBe(handoffMessages);
  expect(permissionMode).toBe('manual');
  expect(await api('/oauth/region')).toEqual({ region: 'global' });
  expect(await api('/fs:home')).toMatchObject({ recent_roots: [dir, historicalRoot] });
  const external = await mkdtemp(join(tmpdir(), 'megafloyd-external-'));
  cleanups.push(() => rm(external, { recursive: true, force: true }));
  await mkdir(join(external, 'Example project'));
  await mkdir(join(external, 'blocked'));
  await writeFile(join(external, 'document.txt'), 'Existing external file.');
  await symlink(join(external, 'Example project'), join(external, 'Project link'));
  const externalPath = await realpath(external);
  const beforeBrowseAttachments = workspaceAttachments;
  expect(await api(`/fs:browse?path=${encodeURIComponent(external)}`)).toMatchObject({ path: externalPath, entries: [
    { name: 'blocked', path: join(externalPath, 'blocked'), is_dir: true },
    { name: 'Example project', path: join(externalPath, 'Example project'), is_dir: true },
    { name: 'Project link', path: join(externalPath, 'Project link'), is_dir: true },
  ] });
  expect(await api(`/fs:browse?path=${encodeURIComponent(join(external, 'Project link'))}`)).toMatchObject({ path: join(externalPath, 'Example project'), parent: externalPath, entries: [] });
  expect(await api('/fs:browse?path=%2F')).toMatchObject({ path: '/', parent: null });
  expect(await api('/fs:browse')).toMatchObject({ path: await realpath(homedir()), entries: expect.arrayContaining([{ name: 'Filesystem', path: '/', is_dir: true }]) });
  for (const [path, status] of [['relative/path', 400], [join(external, 'missing'), 404], [join(external, 'document.txt'), 404]] as const) {
    const response = await web.app.inject({ method: 'GET', url: `/api/v1/fs:browse?path=${encodeURIComponent(path)}` });
    expect(response.statusCode).toBe(status);
    expect(response.json().data).toBeNull();
  }
  await mkdir(join(external, 'blocked', 'private-folder'));
  await symlink(join(external, 'blocked', 'private-folder'), join(external, 'Protected link'));
  await chmod(join(external, 'blocked'), 0);
  try {
    expect(await api(`/fs:browse?path=${encodeURIComponent(external)}`)).toMatchObject({ entries: expect.arrayContaining([{ name: 'Protected link (access denied)', path: join(externalPath, 'Protected link'), is_dir: true }, { name: 'Example project', path: join(externalPath, 'Example project'), is_dir: true }]) });
    const linkDenied = await web.app.inject({ method: 'GET', url: `/api/v1/fs:browse?path=${encodeURIComponent(join(external, 'Protected link'))}` });
    expect(linkDenied.statusCode).toBe(403);
    const denied = await web.app.inject({ method: 'GET', url: `/api/v1/fs:browse?path=${encodeURIComponent(join(external, 'blocked'))}` });
    expect(denied.statusCode).toBe(403);
    expect(denied.json().msg).toContain('operating system denied');
  } finally { await chmod(join(external, 'blocked'), 0o700); }
  expect(workspaceAttachments).toBe(beforeBrowseAttachments);
  expect(JSON.stringify(config)).toBe(handoffConfig);

  expect((await api('/workspaces')).items).toEqual([expect.objectContaining({ root: dir, name: 'Canonical workspace', session_count: 1 }), expect.objectContaining({ root: historicalRoot, name: 'Historical workspace', session_count: null })]);
  expect(workspaceAttachments).toBe(1);
  expect(await readFile(join(dir, 'workspaces.json'), 'utf8')).toBe(workspacePreferences);
  await expect(readFile(join(dir, 'go-workspaces.json'), 'utf8')).rejects.toMatchObject({ code: 'ENOENT' });
  await api('/workspaces', { root: dir, name: 'Canonical Go workspace' });
  expect(await readFile(join(dir, 'workspaces.json'), 'utf8')).toBe(workspacePreferences);
  expect(JSON.parse(await readFile(join(dir, 'go-workspaces.json'), 'utf8'))).toContainEqual(expect.objectContaining({ root: historicalRoot, name: 'Historical workspace' }));
  expect(workspaceAttachments).toBe(1);
  const listV2 = async (query: string) => {
    const response = await web.app.inject({ method: 'GET', url: `/api/v2/sessions?${query}` });
    expect(response.statusCode, response.body).toBe(200);
    return response.json().data;
  };
  const grouped = await listV2('view=by_workspace&group.page_size=5&meta.has_prompt=true');
  expect(grouped).toMatchObject({ total: 1, has_more: false, next_page_token: null, groups: [{ workspace: { cwd: dir }, total: 1, sessions: [{ id: sessionId, meta: { title: session.title, last_prompt: 'Check the gateway', created_at: 1720000000000, archived: false }, activity: { status: 'approval', model: 'example/example-model' } }] }] });
  const flat = await listV2('page_size=50&include=git');
  expect(flat).toMatchObject({ total: 1, items: [{ id: sessionId, git: { branch: null, pull_request: null } }] });
  expect(await listV2('page_size=50&meta.archived=true&include=git')).toEqual({ total: 0, items: [], has_more: false, next_page_token: null });
  const archive = await web.app.inject({ method: 'POST', url: '/api/v2/sessions:archive', payload: { ids: [sessionId] } });
  expect(archive.statusCode, archive.body).toBe(200);
  expect(archive.json().data).toMatchObject({ results: [{ id: sessionId, ok: true }], succeeded: 1, failed: 0 });
  expect((await listV2('meta.archived=true')).items[0]).toMatchObject({ id: sessionId, meta: { archived: true, archived_at: 1720000002000 } });
  expect(await api(`/sessions/${sessionId}:restore`, {})).toMatchObject({ archived: false });
  expect(await api(`/sessions/${sessionId}:update`, { metadata: { pinned: true } })).toMatchObject({ metadata: { pinned: true, cwd: dir } });
  expect(session.metadata).toEqual({ pinned: true });
  expect((await api('/models')).items).toContainEqual(expect.objectContaining({ provider: 'example', model: 'example/example-model' }));
  expect(JSON.stringify(await api('/config'))).not.toContain('fixture-secret-never-exposed');
  const catalogConfigBefore = JSON.stringify(config);
  expect(await api('/config')).toMatchObject({ default_permission_mode: 'manual', yolo: false, models: {
    'example/example-model': { provider: 'example', model: 'example-model', displayName: 'Example Model', maxContextSize: 4096, capabilities: [], supportEfforts: [] },
  } });
  expect(Object.values((await api('/config')).models)).toHaveLength((await api('/models')).items.length);
  expect(JSON.stringify(config)).toBe(catalogConfigBefore);
  expect(await api(`/sessions/${sessionId}/status`)).toMatchObject({ permission: 'manual' });
  await api('/config', { default_permission_mode: 'auto' });
  expect(await api(`/sessions/${sessionId}/status`)).toMatchObject({ permission: 'auto' });
  await api('/config', { default_permission_mode: 'manual' });
  await api('/config', { default_permission_mode: 'yolo' });
  expect(await api(`/sessions/${sessionId}/status`)).toMatchObject({ permission: 'yolo' });
  await api('/config', { default_permission_mode: 'manual' });
  expect((await api('/sessions')).items[0].id).toBe(sessionId);
  expect((await api('/mcp/servers')).servers).toContainEqual(expect.objectContaining({ name: 'gateway', status: 'connected', tool_count: 12 }));
  expect((await api('/tools')).tools).toContainEqual(expect.objectContaining({ name: 'mcp_gateway_health' }));
  expect(await api(`/sessions/${sessionId}/tasks`)).toEqual({ items: [] });
  expect(await api(`/sessions/${sessionId}/goal`)).toBeNull();
  expect(await api(`/sessions/${sessionId}/transcript/plan?agent_id=main`)).toEqual({ agent_id: 'main', plans: [] });
  expect(await api(`/sessions/${sessionId}/file-history/changes?turn_id=1`)).toEqual({ changes: [], recorded: false });
  history.push({ ID: 'version-1', SessionID: sessionId, Path: '/workspace/example.txt', Version: 1, Content: 'A recorded edit.' });
  const recordedHistory = await web.app.inject({ method: 'GET', url: `/api/v1/sessions/${sessionId}/file-history/changes?turn_id=1` });
  expect(recordedHistory.statusCode, recordedHistory.body).toBe(200);
  expect(recordedHistory.json().data).toEqual({ changes: [{ path: '/workspace/example.txt', status: 'modified', additions: 1, deletions: 0 }], recorded: true });
  expect(await api(`/sessions/${sessionId}/file-history/content?turn_id=1&path=${encodeURIComponent('/workspace/example.txt')}&phase=end`)).toEqual({ content: { version: 1, content: 'A recorded edit.' } });
  history.length = 0;
  expect(await api(`/sessions/${sessionId}/prompts`)).toEqual({ active: null, queued: [] });
  const page = transcriptResponseSchema.parse(await api(`/sessions/${sessionId}/transcript?agent_id=main`));
  const frames = page.items.flatMap((item) => item.kind === 'turn' ? item.steps.flatMap((step) => step.frames) : []);
  expect(page.attachments).toContainEqual(expect.objectContaining({ attachmentId: 'user-1-1', mediaType: 'text/plain', name: 'evidence.txt', source: { kind: 'url', url: `data:text/plain;base64,${attachmentBytes}` } }));
  expect(page.items).toContainEqual(expect.objectContaining({ kind: 'turn', prompt: 'Check the gateway', attachmentIds: ['user-1-1'] }));
  expect(frames.filter((frame) => frame.kind === 'text' && frame.role === 'user')).toEqual([]);
  expect((await api(`/sessions/${sessionId}/transcript/user-messages`)).agents[0].messages[0]).toMatchObject({ prompt: 'Check the gateway', attachment_ids: ['user-1-1'] });
  expect(frames).toEqual(expect.arrayContaining([
    expect.objectContaining({ kind: 'thinking', text: 'Use the health tool.' }),
    expect.objectContaining({ kind: 'tool', toolCallId: 'call-1', input: { scope: 'room' }, output: 'gateway healthy', state: 'done' }),
    expect.objectContaining({ kind: 'text', role: 'assistant', text: 'The gateway is healthy.' }),
  ]));
  const pending = page.interactions.find((item) => item.interactionId === 'permission-1');
  expect(pending).toMatchObject({ interactionKind: 'approval', state: 'pending' });
  expect((pending!.request as any).display).toMatchObject({ kind: 'command', command: 'pwd' });
  const ws = new WebSocket(`ws://127.0.0.1:${web.port}/api/v1/ws`);
  const received: any[] = [];
  ws.on('message', (data) => received.push(JSON.parse(String(data))));
  cleanups.push(async () => { ws.terminate(); });
  await once(ws, 'open');
  ws.send(JSON.stringify({ type: 'client_hello', id: 'hello', payload: { client_id: 'example-browser', subscriptions: [sessionId] } }));
  ws.send(JSON.stringify({ type: 'subscribe_v2', id: 'transcript', payload: { session_id: sessionId, transcript: { main: 'delta' } } }));
  await expect.poll(() => received.find((frame) => frame.type === 'transcript.reset')).toBeDefined();
  await expect.poll(attachedClients).toBe(1);
  const second = new WebSocket(`ws://127.0.0.1:${web.port}/api/v1/ws`);
  const secondReceived: any[] = [];
  second.on('message', (data) => secondReceived.push(JSON.parse(String(data))));
  cleanups.push(async () => { second.terminate(); });
  await once(second, 'open');
  second.send(JSON.stringify({ type: 'client_hello', id: 'second-hello', payload: { subscriptions: [sessionId] } }));
  second.send(JSON.stringify({ type: 'subscribe_v2', id: 'second-transcript', payload: { session_id: sessionId, transcript: { main: 'delta' } } }));
  await expect.poll(attachedClients).toBe(2);
  second.send(JSON.stringify({ type: 'unsubscribe_v2', id: 'second-leave', payload: { session_id: sessionId } }));
  await expect.poll(() => secondReceived.find((frame) => frame.type === 'ack' && frame.id === 'second-leave')?.code).toBe(0);
  await expect.poll(attachedClients).toBe(1);
  second.send(JSON.stringify({ type: 'subscribe_v2', id: 'second-return', payload: { session_id: sessionId, transcript: { main: 'delta' } } }));
  await expect.poll(attachedClients).toBe(2);
  second.close();
  await once(second, 'close');
  await expect.poll(attachedClients).toBe(1);
  expect(received.find((frame) => frame.type === 'transcript.reset').payload.snapshot.interactions[0].interactionId).toBe('permission-1');
  await api(`/sessions/${sessionId}/prompts`, { prompt_id: 'handoff-subscribed', content: [{ type: 'text', text: '/tui' }] });
  await expect.poll(() => received.find((frame) => frame.type === 'prompt.completed' && frame.payload.promptId === 'handoff-subscribed')).toBeDefined();
  expect(await api(`/sessions/${sessionId}/prompts`)).toEqual({ active: null, queued: [] });
  expect(await api(`/sessions/${sessionId}/status`)).toMatchObject({ busy: false });
  expect(messages).toHaveLength(4);
  expect(submissions).toEqual([]);
  await api(`/sessions/${sessionId}/approvals/permission-1:respond`, { decision: 'approved' });
  expect((await api(`/sessions/${sessionId}/transcript?agent_id=main`)).pending_interactions).toEqual([]);
  const mcpPermission = { id: 'permission-mcp', session_id: sessionId, tool_call_id: 'call-mcp', tool_name: 'mcp_gateway_health_check', description: 'execute mcp_gateway_health_check with the following parameters:', action: 'execute', params: JSON.stringify({ server: 'example-environment', harness_id: 'example-canonical' }), path: dir };
  permissions = [mcpPermission];
  for (const stream of streams) stream.write(`data: ${JSON.stringify({ type: 'permission', payload: { type: 'created', payload: mcpPermission } })}\n\n`);
  await expect.poll(() => received.findLast((frame) => frame.type === 'transcript.reset')?.payload.snapshot.interactions.find((item: any) => item.interactionId === mcpPermission.id)?.request.input).toEqual({ server: 'example-environment', harness_id: 'example-canonical' });
  const mcpApproval = (await api(`/sessions/${sessionId}/approvals`)).items[0];
  expect(mcpApproval).toMatchObject({ tool_name: mcpPermission.tool_name, action: mcpPermission.description, tool_input_display: { kind: 'generic', summary: mcpPermission.description, detail: { server: 'example-environment', harness_id: 'example-canonical' } } });
  await api(`/sessions/${sessionId}/approvals/${mcpPermission.id}`, { decision: 'approved' });
  await expect.poll(() => received.findLast((frame) => frame.type === 'transcript.reset')?.payload.snapshot.interactions).toEqual([]);
  const [result, retried] = await Promise.all([1, 2].map(() => api(`/sessions/${sessionId}/prompts`, { prompt_id: 'browser-turn-1', permission_mode: 'manual', content: [{ type: 'text', text: 'Continue here.' }] })));
  expect(result.prompt_id).toBe('browser-turn-1');
  expect(result.user_message_id).toBe('go-browser-user-1');
  expect(retried).toEqual(result);
  expect(await api(`/sessions/${sessionId}/status`)).toMatchObject({ permission: 'manual' });
  expect(submissions).toEqual([expect.objectContaining({ session_id: sessionId, run_id: 'browser-turn-1', prompt: 'Continue here.' })]);
  const running = transcriptResponseSchema.parse(await api(`/sessions/${sessionId}/transcript?agent_id=main`));
  expect(running.prompts).toContainEqual(expect.objectContaining({ promptId: 'browser-turn-1', userMessageId: 'go-browser-user-1', status: 'running' }));
  expect(running.items).toContainEqual(expect.objectContaining({ turnId: 'go-browser-user-1', triggerPromptId: 'browser-turn-1' }));
  expect((await api(`/sessions/${sessionId}/prompts`)).active).toMatchObject({ prompt_id: 'browser-turn-1', user_message_id: 'go-browser-user-1' });
  messages.push({ id: 'go-browser-response-1', role: 'assistant', session_id: sessionId, created_at: 1720000003, updated_at: 1720000003, parts: [{ type: 'text', data: { text: 'Continued in the shared session.' } }, { type: 'finish', data: { reason: 'end_turn', time: 1720000003 } }] });
  session.is_busy = false;
  for (const stream of streams) stream.write(`data: ${JSON.stringify({ type: 'run_complete', payload: { type: 'updated', payload: { session_id: sessionId, run_id: 'browser-turn-1', message_id: 'go-browser-response-1', text: 'Continued in the shared session.' } } })}\n\n`);
  await expect.poll(() => received.find((frame) => frame.type === 'prompt.completed' && frame.payload.promptId === 'browser-turn-1')).toBeDefined();
  await expect.poll(() => received.findLast((frame) => frame.type === 'event.session.status_changed')?.payload.status).toBe('idle');
  await expect.poll(() => received.findLast((frame) => frame.type === 'transcript.reset')?.payload.snapshot.prompts.find((item: any) => item.promptId === 'browser-turn-1')?.status).toBe('completed');
  expect(await api(`/sessions/${sessionId}/prompts`)).toEqual({ active: null, queued: [] });
  await api(`/sessions/${sessionId}/skills/gateway-check:activate`, { args: 'inference' });
  expect(submissions.at(-1)!['prompt']).toContain(skillInstruction);
  expect(submissions.at(-1)!['prompt']).toContain('Skill arguments: inference');
  expect(submissions.at(-1)!['prompt']).not.toContain(Buffer.from(skillInstruction).toString('base64'));
  received.length = 0;
  questions = [{ id: 'question-1', session_id: sessionId, tool_call_id: 'call-3', questions: [{ id: 'q1', type: 'multi_choice', question: 'Which tools?', choices: [{ id: 'one', label: 'One' }, { id: 'two', label: 'Two' }] }] }];
  for (const stream of streams) stream.end();
  await expect.poll(() => received.find((frame) => frame.type === 'transcript.reset' && frame.payload.snapshot.interactions.some((item: any) => item.interactionId === 'question-1')), { timeout: 3000 }).toBeDefined();
  const recovered = received.find((frame) => frame.type === 'transcript.reset' && frame.payload.snapshot.interactions.some((item: any) => item.interactionId === 'question-1'));
  expect(recovered.payload.snapshot.interactions[0].request.questions[0].multi_select).toBe(true);
  await expect.poll(attachedClients).toBe(1);
  await api(`/sessions/${sessionId}/questions/question-1`, { answers: { q1: { kind: 'multi', option_ids: ['one', 'two'] } } });
  expect((await api(`/sessions/${sessionId}/fs:read`, { path: 'canonical.txt' })).content).toBe('The shared Go workspace.\n');
  expect((await api(`/sessions/${sessionId}/fs:list`, { path: '.' })).items).toContainEqual(expect.objectContaining({ name: 'canonical.txt' }));
  const download = await web.app.inject({ method: 'GET', url: `/api/v1/sessions/${sessionId}/fs/canonical.txt:download`, headers: { range: 'bytes=0-2' } });
  expect(download.statusCode).toBe(206);
  expect(download.body).toBe('The');
  const terminal = await api(`/sessions/${sessionId}/terminals`, { shell: '/bin/sh', cols: 80, rows: 24 });
  expect(terminal).toMatchObject({ session_id: sessionId, status: 'running', cwd: dir });
  ws.send(JSON.stringify({ type: 'terminal_attach', id: 'terminal-attach', payload: { session_id: sessionId, terminal_id: terminal.id } }));
  ws.send(JSON.stringify({ type: 'terminal_input', id: 'terminal-input', payload: { session_id: sessionId, terminal_id: terminal.id, data: 'pwd\n' } }));
  await expect.poll(() => received.filter((frame) => frame.type === 'terminal_output').map((frame) => frame.payload.data).join('')).toContain(dir);
  expect(await api(`/sessions/${sessionId}/terminals/${terminal.id}:close`, {})).toEqual({ closed: true });
  expect(await api(`/sessions/${sessionId}:undo`, { count: 1 })).toMatchObject({ status: { busy: false }, messages: { has_more: false } });
  messages.push({ id: 'plan-result', role: 'tool', session_id: sessionId, created_at: 1720000002, updated_at: 1720000002, parts: [{ type: 'tool_result', data: { tool_call_id: 'plan-call', name: 'exit_plan_mode', content: 'Plan mode deactivated.\nPlan saved to: /workspace/plan.md\n\n## Plan (auto-approved, not user-reviewed):\n# Review the gateway evidence', is_error: false } }] });
  expect(await api(`/sessions/${sessionId}/transcript/plan?agent_id=main`)).toEqual({ agent_id: 'main', plans: [{ tool_call_id: 'plan-call', turn_id: 'go-browser-user-1', source: 'output', plan: '# Review the gateway evidence', path: '/workspace/plan.md' }] });
  messages.push({ id: 'background-result', role: 'tool', session_id: sessionId, created_at: 1720000003, updated_at: 1720000003, parts: [{ type: 'tool_result', data: { tool_call_id: 'background-call', name: 'bash', content: 'Background shell started with ID: job-1\n\nUse job_output tool to view output or job_kill to terminate.', is_error: false } }] });
  const backgroundTasks = await web.app.inject({ method: 'GET', url: `/api/v1/sessions/${sessionId}/tasks` });
  expect(backgroundTasks.statusCode).toBe(200);
  expect(backgroundTasks.json().data).toEqual({ items: [] });
  tasks.push({ id: 'bash:example', session_id: sessionId, kind: 'bash', description: 'Example task', command: 'pwd', status: 'running', created_at: '2026-01-01T00:00:00.000Z', run_in_background: false, output_preview: dir });
  expect(await api(`/sessions/${sessionId}/tasks`)).toMatchObject({ items: [{ id: 'bash:example', status: 'running' }] });
  expect(await api(`/sessions/${sessionId}/tasks/bash%3Aexample:detach`, {})).toMatchObject({ detached: true });
  expect(await api(`/sessions/${sessionId}/tasks/bash%3Aexample:cancel`, {})).toEqual({ cancelled: true });
  expect(await api(`/sessions/${sessionId}/goal`, { objective: 'Finish this example' })).toMatchObject({ objective: 'Finish this example', status: 'active' });
  expect(await api(`/sessions/${sessionId}/goal`, { control: 'pause' })).toMatchObject({ status: 'paused' });
  expect(await api(`/sessions/${sessionId}:update`, { agent_config: { plan_mode: true, swarm_mode: true, system_prompt: 'Use the example fixture.' } })).toMatchObject({ id: sessionId });
  expect(await api(`/sessions/${sessionId}/status`)).toMatchObject({ plan_mode: true, swarm_mode: true });
  const forked = await api(`/sessions/${sessionId}:fork`, { title: 'Saved branch' });
  expect(forked).toMatchObject({ id: 'forked-session', title: 'Saved branch' });
  const branched = await api(`/sessions/${sessionId}/children`, { title: 'New branch' });
  expect(branched).toMatchObject({ id: 'branched-session', title: 'New branch' });
  expect((await api('/sessions/branched-session/transcript?agent_id=main')).items).toEqual([]);
  queued.push({ id: 'queue-1', run_id: 'queued-run', prompt: 'Next step', created_at: '2026-01-01T00:00:00.000Z' });
  expect((await api(`/sessions/${sessionId}/prompts`)).queued).toMatchObject([{ prompt_id: 'queued-run', content: [{ type: 'text', text: 'Next step' }] }]);
  expect(await api(`/sessions/${sessionId}/prompts::steer`, { prompt_ids: ['queued-run'] })).toMatchObject({ steered: true });
  expect((await api(`/sessions/${sessionId}/prompts`)).queued).toEqual([]);
  const mcpList = await web.app.inject({ method: 'GET', url: '/api/v2/mcp/servers' });
  expect(mcpList.statusCode, mcpList.body).toBe(200);
  expect(mcpList.json().data).toContainEqual(expect.objectContaining({ name: 'gateway', config: expect.objectContaining({ transport: 'http' }) }));
  const mcpAdd = await web.app.inject({ method: 'POST', url: '/api/v2/mcp/servers', payload: { name: 'example.server', transport: 'stdio', command: 'example-command', cwd: dir, startupTimeoutMs: 4321, toolTimeoutMs: 5678 } });
  expect(mcpAdd.statusCode, mcpAdd.body).toBe(200);
  expect(config['mcp']['example.server']).toMatchObject({ type: 'stdio', command: 'example-command', cwd: dir, startup_timeout_ms: 4321, tool_timeout_ms: 5678 });
  const browsePrimary = join(dir, 'browse-primary');
  const browseSecondary = join(dir, 'browse-secondary');
  await mkdir(browsePrimary); await mkdir(browseSecondary);
  await writeFile(join(browsePrimary, 'alpha-primary.txt'), 'Primary root');
  await writeFile(join(browseSecondary, 'alpha-secondary.txt'), 'Secondary root');
  const attachmentsBeforeSuggest = workspaceAttachments;
  const suggested = await api('/fs:suggest', { roots: [browsePrimary, browseSecondary], query: 'alpha', follow_gitignore: false });
  expect(suggested.items).toContainEqual(expect.objectContaining({ path: 'alpha-primary.txt' }));
  expect(suggested.items).toContainEqual(expect.objectContaining({ path: join(browseSecondary, 'alpha-secondary.txt') }));
  expect(workspaceAttachments).toBe(attachmentsBeforeSuggest);
  const providerAdded = await api('/providers', { id: 'another-example', type: 'openai', base_url: 'https://example.test/v1', models: [{ model: 'another', max_context_size: 8192 }] });
  expect(providerAdded).toMatchObject({ id: 'another-example', type: 'openai' });
  expect(config['providers'].example.api_key).toBe('fixture-secret-never-exposed');
  expect(config['models'].large).toEqual({ provider: 'example', model: 'example-model' });
  const pluginRoot = join(dir, 'plugin-fixture');
  await mkdir(join(pluginRoot, 'skills', 'fixture-skill'), { recursive: true });
  await writeFile(join(pluginRoot, 'skills', 'fixture-skill', 'SKILL.md'), '---\nname: fixture-skill\ndescription: Fixture skill\n---\nUse the fixture instructions.');
  await writeFile(join(pluginRoot, 'floyd.plugin.json'), JSON.stringify({ name: 'canonical-fixture', skills: './skills/', sessionStart: { skill: 'fixture-skill' }, systemPrompt: 'Canonical plugin instructions.', mcpServers: { example: { transport: 'stdio', command: 'example-mcp', cwd: './', startupTimeoutMs: 1000, toolTimeoutMs: 2000 } } }));
  const zipPath = join(dir, 'fixture.zip');
  execFileSync('/usr/bin/zip', ['-qr', zipPath, '.'], { cwd: pluginRoot });
  const zipBytes = await readFile(zipPath);
  const archiveServer = createServer((req, res) => { res.setHeader('Content-Type', 'application/zip'); res.end(zipBytes); });
  await new Promise<void>((resolve) => archiveServer.listen(0, '127.0.0.1', resolve));
  cleanups.push(() => new Promise<void>((resolve) => archiveServer.close(() => resolve())));
  const plugin = await api('/plugins', { source: `http://127.0.0.1:${(archiveServer.address() as import('node:net').AddressInfo).port}/fixture.zip` });
  expect(plugin).toMatchObject({ id: 'canonical-fixture', enabled: true, skillCount: 1 });
  const contributions = pluginReloads.at(-1)!;
  expect(contributions).toMatchObject({ system_prompts: [{ pluginId: 'canonical-fixture', content: 'Canonical plugin instructions.' }], session_starts: [{ pluginId: 'canonical-fixture', skillName: 'fixture-skill' }] });
  expect(Object.values(contributions['mcp'] as object)).toContainEqual(expect.objectContaining({ cwd: expect.stringContaining('canonical-fixture'), startup_timeout_ms: 1000, tool_timeout_ms: 2000 }));
  const nativePlugin = await readFile(join((contributions['plugin_paths'] as string[])[0]!, 'PLUGIN.md'), 'utf8');
  expect(nativePlugin.startsWith('---\nname: plugin-runtime\ndescription: Instructions from enabled MegaFloyd plugins\n---\n')).toBe(true);
  await api(`/plugins/${plugin.id}:disable`, {});
  expect(pluginReloads.at(-1)).toMatchObject({ skill_paths: [], plugin_paths: [], system_prompts: [], session_starts: [], mcp: {} });
  const boundary = 'canonical-upload-boundary';
  const uploaded = await web.app.inject({ method: 'POST', url: '/api/v1/files', headers: { 'content-type': `multipart/form-data; boundary=${boundary}` }, payload: Buffer.from(`--${boundary}\r\nContent-Disposition: form-data; name="file"; filename="metadata.json"\r\nContent-Type: text/plain\r\n\r\nUpload stays intact.\r\n--${boundary}--\r\n`) });
  expect(uploaded.statusCode, uploaded.body).toBe(200);
  const file = uploaded.json().data;
  expect(file).toMatchObject({ name: 'metadata.json', media_type: 'text/plain', size: 20 });
  const uploadedDownload = await web.app.inject({ method: 'GET', url: `/api/v1/files/${file.id}`, headers: { range: 'bytes=0-5' } });
  expect(uploadedDownload.statusCode).toBe(206);
  expect(uploadedDownload.body).toBe('Upload');
  await api(`/sessions/${sessionId}/prompts`, { prompt_id: 'attachment-turn', permission_mode: 'manual', content: [{ type: 'text', text: 'Read the attached fixture.' }, { type: 'file', file_id: file.id, name: file.name }] });
  expect(submissions.at(-1)!['attachments']).toEqual([expect.objectContaining({ file_name: 'metadata.json', mime_type: 'text/plain', content: Buffer.from('Upload stays intact.').toString('base64') })]);
  const openedExternal = await api('/workspaces', { root: external, name: 'External project' });
  expect(workspaceCreationRequests.at(-1)).not.toHaveProperty('data_dir');
  expect(openedExternal).toMatchObject({ root: externalPath, name: 'External project', session_count: 0 });
  expect(await api(`/runtime?workspace_id=${openedExternal.id}`)).toMatchObject({ cwd: externalPath, data_dir: join(dir, 'private-workspaces', 'external-project'), workspace_id: 'workspace-external' });
  await expect(readFile(join(external, '.megafloyd', 'floyd.db'))).rejects.toMatchObject({ code: 'ENOENT' });
  ws.close();
  await once(ws, 'close');
  await expect.poll(attachedClients).toBe(0);
});

test('canonical company account login uses its configured host and preserves the selected Go provider', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'megafloyd-account-'));
  cleanups.push(() => rm(dir, { recursive: true, force: true }));
  const provider = 'managed:floyd-code';
  const config: Record<string, any> = { providers: { example: { type: 'openai-compat', api_key: 'existing-example-key', models: [{ id: 'example-model', context_window: 4096 }] } }, models: { large: { provider: 'example', model: 'example-model', max_tokens: 4096 } } };
  const originalSelection = structuredClone(config['models']);
  const account = { provider, authenticated: false, status: 'unauthenticated', base_url: '', oauth_host: '', region: 'global' };
  const requests: string[] = [];
  let registryVersion = 1;
  const company = createServer(async (req, res) => {
    const path = new URL(req.url!, 'http://fixture').pathname;
    let data = '';
    for await (const chunk of req) data += chunk;
    requests.push(path);
    res.setHeader('Content-Type', 'application/json');
    if (path === '/api/oauth/device_authorization') {
      expect(new URLSearchParams(data).get('client_id')).toBe('17e5f671-d194-4dfb-9706-5516cb48c098');
      return res.end(JSON.stringify({ device_code: 'fixture-device', user_code: 'EXAMPLE', verification_uri: `${account.oauth_host}/activate`, verification_uri_complete: `${account.oauth_host}/activate?code=EXAMPLE`, expires_in: 30, interval: 1 }));
    }
    if (path === '/api/oauth/token') {
      expect(new URLSearchParams(data).get('device_code')).toBe('fixture-device');
      return res.end(JSON.stringify({ access_token: 'fixture-account-access', refresh_token: 'fixture-account-refresh', expires_in: 3600, token_type: 'Bearer' }));
    }
    if (path === '/catalog/api.json') return res.end(JSON.stringify({ example: { id: 'example', name: 'Company catalog fixture', api: `${account.base_url}/v1`, npm: '@ai-sdk/openai-compatible', models: { example: { id: 'example', name: 'Example', limit: { context: 8192, output: 1024 } } } } }));
    if (path === '/registry.json') {
      expect(req.headers.authorization).toBe('Bearer fixture-registry');
      return res.end(JSON.stringify({ 'registry-example': { id: 'registry-example', name: 'Registry fixture', api: `${account.base_url}/v1`, type: 'openai', models: { first: { id: 'first', limit: { context: 4096, output: 1024 } }, ...(registryVersion > 1 ? { second: { id: 'second', limit: { context: 8192, output: 2048 } } } : {}) } } }));
    }
    expect(req.headers.authorization).toBe('Bearer fixture-account-access');
    if (path === '/models') return res.end(JSON.stringify({ data: [{ id: 'company-example', display_name: 'Company Example', context_length: 16384, supports_tool_use: true, base_url: `${account.base_url}/v1` }] }));
    if (path === '/me') return res.end(JSON.stringify({ user_id: 'example-member', nickname: 'Example Member' }));
    if (path === '/usages') return res.end(JSON.stringify({ usages: { limit_5h: { used_ratio: 0.2 } } }));
    res.writeHead(404); res.end('{}');
  });
  await new Promise<void>((resolve) => company.listen(0, '127.0.0.1', resolve));
  cleanups.push(() => new Promise<void>((resolve) => company.close(() => resolve())));
  account.base_url = account.oauth_host = `http://127.0.0.1:${(company.address() as import('node:net').AddressInfo).port}`;
  config['options'] = { company_platform_url: account.base_url };
  const streams = new Set<ServerResponse>();
  const mutations: string[] = [];
  let accountToken: Record<string, unknown> | null = null;
  const socketPath = join(dir, 'account.sock');
  const go = createServer(async (req, res) => {
    const path = new URL(req.url!, 'http://go').pathname;
    let data = '';
    for await (const chunk of req) data += chunk;
    const body = data ? JSON.parse(data) : {};
    const send = (value: unknown) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(value)); };
    if (path.endsWith('/events')) { res.writeHead(200, { 'Content-Type': 'text/event-stream' }); res.flushHeaders(); streams.add(res); res.on('close', () => streams.delete(res)); return; }
    if (path === '/v1/health') return send({ status: 'ok' });
    if (path === '/v1/handoff/web' && req.method === 'POST') return send({ registered: true });
    if (path === '/v1/workspaces') return send(req.method === 'POST' ? { id: 'account-workspace', path: dir, data_dir: join(dir, 'data') } : [{ id: 'account-workspace', path: dir, data_dir: join(dir, 'data') }]);
    if (path.endsWith('/agent')) return send({ is_ready: true, is_busy: false, model_cfg: config['models'].large, model: { id: 'example-model', name: 'Example Model', context_window: 4096 } });
    if (path.endsWith('/interactions')) return send({ permissions: [], questions: [] });
    if (path.endsWith('/config/plugin-reload')) return send({});
    if (path.endsWith('/config')) return send(config);
    if (path.endsWith('/config/set')) { expect(body.key.startsWith('providers.')).toBe(true); config['providers'][body.key.slice(10)] = body.value; return send({}); }
    if (path.endsWith('/agent/update')) return send({});
    if (path.endsWith('/providers/refresh')) { expect(body.provider_id).toBe('oauth-example'); return send({ changed: [{ providerId: 'oauth-example', providerName: 'Other native OAuth', added: 1, removed: 0 }], unchanged: [], failed: [] }); }
    if (path.endsWith('/auth/account/token')) return send(accountToken);
    if (path.endsWith('/auth/account')) {
      if (req.method === 'POST') {
        mutations.push('account-login');
        expect(body).toMatchObject({ provider, base_url: account.base_url, oauth_host: account.oauth_host, token: { access_token: 'fixture-account-access', refresh_token: 'fixture-account-refresh' } });
        accountToken = body.token;
        config['providers'][provider] = { type: 'openai-compat', base_url: account.base_url, api_key: 'company-auth://fixture-account', models: [] };
        account.authenticated = true; account.status = 'authenticated';
      }
      if (req.method === 'DELETE') { mutations.push('account-logout'); delete config['providers'][provider]; accountToken = null; account.authenticated = false; account.status = 'unauthenticated'; }
      return send(account);
    }
    if (path.includes('/clients/')) { res.end(); return; }
    res.writeHead(404); send({ message: `unexpected ${req.method} ${path}` });
  });
  await new Promise<void>((resolve) => go.listen(socketPath, resolve));
  cleanups.push(async () => { for (const stream of streams) stream.end(); await new Promise<void>((resolve) => go.close(() => resolve())); });
  const web = await startGoServer({ host: '127.0.0.1', port: 0, homeDir: dir, disableAuth: true, hostIdentity: { name: 'example', version: 'test' } as never, env: { MEGAFLOYD_GO_HOST: `unix://${socketPath}`, MEGAFLOYD_GO_CWD: dir, MEGAFLOYD_GO_DATA_DIR: join(dir, 'data') } });
  cleanups.push(() => web.close());
  const api = async (path: string, payload?: unknown) => {
    const response = await web.app.inject({ method: payload === undefined ? 'GET' : 'POST', url: `/api/v1${path}`, payload: payload === undefined ? undefined : JSON.stringify(payload), headers: payload === undefined ? {} : { 'content-type': 'application/json' } });
    expect(response.statusCode, response.body).toBe(200);
    expect(response.body).not.toContain('fixture-account-access');
    expect(response.body).not.toContain('fixture-account-refresh');
    return response.json().data;
  };
  expect(await api('/auth')).toMatchObject({ managed_provider: { name: provider, status: 'unauthenticated' }, models_ready: true });
  expect(requests).toEqual([]);
  expect((await api('/catalog/providers')).items).toContainEqual(expect.objectContaining({ id: 'example', name: 'Company catalog fixture' }));
  const start = await api('/oauth/login', { region: 'global' });
  expect(start).toMatchObject({ provider, status: 'pending', user_code: 'EXAMPLE', verification_uri: `${account.oauth_host}/activate` });
  await expect.poll(async () => (await api('/oauth/login')).status, { timeout: 3000 }).toBe('authenticated');
  expect(await api('/auth')).toMatchObject({ managed_provider: { name: provider, status: 'authenticated' } });
  expect(await api('/oauth/userinfo')).toMatchObject({ kind: 'ok', userInfo: { userId: 'example-member' } });
  expect(await api('/oauth/usage')).toMatchObject({ kind: 'ok', quota: { usages: { limit5h: { usedRatio: 0.2 } } } });
  expect(config['models']).toEqual(originalSelection);
  expect(config['providers'][provider]).toMatchObject({ base_url: `${account.base_url}/v1`, models: [{ id: 'company-example', context_window: 16384 }] });
  expect(config['providers'].example.api_key).toBe('existing-example-key');
  const originalAccountToken = JSON.stringify(accountToken);
  const originalProvider = structuredClone(config['providers'].example);
  delete config['providers'][provider];
  expect(await api('/oauth/login', { region: 'global' })).toMatchObject({ provider, status: 'authenticated' });
  expect(config['providers'][provider]).toMatchObject({ id: provider, name: 'Company account', type: 'openai-compat', api_key: 'company-auth://default', disable: false, discover_models: false, base_url: `${account.base_url}/v1`, models: [{ id: 'company-example', context_window: 16384 }] });
  expect(config['providers'][provider]['oauth']).toBeUndefined();
  expect(JSON.stringify(accountToken)).toBe(originalAccountToken);
  expect(config['providers'].example).toEqual(originalProvider);
  expect(config['models']).toEqual(originalSelection);
  delete config['providers'][provider];
  expect(await api(`/providers/${provider}:refresh`, {})).toMatchObject({ changed: [expect.objectContaining({ provider_id: provider, added: 1 })], failed: [] });
  expect(config['providers'][provider].api_key).toBe('company-auth://default');
  expect(JSON.stringify(accountToken)).toBe(originalAccountToken);
  delete config['providers'][provider];
  config['providers']['oauth-example'] = { type: 'openai-compat', oauth: { access_token: 'native-fixture-only' }, models: [] };
  const refreshed = await api('/providers:refresh_oauth', {});
  expect(refreshed).toMatchObject({ changed: [{ provider_id: 'oauth-example', provider_name: 'Other native OAuth', added: 1, removed: 0 }, expect.objectContaining({ provider_id: provider })], unchanged: [], failed: [] });
  expect(JSON.stringify(accountToken)).toBe(originalAccountToken);
  expect(config['providers'].example).toEqual(originalProvider);
  expect((await api('/providers:import_registry', { url: `${account.base_url}/registry.json`, api_key: 'fixture-registry' })).models_imported).toBe(1);
  registryVersion = 2;
  expect(await api('/providers/registry-example:refresh', {})).toEqual({ changed: [{ provider_id: 'registry-example', provider_name: 'Registry fixture', added: 1, removed: 0 }], unchanged: [], failed: [] });
  expect(config['providers']['registry-example'].source).toEqual({ kind: 'apiJson', url: `${account.base_url}/registry.json`, apiKey: 'fixture-registry' });
  expect(config['providers']['registry-example'].api_key).toBe('fixture-registry');
  expect(config['models']).toEqual(originalSelection);
  expect(await api('/oauth/logout', {})).toEqual({ logged_out: true, provider });
  expect(config['models']).toEqual(originalSelection);
  expect(config['providers'].example.api_key).toBe('existing-example-key');
  expect(mutations).toEqual(['account-login', 'account-logout']);
  expect(requests).toEqual(['/catalog/api.json', '/api/oauth/device_authorization', '/api/oauth/token', '/models', '/me', '/usages', '/models', '/models', '/models', '/registry.json', '/registry.json']);
});
