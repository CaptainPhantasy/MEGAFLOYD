import { spawn } from 'node:child_process';

import { isLoopbackHost, resolveHost } from '../config';

export interface RevealCommand {
  readonly command: string;
  readonly args: readonly string[];
}

interface RequestBindings {
  readonly incoming?: { readonly socket?: { readonly localAddress?: string } };
}

/** Resolve the platform-specific "reveal in file manager" command for
 *  the given absolute path. Kept pure (no IO) so it can be unit-tested. */
export function revealCommandFor(path: string, platform: NodeJS.Platform = process.platform): RevealCommand {
  switch (platform) {
    case 'darwin':
      return { command: 'open', args: [path] };
    case 'win32':
      // `start` is a cmd built-in; the empty title `""` prevents the path
      // from being mistaken for a window title.
      return { command: 'cmd', args: ['/c', 'start', '""', path] };
    default:
      return { command: 'xdg-open', args: [path] };
  }
}

/** Whether the request was served over a loopback interface — the local
 *  socket address that accepted it. Reads the bindings `@hono/node-server`
 *  hands to `app.fetch`, falling back to the configured bind host when the
 *  socket is unavailable (Hono's `app.request()` test harness, or a
 *  connection already torn down). */
export function isLoopbackBound(bindings: unknown, fallbackHost: string = resolveHost()): boolean {
  const incoming = (bindings as RequestBindings | undefined)?.incoming;
  const localAddress = incoming?.socket?.localAddress;
  const address = localAddress !== undefined && localAddress.length > 0 ? localAddress : fallbackHost;
  return isLoopbackHost(address);
}

/** Spawn the OS file manager to reveal `path`. Resolves once the launcher
 *  process has started; rejects only if the launcher itself fails to
 *  spawn (missing binary etc.) — not if it later fails to find the path,
 *  since the launcher exits asynchronously after we've detached. */
export async function revealInOs(path: string): Promise<void> {
  const { command, args } = revealCommandFor(path);
  return new Promise<void>((resolve, reject) => {
    let settled = false;
    const child = spawn(command, args, { detached: true, stdio: 'ignore' });
    child.once('error', (err) => {
      if (settled) return;
      settled = true;
      reject(err);
    });
    child.once('spawn', () => {
      if (settled) return;
      settled = true;
      child.unref();
      resolve();
    });
  });
}
