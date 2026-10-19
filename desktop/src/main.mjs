import {
  BrowserWindow,
  Menu,
  Notification,
  Tray,
  app,
  clipboard,
  dialog,
  globalShortcut,
  nativeImage,
  ipcMain,
  screen,
  session,
  shell,
} from 'electron';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import {
  APP_NAME,
  APP_VERSION,
  DEFAULT_WORKSPACE,
  DESKTOP_STATE_DIR,
  DOCS_URL,
  HOME_DIR,
  LOGS_DIR,
  PROJECT_URL,
} from './config.mjs';
import { Engine } from './engine.mjs';
import { Log } from './log.mjs';
import { buildMenu } from './menu.mjs';
import { tokenFromUrl } from './meta.mjs';
import { sanitizeState } from './redact.mjs';
import {
  buildSessionNotice,
  chassisDbCandidates,
  findChassisDb,
  lookupChassisSession,
  lookupEngineSession,
  resolveSessionOwnership,
} from './session-store.mjs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const HOME_PAGE = app.isPackaged
  ? path.join(process.resourcesPath, 'home.html')
  : path.join(__dirname, 'home.html');
const WINDOW_STATE_FILE = path.join(DESKTOP_STATE_DIR, 'window-state.json');
const APP_ICON = path.join(process.resourcesPath, 'icon.icns');
const PROVENANCE_FILE = path.join(process.resourcesPath, 'provenance.json');
const SINGLE_INSTANCE = 'ai.legacy.megafloyd';
const TOGGLE_ACCELERATOR = 'CommandOrControl+Shift+Space';
const SMOKE = process.argv.includes('--smoke');
const SMOKE_UI = process.argv.includes('--smoke-ui');

app.setName(APP_NAME);
app.setPath('userData', DESKTOP_STATE_DIR);
app.setPath('sessionData', path.join(DESKTOP_STATE_DIR, 'session'));
if (process.platform === 'darwin') app.setAboutPanelOptions({ applicationName: APP_NAME, applicationVersion: APP_VERSION });

const log = new Log(LOGS_DIR);
const engine = new Engine({ homeDir: HOME_DIR, workspace: DEFAULT_WORKSPACE, log });

/** @type {BrowserWindow[]} */
const windows = new Set();
let tray = null;
let quitting = false;
let engineOrigin = null;
/** @type {(() => void) | null} */
let pendingRoute = null;
let pendingOpenFile = null;

log.info('desktop boot', { version: APP_VERSION, home: HOME_DIR, electron: process.versions.electron });

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', () => {
    const win = firstWindow();
    if (win) {
      if (win.isMinimized()) win.restore();
      win.show();
      win.focus();
    }
  });

  app.on('open-url', (event, url) => {
    event.preventDefault();
    routeURL(url);
  });

  app.on('open-file', (event, filePath) => {
    event.preventDefault();
    if (!app.isReady()) {
      pendingOpenFile = filePath;
      return;
    }
    routeWorkspace(showWindow(), path.dirname(filePath));
  });

  app.whenReady().then(onReady).catch((error) => {
    log.error('fatal during ready', { message: error?.message });
    dialog.showErrorBox('MEGAFLOYD failed to start', String(error?.stack ?? error));
    app.quit();
  });

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });

  app.on('window-all-closed', () => {
    if (process.platform !== 'darwin') app.quit();
  });

  const shutdown = () => {
    engine.forceStop();
    app.exit(0);
  };
  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  app.on('will-quit', () => {
    globalShortcut.unregisterAll();
    engine.forceStop();
  });

  app.on('before-quit', async (event) => {
    if (quitting) return;
    event.preventDefault();
    quitting = true;
    log.info('desktop quitting');
    await engine.stop();
    app.exit(0);
  });
}

async function onReady() {
  if (SMOKE) return runSmoke();
  if (SMOKE_UI) return runUiSmoke();
  app.setAsDefaultProtocolClient('megafloyd');

  if (process.platform === 'darwin' && app.dock) {
    const icon = nativeImage.createFromPath(APP_ICON);
    if (!icon.isEmpty()) app.dock.setIcon(icon);
  }

  installSecurityPolicy();
  createMenu();
  createTray();
  registerShortcuts();

  if (pendingOpenFile) {
    const dir = path.dirname(pendingOpenFile);
    pendingOpenFile = null;
    engine.workspace = dir;
  }

  engine.on('state', (state) => {
    broadcast('megafloyd:state', sanitizeState(state));
    refreshTray();
    if (state.status === 'ready' && state.url) {
      engineOrigin = new URL(state.url).origin;
      if (pendingRoute) {
        const run = pendingRoute;
        pendingRoute = null;
        run();
      } else {
        const target = firstWindow();
        if (target) loadApp(target);
      }
    }
    if (state.status === 'crashed' && Notification.isSupported()) {
      new Notification({ title: 'MEGAFLOYD engine stopped', body: 'Restarting automatically…' }).show();
    }
  });

  createWindow();
  await engine.start();
}

