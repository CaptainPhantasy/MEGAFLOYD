import { mkdtemp, readFile, readdir, rm, mkdir, writeFile, access, chmod, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';

import { Readable, Writable } from 'node:stream';

import { afterEach, beforeEach, describe, expect, it } from 'vitest';

import type { IPluginService } from '#/app/plugin/plugin';
import type { IHostProcess, IHostProcessService } from '#/os/interface/hostProcess';
import {
  __floydWebbridgeInternals,
  createFloydWebbridgeEntry,
} from '#/app/capability/entries/floydWebbridge';
import type { CapabilityEntryContext } from '#/app/capability/entries/context';

const DAEMON_BASE = 'http://127.0.0.1:10086';
const CDN_BASE_ENV = 'FLOYD_CODE_CDN_BASE';
const GLOBAL_CDN_BASE_ENV = 'FLOYD_CODE_GLOBAL_CDN_BASE';
const CONTENT_CDN_BASE_ENV = 'FLOYD_CODE_CONTENT_CDN_BASE';
const CDN_BASE = 'https://cdn.example.test/floyd-code';
const GLOBAL_CDN_BASE = 'https://cdn.example.test/floyd-code-global';
const CONTENT_CDN_BASE = 'https://cdn.example.test/content';

function fakeProc(code: number, stdout = '', stderr = ''): IHostProcess {
  return {
    _serviceBrand: undefined,
    pid: 1234,
    exitCode: code,
    stdin: new Writable({
      write: (_c, _e, cb) => {
        cb();
      },
    }),
    stdout: Readable.from([stdout]),
    stderr: Readable.from([stderr]),
    wait: () => Promise.resolve(code),
    kill: () => Promise.resolve(),
    dispose: () => undefined,
  } as IHostProcess;
}

interface SpawnCall {
  command: string;
  args: readonly string[];
}

function fakeHostProcess(script?: Array<{ match: string; code: number; stdout?: string; stderr?: string }>): {
  service: IHostProcessService;
  calls: SpawnCall[];
} {
  const calls: SpawnCall[] = [];
  const service: IHostProcessService = {
    _serviceBrand: undefined,
    spawn: (command: string, args: readonly string[] = []) => {
      calls.push({ command, args });
      const key = `${command} ${args.join(' ')}`;
      const hit = script?.find((s) => key.includes(s.match));
      return Promise.resolve(fakeProc(hit?.code ?? 0, hit?.stdout ?? '', hit?.stderr ?? ''));
    },
  } as IHostProcessService;
  return { service, calls };
}

function fakePlugins(installed: Array<{ id: string; enabled: boolean; state: string; version?: string }>): {
  service: IPluginService;
  installs: string[];
  enabledCalls: Array<{ id: string; enabled: boolean }>;
} {
  const installs: string[] = [];
  const enabledCalls: Array<{ id: string; enabled: boolean }> = [];
  const service = {
    listPlugins: () =>
      Promise.resolve(
        installed.map((p) => ({
          id: p.id,
          displayName: p.id,
          version: p.version,
          enabled: p.enabled,
          state: p.state,
          skillCount: 1,
          mcpServerCount: 0,
          enabledMcpServerCount: 0,
          hookCount: 0,
          commandCount: 0,
          hasErrors: false,
          source: 'zip-url',
        })),
      ),
    installPlugin: (input: { source: string }) => {
      installs.push(input.source);
      const existing = installed.find((p) => p.id === 'floyd-webbridge');
      if (existing === undefined) {
        installed.push({ id: 'floyd-webbridge', enabled: true, state: 'ok', version: '1.11.3' });
        return Promise.resolve({ enabled: true } as never);
      }
      existing.state = 'ok';
      existing.version = '1.11.3';
      return Promise.resolve({ enabled: existing.enabled } as never);
    },
    setPluginEnabled: (input: { id: string; enabled: boolean }) => {
      enabledCalls.push(input);
      const existing = installed.find((p) => p.id === input.id);
      if (existing !== undefined) existing.enabled = input.enabled;
      return Promise.resolve();
    },
  } as unknown as IPluginService;
  return { service, installs, enabledCalls };
}

function fakeFetch(opts: {
  statusSequence?: Array<object | 'error'>;
  binary?: Uint8Array;
}): { fetchImpl: typeof fetch; urls: string[] } {
  let statusCalls = 0;
  const urls: string[] = [];
  const fetchImpl = (async (url: string | URL): Promise<Response> => {
    const u = String(url);
    urls.push(u);
    if (u === `${DAEMON_BASE}/status`) {
      const step = opts.statusSequence?.[Math.min(statusCalls, (opts.statusSequence?.length ?? 1) - 1)];
      statusCalls += 1;
      if (step === 'error' || step === undefined) throw new Error('connection refused');
      return new Response(JSON.stringify(step), { status: 200 });
    }
    if (u.startsWith(`${CONTENT_CDN_BASE}/webbridge/`)) {
      const bytes = opts.binary ?? new Uint8Array([1, 2, 3, 4]);
      return new Response(bytes, {
        status: 200,
        headers: { 'content-length': String(bytes.length) },
      });
    }
    throw new Error(`unexpected fetch: ${u}`);
  }) as unknown as typeof fetch;
  return { fetchImpl, urls };
}

describe('floyd-webbridge entry', () => {
  let root: string;
  let savedEnv: Record<string, string | undefined>;

  beforeEach(async () => {
    root = await mkdtemp(path.join(tmpdir(), 'floyd-webbridge-entry-'));
    savedEnv = {
      [CONTENT_CDN_BASE_ENV]: process.env[CONTENT_CDN_BASE_ENV],
      [CDN_BASE_ENV]: process.env[CDN_BASE_ENV],
      [GLOBAL_CDN_BASE_ENV]: process.env[GLOBAL_CDN_BASE_ENV],
    };
    process.env[CONTENT_CDN_BASE_ENV] = CONTENT_CDN_BASE;
    process.env[CDN_BASE_ENV] = CDN_BASE;
    process.env[GLOBAL_CDN_BASE_ENV] = GLOBAL_CDN_BASE;
  });
  afterEach(async () => {
    await rm(root, { recursive: true, force: true });
    for (const [name, value] of Object.entries(savedEnv)) {
      if (value === undefined) delete process.env[name];
      else process.env[name] = value;
    }
  });

  function makeCtx(overrides: Partial<CapabilityEntryContext> = {}): CapabilityEntryContext {
    return {
      platform: 'darwin',
      arch: 'arm64',
      floydHomeDir: path.join(root, 'floyd-home'),
      userHomeDir: path.join(root, 'user-home'),
      plugins: fakePlugins([]).service,
      hostProcess: fakeHostProcess().service,
      resolveRegion: () => 'mainland-cn',
      ...overrides,
    };
  }

  it('maps platforms to CDN asset names', () => {
    const { binaryAssetName } = __floydWebbridgeInternals;
    expect(binaryAssetName('darwin', 'arm64')).toBe('floyd-webbridge-darwin-arm64');
    expect(binaryAssetName('darwin', 'x64')).toBe('floyd-webbridge-darwin-amd64');
    expect(binaryAssetName('linux', 'arm64')).toBe('floyd-webbridge-linux-arm64');
    expect(binaryAssetName('linux', 'x64')).toBe('floyd-webbridge-linux-amd64');
    expect(binaryAssetName('win32', 'x64')).toBe('floyd-webbridge-windows-amd64.exe');
    expect(binaryAssetName('win32', 'arm64')).toBeUndefined();
    expect(binaryAssetName('freebsd', 'x64')).toBeUndefined();
  });

  it('EXDEV fallback replaces the destination without opening it for write', async () => {
    const { renameAcrossDevicesFallback } = __floydWebbridgeInternals;
    const from = path.join(root, 'staging', 'floyd-webbridge');
    const to = path.join(root, 'bin', 'floyd-webbridge');
    await mkdir(path.dirname(from), { recursive: true });
    await mkdir(path.dirname(to), { recursive: true });
    await writeFile(from, 'new');
    await writeFile(to, 'old-running');

    await renameAcrossDevicesFallback(from, to);

    expect(await readFile(to, 'utf-8')).toBe('new');
    await expect(access(from)).rejects.toThrow();
    const binEntries = await readdir(path.dirname(to));
    expect(binEntries.filter((entry) => entry.endsWith('.tmp'))).toEqual([]);
  });

  it('is unsupported on unknown platforms', () => {
    const entry = createFloydWebbridgeEntry(makeCtx({ platform: 'freebsd' }));
    expect(entry.supported).toBe(false);
  });

  it('detects a fully installed daemon with extension as soft gate', async () => {
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(userHome, '.floyd-webbridge', 'bin'), { recursive: true });
    const binPath = path.join(userHome, '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await writeFile(binPath, 'bin');
    await chmod(binPath, 0o755);
    const plugins = fakePlugins([{ id: 'floyd-webbridge', enabled: true, state: 'ok', version: '1.11.3' }]);
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: false }],
    });
    const entry = createFloydWebbridgeEntry(makeCtx({ plugins: plugins.service, fetchImpl }));

    const detected = await entry.detect();
    expect(detected.version).toBe('v1.11.3');
    expect(detected.steps).toEqual([
      { id: 'daemon-binary', state: 'ok' },
      { id: 'daemon', state: 'ok', detail: 'v1.11.3' },
      { id: 'skill', state: 'ok', detail: '1.11.3' },
      { id: 'extension', state: 'missing', optional: true },
    ]);
  });

  it('backs up standalone skills after refreshing the managed plugin', async () => {
    const floydHome = path.join(root, 'floyd-home');
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(floydHome, 'skills', 'floyd-webbridge'), { recursive: true });
    await writeFile(path.join(floydHome, 'skills', 'floyd-webbridge', 'SKILL.md'), 'old');
    await mkdir(path.join(userHome, '.agents', 'skills', 'floyd-webbridge'), { recursive: true });
    await writeFile(path.join(userHome, '.agents', 'skills', 'floyd-webbridge', 'SKILL.md'), 'old');
    const plugins = fakePlugins([{ id: 'floyd-webbridge', enabled: true, state: 'ok' }]);
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(makeCtx({ plugins: plugins.service, fetchImpl }));

    const detected = await entry.detect();

    expect(detected.steps.find((step) => step.id === 'standalone-skill-migration')).toEqual({
      id: 'standalone-skill-migration',
      state: 'missing',
      detail: `${path.join(floydHome, 'skills', 'floyd-webbridge')}, ${path.join(userHome, '.agents', 'skills', 'floyd-webbridge')}`,
      optional: true,
    });
    const reports: string[] = [];
    const note = await entry.install((step) => reports.push(step));

    expect(plugins.installs).toEqual([
      `${CDN_BASE}/plugins/official/floyd-webbridge.zip`,
    ]);
    expect(note).toBe('user-skill-migrated');
    expect(reports).toContain('standalone-skill-migration');
    await expect(access(path.join(floydHome, 'skills', 'floyd-webbridge'))).rejects.toThrow();
    await expect(access(path.join(userHome, '.agents', 'skills', 'floyd-webbridge'))).rejects.toThrow();

    const backupDir = path.join(floydHome, 'backups', 'floyd-webbridge-skills');
    const backups = await readdir(backupDir);
    expect(backups).toHaveLength(1);
    await expect(
      readFile(path.join(backupDir, backups[0]!, 'floyd-code', 'SKILL.md'), 'utf8'),
    ).resolves.toBe('old');
    await expect(
      readFile(path.join(backupDir, backups[0]!, 'agents', 'SKILL.md'), 'utf8'),
    ).resolves.toBe('old');
  });

  it('installs end-to-end: download, start-if-down, and plugin wiring', async () => {
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl, urls } = fakeFetch({
      statusSequence: [
        { running: false },
        { running: false },
        { running: true, version: 'v1.11.3', extension_connected: true },
      ],
    });
    const reports: Array<[string, number | undefined]> = [];
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );

    await entry.install((step, percent) => reports.push([step, percent]));

    const binPath = path.join(root, 'user-home', '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await access(binPath);
    expect(host.calls.map((c) => `${c.command} ${c.args.join(' ')}`)).toEqual([`${binPath} start`]);
    expect(plugins.installs).toEqual([
      `${CDN_BASE}/plugins/official/floyd-webbridge.zip`,
    ]);
    expect(urls).toContain(
      `${CONTENT_CDN_BASE}/webbridge/latest/releases/floyd-webbridge-darwin-arm64`,
    );
    expect(urls.every((url) => url.startsWith('http'))).toBe(true);
    expect(reports[0]).toEqual(['download', 0]);
    expect(reports.some(([step]) => step === 'daemon')).toBe(true);
    expect(reports.some(([step]) => step === 'skill')).toBe(true);
  });

  it('installs the plugin zip from the global CDN base when the region is global', async () => {
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({
        plugins: plugins.service,
        hostProcess: host.service,
        fetchImpl,
        resolveRegion: () => 'global',
      }),
    );

    await entry.install(() => {});

    expect(plugins.installs).toEqual([
      `${GLOBAL_CDN_BASE}/plugins/official/floyd-webbridge.zip`,
    ]);
  });

  it('fails closed when the global region has no CDN base configured', async () => {
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(userHome, '.floyd-webbridge', 'bin'), { recursive: true });
    const binPath = path.join(userHome, '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await writeFile(binPath, 'bin');
    await chmod(binPath, 0o755);
    delete process.env[GLOBAL_CDN_BASE_ENV];
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl, urls } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({
        plugins: plugins.service,
        hostProcess: host.service,
        fetchImpl,
        resolveRegion: () => 'global',
      }),
    );

    await expect(entry.install(() => {})).rejects.toThrow(
      /Set FLOYD_CODE_GLOBAL_CDN_BASE to the CDN root URL/,
    );
    expect(plugins.installs).toEqual([]);
    expect(host.calls).toEqual([]);
    expect(urls.every((url) => url.startsWith(DAEMON_BASE))).toBe(true);
  });

  it('fails closed when the content CDN is not configured for the binary download', async () => {
    delete process.env[CONTENT_CDN_BASE_ENV];
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl, urls } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );

    await expect(entry.install(() => {})).rejects.toThrow(
      /Set FLOYD_CODE_CONTENT_CDN_BASE to the CDN root URL/,
    );
    expect(plugins.installs).toEqual([]);
    expect(urls.every((url) => url.startsWith(DAEMON_BASE))).toBe(true);
  });

  it('never starts the daemon when one is already running (coexistence)', async () => {
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );

    const note = await entry.install(() => {});
    expect(host.calls).toEqual([]);
    expect(note).toBeUndefined();
  });

  it('reinstalls the latest binary and plugin for a ready capability', async () => {
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(userHome, '.floyd-webbridge', 'bin'), { recursive: true });
    const binPath = path.join(userHome, '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await writeFile(binPath, 'old-bin');
    await chmod(binPath, 0o755);
    const plugins = fakePlugins([{ id: 'floyd-webbridge', enabled: true, state: 'ok', version: '1.11.3' }]);
    const host = fakeHostProcess();
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
      binary: new TextEncoder().encode('latest-bin'),
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );
    const reports: string[] = [];

    await entry.install((step) => reports.push(step));

    expect(reports[0]).toBe('download');
    expect(reports).toContain('skill');
    expect(host.calls).toEqual([]);
    expect(plugins.installs).toEqual([
      `${CDN_BASE}/plugins/official/floyd-webbridge.zip`,
    ]);
    expect(await readFile(binPath, 'utf8')).toBe('latest-bin');
  });

  it('resumes partial setup without repeating completed runtime layers', async () => {
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(userHome, '.floyd-webbridge', 'bin'), { recursive: true });
    const binPath = path.join(userHome, '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await writeFile(binPath, 'bin');
    await chmod(binPath, 0o755);
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );
    const reports: string[] = [];

    await entry.install((step) => reports.push(step));

    expect(reports).toEqual(['skill']);
    expect(host.calls).toEqual([]);
    expect(plugins.installs).toHaveLength(1);
  });

  it('refreshes the wiring plugin when daemon recovery is the only missing layer', async () => {
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(userHome, '.floyd-webbridge', 'bin'), { recursive: true });
    const binPath = path.join(userHome, '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await writeFile(binPath, 'bin');
    await chmod(binPath, 0o755);
    const plugins = fakePlugins([
      { id: 'floyd-webbridge', enabled: true, state: 'ok', version: '1.11.3' },
    ]);
    const host = fakeHostProcess();
    const { fetchImpl } = fakeFetch({
      statusSequence: [
        { running: false },
        { running: false },
        { running: true, version: 'v1.11.3', extension_connected: true },
      ],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );

    await entry.install(() => {});

    expect(plugins.installs).toEqual([
      `${CDN_BASE}/plugins/official/floyd-webbridge.zip`,
    ]);
    expect(host.calls.map((call) => `${call.command} ${call.args.join(' ')}`)).toEqual([
      `${binPath} start`,
    ]);
  });

  it('rejects install on unsupported platforms before any side effect', async () => {
    const plugins = fakePlugins([]);
    const entry = createFloydWebbridgeEntry(
      makeCtx({ platform: 'freebsd', plugins: plugins.service }),
    );
    await expect(entry.install(() => {})).rejects.toThrow(/not supported/);
    expect(plugins.installs).toEqual([]);
  });
  it('treats a non-executable leftover binary as missing and re-downloads it', async () => {
    const userHome = path.join(root, 'user-home');
    await mkdir(path.join(userHome, '.floyd-webbridge', 'bin'), { recursive: true });
    const binPath = path.join(userHome, '.floyd-webbridge', 'bin', 'floyd-webbridge');
    await writeFile(binPath, 'stale');
    await chmod(binPath, 0o644);
    const plugins = fakePlugins([]);
    const host = fakeHostProcess();
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(
      makeCtx({ plugins: plugins.service, hostProcess: host.service, fetchImpl }),
    );

    const detected = await entry.detect();
    expect(detected.steps.find((step) => step.id === 'daemon-binary')).toEqual({
      id: 'daemon-binary',
      state: 'missing',
      detail: 'not executable',
    });

    await entry.install(() => {});
    expect((await stat(binPath)).mode & 0o111).not.toBe(0);
  });

  it('re-enables a previously disabled wiring plugin during setup', async () => {
    const plugins = fakePlugins([{ id: 'floyd-webbridge', enabled: false, state: 'ok', version: '1.11.3' }]);
    const { fetchImpl } = fakeFetch({
      statusSequence: [{ running: true, version: 'v1.11.3', extension_connected: true }],
    });
    const entry = createFloydWebbridgeEntry(makeCtx({ plugins: plugins.service, fetchImpl }));

    await entry.install(() => {});
    expect(plugins.enabledCalls).toEqual([{ id: 'floyd-webbridge', enabled: true }]);
  });
});
