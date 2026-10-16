// Verify ship-manifest.json: every declared path exists and each entry is well formed.
// A declared-but-absent artifact must fail the build rather than ship a partial package.
import { readFileSync, existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const manifest = JSON.parse(readFileSync(resolve(root, 'ship-manifest.json'), 'utf8'));

const problems = [];
if (!Array.isArray(manifest.ship_set) || manifest.ship_set.length === 0) {
  problems.push('ship_set must be a non-empty array');
}
const seen = new Set();
for (const entry of manifest.ship_set || []) {
  if (typeof entry.path !== 'string' || !entry.path) { problems.push('entry without a path'); continue; }
  if (!entry.kind) problems.push(`${entry.path}: missing kind`);
  if (seen.has(entry.path)) problems.push(`${entry.path}: duplicate entry`);
  seen.add(entry.path);
  if (!existsSync(resolve(root, entry.path))) problems.push(`${entry.path}: declared but absent`);
}

if (problems.length) {
  console.error('ship manifest FAILED:');
  for (const problem of problems) console.error('  - ' + problem);
  process.exit(1);
}
console.log(`ship manifest OK (${manifest.ship_set.length} entries)`);
