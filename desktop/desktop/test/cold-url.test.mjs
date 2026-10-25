import test from 'node:test';
import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';
import { handoffUrlFromLine } from '../src/meta.mjs';

const sid = '41cffa2c-231a-46d8-b609-a729e74d78a8';
const second = 'second-real-session';
const host = 'unix:///fixture/megafloyd.sock';
const navigationFile = '/fixture/desktop/navigation.json';
const tick = () => new Promise(resolve => setImmediate(resolve));

async function fixture({ saved, env = {} } = {}) {
  const app = new EventEmitter();
  let appReady = false;
  let resolveReady;
  const ready = new Promise(resolve => { resolveReady = resolve; });
  const exits = [];
  Object.assign(app, { setName() {}, setPath() {}, setAboutPanelOptions() {}, requestSingleInstanceLock: () => true, isReady: () => appReady, whenReady: () => ready, setAsDefaultProtocolClient() {}, quit() {}, exit: code => exits.push(code), dock: { setIcon() {} } });
  const created = [], loaded = [], lookups = [], errors = [], calls = [], copied = [];
  const descriptors = new Map([
    [sid, { cwd: '/fixture/workspace', data_dir: '/fixture/custom-state', host, session_id: sid }],
    [second, { cwd: '/fixture/other-workspace', data_dir: '/fixture/other-state', host, session_id: second }],
  ]);
  const files = new Map(saved ? [[navigationFile, JSON.stringify(saved)]] : []);
  const fs = {
    readFileSync(file) { if (files.has(file)) return files.get(file); throw Object.assign(new Error('missing fixture'), { code: 'ENOENT' }); },
    writeFileSync(file, content) { files.set(file, content); }, mkdirSync() {},
    renameSync(from, to) { files.set(to, files.get(from)); files.delete(from); },
  };
  class BrowserWindow extends EventEmitter {
    constructor() {
      super();
      assert.equal(appReady, true, 'BrowserWindow was created before Electron ready');
      this.url = 'file:///fixture/home.html';
      this.webContents = Object.assign(new EventEmitter(), { setWindowOpenHandler() {}, send() {}, getURL: () => this.url });
      created.push(this);
    }
    loadFile() { this.url = 'file:///fixture/home.html'; }
    async loadURL(url) { this.url = url; loaded.push(url); this.webContents.emit('did-navigate', {}, url); }
    isDestroyed() { return false; }
    isMinimized() { return false; }
    show() {}
    focus() {}
    static getAllWindows() { return created; }
    static getFocusedWindow() { return created.at(-1); }
    static fromWebContents(contents) { return created.find(win => win.webContents === contents) || null; }
  }
  let engine;
  class Engine extends EventEmitter {
    constructor(options) { super(); Object.assign(this, options); this.options = options; this.state = { status: 'idle' }; this.starts = []; this.switches = 0; this.releases = 0; engine = this; }
    snapshot() { return this.state; }
    async start() { this.starts.push({ cwd: this.workspace, host: this.goHost, data_dir: this.dataDir }); this.state = { status: 'starting' }; }
    async switchWorkspace() { this.switches++; await this.start(); }
    release() { this.releases++; }
    clearNotice() {}
  }
  let actions;
  const handlers = new Map();
  const fetch = async (url, options) => {
    const parsed = new URL(url);
    calls.push({ path: parsed.pathname, method: options.method, body: options.body });
    const id = parsed.pathname.match(/\/sessions\/([^/]+)\/handoff$/)?.[1];
    const descriptor = id ? descriptors.get(decodeURIComponent(id)) : { cwd: engine.workspace, host };
    assert.ok(descriptor, `Unexpected descriptor request ${parsed.pathname}`);
    return { ok: true, status: 200, json: async () => ({ code: 0, data: { ...descriptor, ...(options.method === 'POST' ? { ...JSON.parse(options.body), opened: true } : {}) } }) };
  };
  const context = vm.createContext({ URL, URLSearchParams, AbortSignal, console, setTimeout, clearTimeout, fetch, process: { platform: 'darwin', argv: [], env, pid: 123, versions: { electron: 'test' }, resourcesPath: '/fixture/resources', on() {} } });
  const noop = () => {};
  const mocks = {
    electron: { BrowserWindow, app, Menu: { setApplicationMenu() {} }, Notification: { isSupported: () => false }, Tray: class {}, clipboard: { writeText: value => copied.push(value) }, dialog: { showErrorBox: (...args) => errors.push(args), showOpenDialog: async () => ({ canceled: false, filePaths: ['/fixture/chosen'] }) }, globalShortcut: { register: () => true, unregisterAll() {} }, nativeImage: { createFromPath: () => ({ isEmpty: () => true }) }, ipcMain: { handle: (name, handler) => handlers.set(name, handler) }, screen: {}, session: { defaultSession: { setPermissionRequestHandler() {}, setPermissionCheckHandler() {} } }, shell: {} },
    'node:fs': { default: fs }, 'node:path': { default: path }, 'node:url': { fileURLToPath },
    './config.mjs': { APP_NAME: 'MEGAFLOYD', APP_VERSION: 'fixture', DEFAULT_WORKSPACE: '/fixture/default', DESKTOP_STATE_DIR: '/fixture/desktop', NAVIGATION_STATE_FILE: navigationFile, DOCS_URL: '', HOME_DIR: '/fixture/home', LOGS_DIR: '/fixture/logs', PROJECT_URL: '' },
    './engine.mjs': { Engine },
    './log.mjs': { Log: class { info() {} warn(...args) { if (args[0] !== 'tray icon missing') errors.push(args); } error(...args) { errors.push(args); } } },
    './menu.mjs': { buildMenu: (_menu, value) => { actions = value; return {}; } },
    './updates.mjs': { checkPackageUpdate: noop, installPackageUpdate: noop },
    './meta.mjs': { tokenFromUrl: () => undefined }, './redact.mjs': { sanitizeState: value => value },
    './session-store.mjs': { buildSessionNotice: noop, chassisDbCandidates: () => [], findChassisDb: () => null, lookupChassisSession: () => ({ status: 'missing' }), lookupEngineSession: async request => { lookups.push(request); return { status: 'found' }; }, resolveSessionOwnership: () => 'engine' },
  };
  const modules = new Map();
  async function load(file) {
    const sourceURL = new URL(file, import.meta.url);
    const module = new vm.SourceTextModule(await readFile(sourceURL, 'utf8'), { context, identifier: sourceURL.href, initializeImportMeta(meta) { meta.url = sourceURL.href; } });
    await module.link(async specifier => {
      if (specifier === './navigation.mjs') return load('../src/navigation.mjs');
      if (modules.has(specifier)) return modules.get(specifier);
      const exports = mocks[specifier]; assert.ok(exports, `Unexpected dependency ${specifier}`);
      const dependency = new vm.SyntheticModule(Object.keys(exports), function () { for (const [name, value] of Object.entries(exports)) this.setExport(name, value); }, { context });
      modules.set(specifier, dependency); return dependency;
    });
    return module;
  }
  await (await load('../src/main.mjs')).evaluate();
  return {
    app, engine, created, loaded, lookups, errors, calls, copied, files, handlers, exits,
    get actions() { return actions; },
    get saved() { return JSON.parse(files.get(navigationFile)); },
    emitURL(url) { app.emit('open-url', { preventDefault() {} }, url); },
    async ready() { appReady = true; resolveReady(); await tick(); },
    async engineReady() {
      engine.state = { status: 'ready', url: 'http://127.0.0.1:57876/sessions/', port: 57876, runtime: { host: engine.goHost || host, cwd: engine.workspace, data_dir: engine.dataDir } };
      engine.emit('state', engine.state); await tick();
    },
  };
}

