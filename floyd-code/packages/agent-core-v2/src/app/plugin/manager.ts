import { cp, mkdir, mkdtemp, readdir, realpath, rename, rm, stat } from 'node:fs/promises';
import { existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';

import type { HookDef } from '#/features/externalHooks/internal/types';
import { discoverFileSkills } from '#/features/skill/catalog/fileSkillDiscovery';
import type { SkillDiscoveryResult } from '#/features/skill/catalog/skillDiscovery';
import type { SkillRoot } from '#/features/skill/catalog/types';
import { BugIndicatingError, Error2, ErrorCodes, PluginErrors } from '#/errors';
import type { McpServerConfig } from '#/mcpCore/config-schema';

import { downloadZip, extractZip } from './archive';
import {
  collectLoosePieces,
  detectForeignPlugin,
  sanitizePluginName,
  selectClaudePackEntries,
  setPluginNameInManifest,
  synthesizeFromClaudePlugin,
  synthesizeFromFolders,
  synthesizeFromGeminiExtension,
  synthesizeFromLoosePieces,
  synthesizeFromPackEntry,
  type SynthOutcome,
} from './adapt';
import { loadPluginCommand } from './commands';
import { resolveGithubCommitSha, resolveGithubSource } from './github-resolver';
import { parseManifest, type ParsedManifestResult } from './manifest';
import { resolveInstallSource } from './source';
import { readInstalled, writeInstalled, type InstalledRecord } from './store';
import type { PluginAgentRoot } from './types';
import {
  normalizePluginId,
  type EnabledPluginSessionStart,
  type EnabledPluginSystemPrompt,
  type PluginAdaptedFrom,
  type PluginCapabilityState,
  type PluginCommandDef,
  type PluginDiagnostic,
  type PluginGithubMetadata,
  type PluginInfo,
  type PluginInstallSkip,
  type PluginMcpServerEntry,
  type PluginMcpServerInfo,
  type PluginRecord,
  type PluginSource,
  type PluginSummary,
  type PluginUpdateStatus,
  type ReloadSummary,
} from './types';

export interface PluginManagerOptions {
  readonly floydHomeDir: string;
  readonly discoverSkills?: (roots: readonly SkillRoot[]) => Promise<SkillDiscoveryResult>;
}

interface ManagedPluginCopy {
  readonly root: string;
  readonly previousRoot?: string;
}

export interface PluginManagerInstallAllResult {
  readonly installed: readonly PluginRecord[];
  readonly skipped: readonly PluginInstallSkip[];
  readonly report?: string;
}

export class PluginManager {
  private readonly floydHomeDir: string;
  private readonly discoverSkills: (roots: readonly SkillRoot[]) => Promise<SkillDiscoveryResult>;
  private records = new Map<string, PluginRecord>();

  constructor(options: PluginManagerOptions) {
    this.floydHomeDir = options.floydHomeDir;
    this.discoverSkills = options.discoverSkills ?? discoverFileSkills;
  }

  async load(): Promise<void> {
    const file = await readInstalled(this.floydHomeDir);
    const next = new Map<string, PluginRecord>();
    for (const entry of file.plugins) {
      next.set(entry.id, await this.materialize(entry));
    }
    this.records = next;
  }

  list(): readonly PluginRecord[] {
    return [...this.records.values()].toSorted((a, b) => a.id.localeCompare(b.id));
  }

  get(id: string): PluginRecord | undefined {
    return this.records.get(normalizePluginId(id));
  }

  async install(source: string): Promise<PluginRecord> {
    const result = await this.installAll(source);
    const first = result.installed[0];
    if (first === undefined) {
      throw new Error2(ErrorCodes.PLUGIN_LOAD_FAILED, `Nothing was installed from ${source}`);
    }
    return first;
  }

  async installAll(source: string): Promise<PluginManagerInstallAllResult> {
    const trimmed = source.trim();
    const { base, fragment } = splitSourceFragment(trimmed);
    const resolved = resolveInstallSource(base);

    let sourceType: PluginSource;
    let sourceRoot: string;
    let localRealRoot: string | undefined;
    let zipTmpDir: string | undefined;
    let stagedDir: string | undefined;
    let github: PluginGithubMetadata | undefined;
    const installed: PluginRecord[] = [];
    const skipped: PluginInstallSkip[] = [];

    try {
      if (resolved.kind === 'local-path') {
        sourceType = 'local-path';
        sourceRoot = await normalizeInstallRoot(resolved.path);
        localRealRoot = sourceRoot;
      } else {
        sourceType = resolved.kind === 'github' ? 'github' : 'zip-url';
        const zipUrl =
          resolved.kind === 'github'
            ? await (async () => {
                const resolution = await resolveGithubSource(resolved);
                const installedSha = await installedGithubSha(
                  resolved.owner,
                  resolved.repo,
                  resolution.ref,
                );
                github = {
                  owner: resolved.owner,
                  repo: resolved.repo,
                  ref: resolution.ref,
                  installedSha,
                };
                if (installedSha !== undefined) {
                  return `https://codeload.github.com/${resolved.owner}/${resolved.repo}/zip/${installedSha}`;
                }
                return resolution.tarballUrl;
              })()
            : resolved.path;
        const buffer = await downloadZip(zipUrl);
        zipTmpDir = await mkdtemp(path.join(tmpdir(), 'floyd-plugin-zip-'));
        sourceRoot = await extractZip(buffer, zipTmpDir);
      }

      const missing = (await parseManifest(sourceRoot)).diagnostics.find(
        (d) => d.severity === 'error',
      );
      if (missing === undefined) {
        installed.push(
          await this.publishRecord({
            root: sourceRoot,
            sourceType,
            originalSource: trimmed,
            github,
          }),
        );
        return { installed, skipped };
      }

      const foreign = await detectForeignPlugin(sourceRoot);
      if (sourceType === 'local-path') {
        stagedDir = await mkdtemp(path.join(tmpdir(), 'floyd-plugin-stage-'));
        const staged = path.join(stagedDir, 'plugin');
        await cp(sourceRoot, staged, { recursive: true });
        sourceRoot = staged;
      }

      if (foreign?.kind === 'claude-plugin') {
        const synth = await synthesizeFromClaudePlugin(sourceRoot, foreign.path);
        installed.push(
          await this.publishRecord({
            root: sourceRoot,
            sourceType,
            originalSource: trimmed,
            github,
            adaptedFrom: 'claude-code',
            extraDiagnostics: synth.diagnostics,
          }),
        );
        return { installed, skipped };
      }
      if (foreign?.kind === 'gemini-extension') {
        const synth = await synthesizeFromGeminiExtension(sourceRoot, foreign.path);
        installed.push(
          await this.publishRecord({
            root: sourceRoot,
            sourceType,
            originalSource: trimmed,
            github,
            adaptedFrom: 'gemini-cli',
            extraDiagnostics: synth.diagnostics,
          }),
        );
        return { installed, skipped };
      }
      if (foreign?.kind === 'claude-pack') {
        const selection = await selectClaudePackEntries(sourceRoot, foreign.path, fragment);
        skipped.push(...selection.skipped);
        const seen = new Set<string>();
        for (const entry of selection.entries) {
          const synth = await synthesizeFromPackEntry(sourceRoot, entry);
          let name = (await parseManifest(entry.dir)).manifest?.name ?? sanitizePluginName(entry.name);
          if (seen.has(name)) {
            name = sanitizePluginName(`${name}-${nextNameSuffix(name, seen)}`);
            await setPluginNameInManifest(entry.dir, name);
          }
          seen.add(name);
          installed.push(
            await this.publishRecord({
              root: entry.dir,
              sourceType,
              originalSource: `${base}#${entry.name}`,
              github,
              adaptedFrom: 'claude-code-pack',
              extraDiagnostics: synth.diagnostics,
            }),
          );
        }
        if (installed.length === 0) {
          throw new Error2(
            ErrorCodes.PLUGIN_LOAD_FAILED,
            `Cannot install plugin from ${trimmed}: ${missing.message}`,
            { details: { sourceType } },
          );
        }
        return { installed, skipped };
      }

      const fallback = await this.installBestEffort({
        sourceRoot,
        sourceType,
        originalSource: trimmed,
        github,
      });
      if (fallback !== undefined) {
        installed.push(fallback);
        return { installed, skipped };
      }
      return {
        installed,
        skipped,
        report: await describeNothingInstallable(
          sourceRoot,
          sourceType === 'local-path' ? (localRealRoot ?? sourceRoot) : trimmed,
        ),
      };
    } finally {
      if (zipTmpDir !== undefined) {
        await rm(zipTmpDir, { recursive: true, force: true });
      }
      if (stagedDir !== undefined) {
        await rm(stagedDir, { recursive: true, force: true });
      }
    }
  }

  private async installBestEffort(input: {
    readonly sourceRoot: string;
    readonly sourceType: PluginSource;
    readonly originalSource: string;
    readonly github?: PluginGithubMetadata;
  }): Promise<PluginRecord | undefined> {
    let synth: SynthOutcome | undefined;
    try {
      synth = await synthesizeFromFolders(input.sourceRoot, {});
    } catch {
      synth = undefined;
    }
    if (synth === undefined) {
      const pieces = await collectLoosePieces(input.sourceRoot);
      if (pieces === undefined) return undefined;
      synth = await synthesizeFromLoosePieces(input.sourceRoot, pieces);
    }
    return this.publishRecord({
      root: input.sourceRoot,
      sourceType: input.sourceType,
      originalSource: input.originalSource,
      github: input.github,
      adaptedFrom: 'skills',
      extraDiagnostics: synth.diagnostics,
    });
  }

  private async publishRecord(input: {
    readonly root: string;
    readonly sourceType: PluginSource;
    readonly originalSource: string;
    readonly github?: PluginGithubMetadata;
    readonly adaptedFrom?: PluginAdaptedFrom;
    readonly extraDiagnostics?: readonly PluginDiagnostic[];
  }): Promise<PluginRecord> {
    const parsed = await parseManifest(input.root);
    if (parsed.manifest === undefined) {
      const msg = parsed.diagnostics.find((d) => d.severity === 'error')?.message ?? 'no manifest';
      throw new Error2(
        ErrorCodes.PLUGIN_LOAD_FAILED,
        `Cannot install plugin from ${input.originalSource}: ${msg}`,
        { details: { sourceType: input.sourceType } },
      );
    }
    const id = normalizePluginId(parsed.manifest.name);
    let managedCopy: ManagedPluginCopy | undefined;
    try {
      managedCopy = await copyPluginToManagedRoot(this.floydHomeDir, id, input.root);
      const normalizedRoot = managedCopy.root;
      const managedParsed = await parseManifest(normalizedRoot);
      const existing = this.records.get(id);
      const now = new Date().toISOString();
      const record = await recordFrom({
        id,
        root: normalizedRoot,
        enabled: existing?.enabled ?? true,
        installedAt: existing?.installedAt ?? now,
        updatedAt: now,
        originalSource: input.originalSource,
        source: input.sourceType,
        capabilities: existing?.capabilities,
        github: input.github,
        adaptedFrom: input.adaptedFrom,
        parsed: managedParsed,
        extraDiagnostics: input.extraDiagnostics,
        discoverSkills: this.discoverSkills,
      });
      const next = new Map(this.records);
      next.set(id, record);
      await this.persist(next);
      this.records = next;
      if (managedCopy.previousRoot !== undefined) {
        await rm(managedCopy.previousRoot, { recursive: true, force: true }).catch(() => undefined);
      }
      managedCopy = undefined;
      return record;
    } catch (error) {
      if (managedCopy !== undefined) {
        try {
          await rollbackManagedPluginCopy(managedCopy);
        } catch (rollbackError) {
          throw new Error2(
            ErrorCodes.PLUGIN_LOAD_FAILED,
            'Plugin installation failed and the previous managed copy could not be restored',
            {
              cause: new AggregateError(
                [error, rollbackError],
                'Plugin installation failed and the previous managed copy could not be restored',
                { cause: error },
              ),
            },
          );
        }
      }
      throw error;
    }
  }

  async setEnabled(id: string, enabled: boolean): Promise<void> {
    const key = normalizePluginId(id);
    const current = this.records.get(key);
    if (current === undefined) throw pluginNotFound(id);
    if (current.enabled === enabled) return;
    const next = new Map(this.records);
    next.set(key, { ...current, enabled, updatedAt: new Date().toISOString() });
    await this.persist(next);
    this.records = next;
  }

  async setMcpServerEnabled(id: string, server: string, enabled: boolean): Promise<void> {
    const key = normalizePluginId(id);
    const current = this.records.get(key);
    if (current === undefined) throw pluginNotFound(id);
    if (current.manifest?.mcpServers?.[server] === undefined) {
      throw new Error2(
        ErrorCodes.MCP_SERVER_NOT_FOUND,
        `Plugin "${id}" does not declare MCP server "${server}"`,
        { details: { id, server } },
      );
    }
    const currentMcpServers = current.capabilities?.mcpServers ?? {};
    const nextCapabilities: PluginCapabilityState = {
      ...current.capabilities,
      mcpServers: {
        ...currentMcpServers,
        [server]: { enabled },
      },
    };
    const next = new Map(this.records);
    next.set(key, {
      ...current,
      capabilities: nextCapabilities,
      updatedAt: new Date().toISOString(),
    });
    await this.persist(next);
    this.records = next;
  }

  async remove(id: string): Promise<void> {
    const key = normalizePluginId(id);
    const next = new Map(this.records);
    if (!next.delete(key)) {
      throw pluginNotFound(id);
    }
    await this.persist(next);
    this.records = next;
  }

  async checkUpdates(): Promise<readonly PluginUpdateStatus[]> {
    const records = [...this.records.values()].filter(
      (record) => record.source === 'github' && record.github !== undefined,
    );
    const results = await Promise.all(
      records.map(async (record) => {
        try {
          return await checkGithubUpdate(record);
        } catch {
          return undefined;
        }
      }),
    );
    return results
      .filter((result): result is PluginUpdateStatus => result !== undefined)
      .toSorted((a, b) => a.id.localeCompare(b.id));
  }

  async reload(): Promise<ReloadSummary> {
    const prevIds = new Set(this.records.keys());
    const file = await readInstalled(this.floydHomeDir);
    const next = new Map<string, PluginRecord>();
    const errors: Array<{ id: string; message: string }> = [];
    for (const entry of file.plugins) {
      try {
        next.set(entry.id, await this.materialize(entry));
      } catch (error) {
        errors.push({ id: entry.id, message: (error as Error).message });
      }
    }
    const added: string[] = [];
    for (const id of next.keys()) if (!prevIds.has(id)) added.push(id);
    const removed: string[] = [];
    for (const id of prevIds) if (!next.has(id)) removed.push(id);
    this.records = next;
    return { added, removed, errors };
  }

  enabledHooks(): readonly HookDef[] {
    const out: HookDef[] = [];
    for (const record of this.records.values()) {
      if (!record.enabled || record.state !== 'ok' || record.manifest === undefined) continue;
      for (const hook of record.manifest.hooks ?? []) {
        out.push({
          ...hook,
          cwd: record.root,
          env: {
            FLOYD_CODE_HOME: this.floydHomeDir,
            FLOYD_PLUGIN_ROOT: record.root,
          },
        });
      }
    }
    return out;
  }

  async enabledCommands(): Promise<readonly PluginCommandDef[]> {
    const out: PluginCommandDef[] = [];
    const records = [...this.records.values()];
    for (const record of records) {
      if (!record.enabled || record.state !== 'ok' || record.manifest === undefined) continue;
      for (const entry of record.manifest.commands ?? []) {
        const def = await loadPluginCommand({
          commandPath: entry.path,
          pluginId: record.id,
          fallbackName: entry.name,
        });
        if (def !== undefined) out.push(def);
      }
    }
    return out;
  }

  pluginSkillRoots(): readonly SkillRoot[] {
    const roots: SkillRoot[] = [];
    for (const record of this.records.values()) {
      if (!record.enabled || record.state !== 'ok' || record.manifest === undefined) continue;
      for (const dir of record.manifest.skills ?? []) {
        roots.push({
          path: dir,
          source: 'extra',
          plugin: { id: record.id, instructions: record.skillInstructions },
          scanMode: record.manifest.rootSkillFallback ? 'root-skill-only' : undefined,
        });
      }
    }
    return roots;
  }

  pluginAgentRoots(): readonly PluginAgentRoot[] {
    const roots: PluginAgentRoot[] = [];
    for (const record of this.records.values()) {
      if (!record.enabled || record.state !== 'ok' || record.manifest === undefined) continue;
      for (const dir of record.manifest.agents ?? []) {
        roots.push({ path: dir, source: 'plugin' });
      }
    }
    return roots;
  }

  enabledSessionStarts(): readonly EnabledPluginSessionStart[] {
    const out: EnabledPluginSessionStart[] = [];
    for (const record of this.records.values()) {
      if (!record.enabled || record.state !== 'ok') continue;
      const skill = record.manifest?.sessionStart?.skill;
      if (skill === undefined) continue;
      out.push({ pluginId: record.id, skillName: skill });
    }
    return out;
  }

  enabledSystemPrompts(): readonly EnabledPluginSystemPrompt[] {
    const out: EnabledPluginSystemPrompt[] = [];
    for (const record of this.records.values()) {
      if (!record.enabled || record.state !== 'ok') continue;
      const content = record.manifest?.systemPrompt;
      if (content === undefined) continue;
      out.push({ pluginId: record.id, content });
    }
    return out;
  }

  enabledMcpServers(): Record<string, McpServerConfig> {
    const out: Record<string, McpServerConfig> = {};
    for (const record of this.records.values()) {
      if (!record.enabled || record.state !== 'ok' || record.manifest === undefined) continue;
      for (const [name, config] of Object.entries(record.manifest.mcpServers ?? {})) {
        if (!isMcpServerEnabled(record, name, config)) continue;
        out[pluginMcpRuntimeName(record.id, name)] = withPluginMcpRuntime(
          withMcpServerEnabled(config, true),
          record.root,
          this.floydHomeDir,
        );
      }
    }
    return out;
  }

  mcpServerEntries(): readonly PluginMcpServerEntry[] {
    const out: PluginMcpServerEntry[] = [];
    for (const record of this.records.values()) {
      if (record.state !== 'ok' || record.manifest === undefined) continue;
      for (const [name, config] of Object.entries(record.manifest.mcpServers ?? {})) {
        const enabled = record.enabled && isMcpServerEnabled(record, name, config);
        const effective = withPluginMcpRuntime(
          withMcpServerEnabled(config, enabled),
          record.root,
          this.floydHomeDir,
        );
        out.push({
          name: pluginMcpRuntimeName(record.id, name),
          config: effective,
          pluginId: record.id,
          serverName: name,
        });
      }
    }
    return out;
  }

  summaries(): readonly PluginSummary[] {
    return this.list().map((record) => recordToSummary(record));
  }

  info(id: string): PluginInfo | undefined {
    const record = this.get(id);
    return record === undefined ? undefined : recordToInfo(record);
  }

  private async persist(records: ReadonlyMap<string, PluginRecord>): Promise<void> {
    const installed: InstalledRecord[] = [...records.values()].map((record) => ({
      id: record.id,
      root: record.root,
      source: record.source,
      enabled: record.enabled,
      installedAt: record.installedAt,
      updatedAt: record.updatedAt,
      originalSource: record.originalSource,
      capabilities: record.capabilities,
      github: record.github,
      adaptedFrom: record.adaptedFrom,
    }));
    await writeInstalled(this.floydHomeDir, { version: 1, plugins: installed });
  }

  private async materialize(entry: InstalledRecord): Promise<PluginRecord> {
    const parsed = await parseManifest(entry.root);
    return recordFrom({
      id: entry.id,
      root: entry.root,
      enabled: entry.enabled,
      installedAt: entry.installedAt,
      updatedAt: entry.updatedAt,
      originalSource: entry.originalSource,
      capabilities: entry.capabilities,
      github: entry.github,
      source: entry.source,
      parsed,
      discoverSkills: this.discoverSkills,
    });
  }
}

async function installedGithubSha(
  owner: string,
  repo: string,
  ref: PluginGithubMetadata['ref'],
): Promise<string | undefined> {
  if (ref.kind === 'sha' && ref.value.length === 40) return ref.value.toLowerCase();
  return resolveGithubCommitSha(owner, repo, ref.value);
}

async function checkGithubUpdate(record: PluginRecord): Promise<PluginUpdateStatus> {
  const github = record.github;
  if (github === undefined)
    throw new BugIndicatingError(`Plugin "${record.id}" has no GitHub metadata`);
  const current = github.ref;
  const pinned = explicitGithubRef(record);

  if (pinned?.kind === 'tag' || pinned?.kind === 'sha') {
    return {
      id: record.id,
      source: 'github',
      current,
      latest: current,
      displayVersion: current.value,
      updateAvailable: false,
    };
  }

  if (pinned?.kind === 'branch') {
    const latestSha = await resolveGithubCommitSha(github.owner, github.repo, pinned.value);
    return {
      id: record.id,
      source: 'github',
      current,
      latest: current,
      displayVersion: latestSha.slice(0, 12),
      updateAvailable: github.installedSha === undefined || github.installedSha !== latestSha,
    };
  }

  const latest = await resolveGithubSource({
    kind: 'github',
    owner: github.owner,
    repo: github.repo,
  });
  let updateAvailable = current.kind !== latest.ref.kind || current.value !== latest.ref.value;
  if (!updateAvailable && (latest.ref.kind === 'branch' || latest.ref.kind === 'tag')) {
    const latestSha = await resolveGithubCommitSha(github.owner, github.repo, latest.ref.value);
    updateAvailable = github.installedSha === undefined || github.installedSha !== latestSha;
  }
  return {
    id: record.id,
    source: 'github',
    current,
    latest: latest.ref,
    displayVersion: latest.displayVersion,
    updateAvailable,
  };
}

function explicitGithubRef(record: PluginRecord): PluginGithubMetadata['ref'] | undefined {
  const fallback =
    record.github?.ref.kind === 'sha' ||
    (record.github?.ref.kind === 'branch' && record.github.ref.value !== 'HEAD')
      ? record.github.ref
      : undefined;
  if (record.originalSource === undefined) return fallback;
  try {
    const source = resolveInstallSource(splitSourceFragment(record.originalSource).base);
    return source.kind === 'github' ? source.ref : fallback;
  } catch {
    return fallback;
  }
}

function splitSourceFragment(source: string): { base: string; fragment?: string } {
  const idx = source.indexOf('#');
  if (idx === -1) return { base: source };
  const base = source.slice(0, idx);
  const fragment = source.slice(idx + 1);
  if (fragment.length === 0) return { base: source };
  if (source.startsWith('http://') || source.startsWith('https://')) return { base, fragment };
  if (existsSync(base)) return { base, fragment };
  return { base: source };
}

function nextNameSuffix(name: string, seen: ReadonlySet<string>): number {
  let suffix = 2;
  while (seen.has(`${name}-${suffix}`)) suffix += 1;
  return suffix;
}

async function describeNothingInstallable(root: string, source: string): Promise<string> {
  let names: readonly string[] = [];
  try {
    names = (await readdir(root, { withFileTypes: true })).map((entry) => entry.name).slice(0, 8);
  } catch {
    names = [];
  }
  const listing = names.length > 0 ? names.join(', ') : 'an empty folder';
  return (
    `Nothing installable in ${source}: no plugin name-tags from any tool, no skills, no ` +
    `commands, no agents. The folder holds: ${listing}.`
  );
}

function pluginNotFound(id: string): Error2 {
  return new Error2(PluginErrors.codes.PLUGIN_NOT_FOUND, `Plugin "${id}" is not installed`, {
    details: { id },
  });
}

async function normalizeInstallRoot(rootPath: string): Promise<string> {
  const trimmed = rootPath.trim();
  if (!path.isAbsolute(trimmed)) {
    throw new Error2(
      ErrorCodes.VALIDATION_FAILED,
      `Plugin root must be an absolute path (got "${rootPath}")`,
      { details: { path: rootPath } },
    );
  }
  let resolved: string;
  try {
    resolved = await realpath(trimmed);
  } catch (error) {
    throw new Error2(ErrorCodes.FS_PATH_NOT_FOUND, `Plugin root does not exist: ${trimmed}`, {
      cause: error,
      details: { path: trimmed },
    });
  }
  if (!(await stat(resolved)).isDirectory()) {
    throw new Error2(ErrorCodes.VALIDATION_FAILED, `Plugin root is not a directory: ${trimmed}`, {
      details: { path: trimmed },
    });
  }
  return resolved;
}

async function copyPluginToManagedRoot(
  floydHomeDir: string,
  id: string,
  sourceRoot: string,
): Promise<ManagedPluginCopy> {
  const managedRoot = path.join(floydHomeDir, 'plugins', 'managed', id);
  const managedDir = path.dirname(managedRoot);
  await mkdir(managedDir, { recursive: true });
  const stagingRoot = await mkdtemp(path.join(managedDir, `${id}-`));
  const previousRoot = `${stagingRoot}-previous`;
  let movedPreviousRoot = false;
  let published = false;
  try {
    await cp(sourceRoot, stagingRoot, { recursive: true });
    try {
      await rename(managedRoot, previousRoot);
      movedPreviousRoot = true;
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error;
    }
    await rename(stagingRoot, managedRoot);
    published = true;
    return {
      root: await realpath(managedRoot),
      previousRoot: movedPreviousRoot ? previousRoot : undefined,
    };
  } catch (error) {
    await rm(published ? managedRoot : stagingRoot, { recursive: true, force: true });
    if (movedPreviousRoot) await rename(previousRoot, managedRoot);
    throw error;
  }
}

async function rollbackManagedPluginCopy(copy: ManagedPluginCopy): Promise<void> {
  await rm(copy.root, { recursive: true, force: true });
  if (copy.previousRoot !== undefined) {
    await rename(copy.previousRoot, copy.root);
  }
}

async function recordFrom(input: {
  id: string;
  root: string;
  enabled: boolean;
  installedAt: string;
  updatedAt?: string;
  originalSource?: string;
  capabilities?: PluginCapabilityState;
  github?: PluginGithubMetadata;
  adaptedFrom?: PluginAdaptedFrom;
  source?: PluginSource;
  parsed: ParsedManifestResult;
  extraDiagnostics?: readonly PluginDiagnostic[];
  discoverSkills: (roots: readonly SkillRoot[]) => Promise<SkillDiscoveryResult>;
}): Promise<PluginRecord> {
  const { parsed } = input;
  const diagnostics = [...parsed.diagnostics, ...(input.extraDiagnostics ?? [])];
  const hasError = diagnostics.some((d) => d.severity === 'error');
  return {
    id: input.id,
    root: input.root,
    source: input.source ?? 'local-path',
    enabled: input.enabled,
    state: hasError || parsed.manifest === undefined ? 'error' : 'ok',
    installedAt: input.installedAt,
    updatedAt: input.updatedAt,
    originalSource: input.originalSource,
    capabilities: input.capabilities,
    github: input.github,
    adaptedFrom: input.adaptedFrom,
    skillCount: await countDiscoveredPluginSkills(input.id, parsed.manifest, input.discoverSkills),
    manifest: parsed.manifest,
    manifestKind: parsed.manifestKind,
    manifestPath: parsed.manifestPath,
    shadowedManifestPath: parsed.shadowedManifestPath,
    diagnostics,
    skillInstructions: parsed.manifest?.skillInstructions,
  };
}

function recordToSummary(record: PluginRecord): PluginSummary {
  return {
    id: record.id,
    displayName: record.manifest?.interface?.displayName ?? record.id,
    version: record.manifest?.version,
    enabled: record.enabled,
    state: record.state,
    skillCount: record.skillCount,
    mcpServerCount: Object.keys(record.manifest?.mcpServers ?? {}).length,
    enabledMcpServerCount: pluginMcpServersInfo(record).filter((server) => server.enabled).length,
    hookCount: record.manifest?.hooks?.length ?? 0,
    commandCount: record.manifest?.commands?.length ?? 0,
    hasErrors: record.diagnostics.some((d) => d.severity === 'error'),
    source: record.source,
    originalSource: record.originalSource,
    github: record.github,
    adaptedFrom: record.adaptedFrom,
  };
}

function recordToInfo(record: PluginRecord): PluginInfo {
  return {
    ...recordToSummary(record),
    root: record.root,
    installedAt: record.installedAt,
    updatedAt: record.updatedAt,
    manifestKind: record.manifestKind,
    manifestPath: record.manifestPath,
    manifest: record.manifest,
    mcpServers: pluginMcpServersInfo(record),
    shadowedManifestPath: record.shadowedManifestPath,
    diagnostics: record.diagnostics,
  };
}

function isMcpServerEnabled(record: PluginRecord, name: string, config: McpServerConfig): boolean {
  return record.capabilities?.mcpServers?.[name]?.enabled ?? config.enabled !== false;
}

function pluginMcpServersInfo(record: PluginRecord): readonly PluginMcpServerInfo[] {
  return Object.entries(record.manifest?.mcpServers ?? {})
    .map(([name, config]) => pluginMcpServerInfo(record, name, config))
    .toSorted((a, b) => a.name.localeCompare(b.name));
}

function pluginMcpServerInfo(
  record: PluginRecord,
  name: string,
  config: McpServerConfig,
): PluginMcpServerInfo {
  if (config.transport === 'http' || config.transport === 'sse') {
    return {
      name,
      runtimeName: pluginMcpRuntimeName(record.id, name),
      enabled: isMcpServerEnabled(record, name, config),
      transport: config.transport,
      url: config.url,
      headerKeys: config.headers === undefined ? undefined : Object.keys(config.headers).toSorted(),
    };
  }
  return {
    name,
    runtimeName: pluginMcpRuntimeName(record.id, name),
    enabled: isMcpServerEnabled(record, name, config),
    transport: 'stdio',
    command: config.command,
    args: config.args,
    cwd: config.cwd,
    envKeys: config.env === undefined ? undefined : Object.keys(config.env).toSorted(),
  };
}

function pluginMcpRuntimeName(pluginId: string, serverName: string): string {
  return `plugin-${pluginId}:${serverName}`;
}

const FLOYD_NODE_FALLBACK_SUBCOMMAND = '__plugin_run_node';

function withMcpServerEnabled(config: McpServerConfig, enabled: boolean): McpServerConfig {
  return { ...config, enabled };
}

function withPluginMcpRuntime(
  config: McpServerConfig,
  pluginRoot: string,
  floydHomeDir: string,
): McpServerConfig {
  if (config.transport === 'http' || config.transport === 'sse') return config;

  const env = {
    ...config.env,
    FLOYD_CODE_HOME: floydHomeDir,
    FLOYD_PLUGIN_ROOT: pluginRoot,
  };

  if (config.command === 'node' && isElectron()) {
    return {
      ...config,
      command: process.execPath,
      args: config.args ?? [],
      cwd: config.cwd ?? pluginRoot,
      env: { ...env, ELECTRON_RUN_AS_NODE: '1' },
    };
  }

  if (config.command === 'node' && isFloydNativeBinary()) {
    return {
      ...config,
      command: process.execPath,
      args: [FLOYD_NODE_FALLBACK_SUBCOMMAND, ...(config.args ?? [])],
      cwd: config.cwd ?? pluginRoot,
      env,
    };
  }

  return { ...config, cwd: config.cwd ?? pluginRoot, env };
}

function isElectron(): boolean {
  return typeof process.versions['electron'] === 'string';
}

function isFloydNativeBinary(): boolean {
  return !path.basename(process.execPath).toLowerCase().startsWith('node');
}

async function countDiscoveredPluginSkills(
  pluginId: string,
  manifest: PluginRecord['manifest'],
  discoverSkills: (roots: readonly SkillRoot[]) => Promise<SkillDiscoveryResult>,
): Promise<number> {
  const dirs = manifest?.skills ?? [];
  if (dirs.length === 0) return 0;
  const roots: SkillRoot[] = dirs.map((dir) => ({
    path: dir,
    source: 'extra',
    plugin: { id: pluginId, instructions: manifest?.skillInstructions },
    scanMode: manifest?.rootSkillFallback ? 'root-skill-only' : undefined,
  }));
  const result = await discoverSkills(roots);
  return result.skills.length;
}
