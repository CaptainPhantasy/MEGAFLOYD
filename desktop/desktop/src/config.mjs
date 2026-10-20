import { app } from 'electron';
import os from 'node:os';
import path from 'node:path';

export const APP_NAME = 'MEGAFLOYD';
export const APP_VERSION = app.getVersion();

/**
 * MEGAFLOYD owns one home directory. The platform uses the same one, so the
 * desktop and the CLI never diverge.
 */
export const HOME_DIR =
  process.env.MEGAFLOYD_HOME && process.env.MEGAFLOYD_HOME.length > 0
    ? path.resolve(process.env.MEGAFLOYD_HOME)
    : path.join(os.homedir(), '.megafloyd');

export const DESKTOP_STATE_DIR = path.join(HOME_DIR, 'desktop');
export const LOGS_DIR = path.join(HOME_DIR, 'logs');
export const DEFAULT_PORT = 58627;
export const ENGINE_START_TIMEOUT_MS = 60_000;
export const ENGINE_STOP_TIMEOUT_MS = 8_000;
export const DEFAULT_WORKSPACE = process.env.MEGAFLOYD_WORKSPACE || os.homedir();

export const PROJECT_URL = 'https://github.com/CaptainPhantasy/MEGAFLOYD';
export const DOCS_URL = 'https://github.com/CaptainPhantasy/MEGAFLOYD#readme';

const platformRoot = path.join(process.resourcesPath, 'megafloyd');
export const WEB_SCRIPT =
  process.env.MEGAFLOYD_WEB_SCRIPT || path.join(platformRoot, 'web', 'dist', 'main.mjs');
export const WEB_BUNDLE_DIR =
  process.env.MEGAFLOYD_WEB_BUNDLE || path.join(platformRoot, 'web', 'dist-web');

export function megafloydBinaryCandidates() {
  const candidates = [];
  if (process.env.MEGAFLOYD_BIN) {
    candidates.push({ file: path.resolve(process.env.MEGAFLOYD_BIN), source: 'MEGAFLOYD_BIN' });
  }
  candidates.push({ file: path.join(platformRoot, 'bin', 'megafloyd'), source: 'bundled' });
  candidates.push({ file: path.join(HOME_DIR, 'bin', 'megafloyd'), source: 'user-install' });
  for (const dir of (process.env.PATH ?? '').split(path.delimiter)) {
    if (!dir) continue;
    candidates.push({ file: path.join(dir, 'megafloyd'), source: 'PATH' });
  }
  return candidates;
}

export function nodeWrapperCandidates() {
  const candidates = [];
  if (process.env.MEGAFLOYD_WEB_NODE) candidates.push(path.resolve(process.env.MEGAFLOYD_WEB_NODE));
  candidates.push(path.join(platformRoot, 'bin', 'node'));
  return candidates;
}