test('desktop preserves cold and warm handoff identity, current-session menu actions, and shared adapter lifetime', async () => {
  const f = await fixture();
  const link = `megafloyd://session/${sid}?cwd=/fixture/workspace&host=${encodeURIComponent(host)}&data_dir=/fixture/custom-state&home=/fixture/home`;
  f.emitURL(link);
  assert.equal(f.created.length, 0);
  await f.ready();
  assert.equal(f.created.length, 1);
  assert.deepEqual(JSON.parse(JSON.stringify(f.engine.starts)), [{ cwd: '/fixture/workspace', host, data_dir: '/fixture/custom-state' }]);
  assert.equal(f.loaded.length, 0);
  await f.engineReady();
  assert.deepEqual(f.loaded, [`http://127.0.0.1:57876/sessions/${sid}`]);
  assert.equal(f.saved.session_id, sid);
  assert.equal(f.saved.data_dir, '/fixture/custom-state');
  f.actions.copyUrl();
  assert.equal(f.copied.at(-1), f.loaded.at(-1));
  f.actions.openInBrowser(); f.actions.openInTerminal(); await tick();
  assert.deepEqual(f.calls.filter(call => call.method === 'POST').map(call => [call.path, JSON.parse(call.body).target]), [
    [`/api/v1/sessions/${sid}/handoff`, 'web'], [`/api/v1/sessions/${sid}/handoff`, 'tui'],
  ]);
  f.emitURL(`megafloyd://session/${second}?cwd=/fixture/other-workspace&host=${encodeURIComponent(host)}&data_dir=/fixture/other-state`);
  await tick();
  assert.equal(f.engine.switches, 0, 'another workspace on the same host must reuse the current adapter');
  assert.equal(f.saved.cwd, '/fixture/other-workspace');
  assert.equal(f.saved.session_id, second);
  f.app.emit('before-quit', { preventDefault() {} }); await tick();
  assert.equal(f.engine.releases, 1, 'normal quit releases the shared adapter');
  assert.deepEqual(f.exits, [0]);
  assert.deepEqual(f.errors, []);
});

