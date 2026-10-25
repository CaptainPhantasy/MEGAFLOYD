import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { cp, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { basename, dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

const appRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const checker = join(appRoot, 'scripts', 'check-web-assets.mjs');

test('the canonical custom web bundle passes and a replaced mascot is rejected', async () => {
  const source = join(appRoot, 'dist-web');
  const check = (target) => spawnSync(process.execPath, [checker, target], { encoding: 'utf8' });
  const original = check(source);
  assert.equal(original.status, 0, original.stderr);
  const temporary = await mkdtemp(join(tmpdir(), 'megafloyd-web-assets-'));
  try {
    const copy = join(temporary, 'dist-web');
    await cp(source, copy, { recursive: true, filter: (path) => !basename(path).includes('.bak-') && basename(path) !== '.DS_Store' });
    const preserved = check(copy);
    assert.equal(preserved.status, 0, preserved.stderr);
    const mascot = join(copy, 'mascot.js');
    const before = await readFile(mascot);
    await writeFile(mascot, Buffer.concat([before, Buffer.from('\nvoid "unintended replacement";\n')]));
    const replaced = check(copy);
    assert.notEqual(replaced.status, 0, 'The asset guard accepted a replaced custom mascot');
    assert.match(replaced.stderr, /mascot\.js/);
    assert.deepEqual(await readFile(join(source, 'mascot.js')), before);
  } finally {
    await rm(temporary, { recursive: true, force: true });
  }
});