async function runSmoke() {
  const result = await new Promise((resolve) => {
    const timer = setTimeout(() => resolve({ ok: false, reason: 'timeout', state: engine.snapshot() }), 90_000);
    engine.on('state', (state) => {
      if (state.status === 'ready') {
        clearTimeout(timer);
        resolve({ ok: true, state });
      } else if (state.status === 'failed' || state.status === 'missing') {
        clearTimeout(timer);
        resolve({ ok: false, reason: state.error ?? state.status, state });
      }
    });
    void engine.start();
  });
  process.stdout.write(`${JSON.stringify(result)}
`);
  await engine.stop();
  app.exit(result.ok ? 0 : 1);
}

async function runUiSmoke() {
  createMenu();
  const win = createWindow();
  const ready = await new Promise((resolve) => {
    const timer = setTimeout(() => resolve({ ok: false, reason: 'engine timeout', state: engine.snapshot() }), 90_000);
    engine.on('state', (state) => {
      if (state.status === 'ready') {
        clearTimeout(timer);
        resolve({ ok: true, state });
      } else if (state.status === 'failed' || state.status === 'missing') {
        clearTimeout(timer);
        resolve({ ok: false, reason: state.error ?? state.status, state });
      }
    });
    void engine.start();
  });

  let ui = { loaded: false };
  if (ready.ok) {
    const loaded = new Promise((resolve) => {
      const timer = setTimeout(() => resolve({ ok: false, reason: 'load timeout' }), 45_000);
      win.webContents.once('did-finish-load', async () => {
        clearTimeout(timer);
        try {
          const title = await win.webContents.executeJavaScript('document.title');
          const bridge = await win.webContents.executeJavaScript('typeof window.megafloyd');
          const url = win.webContents.getURL();
          resolve({ ok: true, title, windowTitle: win.getTitle(), bridge, url });
        } catch (error) {
          resolve({ ok: false, reason: String(error?.message ?? error) });
        }
      });
    });
    win.__megafloydMode = 'app';
    win.loadURL(ready.state.url);
    ui = await loaded;
  }

  process.stdout.write(`${JSON.stringify({ engine: ready, ui })}
`);
  await engine.stop();
  app.exit(ready.ok && ui.ok ? 0 : 1);
}

function createWindow() {
  const bounds = readWindowState();
  const win = new BrowserWindow({
    width: bounds.width,
    height: bounds.height,
    x: bounds.x,
    y: bounds.y,
    minWidth: 900,
    minHeight: 620,
    title: APP_NAME,
    backgroundColor: '#0b0b0d',
    show: false,
    icon: APP_ICON,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webviewTag: false,
      spellcheck: true,
    },
  });

  win.__megafloydMode = 'home';
  windows.add(win);

  win.once('ready-to-show', () => win.show());
  win.on('close', () => saveWindowState(win));
  win.on('closed', () => windows.delete(win));

  installWindowPolicy(win);
  win.loadFile(HOME_PAGE);

  const state = engine.snapshot();
  if (state.status === 'ready' && state.url) {
    engineOrigin = new URL(state.url).origin;
    loadApp(win);
  }
  return win;
}

function installWindowPolicy(win) {
  const contents = win.webContents;

  contents.setWindowOpenHandler(({ url }) => {
    openExternal(url);
    return { action: 'deny' };
  });

  contents.on('will-navigate', (event, url) => {
    if (isInternal(url)) return;
    event.preventDefault();
    openExternal(url);
  });

  contents.on('will-attach-webview', (event) => event.preventDefault());

  contents.on('page-title-updated', (event) => {
    event.preventDefault();
    if (!win.isDestroyed()) win.setTitle(APP_NAME);
  });

  contents.on('render-process-gone', (_event, details) => {
    log.error('renderer gone', { reason: details.reason, exitCode: details.exitCode });
    if (!win.isDestroyed()) {
      win.__megafloydMode = 'home';
      void win.loadFile(HOME_PAGE);
    }
  });

  contents.on('did-fail-load', (_event, errorCode, errorDescription, validatedURL) => {
    if (errorCode === -3) return; // Aborted navigation, not a failure.
    log.error('load failed', { errorCode, errorDescription, validatedURL });
    if (win.__megafloydMode === 'app') {
      win.__megafloydMode = 'home';
      win.__megafloydLoadError = errorDescription;
      void win.loadFile(HOME_PAGE);
    }
  });

  contents.on('before-input-event', (event, input) => {
    if (input.type !== 'keyDown') return;
    if (input.meta && input.shift && input.key.toLowerCase() === 'i') {
      contents.toggleDevTools();
      event.preventDefault();
    }
  });
}

