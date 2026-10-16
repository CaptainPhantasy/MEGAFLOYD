// Glyph mood engine — maps live harness state onto one of the 16 glyph moods.
// Pure and deterministic: every rule reads only the `GlyphSignals` snapshot it
// is handed, and time always arrives as an explicit `now` so it is trivially
// unit-testable. The subscription layer (`useGlyphMood`) polls the existing
// react-query surfaces and folds them into that snapshot.

import type {
  BackgroundTasksResponse,
  ContextResponse,
  SessionDetail,
  SessionSummary,
} from '../types';
import type { GlyphMood } from './glyph';

/** Recency windows (ms) that gate the decaying moods. */
export const GLYPH_WINDOWS = {
  /** A failure this fresh keeps the glyph angry. */
  error: 60_000,
  /** After `error` but inside this window a failure still earns a WARNING. */
  warning: 5 * 60_000,
  /** A clean assistant tail inside this window is a SUCCESS… */
  success: 8_000,
  /** …and inside this one it is still HAPPY. */
  happy: 60_000,
  /** A session touched this recently counts as live. */
  active: 45_000,
  /** A live burst that has lasted this long becomes FOCUSED. */
  focused: 90_000,
  /** Fleet-wide silence for this long puts the glyph to sleep. */
  sleepy: 10 * 60_000,
} as const;

const FAILURE_STATUSES = new Set(['failed', 'timed_out', 'killed', 'lost']);

const DEPLOY_COMMAND_RE = /\b(deploy|deploying|publish|publishing|rollout|ship|shipping)\b/i;
const BUILD_TOOL_RE = /bash|shell|command|process|terminal/i;

/** The subset of a wire record the engine cares about. */
export interface GlyphWireTailRecord {
  readonly type: string;
  readonly time?: number;
}

/** The newest projected transcript message, flattened for the engine. */
export interface GlyphTailMessage {
  readonly role: 'system' | 'user' | 'assistant' | 'tool';
  readonly time?: number;
  readonly isError: boolean;
  readonly toolNames: readonly string[];
}

export interface GlyphOpenSignals {
  readonly updatedAt: number;
  readonly health: SessionSummary['health'];
  readonly agentCount: number;
  readonly swarmActive: boolean;
  readonly planModeActive: boolean;
  readonly goalActive: boolean;
  readonly tail: GlyphTailMessage | null;
  /** Commands of currently-running background process tasks. */
  readonly runningProcessCommands: readonly string[];
  /** Currently-running background agent (subagent) tasks. */
  readonly runningAgentTasks: number;
  /** Tasks that failed inside the `error` window. */
  readonly recentFailedTasks: number;
  /** Tasks that failed inside the `warning` window but outside `error`. */
  readonly staleFailedTasks: number;
  /** An `interaction.request` sits unanswered at the tail of the wire. */
  readonly awaitingInput: boolean;
  /** When the current live burst started, or null while not live. */
  readonly liveSince: number | null;
}

export interface GlyphSignals {
  readonly now: number;
  readonly sessionCount: number;
  readonly newestUpdatedAt: number | null;
  readonly open: GlyphOpenSignals | null;
}

export interface GlyphMoodResult {
  readonly mood: GlyphMood;
  /** Short human explanation, shown next to the glyph. */
  readonly reason: string;
}

export function buildTailMessage(
  projected: ContextResponse['messages'][number] | undefined,
): GlyphTailMessage | null {
  if (!projected) return null;
  const message = projected.message;
  return {
    role: message.role,
    time: projected.time,
    isError: message.isError === true,
    toolNames: message.role === 'assistant' ? message.toolCalls.map((tc) => tc.name) : [],
  };
}

/** An unanswered `interaction.request` near the wire tail means the agent is
 *  blocked on the user. Only meaningful when the wire is actually loaded. */
function computeAwaitingInput(
  tail: readonly GlyphWireTailRecord[],
  now: number,
): boolean {
  for (let i = tail.length - 1; i >= 0; i -= 1) {
    const record = tail[i]!;
    if (record.type === 'interaction.resolved') return false;
    if (record.type === 'interaction.request') {
      return record.time === undefined || now - record.time < GLYPH_WINDOWS.active;
    }
  }
  return false;
}

export function buildOpenSignals(input: {
  now: number;
  summary: SessionSummary | undefined;
  detail: SessionDetail | undefined;
  context: ContextResponse | undefined;
  tasks: BackgroundTasksResponse | undefined;
  wireTail: readonly GlyphWireTailRecord[] | undefined;
  liveSince: number | null;
}): GlyphOpenSignals | null {
  if (!input.detail) return null;

  let runningProcessCommands: string[] = [];
  let runningAgentTasks = 0;
  let recentFailedTasks = 0;
  let staleFailedTasks = 0;
  for (const entry of input.tasks?.tasks ?? []) {
    const task = entry.task;
    if (task.status === 'running') {
      if (task.kind === 'process') runningProcessCommands = [...runningProcessCommands, task.command];
      if (task.kind === 'agent') runningAgentTasks += 1;
      continue;
    }
    if (!FAILURE_STATUSES.has(task.status)) continue;
    const endedAt = task.endedAt ?? task.startedAt;
    if (input.now - endedAt < GLYPH_WINDOWS.error) recentFailedTasks += 1;
    else if (input.now - endedAt < GLYPH_WINDOWS.warning) staleFailedTasks += 1;
  }

  const messages = input.context?.messages ?? [];
  const tail = buildTailMessage(messages.at(-1));

  return {
    updatedAt: input.summary?.updatedAt ?? 0,
    health: input.summary?.health ?? 'ok',
    agentCount: input.detail.agents.length,
    swarmActive: input.context?.swarm.active === true,
    planModeActive: input.context?.planMode.active === true,
    goalActive: (input.context?.goal ?? null) !== null,
    tail,
    runningProcessCommands,
    runningAgentTasks,
    recentFailedTasks,
    staleFailedTasks,
    awaitingInput: computeAwaitingInput(input.wireTail ?? [], input.now),
    liveSince: input.liveSince,
  };
}

