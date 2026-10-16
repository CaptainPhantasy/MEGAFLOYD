import { useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { matchPath, useLocation } from 'react-router-dom';
import { api } from '../api';
import type { SessionSummary, WireResponse } from '../types';
import { GLYPH_MOOD_SPECS, type GlyphMoodSpec } from '../lib/glyph';
import {
  GLYPH_WINDOWS,
  buildOpenSignals,
  resolveGlyphMood,
  type GlyphWireTailRecord,
} from '../lib/glyph-mood';

const TICK_MS = 1000;
const SESSIONS_POLL_MS = 5000;
const CONTEXT_POLL_MS = 8000;
const TASKS_POLL_MS = 8000;
const DETAIL_POLL_MS = 15_000;
const WIRE_TAIL_RECORDS = 40;

export interface GlyphMoodState {
  spec: GlyphMoodSpec;
  reason: string;
}

/** Keeps the glyph alive: the harness state it reads is only as fresh as the
 *  polls below. Intervals live on this observer; other observers of the same
 *  query keys keep their own (default: never refetch) behaviour. */
export function useGlyphMood(): GlyphMoodState {
  const qc = useQueryClient();
  const location = useLocation();
  const sessionId =
    matchPath('/sessions/:sessionId', location.pathname)?.params.sessionId ??
    matchPath('/sessions/:sessionId/agents/:agentId', location.pathname)?.params.sessionId;

  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), TICK_MS);
    return () => window.clearInterval(id);
  }, []);

  const sessionsQuery = useQuery({
    queryKey: ['sessions'] as const,
    queryFn: () => api.listSessions(),
    refetchInterval: SESSIONS_POLL_MS,
  });
  const detailQuery = useQuery({
    queryKey: ['session', sessionId] as const,
    queryFn: () => api.getSession(sessionId!),
    enabled: !!sessionId,
    refetchInterval: DETAIL_POLL_MS,
  });
  const mainAgentId = detailQuery.data?.agents.find((agent) => agent.type === 'main')?.agentId;
  const contextQuery = useQuery({
    queryKey: ['context', sessionId, mainAgentId, 'model'] as const,
    queryFn: () => api.getContext(sessionId!, mainAgentId!, 'model'),
    enabled: !!sessionId && !!mainAgentId,
    refetchInterval: CONTEXT_POLL_MS,
  });
  const tasksQuery = useQuery({
    queryKey: ['tasks', sessionId] as const,
    queryFn: () => api.getTasks(sessionId!),
    enabled: !!sessionId,
    refetchInterval: TASKS_POLL_MS,
  });

  // The wire is only fetched when the user opens the Wire tab; read it from
  // the cache without triggering a fetch and treat it as a bonus signal.
  const wireTail: GlyphWireTailRecord[] | undefined =
    sessionId && mainAgentId
      ? (qc
          .getQueryData<WireResponse>(['session', sessionId, 'wire', mainAgentId])
          ?.records.slice(-WIRE_TAIL_RECORDS)
          .map((entry) => entry.data) ?? undefined)
      : undefined;

  // Track the current live burst so FOCUSED can kick in after sustained work.
  const summary: SessionSummary | undefined = sessionId
    ? sessionsQuery.data?.find((session) => session.sessionId === sessionId)
    : undefined;
  const [liveSince, setLiveSince] = useState<number | null>(null);
  const lastActiveRef = useRef(0);
  useEffect(() => {
    const live = summary ? now - summary.updatedAt < GLYPH_WINDOWS.active : false;
    if (!live) {
      if (now - lastActiveRef.current >= GLYPH_WINDOWS.active) setLiveSince(null);
      return;
    }
    if (lastActiveRef.current === 0 || now - lastActiveRef.current >= GLYPH_WINDOWS.active) {
      setLiveSince(now);
    }
    lastActiveRef.current = now;
  }, [now, summary, lastActiveRef]);

  const signals = {
    now,
    sessionCount: sessionsQuery.data?.length ?? 0,
    newestUpdatedAt:
      sessionsQuery.data?.reduce((newest, session) => Math.max(newest, session.updatedAt), 0) ||
      null,
    open: buildOpenSignals({
      now,
      summary,
      detail: detailQuery.data,
      context: contextQuery.data,
      tasks: tasksQuery.data,
      wireTail,
      liveSince,
    }),
  };
  const result = resolveGlyphMood(signals);
  return { spec: GLYPH_MOOD_SPECS[result.mood], reason: result.reason };
}
