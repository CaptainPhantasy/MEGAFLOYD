import test from 'node:test';
import assert from 'node:assert/strict';
import { findOwnedListener } from '../src/process-owner.mjs';
import { fetchRuntime } from '../src/meta.mjs';

const runtime = { engine: 'go', host: 'unix:///canonical.sock', cwd: '/primary', data_dir: '/own/data', workspace_id: 'primary-id' };
const response = async () => ({ ok: true, json: async () => ({ code: 0, data: runtime }) });

test('shared runtime health checks the established Go host without an implicit primary-workspace comparison', async () => {
  const healthy = await fetchRuntime({ port: 58627, host: runtime.host, attempts: 1, fetchImpl: response });
  assert.equal(healthy.ok, true);
  const foreign = await fetchRuntime({ port: 58627, host: 'unix:///other.sock', attempts: 1, fetchImpl: response });
  assert.equal(foreign.ok, false);
  const explicitMismatch = await fetchRuntime({ port: 58627, host: runtime.host, workspace: '/selected-other', attempts: 1, fetchImpl: response });
  assert.equal(explicitMismatch.ok, false, 'an explicit workspace requirement remains enforced');
});

test('a blocked process lookup leaves timers responsive, resolves by deadline, and kills only its own lookup child', async () => {
  const killed = []; let ticks = 0;
  const interval = setInterval(() => ticks++, 2);
  const started = Date.now();
  const result = await findOwnedListener({ port: 58627, uid: 501, verifyRuntime: async () => true, timeoutMs: 30,
    run: (command, args, options, callback) => {
      assert.equal(command, '/usr/sbin/lsof'); assert.ok(args.includes('-a')); assert.ok(args.includes('-u501'));
      return { kill: signal => killed.push(signal), stdout: { destroy() {} }, stderr: { destroy() {} }, unref() {} };
    },
  });
  clearInterval(interval);
  assert.equal(result, null); assert.ok(ticks > 1, 'event loop must not block');
  assert.ok(Date.now() - started < 500, 'deadline must not wait for an uninterruptible child to exit');
  assert.deepEqual(killed, ['SIGKILL']);
});

test('PID ownership requires the current user, a Node adapter, and the same live Go host', async () => {
  const probe = output => (_cmd, _args, _options, callback) => { setImmediate(() => callback(null, output)); return { kill() {} }; };
  let checks = 0;
  assert.equal(await findOwnedListener({ port: 58627, uid: 501, verifyRuntime: async () => { checks++; return true; }, run: probe('p700\ncmegafloyd-web\nu501\n') }), 700);
  assert.equal(checks, 2, 'runtime identity must still match after resolving the PID');
  for (const output of ['p700\ncmegafloyd-web\nu502\n', 'p700\ncunrelated\nu501\n', 'p700\ncnode\nu501\np701\ncnode\nu501\n']) {
    assert.equal(await findOwnedListener({ port: 58627, uid: 501, verifyRuntime: async () => true, run: probe(output) }), null);
  }
  let invoked = false;
  assert.equal(await findOwnedListener({ port: 58627, uid: 501, verifyRuntime: async () => false, run: () => { invoked = true; } }), null);
  assert.equal(invoked, false);
});
