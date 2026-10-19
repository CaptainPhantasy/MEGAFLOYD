import { execFileSync, spawn } from 'node:child_process';
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
import { fetchMeta, tokenFromUrl } from './meta.mjs';
import { redactSecrets } from './redact.mjs';

const URL_PATTERN = /(http:\/\/127\.0\.0\.1:(\d+)\/[^\s]*#token=[A-Za-z0-9_-]+)/;
const MAX_AUTO_RESTARTS = 3;
const RESTART_WINDOW_MS = 5 * 60 * 1000;
const HEALTH_INTERVAL_MS = 10_000;
const HEALTH_FAILURES = 3;

/**
 * Owns the MEGAFLOYD engine.
 *
 * The engine is the platform binary itself: `megafloyd --web` performs the
 * platform's own handoff (resolve Node, resolve the committed web bundle,
 * spawn the detached server, print the tokenised URL). The desktop never
 * reimplements that contract; it launches it and supervises the result.
 */
export class Engine extends EventEmitter {
  constructor({ homeDir = HOME_DIR, workspace, log }) {
    super();
    this.homeDir = homeDir;
    this.workspace = workspace || process.env.MEGAFLOYD_WORKSPACE || os.homedir();
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
    };
  }

  snapshot() {
    return { ...this.state };
  }

  setState(patch) {
    this.state = { ...this.state, ...patch };
    this.emit('state', this.snapshot());
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
        detail: 'MEGAFLOYD engine not found',
        error: 'No megafloyd binary found. Set MEGAFLOYD_BIN or install the platform.',
      });
      return;
    }

    if (!fs.existsSync(WEB_SCRIPT)) {
      this.log.error('web server missing', { path: WEB_SCRIPT });
      this.setState({
        status: 'missing',
        detail: 'MEGAFLOYD web engine not found',
        error: `Expected the platform web server at ${WEB_SCRIPT}.`,
      });
      return;
    }

    const port = await pickPort(DEFAULT_PORT);
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
      detail: 'Starting MEGAFLOYD engine…',
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
    });
    this.handshakePending = false;
    this.log.info('engine starting', { binary: binary.file, source: binary.source, port, node });

    const env = {
      ...process.env,
      MEGAFLOYD_HOME: this.homeDir,
      MEGAFLOYD_WEB_PORT: String(port),
      MEGAFLOYD_WEB_SCRIPT: WEB_SCRIPT,
      MEGAFLOYD_WEB_BUNDLE: WEB_BUNDLE_DIR,
      // The platform writes its Node data under <home>/web unless told otherwise.
      FLOYD_DISABLE_TELEMETRY: '1',
      NODE_ENV: 'production',
    };
    if (node) env.MEGAFLOYD_WEB_NODE = node;

    const child = spawn(binary.file, ['--web'], {
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
      clearTimeout(this.startTimer);
      this.startTimer = null;

      // Spec C4: prove the API contract before any UI is loaded.
      this.handshakePending = true;
      let meta;
      try {
        meta = await fetchMeta({ port, token: tokenFromUrl(url) });
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
        // Refuse the UI and leave no orphan listener behind.
        this.stopping = true;
        const childPid = this.listenerPid(port);
        if (childPid) {
          try {
            process.kill(childPid, 'SIGTERM');
          } catch {
            // Already gone.
          }
        }
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
          const match = URL_PATTERN.exec(line);
          if (match) becomeReady(match[1]);
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
        this.setState({ pid: this.listenerPid(port) });
        return;
      }
      if (settled && this.state.status === 'ready') {
        // `megafloyd --web` exits after the detached server is healthy.
        this.log.info('handoff process exited; server stays up', { code, signal });
        this.setState({ pid: this.listenerPid(port) });
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
    const pid = this.listenerPid(this.serverPort);
    if (child) this.stopChild(child);
    if (pid) {
      this.log.info('stopping web server', { pid, port: this.serverPort });
      try {
        process.kill(pid, 'SIGTERM');
      } catch {
        // Already gone.
      }
      const deadline = Date.now() + ENGINE_STOP_TIMEOUT_MS;
      while (Date.now() < deadline && this.listenerPid(this.serverPort)) {
        await new Promise((resolve) => setTimeout(resolve, 200));
      }
      const remaining = this.listenerPid(this.serverPort);
      if (remaining) {
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
    const pid = this.listenerPid(this.serverPort);
    if (pid) {
      try {
        process.kill(pid, 'SIGTERM');
      } catch {
        // Already gone.
      }
    }
  }

  writePid(pid) {
    try {
      fs.mkdirSync(path.dirname(this.pidFile), { recursive: true, mode: 0o700 });
      fs.writeFileSync(this.pidFile, JSON.stringify({ pid, port: this.serverPort, at: Date.now() }), { mode: 0o600 });
    } catch {
      // Best effort.
    }
  }

  listenerPid(port) {
    if (!port) return null;
    try {
      const out = execFileSync('lsof', ['-tiTCP:' + port, '-sTCP:LISTEN'], { encoding: 'utf8' }).trim();
      const pid = Number(out.split('\n')[0]);
      return Number.isInteger(pid) && pid > 0 ? pid : null;
    } catch {
      return null;
    }
  }

  async health(port) {
    if (!port) return false;
    try {
      const response = await fetch(`http://127.0.0.1:${port}/api/v1/meta`, {
        signal: AbortSignal.timeout(2000),
      });
      return response.status < 500;
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
