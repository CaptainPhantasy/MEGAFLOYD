import { EventEmitter } from 'node:events';
import { randomUUID } from 'node:crypto';
import { realpath } from 'node:fs/promises';
import { resolve } from 'node:path';
import type { AgentDescriptor, AgentTranscriptSnapshot, TranscriptTask } from '@legacy-ai/transcript';
import { GoClient, GoRequestError, type GoAgentInfo, type GoEvent, type GoInteractions, type GoMessage, type GoSession, type GoWorkspace } from './client';
import { array, iso, modelKey, object, projectInteractions, projectSession, projectTranscript, text, thinkingLevel } from './projection';
import type { GoAttachments } from './attachments';
import { providerWireType } from './providers';
import { fixedHandoff, nativeHandoff, type HandoffTarget } from './handoff';

function preferredModelEffort(levels: string[]): string | undefined {
  for (const effort of ['high', 'max', 'xhigh', 'medium', 'low', 'minimal']) if (levels.includes(effort)) return effort;
  return levels.filter((effort) => !['none', 'off', 'disabled'].includes(effort)).at(-1);
}

export class UnsupportedError extends Error {
  readonly status = 501;
  constructor(operation: string) { super(`${operation} is not supported by the shared Go runtime.`); }
}

export interface SessionView {
  session: GoSession;
  info: GoAgentInfo;
  permission: 'manual' | 'yolo' | 'auto';
  messages: GoMessage[];
  snapshot: AgentTranscriptSnapshot;
  seq: number;
  wire: ReturnType<typeof projectSession> & { current_prompt_id?: string };
}

interface GoQueuedPrompt { id: string; prompt: string; run_id: string; user_message_id?: string; created_at: string | number }
interface GoQueue { items: GoQueuedPrompt[]; active: GoQueuedPrompt | null; count: number }

export class GoRuntime extends EventEmitter {
  readonly client: GoClient;
  readonly epoch = randomUUID();
  readonly timestamps = new Map<string, string>();
  attachments?: GoAttachments;
  workspace!: GoWorkspace;
  private closed = false;
  private stopStream?: () => void;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private updateTimer?: ReturnType<typeof setTimeout>;
  private retryDelay = 250;
  private seq = new Map<string, number>();
  private dirty = new Set<string>();
  private allDirty = false;
  private proposals = new Map<string, { sessionId: string; fingerprint: string; result: Promise<Record<string, unknown>> }>();
  private promptIds = new Map<string, string>();
  private completedControls = new Map<string, { anchor?: string; items: Extract<AgentTranscriptSnapshot['items'][number], { kind: 'marker' }>[] }>();
  private readFlights = new Map<string, Promise<SessionView>>();

  constructor(readonly host: string, readonly cwd: string, public dataDir: string) { super(); this.client = new GoClient(host); }
  get base(): string { return `/v1/workspaces/${encodeURIComponent(this.workspace.id)}`; }

  async start(): Promise<void> {
    await this.client.json('GET', '/v1/health');
    this.workspace = await this.client.json<GoWorkspace>('POST', '/v1/workspaces', { path: this.cwd, data_dir: this.dataDir || undefined, client_id: this.client.clientId });
    const actualPath = await realpath(this.workspace.path);
    if (actualPath !== await realpath(this.cwd)) throw new Error('Go runtime joined a different working directory');
    if (this.dataDir && this.workspace.data_dir && resolve(this.workspace.data_dir) !== resolve(this.dataDir)) throw new Error('Go runtime data directory differs from the canonical launch');
    this.dataDir = this.workspace.data_dir || this.dataDir;
    if (!this.dataDir) throw new Error('Go runtime did not provide its workspace data directory');
    await this.connect();
    if (!(await this.info()).is_ready) await this.client.json('POST', `${this.base}/agent/init`, { interactive: true });
    if (!(await this.info()).is_ready) throw new Error('The shared Go agent did not become ready');
  }

  private async connect(): Promise<void> {
    this.stopStream = await this.client.subscribe(this.workspace.id, (event) => this.onEvent(event), (error) => this.reconnect(error));
    this.retryDelay = 250;
    await this.interactions();
    this.markDirty();
  }