function loadApp(win) {
  const state = engine.snapshot();
  if (state.status !== 'ready' || !state.url) return;
  engine.clearNotice();
  win.__megafloydMode = 'app';
  win.loadURL(state.url);
}

function broadcast(channel, payload) {
  for (const win of windows) {
    if (!win.isDestroyed()) win.webContents.send(channel, payload);
  }
}

function installSecurityPolicy() {
  app.userAgentFallback = `${APP_NAME}/${APP_VERSION} (${process.platform}; Electron ${process.versions.electron})`;

  session.defaultSession.setPermissionRequestHandler((_contents, permission, callback) => {
    const allowed = new Set(['clipboard-read', 'clipboard-sanitized-write', 'notifications', 'media', 'fullscreen']);
    callback(allowed.has(permission));
  });
  session.defaultSession.setPermissionCheckHandler((_contents, permission) => {
    const allowed = new Set(['clipboard-read', 'clipboard-sanitized-write', 'notifications', 'media', 'fullscreen']);
    return allowed.has(permission);
  });
}

function isLoopbackHost(hostname) {
  return hostname === '127.0.0.1' || hostname === 'localhost' || hostname === '[::1]' || hostname === '::1';
}

function isInternal(url) {
  if (url.startsWith('file://')) return true;
  try {
    const parsed = new URL(url);
    if (isLoopbackHost(parsed.hostname)) return true;
    return engineOrigin !== null && parsed.origin === engineOrigin;
  } catch {
    return false;
  }
}

function openExternal(rawUrl) {
  try {
    const url = new URL(rawUrl);
    if (url.protocol !== 'https:' && url.protocol !== 'http:' && url.protocol !== 'mailto:') return;
    if (isLoopbackHost(url.hostname)) {
      void shell.openExternal(url.toString());
      return;
    }
    void shell.openExternal(url.toString());
  } catch {
    // Ignore malformed URLs.
  }
}

function firstWindow() {
  for (const win of windows) if (!win.isDestroyed()) return win;
  return null;
}

function showWindow() {
  const win = firstWindow() ?? createWindow();
  if (win.isMinimized()) win.restore();
  win.show();
  win.focus();
  return win;
}

function openHome(win) {
  win.__megafloydMode = 'home';
  void win.loadFile(HOME_PAGE);
  win.show();
  win.focus();
}

/**
 * Toggle the app from anywhere: the one affordance a browser tab cannot offer.
 * Registration is verified, not assumed; a dead accelerator is logged.
 */
function registerShortcuts() {
  const ok = globalShortcut.register(TOGGLE_ACCELERATOR, () => {
    const win = firstWindow();
    if (win && win.isVisible() && win.isFocused()) {
      win.hide();
      return;
    }
    showWindow();
  });
  if (!ok) log.warn('global shortcut unavailable', { accelerator: TOGGLE_ACCELERATOR });
  else log.info('global shortcut registered', { accelerator: TOGGLE_ACCELERATOR });
}

/**
 * `megafloyd://` route table (spec C12).
 *   session/<id>        -> engine UI at /sessions/<id>
 *   workspace?path=<abs>-> restart the engine with that working directory
 *   settings, anything else, unknown -> app home
 */
function routeURL(rawUrl) {
  let url;
  try {
    url = new URL(rawUrl);
  } catch {
    return;
  }
  if (url.protocol !== 'megafloyd:') return;
  const win = showWindow();
  const kind = url.hostname.toLowerCase();
  const rest = decodeURIComponent(url.pathname.replace(/^\/+/, ''));

  if (kind === 'session' && rest) {
    void routeSession(rest.split('/')[0], 0);
    return;
  }
  if (kind === 'workspace') {
    const dir = url.searchParams.get('path');
    if (dir) {
      routeWorkspace(win, dir);
      return;
    }
  }
  log.info('deep link fell back to home', { url: rawUrl });
  openHome(win);
}

