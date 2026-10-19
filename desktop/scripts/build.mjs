#!/usr/bin/env node
/**
 * Build MEGAFLOYD.app from the pinned Kimi 3.2.17 Electron base.
 *
 * The base app is never modified. Every run copies it, renames the bundle and
 * helpers, replaces app.asar with ../../desktop, embeds the MEGAFLOYD engine,
 * strips Kimi's updater/cloud resources, and ad-hoc signs the result.
 */
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..', '..');
const DESKTOP_SRC = path.join(ROOT, 'desktop');
const BASE = process.env.MEGAFLOYD_BASE ?? path.join(ROOT, 'vendor', 'Kimi-3.2.17-arm64.app');
const PLATFORM_BIN = process.env.MEGAFLOYD_PLATFORM_BIN ?? path.join(ROOT, 'platform-src', 'bin', 'megafloyd');
const PLATFORM_WEB = process.env.MEGAFLOYD_PLATFORM_WEB ?? path.join(ROOT, 'platform', 'web');
const OUT = path.join(ROOT, 'MEGAFLOYD.app');
const BUILD = path.join(ROOT, 'build');

const PRODUCT = 'MEGAFLOYD';
const BUNDLE_ID = 'ai.legacy.megafloyd';
const VERSION = '1.0.0';
const BUILD_NUMBER = '1';

function run(cmd, args, options = {}) {
  return execFileSync(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', ...options });
}

function step(message) {
  process.stdout.write(`• ${message}\n`);
}

function plistSet(plist, key, type, value) {
  run('plutil', ['-replace', key, `-${type}`, String(value), plist]);
}

function plistRemove(plist, key) {
  try {
    run('plutil', ['-remove', key, plist]);
  } catch {
    // Key was absent.
  }
}

function assertExists(target, label) {
  if (!fs.existsSync(target)) throw new Error(`${label} not found: ${target}`);
}

function rebrandHelpers(contents) {
  const frameworks = path.join(contents, 'Frameworks');
  for (const entry of fs.readdirSync(frameworks)) {
    if (!entry.startsWith('Kimi Helper') || !entry.endsWith('.app')) continue;
    const oldApp = path.join(frameworks, entry);
    const suffix = entry.slice('Kimi Helper'.length, -'.app'.length); // "", " (GPU)", …
    const newApp = path.join(frameworks, `${PRODUCT} Helper${suffix}.app`);
    fs.renameSync(oldApp, newApp);

    const macos = path.join(newApp, 'Contents', 'MacOS');
    const oldBin = path.join(macos, `Kimi Helper${suffix}`);
    const newBin = path.join(macos, `${PRODUCT} Helper${suffix}`);
    if (fs.existsSync(oldBin)) fs.renameSync(oldBin, newBin);

    const plist = path.join(newApp, 'Contents', 'Info.plist');
    plistSet(plist, 'CFBundleExecutable', 'string', `${PRODUCT} Helper${suffix}`);
    plistSet(plist, 'CFBundleDisplayName', 'string', `${PRODUCT} Helper${suffix}`);
    plistSet(plist, 'CFBundleName', 'string', `${PRODUCT} Helper${suffix}`);
    plistSet(plist, 'CFBundleIdentifier', 'string', `${BUNDLE_ID}.helper${suffix.toLowerCase().replace(/[^a-z]/g, '')}`);
    plistSet(plist, 'CFBundleShortVersionString', 'string', VERSION);
    plistSet(plist, 'CFBundleVersion', 'string', BUILD_NUMBER);

    const provision = path.join(newApp, 'Contents', 'embedded.provisionprofile');
    if (fs.existsSync(provision)) fs.rmSync(provision);
    fs.rmSync(path.join(newApp, 'Contents', '_CodeSignature'), { recursive: true, force: true });
  }
}

function stripKimiResources(contents) {
  const resources = path.join(contents, 'Resources');
  const remove = [
    'app-update.yml',
    'app.asar.unpacked',
    'resources',
    'embedded.provisionprofile',
  ];
  for (const name of remove) {
    fs.rmSync(path.join(resources, name), { recursive: true, force: true });
  }
  for (const entry of fs.readdirSync(resources)) {
    if (entry.endsWith('.lproj')) fs.rmSync(path.join(resources, entry), { recursive: true, force: true });
    else if (entry.endsWith('.icns') && entry !== 'icon.icns') fs.rmSync(path.join(resources, entry), { force: true });
  }
}

function packAsar() {
  const staging = fs.mkdtempSync(path.join(os.tmpdir(), 'megafloyd-asar-'));
  fs.cpSync(path.join(DESKTOP_SRC, 'src'), path.join(staging, 'src'), { recursive: true });
  fs.copyFileSync(path.join(DESKTOP_SRC, 'package.json'), path.join(staging, 'package.json'));
  // package.json must point at the packed location.
  const pkg = JSON.parse(fs.readFileSync(path.join(staging, 'package.json'), 'utf8'));
  pkg.main = 'src/main.mjs';
  fs.writeFileSync(path.join(staging, 'package.json'), `${JSON.stringify(pkg, null, 2)}\n`);

  const asarOut = path.join(BUILD, 'app.asar');
  run('npx', ['--yes', '@electron/asar', 'pack', staging, asarOut], { cwd: ROOT });
  fs.rmSync(staging, { recursive: true, force: true });
  return asarOut;
}

function clearAttributes(target) {
  try {
    run('xattr', ['-cr', target]);
  } catch (error) {
    // Dangling pnpm workspace links make xattr exit 1; attributes are still cleared.
    const line = String(error?.stderr ?? '').trim().split(String.fromCharCode(10))[0];
    process.stdout.write(`  xattr warning: ${line}
`);
  }
}

function signApp(appPath) {
  run('codesign', ['--force', '--deep', '--sign', '-', appPath], { stdio: 'inherit' });
}

/**
 * Remove symlinks whose targets are absent. pnpm's deployed tree includes
 * workspace self-links that point outside the bundle; codesign (and Finder)
 * treat those as broken members of the app.
 */
function pruneDanglingSymlinks(root) {
  let pruned = 0;
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const target = path.join(dir, entry.name);
      if (entry.isSymbolicLink()) {
        if (!fs.existsSync(target)) {
          fs.rmSync(target, { force: true });
          pruned += 1;
        }
        continue;
      }
      if (entry.isDirectory()) walk(target);
    }
  };
  walk(root);
  if (pruned > 0) step(`pruned ${pruned} dangling symlink(s)`);
}

