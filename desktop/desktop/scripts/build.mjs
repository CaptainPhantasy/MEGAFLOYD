#!/usr/bin/env node
/**
 * Build MEGAFLOYD.app from the pinned Kimi 3.2.17 Electron base.
 *
 * The base app is never modified. Every run copies it, renames the bundle and
 * helpers, replaces app.asar with ../../desktop, embeds the MEGAFLOYD engine,
 * strips Kimi's updater/cloud resources, and signs with the persistent local identity.
 */
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { MISMATCH_OVERRIDE_ENV, verifyEmbeddedPolicy } from './policy-check.mjs';
import { signApp, loadSigningIdentity } from './stable-sign.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..', '..');
const DESKTOP_SRC = path.join(ROOT, 'desktop');
const POLICY = path.join(ROOT, '..', 'MegaFloyd.md');
const BASE = process.env.MEGAFLOYD_BASE ?? path.join(ROOT, 'vendor', 'Kimi-3.2.17-arm64.app');
const INSTALLED_PLATFORM_BIN = path.join(ROOT, '..', 'current', 'MEGAFLOYD.app', 'Contents', 'Resources', 'megafloyd', 'bin', 'megafloyd');
const PLATFORM_BIN = process.env.MEGAFLOYD_PLATFORM_BIN ??
  (fs.existsSync(INSTALLED_PLATFORM_BIN) ? INSTALLED_PLATFORM_BIN : path.join(ROOT, 'platform-src', 'bin', 'megafloyd'));
const PLATFORM_WEB = process.env.MEGAFLOYD_PLATFORM_WEB ?? path.join(ROOT, 'platform', 'web');
const OUT = path.resolve(process.env.MEGAFLOYD_DESKTOP_OUT ?? path.join(ROOT, 'MEGAFLOYD.app'));
const BUILD_ROOT = path.resolve(process.env.MEGAFLOYD_DESKTOP_BUILD_DIR ?? path.join(ROOT, 'build'));
let BUILD;

const PRODUCT = 'MEGAFLOYD';
const DISPLAY_NAME = 'MegaFloyd';
const BUNDLE_ID = 'ai.legacy.megafloyd';
const VERSION = process.env.MEGAFLOYD_VERSION ?? '1.0.0';
const BUILD_NUMBER = process.env.MEGAFLOYD_BUILD_NUMBER ?? VERSION;
const BUILD_ID = process.env.MEGAFLOYD_BUILD_ID ?? `megafloyd-${new Date().toISOString().replace(/[^0-9TZ]/g, '')}`;
const SOURCE_COMMIT = process.env.MEGAFLOYD_SOURCE_COMMIT ?? null;

