import { createHash } from 'node:crypto';
import { GoRequestError, type GoMessage } from './client';
import type { GoRuntime } from './runtime';
import type { GoWorkspaceView } from './workspaces';
import type { LocalViews } from './localViews';
import { iso, modelKey, object, text, workspaceKey } from './projection';

type Activity = 'running' | 'approval' | 'question' | 'failed' | 'idle';
type GitDomain = { branch: string | null; pull_request: { number: number; state: string; url: string } | null };
type SessionItem = {
  id: string;
  workspace: { id: string; cwd: string };
  meta: { title: string | null; last_prompt: string | null; created_at: number; updated_at: number; archived: boolean; archived_at: number | null };
  activity: { status: Activity; model: string | null };
  git?: GitDomain;
};

const statuses = ['running', 'approval', 'question', 'failed', 'idle'];
const sorts = ['meta.updated_at_desc', 'meta.updated_at_asc', 'meta.created_at_desc'];

function integer(value: string | null, fallback: number, min: number, max: number): number {
  if (value === null) return fallback;
  const result = Number(value);
  if (!Number.isInteger(result) || result < min || result > max) throw new GoRequestError(400, `Expected an integer from ${min} to ${max}`);
  return result;
}

async function gitDomain(views: LocalViews): Promise<GitDomain> {
  try {
    const status = object(await views.fsAction('git_status', {}));
    const pr = object(status['pullRequest']);
    return {
      branch: text(status['branch']) || null,
      pull_request: typeof pr['number'] === 'number' ? { number: pr['number'], state: pr['state'] === 'draft' ? 'open' : text(pr['state']), url: text(pr['url']) } : null,
    };
  } catch {
    return { branch: null, pull_request: null };
  }
}

async function sessionMessages(runtime: GoRuntime, ids: string[]): Promise<Map<string, GoMessage[]>> {
  const messages = new Map<string, GoMessage[]>();
  let index = 0;
  await Promise.all(Array.from({ length: Math.min(4, ids.length) }, async () => {
    while (index < ids.length) {
      const id = ids[index++]!;
      messages.set(id, await runtime.messages(id));
    }
  }));
  return messages;
}

