import { randomUUID } from 'node:crypto';
import { FLOYD_CODE_FLOW_CONFIG, FLOYD_CODE_PROVIDER_NAME, fetchManagedFloydCodeModels, fetchManagedUsage, fetchManagedUserInfo, pollDeviceToken, requestDeviceAuthorization, tokenToWire, type DeviceAuthorization, type OAuthFlowConfig } from '@legacy-ai/floyd-code-oauth';
import { oauthLoginStartRequestSchema, oauthLoginQuerySchema, oauthLogoutRequestSchema } from '../protocol/rest-oauth';
import type { GoRuntime } from './runtime';
import { GoRequestError } from './client';
import { array, object, text } from './projection';

type Status = 'pending' | 'authenticated' | 'denied' | 'expired' | 'cancelled';
interface Account { provider: string; authenticated: boolean; status?: string; base_url: string; oauth_host: string; region: 'global' | 'mainland-cn' }
interface Flow { id: string; provider: string; runtime: GoRuntime; account: Account; config: OAuthFlowConfig; device?: DeviceAuthorization; status: Status; expiresAt: number; resolvedAt?: string; error?: string; controller: AbortController }

export class GoOAuth {
  private readonly flows = new Map<string, Flow>();
  constructor(readonly env: NodeJS.ProcessEnv) {}

  async account(runtime: GoRuntime, provider = FLOYD_CODE_PROVIDER_NAME): Promise<Account> {
    const account = await runtime.client.json<Account>('GET', `${runtime.base}/auth/account?provider=${encodeURIComponent(provider)}`);
    return { ...account, provider: account.provider || provider, base_url: account.base_url || this.env['FLOYD_CODE_BASE_URL'] || '', oauth_host: account.oauth_host || this.env['FLOYD_CODE_OAUTH_HOST'] || this.env['FLOYD_OAUTH_HOST'] || '', region: account.region || 'global' };
  }

  async summary(runtime: GoRuntime) {
    const [catalog, account] = await Promise.all([runtime.catalog(), this.account(runtime)]);
    const status = account.authenticated ? 'authenticated' : ['expired', 'revoked'].includes(account.status || '') ? account.status : 'unauthenticated';
    return { models_ready: catalog.info.is_ready, providers_count: catalog.providers.length, managed_provider: { name: account.provider, status } };
  }

  private snapshot(flow: Flow) {
    const device = flow.device;
    if (!device) return null;
    return { flow_id: flow.id, provider: flow.provider, status: flow.status, verification_uri: device.verificationUri || device.verificationUriComplete, verification_uri_complete: device.verificationUriComplete, user_code: device.userCode, expires_in: Math.max(1, device.expiresIn ?? 600), interval: Math.max(1, device.interval), expires_at: new Date(flow.expiresAt).toISOString(), resolved_at: flow.resolvedAt, error_message: flow.error };
  }

  private finish(flow: Flow, status: Status, error?: string): void { flow.status = status; flow.resolvedAt = new Date().toISOString(); flow.error = error; }

  private key(runtime: GoRuntime, provider: string): string { return `${runtime.workspace.id}:${provider}`; }

  private cancel(runtime: GoRuntime, provider: string) {
    const flow = this.flows.get(this.key(runtime, provider));
    if (!flow || flow.status !== 'pending') return { cancelled: false, status: flow?.status || 'cancelled' };
    flow.controller.abort();
    this.finish(flow, 'cancelled');
    return { cancelled: true, status: 'cancelled' };
  }

  private async poll(flow: Flow): Promise<void> {
    let interval = Math.max(1, flow.device!.interval) * 1000;
    try {
      while (!flow.controller.signal.aborted && this.flows.get(this.key(flow.runtime, flow.provider)) === flow) {
        if (Date.now() >= flow.expiresAt) { this.finish(flow, 'expired'); return; }
        await pause(Math.min(interval, flow.expiresAt - Date.now()), flow.controller.signal);
        if (flow.controller.signal.aborted) return;
        const result = await pollDeviceToken(flow.config, flow.device!.deviceCode, { deviceHeaders: { 'User-Agent': 'MEGAFLOYD', 'X-Msh-Platform': 'megafloyd' } });
        if (flow.controller.signal.aborted || this.flows.get(this.key(flow.runtime, flow.provider)) !== flow) return;
        if (result.kind === 'pending') { if (result.errorCode === 'slow_down') interval += 5000; continue; }
        if (result.kind === 'denied') { this.finish(flow, 'denied', result.description); return; }
        if (result.kind === 'expired') { this.finish(flow, 'expired'); return; }
        await flow.runtime.client.json('POST', `${flow.runtime.base}/auth/account`, { provider: flow.provider, base_url: flow.account.base_url, oauth_host: flow.account.oauth_host, region: flow.account.region, token: { ...tokenToWire(result.token), client: { client_id: flow.config.clientId, token_url: `${flow.config.oauthHost.replace(/\/+$/, '')}/api/oauth/token` } } }, { timeoutMs: 120_000 });
        await this.refreshModels(flow.runtime, flow.account, result.token.accessToken);
        this.finish(flow, 'authenticated');
        flow.runtime.markDirty();
        return;
      }
    } catch (error) {
      if (flow.controller.signal.aborted) { if (flow.status === 'pending') this.finish(flow, 'cancelled'); }
      else this.finish(flow, 'denied', error instanceof Error ? error.message : String(error));
    }
  }

