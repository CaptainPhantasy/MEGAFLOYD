import { cp, mkdir, readFile, readdir, rename, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { PluginManager } from '@legacy-ai/agent-core-v2/app/plugin/manager';
import { computeUpdateStatus, parsePluginMarketplace, readPluginMarketplace } from '@legacy-ai/agent-core-v2/app/plugin/marketplace';
import type { McpServerConfig } from '@legacy-ai/agent-core-v2/mcpCore/config-schema';
import type { GoRuntime } from './runtime';
import type { GoWorkspaces } from './workspaces';
import { GoRequestError } from './client';
import { object, text } from './projection';

export const shellQuote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;

export function goMcpConfig(config: McpServerConfig): Record<string, unknown> {
  const common = { type: config.transport, disabled: config.enabled === false, disabled_tools: config.disabledTools, enabled_tools: config.enabledTools, startup_timeout_ms: config.startupTimeoutMs, tool_timeout_ms: config.toolTimeoutMs, deferred: config.deferred };
  if (config.transport === 'stdio') return { ...common, command: config.command, args: config.args, env: config.env, cwd: config.cwd };
  return { ...common, url: config.url, headers: { ...config.headers, ...(config.bearerTokenEnvVar ? { Authorization: `Bearer $${config.bearerTokenEnvVar}` } : {}) }, oauth: config.auth === 'oauth' };
}

export class GoPlugins {
  readonly manager: PluginManager;
  private writes: Promise<unknown> = Promise.resolve();
  constructor(readonly home: string, readonly workspaces: GoWorkspaces, readonly marketplaceUrl: string) { this.manager = new PluginManager({ floydHomeDir: home }); }

  async load(): Promise<void> { await this.manager.load(); }

  async sync(runtime: GoRuntime): Promise<void> {
    const enabled = this.manager.list().filter((plugin) => plugin.enabled && plugin.state === 'ok');
    const runtimeDir = join(this.home, 'plugin-runtime');
    const skillRoot = join(runtimeDir, 'skills');
    const commands = await this.manager.enabledCommands();
    const commandNames = new Set(commands.map((command) => `${command.pluginId}-${command.name}`.replace(/[^A-Za-z0-9_-]/g, '-')));
    let existing: string[] = [];
    try { existing = await readdir(skillRoot); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    for (const name of existing) if (!commandNames.has(name)) {
      const quarantine = join(runtimeDir, 'retired-commands');
      await mkdir(quarantine, { recursive: true });
      await rename(join(skillRoot, name), join(quarantine, `${name}-${Date.now()}`));
    }
    if (enabled.length || commands.length) {
      await mkdir(runtimeDir, { recursive: true });
      const instructions = enabled.map((plugin) => `## ${plugin.manifest?.interface?.displayName || plugin.id}\n${plugin.manifest?.description || ''}${plugin.skillInstructions ? `\n\n${plugin.skillInstructions}` : ''}`).join('\n\n');
      await this.writeGenerated(join(runtimeDir, 'PLUGIN.md'), `---\nname: plugin-runtime\ndescription: Instructions from enabled MegaFloyd plugins\n---\n\n${instructions}\n`);
      for (const command of commands) {
        const name = `${command.pluginId}-${command.name}`.replace(/[^A-Za-z0-9_-]/g, '-');
        const root = join(skillRoot, name);
        await mkdir(root, { recursive: true });
        await this.writeGenerated(join(root, 'SKILL.md'), `---\nname: ${name}\ndescription: ${JSON.stringify(command.description || `Run ${command.name} from ${command.pluginId}`)}\nuser-invocable: true\n---\n\n${command.body}\n`);
      }
    }
    const hooks: Record<string, Record<string, unknown>[]> = {};
    for (const hook of this.manager.enabledHooks()) {
      const env = Object.entries(hook.env ?? {}).map(([key, value]) => shellQuote(`${key}=${value}`));
      const command = `${hook.cwd ? `cd ${shellQuote(hook.cwd)} && ` : ''}env ${env.join(' ')} sh -c ${shellQuote(hook.command)}`;
      (hooks[hook.event] ??= []).push({ command, matcher: hook.matcher, timeout: hook.timeout });
    }
    await runtime.client.json('POST', `${runtime.base}/config/plugin-reload`, { skill_paths: [...this.manager.pluginSkillRoots().map((root) => root.path), ...(commands.length ? [skillRoot] : [])], plugin_paths: enabled.length ? [runtimeDir] : [], agent_roots: this.manager.pluginAgentRoots().map((root) => root.path), system_prompts: this.manager.enabledSystemPrompts(), session_starts: this.manager.enabledSessionStarts(), hooks, mcp: Object.fromEntries(Object.entries(this.manager.enabledMcpServers()).map(([name, config]) => [name, goMcpConfig(config)])) });
    runtime.markDirty();
  }

  private async writeGenerated(path: string, content: string): Promise<void> {
    try { if (await readFile(path, 'utf8') === content) return; await cp(path, `${path}.bak-${Date.now()}`); }
    catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
    await writeFile(`${path}.tmp`, content, 'utf8');
    await rename(`${path}.tmp`, path);
  }

  private async mutation<T>(operation: () => Promise<T>): Promise<T> {
    const pending = this.writes.then(async () => {
      const attached = await this.workspaces.all();
      for (const { runtime } of attached) if ((await runtime.info()).is_busy) throw new GoRequestError(409, 'Wait for the shared Go agents to finish before changing plugins');
      const directory = join(this.home, 'plugins');
      try { await cp(directory, `${directory}.bak-${Date.now()}`, { recursive: true, errorOnExist: true, force: false }); }
      catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
      const result = await operation();
      for (const { runtime } of attached) await this.sync(runtime);
      return result;
    });
    this.writes = pending.catch(() => {});
    return pending;
  }

  async setMcpEnabled(pluginId: string, server: string, enabled: boolean): Promise<void> { await this.mutation(() => this.manager.setMcpServerEnabled(pluginId, server, enabled)); }

  async route(runtime: GoRuntime, path: string, method: string, body: Record<string, unknown>): Promise<{ data: unknown } | undefined> {
    if (method === 'GET' && path === '/plugins') return { data: { plugins: this.manager.summaries() } };
    if (method === 'GET' && path === '/plugins/marketplace') {
      const configured = this.marketplaceUrl || text(object((await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`))['options'])['plugin_marketplace_url']);
      const read = await readPluginMarketplace({ source: configured, workDir: runtime.cwd, fetchImpl: (url, init) => fetch(url, { ...init, signal: AbortSignal.timeout(10_000) }) });
      const catalog = parsePluginMarketplace(read.raw, read.location);
      const installed = new Map(this.manager.summaries().map((plugin) => [plugin.id, plugin]));
      return { data: { entries: catalog.plugins.map((entry) => { const existing = installed.get(entry.id); return { ...entry, tier: entry.tier || 'third-party', installed: existing ? { enabled: existing.enabled, version: existing.version } : undefined, updateAvailable: computeUpdateStatus(entry.version, existing?.version, Boolean(existing)).kind === 'update' }; }) } };
    }
    if (method === 'POST' && (path === '/plugins' || path === '/plugins/install-all')) {
      const source = text(body['source']);
      if (!source.trim()) throw new GoRequestError(400, 'Plugin source is required');
      return { data: await this.mutation(async () => {
        const result = await this.manager.installAll(source);
        const summaries = new Map(this.manager.summaries().map((plugin) => [plugin.id, plugin]));
        if (path === '/plugins' && !result.installed.length) throw new GoRequestError(400, 'The source did not contain an installable plugin');
        return path === '/plugins' ? summaries.get(result.installed[0]!.id) : { installed: result.installed.map((plugin) => summaries.get(plugin.id)), skipped: result.skipped, report: result.report };
      }) };
    }
    const action = /^\/plugins\/([^:]+):(enable|disable|remove|reload)$/.exec(path);
    if (method === 'POST' && action) {
      await this.mutation(async () => {
        if (!this.manager.get(action[1]!)) throw new GoRequestError(404, 'Plugin is not installed');
        if (action[2] === 'remove') await this.manager.remove(action[1]!);
        else if (action[2] === 'reload') await this.manager.reload();
        else await this.manager.setEnabled(action[1]!, action[2] === 'enable');
      });
      return { data: { ok: true } };
    }
    if (method === 'GET' && /^\/plugins\/[^/]+$/.test(path)) {
      const plugin = this.manager.info(path.slice(9));
      if (!plugin) throw new GoRequestError(404, 'Plugin is not installed');
      return { data: plugin };
    }
    return undefined;
  }
}
