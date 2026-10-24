import { spawn } from 'node:child_process';
import { findOwnedListener } from './process-owner.mjs';
import { EventEmitter } from 'node:events';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';

import {
  DEFAULT_PORT,
  ENGINE_START_TIMEOUT_MS,
  ENGINE_STOP_TIMEOUT_MS,
  HOME_DIR,
  WEB_BUNDLE_DIR,
  WEB_SCRIPT,
  megafloydBinaryCandidates,
  nodeWrapperCandidates,
} from './config.mjs';
import { fetchMeta, fetchRuntime, handoffUrlFromLine, tokenFromUrl } from './meta.mjs';
import { redactSecrets } from './redact.mjs';
import { requestHandoff, sessionFromURL } from './navigation.mjs';

const MAX_AUTO_RESTARTS = 3;
const RESTART_WINDOW_MS = 5 * 60 * 1000;
const HEALTH_INTERVAL_MS = 10_000;
const HEALTH_FAILURES = 3;

/**
 * Owns the MegaFloyd engine.
 *
 * The engine is the platform binary itself: `megafloyd --web` performs the
 * platform's own handoff (resolve Node, resolve the committed web bundle,
 * spawn the detached server, print the ready URL). The desktop never
 * reimplements that contract; it launches it and supervises the result.
 */
export class Engine extends EventEmitter {
  constructor({ homeDir = HOME_DIR, workspace, goHost, dataDir, log }) {
    super();
    this.homeDir = homeDir;
    this.workspace = workspace || process.env.MEGAFLOYD_WORKSPACE || os.homedir();
    this.goHost = goHost || process.env.MEGAFLOYD_GO_HOST || null;
    this.dataDir = dataDir || process.env.MEGAFLOYD_GO_DATA_DIR || null;
    this.log = log;
    this.child = null;
    this.stopping = false;
    this.restartTimer = null;
    this.restartTimes = [];
    this.startTimer = null;
    this.healthTimer = null;
    this.healthFailures = 0;
    this.serverPort = null;
    this.handshakePending = false;
    this.pidFile = path.join(homeDir, 'desktop', 'engine.pid');
    this.state = {
      status: 'idle',
      detail: 'Engine not started',
      port: null,
      url: null,
      entry: null,
      source: null,
      pid: null,
      error: null,
      restarts: 0,
      startedAt: null,
      workspace: this.workspace,
      webLog: path.join(homeDir, 'logs', 'web-server.log'),
      engineVersion: null,
      protocolMajor: null,
      platformCommit: null,
      capabilities: null,
      metaWarnings: [],
      runtime: null,
      notice: null,
    };
  }

  snapshot() {
    return { ...this.state };
  }

  setState(patch) {
    this.state = { ...this.state, ...patch };
    this.emit('state', this.snapshot());
  }

  setNotice(notice) {
    this.setState({ notice: notice ?? null });
  }

  clearNotice() {
    if (this.state.notice) this.setState({ notice: null });
  }

  resolveBinary() {
    for (const candidate of megafloydBinaryCandidates()) {
      try {
        const stat = fs.statSync(candidate.file);
        if (stat.isFile()) {
          fs.accessSync(candidate.file, fs.constants.X_OK);
          return candidate;
        }
      } catch {
        // Try the next candidate.
      }
    }
    return null;
  }

  resolveNode() {
    for (const file of nodeWrapperCandidates()) {
      try {
        fs.accessSync(file, fs.constants.X_OK);
        return file;
      } catch {
        // Try the next candidate.
      }
    }
    return null;
  }

