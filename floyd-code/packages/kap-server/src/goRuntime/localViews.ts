import { homedir, release } from 'node:os';
import { isAbsolute, join, relative, resolve } from 'node:path';
import { realpath, stat } from 'node:fs/promises';
import { LocalRuntime } from '@legacy-ai/agent-core-v2/runtime/localRuntime';
import { HostFileSystem } from '@legacy-ai/agent-core-v2/os/backends/node-local/hostFsService';
import { HostProcessService } from '@legacy-ai/agent-core-v2/os/backends/node-local/hostProcessService';
import { HostTerminalService } from '@legacy-ai/agent-core-v2/os/backends/node-local/hostTerminalService';
import type { IHostEnvironment } from '@legacy-ai/agent-core-v2/os/interface/hostEnvironment';
import type { IRuntimeResolver, IWorkspaceInstanceManager } from '@legacy-ai/agent-core-v2/workspace/workspaceInstance/workspaceInstanceManager';
import type { IWorkspaceContext } from '@legacy-ai/agent-core-v2/workspace/workspaceContext/workspaceContext';
import type { IWorkspaceDirs, WorkspaceAddDirInput, WorkspaceAdditionalDirsResult } from '@legacy-ai/agent-core-v2/workspace/workspaceDirs/workspaceDirs';
import { WorkspaceFsService } from '@legacy-ai/agent-core-v2/workspace/workspaceFs/fsService';
import { WorkspaceGitService } from '@legacy-ai/agent-core-v2/workspace/workspaceGit/workspaceGitService';
import { GitService } from '@legacy-ai/agent-core-v2/app/git/gitService';
import { noopTelemetryService } from '@legacy-ai/agent-core-v2/app/telemetry/telemetry';
import { SessionTerminalService } from '@legacy-ai/agent-core-v2/session/terminal/terminalService';
import { makeSessionContext } from '@legacy-ai/agent-core-v2/session/sessionContext/sessionContext';
import type { ISessionWorkspaceContext } from '@legacy-ai/agent-core-v2/session/workspaceContext/workspaceContext';
import * as fsSchemas from '@legacy-ai/agent-core-v2/workspace/workspaceFs/fs';
import { fsOpenInRequestSchema, fsOpenRequestSchema, fsRevealRequestSchema } from '../protocol/rest-fs';
import { createTerminalRequestSchema } from '../protocol/rest-terminal';
import { launchDetached, openFileCommandFor, openInAppCommandFor, revealFileCommandFor } from '../lib/fileLaunch';
import { GoRequestError } from './client';
import { GoRuntime, UnsupportedError } from './runtime';
import { array, object, text, workspaceKey } from './projection';

export class LocalViews {
  readonly fs: WorkspaceFsService;
  private readonly os: LocalRuntime;
  private readonly hostTerminal = new HostTerminalService();
  private readonly resolver: IRuntimeResolver;
  private readonly terminals = new Map<string, SessionTerminalService>();
  private additionalDirs: string[] = [];
  private readonly dirListeners = new Set<() => void>();

