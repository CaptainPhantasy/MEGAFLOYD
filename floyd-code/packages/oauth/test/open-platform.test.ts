import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  applyOpenPlatformConfig,
  capabilitiesForModel,
  fetchOpenPlatformModels,
  filterModelsByPrefix,
  getOpenPlatformById,
  isOpenPlatformId,
  OPEN_PLATFORMS,
  OpenPlatformApiError,
  removeOpenPlatformConfig,
  type ManagedFloydConfigShape,
  type OpenPlatformDefinition,
} from '../src/open-platform';
import { refreshProviderModels, type RefreshProviderHost } from '../src/refreshProviderModels';

const PLATFORM_BASE_URL = 'https://api.example.test/v1';

function configuredPlatform(id: string, baseUrl: string = PLATFORM_BASE_URL): OpenPlatformDefinition {
  return { ...getOpenPlatformById(id)!, baseUrl };
}

function makeModelsResponse(): Response {
  return new Response(
    JSON.stringify({
      data: [
        {
          id: 'floyd-k2-0712-preview',
          context_length: 256000,
          supports_reasoning: true,
          supports_image_in: true,
          supports_video_in: true,
          display_name: 'Floyd K2 0712 Preview',
        },
        {
          id: 'floyd-k2-lite',
          context_length: 128000,
          supports_reasoning: false,
          supports_image_in: false,
          supports_video_in: false,
          supports_tool_use: false,
        },
        {
          id: 'non-floyd-model',
          context_length: 1000,
          supports_reasoning: false,
        },
      ],
    }),
    { status: 200, headers: { 'Content-Type': 'application/json' } },
  );
}

describe('OPEN_PLATFORMS', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it('keeps the platform ids and their model prefixes', () => {
    expect(OPEN_PLATFORMS.map((platform) => platform.id)).toEqual(['legacy-cn', 'legacy-ai']);
    expect(getOpenPlatformById('legacy-cn')).toMatchObject({
      id: 'legacy-cn',
      name: 'Floyd Platform (API key · mainland CN)',
      allowedPrefixes: ['floyd-k'],
    });
    expect(getOpenPlatformById('legacy-ai')).toMatchObject({
      id: 'legacy-ai',
      name: 'Floyd Platform (API key · global)',
      allowedPrefixes: ['floyd-k'],
    });
    expect(getOpenPlatformById('unknown')).toBeUndefined();
  });

  it('leaves the endpoints empty until their env override configures them', () => {
    expect(getOpenPlatformById('legacy-cn')).toMatchObject({
      baseUrl: '',
      consoleUrl: undefined,
    });
    expect(getOpenPlatformById('legacy-ai')).toMatchObject({
      baseUrl: '',
      consoleUrl: undefined,
    });

    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL', 'https://api.example.test/v1');
    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_CONSOLE_URL', 'https://console.example.test');
    expect(getOpenPlatformById('legacy-cn')).toMatchObject({
      baseUrl: 'https://api.example.test/v1',
      consoleUrl: 'https://console.example.test',
    });
    expect(getOpenPlatformById('legacy-ai')?.baseUrl).toBe('');
  });

  it('isOpenPlatformId works', () => {
    expect(isOpenPlatformId('legacy-cn')).toBe(true);
    expect(isOpenPlatformId('legacy-ai')).toBe(true);
    expect(isOpenPlatformId('floyd-code')).toBe(false);
  });
});