  async start() {
    if (this.child || this.state.status === 'starting' || this.state.status === 'ready') return;
    clearTimeout(this.restartTimer);
    this.restartTimer = null;
    this.stopping = false;

    const binary = this.resolveBinary();
    if (!binary) {
      this.log.error('megafloyd binary not found');
      this.setState({
        status: 'missing',
        detail: 'MegaFloyd engine not found',
        error: 'No megafloyd binary found. Set MEGAFLOYD_BIN or install the platform.',
      });
      return;
    }

    if (!fs.existsSync(WEB_SCRIPT)) {
      this.log.error('web server missing', { path: WEB_SCRIPT });
      this.setState({
        status: 'missing',
        detail: 'MegaFloyd web engine not found',
        error: `Expected the platform web server at ${WEB_SCRIPT}.`,
      });
      return;
    }

    let port = await pickPort(DEFAULT_PORT);
    if (port === null) {
      this.setState({
        status: 'failed',
        detail: 'No free loopback port',
        error: 'Could not find a free local port for the engine.',
      });
      return;
    }

    const node = this.resolveNode();
    this.setState({
      status: 'starting',
      detail: 'Starting MegaFloyd engine…',
      entry: binary.file,
      source: binary.source,
      port,
      url: null,
      pid: null,
      error: null,
      workspace: this.workspace,
      engineVersion: null,
      protocolMajor: null,
      platformCommit: null,
      capabilities: null,
      metaWarnings: [],
      runtime: null,
      notice: null,
    });
    this.handshakePending = false;
    this.log.info('engine starting', { binary: binary.file, source: binary.source, port, node });

    const env = {
      ...process.env,
      MEGAFLOYD_HOME: this.homeDir,
      // Resolve the MEGAFLOYD-owned web store before setting the engine mirror.
      MEGAFLOYD_WEB_DATA: process.env.MEGAFLOYD_WEB_DATA || path.join(this.homeDir, 'web'),
      FLOYD_CODE_HOME: process.env.MEGAFLOYD_WEB_DATA || path.join(this.homeDir, 'web'),
      MEGAFLOYD_WEB_PORT: String(port),
      MEGAFLOYD_WEB_SCRIPT: WEB_SCRIPT,
      MEGAFLOYD_WEB_BUNDLE: WEB_BUNDLE_DIR,
      // The platform writes its Node data under <home>/web unless told otherwise.
      FLOYD_DISABLE_TELEMETRY: '1',
      NODE_ENV: 'production',
    };
    if (node) env.MEGAFLOYD_WEB_NODE = node;

    const args = ['--web'];
    if (this.goHost) args.push('--host', this.goHost);
    if (this.dataDir) args.push('--data-dir', this.dataDir);
    const child = spawn(binary.file, args, {
      cwd: this.workspace,
      env,
      stdio: ['ignore', 'pipe', 'pipe'],
    });

    this.child = child;
    this.serverPort = port;
    this.setState({ pid: child.pid ?? null });
    this.writePid(child.pid);

    let settled = false;
    const becomeReady = async (url) => {
      if (settled) return;
      settled = true;
      port = Number(new URL(url).port) || port;
      this.serverPort = port;
      clearTimeout(this.startTimer);
      this.startTimer = null;

      // Spec C4: prove the API contract before any UI is loaded.
      this.handshakePending = true;
      let meta;
      try {
        meta = await fetchMeta({ port, token: tokenFromUrl(url) });
        if (meta.ok) {
          const target = await requestHandoff(url, sessionFromURL(url));
          if (path.resolve(target.cwd) !== path.resolve(this.workspace)) throw new Error('Handoff selected a different workspace');
          if (this.dataDir && path.resolve(target.data_dir || '') !== path.resolve(this.dataDir)) throw new Error('Handoff selected a different workspace store');
          if (this.goHost && target.host !== this.goHost) throw new Error('Handoff selected a different Go host');
          meta.handoff = target;
        }
      } catch (error) {
        meta = { ok: false, error: `meta probe failed: ${error?.message ?? error}` };
      }
      this.handshakePending = false;

      this.setState({
        engineVersion: meta.engineVersion ?? null,
        protocolMajor: meta.protocolMajor ?? null,
        platformCommit: meta.platformCommit ?? null,
        capabilities: meta.capabilities ?? null,
        metaWarnings: meta.warnings ?? [],
        runtime: meta.runtime ?? null,
        handoff: meta.handoff ?? null,
      });

      if (!meta.ok) {
        this.log.error('engine capability handshake failed', { error: meta.error });
        this.setState({
          status: 'failed',
          detail: 'Engine failed the capability handshake',
          error: meta.error,
          url: null,
          pid: null,
        });
        this.stopHealthMonitor();
        // An unverified listener is not ours to stop; only stop our handoff child.
        this.stopping = true;
        if (this.child) this.stopChild(this.child);
        return;
      }

      this.setState({
        status: 'ready',
        detail: `Engine ready on port ${port}`,
        port,
        url,
        error: null,
        startedAt: Date.now(),
      });
      this.log.info('engine ready', {
        port,
        engineVersion: meta.engineVersion,
        protocolMajor: meta.protocolMajor,
        platformCommit: meta.platformCommit ?? 'not-reported',
      });
      for (const warning of meta.warnings) this.log.warn('engine meta warning', { warning });
      this.startHealthMonitor();
      void this.refreshListenerPid(port);
    };

    const bufferLines = (stream, level) => {
      let buffer = '';
      stream.setEncoding('utf8');
      stream.on('data', (chunk) => {
        buffer += chunk;
        let index;
        while ((index = buffer.indexOf('\n')) >= 0) {
          const line = buffer.slice(0, index).trimEnd();
          buffer = buffer.slice(index + 1);
          if (line.length === 0) continue;
          this.log.write(level, `engine: ${redactSecrets(line)}`);
          const readyUrl = handoffUrlFromLine(line);
          if (readyUrl) void becomeReady(readyUrl);
          if (line.startsWith('Log:')) {
            this.setState({ webLog: line.slice(4).trim() });
          }
        }
        if (buffer.length > 8192) buffer = buffer.slice(-8192);
      });
    };
    bufferLines(child.stdout, 'INFO');
    bufferLines(child.stderr, 'ERROR');

    this.startTimer = setTimeout(() => {
      if (settled) return;
      settled = true;
      this.startTimer = null;
      this.log.error('engine start timed out', { port });
      this.setState({
        status: 'failed',
        detail: 'Engine did not become ready',
        error: `No ready signal within ${Math.round(ENGINE_START_TIMEOUT_MS / 1000)}s. See the engine log.`,
      });
      this.stopChild(child);
    }, ENGINE_START_TIMEOUT_MS);

    child.once('error', (error) => {
      this.log.error('engine spawn error', { message: error.message });
      this.setState({ status: 'failed', detail: 'Engine failed to start', error: error.message });
    });

    child.once('exit', (code, signal) => {
      if (this.child !== child) return;
      clearTimeout(this.startTimer);
      this.startTimer = null;
      this.child = null;
      if (this.stopping) {
        this.stopHealthMonitor();
        if (this.state.status !== 'failed') {
          this.setState({ status: 'stopped', detail: 'Engine stopped', url: null, pid: null });
        }
        return;
      }
      if (this.handshakePending) {
        this.log.info('handoff process exited during meta handshake', { code, signal });
        this.setState({ pid: null });
        return;
      }
      if (settled && this.state.status === 'ready') {
        // `megafloyd --web` exits after the detached server is healthy.
        this.log.info('handoff process exited; server stays up', { code, signal });
        if (this.state.pid === child.pid) this.setState({ pid: null });
        return;
      }
      this.log.warn('engine exited before ready', { code, signal });
      this.setState({
        status: 'crashed',
        detail: `Engine exited (${signal ?? `code ${code}`})`,
        url: null,
        pid: null,
        error: `Engine exited with ${signal ?? `code ${code}`}. See the engine log.`,
      });
      this.scheduleRestart();
    });
  }

