import { createHash } from 'node:crypto';
import { readdir, readFile } from 'node:fs/promises';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const appRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const target = resolve(process.argv[2] || join(appRoot, 'dist-web'));
const manifestPath = join(appRoot, 'web-assets-manifest.json');

async function check() {
  if (process.argv.length > 3) throw new Error('Usage: node scripts/check-web-assets.mjs [bundle-directory]');
  const manifest = JSON.parse(await readFile(manifestPath, 'utf8'));
  const expected = manifest.files;
  if (manifest.version !== 1 || manifest.product !== 'MegaFloyd' || !expected || typeof expected !== 'object' || Array.isArray(expected) || !expected['index.html']) {
    throw new Error('The canonical MegaFloyd web asset manifest is invalid');
  }
  for (const [path, hash] of Object.entries(expected)) {
    if (!path || path.startsWith('/') || path.includes('\\') || path.split('/').some((part) => part === '..' || part === '.' || part === '') || !/^[a-f0-9]{64}$/.test(hash)) {
      throw new Error(`Invalid canonical web asset manifest entry: ${path}`);
    }
  }
  const actual = new Map();
  async function walk(directory) {
    for (const entry of await readdir(directory, { withFileTypes: true })) {
      if (entry.name.includes('.bak-') || entry.name === '.DS_Store') continue;
      const path = join(directory, entry.name);
      const key = relative(target, path).split('\\').join('/');
      if (entry.isDirectory()) await walk(path);
      else if (entry.isFile()) actual.set(key, createHash('sha256').update(await readFile(path)).digest('hex'));
      else throw new Error(`Unsupported web asset entry: ${key}`);
    }
  }
  await walk(target);
  const mismatches = [];
  for (const [path, hash] of Object.entries(expected)) {
    if (!actual.has(path)) mismatches.push(`missing: ${path}`);
    else if (actual.get(path) !== hash) mismatches.push(`modified: ${path}`);
  }
  for (const path of actual.keys()) if (!Object.hasOwn(expected, path)) mismatches.push(`unrecorded: ${path}`);
  if (mismatches.length) throw new Error(`Canonical MegaFloyd web assets changed:\n${mismatches.sort().join('\n')}\nRestore the preserved bundle, or explicitly update web-assets-manifest.json with the intended UI changes. Builds never refresh this manifest.`);
  console.log(`Canonical MegaFloyd web assets preserved: ${actual.size} files verified at ${target}`);
}

try {
  await check();
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
