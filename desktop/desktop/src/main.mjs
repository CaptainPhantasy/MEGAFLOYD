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
  NAVIGATION_STATE_FILE,
  PROJECT_URL,
} from './config.mjs';
import { Engine } from './engine.mjs';
import { initialNavigation, normalizeNavigation, parseDesktopRoute, readNavigation, requestHandoff, sessionFromURL, sessionURL, writeNavigation } from './navigation.mjs';
import { Log } from './log.mjs';
import { buildMenu } from './menu.mjs';
import { checkPackageUpdate, installPackageUpdate } from './updates.mjs';
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
let savedNavigation = null;
try {
  savedNavigation = readNavigation(NAVIGATION_STATE_FILE);
} catch (error) {
  log.warn('desktop navigation read failed', { message: error?.message });
}
let navigation = initialNavigation(savedNavigation, process.env, DEFAULT_WORKSPACE);
const engine = new Engine({ homeDir: HOME_DIR, workspace: navigation.cwd, goHost: navigation.host, dataDir: navigation.data_dir, log });

/** @type {BrowserWindow[]} */
const windows = new Set();
let tray = null;
let quitting = false;
let desktopReady = false;
let engineOrigin = null;
/** @type {(() => void) | null} */
let pendingRoute = null;
let pendingOpenFile = null;
let lastReadyURL = null;
let startupSession = navigation.session_id;

log.info('desktop boot', { version: APP_VERSION, home: HOME_DIR, electron: process.versions.electron });

