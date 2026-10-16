/**
 * `floyd acp`
 *
 * Verifies that the ACP sub-command is registered on the program and that
 * the action wires `@legacy-ai/acp-server`'s `runAcpServer` (the real server
 * is stubbed so the test doesn't actually take over stdio). The module is
 * loaded via a lazy dynamic import in the action, so the mock intercepts that
 * import.
 */

import { Command } from 'commander';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('@legacy-ai/acp-server', () => ({
  runAcpServer: vi.fn(async () => undefined),
}));

import { runAcpServer } from '@legacy-ai/acp-server';

import { registerAcpCommand } from '#/cli/sub/acp';
import { getDataDir } from '#/utils/paths';

class ExitCalled extends Error {
  constructor(public code: number | string | null | undefined) {
    super(`process.exit(${String(code)})`);
  }
}

describe('floyd acp', () => {
  let exitSpy: ReturnType<typeof vi.spyOn>;
  let stderrSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.mocked(runAcpServer).mockClear();
    exitSpy = vi.spyOn(process, 'exit').mockImplementation(((code?: number | string | null) => {
      throw new ExitCalled(code);
    }) as never);
    stderrSpy = vi.spyOn(process.stderr, 'write').mockImplementation(() => true);
  });

  afterEach(() => {
    exitSpy.mockRestore();
    stderrSpy.mockRestore();
    vi.unstubAllEnvs();
  });

  it('registers an `acp` subcommand on the program', () => {
    const program = new Command('floyd');
    registerAcpCommand(program);

    const acp = program.commands.find((c) => c.name() === 'acp');
    expect(acp).toBeDefined();
    expect(acp?.description()).toMatch(/Agent Client Protocol/);
  });

  it('documents --region without advertising dead login hosts', () => {
    const program = new Command('floyd');
    registerAcpCommand(program);

    const acp = program.commands.find((c) => c.name() === 'acp');
    const help = acp?.helpInformation() ?? '';
    const region = acp?.options.find((option) => option.long === '--region');
    const regionHelp = region?.description ?? '';
    const loginHelp = acp?.options.find((option) => option.long === '--login')?.description ?? '';

    expect(region).toBeDefined();
    expect(regionHelp).toMatch(/mainland-cn/);
    expect(regionHelp).toMatch(/global/);
    expect(regionHelp).toMatch(/FLOYD_CODE_OAUTH_HOST/);
    expect(regionHelp).toMatch(/--login/);
    expect(`${help}\n${regionHelp}\n${loginHelp}`).not.toMatch(/floyd\.(com|ai)/);
    expect(help).toContain('--region <region>');
    expect(help).toContain('--login');
  });

  it('invokes runAcpServer with the host options and exits 0 on success', async () => {
    const program = new Command('floyd').exitOverride();
    registerAcpCommand(program);

    await expect(program.parseAsync(['node', 'floyd', 'acp'])).rejects.toThrow(ExitCalled);

    expect(runAcpServer).toHaveBeenCalledTimes(1);
    const optsArg = vi.mocked(runAcpServer).mock.calls[0]?.[0];
    expect(optsArg).toEqual(
      expect.objectContaining({
        homeDir: getDataDir(),
        agentInfo: { name: 'Floyd Code CLI', version: expect.any(String) },
      }),
    );
    expect(exitSpy).toHaveBeenCalledWith(0);
  });

  it('forwards FLOYD_CODE_HOME to terminalAuthEnv and homeDir when set', async () => {
    const previous = process.env['FLOYD_CODE_HOME'];
    process.env['FLOYD_CODE_HOME'] = '/tmp/floyd-debug';
    try {
      const program = new Command('floyd').exitOverride();
      registerAcpCommand(program);

      await expect(program.parseAsync(['node', 'floyd', 'acp'])).rejects.toThrow(ExitCalled);

      const optsArg = vi.mocked(runAcpServer).mock.calls[0]?.[0];
      expect(optsArg).toEqual(
        expect.objectContaining({
          homeDir: '/tmp/floyd-debug',
          terminalAuthEnv: { FLOYD_CODE_HOME: '/tmp/floyd-debug' },
        }),
      );
    } finally {
      if (previous === undefined) {
        delete process.env['FLOYD_CODE_HOME'];
      } else {
        process.env['FLOYD_CODE_HOME'] = previous;
      }
    }
  });

  it('omits terminalAuthEnv when FLOYD_CODE_HOME is unset', async () => {
    const previous = process.env['FLOYD_CODE_HOME'];
    delete process.env['FLOYD_CODE_HOME'];
    try {
      const program = new Command('floyd').exitOverride();
      registerAcpCommand(program);

      await expect(program.parseAsync(['node', 'floyd', 'acp'])).rejects.toThrow(ExitCalled);

      const optsArg = vi.mocked(runAcpServer).mock.calls[0]?.[0] as {
        terminalAuthEnv?: unknown;
      };
      expect(optsArg.terminalAuthEnv).toBeUndefined();
    } finally {
      if (previous !== undefined) {
        process.env['FLOYD_CODE_HOME'] = previous;
      }
    }
  });

  it('forwards process.argv[1] as terminalAuthLegacyCommand', async () => {
    const program = new Command('floyd').exitOverride();
    registerAcpCommand(program);

    await expect(program.parseAsync(['node', 'floyd', 'acp'])).rejects.toThrow(ExitCalled);

    const optsArg = vi.mocked(runAcpServer).mock.calls[0]?.[0] as {
      terminalAuthLegacyCommand?: string;
    };
    // process.argv[1] points at the test runner entry — non-empty
    // absolute-ish path, exactly what we want forwarded.
    expect(typeof optsArg.terminalAuthLegacyCommand).toBe('string');
    expect((optsArg.terminalAuthLegacyCommand ?? '').length).toBeGreaterThan(0);
    expect(optsArg.terminalAuthLegacyCommand).toBe(process.argv[1]);
  });

  it('exits without starting the ACP server when --login is passed', async () => {
    // Stub the SDK harness so runLoginFlow doesn't hit a real OAuth endpoint:
    // harness.auth.login resolves immediately and triggers exit 0.
    const loginStub = vi.fn(async () => ({ providerName: 'floyd-code' }));
    vi.doMock(import('@legacy-ai/floyd-code-sdk'), async (importOriginal) => {
      const actual = await importOriginal();
      return {
        ...actual,
        createFloydHarness: () =>
          ({
            auth: { login: loginStub },
          }) as unknown as ReturnType<typeof actual.createFloydHarness>,
      };
    });
    vi.resetModules();
    const { registerAcpCommand: freshRegister } = await import('#/cli/sub/acp');
    try {
      const program = new Command('floyd').exitOverride();
      freshRegister(program);

      await expect(program.parseAsync(['node', 'floyd', 'acp', '--login'])).rejects.toThrow(
        ExitCalled,
      );

      expect(loginStub).toHaveBeenCalledTimes(1);
      expect(runAcpServer).not.toHaveBeenCalled();
      expect(exitSpy).toHaveBeenCalledWith(0);
    } finally {
      vi.doUnmock('@legacy-ai/floyd-code-sdk');
      vi.resetModules();
    }
  });
});