  private reconnect(error?: Error): void {
    if (this.closed || this.reconnectTimer) return;
    this.stopStream?.();
    this.emit('unavailable', error?.message ?? 'Go runtime event connection interrupted; reconnecting');
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = undefined;
      void this.connect().catch((cause) => { this.retryDelay = Math.min(5000, this.retryDelay * 2); this.reconnect(cause instanceof Error ? cause : new Error(String(cause))); });
    }, this.retryDelay);
    this.reconnectTimer.unref();
  }

  private onEvent(event: GoEvent): void {
    const data = object(object(event.payload)['payload']);
    const id = text(data['session_id']) || (event.type === 'session' ? text(data['id']) : '');
    this.markDirty(id || undefined);
    if (event.type === 'run_complete') {
      this.emit('run_complete', data);
      const promptId = text(data['run_id']);
      if (promptId) this.emit('prompt', { sessionId: id, type: data['cancelled'] ? 'prompt.aborted' : 'prompt.completed', payload: { agentId: 'main', promptId, finishedAt: new Date().toISOString(), abortedAt: data['cancelled'] ? new Date().toISOString() : undefined, reason: data['error'] ? 'failed' : 'completed' } });
    }
    if (event.type === 'session') this.emit('session', { type: object(event.payload)['type'], session: data });
  }

  markDirty(sessionId?: string): void {
    if (sessionId) this.dirty.add(sessionId); else this.allDirty = true;
    if (this.updateTimer || this.closed) return;
    this.updateTimer = setTimeout(() => {
      this.updateTimer = undefined;
      const ids = this.allDirty ? undefined : [...this.dirty];
      this.dirty.clear();
      this.allDirty = false;
      if (ids) for (const id of ids) this.seq.set(id, this.sequence(id) + 1);
      else for (const id of this.seq.keys()) this.seq.set(id, this.sequence(id) + 1);
      this.emit('changed', ids);
    }, 80);
    this.updateTimer.unref();
  }

  sequence(sessionId: string): number { if (!this.seq.has(sessionId)) this.seq.set(sessionId, 0); return this.seq.get(sessionId)!; }
  async interactions(): Promise<GoInteractions> { return this.client.json('GET', `${this.base}/interactions`); }
  async info(): Promise<GoAgentInfo> { return this.client.json('GET', `${this.base}/agent`); }
  async sessions(): Promise<GoSession[]> { return this.client.json('GET', `${this.base}/sessions`); }
  async session(id: string): Promise<GoSession> { return this.client.json('GET', `${this.base}/sessions/${encodeURIComponent(id)}`); }
  async messages(id: string): Promise<GoMessage[]> { return this.client.json('GET', `${this.base}/sessions/${encodeURIComponent(id)}/messages`); }
  async permission(): Promise<'manual' | 'yolo' | 'auto'> {
    const permission = await this.client.json<{ mode: 'manual' | 'yolo' | 'auto' }>('GET', `${this.base}/permissions/mode`);
    if (!['manual', 'yolo', 'auto'].includes(permission.mode)) throw new GoRequestError(502, 'Go runtime returned invalid permission settings');
    return permission.mode;
  }

  async agentSession(id: string, agentId = 'main'): Promise<string> {
    if (agentId === 'main') { await this.session(id); return id; }
    if ((await this.agents(id)).some((agent) => agent.type === 'sub' && agent.agentId === agentId)) return agentId;
    throw new GoRequestError(404, 'Agent does not belong to this Go conversation');
  }

  async agents(id: string): Promise<AgentDescriptor[]> {
    await this.session(id);
    const ids = new Set([id]);
    const parents = [id];
    const result: AgentDescriptor[] = [{ agentId: 'main', type: 'main' }];
    for (const parent of parents) {
      const children = await this.client.json<{ items: GoSession[] }>('GET', `${this.base}/sessions/${encodeURIComponent(parent)}/subagents`);
      for (const session of children.items) if (session.parent_session_id === parent && !ids.has(session.id)) {
        ids.add(session.id);
        parents.push(session.id);
        result.push({ agentId: session.id, type: 'sub', parentAgentId: session.parent_session_id === id ? 'main' : session.parent_session_id, label: session.title, createdAt: iso(session.created_at) });
      }
    }
    return result;
  }

  async tasks(id: string): Promise<Record<string, unknown>[]> {
    await this.session(id);
    const response = await this.client.json<{ items: Record<string, unknown>[] }>('GET', `${this.base}/tasks`);
    return response.items.filter((task) => task['session_id'] === id || task['parent_session_id'] === id).map((task) => ({ ...task, description: text(task['description']) || text(task['title']), kind: task['kind'] === 'shell' ? 'bash' : task['kind'], run_in_background: Boolean(task['run_in_background'] ?? task['detached']), agent_id: task['agent_id'] ?? (task['kind'] === 'subagent' ? task['session_id'] : undefined) }));
  }

  async status(id: string) {
    const [view, plan, swarm] = await Promise.all([this.view(id), this.client.json<Record<string, unknown>>('GET', `${this.base}/sessions/${encodeURIComponent(id)}/plan`), this.client.json<Record<string, unknown>>('GET', `${this.base}/sessions/${encodeURIComponent(id)}/swarm`)]);
    return { busy: view.session.is_busy, model: modelKey(view.info), thinking_level: thinkingLevel(view.info), permission: view.permission, plan_mode: plan['active'] === true, swarm_mode: swarm['active'] === true, context_tokens: view.session.prompt_tokens + view.session.completion_tokens, max_context_tokens: view.info.model.context_window };
  }

  async view(id: string): Promise<SessionView> {
    const running = this.readFlights.get(id);
    if (running) return running;
    const task = this.readView(id);
    this.readFlights.set(id, task);
    try { return await task; } finally { this.readFlights.delete(id); }
  }

  private async readView(id: string): Promise<SessionView> {
    const [session, messages, info, pending, permission, tasks, plan, swarm, goal, queue] = await Promise.all([this.session(id), this.messages(id), this.info(), this.interactions(), this.permission(), this.tasks(id), this.client.json<Record<string, unknown>>('GET', `${this.base}/sessions/${encodeURIComponent(id)}/plan`), this.client.json<Record<string, unknown>>('GET', `${this.base}/sessions/${encodeURIComponent(id)}/swarm`), this.client.json<Record<string, unknown> | null>('GET', `${this.base}/sessions/${encodeURIComponent(id)}/goal`), this.queue(id)]);
    if (queue.active?.user_message_id && queue.active.run_id) this.promptIds.set(queue.active.user_message_id, queue.active.run_id);
    const interactions = projectInteractions(pending, id, this.timestamps);
    let projected = projectTranscript(messages, session, info, interactions, permission, this.promptIds);
    const controls = this.completedControls.get(id);
    if (controls?.anchor === messages.findLast((message) => message.role === 'user')?.id) projected = { ...projected, items: [...projected.items, ...(controls?.items ?? [])] };
    else this.completedControls.delete(id);
    const transcriptTasks: TranscriptTask[] = tasks.map((task) => ({ taskId: text(task['id']), kind: task['kind'] === 'bash' ? 'shell' : task['kind'] === 'subagent' ? 'subagent' : 'tool', state: task['status'] === 'cancelled' ? 'killed' : ['running', 'completed', 'failed', 'timed_out', 'lost'].includes(text(task['status'])) ? task['status'] as TranscriptTask['state'] : 'lost', detached: Boolean(task['run_in_background']), description: text(task['description']), agentId: text(task['agent_id']) || undefined, outputTail: text(task['output_preview']), startedAt: text(task['started_at']) || text(task['created_at']) || undefined, endedAt: text(task['completed_at']) || undefined, model: text(task['model']) || undefined }));
    const snapshot: AgentTranscriptSnapshot = { ...projected, tasks: transcriptTasks, meta: { ...projected.meta, modes: { plan: plan['active'] ? { reviewPath: text(plan['path']) || undefined } : undefined, swarm: swarm['active'] ? { trigger: 'manual' } : undefined }, goal: goal ? { objective: text(goal['objective']), status: goal['status'] as 'active' | 'paused' | 'blocked' | 'complete', completionCriterion: text(goal['completionCriterion']) || undefined, budgetUsed: Number(goal['tokensUsed']) || 0, budgetLimit: typeof object(goal['budget'])['tokenBudget'] === 'number' ? Number(object(goal['budget'])['tokenBudget']) : undefined } : undefined } };
    const activePrompt = snapshot.prompts?.findLast((prompt) => prompt.status === 'running' || prompt.status === 'blocked');
    return { session, messages, info, permission, seq: this.sequence(id), snapshot, wire: { ...projectSession(session, this.workspace, info, interactions, this.sequence(id), permission), current_prompt_id: queue.active?.run_id || activePrompt?.promptId } };
  }

  async catalog() {
    const [cfg, info] = await Promise.all([this.client.json<Record<string, unknown>>('GET', `${this.base}/config`), this.info()]);
    const configured = object(cfg['providers']);
    const providers = Object.entries(configured).filter(([, value]) => object(value)['disable'] !== true).map(([id, raw]) => {
      const item = object(raw);
      let baseUrl: string | undefined;
      if (text(item['base_url'])) { try { const url = new URL(text(item['base_url'])); url.username = ''; url.password = ''; url.search = ''; baseUrl = url.toString(); } catch {} }
      return { id, type: providerWireType(text(item['type'])) || 'openai', base_url: baseUrl, has_api_key: Boolean(item['api_key'] || item['oauth']), status: id === info.model_cfg.provider && info.is_ready ? 'connected' as const : item['api_key'] || item['oauth'] ? 'connected' as const : 'unconfigured' as const, default_model: id === info.model_cfg.provider ? modelKey(info) : undefined, models: array(item['models']).map((entry) => `${id}/${text(object(entry)['id'])}`) };
    });
    const models = providers.flatMap((provider) => array(object(configured[provider.id])['models']).map((raw) => {
      const entry = object(raw);
      return { provider: provider.id, model: `${provider.id}/${text(entry['id'])}`, display_name: text(entry['name']) || text(entry['id']), max_context_size: Number(entry['context_window']) || 0, max_output_size: Number(entry['default_max_tokens']) || undefined, capabilities: [...(entry['supports_images'] || entry['supports_attachments'] ? ['image_in'] : []), ...(entry['can_reason'] ? ['thinking'] : [])], support_efforts: array(entry['reasoning_levels']).filter((value): value is string => typeof value === 'string'), default_effort: entry['can_reason'] ? preferredModelEffort(array(entry['reasoning_levels']).filter((value): value is string => typeof value === 'string')) : undefined };
    })).filter((item) => item.model.split('/').slice(1).join('/') && item.max_context_size > 0);
    if (!models.some((item) => item.model === modelKey(info)) && info.model_cfg.model) models.push({ provider: info.model_cfg.provider, model: modelKey(info), display_name: info.model.name || info.model_cfg.model, max_context_size: info.model.context_window || 1, max_output_size: info.model.default_max_tokens, capabilities: info.model.can_reason ? ['thinking'] : [], support_efforts: info.model.reasoning_levels || [], default_effort: info.model.can_reason ? preferredModelEffort(info.model.reasoning_levels || []) : undefined });
    return { providers, models, info };
  }

  async configure(selection: Record<string, unknown>, sessionId?: string, deferGoalStart = false): Promise<void> {
    for (const [key, value] of Object.entries(selection)) {
      if (!['model', 'thinking', 'permission_mode', 'plan_mode', 'swarm_mode', 'goal_objective', 'goal_control', 'profile', 'system_prompt', 'tools', 'mcp_servers', 'disabled_tools'].includes(key) && value !== undefined && value !== false) throw new GoRequestError(400, `Unknown agent setting ${key}`);
    }
    const info = await this.info();
    const model = text(selection['model']);
    const thinking = text(selection['thinking']);
    const modelChanged = Boolean(model && model !== modelKey(info));
    const currentThinking = thinkingLevel(info);
    if (modelChanged || thinking && thinking !== currentThinking) {
      if (await this.client.json<GoAgentInfo>('GET', `${this.base}/agent`).then((value) => value.is_busy)) throw new GoRequestError(409, 'Wait for the shared Go agent to finish before changing its model');
      const selected = modelChanged ? (await this.catalog()).models.find((item) => item.model === model) : undefined;
      if (modelChanged && !selected) throw new GoRequestError(400, 'Model is not configured in the Go runtime');
      const canReason = selected ? selected.capabilities.includes('thinking') : info.model.can_reason;
      const levels = selected ? selected.support_efforts : info.model.reasoning_levels || [];
      let next = { ...info.model_cfg };
      if (selected) {
        next = { provider: selected.provider, model: selected.model.slice(selected.provider.length + 1), max_tokens: selected.max_output_size ?? 0, reasoning_effort: canReason ? selected.default_effort || '' : '', think: canReason && !selected.default_effort };
      }
      if (!canReason) {
        next.reasoning_effort = '';
        next.think = false;
      } else if (['off', 'none', 'disabled'].includes(thinking)) {
        next.reasoning_effort = 'none';
        next.think = false;
      } else if (thinking) {
        const effort = ['on', 'enabled'].includes(thinking) ? preferredModelEffort(levels) || '' : thinking;
        if (effort && !levels.includes(effort)) throw new GoRequestError(400, 'The selected model does not support this reasoning effort');
        next.reasoning_effort = effort;
        next.think = !effort;
      }
      await this.client.json('POST', `${this.base}/config/model`, { scope: sessionId ? 1 : 0, model_type: 'large', model: next });
      await this.client.json('POST', `${this.base}/agent/update`, {});
    }
    if (selection['permission_mode'] !== undefined) {
      if (!['manual', 'yolo', 'auto'].includes(text(selection['permission_mode']))) throw new GoRequestError(400, 'Invalid permission mode');
      if (selection['permission_mode'] !== await this.permission()) await this.client.json('POST', `${this.base}/permissions/mode`, { mode: selection['permission_mode'] });
    }
    if (sessionId) {
      const sessionBase = `${this.base}/sessions/${encodeURIComponent(sessionId)}`;
      if (typeof selection['plan_mode'] === 'boolean' && selection['plan_mode'] !== (await this.client.json<{ active: boolean }>('GET', `${sessionBase}/plan`)).active) await this.client.json('POST', `${sessionBase}/plan`, { action: selection['plan_mode'] ? 'enter' : 'cancel' });
      if (typeof selection['swarm_mode'] === 'boolean' && selection['swarm_mode'] !== (await this.client.json<{ active: boolean }>('GET', `${sessionBase}/swarm`)).active) await this.client.json('POST', `${sessionBase}/swarm`, { active: selection['swarm_mode'] });
      const settings = Object.fromEntries(Object.entries(selection).filter(([key, value]) => ['system_prompt', 'tools', 'mcp_servers', 'profile', 'disabled_tools'].includes(key) && value !== undefined));
      if (Object.keys(settings).length) await this.client.json('POST', `${sessionBase}/settings`, settings);
      if (selection['goal_objective'] !== undefined || selection['goal_control'] !== undefined) await this.client.json('POST', `${sessionBase}/goal`, { objective: selection['goal_objective'], control: selection['goal_control'], defer_start: deferGoalStart || undefined });
    }
    this.markDirty();
  }

  async handoff(id: string, target?: HandoffTarget) {
    if (id) await this.session(id);
    const descriptor = await nativeHandoff(this.client, this.base, id, target);
    if (target) this.markDirty(id);
    return descriptor;
  }

  async activateHandoff(id: string, target: HandoffTarget, skillArgs?: string) {
    const result = await this.performHandoff(id, target, randomUUID(), [{ type: 'text', text: `/${target}` }]);
    const anchor = (await this.messages(id)).findLast((message) => message.role === 'user')?.id;
    const previous = this.completedControls.get(id);
    const items = previous && previous.anchor === anchor ? previous.items : [];
    const marker: Extract<AgentTranscriptSnapshot['items'][number], { kind: 'marker' }> = { kind: 'marker', markerId: text(result['prompt_id']), marker: 'skill', at: text(result['finished_at']), payload: { origin: { kind: 'skill_activation', trigger: 'user-slash', skillName: target, skillArgs }, status: 'completed' } };
    this.completedControls.set(id, { anchor, items: [...items, marker].slice(-32) });
    this.markDirty(id);
    return { activated: true, skill_name: target, ...result };
  }

  async submit(id: string, submission: Record<string, unknown>): Promise<Record<string, unknown>> {
    const content = array(submission['content']);
    if (!content.length) throw new GoRequestError(400, 'Prompt content is required');
    id = await this.agentSession(id, text(submission['agent_id']) || 'main');
    const runId = text(submission['prompt_id']) || randomUUID();
    const fingerprint = JSON.stringify(submission);
    const previous = this.proposals.get(runId);
    if (previous) {
      if (previous.sessionId !== id || previous.fingerprint !== fingerprint) throw new GoRequestError(409, 'Prompt ID already belongs to a different submission');
      return previous.result;
    }
    const target = fixedHandoff(submission);
    const result = target ? this.performHandoff(id, target, runId, content) : this.performSubmit(id, submission, runId, content);
    this.proposals.set(runId, { sessionId: id, fingerprint, result });
    return result;
  }

  private async performHandoff(id: string, target: HandoffTarget, runId: string, content: unknown[]): Promise<Record<string, unknown>> {
    const handoff = await this.handoff(id, target);
    const completedAt = new Date().toISOString();
    this.emit('prompt', { sessionId: id, type: 'prompt.completed', payload: { agentId: 'main', promptId: runId, finishedAt: completedAt, reason: 'completed' } });
    return { prompt_id: runId, user_message_id: runId, status: 'completed', content, created_at: completedAt, finished_at: completedAt, handoff };
  }

  private async performSubmit(id: string, submission: Record<string, unknown>, runId: string, content: unknown[]): Promise<Record<string, unknown>> {
    const session = await this.session(id);
    await this.configure(Object.fromEntries(['model', 'thinking', 'permission_mode', 'plan_mode', 'swarm_mode', 'goal_objective', 'goal_control', 'profile', 'disabled_tools'].filter((key) => submission[key] !== undefined).map((key) => [key, submission[key]])), id, true);
    let prompt = content.map((part) => text(object(part)['text'])).join('\n');
    for (const raw of array(submission['skills'])) {
      const skill = object(raw);
      prompt = `${await this.skillPrompt(text(skill['name']), text(skill['args']))}\n\n${prompt}`;
    }
    const existing = new Set((await this.messages(id)).map((message) => message.id));
    const attachments = this.attachments ? await this.attachments.materialize(this, content) : [];
    await this.client.json('POST', `${this.base}/agent`, { session_id: id, run_id: runId, prompt, attachments });
    const result: Record<string, unknown> = { prompt_id: runId, user_message_id: runId, status: session.is_busy ? 'queued' : 'running', content, created_at: new Date().toISOString() };
    if (!session.is_busy) {
      for (let attempt = 0; attempt < 5; attempt++) {
        const actual = (await this.messages(id)).find((message) => message.role === 'user' && !existing.has(message.id) && message.parts.some((part) => part.type === 'text' && text(part.data['text']) === prompt));
        if (actual) { result['user_message_id'] = actual.id; this.promptIds.set(actual.id, runId); break; }
        await new Promise((resolve) => setTimeout(resolve, 20));
      }
    }
    this.emit('prompt', { sessionId: id, type: 'prompt.submitted', payload: { agentId: 'main', promptId: runId, userMessageId: result['user_message_id'], status: result['status'], content, createdAt: result['created_at'] } });
    if (!session.is_busy) this.emit('prompt', { sessionId: id, type: 'prompt.started', payload: { agentId: 'main', promptId: runId } });
    this.markDirty(id);
    return result;
  }

  async skillPrompt(name: string, args: string): Promise<string> {
    const skills = await this.client.json<Array<{ id: string; name: string }>>('GET', `${this.base}/skills`);
    const skill = skills.find((item) => item.name === name || item.id === name);
    if (!skill) throw new GoRequestError(404, `Skill ${name} is not available in the Go runtime`);
    const read = await this.client.json<{ content: string }>('POST', `${this.base}/skills/read`, { skill_id: skill.id });
    return `Use the ${skill.name} skill for this request.\n\n${Buffer.from(read.content, 'base64').toString('utf8')}\n\nSkill arguments: ${args}`;
  }

  private async queue(id: string): Promise<GoQueue> { return this.client.json('GET', `${this.base}/agent/sessions/${encodeURIComponent(id)}/queue`); }

  async promptState(id: string) {
    const [queue, pending] = await Promise.all([this.queue(id), this.interactions()]);
    const convert = (item: GoQueuedPrompt, status: string) => ({ prompt_id: item.run_id || item.id, user_message_id: item.user_message_id || item.run_id || item.id, status, content: [{ type: 'text', text: item.prompt }], created_at: typeof item.created_at === 'number' ? iso(item.created_at) : item.created_at });
    const blocked = pending.permissions.some((entry) => entry.session_id === id) || pending.questions.some((entry) => entry.session_id === id);
    if (queue.active?.user_message_id && queue.active.run_id) this.promptIds.set(queue.active.user_message_id, queue.active.run_id);
    return { active: queue.active ? convert(queue.active, blocked ? 'blocked' : 'running') : null, queued: queue.items.map((item) => convert(item, 'queued')) };
  }

  async isActivePrompt(id: string, promptId: string): Promise<boolean> {
    const { active } = await this.queue(id);
    return active !== null && [active.id, active.run_id, active.user_message_id].includes(promptId);
  }

  async close(): Promise<void> {
    this.closed = true;
    clearTimeout(this.reconnectTimer);
    clearTimeout(this.updateTimer);
    this.stopStream?.();
    this.removeAllListeners();
    await this.client.close();
  }
}