  startHealthMonitor() {
    this.stopHealthMonitor();
    this.healthFailures = 0;
    this.healthTimer = setInterval(async () => {
      const alive = await this.health(this.serverPort);
      if (alive) {
        this.healthFailures = 0;
        return;
      }
      this.healthFailures += 1;
      this.log.warn('engine health check failed', { failures: this.healthFailures, port: this.serverPort });
      if (this.healthFailures >= HEALTH_FAILURES) {
        this.stopHealthMonitor();
        this.setState({
          status: 'crashed',
          detail: 'Engine stopped responding',
          url: null,
          error: 'The local web server stopped responding.',
        });
        this.scheduleRestart();
      }
    }, HEALTH_INTERVAL_MS);
  }

  stopHealthMonitor() {
    clearInterval(this.healthTimer);
    this.healthTimer = null;
  }

  scheduleRestart() {
    const now = Date.now();
    this.restartTimes = this.restartTimes.filter((time) => now - time < RESTART_WINDOW_MS);
    if (this.restartTimes.length >= MAX_AUTO_RESTARTS) {
      this.log.error('engine restart limit reached');
      this.setState({
        status: 'failed',
        detail: 'Engine keeps exiting',
        error: 'Automatic restart paused. Check the engine log.',
      });
      return;
    }
    const attempt = this.restartTimes.length + 1;
    this.restartTimes.push(now);
    const delay = attempt * 1500;
    this.setState({ restarts: this.state.restarts + 1, detail: `Restarting engine (${attempt})…` });
    this.restartTimer = setTimeout(() => void this.start(), delay);
  }

