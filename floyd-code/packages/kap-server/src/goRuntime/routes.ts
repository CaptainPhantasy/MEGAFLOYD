import { basename, resolve } from 'node:path';
import { homedir } from 'node:os';
import { createReadStream } from 'node:fs';
import type { FastifyInstance, FastifyRequest } from 'fastify';
import { paginateTurns } from '@legacy-ai/transcript';
import { ZodError } from 'zod';
import { isError2, ErrorCodes } from '@legacy-ai/agent-core-v2/errors';
import { GoRequestError, type GoSession } from './client';
import { GoRuntime, UnsupportedError } from './runtime';
import { approval, array, iso, modelKey, object, projectInteractions, projectPlans, projectSession, question, text, thinkingLevel, workspaceKey } from './projection';
import type { LocalViews } from './localViews';
import { parseRangeHeader } from '../lib/httpRange';
import { listGoSessionsV2 } from './sessionList';
import { batchSessions, sessionControl } from './sessionControls';
import type { GoWorkspaces } from './workspaces';
import type { GoProviders } from './providers';
import type { GoPlugins } from './plugins';
import type { GoMcp } from './mcp';
import type { GoOAuth } from './oauth';
import { browseHostFolders } from './hostFolders';
import { handoffCommands, isHandoffTarget, requireHandoffTarget } from './handoff';

interface GoRouteOptions { version: string; startedAt: string; title?: string; shutdown: () => void; workspaces: GoWorkspaces; providers: GoProviders; plugins: GoPlugins; mcp: GoMcp; oauth: GoOAuth }

export const envelope = (data: unknown, requestId = '') => ({ code: 0, msg: 'success', data, request_id: requestId });

export function registerGoRoutes(app: FastifyInstance, runtime: GoRuntime, views: LocalViews, options: GoRouteOptions): void {
  app.all('/api/v1/*', async (req, reply) => {
    try {
      const url = new URL(req.url, 'http://web');
      const selected = await selectRuntime(runtime, views, req, options);
      const download = /^\/api\/v1\/sessions\/([^/]+)\/fs\/(.+):download$/.exec(decodeURIComponent(url.pathname));
      if (req.method === 'GET' && download) {
        await selected.runtime.session(download[1]!);
        selected.views.assertLocal({ runtime_id: url.searchParams.get('runtime_id') });
        await selected.views.refreshDirectories();
        const file = await selected.views.fs.resolveDownload(download[2]!);
        reply.type(file.mime).header('ETag', file.etag).header('Last-Modified', file.modifiedAt.toUTCString()).header('Accept-Ranges', 'bytes');
        if (req.headers['if-none-match'] === file.etag) return reply.code(304).send();
        const range = parseRangeHeader(req.headers.range, file.size);
        if (req.headers.range && !range) return reply.code(416).header('Content-Range', `bytes */${file.size}`).send();
        if (range) return reply.code(206).header('Content-Length', range.length).header('Content-Range', `bytes ${range.start}-${range.end}/${file.size}`).send(createReadStream(file.absolute, { start: range.start, end: range.end }));
        reply.header('Content-Length', file.size);
        return reply.send(createReadStream(file.absolute));
      }
      if (req.method === 'GET' && url.pathname === '/api/v1/company/account') {
        const config = await selected.runtime.client.json<Record<string, unknown>>('GET', `${selected.runtime.base}/config`);
        const company = text(object(config['options'])['company_platform_url']).replace(/\/+$/, '');
        if (!company) throw new GoRequestError(409, 'Configure the MegaFloyd company platform URL before opening your account');
        const account = new URL(`${company}/account`);
        if (!['http:', 'https:'].includes(account.protocol) || account.username || account.password) throw new GoRequestError(409, 'The MegaFloyd company platform URL is invalid');
        return reply.redirect(account.href, 302);
      }
      const path = decodeURIComponent(url.pathname.slice('/api/v1'.length));
      const managed = await options.oauth.route(selected.runtime, path, req.method, object(req.body), url.searchParams) ?? await options.providers.route(selected.runtime, path, req.method, object(req.body)) ?? await options.plugins.route(selected.runtime, path, req.method, object(req.body));
      if (managed) return 'status' in managed && managed.status === 204 ? reply.code(204).send() : reply.send(envelope(managed.data, req.id));
      const data = await dispatch(selected.runtime, selected.views, req, options);
      return reply.send(envelope(data, req.id));
    } catch (error) {
      const status = error instanceof UnsupportedError ? 501 : error instanceof GoRequestError ? error.status : error instanceof ZodError ? 400 : isError2(error) && error.code === ErrorCodes.FS_PATH_NOT_FOUND ? 404 : isError2(error) && error.code === ErrorCodes.FS_PATH_ESCAPES ? 400 : 500;
      return reply.code(status).send({ code: status === 404 ? 40401 : status === 409 ? 40903 : status * 100, msg: error instanceof Error ? error.message : String(error), data: null, request_id: req.id });
    }
  });
  app.all('/api/v2/*', async (req, reply) => {
    try {
      const url = new URL(req.url, 'http://web');
      const selected = await selectRuntime(runtime, views, req, options);
      const managed = await options.mcp.route(selected.runtime, decodeURIComponent(url.pathname.slice('/api/v2'.length)), req.method, object(req.body), url.searchParams);
      if (managed) return reply.send(envelope(managed.data, req.id));
      if (req.method === 'GET' && url.pathname === '/api/v2/sessions') return reply.send(envelope(await listGoSessionsV2(runtime, views, url.searchParams, await options.workspaces.all()), req.id));
      const batch = /^\/api\/v2\/sessions::(archive|restore|delete)$/.exec(url.pathname);
      if (req.method === 'POST' && batch) return reply.send(envelope(await batchSessions(async (id) => (await options.workspaces.session(id)).runtime, batch[1]!, object(req.body)), req.id));
      throw new GoRequestError(404, `No route for ${req.method} ${url.pathname}`);
    } catch (error) {
      const status = error instanceof UnsupportedError ? 501 : error instanceof GoRequestError ? error.status : 500;
      return reply.code(status).send({ code: status * 100, msg: error instanceof Error ? error.message : String(error), data: null, request_id: req.id });
    }
  });
}