  constructor(private readonly go: GoRuntime, private readonly homeDir: string, private readonly explicitRoots?: readonly string[]) {
    const cwd = explicitRoots?.[0] || go.cwd;
    this.additionalDirs = explicitRoots?.slice(1) || [];
    const workspaceId = workspaceKey(cwd);
    const environment: IHostEnvironment = {
      _serviceBrand: undefined,
      osKind: process.platform === 'darwin' ? 'macOS' : process.platform,
      osArch: process.arch,
      osVersion: release(),
      shellName: 'sh',
      shellPath: process.env['SHELL'] || '/bin/sh',
      pathClass: 'posix',
      homeDir: homedir(),
      ready: Promise.resolve(),
    };
    const hostFs = new HostFileSystem();
    this.os = new LocalRuntime(workspaceId, environment, hostFs, new HostProcessService(), this.hostTerminal);
    this.resolver = {
      _serviceBrand: undefined,
      inspect: () => this.os,
      acquire: (binding, capabilities = []) => {
        if (binding.workspaceId !== workspaceId || binding.runtimeId !== 'local') throw new UnsupportedError('Another local view runtime');
        if (capabilities.some((capability) => !this.os.capabilities.has(capability))) throw new UnsupportedError('The requested local view capability');
        return { runtime: this.os, track: (resource) => resource, dispose: () => {} };
      },
    };
    const workspace: IWorkspaceContext = {
      _serviceBrand: undefined, workspaceId, cwd, source: 'local',
      meta: { id: workspaceId, root: cwd, name: workspaceId, createdAt: 0, lastOpenedAt: 0 },
      persistenceScope: `sessions/${workspaceId}`,
    };
    const owner = this;
    const onDidChange = (listener: () => void) => { this.dirListeners.add(listener); return { dispose: () => { this.dirListeners.delete(listener); } }; };
    const dirs: IWorkspaceDirs = {
      _serviceBrand: undefined, ready: Promise.resolve(), get additionalDirs() { return owner.additionalDirs; }, onDidChange,
      addDir: (input) => this.addDirectory(input),
      mergeAdditionalDirs: async (baseDir, paths) => { for (const path of paths) await this.addDirectory({ path: resolve(baseDir, path), persist: false }); },
      sessionInfo: () => ({ _serviceBrand: undefined, ready: Promise.resolve(), onDidChange, workDir: cwd, get additionalDirs() { return owner.additionalDirs; } }),
    };
    const instances = { findByRoot: (root: string) => root === cwd ? { id: workspaceId } : undefined } as unknown as IWorkspaceInstanceManager;
    const git = new WorkspaceGitService(workspace, { current: new GitService(this.resolver, instances, hostFs), onDidChange: () => ({ dispose: () => {} }) });
    this.fs = new WorkspaceFsService(workspace, dirs, hostFs, this.resolver, noopTelemetryService, git);
  }

  async refreshDirectories(): Promise<void> {
    if (this.explicitRoots) return;
    const config = await this.go.client.json<Record<string, unknown>>('GET', `${this.go.base}/config`);
    const dirs = array(object(config['options'])['additional_dirs']).map(text).filter(Boolean).map((path) => resolve(this.go.cwd, path));
    if (JSON.stringify(dirs) !== JSON.stringify(this.additionalDirs)) { this.additionalDirs = dirs; for (const listener of this.dirListeners) listener(); }
  }

  async suggestRoots(roots: unknown[], body: Record<string, unknown>) {
    this.assertLocal(body);
    if (!roots.length || roots.length > 32 || roots.some((root) => typeof root !== 'string' || !isAbsolute(root))) throw new GoRequestError(400, 'Choose between one and 32 absolute directories');
    const validated: string[] = [];
    for (const value of roots) {
      const root = await realpath(value as string);
      if (!(await stat(root)).isDirectory()) throw new GoRequestError(400, 'Suggestion roots must be directories');
      validated.push(value as string);
    }
    const temporary = new LocalViews(this.go, this.homeDir, validated);
    try { return await temporary.fsAction('suggest', body); } finally { temporary.close(); }
  }

  async addDirectory(input: WorkspaceAddDirInput): Promise<WorkspaceAdditionalDirsResult> {
    if (!input.path.trim()) throw new GoRequestError(400, 'Additional directory path is required');
    const result = await this.go.client.json<{ project_root: string; config_path: string; additional_dirs: string[]; persisted: boolean }>('POST', `${this.go.base}/workspace/add-dir`, input);
    this.additionalDirs = result.additional_dirs;
    for (const listener of this.dirListeners) listener();
    return { projectRoot: result.project_root, configPath: result.config_path, additionalDirs: result.additional_dirs, persisted: result.persisted };
  }

  assertLocal(input: Record<string, unknown>): void {
    if (input['runtime_id'] && input['runtime_id'] !== 'local') throw new UnsupportedError('Another filesystem or terminal runtime');
  }

