import { createServer } from 'node:http';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { expect, test } from 'vitest';
import { GoRuntime } from '../src/goRuntime/runtime';
import type { GoAgentInfo } from '../src/goRuntime/client';

test('canonical model switching applies destination defaults and explicit reasoning without generation', async () => {
  const dir = await mkdtemp(join(tmpdir(), 'megafloyd-model-defaults-'));
  const socket = join(dir, 'go.sock');
  const models = [
    { id: 'beam', context_window: 200000, default_max_tokens: 131072, can_reason: true, reasoning_levels: ['low', 'medium', 'high'] },
    { id: 'plain', context_window: 200000, default_max_tokens: 8192, can_reason: false, reasoning_levels: [] },
    { id: 'reasoning', context_window: 200000, default_max_tokens: 32768, can_reason: true, reasoning_levels: ['low', 'medium', 'high'] },
    { id: 'bounded', context_window: 200000, default_max_tokens: 4096, can_reason: true, reasoning_levels: ['low', 'medium'] },
    { id: 'boolean', context_window: 200000, default_max_tokens: 16384, can_reason: true, reasoning_levels: [] },
  ];
  let selected: GoAgentInfo['model_cfg'] = { provider: 'fixture', model: 'beam', max_tokens: 131072, reasoning_effort: 'medium' };
  const writes: Array<{ scope: number; model: GoAgentInfo['model_cfg'] }> = [];
  let generationRequests = 0;
  const server = createServer(async (req, res) => {
    const path = new URL(req.url!, 'http://fixture').pathname;
    let raw = ''; for await (const chunk of req) raw += chunk;
    const body = raw ? JSON.parse(raw) : {};
    const send = (value: unknown) => { res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify(value)); };
    if (path.endsWith('/agent') && req.method === 'GET') return send({ is_ready: true, is_busy: false, model_cfg: selected, model: models.find((model) => model.id === selected.model) });
    if (path.endsWith('/config')) return send({ providers: { fixture: { type: 'openai-compat', models } } });
    if (path.endsWith('/config/model')) { selected = body.model; writes.push(body); return send({}); }
    if (path.endsWith('/agent/update')) return send({});
    if (path.endsWith('/sessions/existing-session')) return send({ id: 'existing-session', title: 'Existing conversation', created_at: 1720000000, updated_at: 1720000000, is_busy: false, message_count: 0, prompt_tokens: 0, completion_tokens: 0, cost: 0 });
    if (path.endsWith('/messages')) return send([]);
    if (path.endsWith('/interactions')) return send({ permissions: [], questions: [] });
    if (path.endsWith('/permissions/mode')) return send({ mode: 'manual' });
    if (path.endsWith('/tasks')) return send({ items: [] });
    if (path.endsWith('/plan') || path.endsWith('/swarm')) return send({ active: false });
    if (path.endsWith('/goal')) return send(null);
    if (path.endsWith('/queue')) return send({ items: [], active: null, count: 0 });
    if (path.startsWith('/v1/clients/') && req.method === 'DELETE') return send({});
    generationRequests++; res.statusCode = 404; send({ error: 'Unexpected provider/generation route: ' + path });
  });
  await new Promise<void>((resolve) => server.listen(socket, resolve));
  const runtime = new GoRuntime(`unix://${socket}`, dir, join(dir, 'data'));
  runtime.workspace = { id: 'fixture-workspace', path: dir, data_dir: join(dir, 'data') };
  const assertThinking = async (expected: string) => {
    const [status, view] = await Promise.all([runtime.status('existing-session'), runtime.view('existing-session')]);
    expect([status.thinking_level, view.wire.agent_config.thinking, view.snapshot.meta?.agent?.thinkingEffort]).toEqual([expected, expected, expected]);
    return view.wire.agent_config.thinking;
  };
  try {
    await runtime.configure({ model: 'fixture/beam', thinking: 'medium' }, 'existing-session');
    expect(writes).toHaveLength(0);
    expect(selected).toEqual({ provider: 'fixture', model: 'beam', max_tokens: 131072, reasoning_effort: 'medium' });
    await assertThinking('medium');
    expect((await runtime.catalog()).models.find((model) => model.model === 'fixture/reasoning')?.default_effort).toBe('high');
    await runtime.configure({ model: 'fixture/plain', thinking: 'medium' }, 'existing-session');
    expect(selected).toEqual({ provider: 'fixture', model: 'plain', max_tokens: 8192, reasoning_effort: '', think: false });
    expect(writes.at(-1)?.scope).toBe(1);
    await assertThinking('off');
    await runtime.configure({ model: 'fixture/reasoning' });
    expect(selected).toEqual({ provider: 'fixture', model: 'reasoning', max_tokens: 32768, reasoning_effort: 'high', think: false });
    expect(writes.at(-1)?.scope).toBe(0);
    await assertThinking('high');
    await runtime.configure({ thinking: 'low' });
    expect(selected.reasoning_effort).toBe('low');
    expect(selected.max_tokens).toBe(32768);
    await runtime.configure({ thinking: 'off' });
    expect(selected.reasoning_effort).toBe('none'); expect(selected.think).toBe(false);
    const count = writes.length;
    await runtime.configure({ thinking: await assertThinking('off') }); expect(writes).toHaveLength(count);
    await runtime.configure({ model: 'fixture/bounded' });
    expect(selected).toEqual({ provider: 'fixture', model: 'bounded', max_tokens: 4096, reasoning_effort: 'medium', think: false });
    await expect(runtime.configure({ thinking: 'high' })).rejects.toThrow('does not support');
    await runtime.configure({ model: 'fixture/reasoning', thinking: 'off' });
    expect(selected).toEqual({ provider: 'fixture', model: 'reasoning', max_tokens: 32768, reasoning_effort: 'none', think: false });
    await assertThinking('off');
    await runtime.configure({ model: 'fixture/bounded', thinking: 'low' });
    expect(selected).toEqual({ provider: 'fixture', model: 'bounded', max_tokens: 4096, reasoning_effort: 'low', think: false });
    const writesBeforeInvalid = writes.length;
    await expect(runtime.configure({ model: 'fixture/boolean', thinking: 'high' })).rejects.toThrow('does not support');
    expect(writes).toHaveLength(writesBeforeInvalid);
    expect(selected.model).toBe('bounded');
    await runtime.configure({ model: 'fixture/boolean' });
    expect(selected).toEqual({ provider: 'fixture', model: 'boolean', max_tokens: 16384, reasoning_effort: '', think: true });
    await assertThinking('on');
    await runtime.configure({ thinking: 'off' });
    await assertThinking('off');
    await runtime.configure({ thinking: 'on' });
    await assertThinking('on');
    for (const sentinel of ['none', 'off', 'disabled']) {
      selected = { ...selected, reasoning_effort: sentinel, think: false };
      await assertThinking('off');
    }
    expect(generationRequests).toBe(0);
  } finally {
    await runtime.close();
    await new Promise<void>((resolve) => server.close(() => resolve()));
    await rm(dir, { recursive: true, force: true });
  }
});