function withinWindow(now: number, time: number | undefined, window: number): boolean {
  return time !== undefined && now - time < window;
}

/** Errors visible right now: recently failed background tasks plus an error
 *  flag on the tail message. A tail error without a timestamp counts — it is
 *  the newest thing in the transcript. */
function countFreshErrors(open: GlyphOpenSignals, now: number): number {
  let count = open.recentFailedTasks;
  if (open.tail?.isError && (open.tail.time === undefined || withinWindow(now, open.tail.time, GLYPH_WINDOWS.error))) {
    count += 1;
  }
  return count;
}

function classifyToolWork(toolNames: readonly string[]): GlyphMoodResult {
  if (toolNames.some((name) => BUILD_TOOL_RE.test(name))) {
    return { mood: 'BUILDING', reason: 'driving the shell' };
  }
  return { mood: 'ANALYZING', reason: 'running tools' };
}

/**
 * Mood precedence, highest first: broken session > fresh errors > awaiting
 * user input > deploy/build steps > swarm > celebration > stale failures >
 * long-running focus > active tool work > thinking > background watching >
 * sleepy > default. Each rule fires only on signals fresher than its window,
 * so the glyph decays back to DEFAULT on its own.
 */
export function resolveGlyphMood(signals: GlyphSignals): GlyphMoodResult {
  if (signals.sessionCount === 0) {
    return { mood: 'DEFAULT', reason: 'no sessions yet' };
  }

  const { now } = signals;
  const open = signals.open;
  const idleFor =
    signals.newestUpdatedAt === null ? Number.POSITIVE_INFINITY : now - signals.newestUpdatedAt;

  if (open) {
    if (open.health.startsWith('broken')) {
      return { mood: 'BROKEN', reason: 'session wire is damaged' };
    }

    const freshErrors = countFreshErrors(open, now);
    if (freshErrors >= 2) return { mood: 'BROKEN', reason: 'errors are piling up' };
    if (freshErrors === 1) return { mood: 'ERROR', reason: 'something just failed' };

    if (open.awaitingInput) {
      return { mood: 'SUSPICIOUS', reason: 'waiting on your answer' };
    }

    if (open.runningProcessCommands.some((command) => DEPLOY_COMMAND_RE.test(command))) {
      return { mood: 'DEPLOYING', reason: 'shipping a release' };
    }
    if (open.runningProcessCommands.length > 0) {
      return { mood: 'BUILDING', reason: 'running a background command' };
    }

    if (open.swarmActive && open.agentCount >= 4) {
      return { mood: 'BRAINSTORM', reason: 'the swarm is ideating' };
    }
    if (open.swarmActive) {
      return { mood: 'EXCITED', reason: 'swarm agents in flight' };
    }

    const tail = open.tail;
    const cleanAssistantTail =
      tail !== null && tail.role === 'assistant' && !tail.isError && tail.toolNames.length === 0;
    if (cleanAssistantTail && withinWindow(now, tail.time, GLYPH_WINDOWS.success)) {
      return { mood: 'SUCCESS', reason: 'turn wrapped up clean' };
    }
    if (cleanAssistantTail && withinWindow(now, tail.time, GLYPH_WINDOWS.happy)) {
      return { mood: 'HAPPY', reason: 'still riding the high' };
    }

    if (open.staleFailedTasks > 0) {
      return { mood: 'WARNING', reason: 'a background task failed earlier' };
    }

    const live = now - open.updatedAt < GLYPH_WINDOWS.active;
    if (live) {
      if (open.planModeActive || open.goalActive) {
        return { mood: 'FOCUSED', reason: 'locked onto the plan' };
      }
      if (open.liveSince !== null && now - open.liveSince >= GLYPH_WINDOWS.focused) {
        return { mood: 'FOCUSED', reason: 'deep in a long turn' };
      }
      if (tail !== null && tail.role === 'assistant' && tail.toolNames.length > 0) {
        return classifyToolWork(tail.toolNames);
      }
      return { mood: 'THINKING', reason: 'the model is chewing on it' };
    }
  }

  if (open !== null && open.runningAgentTasks > 0 && idleFor >= GLYPH_WINDOWS.active) {
    return { mood: 'STEALTH', reason: 'background agents are working' };
  }
  if (idleFor >= GLYPH_WINDOWS.sleepy) {
    return { mood: 'SLEEPY', reason: 'nothing has moved in a while' };
  }
  return { mood: 'DEFAULT', reason: 'standing by' };
}