async function dispatch(runtime: GoRuntime, views: LocalViews, req: FastifyRequest, options: GoRouteOptions): Promise<unknown> {
  const url = new URL(req.url, 'http://web');
  const path = decodeURIComponent(url.pathname.slice('/api/v1'.length));
  const query = url.searchParams;
  const body = object(req.body);
  const get = req.method === 'GET';
  const post = req.method === 'POST';
  if (get && path === '/healthz') { await runtime.client.json('GET', '/v1/health'); return { ok: true, engine: 'go', uptime_sec: Math.floor((Date.now() - Date.parse(options.startedAt)) / 1000) }; }
  if (get && path === '/runtime') return { engine: 'go', host: runtime.host, cwd: runtime.workspace.path, data_dir: runtime.workspace.data_dir || runtime.dataDir, workspace_id: runtime.workspace.id };
  if (path === '/handoff' && (get || post)) return runtime.handoff('', post ? requireHandoffTarget(body['target']) : undefined);
  if (get && path === '/fs:browse') return browseHostFolders(query.get('path') ?? undefined);
  if (get && path === '/fs:home') return { home: homedir(), recent_roots: await options.workspaces.recentRoots() };
  if (get && path === '/meta') return { server_version: options.version, server_id: runtime.epoch, started_at: options.startedAt, capabilities: { websocket: true, file_upload: true, fs_query: true, mcp: true, tasks: true, terminal: true }, open_in_apps: process.platform === 'darwin' ? ['finder', 'cursor', 'vscode', 'iterm', 'terminal'] : [], dangerous_bypass_auth: true, backend: 'v1', web_title: options.title || 'MegaFloyd', experimental_flags: {}, features: [{ name: 'Shared Go runtime', state: 'Active', meta: { cwd: runtime.workspace.path, data_dir: runtime.dataDir } }] };
  if (path === '/config') {
    if (!get) {
      const allowed = ['default_model', 'thinking', 'yolo', 'default_permission_mode'];
      for (const key of Object.keys(body)) if (!allowed.includes(key)) throw new UnsupportedError(`Changing ${key} from web settings`);
      await runtime.configure({ model: body['default_model'], thinking: body['thinking'], permission_mode: body['default_permission_mode'] ?? (typeof body['yolo'] === 'boolean' ? body['yolo'] ? 'auto' : 'manual' : undefined) });
    }
    const { providers, models, info } = await runtime.catalog();
    const permission = await runtime.permission();
    return { providers: Object.fromEntries(providers.map((item) => [item.id, item])), models: Object.fromEntries(models.map((item) => [item.model, { provider: item.provider, model: item.model.slice(item.provider.length + 1), displayName: item.display_name, maxContextSize: item.max_context_size, maxOutputSize: item.max_output_size, capabilities: item.capabilities, supportEfforts: item.support_efforts }])), default_provider: info.model_cfg.provider, default_model: modelKey(info), thinking: thinkingLevel(info), yolo: permission === 'auto', default_permission_mode: permission, experimental: {} };
  }
  if (get && path === '/models') return { items: (await runtime.catalog()).models };
  if (get && path === '/providers') return { items: (await runtime.catalog()).providers };
  if (get && path.startsWith('/providers/')) { const provider = (await runtime.catalog()).providers.find((item) => item.id === path.slice(11)); if (!provider) throw new GoRequestError(404, 'Provider is not configured in the Go runtime'); return provider; }
  if (post && path.startsWith('/models/') && path.endsWith(':set_default')) { const model = path.slice(8, -12); await runtime.configure({ model }); return { default_model: model, model: (await runtime.catalog()).models.find((item) => item.model === model) }; }
  if (get && path === '/workspaces') { const items = []; for (const entry of await options.workspaces.all()) items.push(await options.workspaces.wire(entry)); return { items }; }
  if (/^\/workspaces\/[^/]+$/.test(path)) {
    const id = path.slice(12);
    if (get) return options.workspaces.wire(await options.workspaces.workspace(id));
    if (req.method === 'PATCH') return options.workspaces.register((await options.workspaces.workspace(id)).runtime.cwd, text(body['name']));
    if (req.method === 'DELETE') return options.workspaces.forget(id);
  }
  if (post && /^\/workspaces\/[^/]+\/add-dir$/.test(path)) { const result = await views.addDirectory({ path: text(body['path']), persist: typeof body['persist'] === 'boolean' ? body['persist'] : undefined }); return { project_root: result.projectRoot, config_path: result.configPath, additional_dirs: result.additionalDirs, persisted: result.persisted }; }
  if (post && path === '/fs:suggest') return views.suggestRoots(array(body['roots']), body);
  if (post && ['/workspace/fs:search', '/workspace/fs:suggest'].includes(path)) return views.fsAction(path.endsWith('search') ? 'search' : 'suggest', body);
  if (post && path === '/workspaces') return options.workspaces.register(text(body['root']), typeof body['name'] === 'string' ? body['name'] : undefined);
  if (get && path === '/sessions') {
    const [sessions, info, pending, permission] = await Promise.all([runtime.sessions(), runtime.info(), runtime.interactions(), runtime.permission()]);
    const selected = sessions.filter((item) => !item.parent_session_id && (query.get('archived_only') === 'true' ? item.archived : query.get('include_archive') === 'true' || !item.archived) && (!query.has('busy') || item.is_busy === (query.get('busy') === 'true')) && (!query.has('exclude_empty') || query.get('exclude_empty') !== 'true' || item.message_count > 0));
    if (query.get('workspace_id') && query.get('workspace_id') !== workspaceKey(runtime.workspace.path)) throw new GoRequestError(404, 'Workspace is not attached to this Go runtime');
    const page = pageItems(selected.sort((a, b) => b.updated_at - a.updated_at || b.id.localeCompare(a.id)), query);
    return { items: page.items.map((item) => projectSession(item, runtime.workspace, info, projectInteractions(pending, item.id, runtime.timestamps), runtime.sequence(item.id), permission)), has_more: page.hasMore };
  }
  if (post && path === '/sessions') {
    const cwd = text(object(body['metadata'])['cwd']);
    if (cwd && resolve(cwd) !== resolve(runtime.workspace.path) || body['workspace_id'] && body['workspace_id'] !== workspaceKey(runtime.workspace.path)) throw new UnsupportedError('Creating a session in another workspace');
    const created = await runtime.client.json<GoSession>('POST', `${runtime.base}/sessions`, { title: text(body['title']) || 'MegaFloyd session' });
    if (body['agent_config']) await runtime.configure(object(body['agent_config']), created.id);
    if (Object.keys(object(body['metadata'])).length) await runtime.client.json('POST', `${runtime.base}/sessions/${encodeURIComponent(created.id)}/metadata`, { metadata: body['metadata'] });
    runtime.markDirty(created.id);
    return (await runtime.view(created.id)).wire;
  }
  if (get && (path === '/skills' || /^\/workspaces\/[^/]+\/skills$/.test(path))) return skills(runtime);
  if (get && path === '/mcp/servers') {
    const [states, cfg] = await Promise.all([runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/mcp/states`), runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`)]);
    const entries: Record<string, unknown>[] = Array.isArray(states) ? states.map(object) : Object.entries(states).map(([name, raw]) => ({ ...object(raw), name: text(object(raw)['name']) || name }));
    return { servers: entries.map((item) => { const config = object(object(cfg['mcp'])[text(item['name'])]); return { id: item['name'], name: item['name'], status: item['state'] === 'connected' || item['state'] === 2 ? 'connected' : item['state'] === 'starting' || item['state'] === 1 ? 'connecting' : item['state'] === 'error' || item['state'] === 3 || item['state'] === 'needs auth' ? 'error' : 'disconnected', transport: text(config['type']) === 'http' ? 'http' : text(config['type']) === 'sse' ? 'sse' : 'stdio', tool_count: Number(item['tool_count']) || 0, last_error: text(item['error']) || undefined }; }) };
  }
  if (get && path === '/agents/profiles') return runtime.client.json('GET', `${runtime.base}/agent/profiles`);
  if (req.method === 'PUT' && path.startsWith('/agents/profiles/')) return runtime.client.json('PUT', `${runtime.base}/agent/profiles/${encodeURIComponent(path.slice(17))}`, body);
  if (get && path === '/tools') return { tools: await runtime.client.json('GET', `${runtime.base}/tools`) };
  if (post && path === '/shutdown') { options.shutdown(); return { shutting_down: true }; }
  const match = /^\/sessions\/([^/:]+)(.*)$/.exec(path);
  if (!match) throw new GoRequestError(404, `No route for ${req.method} ${path}`);
  const id = match[1]!;
  const action = match[2]!;
  const sessionPath = `${runtime.base}/sessions/${encodeURIComponent(id)}`;
  if (action === '/handoff' && (get || post)) return runtime.handoff(id, post ? requireHandoffTarget(body['target']) : undefined);
  const controlled = await sessionControl(runtime, id, action, req.method, body, query);
  if (controlled) return controlled.data;
  if (post && action.startsWith('/fs:')) { await runtime.session(id); return views.fsAction(action.slice(4), body); }
  if (action === '/terminals') {
    if (get) return { items: await (await views.terminal(id)).list() };
    if (post) return views.createTerminal(id, body);
  }
  if (action.startsWith('/terminals/')) {
    const terminalId = action.slice(11).replace(/:close$/, '');
    const service = await views.terminal(id);
    if (get) return service.get(terminalId);
    if (post && action.endsWith(':close')) return service.close(terminalId);
  }
  if (get && action === '') return (await runtime.view(id)).wire;
  if ((post || req.method === 'PATCH') && (action === '' || action === ':update' || action === '/profile')) {
    const metadata = object(body['metadata']);
    if (metadata['cwd'] && resolve(text(metadata['cwd'])) !== resolve(runtime.cwd)) throw new GoRequestError(400, 'Create or fork the session in the target workspace instead of changing its storage root');
    if (body['agent_config']) await runtime.configure(object(body['agent_config']), id);
    if (body['title'] !== undefined || body['metadata'] !== undefined) await runtime.client.json('POST', `${sessionPath}/metadata`, { title: body['title'], metadata: body['metadata'] });
    runtime.markDirty(id);
    return (await runtime.view(id)).wire;
  }
  if (get && action === '/status') return runtime.status(id);
  if (get && action === '/runtime') return { workspace_id: workspaceKey(runtime.workspace.path), runtime_id: 'local', engine: 'go' };
  if (get && action === '/warnings') { await runtime.session(id); return { warnings: [] }; }
  if (get && action === '/transcript') {
    const agentId = query.get('agent_id') || 'main';
    const view = await runtime.view(await runtime.agentSession(id, agentId));
    const page = paginateTurns(view.snapshot.items, { pageSize: Math.min(100, Math.max(1, Number(query.get('page_size')) || 20)), beforeTurn: query.get('before_turn') ?? undefined, afterTurn: query.get('after_turn') ?? undefined });
    return { ...view.snapshot, items: page.items, has_more: page.hasMore, agent_id: agentId, agents: await runtime.agents(id), pending_interactions: view.snapshot.interactions.filter((item) => item.state === 'pending').map((item) => item.interactionId), seq: view.seq };
  }
  if (get && action === '/transcript/ops') { await runtime.session(id); return { agent_id: query.get('agent_id') || 'main', batches: [], latest_seq: runtime.sequence(id), complete: false }; }
  if (get && action === '/transcript/plan') {
    const agentId = query.get('agent_id') || 'main';
    const toolCallId = query.get('tool_call_id') || undefined;
    const plans = projectPlans((await runtime.view(await runtime.agentSession(id, agentId))).snapshot, toolCallId);
    if (toolCallId && !plans.length) throw new GoRequestError(404, 'No exit_plan_mode result found for this tool call');
    return { agent_id: agentId, plans };
  }
  if (get && action === '/transcript/user-messages') {
    const roster = query.get('agent_id') ? [{ agentId: query.get('agent_id')! }] : await runtime.agents(id);
    const agents = [];
    for (const agent of roster) {
      const view = await runtime.view(await runtime.agentSession(id, agent.agentId));
      agents.push({ agent_id: agent.agentId, messages: view.snapshot.items.filter((item) => item.kind === 'turn' && item.origin.kind === 'user').map((item) => { if (item.kind !== 'turn') throw new Error('Invalid transcript turn'); return { turn_id: item.turnId, ordinal: item.ordinal, state: item.state, origin: item.origin, prompt: item.prompt ?? '', attachment_ids: item.attachmentIds, started_at: item.startedAt }; }), attachments: view.snapshot.attachments });
    }
    return { agents };
  }
  if (get && action === '/messages') { const page = pageItems((await runtime.messages(id)).slice().reverse(), query); return { items: page.items.map((message) => ({ id: message.id, session_id: id, role: message.role, content: message.parts.map((part) => ({ type: part.type, ...part.data })), created_at: iso(message.created_at), model: message.model })), has_more: page.hasMore }; }
  if (post && action === '/prompts') return runtime.submit(id, body);
  if (post && action === '/suggestion') return runtime.client.json('POST', `${runtime.base}/agent/sessions/${encodeURIComponent(id)}/suggestion`, { prefix: text(body['prefix']) });
  if (get && action === '/prompts') return runtime.promptState(id);
  if (post && action === ':abort') { await runtime.client.json('POST', `${runtime.base}/agent/sessions/${encodeURIComponent(id)}/cancel`, {}); runtime.markDirty(id); return { aborted: true }; }
  if (post && action === ':compact') { await runtime.client.json('POST', `${runtime.base}/agent/sessions/${encodeURIComponent(id)}/summarize`, {}); runtime.markDirty(id); return { compacted: true }; }
  if ((post && action === ':delete') || req.method === 'DELETE' && action === '') { await runtime.client.json('DELETE', sessionPath); views.closeSession(id); runtime.markDirty(id); return { deleted: true }; }
  if (get && action === '/skills') return skills(runtime);
  if (post && action.startsWith('/skills/') && action.endsWith(':activate')) {
    const name = action.slice(8, -9);
    if (isHandoffTarget(name)) {
      const args = text(body['args']).trim();
      const composer = object(object(body['metadata'])['floyd_code_composer']);
      if (args && args !== `[${name}](floyd-code://skill/${name})` || array(body['attachments']).length || array(composer['attachments']).length || array(composer['attachmentOrder']).length) throw new GoRequestError(400, `Use /${name} by itself without arguments or attachments`);
      return runtime.activateHandoff(id, name, typeof body['args'] === 'string' ? body['args'] : undefined);
    }
    const prompt = await runtime.skillPrompt(name, text(body['args']));
    await runtime.submit(id, { content: [{ type: 'text', text: prompt }, ...array(body['attachments'])] });
    return { activated: true, skill_name: name };
  }
  if (get && (action === '/approvals' || action === '/questions')) {
    const pending = await runtime.interactions();
    const at = (key: string) => runtime.timestamps.get(key) || new Date().toISOString();
    return { items: action === '/approvals' ? pending.permissions.filter((item) => item.session_id === id).map((item) => approval(item, at(item.id))) : pending.questions.filter((item) => item.session_id === id).map((item) => question(item, at(item.id))) };
  }
  if (post && action.startsWith('/approvals/')) {
    const approvalId = action.slice(11).replace(/:respond$/, '');
    const pending = await runtime.interactions();
    const permission = pending.permissions.find((item) => item.id === approvalId);
    if (!permission) throw new GoRequestError(409, 'Approval was already resolved in the Go runtime');
    if (permission.session_id !== id) await runtime.agentSession(id, permission.session_id);
    if (!['approved', 'rejected', 'cancelled'].includes(text(body['decision']))) throw new GoRequestError(400, 'Invalid approval decision');
    const permissionAction = body['decision'] === 'approved' ? body['scope'] === 'session' ? 'allow_session' : 'allow' : 'deny';
    const result = await runtime.client.json<{ resolved: boolean }>('POST', `${runtime.base}/permissions/grant`, { permission, action: permissionAction });
    runtime.markDirty(id);
    return { resolved: result.resolved, resolved_at: new Date().toISOString() };
  }
  if (post && action.startsWith('/questions/')) {
    const dismiss = action.endsWith(':dismiss');
    const questionId = action.slice(11).replace(/:dismiss$/, '');
    const pending = await runtime.interactions();
    const request = pending.questions.find((item) => item.id === questionId);
    if (!request) throw new GoRequestError(409, 'Question was already resolved in the Go runtime');
    if (request.session_id !== id) await runtime.agentSession(id, request.session_id);
    if (dismiss) { await runtime.client.json('POST', `${runtime.base}/questions/cancel`, { batch_request_id: questionId }); runtime.markDirty(id); return { dismissed: true, dismissed_at: new Date().toISOString() }; }
    const answers = object(body['answers']);
    const responses = request.questions.map((item) => {
      const answer = object(answers[item.id]);
      const selected = text(answer['option_id']) ? [text(answer['option_id'])] : array(answer['option_ids']).filter((item): item is string => typeof item === 'string');
      return { request_id: item.id, selected_ids: selected, fill_in_text: text(answer['text']) || text(answer['other_text']) || undefined, yes: item.type === 'yes_no' ? selected.includes('yes') ? true : selected.includes('no') ? false : undefined : undefined };
    });
    const result = await runtime.client.json<{ resolved: boolean }>('POST', `${runtime.base}/questions/answer`, { batch_request_id: questionId, responses });
    runtime.markDirty(id);
    return { resolved: result.resolved, resolved_at: new Date().toISOString() };
  }
  throw new UnsupportedError(`${req.method} ${path}`);
}