describe('fetchOpenPlatformModels', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it('lists and parses models from the configured platform endpoint', async () => {
    const fetchMock = vi.fn(async () => makeModelsResponse());
    const platform = configuredPlatform('legacy-cn');

    const models = await fetchOpenPlatformModels(platform, 'sk-test', fetchMock as unknown as typeof fetch);

    expect(models).toHaveLength(3);
    expect(models[0]).toMatchObject({
      id: 'floyd-k2-0712-preview',
      contextLength: 256000,
      supportsReasoning: true,
      supportsImageIn: true,
      supportsVideoIn: true,
      displayName: 'Floyd K2 0712 Preview',
    });
    expect(models[1]?.supportsToolUse).toBe(false);
    expect(models[2]?.id).toBe('non-floyd-model');

    expect(fetchMock).toHaveBeenCalledWith(
      `${PLATFORM_BASE_URL}/models`,
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer sk-test',
          Accept: 'application/json',
        }),
      }),
    );
  });

  it('refuses to fetch when the platform has no base URL configured', async () => {
    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL', '');
    const fetchMock = vi.fn();

    const error = await fetchOpenPlatformModels(
      getOpenPlatformById('legacy-cn')!,
      'sk-test',
      fetchMock as unknown as typeof fetch,
    ).catch((error: unknown) => error);

    expect(error).toBeInstanceOf(Error);
    expect((error as Error).message).toContain(
      'No base URL configured for platform "legacy-cn"',
    );
    expect((error as Error).message).toContain('base_url');
    expect((error as Error).message).toContain('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('surfaces API error messages and status on HTTP error', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(JSON.stringify({ error: { message: 'invalid API key' } }), { status: 401 }),
    );
    const platform = configuredPlatform('legacy-cn');

    const error = await fetchOpenPlatformModels(
      platform,
      'sk-bad',
      fetchMock as unknown as typeof fetch,
    ).catch((error: unknown) => error);

    expect(error).toBeInstanceOf(OpenPlatformApiError);
    expect((error as OpenPlatformApiError).status).toBe(401);
    expect((error as Error).message).toBe('invalid API key');
  });

  it('throws on unexpected response shape', async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({}), { status: 200 }));
    const platform = configuredPlatform('legacy-cn');

    await expect(
      fetchOpenPlatformModels(platform, 'sk-test', fetchMock as unknown as typeof fetch),
    ).rejects.toThrow(/Unexpected models response/);
  });
});

describe('filterModelsByPrefix', () => {
  it('filters by allowedPrefixes when present', () => {
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      { id: 'floyd-k2-0712-preview', contextLength: 256000, supportsReasoning: true, supportsImageIn: true, supportsVideoIn: true },
      { id: 'gpt-4', contextLength: 1000, supportsReasoning: false, supportsImageIn: false, supportsVideoIn: false },
    ];

    const filtered = filterModelsByPrefix(models as unknown as import('../src/managed-floyd-code').ManagedFloydCodeModelInfo[], platform);
    expect(filtered).toHaveLength(1);
    expect(filtered[0]?.id).toBe('floyd-k2-0712-preview');
  });

  it('returns all models when allowedPrefixes is absent', () => {
    const platform: import('../src/open-platform').OpenPlatformDefinition = {
      id: 'custom',
      name: 'Custom',
      baseUrl: 'https://example.com/v1',
    };
    const models = [
      { id: 'model-a', contextLength: 1000, supportsReasoning: false, supportsImageIn: false, supportsVideoIn: false },
      { id: 'model-b', contextLength: 2000, supportsReasoning: false, supportsImageIn: false, supportsVideoIn: false },
    ];

    const filtered = filterModelsByPrefix(models as unknown as import('../src/managed-floyd-code').ManagedFloydCodeModelInfo[], platform);
    expect(filtered).toHaveLength(2);
  });
});

describe('fetchOpenPlatformModels supports_thinking_type', () => {
  it('parses supports_thinking_type from the models endpoint', async () => {
    const fetchMock = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [
              {
                id: 'floyd-k2-deep',
                context_length: 256000,
                supports_reasoning: true,
                supports_thinking_type: 'only',
              },
              {
                id: 'floyd-k2-lite',
                context_length: 128000,
                supports_reasoning: false,
              },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    );
    const platform = configuredPlatform('legacy-cn');

    const models = await fetchOpenPlatformModels(platform, 'sk-test', fetchMock as unknown as typeof fetch);

    expect(models[0]?.supportsThinkingType).toBe('only');
    expect(models[1]?.supportsThinkingType).toBeUndefined();
  });
});

describe('capabilitiesForModel', () => {
  it("locks thinking on for 'only' models", () => {
    const model = {
      id: 'deep',
      contextLength: 1000,
      supportsReasoning: true,
      supportsImageIn: false,
      supportsVideoIn: false,
      supportsToolUse: false,
      supportsThinkingType: 'only' as const,
    };
    expect(capabilitiesForModel(model)).toEqual(['thinking', 'always_thinking']);
  });

  it("lets 'no' override the legacy supports_reasoning boolean", () => {
    const model = {
      id: 'plain',
      contextLength: 1000,
      supportsReasoning: true,
      supportsImageIn: false,
      supportsVideoIn: false,
      supportsToolUse: false,
      supportsThinkingType: 'no' as const,
    };
    expect(capabilitiesForModel(model)).toBeUndefined();
  });

  it("emits a plain toggleable thinking capability for 'both'", () => {
    const model = {
      id: 'toggle',
      contextLength: 1000,
      supportsReasoning: false,
      supportsImageIn: false,
      supportsVideoIn: false,
      supportsToolUse: false,
      supportsThinkingType: 'both' as const,
    };
    expect(capabilitiesForModel(model)).toEqual(['thinking']);
  });

  it('returns undefined for a model with no capabilities', () => {
    const model = {
      id: 'plain',
      contextLength: 1000,
      supportsReasoning: false,
      supportsImageIn: false,
      supportsVideoIn: false,
      supportsToolUse: false,
    };
    expect(capabilitiesForModel(model)).toBeUndefined();
  });

  it('returns all caps for a full-featured model', () => {
    const model = {
      id: 'full',
      contextLength: 1000,
      supportsReasoning: true,
      supportsImageIn: true,
      supportsVideoIn: true,
      supportsToolUse: true,
    };
    expect(capabilitiesForModel(model)).toEqual(['thinking', 'image_in', 'video_in', 'tool_use']);
  });
});

