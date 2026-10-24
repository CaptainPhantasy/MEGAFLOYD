import { request, type IncomingMessage, type RequestOptions } from 'node:http';
import { randomUUID } from 'node:crypto';

export class GoRequestError extends Error {
  constructor(readonly status: number, message: string) { super(message); }
}

export interface GoWorkspace {
  id: string;
  path: string;
  data_dir?: string;
}

export interface GoMessage {
  id: string;
  session_id: string;
  role: string;
  parts: Array<{ type: string; data: Record<string, unknown> }>;
  model?: string;
  provider?: string;
  created_at: number;
  updated_at: number;
  is_summary_message?: boolean;
}

export interface GoSession {
  id: string;
  title: string;
  created_at: number;
  updated_at: number;
  is_busy: boolean;
  message_count: number;
  prompt_tokens: number;
  completion_tokens: number;
  cost: number;
  parent_session_id?: string;
  archived?: boolean;
  archived_at?: number;
  metadata?: Record<string, unknown>;
  todos?: Array<{ content: string; status: string }>;
}

export interface GoPermission {
  id: string;
  session_id: string;
  tool_call_id: string;
  tool_name: string;
  description: string;
  action: string;
  params: unknown;
  path: string;
}

export interface GoQuestion {
  id: string;
  session_id: string;
  tool_call_id: string;
  questions: Array<{ id: string; type: string; label?: string; question: string; description?: string; choices?: Array<{ id: string; label: string; description?: string }> }>;
}

export interface GoInteractions {
  permissions: GoPermission[];
  questions: GoQuestion[];
}

export interface GoAgentInfo {
  is_ready: boolean;
  is_busy: boolean;
  model: { id: string; name?: string; context_window?: number; default_max_tokens?: number; can_reason?: boolean; reasoning_levels?: string[] };
  model_cfg: { provider: string; model: string; reasoning_effort?: string; think?: boolean; max_tokens?: number };
}

export interface GoEvent {
  type: string;
  payload: { type: string; payload: Record<string, unknown> };
}

export class GoClient {
  readonly clientId = randomUUID();
  readonly host: string;
  private readonly options: RequestOptions;
  private streams = new Set<IncomingMessage>();

  constructor(host: string) {
    this.host = host;
    if (host.startsWith('unix://')) {
      this.options = { socketPath: host.slice(7) };
    } else {
      const url = new URL(host.replace(/^tcp:/, 'http:'));
      if (url.protocol !== 'http:') throw new Error('MEGAFLOYD_GO_HOST must be a unix:// socket or tcp:// address');
      this.options = { hostname: url.hostname, port: url.port };
    }
  }

  async json<T = unknown>(method: string, path: string, body?: unknown, options: { signal?: AbortSignal; timeoutMs?: number } = {}): Promise<T> {
    const content = body === undefined ? undefined : JSON.stringify(body);
    return new Promise<T>((resolve, reject) => {
      const req = request({ ...this.options, method, path, signal: options.signal, headers: content === undefined ? {} : { 'Content-Type': 'application/json', 'Content-Length': Buffer.byteLength(content) } }, (res) => {
        let data = '';
        res.setEncoding('utf8');
        res.on('data', (chunk: string) => { data += chunk; });
        res.on('error', reject);
        res.on('end', () => {
          try {
            const parsed = data.length ? JSON.parse(data) : undefined;
            if ((res.statusCode ?? 500) >= 400) reject(new GoRequestError(res.statusCode ?? 500, parsed?.message ?? `Go runtime returned HTTP ${res.statusCode}`));
            else resolve(parsed as T);
          } catch { reject(new Error('Go runtime returned invalid JSON')); }
        });
      });
      req.setTimeout(options.timeoutMs ?? 30_000, () => req.destroy(new Error('Go runtime request timed out')));
      req.on('error', reject);
      req.end(content);
    });
  }

  async subscribe(workspaceId: string, onEvent: (event: GoEvent) => void, onDisconnect: (error?: Error) => void): Promise<() => void> {
    const path = `/v1/workspaces/${encodeURIComponent(workspaceId)}/events?client_id=${this.clientId}`;
    return new Promise((resolve, reject) => {
      let stopped = false;
      const req = request({ ...this.options, method: 'GET', path }, (res) => {
        if (res.statusCode !== 200) { res.resume(); reject(new GoRequestError(res.statusCode ?? 500, 'Cannot subscribe to Go runtime')); return; }
        req.setTimeout(0);
        this.streams.add(res);
        let buffer = '';
        res.setEncoding('utf8');
        res.on('data', (chunk: string) => {
          buffer += chunk.replaceAll('\r\n', '\n');
          let boundary: number;
          while ((boundary = buffer.indexOf('\n\n')) >= 0) {
            const frame = buffer.slice(0, boundary);
            buffer = buffer.slice(boundary + 2);
            const data = frame.split('\n').filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trimStart()).join('\n');
            if (data) {
              try { onEvent(JSON.parse(data) as GoEvent); } catch (error) { onDisconnect(error instanceof Error ? error : new Error(String(error))); }
            }
          }
        });
        res.on('close', () => { this.streams.delete(res); if (!stopped) onDisconnect(); });
        res.on('error', (error) => { if (!stopped) onDisconnect(error); });
        resolve(() => { stopped = true; this.streams.delete(res); res.destroy(); req.destroy(); });
      });
      req.setTimeout(10_000, () => req.destroy(new Error('Go event connection timed out')));
      req.on('error', reject);
      req.end();
    });
  }

  async close(): Promise<void> {
    for (const stream of this.streams) stream.destroy();
    this.streams.clear();
    await this.json('DELETE', `/v1/clients/${this.clientId}`).catch(() => {});
  }
}
