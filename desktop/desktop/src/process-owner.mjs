import { execFile } from 'node:child_process';

/** Resolve only a same-user Node listener whose current Go host is already verified. */
export async function findOwnedListener({ port, uid = process.getuid?.(), verifyRuntime, run = execFile, timeoutMs = 1500 }) {
  if (!Number.isInteger(port) || port < 1 || port > 65535 || !Number.isInteger(uid) || uid < 0) return null;
  if (!await verifyRuntime()) return null;
  const pid = await new Promise(resolve => {
    let child;
    let settled = false;
    const finish = value => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve(value);
    };
    // Resolve independently of child exit: a process stuck in kernel I/O may ignore
    // a timeout signal until the filesystem returns, but must never hold the UI.
    const timer = setTimeout(() => {
      finish(null);
      try { child?.kill('SIGKILL'); } catch { /* The lookup may already be gone. */ }
      child?.stdout?.destroy(); child?.stderr?.destroy(); child?.unref?.();
    }, timeoutMs);
    try {
      child = run('/usr/sbin/lsof', ['-nP', '+c', '0', '-a', '-u' + uid, '-iTCP:' + port, '-sTCP:LISTEN', '-Fpcu'],
        { encoding: 'utf8', maxBuffer: 64 * 1024 }, (error, output) => {
          if (error) return finish(null);
          const records = [];
          let record;
          for (const line of String(output).split('\n')) {
            if (line.startsWith('p')) { record = { pid: Number(line.slice(1)) }; records.push(record); }
            else if (record && line.startsWith('u')) record.uid = Number(line.slice(1));
            else if (record && line.startsWith('c')) record.command = line.slice(1);
          }
          const owned = records.filter(value => value.uid === uid && Number.isInteger(value.pid) && value.pid > 0 && /^(?:megafloyd-web|megafloyd|node)$/i.test(value.command || ''));
          finish(owned.length === 1 && records.length === 1 ? owned[0].pid : null);
        });
    } catch { finish(null); }
  });
  return pid && await verifyRuntime() ? pid : null;
}
