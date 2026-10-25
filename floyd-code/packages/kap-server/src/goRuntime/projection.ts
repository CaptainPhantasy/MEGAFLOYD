import { createHash } from 'node:crypto';
import { basename } from 'node:path';
import type { AgentTranscriptSnapshot, TranscriptTurn, TranscriptStep, ToolCallFrame, TranscriptInteraction, TranscriptAttachment } from '@legacy-ai/transcript';
import type { GoAgentInfo, GoInteractions, GoMessage, GoPermission, GoQuestion, GoSession, GoWorkspace } from './client';

type Mutable<T> = { -readonly [K in keyof T]: T[K] };
type BuiltStep = Mutable<TranscriptStep>;
type BuiltTurn = Omit<Mutable<TranscriptTurn>, 'steps'> & { steps: BuiltStep[] };

export function iso(seconds: number): string { return new Date(seconds < 100000000000 ? seconds * 1000 : seconds).toISOString(); }
export function text(value: unknown): string { return typeof value === 'string' ? value : ''; }
export function object(value: unknown): Record<string, unknown> { return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
export function array(value: unknown): unknown[] { return Array.isArray(value) ? value : []; }
export function workspaceKey(path: string): string { return `wd_${basename(path).toLowerCase().replace(/[^a-z0-9._-]/g, '_') || 'root'}_${createHash('sha256').update(path).digest('hex').slice(0, 12)}`; }
export function modelKey(info: GoAgentInfo): string { return `${info.model_cfg.provider}/${info.model_cfg.model}`; }
export function thinkingLevel(info: GoAgentInfo): string {
  const effort = info.model_cfg.reasoning_effort || '';
  if (!info.model.can_reason || ['none', 'off', 'disabled'].includes(effort)) return 'off';
  return effort || (info.model_cfg.think ? 'on' : 'off');
}

function permissionInput(request: GoPermission): unknown {
  if (typeof request.params !== 'string') return request.params;
  try { return JSON.parse(request.params); } catch { return request.params; }
}

function permissionDisplay(request: GoPermission) {
  const input = permissionInput(request);
  const params = object(input);
  return typeof params['command'] === 'string'
    ? { kind: 'command', command: params['command'], cwd: request.path, description: request.description, language: 'bash' }
    : { kind: 'generic', summary: request.description || `${request.action} ${request.tool_name}`, detail: input };
}

export function approval(request: GoPermission, createdAt: string) {
  return { approval_id: request.id, session_id: request.session_id, tool_call_id: request.tool_call_id, tool_name: request.tool_name, action: request.description || `${request.action} ${request.tool_name}`, tool_input_display: permissionDisplay(request), created_at: createdAt, expires_at: '9999-12-31T23:59:59.999Z' };
}

export function question(request: GoQuestion, createdAt: string) {
  return { question_id: request.id, session_id: request.session_id, tool_call_id: request.tool_call_id || undefined, created_at: createdAt, questions: request.questions.map((item) => ({ id: item.id, question: item.question, header: item.label, body: item.description, options: item.choices ?? (item.type === 'yes_no' ? [{ id: 'yes', label: 'Yes' }, { id: 'no', label: 'No' }] : []), multi_select: item.type === 'multi_choice', allow_other: true })) };
}

export function projectInteractions(pending: GoInteractions, sessionId: string, timestamps: Map<string, string>): TranscriptInteraction[] {
  const at = (id: string) => { let value = timestamps.get(id); if (!value) { value = new Date().toISOString(); timestamps.set(id, value); } return value; };
  return [
    ...pending.permissions.filter((item) => item.session_id === sessionId).map((item): TranscriptInteraction => ({ interactionId: item.id, interactionKind: 'approval', toolCallId: item.tool_call_id, state: 'pending', request: { ...approval(item, at(item.id)), id: item.id, sessionId, agentId: 'main', toolCallId: item.tool_call_id, toolName: item.tool_name, description: item.description, display: permissionDisplay(item), input: permissionInput(item) } })),
    ...pending.questions.filter((item) => item.session_id === sessionId).map((item): TranscriptInteraction => ({ interactionId: item.id, interactionKind: 'question', toolCallId: item.tool_call_id, state: 'pending', request: question(item, at(item.id)) })),
  ];
}

export function projectSession(session: GoSession, workspace: GoWorkspace, info: GoAgentInfo, pending: TranscriptInteraction[], seq: number, permission: string) {
  return { id: session.id, workspace_id: workspaceKey(workspace.path), title: session.title, created_at: iso(session.created_at), updated_at: iso(session.updated_at), busy: session.is_busy, main_turn_active: session.is_busy, archived: session.archived ?? false, archived_at: session.archived_at ? iso(session.archived_at) : undefined, pending_interaction: pending.some((item) => item.interactionKind === 'approval') ? 'approval' : pending.length ? 'question' : 'none', metadata: { ...session.metadata, cwd: workspace.path, ...(session.parent_session_id ? { parent_session_id: session.parent_session_id } : {}) }, agent_config: { ...object(session.metadata?.['agent_config']), model: modelKey(info), thinking: thinkingLevel(info), permission_mode: permission }, usage: { input_tokens: session.prompt_tokens, output_tokens: session.completion_tokens, cache_read_tokens: 0, cache_creation_tokens: 0, total_cost_usd: session.cost, context_tokens: session.prompt_tokens + session.completion_tokens, context_limit: info.model.context_window }, permission_rules: [], message_count: session.message_count, last_seq: seq };
}

export function projectTranscript(messages: GoMessage[], session: GoSession, info: GoAgentInfo, interactions: TranscriptInteraction[], permission: 'manual' | 'yolo' | 'auto', promptIds: ReadonlyMap<string, string> = new Map()): AgentTranscriptSnapshot {
  const turns: BuiltTurn[] = [];
  const attachments: TranscriptAttachment[] = [];
  const tools = new Map<string, ToolCallFrame>();
  let turn: BuiltTurn | undefined;
  const ensureTurn = (message: GoMessage) => {
    if (!turn) { turn = { kind: 'turn', turnId: message.id, ordinal: turns.length + 1, state: 'completed', origin: { kind: message.is_summary_message ? 'compaction' : 'other' }, steps: [], startedAt: iso(message.created_at) }; turns.push(turn); }
    return turn;
  };
  for (const message of messages) {
    if (message.role === 'user') {
      turn = { kind: 'turn', turnId: message.id, triggerPromptId: promptIds.get(message.id) ?? message.id, ordinal: turns.length + 1, state: 'completed', origin: { kind: 'user' }, prompt: message.parts.filter((part) => part.type === 'text').map((part) => text(part.data['text'])).join('\n'), steps: [], startedAt: iso(message.created_at) };
      turns.push(turn);
    }
    const current = ensureTurn(message);
    const step: BuiltStep = { kind: 'step', stepId: message.id, turnId: current.turnId, ordinal: current.steps.length, state: 'completed', frames: [], startedAt: iso(message.created_at) };
    for (let index = 0; index < message.parts.length; index++) {
      const { type, data } = message.parts[index]!;
      const frameId = `${message.id}-${index}`;
      if (type === 'text' && message.role === 'assistant') step.frames.push({ kind: 'text', frameId, role: 'assistant', text: text(data['text']) });
      else if (type === 'reasoning') step.frames.push({ kind: 'thinking', frameId, text: text(data['thinking']) });
      else if (type === 'tool_call') {
        let input: unknown = text(data['input']);
        try { input = JSON.parse(text(data['input'])); } catch {}
        const frame: ToolCallFrame = { kind: 'tool', frameId, toolCallId: text(data['id']), name: text(data['name']), state: 'running', input, inputText: text(data['input']), approvalId: interactions.find((item) => item.toolCallId === data['id'])?.interactionId };
        tools.set(frame.toolCallId, frame);
        step.frames.push(frame);
      } else if (type === 'tool_result') {
        const id = text(data['tool_call_id']);
        const frame = tools.get(id);
        const result = { state: data['is_error'] === true ? 'error' as const : 'done' as const, output: text(data['content']), error: data['is_error'] === true ? text(data['content']) : undefined };
        if (frame) Object.assign(frame, result);
        else step.frames.push({ kind: 'tool', frameId, toolCallId: id, name: text(data['name']), ...result });
      } else if (type === 'finish') {
        const reason = text(data['reason']);
        step.finishReason = reason;
        step.endedAt = iso(Number(data['time']) || message.updated_at);
        if (reason === 'error' || reason === 'content_filter') { step.state = 'failed'; current.state = 'failed'; current.error = text(data['message']) || text(data['details']) || reason; }
        else if (reason === 'canceled') { step.state = 'interrupted'; current.state = 'cancelled'; }
        if (reason !== 'tool_use') current.endedAt = step.endedAt;
      } else if (type === 'shell_command') step.frames.push({ kind: 'tool', frameId, toolCallId: frameId, name: 'bash', input: { command: text(data['command']) }, output: text(data['output']), state: Number(data['exit_code']) === 0 ? 'done' : 'error' });
      else if (type === 'image_url' || type === 'binary') {
        const mediaType = type === 'image_url' ? /^data:([^;,]+)/.exec(text(data['url']))?.[1] || 'image/*' : text(data['MIMEType']) || text(data['mime_type']) || 'application/octet-stream';
        const encoded = text(data['Data']) || text(data['data']);
        const path = text(data['Path']) || text(data['path']);
        const url = type === 'image_url' ? text(data['url']) : encoded ? `data:${mediaType};base64,${encoded}` : '';
        attachments.push({ attachmentId: frameId, mediaType, name: path ? basename(path) : undefined, size: encoded ? Buffer.from(encoded, 'base64').byteLength : undefined, source: url ? { kind: 'url', url } : undefined, placeholder: url ? undefined : `Attachment retained in Go session${path ? `: ${path}` : ''}` });
        if (message.role === 'user') { current.attachmentIds = [...(current.attachmentIds ?? []), frameId]; continue; }
        const role = 'assistant';
        const textFrame = step.frames.find((frame) => frame.kind === 'text' && frame.role === role);
        if (textFrame?.kind === 'text') Object.assign(textFrame, { attachmentIds: [...(textFrame.attachmentIds ?? []), frameId] });
        else step.frames.push({ kind: 'text', frameId: `${frameId}-ref`, role, text: '', attachmentIds: [frameId] });
      }
    }
    if (step.frames.length) current.steps.push(step);
  }
  if (session.is_busy && turns.length) {
    const last = turns.at(-1)!;
    last.state = 'running';
    last.endedAt = undefined;
    const step = last.steps.at(-1);
    if (step && !step.endedAt) step.state = 'running';
  }
  const last = turns.at(-1);
  return { items: turns, tasks: [], interactions, attachments, todos: session.todos?.length ? [{ todoId: `todos-${session.id}`, items: session.todos.map((item) => ({ title: item.content, status: item.status === 'completed' || item.status === 'done' ? 'done' : item.status === 'in_progress' ? 'in_progress' : 'pending' })) }] : [], prompts: turns.filter((item) => item.origin.kind === 'user').map((item) => ({ promptId: item.triggerPromptId ?? item.turnId, userMessageId: item.turnId, status: item.state === 'cancelled' ? 'aborted' : item.state, content: [{ type: 'text', text: item.prompt ?? '' }], createdAt: item.startedAt!, finishedAt: item.endedAt })), meta: { activity: session.is_busy ? 'turn' : 'idle', agent: { model: modelKey(info), thinkingEffort: thinkingLevel(info), contextTokens: session.prompt_tokens + session.completion_tokens, maxContextTokens: info.model.context_window, permission, phase: last ? session.is_busy ? { kind: 'running', turnId: last.ordinal, step: last.steps.length, stepId: last.steps.at(-1)?.stepId ?? last.turnId, since: Date.now() } : { kind: 'ended', turnId: last.ordinal, reason: last.state === 'failed' ? 'failed' : last.state === 'cancelled' ? 'cancelled' : 'completed', at: Date.now() } : undefined } } };
}

export function projectPlans(snapshot: AgentTranscriptSnapshot, toolCallId?: string) {
  const plans: Array<{ tool_call_id: string; turn_id: string; source: 'output'; plan: string; path?: string }> = [];
  for (const item of snapshot.items) {
    if (item.kind !== 'turn') continue;
    for (const step of item.steps) for (const frame of step.frames) {
      if (frame.kind !== 'tool' || frame.name !== 'exit_plan_mode' || toolCallId && frame.toolCallId !== toolCallId) continue;
      const output = text(frame.output);
      const marker = ['## Approved Plan:\n', '## Plan (auto-approved, not user-reviewed):\n'].find((value) => output.includes(value));
      if (!marker) continue;
      const plan = output.slice(output.indexOf(marker) + marker.length);
      if (plan.trim()) plans.push({ tool_call_id: frame.toolCallId, turn_id: item.turnId, source: 'output', plan, path: /^Plan saved to: (.+)$/m.exec(output)?.[1]?.trim() });
    }
  }
  return plans;
}
