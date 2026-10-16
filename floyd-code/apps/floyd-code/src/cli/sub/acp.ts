/**
 * `floyd acp` sub-command.
 *
 * Starts the Agent Client Protocol (ACP) server backed directly by the
 * DI × Scope agent engine (`agent-core-v2`) over stdio, so ACP-compatible
 * clients can drive a floyd-code session.
 *
 * Wire-up:
 *  - `--login` pivots into the shared device-code login flow (the entry point
 *    ACP clients hit via the first-class `AuthMethodTerminal` path, re-invoking
 *    the agent binary with the advertised `args:['--login']`).
 *  - `FLOYD_CODE_HOME` (if set) is forwarded into `authMethods[0].env` so the
 *    login subprocess writes its token under the same data root the server
 *    reads from, and `process.argv[1]` is advertised as the legacy
 *    `_meta['terminal-auth'].command` fallback.
 *
 * `@legacy-ai/acp-server` (and its `agent-core-v2` engine) is loaded via a
 * lazy dynamic import so parsing the CLI does not initialize the ACP engine —
 * mirroring the `floyd server run` v2 routing in `#/cli/sub/server/run.ts`.
 */

import type { Command } from 'commander';

import { getVersion } from '#/cli/version';
import { FLOYD_CODE_HOME_ENV } from '#/constant/app';
import { getDataDir } from '#/utils/paths';

import { parseRegionFlag, runLoginFlow } from './login-flow';

export function registerAcpCommand(parent: Command): void {
  parent
    .command('acp')
    .description('Run floyd-code as an Agent Client Protocol (ACP) server over stdio.')
    .option(
      '--login',
      'Run the device-code login flow then exit (entry point for ACP terminal-auth).',
      false,
    )
    .option(
      '--region <region>',
      'OAuth server region slot used together with --login: "mainland-cn" or "global". Hosts come from FLOYD_CODE_OAUTH_HOST / FLOYD_CODE_GLOBAL_OAUTH_HOST; optional, and not needed to use your own provider.',
    )
    .action(async (opts: { login?: boolean; region?: string }) => {
      if (opts.login === true) {
        await runLoginFlow({
          region: opts.region === undefined ? undefined : parseRegionFlag(opts.region),
        });
        return;
      }
      // Forward `FLOYD_CODE_HOME` (if set) into `authMethods[0].env` so the
      // login subprocess clients spawn for terminal-auth writes its token
      // under the same data root the ACP server reads from.
      const sandboxHome = process.env[FLOYD_CODE_HOME_ENV];
      const terminalAuthEnv =
        sandboxHome !== undefined && sandboxHome.length > 0
          ? { [FLOYD_CODE_HOME_ENV]: sandboxHome }
          : undefined;
      // Legacy `_meta.terminal-auth` fallback for clients that don't yet
      // honor the first-class `type:'terminal'`. `command` is the absolute
      // path to this very binary so the client can spawn it for login.
      const legacyCommand = process.argv[1];
      try {
        const { runAcpServer } = await import('@legacy-ai/acp-server');
        await runAcpServer({
          homeDir: getDataDir(),
          agentInfo: { name: 'Floyd Code CLI', version: getVersion() },
          ...(terminalAuthEnv ? { terminalAuthEnv } : {}),
          ...(legacyCommand !== undefined && legacyCommand.length > 0
            ? { terminalAuthLegacyCommand: legacyCommand }
            : {}),
        });
        process.exit(0);
      } catch (error) {
        process.stderr.write(`acp server: fatal error: ${String(error)}\n`);
        process.exit(1);
      }
    });
}
