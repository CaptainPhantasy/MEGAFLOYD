import { describe, expect, it } from 'vitest';

import {
  GLYPH_COLS,
  GLYPH_MOODS,
  GLYPH_MOOD_SPECS,
  GLYPH_ROWS,
} from '../src/lib/glyph';
import {
  buildOpenSignals,
  buildTailMessage,
  resolveGlyphMood,
  type GlyphOpenSignals,
  type GlyphSignals,
} from '../src/lib/glyph-mood';
import type {
  BackgroundTasksResponse,
  ContextResponse,
  SessionDetail,
  SessionSummary,
} from '../src/types';

const NOW = 1_800_000_000_000;

/** The guide's mood table: exact ANSI code + caption per mood. */
const GUIDE_TABLE: Record<string, { ansi: number; caption: string }> = {
  DEFAULT: { ansi: 36, caption: 'Ready. Always.' },
  HAPPY: { ansi: 32, caption: 'Good stuff!' },
  EXCITED: { ansi: 33, caption: "Let's go!" },
  FOCUSED: { ansi: 34, caption: 'Deep work...' },
  THINKING: { ansi: 35, caption: 'Processing...' },
  ANALYZING: { ansi: 95, caption: 'Running the numbers.' },
  SUSPICIOUS: { ansi: 208, caption: 'Hmmm...' },
  SUCCESS: { ansi: 92, caption: 'Nailed it.' },
  WARNING: { ansi: 93, caption: 'Careful...' },
  ERROR: { ansi: 91, caption: 'Nope.' },
  SLEEPY: { ansi: 90, caption: 'Later...' },
  BUILDING: { ansi: 96, caption: 'Compiling...' },
  DEPLOYING: { ansi: 37, caption: 'Shipping...' },
  BRAINSTORM: { ansi: 213, caption: 'Ideas incoming!' },
  STEALTH: { ansi: 90, caption: 'Watching...' },
  BROKEN: { ansi: 31, caption: 'Fix me...' },
};

const GUIDE_FRAME_COUNTS: Partial<Record<string, number>> = {
  DEFAULT: 2,
  HAPPY: 3,
  EXCITED: 3,
  THINKING: 3,
  DEPLOYING: 4,
};

