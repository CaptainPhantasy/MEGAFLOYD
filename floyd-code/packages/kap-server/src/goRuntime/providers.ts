import { createProviderRequestSchema, replaceProviderRequestSchema, providerCollectionActionBodySchema } from '../protocol/rest-modelCatalog';
import { modelsDevProviderModels, resolveModelsDevImport, type ModelsDevCatalog, type ModelsDevProviderEntry } from '@legacy-ai/agent-core-v2/app/kosongConfig/modelsDev';
import { toModelsDevProviderItem } from '@legacy-ai/agent-core-v2/app/kosongConfig/modelsDevUpstream';
import { capabilitiesFromCustomEntry, credentialEnvHints, fetchCustomRegistry, FLOYD_CODE_PROVIDER_NAME } from '@legacy-ai/floyd-code-oauth';
import type { GoRuntime } from './runtime';
import { GoRequestError } from './client';
import { array, object, text } from './projection';

const wireTypes: Record<string, string> = { openai: 'openai-compat', openai_responses: 'openai', anthropic: 'anthropic', 'google-genai': 'google', vertexai: 'google-vertex', floyd: 'openai-compat' };
export function providerWireType(type: string): string { return ({ 'openai-compat': 'openai', openai: 'openai_responses', google: 'google-genai', 'google-vertex': 'vertexai' } as Record<string, string>)[type] || type; }

export class GoProviders {
  private writes: Promise<unknown> = Promise.resolve();
  constructor(readonly catalogUrl: string, readonly refreshManaged?: (runtime: GoRuntime, id: string) => Promise<unknown>) {}

