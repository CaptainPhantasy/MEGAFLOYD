#!/usr/bin/env node
/** Structural verification for the built MEGAFLOYD.app. */
import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.resolve(HERE, '..', '..');
const APP = path.join(ROOT, 'MEGAFLOYD.app');
const CONTENTS = path.join(APP, 'Contents');
const NL = String.fromCharCode(10);

const checks = [];
const check = (name, fn) => {
  try {
    const detail = fn();
    checks.push({ name, ok: true, detail });
  } catch (error) {
    checks.push({ name, ok: false, detail: String(error?.message ?? error) });
  }
};

const plistValue = (plist, key) =>
  execFileSync('plutil', ['-extract', key, 'raw', '-o', '-', plist], { encoding: 'utf8' }).trim();

check('app bundle exists', () => {
  if (!fs.existsSync(APP)) throw new Error('MEGAFLOYD.app missing');
  return APP;
});

check('bundle identity', () => {
  const plist = path.join(CONTENTS, 'Info.plist');
  const expected = {
    CFBundleName: 'MEGAFLOYD',
    CFBundleDisplayName: 'MEGAFLOYD',
    CFBundleExecutable: 'MEGAFLOYD',
    CFBundleIdentifier: 'ai.legacy.megafloyd',
    CFBundleShortVersionString: '1.0.0',
  };
  for (const [key, value] of Object.entries(expected)) {
    const actual = plistValue(plist, key);
    if (actual !== value) throw new Error(`${key}=${actual} (want ${value})`);
  }
  return 'ai.legacy.megafloyd 1.0.0';
});

check('executable present', () => {
  const bin = path.join(CONTENTS, 'MacOS', 'MEGAFLOYD');
  if (!fs.existsSync(bin)) throw new Error('MacOS/MEGAFLOYD missing');
  fs.accessSync(bin, fs.constants.X_OK);
  return bin;
});

check('helpers renamed', () => {
  const frameworks = path.join(CONTENTS, 'Frameworks');
  const names = fs.readdirSync(frameworks).filter((name) => name.endsWith('.app'));
  if (names.length < 4) throw new Error(`only ${names.length} helper apps`);
  const stale = names.filter((name) => name.startsWith('Kimi'));
  if (stale.length > 0) throw new Error(`stale helpers: ${stale.join(', ')}`);
  return names.join(', ');
});

check('kimi cloud resources removed', () => {
  const resources = path.join(CONTENTS, 'Resources');
  const forbidden = ['app-update.yml', 'resources', 'app.asar.unpacked'];
  for (const name of forbidden) {
    if (fs.existsSync(path.join(resources, name))) throw new Error(`${name} still present`);
  }
  const lproj = fs.readdirSync(resources).filter((name) => name.endsWith('.lproj'));
  if (lproj.length > 0) throw new Error(`localization dirs remain: ${lproj.length}`);
  return 'updater, gateway bundle, unpacked natives, localizations removed';
});

check('app.asar contents', () => {
  const asar = path.join(CONTENTS, 'Resources', 'app.asar');
  if (!fs.existsSync(asar)) throw new Error('app.asar missing');
  const listing = execFileSync('npx', ['--yes', '@electron/asar', 'list', asar], { encoding: 'utf8', cwd: ROOT });
  const required = ['/src/main.mjs', '/src/engine.mjs', '/src/preload.cjs', '/src/home.html', '/package.json'];
  for (const entry of required) {
    if (!listing.split(NL).includes(entry)) throw new Error(`missing ${entry}`);
  }
  return `${required.length} required entries`;
});

check('platform binary embedded', () => {
  const bin = path.join(CONTENTS, 'Resources', 'megafloyd', 'bin', 'megafloyd');
  if (!fs.existsSync(bin)) throw new Error('megafloyd binary missing');
  fs.accessSync(bin, fs.constants.X_OK);
  return `${(fs.statSync(bin).size / 1e6).toFixed(1)} MB`;
});

check('platform web embedded', () => {
  const script = path.join(CONTENTS, 'Resources', 'megafloyd', 'web', 'dist', 'main.mjs');
  const bundle = path.join(CONTENTS, 'Resources', 'megafloyd', 'web', 'dist-web', 'index.html');
  const node = path.join(CONTENTS, 'Resources', 'megafloyd', 'bin', 'node');
  for (const file of [script, bundle, node]) {
    if (!fs.existsSync(file)) throw new Error(`missing ${file}`);
  }
  fs.accessSync(node, fs.constants.X_OK);
  return `${(fs.statSync(script).size / 1e6).toFixed(1)} MB server + bundle`;
});

check('asar integrity fuse disabled', () => {
  const out = execFileSync('npx', ['--yes', '@electron/fuses', 'read', '--app', APP], { encoding: 'utf8', cwd: ROOT });
  if (!out.includes('EnableEmbeddedAsarIntegrityValidation is Disabled')) {
    throw new Error('fuse still enabled');
  }
  return 'fuse off';
});

check('status page installed', () => {
  const home = path.join(CONTENTS, 'Resources', 'home.html');
  if (!fs.existsSync(home)) throw new Error('Resources/home.html missing');
  return `${fs.statSync(home).size} bytes`;
});

check('icon installed', () => {
  const icon = path.join(CONTENTS, 'Resources', 'icon.icns');
  if (!fs.existsSync(icon)) throw new Error('icon.icns missing');
  return `${fs.statSync(icon).size} bytes`;
});

check('provenance recorded', () => {
  const file = path.join(CONTENTS, 'Resources', 'provenance.json');
  if (!fs.existsSync(file)) throw new Error('provenance.json missing');
  const data = JSON.parse(fs.readFileSync(file, 'utf8'));
  if (data.bundleId !== 'ai.legacy.megafloyd') throw new Error('bundle id not recorded');
  if (!data.platform?.commit) throw new Error('platform commit not recorded');
  if (!data.base?.electron) throw new Error('base Electron version not recorded');
  return `${data.base.product} ${data.base.version} / Electron ${data.base.electron} / ${data.platform.commit.slice(0, 7)}`;
});

check('code signature', () => {
  execFileSync('codesign', ['--verify', '--deep', '--strict', APP], { stdio: 'pipe' });
  return 'ad-hoc signature valid';
});

check('no quarantine attribute', () => {
  let list = '';
  try {
    list = execFileSync('xattr', [APP], { encoding: 'utf8' });
  } catch {
    list = '';
  }
  if (list.split(NL).map((line) => line.trim()).includes('com.apple.quarantine')) {
    throw new Error('com.apple.quarantine still set');
  }
  return 'clean';
});

for (const item of checks) {
  process.stdout.write(`${item.ok ? 'PASS' : 'FAIL'}  ${item.name}  ${item.detail}${NL}`);
}
const failed = checks.filter((item) => !item.ok);
process.stdout.write(`${NL}${checks.length - failed.length}/${checks.length} checks passed${NL}`);
process.exit(failed.length === 0 ? 0 : 1);