function run(cmd, args, options = {}) {
  return execFileSync(cmd, args, { stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8', ...options });
}

// These pinned build tools live outside npm's ancestor-workspace discovery.
// Populate each prefix once: npm install --prefix <TOOL_ROOT>/<name>-<version>
//   --workspaces=false --ignore-scripts --no-audit --no-fund @electron/<name>@<version>
const TOOL_ROOT = path.resolve(process.env.MEGAFLOYD_BUILD_TOOLS ?? path.join(ROOT, '..', '.tmp', 'build-desktop-tools'));
function pinnedTool(name, version, binary) {
  const prefix = path.join(TOOL_ROOT, `${name}-${version}`);
  const packageDir = path.join(prefix, 'node_modules', '@electron', name);
  const metadata = path.join(packageDir, 'package.json');
  if (!fs.existsSync(metadata) || JSON.parse(fs.readFileSync(metadata, 'utf8')).version !== version) {
    throw new Error(`Missing pinned @electron/${name}@${version}; install it under ${prefix} with --workspaces=false`);
  }
  return path.join(packageDir, binary);
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
    const helperPrefix = ['Kimi Helper', `${PRODUCT} Helper`].find(prefix => entry.startsWith(prefix));
    if (!helperPrefix || !entry.endsWith('.app')) continue;
    const oldApp = path.join(frameworks, entry);
    const suffix = entry.slice(helperPrefix.length, -'.app'.length); // "", " (GPU)", …
    const newApp = path.join(frameworks, `${PRODUCT} Helper${suffix}.app`);
    if (oldApp !== newApp) fs.renameSync(oldApp, newApp);

    const macos = path.join(newApp, 'Contents', 'MacOS');
    const oldBin = path.join(macos, `${helperPrefix}${suffix}`);
    const newBin = path.join(macos, `${PRODUCT} Helper${suffix}`);
    if (oldBin !== newBin && fs.existsSync(oldBin)) fs.renameSync(oldBin, newBin);

    const plist = path.join(newApp, 'Contents', 'Info.plist');
    plistSet(plist, 'CFBundleExecutable', 'string', `${PRODUCT} Helper${suffix}`);
    plistSet(plist, 'CFBundleDisplayName', 'string', `${DISPLAY_NAME} Helper${suffix}`);
    plistSet(plist, 'CFBundleName', 'string', `${PRODUCT} Helper${suffix}`);
    plistSet(plist, 'CFBundleIdentifier', 'string', `${BUNDLE_ID}.helper${suffix.toLowerCase().replace(/[^a-z]/g, '')}`);
    plistSet(plist, 'CFBundleShortVersionString', 'string', VERSION);
    plistSet(plist, 'CFBundleVersion', 'string', BUILD_NUMBER);

    const provision = path.join(newApp, 'Contents', 'embedded.provisionprofile');
    if (fs.existsSync(provision)) fs.rmSync(provision);
    fs.rmSync(path.join(newApp, 'Contents', '_CodeSignature'), { recursive: true, force: true });
  }
}

function verifyNativeHelperPaths(contents) {
  const realContents = fs.realpathSync.native(contents);
  const name = run('plutil', ['-extract', 'CFBundleName', 'raw', '-o', '-', path.join(realContents, 'Info.plist')]).trim();
  for (const suffix of ['', ' (Renderer)', ' (GPU)', ' (Plugin)']) {
    const helperName = `${name} Helper${suffix}`;
    const expected = path.join(realContents, 'Frameworks', `${helperName}.app`, 'Contents', 'MacOS', helperName);
    // Electron43 canonicalizes the launched path then compares it case-sensitively.
    // A case-insensitive filesystem can otherwise conceal a fatal naming mismatch.
    if (fs.realpathSync.native(expected) !== expected) {
      throw new Error(`Electron helper path differs from its physical name: ${expected}`);
    }
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
  fs.cpSync(path.join(DESKTOP_SRC, 'src'), path.join(staging, 'src'), {
    recursive: true, filter: (source) => !path.basename(source).includes('.bak'),
  });
  fs.copyFileSync(path.join(DESKTOP_SRC, 'package.json'), path.join(staging, 'package.json'));
  // package.json must point at the packed location.
  const pkg = JSON.parse(fs.readFileSync(path.join(staging, 'package.json'), 'utf8'));
  pkg.main = 'src/main.mjs';
  pkg.version = VERSION;
  pkg.megafloydBuildId = BUILD_ID;
  fs.writeFileSync(path.join(staging, 'package.json'), `${JSON.stringify(pkg, null, 2)}\n`);

  const asarOut = path.join(BUILD, 'app.asar');
  run(process.execPath, [pinnedTool('asar', '4.3.1', 'bin/asar.mjs'), 'pack', staging, asarOut], { cwd: ROOT });
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
        // pnpm deploy retains this package's self-link to the source checkout.
        // Rebase it to the bundled package so installation is self-contained.
        if (entry.name === 'floyd-code' && path.basename(dir) === '@legacy-ai') {
          fs.unlinkSync(target);
          fs.symlinkSync(path.relative(dir, root), target);
        }
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
  const signingIdentity = loadSigningIdentity();
  if (!/^\d+\.\d+\.\d+$/.test(VERSION)) throw new Error('MEGAFLOYD_VERSION must have three numeric parts');
  if (!/^[A-Za-z0-9._+-]+$/.test(BUILD_ID)) throw new Error('invalid MEGAFLOYD_BUILD_ID');
  if (path.resolve(OUT) === path.resolve(BASE)) throw new Error('desktop output must differ from its base');
  assertExists(BASE, 'base app');
  assertExists(PLATFORM_BIN, 'platform binary');
  assertExists(POLICY, 'reviewed MegaFloyd standard');
  assertExists(path.join(PLATFORM_WEB, 'dist', 'main.mjs'), 'platform web server');
  assertExists(path.join(PLATFORM_WEB, 'dist-web', 'index.html'), 'platform web bundle');
  step('proving the platform binary embeds the reviewed standard');
  const embeddedPolicy = verifyEmbeddedPolicy({
    binary: PLATFORM_BIN,
    policy: POLICY,
    allowMismatch: process.env[MISMATCH_OVERRIDE_ENV] === '1',
  });
  step('verifying preserved custom web assets');
  run(process.execPath, [path.join(ROOT, '..', 'floyd-code', 'apps', 'floyd-code', 'scripts', 'check-web-assets.mjs'), path.join(PLATFORM_WEB, 'dist-web')], { stdio: 'inherit' });

  fs.mkdirSync(BUILD_ROOT, { recursive: true });
  BUILD = fs.mkdtempSync(path.join(BUILD_ROOT, 'release-'));

  step(`copying base ${path.basename(BASE)}`);
  run('ditto', [BASE, path.join(BUILD, 'MEGAFLOYD.app')]);
  const stagingApp = path.join(BUILD, 'MEGAFLOYD.app');
  const contents = path.join(stagingApp, 'Contents');

  step('renaming bundle and helpers');
  const baseExecutable = path.join(contents, 'MacOS', 'Kimi');
  const productExecutable = path.join(contents, 'MacOS', PRODUCT);
  if (fs.existsSync(baseExecutable)) {
    fs.renameSync(baseExecutable, productExecutable);
  } else {
    assertExists(productExecutable, 'canonical MEGAFLOYD base executable');
    const baseBundleId = run('plutil', ['-extract', 'CFBundleIdentifier', 'raw', '-o', '-', path.join(contents, 'Info.plist')]).trim();
    if (baseBundleId !== BUNDLE_ID) throw new Error('base is not the canonical MEGAFLOYD app');
  }
  rebrandHelpers(contents);
  fs.rmSync(path.join(contents, '_CodeSignature'), { recursive: true, force: true });
  const provision = path.join(contents, 'embedded.provisionprofile');
  if (fs.existsSync(provision)) fs.rmSync(provision);

  step('rewriting Info.plist');
  const plist = path.join(contents, 'Info.plist');
  plistSet(plist, 'CFBundleDisplayName', 'string', DISPLAY_NAME);
  // Electron uses CFBundleName to derive exact helper paths; display branding is separate.
  plistSet(plist, 'CFBundleName', 'string', PRODUCT);
  plistSet(plist, 'CFBundleExecutable', 'string', PRODUCT);
  plistSet(plist, 'CFBundleIdentifier', 'string', BUNDLE_ID);
  plistSet(plist, 'CFBundleShortVersionString', 'string', VERSION);
  plistSet(plist, 'CFBundleVersion', 'string', BUILD_NUMBER);
  plistSet(plist, 'MEGAFLOYDBuildID', 'string', BUILD_ID);
  plistSet(plist, 'CFBundleIconFile', 'string', 'icon.icns');
  plistSet(plist, 'LSApplicationCategoryType', 'string', 'public.app-category.developer-tools');
  plistSet(plist, 'NSHumanReadableCopyright', 'string', 'Legacy AI');
  plistSet(plist, 'LSMinimumSystemVersion', 'string', '12.0');
  plistSet(plist, 'CFBundleLocalizations', 'json', '["en"]');
  const usageDescriptions = {
    NSAccessibilityUsageDescription: 'MegaFloyd needs accessibility access to show the selection toolbar.',
    NSAppleEventsUsageDescription: 'MegaFloyd needs to send Apple events to open files and apps you choose.',
    NSAudioCaptureUsageDescription: 'MegaFloyd needs audio capture to transcribe recordings.',
    NSBluetoothAlwaysUsageDescription: 'MegaFloyd needs Bluetooth to connect external devices.',
    NSBluetoothPeripheralUsageDescription: 'MegaFloyd needs Bluetooth to connect external devices.',
    NSCameraUsageDescription: 'MegaFloyd needs the camera for video calls.',
    NSLocalNetworkUsageDescription: 'MegaFloyd needs local network access to reach your services.',
    NSMicrophoneUsageDescription: 'MegaFloyd needs the microphone for voice input.',
    NSScreenCaptureUsageDescription: 'MegaFloyd needs screen recording to take screenshots.',
  };
  for (const [key, text] of Object.entries(usageDescriptions)) {
    plistSet(plist, key, 'string', text);
  }
  plistRemove(plist, 'ElectronAsarIntegrity');
  plistRemove(plist, 'CFBundleDocumentTypes');
  plistSet(plist, 'CFBundleURLTypes', 'json', JSON.stringify([{
    CFBundleTypeRole: 'Editor',
    CFBundleURLName: 'MegaFloyd Deep Link',
    CFBundleURLSchemes: ['megafloyd'],
  }]));

  step('validating exact native helper paths');
  verifyNativeHelperPaths(contents);

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
  run(process.execPath, [pinnedTool('fuses', '2.1.3', 'dist/bin.js'), 'write', '--app', stagingApp, 'EnableEmbeddedAsarIntegrityValidation=off'], {
    cwd: ROOT,
    stdio: 'inherit',
  });

  step('embedding platform');
  const platformDir = path.join(contents, 'Resources', 'megafloyd');
  fs.rmSync(platformDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(platformDir, 'bin'), { recursive: true });
  fs.copyFileSync(PLATFORM_BIN, path.join(platformDir, 'bin', 'megafloyd'));
  fs.chmodSync(path.join(platformDir, 'bin', 'megafloyd'), 0o755);
  fs.copyFileSync(POLICY, path.join(platformDir, 'MegaFloyd.md'));
  // The readable copy and the engine's embedded standard are already proven to
  // agree by verifyEmbeddedPolicy; record the hash that check compared.
  const policySHA256 = embeddedPolicy.expected;

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
  fs.writeFileSync(path.join(platformDir, 'web', 'release.json'), `${JSON.stringify({
    product: 'megafloyd', version: VERSION, build_id: BUILD_ID, source_commit: SOURCE_COMMIT,
  }, null, 2)}\n`);
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
  let platformCommit = SOURCE_COMMIT;
  try {
    const buildInfo = execFileSync('go', ['version', '-m', PLATFORM_BIN], { encoding: 'utf8' });
    platformCommit = SOURCE_COMMIT ?? buildInfo.match(/vcs\.revision=([0-9a-f]+)/)?.[1] ?? null;
  } catch {
    platformCommit = SOURCE_COMMIT;
  }
  if (!platformCommit) {
    for (const file of [path.join(ROOT, 'PLATFORM-COMMIT'), path.join(ROOT, 'platform-src', 'PLATFORM-COMMIT')]) {
      try {
        platformCommit = fs.readFileSync(file, 'utf8').trim();
        break;
      } catch {
        platformCommit = null;
      }
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
    signing: { kind: 'persistent-local-certificate', certificate_sha1: signingIdentity.certificate_sha1 },
    policy: { path: 'megafloyd/MegaFloyd.md', sha256: policySHA256, embedded_verified: embeddedPolicy.enforced },
    version: VERSION,
    buildId: BUILD_ID,
    builtAt: new Date().toISOString(),
    base: { product: 'Kimi', version: '3.2.17', electron: electronVersion },
    platform: { engine: engineVersion, productVersion: VERSION, buildId: BUILD_ID, commit: platformCommit },
  };
  fs.writeFileSync(
    path.join(contents, 'Resources', 'provenance.json'),
    `${JSON.stringify(provenance, null, 2)}\n`,
    { mode: 0o644 },
  );

  step('signing with persistent MEGAFLOYD identity');
  clearAttributes(stagingApp);
  signApp(stagingApp);

  step('publishing staged desktop');
  fs.mkdirSync(path.dirname(OUT), { recursive: true });
  const stagedOutput = `${OUT}.staging-${BUILD_ID}`;
  if (fs.existsSync(stagedOutput)) throw new Error(`staged output already exists: ${stagedOutput}`);
  run('ditto', [stagingApp, stagedOutput]);
  clearAttributes(stagedOutput);
  run('codesign', ['--verify', '--deep', '--strict', stagedOutput]);
  const backup = `${OUT}.${new Date().toISOString().replace(/[^0-9TZ]/g, '')}.bak-release`;
  const replacing = fs.existsSync(OUT);
  if (replacing) fs.renameSync(OUT, backup);
  try {
    fs.renameSync(stagedOutput, OUT);
  } catch (error) {
    if (replacing) fs.renameSync(backup, OUT);
    throw error;
  }
  fs.writeFileSync(path.join(BUILD, 'build-receipt.json'), `${JSON.stringify({
    version: VERSION, build_id: BUILD_ID, output: OUT, backup: replacing ? backup : null,
    source_commit: SOURCE_COMMIT, binary: PLATFORM_BIN, web: PLATFORM_WEB,
    policy: { path: 'Contents/Resources/megafloyd/MegaFloyd.md', sha256: policySHA256, embedded_verified: embeddedPolicy.enforced },
  }, null, 2)}\n`);

  step(`built ${OUT}`);
}

try {
  main();
} catch (error) {
  process.stderr.write(`build failed: ${error?.stack ?? error}\n`);
  process.exit(1);
}