describe('applyOpenPlatformConfig', () => {
  it('writes provider, models, and defaults', () => {
    const config: ManagedFloydConfigShape = {
      providers: {},
    };
    const platform = configuredPlatform('legacy-cn');
    const models = [
      { id: 'floyd-k2-0712-preview', contextLength: 256000, supportsReasoning: true, supportsImageIn: true, supportsVideoIn: true, displayName: 'Floyd K2' },
      { id: 'floyd-k2-lite', contextLength: 128000, supportsReasoning: false, supportsImageIn: false, supportsVideoIn: false },
    ];

    const result = applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: true,
      credential: { apiKey: 'sk-test' },
    });

    expect(result).toEqual({
      defaultModel: 'legacy-cn/floyd-k2-0712-preview',
      defaultThinking: true,
    });

    expect(config.providers['legacy-cn']).toMatchObject({
      type: 'floyd',
      baseUrl: PLATFORM_BASE_URL,
      apiKey: 'sk-test',
    });
    expect(config.models?.['legacy-cn/floyd-k2-0712-preview']).toMatchObject({
      provider: 'legacy-cn',
      model: 'floyd-k2-0712-preview',
      maxContextSize: 256000,
      capabilities: ['thinking', 'image_in', 'video_in', 'tool_use'],
      displayName: 'Floyd K2',
    });
    expect(config.defaultModel).toBe('legacy-cn/floyd-k2-0712-preview');
    expect(config.thinking?.enabled).toBe(true);
    expect(config.services).toBeUndefined();
  });

  it('clears stale models for the same provider', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: 'https://stale.example.test/v1',
          apiKey: 'sk-old',
        },
      },
      models: {
        'legacy-cn/stale': { provider: 'legacy-cn', model: 'stale', maxContextSize: 1000 },
        'other/model': { provider: 'other', model: 'other-model', maxContextSize: 1000 },
      },
    };
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      { id: 'floyd-k2-0712-preview', contextLength: 256000, supportsReasoning: true, supportsImageIn: true, supportsVideoIn: true },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: false,
      credential: { apiKey: 'sk-new' },
    });

    expect(config.models?.['legacy-cn/stale']).toBeUndefined();
    expect(config.models?.['other/model']).toBeDefined();
  });

  it('preserves hand-edited fields that upstream does not declare', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: 'https://stale.example.test/v1',
          apiKey: 'sk-old',
        },
      },
      models: {
        'legacy-cn/floyd-k2-0712-preview': {
          provider: 'legacy-cn',
          model: 'floyd-k2-0712-preview',
          maxContextSize: 256000,
          maxOutputSize: 8192,
          supportEfforts: ['low', 'high'],
        } as Record<string, unknown>,
      },
    };
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      {
        id: 'floyd-k2-0712-preview',
        contextLength: 256000,
        supportsReasoning: true,
        supportsImageIn: true,
        supportsVideoIn: true,
      },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: false,
      credential: { apiKey: 'sk-new' },
    });

    const alias = config.models?.['legacy-cn/floyd-k2-0712-preview'];
    expect(alias?.['maxOutputSize']).toBe(8192);
    expect(alias?.['supportEfforts']).toBeUndefined();
  });

  it('preserves open-platform overrides during refresh', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: 'https://stale.example.test/v1',
          apiKey: 'sk-old',
        },
      },
      models: {
        'legacy-cn/floyd-k2-0712-preview': {
          provider: 'legacy-cn',
          model: 'floyd-k2-0712-preview',
          maxContextSize: 256000,
          overrides: { supportEfforts: ['low'] },
        } as Record<string, unknown>,
      },
    };
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      {
        id: 'floyd-k2-0712-preview',
        contextLength: 256000,
        supportsReasoning: true,
        supportsImageIn: false,
        supportsVideoIn: false,
        supportEfforts: ['low', 'high'],
      },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: false,
      credential: { apiKey: 'sk-new' },
    });

    const alias = config.models?.['legacy-cn/floyd-k2-0712-preview'];
    expect(alias?.['supportEfforts']).toEqual(['low', 'high']);
    expect(alias?.['overrides']).toEqual({ supportEfforts: ['low'] });
  });

  it('writes a concrete effort into config.thinking when provided', () => {
    const config: ManagedFloydConfigShape = { providers: {} };
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      {
        id: 'floyd-k2-0712-preview',
        contextLength: 256000,
        supportsReasoning: true,
        supportsImageIn: false,
        supportsVideoIn: false,
      },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: true,
      effort: 'high',
      credential: { apiKey: 'sk-test' },
    });

    expect(config.thinking).toEqual({ enabled: true, effort: 'high' });
  });

  it('omits effort for a boolean on (no concrete effort)', () => {
    const config: ManagedFloydConfigShape = { providers: {} };
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      {
        id: 'floyd-k2-0712-preview',
        contextLength: 256000,
        supportsReasoning: true,
        supportsImageIn: false,
        supportsVideoIn: false,
      },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: true,
      credential: { apiKey: 'sk-test' },
    });

    expect(config.thinking).toEqual({ enabled: true });
    expect(config.thinking?.effort).toBeUndefined();
  });

  it('drops custom-registry provenance when materializing an open platform', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'openai',
          baseUrl: 'https://registry.example.test/v1',
          apiKey: 'sk-registry',
          source: {
            kind: 'apiJson',
            url: 'https://registry.example.test/api.json',
            apiKey: 'sk-registry',
          },
        },
      },
    };
    const platform = getOpenPlatformById('legacy-cn')!;
    const models = [
      {
        id: 'floyd-k2',
        contextLength: 131072,
        supportsReasoning: false,
        supportsImageIn: false,
        supportsVideoIn: false,
      },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: false,
      credential: { apiKey: 'sk-registry' },
    });

    expect(config.providers['legacy-cn']).not.toHaveProperty('source');
  });

  it('persists the apiKeyEnv declaration instead of a resolved secret and keeps hand-written fields', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: 'https://stale.example.test/v1',
          apiKey: 'sk-resolved-secret',
          customHeaders: { 'X-Team': 'infra' },
        },
      },
    };
    const platform = configuredPlatform('legacy-cn');
    const models = [
      {
        id: 'floyd-k2',
        contextLength: 131072,
        supportsReasoning: false,
        supportsImageIn: false,
        supportsVideoIn: false,
      },
    ];

    applyOpenPlatformConfig(config, {
      platform,
      models,
      selectedModel: models[0]!,
      thinking: false,
      credential: { apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY' },
    });

    expect(config.providers['legacy-cn']).toEqual({
      type: 'floyd',
      baseUrl: PLATFORM_BASE_URL,
      apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY',
      customHeaders: { 'X-Team': 'infra' },
    });
  });
});