async function routeSession(id, attempt) {
  const state = engine.snapshot();
  if (state.status !== 'ready' || !state.url) {
    if (attempt < 1) {
      pendingRoute = () => routeSession(id, attempt + 1);
      log.info('deep link queued until engine ready', { session: id });
      return;
    }
    log.warn('deep link dropped, engine not ready', { session: id });
    return;
  }
  const win = showWindow();
  const target = new URL(state.url);
  target.pathname = `/sessions/${encodeURIComponent(id)}`;

  // Spec C6: resolve against both stores before navigating. A session the
  // web engine does not own must not open as an empty transcript.
  const dbPath = findChassisDb(
    chassisDbCandidates({ workspace: engine.workspace, home: HOME_DIR }),
  );
  const [engineLookup, chassisLookup] = await Promise.all([
    lookupEngineSession({ port: state.port, token: tokenFromUrl(state.url), id }),
    Promise.resolve(lookupChassisSession({ dbPath, id })),
  ]);
  const owner = resolveSessionOwnership({
    engine: engineLookup.status,
    chassis: chassisLookup.status,
  });

  if (owner === 'engine' || owner === 'both') {
    engine.clearNotice();
    win.__megafloydMode = 'app';
    log.info('deep link session', { session: id, owner });
    void win.loadURL(target.toString());
    return;
  }

  engine.setNotice(
    buildSessionNotice({ id, owner, engine: engineLookup, chassis: chassisLookup, dbPath }),
  );
  log.warn('deep link session refused', {
    session: id,
    owner,
    engine: engineLookup.status,
    chassis: chassisLookup.status,
  });
  openHome(win);
}

function routeWorkspace(win, dir) {
  if (typeof dir !== 'string' || !path.isAbsolute(dir)) {
    openHome(win);
    return;
  }
  engine.workspace = dir;
  log.info('deep link workspace', { dir });
  win.show();
  win.focus();
  void engine.restart();
}

function readProvenance() {
  try {
    return JSON.parse(fs.readFileSync(PROVENANCE_FILE, 'utf8'));
  } catch {
    return null;
  }
}

function createMenu() {
  const menu = buildMenu(Menu, {
    showStatus: () => {
      const win = firstWindow() ?? createWindow();
      win.__megafloydMode = 'home';
      void win.loadFile(HOME_PAGE);
    },
    restartEngine: () => void engine.restart(),
    newWindow: () => {
      const win = createWindow();
      win.show();
    },
    chooseWorkspace: async () => {
      const win = firstWindow();
      const result = await dialog.showOpenDialog(win, {
        title: 'Open workspace',
        properties: ['openDirectory', 'createDirectory'],
      });
      if (result.canceled || result.filePaths.length === 0) return;
      clipboard.writeText(result.filePaths[0]);
      if (Notification.isSupported()) {
        new Notification({ title: 'Workspace path copied', body: result.filePaths[0] }).show();
      }
      const target = firstWindow();
      target?.show();
      target?.focus();
    },
    copyUrl: () => {
      const state = engine.snapshot();
      if (state.url) {
        clipboard.writeText(state.url);
        if (Notification.isSupported()) {
          new Notification({ title: 'Local URL copied', body: state.url }).show();
        }
      }
    },
    openInBrowser: () => {
      const state = engine.snapshot();
      if (state.url) void shell.openExternal(state.url);
    },
    reload: () => firstWindow()?.webContents.reload(),
    openLogs: () => void shell.openPath(LOGS_DIR),
    revealHome: () => void shell.openPath(HOME_DIR),
    toggleDevTools: () => firstWindow()?.webContents.toggleDevTools(),
    openDocs: () => void shell.openExternal(DOCS_URL),
    openProject: () => void shell.openExternal(PROJECT_URL),
  });
  Menu.setApplicationMenu(menu);
}

function createTray() {
  const icon = nativeImage.createFromPath(APP_ICON);
  if (icon.isEmpty()) {
    log.warn('tray icon missing', { path: APP_ICON });
    return;
  }
  tray = new Tray(icon.resize({ width: 18, height: 18 }));
  tray.setToolTip(APP_NAME);
  tray.on('click', () => {
    const win = firstWindow() ?? createWindow();
    win.show();
    win.focus();
  });
  refreshTray();
}

function refreshTray() {
  if (!tray) return;
  const state = engine.snapshot();
  const label = state.status === 'ready' ? `Engine ready · :${state.port}` : `Engine ${state.status}`;
  tray.setContextMenu(
    Menu.buildFromTemplate([
      { label: APP_NAME, enabled: false },
      { label, enabled: false },
      { type: 'separator' },
      { label: 'Show MEGAFLOYD', click: () => { const win = firstWindow() ?? createWindow(); win.show(); win.focus(); } },
      { label: 'Restart Engine', click: () => void engine.restart() },
      { type: 'separator' },
      { label: 'Quit', click: () => app.quit() },
    ]),
  );
}