  async fsAction(action: string, body: Record<string, unknown>): Promise<unknown> {
    this.assertLocal(body);
    await this.refreshDirectories();
    switch (action) {
      case 'list': return this.fs.list(fsSchemas.fsListRequestSchema.parse(body));
      case 'read': return this.fs.read(fsSchemas.fsReadRequestSchema.parse(body));
      case 'list_many': return this.fs.listMany(fsSchemas.fsListManyRequestSchema.parse(body));
      case 'stat': return this.fs.stat(fsSchemas.fsStatRequestSchema.parse(body));
      case 'stat_many': return this.fs.statMany(fsSchemas.fsStatManyRequestSchema.parse(body));
      case 'mkdir': return this.fs.mkdir(fsSchemas.fsMkdirRequestSchema.parse(body));
      case 'search': return this.fs.search(fsSchemas.fsSearchRequestSchema.parse(body));
      case 'suggest': return this.fs.suggest(fsSchemas.fsSuggestRequestSchema.parse(body));
      case 'grep': return this.fs.grep(fsSchemas.fsGrepRequestSchema.parse(body));
      case 'git_status': return this.fs.gitStatus(fsSchemas.fsGitStatusRequestSchema.parse(body));
      case 'diff': return this.fs.diff(fsSchemas.fsDiffRequestSchema.parse(body));
      case 'open': {
        const value = fsOpenRequestSchema.parse(body);
        const path = await this.fs.resolvePath(value.path);
        await launchDetached(openFileCommandFor(path.absolute, value.line));
        return { opened: true };
      }
      case 'reveal': {
        const value = fsRevealRequestSchema.parse(body);
        const path = await this.fs.resolvePath(value.path);
        await launchDetached(revealFileCommandFor(path.absolute));
        return { revealed: true };
      }
      case 'open-in': {
        const value = fsOpenInRequestSchema.parse(body);
        const path = await this.fs.resolvePath(value.path);
        await launchDetached(openInAppCommandFor(value.app_id, path.absolute, { line: value.line, isDirectory: path.isDirectory }));
        return { opened: true };
      }
      default: throw new UnsupportedError(`Filesystem ${action}`);
    }
  }

  async terminal(sessionId: string): Promise<SessionTerminalService> {
    await this.go.session(sessionId);
    await this.refreshDirectories();
    let terminal = this.terminals.get(sessionId);
    if (terminal) return terminal;
    const cwd = this.go.cwd;
    const allowed = (candidate: string) => {
      return [cwd, ...this.additionalDirs].some((root) => { const path = relative(root, resolve(candidate)); return path !== '..' && !path.startsWith('../') && !path.startsWith('/'); });
    };
    const owner = this;
    const workspace: ISessionWorkspaceContext = {
      _serviceBrand: undefined, workDir: cwd, get additionalDirs() { return owner.additionalDirs; },
      resolve: (path) => resolve(cwd, path), isWithin: allowed,
      assertAllowed: (path) => { if (!allowed(path)) throw new GoRequestError(400, 'Path is outside the shared Go workspace'); return path; },
    };
    terminal = new SessionTerminalService(this.resolver, workspace, makeSessionContext({ sessionId, workspaceId: workspaceKey(cwd), sessionDir: join(this.homeDir, 'terminals', sessionId), sessionScope: sessionId, cwd }));
    this.terminals.set(sessionId, terminal);
    return terminal;
  }

  async createTerminal(sessionId: string, body: Record<string, unknown>) {
    this.assertLocal(body);
    await this.refreshDirectories();
    const request = createTerminalRequestSchema.parse(body);
    if (request.cwd) {
      const canonical = await realpath(resolve(this.go.cwd, request.cwd));
      await this.fs.resolvePath(canonical);
    }
    return (await this.terminal(sessionId)).create({ ...request, runtime_id: 'local' });
  }

  detach(sinkId: string): void {
    for (const terminal of this.terminals.values()) terminal.detachAllForSink(sinkId);
  }

  closeSession(sessionId: string): void {
    this.terminals.get(sessionId)?.dispose();
    this.terminals.delete(sessionId);
  }

  close(): void {
    for (const terminal of this.terminals.values()) terminal.dispose();
    this.terminals.clear();
    this.hostTerminal.dispose();
    this.os.dispose();
  }
}