function fixSpawnHelpers(root) {
  let fixed = 0;
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      const target = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        walk(target);
        continue;
      }
      if (entry.isFile() && entry.name === 'spawn-helper') {
        fs.chmodSync(target, 0o755);
        fixed += 1;
      }
    }
  };
  walk(root);
  if (fixed > 0) step(`made ${fixed} spawn-helper executable`);
}

function main() {
  assertExists(BASE, 'base app');
  assertExists(PLATFORM_BIN, 'platform binary');
  assertExists(path.join(PLATFORM_WEB, 'dist', 'main.mjs'), 'platform web server');
  assertExists(path.join(PLATFORM_WEB, 'dist-web', 'index.html'), 'platform web bundle');

  fs.rmSync(BUILD, { recursive: true, force: true });
  fs.mkdirSync(BUILD, { recursive: true });

  step(`copying base ${path.basename(BASE)}`);
  run('ditto', [BASE, path.join(BUILD, 'MEGAFLOYD.app')]);
  const stagingApp = path.join(BUILD, 'MEGAFLOYD.app');
  const contents = path.join(stagingApp, 'Contents');

  step('renaming bundle and helpers');
  fs.renameSync(path.join(contents, 'MacOS', 'Kimi'), path.join(contents, 'MacOS', PRODUCT));
  rebrandHelpers(contents);
  fs.rmSync(path.join(contents, '_CodeSignature'), { recursive: true, force: true });
  const provision = path.join(contents, 'embedded.provisionprofile');
  if (fs.existsSync(provision)) fs.rmSync(provision);

  step('rewriting Info.plist');
  const plist = path.join(contents, 'Info.plist');
  plistSet(plist, 'CFBundleDisplayName', 'string', PRODUCT);
  plistSet(plist, 'CFBundleName', 'string', PRODUCT);
  plistSet(plist, 'CFBundleExecutable', 'string', PRODUCT);
  plistSet(plist, 'CFBundleIdentifier', 'string', BUNDLE_ID);
  plistSet(plist, 'CFBundleShortVersionString', 'string', VERSION);
  plistSet(plist, 'CFBundleVersion', 'string', BUILD_NUMBER);
  plistSet(plist, 'CFBundleIconFile', 'string', 'icon.icns');
  plistSet(plist, 'LSApplicationCategoryType', 'string', 'public.app-category.developer-tools');
  plistSet(plist, 'NSHumanReadableCopyright', 'string', 'Legacy AI');
  plistSet(plist, 'LSMinimumSystemVersion', 'string', '12.0');
  plistSet(plist, 'CFBundleLocalizations', 'json', '["en"]');
  plistRemove(plist, 'ElectronAsarIntegrity');
  plistRemove(plist, 'CFBundleDocumentTypes');
  plistSet(plist, 'CFBundleURLTypes', 'json', JSON.stringify([{
    CFBundleTypeRole: 'Editor',
    CFBundleURLName: 'MEGAFLOYD Deep Link',
    CFBundleURLSchemes: ['megafloyd'],
  }]));

  step('stripping Kimi cloud resources');
  stripKimiResources(contents);
  const updaterCache = path.join(contents, 'Resources', 'app-update.yml');
  if (fs.existsSync(updaterCache)) fs.rmSync(updaterCache);

  step('installing status page');
  fs.copyFileSync(path.join(DESKTOP_SRC, 'src', 'home.html'), path.join(contents, 'Resources', 'home.html'));

  step('installing icon');
  fs.copyFileSync(path.join(DESKTOP_SRC, 'assets', 'icon.icns'), path.join(contents, 'Resources', 'icon.icns'));

  step('packing app.asar');
  const asar = packAsar();
  fs.copyFileSync(asar, path.join(contents, 'Resources', 'app.asar'));

  step('disabling asar integrity fuse');
  run('npx', ['--yes', '@electron/fuses', 'write', '--app', stagingApp, 'EnableEmbeddedAsarIntegrityValidation=off'], {
    cwd: ROOT,
    stdio: 'inherit',
  });

  step('embedding platform');
  const platformDir = path.join(contents, 'Resources', 'megafloyd');
  fs.mkdirSync(path.join(platformDir, 'bin'), { recursive: true });
  fs.copyFileSync(PLATFORM_BIN, path.join(platformDir, 'bin', 'megafloyd'));
  fs.chmodSync(path.join(platformDir, 'bin', 'megafloyd'), 0o755);

  const nodeWrapper = path.join(platformDir, 'bin', 'node');
  fs.writeFileSync(
    nodeWrapper,
    [
      '#!/bin/sh',
      'set -eu',
      'here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)',
      'export ELECTRON_RUN_AS_NODE=1',
      'exec "$here/../../../MacOS/MEGAFLOYD" "$@"',
      '',
    ].join(String.fromCharCode(10)),
  );
  fs.chmodSync(nodeWrapper, 0o755);

  run('ditto', [PLATFORM_WEB, path.join(platformDir, 'web')]);
  // pnpm's deploy tree carries workspace self-links whose targets are not part
  // of the bundle; codesign refuses the app while any dangling link remains.
  pruneDanglingSymlinks(path.join(platformDir, 'web'));
  // node-pty ships `spawn-helper` without its executable bit when install
  // scripts are skipped, and every pty spawn then fails with EACCES.
  fixSpawnHelpers(path.join(platformDir, 'web'));

  step('recording provenance');
  const frameworkPlist = path.join(
    BASE,
    'Contents',
    'Frameworks',
    'Electron Framework.framework',
    'Versions',
    'Current',
    'Resources',
    'Info.plist',
  );
  let electronVersion = null;
  for (const key of ['CFBundleShortVersionString', 'CFBundleVersion']) {
    try {
      electronVersion = run('plutil', ['-extract', key, 'raw', frameworkPlist]).trim();
      break;
    } catch {
      electronVersion = null;
    }
  }
  let platformCommit = null;
  for (const file of [path.join(ROOT, 'PLATFORM-COMMIT'), path.join(ROOT, 'platform-src', 'PLATFORM-COMMIT')]) {
    try {
      platformCommit = fs.readFileSync(file, 'utf8').trim();
      break;
    } catch {
      platformCommit = null;
    }
  }
  let engineVersion = null;
  try {
    engineVersion = JSON.parse(fs.readFileSync(path.join(PLATFORM_WEB, 'package.json'), 'utf8')).version ?? null;
  } catch {
    engineVersion = null;
  }
  const provenance = {
    product: PRODUCT,
    bundleId: BUNDLE_ID,
    version: VERSION,
    builtAt: new Date().toISOString(),
    base: { product: 'Kimi', version: '3.2.17', electron: electronVersion },
    platform: { engine: engineVersion, commit: platformCommit },
  };
  fs.writeFileSync(
    path.join(contents, 'Resources', 'provenance.json'),
    `${JSON.stringify(provenance, null, 2)}\n`,
    { mode: 0o644 },
  );

  step('signing (ad-hoc)');
  clearAttributes(stagingApp);
  signApp(stagingApp);

  step('installing build');
  fs.rmSync(OUT, { recursive: true, force: true });
  run('ditto', [stagingApp, OUT]);
  clearAttributes(OUT);

  step(`built ${OUT}`);
}

try {
  main();
} catch (error) {
  process.stderr.write(`build failed: ${error?.stack ?? error}\n`);
  process.exit(1);
}