function readWindowState() {
  try {
    const raw = JSON.parse(fs.readFileSync(WINDOW_STATE_FILE, 'utf8'));
    const bounds = { width: Number(raw.width), height: Number(raw.height), x: raw.x, y: raw.y };
    if (!Number.isFinite(bounds.width) || !Number.isFinite(bounds.height)) return { width: 1440, height: 900 };
    const visible = screen.getAllDisplays().some((display) => {
      const area = display.workArea;
      return (
        Number.isFinite(bounds.x) &&
        Number.isFinite(bounds.y) &&
        bounds.x < area.x + area.width &&
        bounds.x + bounds.width > area.x &&
        bounds.y < area.y + area.height &&
        bounds.y + bounds.height > area.y
      );
    });
    if (!visible) return { width: bounds.width, height: bounds.height };
    return bounds;
  } catch {
    return { width: 1440, height: 900 };
  }
}

let saveTimer = null;
function saveWindowState(win) {
  if (win.isMinimized() || win.isFullScreen()) return;
  const bounds = win.getNormalBounds();
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => {
    try {
      fs.mkdirSync(DESKTOP_STATE_DIR, { recursive: true, mode: 0o700 });
      fs.writeFileSync(
        WINDOW_STATE_FILE,
        JSON.stringify({ ...bounds, savedAt: new Date().toISOString() }, null, 2),
        { mode: 0o600 },
      );
    } catch (error) {
      log.warn('window state write failed', { message: error?.message });
    }
  }, 400);
}

function trusted(event) {
  const sender = BrowserWindow.fromWebContents(event.sender);
  return sender !== null && windows.has(sender);
}

ipcMain.handle('megafloyd:info', (event) => {
  if (!trusted(event)) return null;
  return {
    name: APP_NAME,
    version: APP_VERSION,
    electron: process.versions.electron,
    chrome: process.versions.chrome,
    node: process.versions.node,
    platform: process.platform,
    arch: process.arch,
    home: HOME_DIR,
    logs: LOGS_DIR,
    pid: process.pid,
    provenance: readProvenance(),
  };
});

ipcMain.handle('megafloyd:state', (event) =>
  trusted(event) ? sanitizeState(engine.snapshot()) : null,
);

ipcMain.handle('megafloyd:open-app', (event) => {
  if (!trusted(event)) return;
  const win = BrowserWindow.fromWebContents(event.sender);
  if (win) loadApp(win);
});

ipcMain.handle('megafloyd:restart', async (event) => {
  if (!trusted(event)) return null;
  await engine.restart();
  return engine.snapshot();
});

ipcMain.handle('megafloyd:open-logs', async (event) => {
  if (!trusted(event)) return;
  await shell.openPath(LOGS_DIR);
});

ipcMain.handle('megafloyd:open-external', async (event, url) => {
  if (!trusted(event)) return;
  openExternal(url);
});

ipcMain.handle('megafloyd:copy', (event, text) => {
  if (!trusted(event)) return;
  clipboard.writeText(text);
});

ipcMain.handle('megafloyd:choose-workspace', async (event) => {
  if (!trusted(event)) return null;
  const win = BrowserWindow.fromWebContents(event.sender);
  const result = await dialog.showOpenDialog(win, {
    title: 'Open workspace',
    properties: ['openDirectory', 'createDirectory'],
  });
  return result.canceled ? null : result.filePaths[0] ?? null;
});

/**
 * Additive bridge calls (spec C13): surfaces the web UI cannot reach from a
 * browser tab — a dock badge and a native notification that focuses the app.
 */
ipcMain.handle('megafloyd:set-badge', (event, value) => {
  if (!trusted(event)) return null;
  const count = Number.isFinite(Number(value)) ? Math.max(0, Math.floor(Number(value))) : 0;
  if (process.platform === 'darwin' && app.dock) app.dock.setBadge(count > 0 ? String(count) : '');
  return count;
});

ipcMain.handle('megafloyd:notify', (event, payload) => {
  if (!trusted(event)) return false;
  const title = typeof payload?.title === 'string' ? payload.title.slice(0, 120) : APP_NAME;
  const body = typeof payload?.body === 'string' ? payload.body.slice(0, 400) : '';
  if (!Notification.isSupported()) return false;
  const notification = new Notification({ title: title || APP_NAME, body });
  notification.on('click', () => {
    const win = showWindow();
    win.focus();
  });
  notification.show();
  return true;
});