export async function listGoSessionsV2(runtime: GoRuntime, views: LocalViews, query: URLSearchParams, sources: GoWorkspaceView[] = [{ runtime, views }]) {
  const view = query.get('view') || 'flat';
  const archived = query.get('meta.archived') || 'false';
  const hasPrompt = query.get('meta.has_prompt');
  const sort = query.get('sort') || 'meta.updated_at_desc';
  const fields = (query.get('fields') || '').split(',').filter(Boolean).toSorted();
  const projection = fields.length > 0;
  const include = (query.get('include') || '').split(',').filter(Boolean);
  if (!['flat', 'by_workspace'].includes(view) || !['true', 'false', 'all'].includes(archived) || !sorts.includes(sort)) throw new GoRequestError(400, 'Invalid session list view, archive filter, or sort');
  if (hasPrompt !== null && !['true', 'false'].includes(hasPrompt)) throw new GoRequestError(400, 'meta.has_prompt must be true or false');
  if (fields.length && fields.join(',') !== 'archived,id') throw new GoRequestError(400, 'Only fields=id,archived is supported');
  if (include.some((domain) => domain !== 'git') || projection && include.length) throw new GoRequestError(400, 'Invalid include domain for this session projection');
  if (query.getAll('activity.status').some((status) => !statuses.includes(status))) throw new GoRequestError(400, 'Invalid activity status');
  if (query.has('page') && query.has('page_token')) throw new GoRequestError(400, 'page and page_token are mutually exclusive');
  if (query.has('group.page_size') && view !== 'by_workspace') throw new GoRequestError(400, 'group.page_size requires view=by_workspace');
  const pageSize = integer(query.get('page_size'), 50, 1, projection ? 10000 : 100);
  const groupSize = integer(query.get('group.page_size'), 5, 1, projection ? 10000 : 100);
  const page = integer(query.get('page'), 1, 1, Number.MAX_SAFE_INTEGER);
  const after = integer(query.get('meta.updated_after'), 0, 0, Number.MAX_SAFE_INTEGER);
  const before = integer(query.get('meta.updated_before'), Number.MAX_SAFE_INTEGER, 0, Number.MAX_SAFE_INTEGER);
  const fingerprint = createHash('sha256').update(JSON.stringify([sources.map((entry) => entry.runtime.workspace.id).toSorted(), [...query.entries()].filter(([key]) => key !== 'page_token' && key !== 'page').toSorted((a, b) => a[0].localeCompare(b[0]) || a[1].localeCompare(b[1]))])).digest('hex').slice(0, 24);
  let cursor: { time: number; id: string } | undefined;
  if (query.has('page_token')) {
    try {
      const token = object(JSON.parse(Buffer.from(query.get('page_token')!, 'base64url').toString('utf8')));
      if (token['fingerprint'] !== fingerprint || typeof token['time'] !== 'number' || typeof token['id'] !== 'string') throw new Error();
      cursor = { time: token['time'], id: token['id'] };
    } catch {
      throw new GoRequestError(400, 'page_token does not match this query; restart from the first page');
    }
  }
  const items: SessionItem[] = [];
  const workspaceFilter = query.getAll('workspace.id');
  const statusFilter = query.getAll('activity.status');
  for (const entry of sources) {
    const runtime = entry.runtime;
    const workspace = { id: workspaceKey(runtime.workspace.path), cwd: runtime.workspace.path };
    if (workspaceFilter.length && !workspaceFilter.includes(workspace.id)) continue;
    const [sessions, info, pending] = await Promise.all([runtime.sessions(), runtime.info(), runtime.interactions()]);
    const eligible = sessions.filter((session) => {
      const updated = Date.parse(iso(session.updated_at));
      return !session.parent_session_id && updated >= after && updated <= before && (archived === 'all' || Boolean(session.archived) === (archived === 'true'));
    });
    const messages = await sessionMessages(runtime, eligible.filter((session) => session.message_count > 0).map((session) => session.id));
    for (const session of eligible) {
      const transcript = messages.get(session.id) || [];
      const user = transcript.findLast((message) => message.role === 'user');
      const lastPrompt = user?.parts.filter((part) => part.type === 'text').map((part) => text(part.data['text'])).join('\n') || null;
      const finish = transcript.findLast((message) => message.role === 'assistant')?.parts.findLast((part) => part.type === 'finish');
      const status: Activity = pending.permissions.some((request) => request.session_id === session.id) ? 'approval'
        : pending.questions.some((request) => request.session_id === session.id) ? 'question'
        : session.is_busy ? 'running'
        : ['error', 'content_filter'].includes(text(finish?.data['reason'])) ? 'failed' : 'idle';
      if (hasPrompt !== null && Boolean(lastPrompt) !== (hasPrompt === 'true') || statusFilter.length && !statusFilter.includes(status)) continue;
      items.push({ id: session.id, workspace, meta: { title: session.title || null, last_prompt: lastPrompt, created_at: Date.parse(iso(session.created_at)), updated_at: Date.parse(iso(session.updated_at)), archived: session.archived ?? false, archived_at: session.archived_at ? Date.parse(iso(session.archived_at)) : null }, activity: { status, model: info.is_ready ? modelKey(info) : null } });
    }
  }
  const key = (item: SessionItem) => sort === 'meta.created_at_desc' ? item.meta.created_at : item.meta.updated_at;
  const compare = (a: { time: number; id: string }, b: { time: number; id: string }) => (a.time - b.time || a.id.localeCompare(b.id)) * (sort === 'meta.updated_at_asc' ? 1 : -1);
  items.sort((a, b) => compare({ time: key(a), id: a.id }, { time: key(b), id: b.id }));
  const git = new Map<string, GitDomain>();
  const serve = async (window: SessionItem[]) => {
    if (include.includes('git')) for (const item of window) if (!git.has(item.workspace.id)) {
      const source = sources.find((entry) => workspaceKey(entry.runtime.workspace.path) === item.workspace.id)!;
      git.set(item.workspace.id, await gitDomain(source.views));
    }
    return projection ? window.map((item) => ({ id: item.id, archived: item.meta.archived })) : window.map((item) => ({ ...item, ...(git.has(item.workspace.id) ? { git: git.get(item.workspace.id) } : {}) }));
  };
  if (view === 'by_workspace') {
    const groups = new Map<string, SessionItem[]>();
    for (const item of items) { const group = groups.get(item.workspace.id) ?? []; group.push(item); groups.set(item.workspace.id, group); }
    const ordered = [...groups.values()];
    const offset = cursor ? ordered.findIndex((group) => compare({ time: key(group[0]!), id: group[0]!.id }, cursor!) > 0) : (page - 1) * pageSize;
    const selected = offset < 0 ? [] : ordered.slice(offset, offset + pageSize);
    const served = [];
    for (const group of selected) served.push({ workspace: group[0]!.workspace, sessions: await serve(group.slice(0, groupSize)), total: group.length });
    const hasMore = offset >= 0 && offset + selected.length < ordered.length;
    const last = selected.at(-1)?.[0];
    return { groups: served, total: ordered.length, has_more: hasMore, next_page_token: hasMore && last && !query.has('page') ? Buffer.from(JSON.stringify({ fingerprint, time: key(last), id: last.id })).toString('base64url') : null };
  }
  const start = cursor ? items.findIndex((item) => compare({ time: key(item), id: item.id }, cursor!) > 0) : (page - 1) * pageSize;
  const window = start < 0 ? [] : items.slice(start, start + pageSize);
  const hasMore = start >= 0 && start + window.length < items.length && window.length > 0;
  const last = window.at(-1);
  const next = hasMore && last && !query.has('page') ? Buffer.from(JSON.stringify({ fingerprint, time: key(last), id: last.id })).toString('base64url') : null;
  return { items: await serve(window), total: items.length, has_more: hasMore, next_page_token: next };
}
