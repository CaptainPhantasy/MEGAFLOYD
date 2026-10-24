import { GoClient } from './client';

export class GoSessionPresence {
  private readonly client: GoClient;
  private stopStream?: () => void;
  private reconnectTimer?: ReturnType<typeof setTimeout>;
  private retryDelay = 250;
  private queue = Promise.resolve();
  private closed = false;
  private selected = '';

  constructor(host: string, private readonly workspaceId: string, private readonly unavailable: (message: string) => void) { this.client = new GoClient(host); }

  get currentSession(): string { return this.selected; }

  select(sessionId: string): Promise<void> {
    if (this.closed) return Promise.resolve();
    this.selected = sessionId;
    const operation = this.queue.then(async () => {
      if (this.closed) return;
      if (!this.stopStream) {
        const stop = await this.client.subscribe(this.workspaceId, () => {}, (error) => this.reconnect(error));
        if (this.closed) { stop(); return; }
        this.stopStream = stop;
      }
      if (this.closed) return;
      await this.client.json('POST', `/v1/workspaces/${encodeURIComponent(this.workspaceId)}/current-session?client_id=${this.client.clientId}`, { session_id: this.selected });
      this.retryDelay = 250;
    });
    this.queue = operation.catch(() => {});
    return operation;
  }

  private reconnect(error?: Error): void {
    if (this.closed || this.reconnectTimer) return;
    this.stopStream?.();
    this.stopStream = undefined;
    this.unavailable(error?.message ?? 'Go session presence interrupted; reconnecting');
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = undefined;
      void this.select(this.selected).catch((cause) => {
        this.retryDelay = Math.min(5000, this.retryDelay * 2);
        this.reconnect(cause instanceof Error ? cause : new Error(String(cause)));
      });
    }, this.retryDelay);
    this.reconnectTimer.unref();
  }

  async close(): Promise<void> {
    if (this.closed) return;
    this.closed = true;
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer);
    this.stopStream?.();
    this.stopStream = undefined;
    await this.queue;
    await this.client.close();
  }
}
