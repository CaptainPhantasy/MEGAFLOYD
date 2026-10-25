import { randomUUID } from 'node:crypto';
import { McpServerConfigSchema, type McpServerConfig } from '@legacy-ai/agent-core-v2/mcpCore/config-schema';
import type { GoRuntime } from './runtime';
import { GoRequestError } from './client';
import { array, object, text } from './projection';
import { goMcpConfig, type GoPlugins } from './plugins';

type Locator = { source: 'global'; name: string } | { source: 'plugin'; pluginId: string; serverName: string };
interface Entry { name: string; config: McpServerConfig; raw: Record<string, unknown>; source: 'global' | 'plugin'; origin: string; mutable: boolean; locator: Locator; plugin?: { id: string; name: string } }
interface Flow { runtime: GoRuntime; name: string; controller: AbortController; done: Promise<void>; settled: boolean; error?: unknown }

export class GoMcp {
  private readonly flows = new Map<string, Flow>();
  private writes: Promise<unknown> = Promise.resolve();
  constructor(readonly plugins: GoPlugins) {}

  private async entries(runtime: GoRuntime): Promise<Entry[]> {
    const config = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
    const pluginEntries = this.plugins.manager.mcpServerEntries();
    const pluginNames = new Set(pluginEntries.map((entry) => entry.name));
    const entries: Entry[] = Object.entries(object(config['mcp'])).filter(([name, raw]) => !pluginNames.has(name) && (object(raw)['command'] || object(raw)['url'])).map(([name, raw]) => ({ name, config: wireConfig(object(raw)), raw: object(raw), source: 'global', origin: 'Go configuration', mutable: true, locator: { source: 'global', name } }));
    for (const entry of pluginEntries) entries.push({ name: entry.name, config: entry.config, raw: { ...goMcpConfig(entry.config), ...object(object(config['mcp'])[entry.name]) }, source: 'plugin', origin: entry.pluginId, mutable: false, locator: { source: 'plugin', pluginId: entry.pluginId, serverName: entry.serverName }, plugin: { id: entry.pluginId, name: this.plugins.manager.info(entry.pluginId)?.displayName || entry.pluginId } });
    return entries;
  }

  private display(entry: Entry) {
    const config = entry.config.transport === 'stdio' ? { ...entry.config, env: undefined, envKeys: Object.keys(entry.config.env ?? {}) } : { ...entry.config, headers: undefined, headerKeys: Object.keys(entry.config.headers ?? {}) };
    return { name: entry.name, config, source: entry.source, origin: entry.origin, mutable: entry.mutable, plugin: entry.plugin };
  }

  private locate(entries: Entry[], body: Record<string, unknown>): Entry {
    const entry = entries.find((item) => body['source'] === 'plugin' ? item.locator.source === 'plugin' && item.locator.pluginId === body['pluginId'] && item.locator.serverName === body['serverName'] : item.name === body['name'] && item.source === 'global');
    if (!entry) throw new GoRequestError(404, 'MCP server was not found');
    return entry;
  }

