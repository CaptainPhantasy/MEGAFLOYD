import { execFileSync } from 'node:child_process';
import { createServer } from 'node:http';
import { access, mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import { PluginManager } from '#/app/plugin/manager';

describe('PluginManager', () => {
  let home: string;
  let root: string;

  beforeEach(async () => {
    home = await mkdtemp(join(tmpdir(), 'plugin-manager-home-'));
    root = await mkdtemp(join(tmpdir(), 'plugin-manager-root-'));
    await mkdir(join(home, 'plugins'), { recursive: true });
    await mkdir(join(root, 'commands'), { recursive: true });
    await writeFile(join(root, 'commands', 'deploy.md'), '---\ndescription: Deploy\n---\n\nBody', 'utf8');
    await writeFile(
      join(root, 'floyd.plugin.json'),
      JSON.stringify({
        name: 'demo',
        commands: ['./commands'],
        hooks: [{ event: 'Stop', command: 'echo stop' }],
      }),
      'utf8',
    );
    await writeFile(
      join(home, 'plugins', 'installed.json'),
      JSON.stringify({
        version: 1,
        plugins: [
          {
            id: 'demo',
            root,
            source: 'local-path',
            enabled: true,
            installedAt: '2026-01-01T00:00:00.000Z',
          },
        ],
      }),
      'utf8',
    );
  });

  afterEach(async () => {
    vi.unstubAllGlobals();
    await rm(home, { recursive: true, force: true });
    await rm(root, { recursive: true, force: true });
  });

  it('loads installed plugins and exposes summaries, hooks, and commands', async () => {
    const manager = new PluginManager({ floydHomeDir: home });
    await manager.load();

    expect(manager.summaries()).toEqual([
      expect.objectContaining({
        id: 'demo',
        state: 'ok',
        commandCount: 1,
        hookCount: 1,
      }),
    ]);
    expect(manager.enabledHooks()).toEqual([
      {
        event: 'Stop',
        command: 'echo stop',
        cwd: root,
        env: { FLOYD_CODE_HOME: home, FLOYD_PLUGIN_ROOT: root },
      },
    ]);
    await expect(manager.enabledCommands()).resolves.toEqual([
      expect.objectContaining({ pluginId: 'demo', name: 'deploy', description: 'Deploy' }),
    ]);
  });

  it('installs a local-path plugin into the managed root', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'plugin-install-source-'));
    try {
      await writeFile(join(sourceRoot, 'floyd.plugin.json'), JSON.stringify({ name: 'other' }), 'utf8');
      const manager = new PluginManager({ floydHomeDir: home });

      const record = await manager.install(sourceRoot);

      expect(record.id).toBe('other');
      expect(record.root).toContain(join(home, 'plugins', 'managed', 'other'));
      expect(manager.get('other')?.manifest?.name).toBe('other');
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('installs a zip-url plugin', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'plugin-zip-source-'));
    const zipPath = join(tmpdir(), `plugin-${Date.now()}.zip`);
    const server = createServer((_req, res) => {
      void readFile(zipPath).then((data) => res.end(data));
    });
    try {
      await writeFile(join(sourceRoot, 'floyd.plugin.json'), JSON.stringify({ name: 'zip-plugin' }), 'utf8');
      execFileSync('zip', ['-qr', zipPath, '.'], { cwd: sourceRoot });
      await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
      const address = server.address();
      if (address === null || typeof address === 'string') throw new Error('bad server address');
      const manager = new PluginManager({ floydHomeDir: home });

      const record = await manager.install(`http://127.0.0.1:${address.port}/plugin.zip`);

      expect(record.id).toBe('zip-plugin');
      expect(record.source).toBe('zip-url');
      expect(manager.get('zip-plugin')?.manifest?.name).toBe('zip-plugin');
    } finally {
      await new Promise<void>((resolve, reject) => server.close((err) => (err === undefined ? resolve() : reject(err))));
      await rm(sourceRoot, { recursive: true, force: true });
      await rm(zipPath, { force: true });
    }
  });

  it('installs a github plugin through codeload', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'plugin-github-source-'));
    const zipPath = join(tmpdir(), `plugin-github-${Date.now()}.zip`);
    try {
      await writeFile(join(sourceRoot, 'floyd.plugin.json'), JSON.stringify({ name: 'github-plugin' }), 'utf8');
      execFileSync('zip', ['-qr', zipPath, '.'], { cwd: sourceRoot });
      const zip = await readFile(zipPath);
      const fetchMock = vi.fn(async (input: Parameters<typeof fetch>[0]) => {
        const url =
          typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.endsWith('/commits/v1.atom')) {
          return new Response(
            '<entry><id>tag:github.com,2008:Grit::Commit/1111111111111111111111111111111111111111</id></entry>',
          );
        }
        return new Response(zip);
      });
      vi.stubGlobal('fetch', fetchMock as typeof fetch);
      const manager = new PluginManager({ floydHomeDir: home });

      const record = await manager.install('https://github.com/owner/repo/tree/v1');

      expect(record.id).toBe('github-plugin');
      expect(record.source).toBe('github');
      expect(record.github).toEqual({
        owner: 'owner',
        repo: 'repo',
        ref: { kind: 'branch', value: 'v1' },
        installedSha: '1111111111111111111111111111111111111111',
      });
      expect(fetchMock).toHaveBeenCalledWith(
        'https://codeload.github.com/owner/repo/zip/1111111111111111111111111111111111111111',
        expect.objectContaining({ signal: expect.any(AbortSignal) }),
      );
      const stored = JSON.parse(
        await readFile(join(home, 'plugins', 'installed.json'), 'utf8'),
      ) as { plugins: Array<{ id: string; github?: { installedSha?: string } }> };
      expect(stored.plugins.find((plugin) => plugin.id === 'github-plugin')?.github?.installedSha)
        .toBe('1111111111111111111111111111111111111111');
      expect(manager.get('github-plugin')?.manifest?.name).toBe('github-plugin');
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
      await rm(zipPath, { force: true });
    }
  });

  it('checks github plugin updates against latest release', async () => {
    await writeFile(
      join(home, 'plugins', 'installed.json'),
      JSON.stringify({
        version: 1,
        plugins: [
          {
            id: 'demo',
            root,
            source: 'github',
            enabled: true,
            installedAt: '2026-01-01T00:00:00.000Z',
            originalSource: 'https://github.com/owner/repo',
            github: { owner: 'owner', repo: 'repo', ref: { kind: 'branch', value: 'v1' } },
          },
        ],
      }),
      'utf8',
    );
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        status: 302,
        ok: false,
        headers: new Headers({ location: 'https://github.com/owner/repo/releases/tag/v2' }),
      }),
    );
    const manager = new PluginManager({ floydHomeDir: home });
    await manager.load();

    await expect(manager.checkUpdates()).resolves.toEqual([
      {
        id: 'demo',
        source: 'github',
        current: { kind: 'branch', value: 'v1' },
        latest: { kind: 'tag', value: 'v2' },
        displayVersion: 'v2',
        updateAvailable: true,
      },
    ]);
  });

  it('reports a pinned branch update only when its commit advances', async () => {
    await writeFile(
      join(home, 'plugins', 'installed.json'),
      JSON.stringify({
        version: 1,
        plugins: [
          {
            id: 'demo',
            root,
            source: 'github',
            enabled: true,
            installedAt: '2026-01-01T00:00:00.000Z',
            originalSource: 'https://github.com/owner/repo/tree/main',
            github: {
              owner: 'owner',
              repo: 'repo',
              ref: { kind: 'branch', value: 'main' },
              installedSha: '1111111111111111111111111111111111111111',
            },
          },
        ],
      }),
      'utf8',
    );
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValueOnce(
          new Response(
            '<entry><id>tag:github.com,2008:Grit::Commit/1111111111111111111111111111111111111111</id></entry>',
          ),
        )
        .mockResolvedValueOnce(
          new Response(
            '<entry><id>tag:github.com,2008:Grit::Commit/2222222222222222222222222222222222222222</id></entry>',
          ),
        ),
    );
    const manager = new PluginManager({ floydHomeDir: home });
    await manager.load();

    await expect(manager.checkUpdates()).resolves.toEqual([
      expect.objectContaining({ id: 'demo', updateAvailable: false }),
    ]);
    await expect(manager.checkUpdates()).resolves.toEqual([
      expect.objectContaining({
        id: 'demo',
        current: { kind: 'branch', value: 'main' },
        latest: { kind: 'branch', value: 'main' },
        updateAvailable: true,
      }),
    ]);
  });

  it('treats legacy commit metadata without originalSource as pinned', async () => {
    const sha = '1111111111111111111111111111111111111111';
    await writeFile(
      join(home, 'plugins', 'installed.json'),
      JSON.stringify({
        version: 1,
        plugins: [
          {
            id: 'demo',
            root,
            source: 'github',
            enabled: true,
            installedAt: '2026-01-01T00:00:00.000Z',
            github: { owner: 'owner', repo: 'repo', ref: { kind: 'sha', value: sha } },
          },
        ],
      }),
      'utf8',
    );
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const manager = new PluginManager({ floydHomeDir: home });
    await manager.load();

    await expect(manager.checkUpdates()).resolves.toEqual([
      expect.objectContaining({
        id: 'demo',
        latest: { kind: 'sha', value: sha },
        updateAvailable: false,
      }),
    ]);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('keeps successful update results when another repository lookup fails', async () => {
    await writeFile(
      join(home, 'plugins', 'installed.json'),
      JSON.stringify({
        version: 1,
        plugins: ['good', 'offline'].map((id) => ({
          id,
          root,
          source: 'github',
          enabled: true,
          installedAt: '2026-01-01T00:00:00.000Z',
          github: {
            owner: 'owner',
            repo: id,
            ref: { kind: 'tag', value: 'v1' },
          },
        })),
      }),
      'utf8',
    );
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: Parameters<typeof fetch>[0]) => {
        const url =
          typeof input === 'string' ? input : input instanceof URL ? input.href : input.url;
        if (url.includes('/offline/')) throw new Error('network offline');
        return new Response(null, {
          status: 302,
          headers: { location: 'https://github.com/owner/good/releases/tag/v2' },
        });
      }) as typeof fetch,
    );
    const manager = new PluginManager({ floydHomeDir: home });
    await manager.load();

    await expect(manager.checkUpdates()).resolves.toEqual([
      expect.objectContaining({ id: 'good', updateAvailable: true }),
    ]);
  });

  it('persists enabled state changes', async () => {
    const manager = new PluginManager({ floydHomeDir: home });
    await manager.load();

    await manager.setEnabled('demo', false);

    expect(manager.get('demo')?.enabled).toBe(false);
    const stored = JSON.parse(await readFile(join(home, 'plugins', 'installed.json'), 'utf8')) as {
      plugins: Array<{ id: string; enabled: boolean }>;
    };
    expect(stored.plugins).toEqual([expect.objectContaining({ id: 'demo', enabled: false })]);
  });

  it('installs a Claude Code plugin by translating its manifest', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'claude-plugin-source-'));
    try {
      await mkdir(join(sourceRoot, '.claude-plugin'), { recursive: true });
      await mkdir(join(sourceRoot, 'commands'), { recursive: true });
      await writeFile(
        join(sourceRoot, '.claude-plugin', 'plugin.json'),
        JSON.stringify({
          name: 'Legacy Tool',
          version: '1.2.3',
          description: 'A Claude plugin',
          author: { name: 'Ada', email: 'ada@example.com', url: 'https://example.com/ada' },
          keywords: ['demo'],
          commands: './commands',
          hooks: { Stop: [{ matcher: 'Bash', hooks: [{ type: 'command', command: 'echo hi' }] }] },
          mcpServers: { srv: { command: 'node server.js' } },
          lspServers: { unused: {} },
        }),
        'utf8',
      );
      await writeFile(
        join(sourceRoot, 'commands', 'run.md'),
        '---\ndescription: Run\n---\n\nBody',
        'utf8',
      );
      const manager = new PluginManager({ floydHomeDir: home });

      const record = await manager.installAll(sourceRoot).then((result) => result.installed[0]!);

      expect(record.id).toBe('legacy-tool');
      expect(record.adaptedFrom).toBe('claude-code');
      expect(record.manifest?.version).toBe('1.2.3');
      expect(record.manifest?.author).toEqual({ name: 'Ada', email: 'ada@example.com' });
      expect(record.manifest?.interface?.websiteURL).toBe('https://example.com/ada');
      expect(record.manifest?.hooks).toEqual([
        { event: 'Stop', matcher: 'Bash', command: 'echo hi' },
      ]);
      expect(Object.keys(record.manifest?.mcpServers ?? {})).toEqual(['srv']);
      expect(record.diagnostics.some((d) => d.message.includes('lspServers'))).toBe(true);
      expect(record.originalSource).toBe(sourceRoot);
      await expect(access(join(sourceRoot, 'floyd.plugin.json'))).rejects.toThrow();
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('installs every local entry of a Claude pack and reports skipped external ones', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'claude-pack-source-'));
    try {
      await mkdir(join(sourceRoot, '.claude-plugin'), { recursive: true });
      await mkdir(join(sourceRoot, 'plugins', 'one', '.claude-plugin'), { recursive: true });
      await mkdir(join(sourceRoot, 'plugins', 'two', 'commands'), { recursive: true });
      await writeFile(
        join(sourceRoot, '.claude-plugin', 'marketplace.json'),
        JSON.stringify({
          name: 'pack',
          plugins: [
            { name: 'one', source: './plugins/one', description: 'First' },
            { name: 'two', source: './plugins/two', version: '0.2.0' },
            { name: 'ext', source: { source: 'github', repo: 'owner/external' } },
          ],
        }),
        'utf8',
      );
      await writeFile(
        join(sourceRoot, 'plugins', 'one', '.claude-plugin', 'plugin.json'),
        JSON.stringify({ name: 'Pack One', description: 'Inner tag' }),
        'utf8',
      );
      await writeFile(
        join(sourceRoot, 'plugins', 'two', 'commands', 'go.md'),
        '---\ndescription: Go\n---\n\nBody',
        'utf8',
      );
      const manager = new PluginManager({ floydHomeDir: home });

      const result = await manager.installAll(sourceRoot);

      expect(result.installed.map((record) => record.id)).toEqual(['pack-one', 'two']);
      expect(result.installed[0]!.adaptedFrom).toBe('claude-code-pack');
      expect(result.installed[0]!.originalSource).toBe(`${sourceRoot}#one`);
      expect(result.installed[0]!.manifest?.description).toBe('Inner tag');
      expect(result.installed[1]!.manifest?.version).toBe('0.2.0');
      expect(result.skipped).toEqual([
        {
          name: 'ext',
          reason: 'pack entry points at another repository',
          installCommand: '/plugins install https://github.com/owner/external',
        },
      ]);
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('installs a single pack entry picked with #name', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'claude-pack-pick-'));
    try {
      await mkdir(join(sourceRoot, '.claude-plugin'), { recursive: true });
      await mkdir(join(sourceRoot, 'plugins', 'a'), { recursive: true });
      await mkdir(join(sourceRoot, 'plugins', 'b'), { recursive: true });
      await writeFile(
        join(sourceRoot, '.claude-plugin', 'marketplace.json'),
        JSON.stringify({
          name: 'pack',
          plugins: [
            { name: 'a', source: './plugins/a' },
            { name: 'b', source: './plugins/b' },
          ],
        }),
        'utf8',
      );
      await writeFile(join(sourceRoot, 'plugins', 'a', 'SKILL.md'), '---\nname: a\n---\n\nA', 'utf8');
      await writeFile(join(sourceRoot, 'plugins', 'b', 'SKILL.md'), '---\nname: b\n---\n\nB', 'utf8');
      const manager = new PluginManager({ floydHomeDir: home });

      const result = await manager.installAll(`${sourceRoot}#b`);

      expect(result.installed.map((record) => record.id)).toEqual(['b']);
      expect(result.installed[0]!.adaptedFrom).toBe('claude-code-pack');
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('generates a manifest for a bare skills folder', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'skills-folder-'));
    try {
      await mkdir(join(sourceRoot, 'skills', 'alpha'), { recursive: true });
      await writeFile(
        join(sourceRoot, 'skills', 'alpha', 'SKILL.md'),
        '---\nname: alpha\ndescription: Alpha skill\n---\n\nDo alpha things.',
        'utf8',
      );
      const manager = new PluginManager({ floydHomeDir: home });

      const result = await manager.installAll(sourceRoot);

      expect(result.installed).toHaveLength(1);
      expect(result.installed[0]!.adaptedFrom).toBe('skills');
      expect(result.installed[0]!.skillCount).toBe(1);
      expect(result.installed[0]!.manifest?.skills).toEqual([expect.stringContaining('skills')]);
      await expect(access(join(sourceRoot, 'floyd.plugin.json'))).rejects.toThrow();
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('installs a Gemini CLI extension', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'gemini-extension-'));
    try {
      await mkdir(join(sourceRoot, 'commands'), { recursive: true });
      await writeFile(
        join(sourceRoot, 'gemini-extension.json'),
        JSON.stringify({
          name: 'Gem Tool',
          version: '3.1.0',
          mcpServers: { helper: { command: 'node helper.js --port 9' } },
        }),
        'utf8',
      );
      await writeFile(
        join(sourceRoot, 'GEMINI.md'),
        'Extension instructions live here.',
        'utf8',
      );
      await writeFile(
        join(sourceRoot, 'commands', 'deploy.toml'),
        'description = "Deploy it"\nprompt = """Run the deploy steps."""\n',
        'utf8',
      );
      const manager = new PluginManager({ floydHomeDir: home });

      const result = await manager.installAll(sourceRoot);

      expect(result.installed).toHaveLength(1);
      const record = result.installed[0]!;
      expect(record.id).toBe('gem-tool');
      expect(record.adaptedFrom).toBe('gemini-cli');
      expect(record.manifest?.version).toBe('3.1.0');
      expect(record.manifest?.systemPrompt).toContain('Extension instructions');
      expect(record.manifest?.mcpServers?.['helper']).toMatchObject({
        transport: 'stdio',
        command: 'node',
        args: ['helper.js', '--port', '9'],
      });
      expect(record.manifest?.commands?.[0]?.name).toBe('deploy');
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('sweeps unknown shapes for loose skills and commands instead of refusing', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'loose-pieces-'));
    try {
      await mkdir(join(sourceRoot, 'packs', 'deep'), { recursive: true });
      await writeFile(join(sourceRoot, 'tools.md'), '---\ndescription: Tools\n---\n\nBody', 'utf8');
      await writeFile(
        join(sourceRoot, 'packs', 'deep', 'SKILL.md'),
        '---\nname: deep\ndescription: Deep skill\n---\n\nDo deep things.',
        'utf8',
      );
      const manager = new PluginManager({ floydHomeDir: home });

      const result = await manager.installAll(sourceRoot);

      expect(result.report).toBeUndefined();
      expect(result.installed).toHaveLength(1);
      const record = result.installed[0]!;
      expect(record.adaptedFrom).toBe('skills');
      expect(record.skillCount).toBe(1);
      expect(record.manifest?.commands).toHaveLength(1);
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });

  it('reports a folder that holds nothing installable instead of throwing', async () => {
    const sourceRoot = await mkdtemp(join(tmpdir(), 'nothing-inside-'));
    try {
      await writeFile(join(sourceRoot, 'README.md'), 'nothing usable', 'utf8');
      const manager = new PluginManager({ floydHomeDir: home });

      const result = await manager.installAll(sourceRoot);

      expect(result.installed).toEqual([]);
      expect(result.report).toContain('Nothing installable');
      expect(result.report).toContain('README.md');
    } finally {
      await rm(sourceRoot, { recursive: true, force: true });
    }
  });
});