async function workspace(runtime: GoRuntime) {
  const sessions = await runtime.sessions();
  const oldest = Math.min(...sessions.map((item) => item.created_at), Date.now() / 1000);
  const newest = Math.max(...sessions.map((item) => item.updated_at), oldest);
  return { id: workspaceKey(runtime.workspace.path), root: runtime.workspace.path, name: basename(runtime.workspace.path), created_at: iso(oldest), last_opened_at: iso(newest), session_count: sessions.length };
}

async function skills(runtime: GoRuntime) {
  const items = await runtime.client.json<Array<{ id: string; name: string; description: string; source: string; user_invocable: boolean }>>('GET', `${runtime.base}/skills`);
  return { skills: [...handoffCommands(), ...items.filter((item) => item.user_invocable && !isHandoffTarget(item.name)).map((item) => ({ name: item.name, description: item.description, path: item.id, source: ['project', 'user', 'extra', 'builtin'].includes(item.source) ? item.source : 'extra', type: 'skill', scopes: ['tui', 'web'] }))] };
}

function pageItems<T extends { id: string }>(items: T[], query: URLSearchParams): { items: T[]; hasMore: boolean } {
  const before = query.get('before_id');
  const after = query.get('after_id');
  const limit = Math.min(100, Math.max(1, Number(query.get('page_size')) || 20));
  let start = 0;
  let end = items.length;
  if (before) start = items.findIndex((item) => item.id === before) + 1;
  if (after) end = items.findIndex((item) => item.id === after);
  const selected = items.slice(start, end < 0 ? undefined : end);
  return { items: selected.slice(0, limit), hasMore: selected.length > limit };
}

async function selectRuntime(runtime: GoRuntime, views: LocalViews, req: FastifyRequest, options: GoRouteOptions) {
  const url = new URL(req.url, 'http://web');
  const path = decodeURIComponent(url.pathname);
  const session = /^\/api\/v[12]\/sessions\/([^/:]+)/.exec(path);
  if (session) return options.workspaces.session(session[1]!);
  const workspace = /^\/api\/v[12]\/workspaces\/([^/]+)/.exec(path);
  if (workspace) return options.workspaces.workspace(workspace[1]!);
  const body = object(req.body);
  const root = url.searchParams.get('cwd') || text(body['cwd']) || text(object(body['metadata'])['cwd']) || text(body['workspace']);
  if (root) return options.workspaces.root(root);
  const workspaceId = url.searchParams.get('workspace_id') || text(body['workspace_id']);
  if (workspaceId) return options.workspaces.workspace(workspaceId);
  return { runtime, views };
}