describe('glyph data', () => {
  it('covers exactly the guide’s 16 moods', () => {
    expect([...GLYPH_MOODS].toSorted()).toEqual(Object.keys(GUIDE_TABLE).toSorted());
  });

  it('matches the guide’s colour codes and captions', () => {
    for (const mood of GLYPH_MOODS) {
      const spec = GLYPH_MOOD_SPECS[mood];
      expect(spec.ansi, mood).toBe(GUIDE_TABLE[mood]!.ansi);
      expect(spec.caption, mood).toBe(GUIDE_TABLE[mood]!.caption);
      expect(spec.color, mood).toMatch(/^#[0-9a-f]{6}$/);
    }
  });

  it('keeps every frame on the 12×6 pixel canvas with binary cells', () => {
    for (const mood of GLYPH_MOODS) {
      for (const frame of GLYPH_MOOD_SPECS[mood].frames) {
        expect(frame.length, mood).toBe(GLYPH_ROWS);
        for (const row of frame) {
          expect(row.length, `${mood}: ${row}`).toBe(GLYPH_COLS);
          expect(row, `${mood}: ${row}`).toMatch(/^[.#]+$/);
        }
        expect(frame.some((row) => row.includes('#')), `${mood} renders blank`).toBe(true);
      }
    }
  });

  it('animates the moods the guide lists and aligns frame timing', () => {
    for (const mood of GLYPH_MOODS) {
      const spec = GLYPH_MOOD_SPECS[mood];
      expect(spec.frameMs.length, mood).toBe(spec.frames.length);
      const expected = GUIDE_FRAME_COUNTS[mood] ?? 1;
      expect(spec.frames.length, mood).toBe(expected);
    }
  });

  it('keeps the canon DEFAULT glyph: bars, eye row, grin row', () => {
    const [open] = GLYPH_MOOD_SPECS.DEFAULT.frames;
    expect(open![2]![0]).toBe('#');
    expect(open![2]![GLYPH_COLS - 1]).toBe('#');
    expect(open![3]![0]).toBe('#');
    expect(open![3]![GLYPH_COLS - 1]).toBe('#');
    expect(open![2]).toContain('##');
    expect(open![4]!.slice(1, GLYPH_COLS - 1)).toContain('#');
    expect(open![5]!.slice(2, GLYPH_COLS - 2)).toContain('#');
  });

  it('floats THINKING’s question mark above the eye rows', () => {
    for (const frame of GLYPH_MOOD_SPECS.THINKING.frames) {
      expect(frame[0]!.includes('#') || frame[1]!.includes('#')).toBe(true);
    }
  });
});

function openSignals(overrides: Partial<GlyphOpenSignals> = {}): GlyphOpenSignals {
  return {
    updatedAt: NOW - 5_000,
    health: 'ok',
    agentCount: 1,
    swarmActive: false,
    planModeActive: false,
    goalActive: false,
    tail: null,
    runningProcessCommands: [],
    runningAgentTasks: 0,
    recentFailedTasks: 0,
    staleFailedTasks: 0,
    awaitingInput: false,
    liveSince: null,
    ...overrides,
  };
}

function signals(overrides: Partial<GlyphSignals> = {}): GlyphSignals {
  return {
    now: NOW,
    sessionCount: 1,
    newestUpdatedAt: NOW - 5_000,
    open: openSignals(),
    ...overrides,
  };
}

describe('resolveGlyphMood', () => {
  it('is deterministic for identical snapshots', () => {
    const snapshot = signals();
    expect(resolveGlyphMood(snapshot)).toEqual(resolveGlyphMood(snapshot));
  });

  it('shows DEFAULT with no sessions', () => {
    expect(resolveGlyphMood({ now: NOW, sessionCount: 0, newestUpdatedAt: null, open: null })).toEqual({
      mood: 'DEFAULT',
      reason: 'no sessions yet',
    });
  });

  it('escalates broken session health to BROKEN', () => {
    expect(resolveGlyphMood(signals({ open: openSignals({ health: 'broken_main_wire' }) })).mood).toBe('BROKEN');
  });

  it('maps one fresh error to ERROR and several to BROKEN', () => {
    const oneError = openSignals({
      tail: { role: 'tool', time: NOW - 1_000, isError: true, toolNames: [] },
    });
    expect(resolveGlyphMood(signals({ open: oneError })).mood).toBe('ERROR');

    const twoErrors = openSignals({
      tail: { role: 'tool', time: NOW - 1_000, isError: true, toolNames: [] },
      recentFailedTasks: 1,
    });
    expect(resolveGlyphMood(signals({ open: twoErrors })).mood).toBe('BROKEN');
  });

  it('expires errors after the error window', () => {
    const cooled = openSignals({
      tail: { role: 'tool', time: NOW - 61_000, isError: true, toolNames: [] },
    });
    expect(resolveGlyphMood(signals({ open: cooled })).mood).not.toBe('ERROR');
  });

  it('ranks awaiting-input above running work', () => {
    const waiting = openSignals({ awaitingInput: true, runningProcessCommands: ['pnpm build'] });
    expect(resolveGlyphMood(signals({ open: waiting })).mood).toBe('SUSPICIOUS');
  });

  it('detects deploys and builds from running process commands', () => {
    const deploy = openSignals({ runningProcessCommands: ['pnpm run deploy --prod'] });
    expect(resolveGlyphMood(signals({ open: deploy })).mood).toBe('DEPLOYING');

    const build = openSignals({ runningProcessCommands: ['cargo build --release'] });
    expect(resolveGlyphMood(signals({ open: build })).mood).toBe('BUILDING');
  });

  it('separates a small swarm (EXCITED) from a big one (BRAINSTORM)', () => {
    const trio = openSignals({ swarmActive: true, agentCount: 3 });
    expect(resolveGlyphMood(signals({ open: trio })).mood).toBe('EXCITED');

    const five = openSignals({ swarmActive: true, agentCount: 5 });
    expect(resolveGlyphMood(signals({ open: five })).mood).toBe('BRAINSTORM');
  });

  it('celebrates a clean assistant tail, then decays SUCCESS → HAPPY → away', () => {
    const fresh = openSignals({
      tail: { role: 'assistant', time: NOW - 2_000, isError: false, toolNames: [] },
    });
    expect(resolveGlyphMood(signals({ open: fresh })).mood).toBe('SUCCESS');

    const settling = openSignals({
      tail: { role: 'assistant', time: NOW - 30_000, isError: false, toolNames: [] },
    });
    expect(resolveGlyphMood(signals({ open: settling })).mood).toBe('HAPPY');

    const cooled = openSignals({
      tail: { role: 'assistant', time: NOW - 120_000, isError: false, toolNames: [] },
    });
    expect(resolveGlyphMood(signals({ open: cooled })).mood).not.toBe('SUCCESS');
    expect(resolveGlyphMood(signals({ open: cooled })).mood).not.toBe('HAPPY');
  });

  it('warns about stale background failures', () => {
    const wary = openSignals({ staleFailedTasks: 1 });
    expect(resolveGlyphMood(signals({ open: wary })).mood).toBe('WARNING');
  });

  it('goes FOCUSED for long bursts, plan mode, and active goals', () => {
    const long = openSignals({ liveSince: NOW - 120_000 });
    expect(resolveGlyphMood(signals({ open: long })).mood).toBe('FOCUSED');

    const planning = openSignals({ planModeActive: true });
    expect(resolveGlyphMood(signals({ open: planning })).mood).toBe('FOCUSED');

    const goal = openSignals({ goalActive: true });
    expect(resolveGlyphMood(signals({ open: goal })).mood).toBe('FOCUSED');
  });

  it('classifies live tool work as BUILDING or ANALYZING, else THINKING', () => {
    const shelling = openSignals({
      tail: { role: 'assistant', time: NOW - 1_000, isError: false, toolNames: ['Bash'] },
    });
    expect(resolveGlyphMood(signals({ open: shelling })).mood).toBe('BUILDING');

    const scanning = openSignals({
      tail: { role: 'assistant', time: NOW - 1_000, isError: false, toolNames: ['Grep', 'Read'] },
    });
    expect(resolveGlyphMood(signals({ open: scanning })).mood).toBe('ANALYZING');

    const pondering = openSignals({
      tail: { role: 'user', time: NOW - 1_000, isError: false, toolNames: [] },
    });
    expect(resolveGlyphMood(signals({ open: pondering })).mood).toBe('THINKING');
  });

  it('watches background agents while idle, then falls asleep', () => {
    const watching = signals({
      newestUpdatedAt: NOW - 120_000,
      open: openSignals({ updatedAt: NOW - 120_000, runningAgentTasks: 2 }),
    });
    expect(resolveGlyphMood(watching).mood).toBe('STEALTH');

    const asleep = signals({
      newestUpdatedAt: NOW - 11 * 60_000,
      open: openSignals({ updatedAt: NOW - 11 * 60_000 }),
    });
    expect(resolveGlyphMood(asleep).mood).toBe('SLEEPY');
  });
});

describe('snapshot builders', () => {
  it('flattens the tail message with tool names', () => {
    const tail = buildTailMessage({
      lineNo: 12,
      time: NOW - 1_000,
      source: 'append_message',
      message: {
        role: 'assistant',
        content: [],
        toolCalls: [{ type: 'function', id: 't1', name: 'Grep', arguments: null }],
      },
      toolStepUuids: [],
    });
    expect(tail).toEqual({
      role: 'assistant',
      time: NOW - 1_000,
      isError: false,
      toolNames: ['Grep'],
    });
  });

  it('folds sessions, tasks, context, and wire tail into open signals', () => {
    const summary: SessionSummary = {
      sessionId: 's1',
      sessionDir: '/tmp/s1',
      workDir: '/tmp',
      title: null,
      lastPrompt: null,
      isCustomTitle: false,
      createdAt: NOW - 60_000,
      updatedAt: NOW - 5_000,
      agentCount: 2,
      mainAgentExists: true,
      mainWireRecordCount: 10,
      wireProtocolVersion: '2.0',
      health: 'ok',
      imported: false,
      importMeta: null,
    };
    const detail: SessionDetail = {
      sessionId: 's1',
      sessionDir: '/tmp/s1',
      workDir: '/tmp',
      state: {},
      agents: [
        {
          agentId: 'main',
          type: 'main',
          parentAgentId: null,
          profileName: null,
          homedir: '/tmp/s1/agents/main',
          wireExists: true,
          wireRecordCount: 10,
          wireProtocolVersion: '2.0',
          swarmItem: null,
        },
      ],
      imported: false,
      importMeta: null,
    };
    const tasks: BackgroundTasksResponse = {
      sessionId: 's1',
      tasks: [
        {
          task: {
            taskId: 'p1',
            description: 'build',
            status: 'running',
            startedAt: NOW - 2_000,
            endedAt: null,
            kind: 'process',
            command: 'pnpm build',
            pid: 4242,
            exitCode: null,
          },
          agentId: 'main',
          outputSizeBytes: 0,
          outputExists: false,
        },
        {
          task: {
            taskId: 'p2',
            description: 'flaky',
            status: 'failed',
            startedAt: NOW - 600_000,
            endedAt: NOW - 200_000,
            kind: 'process',
            command: 'npm test',
            pid: 4243,
            exitCode: 1,
          },
          agentId: 'main',
          outputSizeBytes: 0,
          outputExists: false,
        },
      ],
    };
    const context = {
      sessionId: 's1',
      agentId: 'main',
      messages: [],
      usage: {},
      contextTokens: 0,
      config: {},
      permission: { mode: 'manual' },
      planMode: { active: false },
      goal: null,
      swarm: { active: false },
    } as unknown as ContextResponse;
    const open = buildOpenSignals({
      now: NOW,
      summary,
      detail,
      context,
      tasks,
      wireTail: [{ type: 'interaction.request', time: NOW - 3_000 }],
      liveSince: null,
    });

    expect(open).toEqual(
      openSignals({
        updatedAt: NOW - 5_000,
        agentCount: 1,
        runningProcessCommands: ['pnpm build'],
        staleFailedTasks: 1,
        awaitingInput: true,
      }),
    );
  });
});