  async route(runtime: GoRuntime, path: string, method: string, body: Record<string, unknown>, query: URLSearchParams): Promise<{ data: unknown } | undefined> {
    if (method === 'GET' && path === '/auth') return { data: await this.summary(runtime) };
    if (!path.startsWith('/oauth/')) return undefined;
    const queryInput = oauthLoginQuerySchema.parse(Object.fromEntries(query));
    const provider = queryInput.provider || FLOYD_CODE_PROVIDER_NAME;
    if (method === 'GET' && path === '/oauth/region') return { data: { region: (await this.account(runtime, provider)).region } };
    if (path === '/oauth/login') {
      if (method === 'GET') {
        const flow = this.flows.get(this.key(runtime, provider));
        if (flow?.status === 'pending' && Date.now() >= flow.expiresAt) { flow.controller.abort(); this.finish(flow, 'expired'); }
        return { data: flow ? this.snapshot(flow) : null };
      }
      if (method === 'DELETE') return { data: this.cancel(runtime, provider) };
      if (method === 'POST') {
        const request = oauthLoginStartRequestSchema.parse(body);
        const selected = request.provider || FLOYD_CODE_PROVIDER_NAME;
        const account = await this.account(runtime, selected);
        this.cancel(runtime, selected);
        const id = `oauth_${randomUUID()}`;
        if (account.authenticated) {
          const token = await this.token(runtime, account);
          if (!token) throw new GoRequestError(409, 'The native company account has no usable token');
          await this.refreshModels(runtime, account, token);
          return { data: { flow_id: id, provider: account.provider, status: 'authenticated' } };
        }
        if (!account.oauth_host || !account.base_url) throw new GoRequestError(409, 'The company account host and API base URL have not been configured');
        new URL(account.oauth_host);
        new URL(account.base_url);
        const flow: Flow = { id, provider: account.provider, runtime, account: { ...account, region: request.region || account.region }, config: { ...FLOYD_CODE_FLOW_CONFIG, name: account.provider, oauthHost: account.oauth_host }, status: 'pending', expiresAt: Date.now() + 600_000, controller: new AbortController() };
        this.flows.set(this.key(runtime, account.provider), flow);
        try {
          flow.device = await requestDeviceAuthorization(flow.config, { deviceHeaders: { 'User-Agent': 'MEGAFLOYD', 'X-Msh-Platform': 'megafloyd' } });
          flow.expiresAt = Date.now() + Math.max(1, flow.device.expiresIn ?? 600) * 1000;
          if (flow.controller.signal.aborted) throw new GoRequestError(409, 'The account login was cancelled');
          void this.poll(flow);
          return { data: this.snapshot(flow) };
        } catch (error) { this.finish(flow, flow.controller.signal.aborted ? 'cancelled' : 'denied', error instanceof Error ? error.message : String(error)); throw error; }
      }
    }
    if (method === 'POST' && path === '/oauth/logout') {
      const request = oauthLogoutRequestSchema.parse(body);
      const selected = request.provider || FLOYD_CODE_PROVIDER_NAME;
      this.cancel(runtime, selected);
      await runtime.client.json('DELETE', `${runtime.base}/auth/account?provider=${encodeURIComponent(selected)}`);
      runtime.markDirty();
      return { data: { logged_out: true, provider: selected } };
    }
    if (method === 'GET' && ['/oauth/usage', '/oauth/userinfo'].includes(path)) {
      const account = await this.account(runtime, provider);
      const token = await this.token(runtime, account);
      if (!token) return { data: { kind: 'error', status: 401, message: 'Sign in to the configured company account to view this information.' } };
      return { data: path.endsWith('/usage') ? await fetchManagedUsage(`${account.base_url.replace(/\/+$/, '')}/usages`, token) : await fetchManagedUserInfo(`${account.base_url.replace(/\/+$/, '')}/me`, token) };
    }
    return undefined;
  }