  private async catalog(runtime: GoRuntime, apiKey?: string): Promise<ModelsDevCatalog> {
    const options = object((await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`))['options']);
    const company = text(options['company_platform_url']).replace(/\/+$/, '');
    const url = this.catalogUrl || text(options['catalog_url']) || (company ? `${company}/catalog/api.json` : '');
    if (!url) throw new GoRequestError(409, 'The company model catalog URL has not been configured');
    const response = await fetch(url, { headers: { Accept: 'application/json', 'User-Agent': 'MEGAFLOYD', ...(apiKey ? { Authorization: `Bearer ${apiKey}` } : {}) }, signal: AbortSignal.timeout(10_000) });
    if (!response.ok) throw new GoRequestError(502, `Model catalog returned HTTP ${response.status}`);
    const data: unknown = await response.json();
    if (!data || typeof data !== 'object' || Array.isArray(data)) throw new GoRequestError(502, 'Model catalog must contain a provider map');
    return data as ModelsDevCatalog;
  }

  private async mutate<T>(runtime: GoRuntime, action: () => Promise<T>): Promise<T> {
    const pending = this.writes.then(async () => {
      if ((await runtime.info()).is_busy) throw new GoRequestError(409, 'Wait for the shared Go agent to finish before changing provider configuration');
      const result = await action();
      await runtime.client.json('POST', `${runtime.base}/agent/update`, {});
      runtime.markDirty();
      return result;
    });
    this.writes = pending.catch(() => {});
    return pending;
  }

  private async set(runtime: GoRuntime, id: string, config: Record<string, unknown>) {
    await runtime.client.json('POST', `${runtime.base}/config/set`, { scope: 0, key: `providers.${configSegment(id)}`, value: config });
  }

  private async imported(runtime: GoRuntime, id: string, entry: ModelsDevProviderEntry, baseUrl?: string, apiKey?: string) {
    const resolution = resolveModelsDevImport(entry, baseUrl);
    if (resolution.kind !== 'ok') throw new GoRequestError(400, `Provider ${id} requires a valid supported protocol and endpoint`);
    const cfg = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
    const previous = object(object(cfg['providers'])[id]);
    if (id.startsWith('managed:') || previous['oauth']) throw new GoRequestError(409, `Provider ${id} is managed by OAuth`);
    const models = modelsDevProviderModels(entry).map((model) => ({ id: model.id, name: model.name || model.id, context_window: model.capability.max_context_tokens, default_max_tokens: model.maxOutputSize, supports_images: model.capability.image_in, supports_attachments: model.capability.image_in, can_reason: model.capability.thinking, reasoning_levels: model.supportEfforts, cost_per_1m_in: object(model.cost)['input'], cost_per_1m_out: object(model.cost)['output'] }));
    if (!models.length) throw new GoRequestError(400, `Provider ${id} has no usable chat models in this catalog`);
    await this.set(runtime, id, { ...previous, id, name: entry.name || id, type: wireTypes[resolution.wire], base_url: resolution.baseUrl, api_key: apiKey ?? previous['api_key'], models, discover_models: false });
    return models.length;
  }

  async route(runtime: GoRuntime, path: string, method: string, body: Record<string, unknown>): Promise<{ data: unknown; status?: number } | undefined> {
    if (method === 'GET' && path.startsWith('/catalog/providers')) {
      const catalog = await this.catalog(runtime);
      if (path === '/catalog/providers') return { data: { items: Object.entries(catalog).map(([id, entry]) => toModelsDevProviderItem(id, entry)) } };
      const id = path.slice('/catalog/providers/'.length);
      const entry = catalog[id];
      if (!entry) throw new GoRequestError(404, 'Provider was not found in the company model catalog');
      return { data: toModelsDevProviderItem(id, entry) };
    }
    if (path === '/providers' && method === 'POST') {
      const request = createProviderRequestSchema.parse(body);
      if (request.id.startsWith('managed:')) throw new GoRequestError(409, 'Manage company account credentials through sign in and logout');
      return { data: await this.mutate(runtime, async () => {
        const cfg = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
        if (object(cfg['providers'])[request.id]) throw new GoRequestError(409, 'Provider already exists');
        await this.writeForm(runtime, request.id, request, {});
        return (await runtime.catalog()).providers.find((item) => item.id === request.id);
      }) };
    }
    const provider = /^\/providers\/([^/]+)$/.exec(path);
    if (provider && method === 'PUT') {
      const request = replaceProviderRequestSchema.parse(body);
      const id = provider[1]!;
      if (id.startsWith('managed:') || request.new_id?.startsWith('managed:')) throw new GoRequestError(409, 'Manage company account credentials through sign in and logout');
      return { data: await this.mutate(runtime, async () => {
        const cfg = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
        const providers = object(cfg['providers']);
        if (!providers[id]) throw new GoRequestError(404, 'Provider is not configured');
        const previous = object(providers[id]);
        if (previous['oauth']) throw new GoRequestError(409, 'Log out of an OAuth-managed provider before replacing its credentials');
        const nextId = request.new_id || id;
        if (nextId !== id && providers[nextId]) throw new GoRequestError(409, 'The new provider ID already exists');
        await this.writeForm(runtime, nextId, request, previous);
        if (nextId !== id) {
          const selected = object(cfg['models']);
          for (const [kind, raw] of Object.entries(selected)) if (object(raw)['provider'] === id) await runtime.client.json('POST', `${runtime.base}/config/model`, { scope: 0, model_type: kind, model: { ...object(raw), provider: nextId } });
          await runtime.client.json('POST', `${runtime.base}/config/remove`, { scope: 0, key: `providers.${configSegment(id)}` });
        }
        return { provider: (await runtime.catalog()).providers.find((item) => item.id === nextId) };
      }) };
    }
    if (provider && method === 'DELETE') {
      if (provider[1]!.startsWith('managed:')) throw new GoRequestError(409, 'Sign out of the company account to remove its access');
      await this.mutate(runtime, () => runtime.client.json('POST', `${runtime.base}/config/remove`, { scope: 0, key: `providers.${configSegment(provider[1]!)}` }));
      return { data: null, status: 204 };
    }
    const action = /^\/providers(?::(refresh|refresh_oauth|import_catalog|import_registry)|\/([^/]+):refresh)$/.exec(path);
    if (method === 'POST' && action) {
      const request = providerCollectionActionBodySchema.parse(body);
      if (action[1] === 'import_catalog') {
        const entry = (await this.catalog(runtime))[request.catalog_id || ''];
        if (!entry) throw new GoRequestError(404, 'Provider was not found in the company model catalog');
        const id = request.id || request.catalog_id!;
        return { data: await this.mutate(runtime, async () => { const count = await this.imported(runtime, id, entry, request.base_url, request.api_key); return { provider: (await runtime.catalog()).providers.find((item) => item.id === id), models_imported: count }; }) };
      }
      if (action[1] === 'import_registry') {
        if (!request.url) throw new GoRequestError(400, 'Registry URL is required');
        const source = { kind: 'apiJson' as const, url: request.url, apiKey: request.api_key || '' };
        const catalog = await fetchCustomRegistry(source, { signal: AbortSignal.timeout(10_000), userAgent: 'MEGAFLOYD' });
        return { data: await this.mutate(runtime, async () => {
          const cfg = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
          const existing = object(cfg['providers']);
          for (const entry of Object.values(catalog)) {
            const previous = object(existing[entry.id]);
            if (previous['oauth']) throw new GoRequestError(409, `Provider ${entry.id} is managed by OAuth`);
            if (typeof previous['api_key'] === 'string' && previous['api_key'].startsWith('$') && object(previous['source'])['url'] === source.url && String(previous['base_url']).replace(/\/+$/, '') !== entry.api.replace(/\/+$/, '')) throw new GoRequestError(409, `Provider ${entry.id} changed endpoint; review its environment credential before importing`);
          }
          let count = 0;
          for (const entry of Object.values(catalog)) {
            const previous = object(existing[entry.id]);
            if (previous['oauth']) throw new GoRequestError(409, `Provider ${entry.id} is managed by OAuth`);
            const models = Object.values(entry.models).map((model) => ({ model: model.id, display_name: model.name, max_context_size: model.limit?.context || 131072, max_output_size: model.limit?.output, capabilities: capabilitiesFromCustomEntry(model), support_efforts: model.support_efforts ? [...model.support_efforts] : undefined }));
            if (!models.length) continue;
            const key = typeof previous['api_key'] === 'string' && previous['api_key'].startsWith('$') && object(previous['source'])['url'] === source.url ? previous['api_key'] : source.apiKey;
            await this.writeForm(runtime, entry.id, { type: entry.type, base_url: entry.api, api_key: key, models }, { ...previous, name: entry.name, source });
            count += models.length;
          }
          const ids = new Set(Object.values(catalog).map((entry) => entry.id));
          return { providers: (await runtime.catalog()).providers.filter((provider) => ids.has(provider.id)), models_imported: count, credential_env: credentialEnvHints(Object.values(catalog)) };
        }) };
      }
      return { data: await this.mutate(runtime, async () => {
        const cfg = await runtime.client.json<Record<string, unknown>>('GET', `${runtime.base}/config`);
        const providers = object(cfg['providers']);
        const selected = Object.entries(providers).filter(([id, raw]) => (!action[2] || id === action[2]) && (action[1] !== 'refresh_oauth' || id.startsWith('managed:') || object(raw)['oauth']));
        if (this.refreshManaged && action[2]?.startsWith('managed:') && !providers[action[2]]) selected.push([action[2], {}]);
        if (this.refreshManaged && action[1] === 'refresh_oauth' && !providers[FLOYD_CODE_PROVIDER_NAME]) {
          const account = await runtime.client.json<{ authenticated: boolean }>('GET', `${runtime.base}/auth/account?provider=${encodeURIComponent(FLOYD_CODE_PROVIDER_NAME)}`);
          if (account.authenticated) selected.push([FLOYD_CODE_PROVIDER_NAME, {}]);
        }
        if (action[2] && !selected.length) throw new GoRequestError(404, 'Provider is not configured');
        const registries = new Map<string, Awaited<ReturnType<typeof fetchCustomRegistry>>>();
        const result: { changed: Array<{ provider_id: string; provider_name: string; added: number; removed: number }>; unchanged: string[]; failed: Array<{ provider: string; reason: string }> } = { changed: [], unchanged: [], failed: [] };
        for (const [id, raw] of selected) {
          const previous = object(raw);
          const source = object(previous['source']);
          try {
          if (source['kind'] === 'apiJson') {
            const url = text(source['url']);
            const key = text(source['apiKey']);
            const cacheKey = JSON.stringify([url, key]);
            let catalog = registries.get(cacheKey);
            if (!catalog) { catalog = await fetchCustomRegistry({ kind: 'apiJson', url, apiKey: key }, { signal: AbortSignal.timeout(10_000), userAgent: 'MEGAFLOYD' }); registries.set(cacheKey, catalog); }
            const entry = catalog[id];
            if (!entry) throw new GoRequestError(404, `Provider ${id} is no longer present in its registry`);
            if (text(previous['api_key']).startsWith('$') && text(previous['base_url']).replace(/\/+$/, '') !== entry.api.replace(/\/+$/, '')) throw new GoRequestError(409, `Provider ${id} changed endpoint; review its environment credential before refreshing`);
            const models = Object.values(entry.models).map((model) => ({ model: model.id, display_name: model.name, max_context_size: model.limit?.context || 131072, max_output_size: model.limit?.output, capabilities: capabilitiesFromCustomEntry(model), support_efforts: model.support_efforts ? [...model.support_efforts] : undefined }));
            if (!models.length) throw new GoRequestError(409, `Provider ${id} has no usable models in its registry`);
            await this.writeForm(runtime, id, { type: entry.type, base_url: entry.api, models }, { ...previous, name: entry.name });
            const before = new Set(array(previous['models']).map((model) => text(object(model)['id'])));
            const after = new Set(models.map((model) => model.model));
            result.changed.push({ provider_id: id, provider_name: entry.name || id, added: [...after].filter((model) => !before.has(model)).length, removed: [...before].filter((model) => !after.has(model)).length });
          } else {
            const reply = object(id.startsWith('managed:') && this.refreshManaged ? await this.refreshManaged(runtime, id) : await runtime.client.json('POST', `${runtime.base}/providers/refresh`, { provider_id: id, oauth_only: action[1] === 'refresh_oauth' }));
            result.changed.push(...array(reply['changed']).map((value) => { const item = object(value); return { provider_id: text(item['provider_id']) || text(item['providerId']), provider_name: text(item['provider_name']) || text(item['providerName']) || id, added: Number(item['added']) || 0, removed: Number(item['removed']) || 0 }; }));
            result.unchanged.push(...array(reply['unchanged']).map(text));
            result.failed.push(...array(reply['failed']).map((value) => ({ provider: text(object(value)['provider']), reason: text(object(value)['reason']) })));
          }
          } catch (error) { result.failed.push({ provider: id, reason: error instanceof Error ? error.message : String(error) }); }
        }
        return result;
      }) };
    }
    return undefined;
  }

  private async writeForm(runtime: GoRuntime, id: string, request: { type: string; base_url?: string; api_key?: string; api_key_env?: string; default_model?: string; models: Array<{ model: string; display_name?: string; max_context_size: number; max_output_size?: number; capabilities?: string[]; support_efforts?: string[] }> }, previous: Record<string, unknown>) {
    if (request.api_key !== undefined && request.api_key_env !== undefined) throw new GoRequestError(400, 'Choose either an API key or an environment variable');
    if (request.api_key_env && !/^[A-Za-z_][A-Za-z0-9_]*$/.test(request.api_key_env)) throw new GoRequestError(400, 'Invalid API key environment variable');
    const key = request.api_key_env !== undefined ? request.api_key_env ? `$${request.api_key_env}` : '' : request.api_key ?? previous['api_key'];
    await this.set(runtime, id, { ...previous, id, type: wireTypes[request.type], base_url: request.base_url, api_key: key, discover_models: false, models: request.models.map((model) => ({ id: model.model, name: model.display_name || model.model, context_window: model.max_context_size, default_max_tokens: model.max_output_size, supports_images: model.capabilities?.some((capability) => ['image', 'image_in'].includes(capability)), supports_attachments: model.capabilities?.some((capability) => ['image', 'image_in', 'video_in', 'audio_in'].includes(capability)), can_reason: model.capabilities?.includes('thinking') || Boolean(model.support_efforts?.length), reasoning_levels: model.support_efforts })) });
    if (request.default_model) {
      const selected = request.models.find((model) => model.model === request.default_model)!;
      await runtime.client.json('POST', `${runtime.base}/config/model`, { scope: 0, model_type: 'large', model: { provider: id, model: selected.model, max_tokens: selected.max_output_size || Math.min(8192, selected.max_context_size) } });
    }
  }
}

function configSegment(value: string): string { return value.replaceAll('\\', '\\\\').replaceAll('.', '\\.'); }
