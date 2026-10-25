import { EventEmitter } from 'node:events';
import { basename, isAbsolute, join, resolve } from 'node:path';
import { copyFile, mkdir, readFile, realpath, rename, stat, writeFile } from 'node:fs/promises';
import { GoRequestError, type GoWorkspace } from './client';
import { GoRuntime } from './runtime';
import { LocalViews } from './localViews';
import { object, text, workspaceKey } from './projection';

export interface GoWorkspaceView { runtime: GoRuntime; views: LocalViews }
interface WorkspaceRecord { root: string; name: string; created_at: string; last_opened_at: string; hidden?: boolean }

export class GoWorkspaces extends EventEmitter {
  private readonly attached = new Map<string, GoWorkspaceView>();
  private readonly records = new Map<string, WorkspaceRecord>();
  private readonly flights = new Map<string, Promise<GoWorkspaceView>>();
  private readonly sessionOwners = new Map<string, GoRuntime>();
  private writeQueue: Promise<unknown> = Promise.resolve();
  onAttach?: (runtime: GoRuntime) => Promise<void>;

  constructor(readonly primary: GoRuntime, readonly primaryViews: LocalViews, readonly homeDir: string) {
    super();
    this.add(primary, primaryViews);
  }

  async load(): Promise<void> {
    for (const name of ['workspaces.json', 'go-workspaces.json']) {
      let saved: unknown;
      try { saved = JSON.parse(await readFile(join(this.homeDir, name), 'utf8')); }
      catch (error) { if ((error as NodeJS.ErrnoException).code === 'ENOENT') continue; throw error; }
      const catalog = object(saved);
      const legacy = !Array.isArray(saved) && catalog['version'] === 1 && catalog['workspaces'] !== null && typeof catalog['workspaces'] === 'object' && !Array.isArray(catalog['workspaces']);
      if (!Array.isArray(saved) && !legacy) throw new Error(`Canonical workspace preferences ${name} contain an unsupported format`);
      const deleted = new Set(Array.isArray(catalog['deleted_workspace_ids']) ? catalog['deleted_workspace_ids'] : []);
      const entries = Array.isArray(saved) ? saved.map((entry, index) => [String(index), entry] as const) : Object.entries(object(catalog['workspaces']));
      for (const [id, entry] of entries) {
        const record = object(entry);
        if (!isAbsolute(text(record['root'])) || !text(record['name']) || !text(record['created_at']) || !text(record['last_opened_at'])) throw new Error(`Canonical workspace preferences ${name} contain an invalid root`);
        this.records.set(workspaceKey(text(record['root'])), { root: text(record['root']), name: text(record['name']), created_at: text(record['created_at']), last_opened_at: text(record['last_opened_at']), hidden: record['hidden'] === true || deleted.has(id) });
      }
    }
    this.record(this.primary.cwd);
  }

  private record(root: string): WorkspaceRecord {
    const key = workspaceKey(root);
    let record = this.records.get(key);
    if (!record) {
      record = { root, name: basename(root) || root, created_at: new Date().toISOString(), last_opened_at: new Date().toISOString() };
      this.records.set(key, record);
    }
    return record;
  }

  private add(runtime: GoRuntime, views: LocalViews): GoWorkspaceView {
    const entry = { runtime, views };
    this.attached.set(workspaceKey(runtime.cwd), entry);
    runtime.on('changed', (ids?: string[]) => this.emit('changed', { runtime, ids }));
    runtime.on('unavailable', (message: string) => this.emit('unavailable', message));
    runtime.on('session', (event) => this.emit('session', { runtime, ...event }));
    runtime.on('prompt', (event) => this.emit('prompt', { runtime, ...event }));
    return entry;
  }

  async root(root: string): Promise<GoWorkspaceView> {
    if (!isAbsolute(root)) throw new GoRequestError(400, 'Workspace root must be an absolute path');
    const canonical = await realpath(root);
    if (!(await stat(canonical)).isDirectory()) throw new GoRequestError(400, 'Workspace root must be a directory');
    const key = workspaceKey(canonical);
    const existing = this.attached.get(key);
    if (existing) return existing;
    for (const entry of this.attached.values()) if (await realpath(entry.runtime.cwd) === canonical) return entry;
    const flight = this.flights.get(key);
    if (flight) return flight;
    const pending = (async () => {
      const running = await this.primary.client.json<GoWorkspace[]>('GET', '/v1/workspaces');
      const resolved = await Promise.all(running.map(async (item) => ({ item, path: await realpath(item.path) })));
      const current = resolved.find((item) => item.path === canonical)?.item;
      const runtime = new GoRuntime(this.primary.host, canonical, current?.data_dir || '');
      try { await runtime.start(); await this.onAttach?.(runtime); } catch (error) { await runtime.close(); throw error; }
      this.record(canonical);
      return this.add(runtime, new LocalViews(runtime, this.homeDir));
    })();
    this.flights.set(key, pending);
    try { return await pending; } finally { this.flights.delete(key); }
  }