test('bare app icon restores the last workspace and session while rediscovering a fresh canonical Go host', async () => {
  const f = await fixture({ saved: { cwd: '/fixture/other-workspace', data_dir: '/fixture/other-state', session_id: second, host: 'unix:///expired/reboot.sock' } });
  assert.equal(f.engine.workspace, '/fixture/other-workspace');
  assert.equal(f.engine.dataDir, '/fixture/other-state');
  assert.equal(f.engine.goHost, undefined, 'an old saved socket must not override canonical discovery');
  await f.ready(); await f.engineReady();
  assert.deepEqual(f.loaded, [`http://127.0.0.1:57876/sessions/${second}`]);
  assert.equal(f.saved.host, host);
  await f.actions.chooseWorkspace();
  assert.equal(f.engine.workspace, '/fixture/chosen');
  assert.equal(f.engine.switches, 1, 'Open Workspace changes the actual selected workspace');
  assert.equal(f.saved.session_id, undefined);
  assert.deepEqual(f.errors, []);
  const override = await fixture({ saved: f.saved, env: { MEGAFLOYD_WORKSPACE: '/fixture/explicit', MEGAFLOYD_GO_HOST: host } });
  assert.equal(override.engine.workspace, '/fixture/explicit');
  assert.equal(override.engine.goHost, host);
});


test('shared adapter startup accepts its actual registered port and desktop release keeps it running', async () => {
  const kills = [], launches = [], metaPorts = [];
  const child = Object.assign(new EventEmitter(), { pid: 4242, stdout: Object.assign(new EventEmitter(), { setEncoding() {} }), stderr: Object.assign(new EventEmitter(), { setEncoding() {} }), kill: signal => kills.push(signal), unref() {} });
  const context = vm.createContext({ URL, console, setTimeout, clearTimeout, setInterval, clearInterval, process: { env: {}, kill: (...args) => kills.push(args) } });
  const mocks = {
    'node:child_process': { spawn: (file, args, options) => { launches.push({ file, args, options }); return child; }, execFileSync: () => { throw new Error('No process lookup allowed during release'); } },
    'node:events': { EventEmitter }, 'node:fs': { default: { existsSync: () => true, mkdirSync() {}, writeFileSync() {} } },
    'node:net': { default: { createServer: () => ({ unref() {}, once() {}, listen(_port, _host, done) { done(); }, address: () => ({ port: 58627 }), close: done => done() }) } },
    'node:os': { default: { homedir: () => '/fixture/home' } }, 'node:path': { default: path },
    './config.mjs': { DEFAULT_PORT: 58627, ENGINE_START_TIMEOUT_MS: 1000, ENGINE_STOP_TIMEOUT_MS: 100, HOME_DIR: '/fixture/home', WEB_BUNDLE_DIR: '/fixture/web-ui', WEB_SCRIPT: '/fixture/main.mjs', megafloydBinaryCandidates: () => [], nodeWrapperCandidates: () => [] },
    './meta.mjs': { fetchMeta: async value => { metaPorts.push(value); return { ok: true, warnings: [], runtime: { host, cwd: '/fixture/different-primary-workspace' } }; }, fetchRuntime: async () => ({ ok: true }), handoffUrlFromLine, tokenFromUrl: () => null },
    './process-owner.mjs': { findOwnedListener: async () => null },
    './redact.mjs': { redactSecrets: value => value },
    './navigation.mjs': { sessionFromURL: () => 'actual', requestHandoff: async () => ({ cwd: '/fixture/workspace', data_dir: '/fixture/custom-state', host, session_id: 'actual' }) },
  };
  const sourceURL = new URL('../src/engine.mjs', import.meta.url);
  const module = new vm.SourceTextModule(await readFile(sourceURL, 'utf8'), { context, identifier: sourceURL.href });
  await module.link(specifier => { const values = mocks[specifier]; assert.ok(values, specifier); return new vm.SyntheticModule(Object.keys(values), function () { for (const [name, value] of Object.entries(values)) this.setExport(name, value); }, { context }); });
  await module.evaluate();
  const engine = new module.namespace.Engine({ homeDir: '/fixture/home', workspace: '/fixture/workspace', goHost: host, dataDir: '/fixture/custom-state', log: { info() {}, warn() {}, error() {}, write() {} } });
  engine.resolveBinary = () => ({ file: '/fixture/embedded/megafloyd', source: 'bundled' });
  engine.resolveNode = () => '/fixture/embedded/node';
  await engine.start();
  assert.deepEqual(Array.from(launches[0].args), ['--web', '--host', host, '--data-dir', '/fixture/custom-state']);
  child.stdout.emit('data', 'Web UI: http://127.0.0.1:57876/sessions/actual\n');
  await tick();
  assert.equal(engine.snapshot().port, 57876, 'registered adapter port wins over a newly reserved free port');
  assert.equal(metaPorts[0].port, 57876);
  assert.equal(engine.snapshot().handoff.data_dir, '/fixture/custom-state');
  assert.equal(engine.snapshot().runtime.cwd, '/fixture/different-primary-workspace');
  engine.release();
  child.emit('exit', 0, null);
  assert.deepEqual(kills, []);
  assert.equal(engine.healthTimer, null);
  assert.equal(engine.serverPort, null);
  assert.equal(engine.snapshot().status, 'released');
});
