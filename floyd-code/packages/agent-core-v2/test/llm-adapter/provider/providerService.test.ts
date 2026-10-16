import { describe, expect, it } from 'vitest';

import {
  ProvidersSectionSchema,
  providersFromToml,
  providersToToml,
} from '#/app/kosongConfig/configSection';
import { ProviderService } from '#/llm-adapter/provider/provider-service';
import { type ProviderConfig } from '#/llm-adapter/provider/provider';

describe('ProviderTypeSchema (free-form vendor identity)', () => {
  it('parses unregistered vendor names — resolve-time validation, not parse-time', () => {
    const parsed = ProvidersSectionSchema.parse({
      'my-vendor': { type: 'a-vendor-registered-elsewhere', baseUrl: 'https://example.com/v1' },
    });
    expect(parsed['my-vendor']?.type).toBe('a-vendor-registered-elsewhere');
  });
});

describe('providers TOML transforms', () => {
  it('converts snake_case entries to camelCase and back', () => {
    const from = providersFromToml({
      'my-provider': {
        type: 'floyd',
        base_url: 'https://api.legacy.ai/v1',
        custom_headers: { 'x-a': 'b' },
        default_model: 'floyd-k2',
        oauth: { storage: 'file', key: 'k', oauth_host: 'example.com' },
      },
    }) as Record<string, Record<string, unknown>>;
    expect(from['my-provider']).toEqual({
      type: 'floyd',
      baseUrl: 'https://api.legacy.ai/v1',
      customHeaders: { 'x-a': 'b' },
      defaultModel: 'floyd-k2',
      oauth: { storage: 'file', key: 'k', oauthHost: 'example.com' },
    });

    const back = providersToToml(from, undefined) as Record<string, Record<string, unknown>>;
    expect(back['my-provider']).toEqual({
      type: 'floyd',
      base_url: 'https://api.legacy.ai/v1',
      custom_headers: { 'x-a': 'b' },
      default_model: 'floyd-k2',
      oauth: { storage: 'file', key: 'k', oauth_host: 'example.com' },
    });
  });

  it('round-trips api_key_env between TOML and camelCase', () => {
    const from = providersFromToml({
      acme: { type: 'openai', api_key_env: 'ACME_API_KEY' },
    }) as Record<string, Record<string, unknown>>;
    expect(from['acme']).toEqual({ type: 'openai', apiKeyEnv: 'ACME_API_KEY' });

    const parsed = ProvidersSectionSchema.parse(from);
    expect(parsed['acme']?.apiKeyEnv).toBe('ACME_API_KEY');

    const back = providersToToml(from, undefined) as Record<string, Record<string, unknown>>;
    expect(back['acme']).toEqual({ type: 'openai', api_key_env: 'ACME_API_KEY' });
  });

  it('drops a stale api_key_env and oauth when the provider is replaced with an inline api_key', () => {
    const raw = {
      acme: {
        type: 'openai',
        api_key_env: 'ACME_API_KEY',
        oauth: { storage: 'file', key: 'oauth/acme' },
      },
    };
    const next = providersToToml(
      { acme: { type: 'openai', apiKey: 'sk-new' } },
      raw,
    ) as Record<string, Record<string, unknown>>;
    expect(next['acme']).toEqual({ type: 'openai', api_key: 'sk-new' });
  });
});

describe('ProviderService', () => {
  function createService(providers: Readonly<Record<string, ProviderConfig>> = {}): ProviderService {
    const service = new ProviderService();
    service.loadAll({ ...providers }, undefined);
    return service;
  }

  it('resolves ready on the first loadAll and gates mutations on it', async () => {
    const service = new ProviderService();
    let ready = false;
    void service.ready.then(() => {
      ready = true;
    });
    await Promise.resolve();
    expect(ready).toBe(false);

    service.loadAll({ legacy: { type: 'floyd' } }, 'legacy');
    await service.ready;
    expect(ready).toBe(true);
    expect(service.get('legacy')).toEqual({ type: 'floyd' });
    expect(service.getDefaultProvider()).toBe('legacy');
  });

  it('supports CRUD and diffs state changes into onDidChangeProviders', async () => {
    const service = createService();
    const events: Array<{
      added: readonly string[];
      removed: readonly string[];
      changed: readonly string[];
    }> = [];
    service.onDidChangeProviders((e) =>
      events.push({ added: e.added, removed: e.removed, changed: e.changed }),
    );

    const legacy: ProviderConfig = { type: 'floyd', baseUrl: 'https://api.legacy.ai/v1' };
    await service.set('legacy', legacy);
    expect(service.get('legacy')).toEqual(legacy);
    expect(service.list()).toEqual({ legacy });
    expect(events).toEqual([{ added: ['legacy'], removed: [], changed: [] }]);

    const updated: ProviderConfig = { ...legacy, apiKey: 'sk-1' };
    await service.set('legacy', updated);
    expect(events.at(-1)).toEqual({ added: [], removed: [], changed: ['legacy'] });

    await service.set('legacy', updated);
    expect(events).toHaveLength(2);

    await service.delete('legacy');
    expect(service.get('legacy')).toBeUndefined();
    expect(events.at(-1)).toEqual({ added: [], removed: ['legacy'], changed: [] });
  });

  it('loadAll fires only for real diffs on re-sync', async () => {
    const service = createService({ legacy: { type: 'floyd' } });
    const events: unknown[] = [];
    service.onDidChangeProviders((e) =>
      events.push({ added: e.added, removed: e.removed, changed: e.changed }),
    );

    service.loadAll({ legacy: { type: 'floyd' } }, undefined);
    expect(events).toHaveLength(0);

    service.loadAll({ legacy: { type: 'floyd' }, other: { baseUrl: 'https://example.com' } }, undefined);
    expect(events).toEqual([{ added: ['other'], removed: [], changed: [] }]);
  });

  it('replaceAll replaces the records and keeps the default pointer', async () => {
    const service = createService({ a: { type: 'floyd' }, b: { type: 'floyd' } });
    await service.setDefaultProvider('a');

    await service.replaceAll({ c: { type: 'floyd' } });
    expect(service.list()).toEqual({ c: { type: 'floyd' } });
    expect(service.getDefaultProvider()).toBe('a');
  });

  it('clears the defaultProvider pointer when the default provider is deleted', async () => {
    const service = createService({ legacy: { type: 'floyd' } });
    const pointerEvents: Array<string | undefined> = [];
    service.onDidChangeDefaultProvider((e) => pointerEvents.push(e.id));

    await service.setDefaultProvider('legacy');
    expect(service.getDefaultProvider()).toBe('legacy');

    await service.delete('legacy');
    expect(service.getDefaultProvider()).toBeUndefined();
    expect(pointerEvents).toEqual(['legacy', undefined]);
  });

  it('a mutation resolves only after the listeners’ waitUntil work completes', async () => {
    const service = createService();
    let persistDone = false;
    service.onDidChangeProviders((e) => {
      e.waitUntil(
        new Promise<void>((resolve) => setTimeout(resolve, 50)).then(() => {
          persistDone = true;
        }),
      );
    });

    await service.set('legacy', { type: 'floyd' });
    expect(persistDone).toBe(true);
  });
});