describe('removeOpenPlatformConfig', () => {
  it('removes provider, its models, and defaultModel when matched', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: 'https://stale.example.test/v1',
          apiKey: 'sk-test',
        },
        'other': { type: 'floyd', baseUrl: 'https://other.test/v1', apiKey: 'sk-other' },
      },
      models: {
        'legacy-cn/floyd-k2': { provider: 'legacy-cn', model: 'floyd-k2', maxContextSize: 256000 },
        'other/model': { provider: 'other', model: 'other-model', maxContextSize: 1000 },
      },
      defaultModel: 'legacy-cn/floyd-k2',
    };

    removeOpenPlatformConfig(config, 'legacy-cn');

    expect(config.providers['legacy-cn']).toBeUndefined();
    expect(config.providers['other']).toBeDefined();
    expect(config.models?.['legacy-cn/floyd-k2']).toBeUndefined();
    expect(config.models?.['other/model']).toBeDefined();
    expect(config.defaultModel).toBeUndefined();
  });

  it('leaves defaultModel intact when it belongs to another provider', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: 'https://stale.example.test/v1',
          apiKey: 'sk-test',
        },
      },
      models: {
        'legacy-cn/floyd-k2': { provider: 'legacy-cn', model: 'floyd-k2', maxContextSize: 256000 },
      },
      defaultModel: 'other/model',
    };

    removeOpenPlatformConfig(config, 'legacy-cn');

    expect(config.defaultModel).toBe('other/model');
  });
});