if (!app.requestSingleInstanceLock()) {
  app.quit();
} else {
  app.on('second-instance', (_event, argv) => {
    const route = argv?.find(value => value.startsWith('megafloyd://'));
    if (route) routeURL(route);
    else if (desktopReady) showWindow();
  });

  app.on('open-url', (event, url) => {
    event.preventDefault();
    routeURL(url);
  });

  app.on('open-file', (event, filePath) => {
    event.preventDefault();
    if (!desktopReady) {
      pendingOpenFile = filePath;
      return;
    }
    routeWorkspace(showWindow(), path.dirname(filePath));
  });

  app.whenReady().then(onReady).catch((error) => {
    log.error('fatal during ready', { message: error?.message });
    dialog.showErrorBox('MegaFloyd failed to start', String(error?.stack ?? error));
    app.quit();
  });

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });

  app.on('window-all-closed', () => {
    if (process.platform !== 'darwin') app.quit();
  });

  const shutdown = () => {
    engine.release();
    app.exit(0);
  };
  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);

  app.on('will-quit', () => {
    globalShortcut.unregisterAll();
    engine.release();
  });

  app.on('before-quit', async (event) => {
    if (quitting) return;
    event.preventDefault();
    quitting = true;
    log.info('desktop quitting');
    const win = firstWindow();
    if (win) await rememberWindow(win, win.webContents.getURL());
    engine.release();
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
    applyEngineNavigation({ cwd: dir });
    startupSession = null;
  }

  engine.on('state', (state) => {
    broadcast('megafloyd:state', sanitizeState(state));
    refreshTray();
    if (state.status !== 'ready') lastReadyURL = null;
    if (state.status === 'ready' && state.url && (lastReadyURL !== state.url || pendingRoute)) {
      lastReadyURL = state.url;
      engineOrigin = new URL(state.url).origin;
      if (pendingRoute) {
        const run = pendingRoute;
        pendingRoute = null;
        run();
      } else if (startupSession) {
        const id = startupSession;
        startupSession = null;
        void routeSession(id, 0).catch(reportHandoffError);
      } else {
        const target = firstWindow();
        if (target) loadApp(target);
      }
    }
    if (state.status === 'crashed' && Notification.isSupported()) {
      new Notification({ title: 'MegaFloyd engine stopped', body: 'Restarting automatically…' }).show();
    }
  });

  createWindow();
  desktopReady = true;
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

  for (const event of ['did-navigate', 'did-navigate-in-page']) {
    contents.on(event, (_event, url, isMainFrame = true) => {
      if (isMainFrame !== false) void rememberWindow(win, url);
    });
  }
  win.on('focus', () => void rememberWindow(win, contents.getURL()));

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
  win.loadURL(sessionURL(state.url, navigation.session_id));
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
  const focused = BrowserWindow.getFocusedWindow?.();
  if (focused && windows.has(focused) && !focused.isDestroyed()) return focused;
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

function applyEngineNavigation(value) {
  const next = normalizeNavigation({ ...value, cwd: value.cwd || engine.workspace });
  if (!next) throw new Error('Workspace must be an absolute directory');
  engine.workspace = next.cwd;
  engine.goHost = next.host || null;
  engine.dataDir = next.data_dir || null;
  navigation = next;
}

function persistNavigation(value) {
  navigation = writeNavigation(NAVIGATION_STATE_FILE, value);
}

function reportHandoffError(error) {
  log.warn('desktop handoff failed', { message: error?.message });
  dialog.showErrorBox('MegaFloyd Handoff', String(error?.message ?? error));
}

function activeURL(win = firstWindow()) {
  const state = engine.snapshot();
  if (!state.url) return null;
  const current = win?.webContents.getURL();
  try {
    if (current && new URL(current).origin === new URL(state.url).origin) return current;
  } catch {}
  return sessionURL(state.url, navigation.session_id);
}

async function rememberWindow(win, rawURL) {
  const state = engine.snapshot();
  if (!state.url || !rawURL || win.isDestroyed()) return;
  try {
    if (new URL(rawURL).origin !== new URL(state.url).origin) return;
    const id = sessionFromURL(rawURL);
    if (!id) return;
    const revision = (win.__megafloydNavigationRevision || 0) + 1;
    win.__megafloydNavigationRevision = revision;
    const descriptor = await requestHandoff(state.url, id);
    if (win.isDestroyed() || firstWindow() !== win || win.__megafloydNavigationRevision !== revision || sessionFromURL(win.webContents.getURL()) !== id) return;
    win.__megafloydNavigation = normalizeNavigation(descriptor);
    persistNavigation(descriptor);
  } catch (error) {
    log.warn('desktop navigation save failed', { message: error?.message });
  }
}

async function handoff(target, win = firstWindow()) {
  if (!['web', 'tui', 'desktop'].includes(target)) throw new Error('Unknown handoff target');
  const state = engine.snapshot();
  if (state.status !== 'ready' || !state.url) throw new Error('MegaFloyd is still starting');
  const result = await requestHandoff(state.url, sessionFromURL(activeURL(win)), { target });
  persistNavigation(result);
  return result;
}

function routeURL(rawUrl) {
  const route = parseDesktopRoute(rawUrl);
  if (!route) return;
  if (route.home && path.resolve(route.home) !== HOME_DIR) {
    log.warn('deep link home differs from this instance', { home: route.home });
    if (desktopReady) reportHandoffError(new Error('Open the MegaFloyd instance for the requested home directory.'));
    return;
  }
  if (!desktopReady) {
    if (route.cwd) applyEngineNavigation(route);
    startupSession = null;
    pendingRoute = () => routeURL(rawUrl);
    return;
  }
  const win = showWindow();
  const state = engine.snapshot();
  const currentHost = state.runtime?.host || engine.goHost;
  if (route.kind === 'session') {
    if (route.cwd && route.host && currentHost && route.host !== currentHost) {
      applyEngineNavigation(route);
      pendingRoute = () => void routeSession(route.session_id, 0).catch(reportHandoffError);
      void engine.switchWorkspace().catch(reportHandoffError);
      return;
    }
    void routeSession(route.session_id, 0).catch(reportHandoffError);
    return;
  }
  if (route.kind === 'workspace' && route.cwd) {
    routeWorkspace(win, route.cwd, route);
    return;
  }
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
  const target = sessionURL(state.url, id);

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
    void win.loadURL(target);
    await rememberWindow(win, target);
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

function routeWorkspace(win, dir, context = {}) {
  if (typeof dir !== 'string' || !path.isAbsolute(dir)) {
    openHome(win);
    return;
  }
  const state = engine.snapshot();
  const next = { cwd: dir, host: context.host || state.runtime?.host || engine.goHost, data_dir: context.data_dir };
  const sameWorkspace = state.status === 'ready' && path.resolve(state.runtime?.cwd || engine.workspace) === path.resolve(dir) && (!next.host || next.host === state.runtime?.host) && (!next.data_dir || next.data_dir === state.runtime?.data_dir);
  applyEngineNavigation(next);
  persistNavigation(next);
  startupSession = null;
  log.info('desktop workspace', { dir });
  win.show();
  win.focus();
  if (sameWorkspace) loadApp(win);
  else void engine.switchWorkspace().catch(reportHandoffError);
}

function canonicalAppPath() {
  const bundled = path.resolve(process.resourcesPath, '..', '..');
  const release = path.dirname(bundled);
  if (path.basename(path.dirname(release)) === 'releases') {
    const stable = path.join(path.dirname(path.dirname(release)), 'MEGAFLOYD.app');
    try { if (fs.realpathSync(stable) === fs.realpathSync(bundled)) return stable; } catch {}
  }
  return bundled;
}

function readProvenance() {
  try {
    return JSON.parse(fs.readFileSync(PROVENANCE_FILE, 'utf8'));
  } catch {
    return null;
  }
}

let updateCheckRunning = false;

async function checkForUpdates() {
  if (updateCheckRunning) return;
  updateCheckRunning = true;
  try {
    const result = await checkPackageUpdate(process.resourcesPath);
    if (!result.update_available) {
      await dialog.showMessageBox({ type: 'info', title: 'MegaFloyd Updates', message: 'MegaFloyd is up to date.' });
      return;
    }
    const choice = await dialog.showMessageBox({
      type: 'info', title: 'MegaFloyd Updates',
      message: 'A complete MegaFloyd update is available.',
      detail: `${result.available}\nYour current session continues while the update installs.`,
      buttons: ['Install Update', 'Later'], defaultId: 0, cancelId: 1,
    });
    if (choice.response !== 0) return;
    await installPackageUpdate(process.resourcesPath);
    await dialog.showMessageBox({
      type: 'info', title: 'MegaFloyd Updated',
      message: 'The complete update is installed.',
      detail: 'Finish your current work, then quit and reopen MegaFloyd to use it.',
    });
  } catch (error) {
    dialog.showErrorBox('MegaFloyd Update', String(error?.message ?? error));
  } finally {
    updateCheckRunning = false;
  }
}

function createMenu() {
  const menu = buildMenu(Menu, {
    checkUpdates: () => void checkForUpdates(),
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
      routeWorkspace(win ?? showWindow(), result.filePaths[0]);
    },
    copyUrl: () => {
      const url = activeURL();
      if (url) clipboard.writeText(url);
    },
    openInBrowser: () => void handoff('web').catch(reportHandoffError),
    openInTerminal: () => void handoff('tui').catch(reportHandoffError),
    driveAccessSettings: () => void shell.openExternal('x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles'),
    revealApp: () => { const file = canonicalAppPath(); clipboard.writeText(file); shell.showItemInFolder(file); },
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
      { label: 'Show MegaFloyd', click: () => { const win = firstWindow() ?? createWindow(); win.show(); win.focus(); } },
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
  const dir = result.canceled ? null : result.filePaths[0] ?? null;
  if (dir) routeWorkspace(win, dir);
  return dir;
});

ipcMain.handle('megafloyd:handoff', async (event, target) => {
  if (!trusted(event)) return null;
  return handoff(target, BrowserWindow.fromWebContents(event.sender));
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
