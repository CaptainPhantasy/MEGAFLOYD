/**
 * Deprecated `f7 server` shim.
 *
 * The `f7 server` command tree was replaced by `f7 web` (a foreground
 * server opened in the browser). Any `f7 server …` invocation — bare or
 * with any legacy subcommand/flags — lands here, prints the deprecation
 * notice, and exits 1. The shim itself is scheduled for removal in the next
 * major version of Floyd Code.
 *
 * One subcommand stays functional: `f7 server kill`, the cleanup path for
 * background servers started by pre-0.28.0 builds (recorded in the legacy
 * single-instance lock, which the instance registry never sees).
 */

import type { Command } from 'commander';

import { registerLegacyKillCommand } from './legacy-kill';

export const DEPRECATED_SERVER_NOTICE =
  '`f7 server` has been deprecated and no longer works.\n' +
  'Use `f7 web` instead — it runs the local server in the foreground and opens the web UI (`--no-open` to skip).\n' +
  'To stop a server started by a version before 0.28.0, use `f7 server kill`.\n' +
  'This notice will be removed in the next major version of Floyd Code.\n';

export function registerDeprecatedServerCommand(program: Command): void {
  const server = program
    .command('server')
    .description('Deprecated — use `f7 web` instead.')
    // Swallow every legacy subcommand/flag (`run`, `kill`, `--port`, …) so
    // they all land in the same notice instead of a commander parse error.
    .allowUnknownOption(true)
    .allowExcessArguments(true)
    .action(() => {
      process.stderr.write(DEPRECATED_SERVER_NOTICE);
      process.exit(1);
    });
  registerLegacyKillCommand(server);
}
