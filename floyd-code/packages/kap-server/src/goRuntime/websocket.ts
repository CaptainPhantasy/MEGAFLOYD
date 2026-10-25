import { randomUUID } from 'node:crypto';
import type { IncomingMessage } from 'node:http';
import { WebSocket, WebSocketServer } from 'ws';
import type { IConnectionRegistry } from '../transport/ws/connectionRegistry';
import { GoRuntime, UnsupportedError } from './runtime';
import { GoSessionPresence } from './presence';
import { array, object, text } from './projection';
import type { LocalViews } from './localViews';
import type { GoWorkspaces, GoWorkspaceView } from './workspaces';
import { terminalAttachPayloadSchema, terminalDetachPayloadSchema, terminalInputPayloadSchema, terminalResizePayloadSchema, terminalClosePayloadSchema } from '../protocol/ws-control';

export function createGoWebSocket(runtime: GoRuntime, registry: IConnectionRegistry, views: LocalViews, workspaces: GoWorkspaces): WebSocketServer {
  const server = new WebSocketServer({ noServer: true });
  server.on('connection', (socket, request) => attach(socket, request, runtime, registry, views, workspaces));
  return server;
}

function attach(socket: WebSocket, request: IncomingMessage, runtime: GoRuntime, registry: IConnectionRegistry, views: LocalViews, workspaces: GoWorkspaces): void {
  const id = randomUUID();
  const subscriptions = new Set<string>();
  const transcriptSubscriptions = new Map<string, Set<string>>();
  const owners = new Map<string, GoWorkspaceView>();
  const statuses = new Map<string, string>();
  let hello = false;
  let closed = false;
  let refreshing = false;
  let refreshAgain = false;
  let queue = Promise.resolve();
  let lastInbound = Date.now();
  const send = (value: unknown) => { if (!closed && socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(value)); };
  const event = (sessionId: string, type: string, payload: Record<string, unknown>) => send({ type, session_id: sessionId, seq: (owners.get(sessionId)?.runtime ?? runtime).sequence(sessionId), epoch: (owners.get(sessionId)?.runtime ?? runtime).epoch, volatile: true, timestamp: new Date().toISOString(), payload: { type, ...payload } });
  const ack = (frameId: unknown, payload: unknown, code = 0, msg = 'success') => send({ type: 'ack', id: text(frameId), code, msg, payload });
  const refresh = async () => {
    if (refreshing) { refreshAgain = true; return; }
    refreshing = true;
    try {
      do {
        refreshAgain = false;
        for (const sessionId of subscriptions) {
          try {
            const current = owners.get(sessionId)!.runtime;
            const view = await current.view(sessionId);
            event(sessionId, 'event.session.work_changed', { busy: view.session.is_busy, main_turn_active: view.session.is_busy, pending_interaction: view.wire.pending_interaction, last_turn_reason: view.session.is_busy ? undefined : 'completed' });
            const status = view.session.is_busy ? view.wire.pending_interaction === 'approval' ? 'awaiting_approval' : view.wire.pending_interaction === 'question' ? 'awaiting_question' : 'running' : 'idle';
            if (statuses.get(sessionId) !== status) {
              event(sessionId, 'event.session.status_changed', { status, previous_status: statuses.get(sessionId) ?? 'idle', current_prompt_id: view.wire.current_prompt_id });
              statuses.set(sessionId, status);
            }
            event(sessionId, 'session.meta.updated', { title: view.session.title });
            if (transcriptSubscriptions.has(sessionId)) {
              const requested = transcriptSubscriptions.get(sessionId)!;
              const roster = await current.agents(sessionId);
              for (const agent of roster) if (requested.has('*') || requested.has(agent.agentId)) {
                const agentView = agent.agentId === 'main' ? view : await current.view(await current.agentSession(sessionId, agent.agentId));
                event(sessionId, 'transcript.reset', { agent_id: agent.agentId, snapshot: agentView.snapshot, has_more_older: false, seq: agentView.seq });
              }
            }
          } catch (error) {
            send({ type: 'error', timestamp: new Date().toISOString(), payload: { code: 50200, msg: error instanceof Error ? error.message : String(error), fatal: false } });
          }
        }
      } while (refreshAgain && !closed);
    } finally { refreshing = false; }
  };
  const changed = ({ runtime: current, ids }: { runtime: GoRuntime; ids?: string[] }) => { if ([...subscriptions].some((sessionId) => owners.get(sessionId)?.runtime === current) && (!ids || ids.length)) void refresh(); };
  const unavailable = (message: string) => send({ type: 'error', timestamp: new Date().toISOString(), payload: { code: 50200, msg: message, fatal: false } });
  const presences = new Map<string, GoSessionPresence>();
  let selectedPresence: GoSessionPresence | undefined;
  const selectPresence = async (sessionId: string) => {
    if (!sessionId) { await selectedPresence?.select(''); selectedPresence = undefined; return; }
    const current = owners.get(sessionId)!.runtime;
    let presence = presences.get(current.workspace.id);
    if (!presence) { presence = new GoSessionPresence(current.host, current.workspace.id, unavailable); presences.set(current.workspace.id, presence); }
    if (selectedPresence && selectedPresence !== presence) await selectedPresence.select('');
    selectedPresence = presence;
    await presence.select(sessionId);
  };
  const prompt = (value: { runtime: GoRuntime; sessionId: string; type: string; payload: Record<string, unknown> }) => {
    if (subscriptions.has(value.sessionId)) event(value.sessionId, value.type, value.payload);
    for (const sessionId of subscriptions) if (sessionId !== value.sessionId && owners.get(sessionId)?.runtime === value.runtime && transcriptSubscriptions.has(sessionId)) {
      void value.runtime.agentSession(sessionId, value.sessionId).then(() => event(sessionId, value.type, { ...value.payload, agentId: value.sessionId })).catch(() => {});
    }
  };
  const sessionChanged = ({ runtime: current, type, session }: { runtime: GoRuntime; type: string; session: Record<string, unknown> }) => {
    const sessionId = text(session['id']);
    if (type === 'created') void current.view(sessionId).then((view) => event(sessionId, 'event.session.created', { session: view.wire })).catch((error) => unavailable(error instanceof Error ? error.message : String(error)));
    if (type === 'deleted') event(sessionId, 'event.session.deleted', { workspace_id: text(session['workspace_id']) });
  };
  workspaces.on('changed', changed);
  workspaces.on('unavailable', unavailable);
  workspaces.on('session', sessionChanged);
  workspaces.on('prompt', prompt);
  registry.add({ id, connectedAt: new Date().toISOString(), remoteAddress: request.socket.remoteAddress ?? null, userAgent: request.headers['user-agent'] ?? null, get hasClientHello() { return hello; }, get subscriptionSessionIds() { return [...subscriptions]; }, close: (code, reason) => socket.close(code, reason) });
  send({ type: 'server_hello', timestamp: new Date().toISOString(), payload: { ws_connection_id: id, protocol_version: 2, heartbeat_ms: 10000, max_event_buffer_size: 1000, capabilities: { event_batching: false, compression: false } } });
  const heartbeat = setInterval(() => {
    if (Date.now() - lastInbound > 30000) socket.close(1001, 'heartbeat timeout');
    else send({ type: 'ping', timestamp: new Date().toISOString(), payload: { nonce: randomUUID() } });
  }, 10000);
  heartbeat.unref();

  const subscribe = async (ids: unknown[], cursors: Record<string, unknown> = {}) => {
    const accepted: string[] = [];
    const notFound: string[] = [];
    const resync: string[] = [];
    const current: Record<string, unknown> = {};
    for (const raw of ids) {
      const sessionId = text(raw);
      if (!sessionId) continue;
      try { owners.set(sessionId, await workspaces.session(sessionId)); } catch { notFound.push(sessionId); continue; }
      if (closed) break;
      subscriptions.add(sessionId);
      accepted.push(sessionId);
      const owner = owners.get(sessionId)!.runtime;
      current[sessionId] = { seq: owner.sequence(sessionId), epoch: owner.epoch };
      if (cursors[sessionId] && object(cursors[sessionId])['epoch'] !== owner.epoch) resync.push(sessionId);
    }
    if (accepted.length) await selectPresence(accepted.at(-1)!);
    return { accepted, not_found: notFound, resync_required: resync, cursors: current };
  };
  socket.on('message', (data) => {
    lastInbound = Date.now();
    queue = queue.then(async () => {
      if (closed) return;
      let frame: Record<string, unknown>;
      try { frame = object(JSON.parse(String(data))); } catch { throw new Error('Invalid WebSocket JSON'); }
      const payload = object(frame['payload']);
      try {
        if (frame['type'] === 'pong') return;
        if (frame['type'] === 'client_hello') {
          hello = true;
          const result = await subscribe(array(payload['subscriptions']), object(payload['cursors']));
          ack(frame['id'], { accepted_subscriptions: result.accepted, resync_required: result.resync_required, cursors: result.cursors });
          await refresh();
          return;
        }
        if (!hello) throw new Error('Send client_hello before subscribing');
        if (frame['type'] === 'terminal_attach') {
          const value = terminalAttachPayloadSchema.parse(payload);
          const terminal = await (await workspaces.session(value.session_id)).views.terminal(value.session_id);
          const result = await terminal.attach(value.terminal_id, { id, send }, { sinceSeq: value.since_seq });
          ack(frame['id'], { attached: true, replayed: result.replayed });
          return;
        }
        if (frame['type'] === 'terminal_detach') {
          const value = terminalDetachPayloadSchema.parse(payload);
          (await (await workspaces.session(value.session_id)).views.terminal(value.session_id)).detach(value.terminal_id, id);
          ack(frame['id'], { detached: true });
          return;
        }
        if (frame['type'] === 'terminal_input') {
          const value = terminalInputPayloadSchema.parse(payload);
          await (await (await workspaces.session(value.session_id)).views.terminal(value.session_id)).write(value.terminal_id, value.data);
          ack(frame['id'], { accepted: true });
          return;
        }
        if (frame['type'] === 'terminal_resize') {
          const value = terminalResizePayloadSchema.parse(payload);
          await (await (await workspaces.session(value.session_id)).views.terminal(value.session_id)).resize(value.terminal_id, value.cols, value.rows);
          ack(frame['id'], { resized: true });
          return;
        }
        if (frame['type'] === 'terminal_close') {
          const value = terminalClosePayloadSchema.parse(payload);
          ack(frame['id'], await (await (await workspaces.session(value.session_id)).views.terminal(value.session_id)).close(value.terminal_id));
          return;
        }
        if (frame['type'] === 'subscribe') { ack(frame['id'], await subscribe(array(payload['session_ids']), object(payload['cursors']))); await refresh(); return; }
        if (frame['type'] === 'subscribe_v2') {
          const sessionId = text(payload['session_id']);
          const grade = payload['transcript'];
          const result = await subscribe([sessionId]);
          if (result.accepted.length) {
            const requested = grade && typeof grade === 'object' ? Object.keys(object(grade)).filter((key) => object(grade)[key] !== false) : ['main'];
            for (const agentId of requested) if (agentId !== '*') await owners.get(sessionId)!.runtime.agentSession(sessionId, agentId);
            transcriptSubscriptions.set(sessionId, new Set(requested.length ? requested : ['main']));
          }
          ack(frame['id'], result);
          await refresh();
          return;
        }
        if (frame['type'] === 'unsubscribe' || frame['type'] === 'unsubscribe_v2') {
          const ids = frame['type'] === 'unsubscribe' ? array(payload['session_ids']).map(text) : [text(payload['session_id'])];
          for (const sessionId of ids) { transcriptSubscriptions.delete(sessionId); if (frame['type'] === 'unsubscribe') subscriptions.delete(sessionId); }
          if (selectedPresence && ids.includes(selectedPresence.currentSession)) await selectPresence('');
          ack(frame['id'], { accepted: ids, not_found: [], resync_required: [] });
          return;
        }
        if (frame['type'] === 'abort') { const current = (await workspaces.session(text(payload['session_id']))).runtime; await current.client.json('POST', `${current.base}/agent/sessions/${encodeURIComponent(text(payload['session_id']))}/cancel`, {}); current.markDirty(text(payload['session_id'])); ack(frame['id'], { aborted: true }); return; }
        throw new UnsupportedError(`WebSocket ${text(frame['type'])}`);
      } catch (error) { ack(frame['id'], null, error instanceof UnsupportedError ? 50100 : 40000, error instanceof Error ? error.message : String(error)); }
    }).catch((error) => unavailable(error instanceof Error ? error.message : String(error)));
  });
  const close = () => {
    if (closed) return;
    closed = true;
    clearInterval(heartbeat);
    workspaces.off('changed', changed);
    workspaces.off('unavailable', unavailable);
    workspaces.off('session', sessionChanged);
    workspaces.off('prompt', prompt);
    registry.remove(id);
    views.detach(id);
    for (const owner of owners.values()) owner.views.detach(id);
    for (const presence of presences.values()) void presence.close();
  };
  socket.on('close', close);
  socket.on('error', close);
}