  async refreshProvider(runtime: GoRuntime, provider: string) {
    const account = await this.account(runtime, provider);
    const token = await this.token(runtime, account);
    if (!token) return { changed: [], unchanged: [], failed: [{ provider: account.provider, reason: 'The company account is not signed in.' }] };
    return { changed: [await this.refreshModels(runtime, account, token)], unchanged: [], failed: [] };
  }

  private async token(runtime: GoRuntime, account: Account): Promise<string | undefined> {
    const endpoint = `${runtime.base}/auth/account/token?provider=${encodeURIComponent(account.provider)}`;
    let token = object(await runtime.client.json<Record<string, unknown> | null>('GET', endpoint));
    if (!text(token['access_token'])) return undefined;
    if (Number(token['expires_at']) <= Date.now() / 1000 + 30) {
      await runtime.client.json('POST', `${runtime.base}/auth/account/refresh`, { provider: account.provider });
      token = object(await runtime.client.json<Record<string, unknown> | null>('GET', endpoint));
    }
    return text(token['access_token']) || undefined;
  }

  private async refreshModels(runtime: GoRuntime, account: Account, token: string) {
    let records: Record<string, unknown>[] = [];
    const models = await fetchManagedFloydCodeModels({ accessToken: token, baseUrl: account.base_url, fetchImpl: async (url, init) => {
      const response = await fetch(url, { ...init, signal: AbortSignal.timeout(15_000) });
      if (response.ok) records = array(object(await response.clone().json())['data']).map(object);
      return response;
    } });
    if (!models.length) throw new GoRequestError(409, 'The company account has no assigned models');
    const cfg = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
    const previous = object(object(cfg['providers'])[account.provider]);
    const priorModels = array(previous['models']).map(object);
    const mapped = models.map((model) => {
      const prior = priorModels.find((entry) => entry['id'] === model.id) || {};
      const source = records.find((entry) => entry['id'] === model.id) || {};
      return { ...prior, id: model.id, name: model.displayName || model.id, context_window: model.contextLength, default_max_tokens: Number(source['max_output_tokens']) || Number(source['max_output_size']) || Number(object(source['limit'])['output']) || prior['default_max_tokens'], can_reason: model.supportsReasoning, supports_images: model.supportsImageIn, supports_attachments: model.supportsImageIn || model.supportsVideoIn, reasoning_levels: model.supportEfforts };
    });
    const endpoints = [...new Set(records.map((entry) => text(entry['base_url'])).filter(Boolean))];
    if (endpoints.length > 1) throw new GoRequestError(502, 'The company account returned models with different inference endpoints');
    await runtime.client.json('POST', `${runtime.base}/config/set`, { scope: 0, key: `providers.${account.provider}`, value: { ...previous, id: account.provider, name: text(previous['name']) || 'Company account', type: text(previous['type']) || 'openai-compat', api_key: 'company-auth://default', oauth: undefined, disable: false, models: mapped, base_url: endpoints[0] || previous['base_url'] || account.base_url, discover_models: false } });
    const info = await runtime.info();
    if (info.model_cfg.provider === account.provider && !info.is_busy) await runtime.client.json('POST', `${runtime.base}/agent/update`, {});
    runtime.markDirty();
    const priorIds = new Set(priorModels.map((entry) => text(entry['id'])));
    const nextIds = new Set(models.map((model) => model.id));
    return { provider_id: account.provider, provider_name: text(previous['name']) || 'Company account', added: [...nextIds].filter((id) => !priorIds.has(id)).length, removed: [...priorIds].filter((id) => !nextIds.has(id)).length };
  }

  close(): void { for (const flow of this.flows.values()) flow.controller.abort(); this.flows.clear(); }
}

function pause(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const finish = () => { signal.removeEventListener('abort', abort); resolve(); };
    const timer = setTimeout(finish, milliseconds);
    const abort = () => { clearTimeout(timer); signal.removeEventListener('abort', abort); reject(new Error('Account login cancelled')); };
    if (signal.aborted) abort(); else signal.addEventListener('abort', abort, { once: true });
  });
}