  async all(workspaceIds: string[] = []): Promise<GoWorkspaceView[]> {
    if (workspaceIds.length) return Promise.all([...new Set(workspaceIds)].map((id) => this.workspace(id)));
    const running = await this.primary.client.json<GoWorkspace[]>('GET', '/v1/workspaces');
    const items: GoWorkspaceView[] = [];
    for (const item of running) if (!this.record(item.path).hidden) items.push(await this.root(item.path));
    return items;
  }

  async list() {
    await this.all();
    return Promise.all([...this.records.values()].filter((record) => !record.hidden).map(async (record) => {
      const entry = this.attached.get(workspaceKey(record.root));
      if (entry) return this.wire(entry);
      return { id: workspaceKey(record.root), root: record.root, name: record.name, created_at: record.created_at, last_opened_at: record.last_opened_at, session_count: null };
    }));
  }

  async recentRoots(): Promise<string[]> {
    for (const item of await this.primary.client.json<GoWorkspace[]>('GET', '/v1/workspaces')) this.record(item.path);
    return [...this.records.values()].filter((record) => !record.hidden).toSorted((a, b) => b.last_opened_at.localeCompare(a.last_opened_at)).map((record) => record.root);
  }

  async workspace(id: string): Promise<GoWorkspaceView> {
    let record = this.records.get(id);
    if (!record) { await this.all(); record = this.records.get(id); }
    if (!record || record.hidden) throw new GoRequestError(404, 'Workspace is not registered');
    return this.root(record.root);
  }

  async session(id: string): Promise<GoWorkspaceView> {
    const cached = this.sessionOwners.get(id);
    if (cached) {
      try { await cached.session(id); return this.attached.get(workspaceKey(cached.cwd))!; }
      catch (error) { if (!(error instanceof GoRequestError) || error.status !== 404) throw error; this.sessionOwners.delete(id); }
    }
    for (const entry of await this.all()) {
      const sessions = await entry.runtime.sessions();
      for (const session of sessions) this.sessionOwners.set(session.id, entry.runtime);
      if (sessions.some((session) => session.id === id)) return entry;
    }
    throw new GoRequestError(404, `Session ${id} does not exist in the shared Go server`);
  }

  async wire(entry: GoWorkspaceView) {
    const record = this.record(entry.runtime.cwd);
    return { id: workspaceKey(record.root), root: record.root, name: record.name, created_at: record.created_at, last_opened_at: record.last_opened_at, session_count: (await entry.runtime.sessions()).length };
  }

  async register(root: string, name?: string) {
    const entry = await this.root(root);
    const record = this.record(entry.runtime.cwd);
    if (name !== undefined) { if (!name.trim() || name.length > 100) throw new GoRequestError(400, 'Workspace name must contain between one and 100 characters'); record.name = name; }
    record.hidden = false;
    record.last_opened_at = new Date().toISOString();
    await this.persist();
    return this.wire(entry);
  }

  async forget(id: string): Promise<{ deleted: true }> {
    const entry = await this.workspace(id);
    this.record(entry.runtime.cwd).hidden = true;
    await this.persist();
    return { deleted: true };
  }

  private async persist(): Promise<void> {
    const data = JSON.stringify([...this.records.values()], null, 2);
    const pending = this.writeQueue.then(async () => {
      await mkdir(this.homeDir, { recursive: true });
      const path = join(this.homeDir, 'go-workspaces.json');
      try { await copyFile(path, `${path}.bak-${Date.now()}`); } catch (error) { if ((error as NodeJS.ErrnoException).code !== 'ENOENT') throw error; }
      await writeFile(`${path}.tmp`, data, 'utf8');
      await rename(`${path}.tmp`, path);
    });
    this.writeQueue = pending.catch(() => {});
    await pending;
  }

  async close(): Promise<void> {
    for (const entry of this.attached.values()) { entry.views.close(); await entry.runtime.close(); }
    this.attached.clear();
    this.removeAllListeners();
  }
}