  release() {
    this.stopping = true;
    clearTimeout(this.restartTimer);
    clearTimeout(this.startTimer);
    this.restartTimer = null;
    this.startTimer = null;
    this.stopHealthMonitor();
    if (this.state.status !== 'ready') {
      this.forceStop();
      return;
    }
    this.child?.unref?.();
    this.child = null;
    this.serverPort = null;
    this.setState({ status: 'released', detail: 'Shared runtime continues independently', url: null, pid: null });
  }

  async switchWorkspace() {
    this.release();
    await this.start();
  }

  async restart() {
    this.restartTimes = [];
    this.setState({ restarts: 0 });
    await this.stop();
    await this.start();
  }

  async stop() {
    this.stopping = true;
    clearTimeout(this.restartTimer);
    this.restartTimer = null;
    clearTimeout(this.startTimer);
    this.startTimer = null;
    this.stopHealthMonitor();

    const child = this.child;
    const pid = await this.listenerPid(this.serverPort);
    if (child) this.stopChild(child);
    if (pid) {
      this.log.info('stopping web server', { pid, port: this.serverPort });
      try {
        process.kill(pid, 'SIGTERM');
      } catch {
        // Already gone.
      }
      const deadline = Date.now() + ENGINE_STOP_TIMEOUT_MS;
      while (Date.now() < deadline && await this.listenerPid(this.serverPort) === pid) {
        await new Promise((resolve) => setTimeout(resolve, 200));
      }
      const remaining = await this.listenerPid(this.serverPort);
      if (remaining === pid) {
        try {
          process.kill(remaining, 'SIGKILL');
        } catch {
          // Already gone.
        }
      }
    }
    this.child = null;
    this.setState({ status: 'stopped', detail: 'Engine stopped', url: null, pid: null });
  }

  stopChild(child) {
    try {
      child.kill('SIGTERM');
    } catch {
      // Already gone.
    }
  }

  forceStop() {
    this.stopping = true;
    if (this.child) this.stopChild(this.child);
  }

  writePid(pid) {
    try {
      fs.mkdirSync(path.dirname(this.pidFile), { recursive: true, mode: 0o700 });
      fs.writeFileSync(this.pidFile, JSON.stringify({ pid, port: this.serverPort, at: Date.now() }), { mode: 0o600 });
    } catch {
      // Best effort.
    }
  }

  async refreshListenerPid(port) {
    const pid = await this.listenerPid(port);
    if (!this.stopping && this.serverPort === port && this.state.status === 'ready') {
      this.setState({ pid });
    }
  }

  async listenerPid(port) {
    return findOwnedListener({ port, verifyRuntime: () => this.health(port) });
  }

  async health(port) {
    const host = this.state.handoff?.host || this.goHost;
    if (!port || !host) return false;
    try {
      const runtime = await fetchRuntime({
        port,
        token: tokenFromUrl(this.state.url),
        host,
        attempts: 1,
        timeoutMs: 1000,
      });
      return runtime.ok;
    } catch {
      return false;
    }
  }
}

function pickPort(preferred) {
  return new Promise((resolve) => {
    const attempt = (port, retries) => {
      const server = net.createServer();
      server.unref();
      server.once('error', () => {
        if (retries <= 0) return resolve(null);
        attempt(0, retries - 1);
      });
      server.listen(port, '127.0.0.1', () => {
        const chosen = server.address().port;
        server.close(() => resolve(chosen));
      });
    };
    attempt(preferred, 4);
  });
}