describe('refreshProviderModels platform base URL', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  const PROVIDER_BASE_URL = 'https://platform.example.test/v1';

  function makeHost(initial: ManagedFloydConfigShape): {
    host: RefreshProviderHost;
    snapshot: () => ManagedFloydConfigShape;
  } {
    let current = structuredClone(initial);
    return {
      snapshot: () => current,
      host: {
        getConfig: async () => structuredClone(current),
        removeProvider: async (providerId) => {
          delete current.providers[providerId];
          return structuredClone(current);
        },
        setConfig: async (patch) => {
          current = {
            ...current,
            ...patch,
            providers: { ...current.providers, ...patch.providers },
            models: { ...current.models, ...patch.models },
          };
          return structuredClone(current);
        },
        resolveOAuthToken: async () => '',
      },
    };
  }

  function makeEnvKeyProvider(): ManagedFloydConfigShape {
    vi.stubEnv('FLOYD_TEST_OPEN_PLATFORM_KEY', 'sk-open-platform');
    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL', '');
    return {
      providers: {
        'legacy-cn': {
          type: 'floyd',
          baseUrl: PROVIDER_BASE_URL,
          apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY',
        },
      },
      models: {},
    };
  }

  it('refreshes through the configured provider base_url when the platform env override is unset', async () => {
    const fetchMock = vi.fn(async () => makeModelsResponse());
    vi.stubGlobal('fetch', fetchMock);
    const { host, snapshot } = makeHost(makeEnvKeyProvider());

    const result = await refreshProviderModels(host, { providerId: 'legacy-cn' });

    expect(result.failed).toEqual([]);
    expect(result.changed).toEqual([
      {
        providerId: 'legacy-cn',
        providerName: 'Floyd Platform (API key · mainland CN)',
        added: 2,
        removed: 0,
      },
    ]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith(
      `${PROVIDER_BASE_URL}/models`,
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer sk-open-platform' }),
      }),
    );
    expect(snapshot().providers['legacy-cn']).toEqual({
      type: 'floyd',
      baseUrl: PROVIDER_BASE_URL,
      apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY',
    });
    expect(snapshot().models?.['legacy-cn/floyd-k2-0712-preview']).toMatchObject({
      provider: 'legacy-cn',
      model: 'floyd-k2-0712-preview',
      maxContextSize: 256000,
    });
    expect(snapshot().models?.['legacy-cn/non-floyd-model']).toBeUndefined();
  });

  it('prefers the platform env override over the provider base_url', async () => {
    const config = makeEnvKeyProvider();
    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL', 'https://env.example.test/v1');
    const fetchMock = vi.fn(async () => makeModelsResponse());
    vi.stubGlobal('fetch', fetchMock);
    const { host, snapshot } = makeHost(config);

    const result = await refreshProviderModels(host, { providerId: 'legacy-cn' });

    expect(result.failed).toEqual([]);
    expect(fetchMock).toHaveBeenCalledWith(
      'https://env.example.test/v1/models',
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: 'Bearer sk-open-platform' }),
      }),
    );
    expect(snapshot().providers['legacy-cn']).toMatchObject({
      baseUrl: 'https://env.example.test/v1',
      apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY',
    });
  });

  it('reports the base_url setting to provide when neither the provider nor the platform has one', async () => {
    vi.stubEnv('FLOYD_TEST_OPEN_PLATFORM_KEY', 'sk-open-platform');
    vi.stubEnv('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL', '');
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const { host, snapshot } = makeHost({
      providers: {
        'legacy-cn': { type: 'floyd', apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY' },
      },
      models: {},
    });

    const result = await refreshProviderModels(host, { providerId: 'legacy-cn' });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(result.changed).toEqual([]);
    expect(result.failed).toHaveLength(1);
    expect(result.failed[0]?.provider).toBe('legacy-cn');
    expect(result.failed[0]?.reason).toContain('base_url');
    expect(result.failed[0]?.reason).toContain('FLOYD_CODE_OPEN_PLATFORM_LEGACY_CN_BASE_URL');
    expect(snapshot().providers['legacy-cn']).toEqual({
      type: 'floyd',
      apiKeyEnv: 'FLOYD_TEST_OPEN_PLATFORM_KEY',
    });
  });
});