  private async inspect(runtime: GoRuntime, entries: Entry[], verify: boolean) {
    const probes = new Map<string, { success: boolean; output: string }>();
    if (verify) for (const entry of entries) if (entry.config.enabled !== false && entry.config.transport !== 'stdio' && entry.config.auth === 'oauth') probes.set(entry.name, await runtime.client.json('POST', `${runtime.base}/mcp/test`, { name: entry.name }, { timeoutMs: 120_000 }));
    const states = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/mcp/states`);
    return entries.map((entry) => {
      const state = object(states[entry.name]);
      const token = object(entry.raw['oauth_token']);
      const expires = Number(token['expires_at']) || 0;
      const probe = probes.get(entry.name);
      const connected = probe ? probe.success : state['state'] === 'connected' || state['state'] === 2;
      const authStatus = probe && !probe.success ? 'unavailable' : entry.config.transport === 'stdio' ? 'not-applicable' : entry.config.auth !== 'oauth' && state['state'] !== 'needs auth' ? entry.config.bearerTokenEnvVar || Object.keys(entry.config.headers ?? {}).some((key) => key.toLowerCase() === 'authorization') ? 'bearer-token' : 'not-applicable' : connected ? 'oauth-authorized' : token['access_token'] && expires && expires < Date.now() / 1000 ? 'oauth-expired' : state['state'] === 'error' || state['state'] === 3 ? 'unavailable' : 'oauth-required';
      return { serverId: entry.name, locator: entry.locator, runtimeName: entry.name, canonicalUrl: entry.config.transport === 'stdio' ? undefined : entry.config.url, origin: entry.source, config: this.display(entry).config, enabled: entry.config.enabled !== false, editable: entry.mutable, authStatus, checkedAt: state['connected_at'] ? Date.parse(text(state['connected_at'])) : undefined, error: probe && !probe.success ? probe.output : text(state['error']) || undefined };
    });
  }

  private async mutate<T>(operation: () => Promise<T>): Promise<T> {
    const next = this.writes.then(operation);
    this.writes = next.catch(() => {});
    return next;
  }

  async route(runtime: GoRuntime, path: string, method: string, body: Record<string, unknown>, query: URLSearchParams): Promise<{ data: unknown } | undefined> {
    if (!path.startsWith('/mcp/')) return undefined;
    const server = /^\/mcp\/servers\/([^/]+)$/.exec(path);
    if (method === 'GET' && (path === '/mcp/servers' || server)) {
      const entries = await this.entries(runtime);
      if (!server) return { data: entries.map((entry) => this.display(entry)) };
      const entry = entries.find((item) => item.name === server[1]);
      if (!entry) throw new GoRequestError(404, 'MCP server was not found');
      return { data: this.display(entry) };
    }
    if ((method === 'POST' && path === '/mcp/servers') || (server && ['PUT', 'DELETE'].includes(method))) {
      return { data: await this.mutate(async () => {
        const entries = await this.entries(runtime);
        const name = server?.[1] || text(body['name']);
        if (!name || name.includes('\0')) throw new GoRequestError(400, 'MCP server name must be nonempty and contain no null characters');
        const previous = entries.find((entry) => entry.name === name);
        if (previous?.source === 'plugin') throw new GoRequestError(409, 'Change a plugin MCP server through the installed plugin settings');
        if (method === 'POST' && previous) throw new GoRequestError(409, 'MCP server already exists');
        if (method !== 'POST' && !previous) throw new GoRequestError(404, 'MCP server was not found');
        if (method === 'DELETE') await runtime.client.json('POST', `${runtime.base}/config/remove`, { scope: 0, key: `mcp.${name.replaceAll('\\', '\\\\').replaceAll('.', '\\.')}` });
        else {
          const config = McpServerConfigSchema.parse(body);
          const next = goMcpConfig(config);
          if (previous) { if (!('env' in body) && previous.raw['env']) next['env'] = previous.raw['env']; if (!('headers' in body) && previous.raw['headers']) next['headers'] = previous.raw['headers']; }
          await runtime.client.json('POST', `${runtime.base}/config/set`, { scope: 0, key: `mcp.${name.replaceAll('\\', '\\\\').replaceAll('.', '\\.')}`, value: { ...previous?.raw, ...next } });
        }
        runtime.markDirty();
        return (await this.entries(runtime)).map((entry) => this.display(entry));
      }) };
    }
    if (method === 'POST' && path === '/mcp/servers::test') {
      if (!body['server'] && !text(body['name'])) throw new GoRequestError(400, 'Choose an MCP server or supply a server configuration');
      return { data: await runtime.client.json('POST', `${runtime.base}/mcp/test`, { name: body['name'] || object(body['server'])['name'], config: body['server'] ? goMcpConfig(McpServerConfigSchema.parse(body['server'])) : undefined }, { timeoutMs: 120_000 }) };
    }
    if ((method === 'POST' && path === '/mcp/servers::inspect') || (method === 'GET' && path === '/mcp/auth-statuses')) {
      const entries = await this.entries(runtime);
      const selected = body['targets'] === undefined ? entries : array(body['targets']).map((target) => this.locate(entries, object(target)));
      const inspected = await this.inspect(runtime, selected, path.endsWith('::inspect') || query.get('verify') === 'true');
      return { data: path.endsWith('/auth-statuses') ? inspected.map((entry) => ({ name: entry.runtimeName, authStatus: entry.authStatus })) : inspected };
    }
    if (method === 'POST' && path === '/mcp/auth::begin') {
      const entry = this.locate(await this.entries(runtime), body);
      const inspection = (await this.inspect(runtime, [entry], false))[0]!;
      if (inspection.authStatus === 'oauth-authorized') return { data: { status: 'already-authorized' } };
      const id = randomUUID();
      const controller = new AbortController();
      const flow: Flow = { runtime, name: entry.name, controller, done: Promise.resolve(), settled: false };
      flow.done = runtime.client.json('POST', `${runtime.base}/mcp/auth`, { name: entry.name }, { signal: controller.signal, timeoutMs: 10 * 60_000 }).then(() => { flow.settled = true; }, (error: unknown) => { flow.error = error; flow.settled = true; });
      this.flows.set(id, flow);
      for (let attempt = 0; attempt < 50; attempt++) {
        if (flow.settled) { this.flows.delete(id); if (flow.error) throw flow.error; return { data: { status: 'already-authorized' } }; }
        const current = await runtime.client.json<{ auth_url?: string }>('GET', `${runtime.base}/mcp/auth-url?name=${encodeURIComponent(entry.name)}`);
        if (current.auth_url) return { data: { status: 'authorization-required', flowId: id, authorizationUrl: current.auth_url } };
        await new Promise((resolve) => setTimeout(resolve, 100));
      }
      controller.abort();
      this.flows.delete(id);
      throw new GoRequestError(504, 'The MCP server did not provide an authorization URL');
    }
    if (method === 'POST' && ['/mcp/auth::complete', '/mcp/auth::cancel'].includes(path)) {
      const id = text(body['flowId']);
      const flow = this.flows.get(id);
      if (!flow && path.endsWith('::cancel')) return { data: null };
      if (!flow) throw new GoRequestError(400, 'MCP authorization flow was not found');
      if (path.endsWith('::cancel')) { flow.controller.abort(); this.flows.delete(id); return { data: null }; }
      const timeout = Math.min(2 ** 31 - 1, Math.max(1, Number(body['timeoutMs']) || 120_000));
      let timer: ReturnType<typeof setTimeout> | undefined;
      try {
        await Promise.race([flow.done, new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new GoRequestError(408, 'MCP authorization is still pending')), timeout); })]);
        this.flows.delete(id);
        if (flow.error) throw flow.error;
        runtime.markDirty();
        return { data: null };
      } finally { clearTimeout(timer); }
    }
    if (method === 'POST' && path === '/mcp/auth::reset') {
      const entry = this.locate(await this.entries(runtime), body);
      await runtime.client.json('POST', `${runtime.base}/mcp/auth-reset`, { name: entry.name });
      runtime.markDirty();
      return { data: null };
    }
    return undefined;
  }

  close(): void { for (const flow of this.flows.values()) flow.controller.abort(); this.flows.clear(); }
}

function wireConfig(raw: Record<string, unknown>): McpServerConfig {
  const common = { enabled: raw['disabled'] !== true, deferred: raw['deferred'] === true, startupTimeoutMs: Number(raw['startup_timeout_ms']) || (Number(raw['timeout']) || 10) * 1000, toolTimeoutMs: Number(raw['tool_timeout_ms']) || undefined, enabledTools: array(raw['enabled_tools']).map(text), disabledTools: array(raw['disabled_tools']).map(text) };
  if (raw['type'] === 'http' || raw['type'] === 'sse') return { ...common, transport: raw['type'], url: text(raw['url']), headers: object(raw['headers']) as Record<string, string>, auth: raw['oauth'] ? 'oauth' : undefined };
  return { ...common, transport: 'stdio', command: text(raw['command']), args: array(raw['args']).map(text), env: object(raw['env']) as Record<string, string>, cwd: text(raw['cwd']) || undefined };
}
