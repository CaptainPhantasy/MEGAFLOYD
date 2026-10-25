import type { GoRuntime } from './runtime';
import { GoRequestError, type GoSession } from './client';
import { array, iso, text } from './projection';
import { undoSessionRequestSchema, forkSessionRequestSchema, createSessionChildRequestSchema } from '../protocol/rest-session';
import { fileHistoryChangesQuerySchema } from '../protocol/rest-file-history';

export async function sessionControl(runtime: GoRuntime, id: string, action: string, method: string, body: Record<string, unknown>, query: URLSearchParams): Promise<{ data: unknown } | undefined> {
  const base = `${runtime.base}/sessions/${encodeURIComponent(id)}`;
  if (method === 'POST' && [':archive', ':restore'].includes(action)) {
    await runtime.client.json('POST', `${base}/archive`, { archived: action === ':archive' });
    runtime.markDirty(id);
    return { data: action === ':archive' ? { archived: true } : (await runtime.view(id)).wire };
  }
  if (method === 'POST' && [':fork', ':branch', '/children', ':btw'].includes(action)) {
    const request = action === ':fork' ? forkSessionRequestSchema.parse(body) : createSessionChildRequestSchema.parse(body);
    const session = await runtime.client.json<GoSession>('POST', `${base}/${action === ':fork' ? 'fork' : 'branch'}`, request);
    runtime.markDirty();
    return { data: action === ':btw' ? { agent_id: session.id } : (await runtime.view(session.id)).wire };
  }
  if (method === 'GET' && action === '/children') {
    const children = (await runtime.sessions()).filter((session) => session.parent_session_id === id && (query.get('include_archive') === 'true' || !session.archived) && (!query.has('busy') || session.is_busy === (query.get('busy') === 'true')));
    const limit = Math.max(1, Math.min(100, Number(query.get('page_size')) || 20));
    const cursor = query.get('after');
    const offset = cursor ? children.findIndex((session) => session.id === cursor) + 1 : 0;
    const items = [];
    for (const child of children.slice(offset, offset + limit)) items.push((await runtime.view(child.id)).wire);
    return { data: { items, has_more: offset + items.length < children.length } };
  }
  if (method === 'POST' && action === ':undo') {
    const request = undoSessionRequestSchema.parse(body);
    await runtime.client.json('POST', `${base}/undo`, { count: request.count });
    runtime.markDirty(id);
    const messages = (await runtime.messages(id)).slice().reverse();
    const limit = request.page_size ?? 20;
    return { data: { messages: { items: messages.slice(0, limit).map((message) => ({ id: message.id, session_id: id, role: message.role, content: message.parts.map((part) => ({ type: part.type, ...part.data })), created_at: iso(message.created_at), model: message.model })), has_more: messages.length > limit }, status: await runtime.status(id) } };
  }
  if (method === 'GET' && ['/file-history/changes', '/file-history/content'].includes(action)) {
    fileHistoryChangesQuerySchema.parse({ turn_id: query.get('turn_id') ?? undefined });
    if (action.endsWith('/content') && (!query.get('path') || !['start', 'end'].includes(query.get('phase') || ''))) throw new GoRequestError(400, 'File history content requires a path and phase=start or phase=end');
    const [view, history] = await Promise.all([runtime.view(id), runtime.client.json<{ turns: Array<{ turn_id: number; user_message_id: string; recorded: boolean }> }>('GET', `${base}/file-history/turns`)]);
    const turn = view.snapshot.items.find((item) => item.kind === 'turn' && item.ordinal === Number(query.get('turn_id')));
    const recorded = turn?.kind === 'turn' ? history.turns.find((item) => item.user_message_id === turn.turnId) : undefined;
    if (!recorded?.recorded) return { data: action.endsWith('/changes') ? { changes: [], recorded: false } : { content: null } };
    const actual = new URLSearchParams(query);
    actual.set('turn_id', String(recorded.turn_id));
    return { data: await runtime.client.json('GET', `${base}${action}?${actual}`) };
  }
  if (action === '/goal' || action === '/plan' || action === '/swarm') {
    if (method === 'GET' || method === 'POST' || method === 'PATCH') {
      const data = await runtime.client.json(method === 'GET' ? 'GET' : 'POST', `${base}${action}`, method === 'GET' ? undefined : body);
      if (method !== 'GET') runtime.markDirty(id);
      return { data };
    }
  }
  if (method === 'GET' && action === '/tasks') {
    const items = await runtime.tasks(id);
    return { data: { items: query.has('status') ? items.filter((item) => item['status'] === query.get('status')) : items } };
  }
  const task = /^\/tasks\/(.+?)(?::(cancel|detach))?$/.exec(action);
  if (task) {
    const selected = (await runtime.tasks(id)).find((item) => item['id'] === task[1]);
    if (!selected) throw new GoRequestError(404, 'Task does not belong to this Go session');
    const taskBase = `${runtime.base}/tasks/${encodeURIComponent(task[1]!)}`;
    if (method === 'GET' && !task[2]) return { data: { ...selected, ...await runtime.client.json<Record<string, unknown>>('GET', `${taskBase}?${query}`) } };
    if (method === 'POST' && task[2]) { const data = await runtime.client.json('POST', `${taskBase}/${task[2]}`, {}); runtime.markDirty(id); return { data }; }
  }
  if (method === 'POST' && action === '/subagents') {
    const data = await runtime.client.json('POST', `${base}/subagents`, body);
    runtime.markDirty(id);
    return { data };
  }
  if (method === 'GET' && action === '/agents') return { data: { items: await runtime.agents(id) } };
  const prompt = /^\/prompts\/([^:]+)(?::(abort|steer))?$/.exec(action);
  if (prompt && method === 'POST' && prompt[2] === 'abort' && await runtime.isActivePrompt(id, prompt[1]!)) {
    await runtime.client.json('POST', `${runtime.base}/agent/sessions/${encodeURIComponent(id)}/cancel`, {});
    runtime.markDirty(id);
    return { data: { aborted: true } };
  }
  if (action === '/prompts::steer' || prompt) {
    const queueBase = `${runtime.base}/agent/sessions/${encodeURIComponent(id)}/queue`;
    const queue = await runtime.client.json<{ items: Array<{ id: string; run_id: string }> }>('GET', queueBase);
    const requested = action === '/prompts::steer' ? array(body['prompt_ids']).map(text) : [prompt![1]!];
    const selected = requested.map((id) => queue.items.find((item) => item.id === id || item.run_id === id));
    if (selected.some((item) => !item)) throw new GoRequestError(404, 'Queued prompt was not found');
    if (method === 'POST' && (action === '/prompts::steer' || prompt?.[2] === 'steer')) {
      await runtime.client.json('POST', `${queueBase}/steer`, { ids: selected.map((item) => item!.id) });
      runtime.markDirty(id);
      return { data: { steered: true, prompt_ids: requested } };
    }
    if (method === 'POST' && prompt?.[2] === 'abort' || method === 'DELETE') {
      await runtime.client.json('DELETE', `${queueBase}/${encodeURIComponent(selected[0]!.id)}`);
      runtime.markDirty(id);
      return { data: { aborted: true } };
    }
    if (method === 'PATCH') {
      await runtime.client.json('PATCH', `${queueBase}/${encodeURIComponent(selected[0]!.id)}`, { prompt: text(body['prompt']) || array(body['content']).map((part) => typeof part === 'object' && part ? text((part as Record<string, unknown>)['text']) : '').join('\n') });
      return { data: (await runtime.promptState(id)).queued.find((item) => item.prompt_id === requested[0]) };
    }
  }
  return undefined;
}

export async function batchSessions(runtimeForSession: (id: string) => Promise<GoRuntime>, action: string, body: Record<string, unknown>) {
  const ids = [...new Set(array(body['ids']).map(text))];
  if (!ids.length || ids.length > 5000 || ids.some((id) => !id)) throw new GoRequestError(400, 'Provide between one and 5000 session IDs');
  const results: Array<{ id: string; ok: boolean; error?: { code: number; message: string } }> = [];
  for (const id of ids) {
    try {
      const runtime = await runtimeForSession(id);
      const base = `${runtime.base}/sessions/${encodeURIComponent(id)}`;
      if (action === 'delete') await runtime.client.json('DELETE', base);
      else await runtime.client.json('POST', `${base}/archive`, { archived: action === 'archive' });
      runtime.markDirty(id);
      results.push({ id, ok: true });
    } catch (error) {
      results.push({ id, ok: false, error: { code: error instanceof GoRequestError ? error.status * 100 : 50000, message: error instanceof Error ? error.message : String(error) } });
    }
  }
  return { results, succeeded: results.filter((item) => item.ok).length, failed: results.filter((item) => !item.ok).length };
}
