/**
 * `floyd login` — drive the OAuth device-code flow non-interactively.
 * The `authMethods.terminal-auth.args=['login']` (legacy `_meta` path)
 * advertised by the ACP server points clients at this entry point. The
 * first-class ACP `args=['--login']` path enters the same flow via
 * `floyd acp --login`.
 */

import type { Command } from 'commander';

import { parseRegionFlag, runLoginFlow } from './login-flow';

export function registerLoginCommand(parent: Command): void {
  parent
    .command('login')
    .description(
      'Authenticate via the device-code flow against an operator-supplied OAuth server. To use your own model provider instead, configure [providers.*] in config.toml or run /provider.',
    )
    .option(
      '--region <region>',
      'OAuth server region slot: "mainland-cn" or "global". Hosts come from FLOYD_CODE_OAUTH_HOST / FLOYD_CODE_GLOBAL_OAUTH_HOST; optional, and not needed to use your own provider.',
    )
    .action(async (opts: { region?: string }) => {
      await runLoginFlow({
        region: opts.region === undefined ? undefined : parseRegionFlag(opts.region),
      });
    });
}
