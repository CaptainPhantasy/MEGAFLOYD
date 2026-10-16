import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  applyManagedApiKeyProviderModels,
  applyManagedFloydCodeLogoutConfig,
  applyManagedFloydCodeConfig,
  clearManagedFloydCodeConfig,
  fetchManagedFloydCodeModels,
  FLOYD_CODE_OAUTH_KEY,
  FLOYD_CODE_PROVIDER_NAME,
  ManagedFloydCodeModelsAuthError,
  provisionManagedFloydCodeConfig,
  resolveFloydCodeLoginAuth,
  resolveFloydCodeOAuthKey,
  resolveFloydCodeOAuthRef,
  resolveFloydCodeRuntimeAuth,
  type ManagedFloydCodeModelInfo,
  type ManagedFloydConfigShape,
} from '../src/managed-floyd-code';
import { OAuthUnauthorizedError } from '../src/errors';

function makeModelsResponse(): Response {
  return new Response(
    JSON.stringify({
      data: [
        {
          id: 'floyd-for-coding',
          context_length: 262144,
          supports_reasoning: true,
          supports_image_in: true,
          supports_video_in: true,
          display_name: 'Floyd for Coding',
        },
        {
          id: 'floyd-k2.5',
          context_length: 250000,
          supports_reasoning: false,
          supports_image_in: false,
          supports_video_in: false,
          supports_tool_use: false,
        },
      ],
    }),
    { status: 200, headers: { 'Content-Type': 'application/json' } },
  );
}

const MANAGED_BASE_URL = 'https://api.example.test/coding/v1';

// These tests run against a configured managed environment: no deployment is
// baked in, so the models endpoint is only reachable through configuration.
// The unconfigured path has its own test below.
beforeEach(() => {
  vi.stubEnv('FLOYD_CODE_BASE_URL', MANAGED_BASE_URL);
});

afterEach(() => {
  vi.unstubAllEnvs();
});

describe('provisionManagedFloydCodeConfig', () => {
  it('keeps the shared credential key when no host or base URL is configured', () => {
    expect(resolveFloydCodeOAuthKey({ oauthHost: '', baseUrl: '' })).toBe(FLOYD_CODE_OAUTH_KEY);
  });

  it('scopes credential keys for non-default OAuth hosts and API base URLs', () => {
    const devKey = resolveFloydCodeOAuthKey({
      oauthHost: 'https://auth.dev.example.test',
      baseUrl: 'https://api.dev.example.test/coding/v1',
    });

    expect(devKey).not.toBe(FLOYD_CODE_OAUTH_KEY);
    expect(devKey).toMatch(/^oauth\/floyd-code-env-[a-f0-9]{16}$/);
    expect(
      resolveFloydCodeOAuthKey({
        oauthHost: 'https://auth.dev.example.test/',
        baseUrl: 'https://api.dev.example.test/coding/v1/',
      }),
    ).toBe(devKey);
  });

  it('derives a full OAuth ref whose key and persisted host stay in sync', () => {
    // An unconfigured environment collapses to the shared ref with no
    // persisted host, so existing `floyd-code.json` credentials keep working.
    expect(
      resolveFloydCodeOAuthRef({
        oauthHost: '',
        baseUrl: '',
      }),
    ).toEqual({ storage: 'file', key: FLOYD_CODE_OAUTH_KEY, oauthHost: undefined });

    // A custom API base with no OAuth host of its own scopes the key and
    // persists the empty host it was derived from.
    const customApiRef = resolveFloydCodeOAuthRef({
      baseUrl: 'https://api.example.test/coding/v1',
    });
    expect(customApiRef).toEqual({
      storage: 'file',
      key: resolveFloydCodeOAuthKey({
        oauthHost: '',
        baseUrl: 'https://api.example.test/coding/v1',
      }),
      oauthHost: '',
    });

    // A configured environment yields a scoped key AND the normalized host,
    // both derived from the same input — login and runtime cannot drift apart.
    const devRef = resolveFloydCodeOAuthRef({
      oauthHost: 'https://auth.dev.example.test/',
      baseUrl: 'https://api.dev.example.test/coding/v1',
    });
    expect(devRef).toEqual({
      storage: 'file',
      key: resolveFloydCodeOAuthKey({
        oauthHost: 'https://auth.dev.example.test',
        baseUrl: 'https://api.dev.example.test/coding/v1',
      }),
      oauthHost: 'https://auth.dev.example.test',
    });
  });

  it('resolves runtime auth from environment overrides over persisted config', () => {
    const configuredBaseUrl = 'https://api.configured.example.test/coding/v1';
    const envBaseUrl = 'https://api.env.example.test/coding/v1/';
    const envOauthHost = 'https://auth.env.example.test/';
    const configuredOAuthRef = resolveFloydCodeOAuthRef({
      baseUrl: configuredBaseUrl,
    });

    const auth = resolveFloydCodeRuntimeAuth({
      configuredBaseUrl,
      configuredOAuthRef,
      env: {
        FLOYD_CODE_BASE_URL: envBaseUrl,
        FLOYD_CODE_OAUTH_HOST: envOauthHost,
      },
    });

    expect(auth.baseUrl).toBe('https://api.env.example.test/coding/v1');
    expect(auth.oauthRef).toEqual({
      storage: 'file',
      key: resolveFloydCodeOAuthKey({
        oauthHost: 'https://auth.env.example.test',
        baseUrl: 'https://api.env.example.test/coding/v1',
      }),
      oauthHost: 'https://auth.env.example.test',
    });
  });

  it('preserves a matching configured runtime OAuth ref when env is not overridden', () => {
    const baseUrl = 'https://api.dev.example.test/coding/v1';
    const configuredOAuthRef = {
      storage: 'keyring' as const,
      key: resolveFloydCodeOAuthKey({
        oauthHost: 'https://auth.dev.example.test',
        baseUrl,
      }),
      oauthHost: 'https://auth.dev.example.test',
    };

    expect(
      resolveFloydCodeRuntimeAuth({
        configuredBaseUrl: baseUrl,
        configuredOAuthRef,
        env: {},
      }),
    ).toEqual({
      baseUrl,
      oauthRef: configuredOAuthRef,
    });
  });

  it('resolves login auth without reusing persisted refs under explicit or env overrides', () => {
    const configuredBaseUrl = 'https://api.configured.example.test/coding/v1';
    const configuredOAuthRef = resolveFloydCodeOAuthRef({ baseUrl: configuredBaseUrl });

    expect(
      resolveFloydCodeLoginAuth({
        configuredBaseUrl,
        configuredOAuthRef,
        requestedBaseUrl: 'https://api.requested.example.test/coding/v1/',
        env: {},
      }),
    ).toEqual({
      baseUrl: 'https://api.requested.example.test/coding/v1',
      oauthHost: undefined,
    });

    expect(
      resolveFloydCodeLoginAuth({
        configuredBaseUrl,
        configuredOAuthRef,
        env: {},
      }),
    ).toEqual({
      baseUrl: configuredBaseUrl,
      oauthHost: undefined,
      oauthRef: configuredOAuthRef,
    });
  });

  it('refuses to provision when no API base URL is configured', async () => {
    const fetchMock = vi.fn();
    const config: ManagedFloydConfigShape = { providers: {} };

    await expect(
      provisionManagedFloydCodeConfig({
        accessToken: 'oauth-access-token',
        baseUrl: '',
        fetchImpl: fetchMock as unknown as typeof fetch,
        adapter: {
          read: () => config,
          write: vi.fn(),
          apply: applyManagedFloydCodeConfig,
        },
      }),
    ).rejects.toThrow(/FLOYD_CODE_BASE_URL/);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('omits the derived service endpoints when no API base URL is configured', () => {
    const config: ManagedFloydConfigShape = { providers: {} };

    applyManagedFloydCodeConfig(config, {
      models: [
        {
          id: 'floyd-for-coding',
          contextLength: 262144,
          supportsReasoning: true,
          supportsImageIn: false,
          supportsVideoIn: false,
        },
      ],
      baseUrl: '',
    });

    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({ baseUrl: '' });
    // `/search` and `/fetch` would be root-relative without a base.
    expect(config.services).toBeUndefined();
  });

  it('writes the managed provider, models, services, and default model through an adapter', async () => {
    const baseUrl = 'https://api.example.test/coding/v1';
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
          baseUrl: 'https://example.test/v1',
        },
      },
      models: {
        'floyd-code/stale': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'stale',
        },
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
        },
      },
    };
    const write = vi.fn();
    const fetchMock = vi.fn(async () => makeModelsResponse());

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      baseUrl,
      fetchImpl: fetchMock as unknown as typeof fetch,
      adapter: {
        configPath: '/tmp/config.toml',
        read: () => config,
        write,
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result).toMatchObject({
      providerName: FLOYD_CODE_PROVIDER_NAME,
      defaultModel: 'floyd-code/floyd-for-coding',
      defaultThinking: true,
      configPath: '/tmp/config.toml',
    });
    expect(result.models[0]?.supportsToolUse).toBe(true);
    expect(result.models[1]?.supportsToolUse).toBe(false);
    expect(fetchMock).toHaveBeenCalledWith(
      `${baseUrl}/models`,
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer oauth-access-token',
          Accept: 'application/json',
        }),
      }),
    );
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit?][];
    const init = calls[0]?.[1] ?? {};
    const headers = new Headers((init.headers ?? {}) as Record<string, string>);
    expect(headers.get('user-agent')).toBeNull();
    expect(headers.get('x-msh-platform')).toBeNull();
    expect(write).toHaveBeenCalledWith(config);

    expect(config.providers['custom']).toMatchObject({
      apiKey: 'sk-existing',
    });
    expect(config.models?.['custom-default']?.provider).toBe('custom');
    expect(config.models?.['floyd-code/stale']).toBeUndefined();
    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      type: 'floyd',
      baseUrl,
      apiKey: '',
      oauth: { storage: 'file', key: resolveFloydCodeOAuthKey({ baseUrl }) },
    });
    expect(config.models?.['floyd-code/floyd-for-coding']).toMatchObject({
      provider: FLOYD_CODE_PROVIDER_NAME,
      model: 'floyd-for-coding',
      maxContextSize: 262144,
      capabilities: ['thinking', 'image_in', 'video_in', 'tool_use'],
      displayName: 'Floyd for Coding',
    });
    expect(config.models?.['floyd-code/floyd-k2.5']?.capabilities).toBeUndefined();
    expect(config.services?.legacySearch).toMatchObject({
      baseUrl: `${baseUrl}/search`,
      apiKey: '',
      oauth: { storage: 'file', key: resolveFloydCodeOAuthKey({ baseUrl }) },
    });
    expect(Object.keys(config.services ?? {})).toEqual(['legacySearch', 'legacyFetch']);
  });

  it('writes scoped OAuth refs when provisioning against a non-default environment', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {},
    };
    const oauthKey = resolveFloydCodeOAuthKey({
      oauthHost: 'https://auth.dev.example.test',
      baseUrl: 'https://api.dev.example.test/coding/v1',
    });

    await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      baseUrl: 'https://api.dev.example.test/coding/v1',
      oauthKey,
      oauthHost: 'https://auth.dev.example.test',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      baseUrl: 'https://api.dev.example.test/coding/v1',
      oauth: {
        storage: 'file',
        key: oauthKey,
        oauthHost: 'https://auth.dev.example.test',
      },
    });
    expect(config.services?.legacySearch?.oauth).toEqual({
      storage: 'file',
      key: oauthKey,
      oauthHost: 'https://auth.dev.example.test',
    });
    expect(config.services?.legacyFetch?.oauth).toEqual({
      storage: 'file',
      key: oauthKey,
      oauthHost: 'https://auth.dev.example.test',
    });
  });

  it('persists the empty OAuth host when only the API base URL is scoped', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {},
    };
    const baseUrl = 'https://api.example.test/coding/v1';
    const oauthKey = resolveFloydCodeOAuthKey({ baseUrl });

    await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      baseUrl,
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    // Nothing configured an OAuth host, so none is invented: the ref carries
    // the empty host it was derived from rather than a baked-in deployment.
    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      baseUrl,
      oauth: {
        storage: 'file',
        key: oauthKey,
        oauthHost: '',
      },
    });
  });

  it('preserves an existing valid default model during refresh', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
          baseUrl: 'https://example.test/v1',
        },
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
        },
      },
      defaultModel: 'custom-default',
      thinking: { enabled: false },
      models: {
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
        },
        'floyd-code/stale': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'stale',
          maxContextSize: 1000,
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('custom-default');
    expect(result.defaultThinking).toBe(false);
    expect(config.defaultModel).toBe('custom-default');
    expect(config.thinking?.enabled).toBe(false);
    expect(config.models?.['floyd-code/stale']).toBeUndefined();
    expect(config.models?.['floyd-code/floyd-for-coding']?.displayName).toBe('Floyd for Coding');
  });

  it('infers default_thinking from fresh managed model capabilities', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
        },
      },
      defaultModel: 'floyd-code/floyd-for-coding',
      models: {
        'floyd-code/floyd-for-coding': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'floyd-for-coding',
          maxContextSize: 1000,
          capabilities: [],
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('floyd-code/floyd-for-coding');
    expect(result.defaultThinking).toBe(true);
    expect(config.thinking?.enabled).toBe(true);
  });

  it('preserves explicit default_thinking when preserving a custom default without capabilities', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'custom-default',
      thinking: { enabled: true },
      models: {
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('custom-default');
    expect(result.defaultThinking).toBe(true);
    expect(config.thinking?.enabled).toBe(true);
  });

  it('defaults default_thinking to false when a preserved custom default has no signal', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'custom-default',
      models: {
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('custom-default');
    expect(result.defaultThinking).toBe(false);
    expect(config.thinking?.enabled).toBe(false);
  });

  it('does not infer default_thinking from preserved custom default capabilities', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'custom-default',
      models: {
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
          capabilities: [],
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('custom-default');
    expect(result.defaultThinking).toBe(false);
    expect(config.thinking?.enabled).toBe(false);
  });

  it('keeps default_thinking off even when preserved custom default has thinking capability', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'custom-default',
      models: {
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
          capabilities: ['thinking'],
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('custom-default');
    expect(result.defaultThinking).toBe(false);
    expect(config.thinking?.enabled).toBe(false);
  });

  it('falls back to the first fetched model when the preserved default was removed', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
        },
      },
      defaultModel: 'floyd-code/stale',
      thinking: { enabled: false },
      models: {
        'floyd-code/stale': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'stale',
          maxContextSize: 1000,
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('floyd-code/floyd-for-coding');
    expect(result.defaultThinking).toBe(false);
    expect(config.defaultModel).toBe('floyd-code/floyd-for-coding');
    expect(config.thinking?.enabled).toBe(false);
  });

  it('removes managed provider, models, services, and default model on logout', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
        },
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'floyd-code/floyd-for-coding',
      thinking: { enabled: true },
      models: {
        'floyd-code/floyd-for-coding': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'floyd-for-coding',
          maxContextSize: 262144,
        },
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
        },
      },
      services: {
        legacySearch: { baseUrl: 'https://api.example.test/coding/v1/search' },
        legacyFetch: { baseUrl: 'https://api.example.test/coding/v1/fetch' },
        customService: { baseUrl: 'https://service.example.test' },
      },
      raw: {
        default_model: 'floyd-code/floyd-for-coding',
        providers: {
          [FLOYD_CODE_PROVIDER_NAME]: { type: 'floyd' },
          custom: { type: 'floyd' },
        },
        models: {
          'floyd-code/floyd-for-coding': {
            provider: FLOYD_CODE_PROVIDER_NAME,
            model: 'floyd-for-coding',
          },
          'custom-default': {
            provider: 'custom',
            model: 'custom-model',
          },
        },
        services: {
          legacy_search: { base_url: 'https://api.example.test/coding/v1/search' },
          legacy_fetch: { base_url: 'https://api.example.test/coding/v1/fetch' },
        },
      },
    };

    applyManagedFloydCodeLogoutConfig(config);

    expect(config.defaultModel).toBeUndefined();
    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toBeUndefined();
    expect(config.providers['custom']).toBeDefined();
    expect(config.models?.['floyd-code/floyd-for-coding']).toBeUndefined();
    expect(config.models?.['custom-default']).toBeDefined();
    expect(config.services?.legacySearch).toBeUndefined();
    expect(config.services?.legacyFetch).toBeUndefined();
    expect(config.services?.['customService']).toEqual({
      baseUrl: 'https://service.example.test',
    });
  });

  it('rejects managed models that do not include a positive context_length', async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [{ id: 'floyd-for-coding', supports_reasoning: true }],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    ) as unknown as typeof fetch;

    await expect(
      fetchManagedFloydCodeModels({
        accessToken: 'oauth-access-token',
        fetchImpl,
      }),
    ).rejects.toThrow(/positive context_length/);
  });

  it('surfaces API error messages from model listing failures', async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(JSON.stringify({ error: { message: 'quota exceeded' } }), {
          status: 429,
          headers: { 'Content-Type': 'application/json' },
        }),
    ) as unknown as typeof fetch;

    await expect(
      fetchManagedFloydCodeModels({
        accessToken: 'oauth-access-token',
        fetchImpl,
      }),
    ).rejects.toThrow('quota exceeded');
  });

  it('classifies model listing 401 responses as OAuth unauthorized', async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            error: { message: 'The API Key appears to be invalid or may have expired.' },
          }),
          {
            status: 401,
            headers: { 'Content-Type': 'application/json' },
          },
        ),
    ) as unknown as typeof fetch;

    await expect(
      fetchManagedFloydCodeModels({
        accessToken: 'oauth-access-token',
        fetchImpl,
      }),
    ).rejects.toBeInstanceOf(OAuthUnauthorizedError);
  });

  it('classifies membership-check 402 responses as OAuth unauthorized', async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            error: {
              message:
                "We're unable to verify your membership benefits at this time. Please ensure your membership is active.",
            },
          }),
          {
            status: 402,
            headers: { 'Content-Type': 'application/json' },
          },
        ),
    ) as unknown as typeof fetch;

    const promise = fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      baseUrl: 'https://api.dev.example.test/coding/v1',
      fetchImpl,
    });

    await expect(promise).rejects.toThrow(
      "Floyd Code models endpoint https://api.dev.example.test/coding/v1 rejected OAuth credentials: We're unable to verify your membership benefits at this time. Please ensure your membership is active.",
    );
    await expect(
      fetchManagedFloydCodeModels({
        accessToken: 'oauth-access-token',
        baseUrl: 'https://api.dev.example.test/coding/v1',
        fetchImpl,
      }),
    ).rejects.toMatchObject({
      status: 402,
      baseUrl: 'https://api.dev.example.test/coding/v1',
    });
    await expect(
      fetchManagedFloydCodeModels({
        accessToken: 'oauth-access-token',
        fetchImpl,
      }),
    ).rejects.toBeInstanceOf(OAuthUnauthorizedError);
    await expect(
      fetchManagedFloydCodeModels({
        accessToken: 'oauth-access-token',
        fetchImpl,
      }),
    ).rejects.toBeInstanceOf(ManagedFloydCodeModelsAuthError);
  });

  it('clears managed provider, models, default model, and services on logout', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
          oauth: { storage: 'file', key: 'oauth/floyd-code' },
        },
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'floyd-code/floyd-for-coding',
      models: {
        'floyd-code/floyd-for-coding': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'floyd-for-coding',
          maxContextSize: 262144,
        },
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 128000,
        },
      },
      services: {
        legacySearch: {
          baseUrl: 'https://api.example.test/coding/v1/search',
          apiKey: '',
          oauth: { storage: 'file', key: 'oauth/floyd-code' },
        },
        legacyFetch: {
          baseUrl: 'https://api.example.test/coding/v1/fetch',
          apiKey: '',
          oauth: { storage: 'file', key: 'oauth/floyd-code' },
        },
        otherService: { baseUrl: 'https://service.example.test' },
      },
    };

    const result = clearManagedFloydCodeConfig(config);

    expect(result).toMatchObject({
      providerName: FLOYD_CODE_PROVIDER_NAME,
      removedProvider: true,
      removedModels: ['floyd-code/floyd-for-coding'],
      defaultModelCleared: true,
      removedServices: ['legacySearch', 'legacyFetch'],
    });
    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toBeUndefined();
    expect(config.providers['custom']).toMatchObject({ apiKey: 'sk-existing' });
    expect(config.defaultModel).toBeUndefined();
    expect(config.models?.['floyd-code/floyd-for-coding']).toBeUndefined();
    expect(config.models?.['custom-default']).toMatchObject({ provider: 'custom' });
    expect(config.services?.legacySearch).toBeUndefined();
    expect(config.services?.legacyFetch).toBeUndefined();
    expect(config.services?.['otherService']).toMatchObject({
      baseUrl: 'https://service.example.test',
    });
  });
});

describe('supports_thinking_type', () => {
  function makeThinkingTypeModelsResponse(): Response {
    return new Response(
      JSON.stringify({
        data: [
          {
            id: 'floyd-for-coding',
            context_length: 262144,
            supports_reasoning: true,
            supports_image_in: true,
            supports_video_in: true,
            supports_thinking_type: 'only',
            supports_dynamic_tools: true,
            display_name: 'Floyd For Coding',
          },
          {
            // 'no' is the authoritative declaration and overrides the legacy
            // supports_reasoning boolean.
            id: 'floyd-plain',
            context_length: 128000,
            supports_reasoning: true,
            supports_thinking_type: 'no',
          },
          {
            id: 'floyd-toggle',
            context_length: 128000,
            supports_reasoning: true,
            supports_thinking_type: 'both',
          },
        ],
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    );
  }

  it('parses supports_thinking_type and supports_dynamic_tools from the models endpoint', async () => {
    const models = await fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeThinkingTypeModelsResponse()) as unknown as typeof fetch,
    });

    expect(models[0]?.supportsThinkingType).toBe('only');
    expect(models[1]?.supportsThinkingType).toBe('no');
    expect(models[2]?.supportsThinkingType).toBe('both');
    expect(models[0]?.supportsDynamicTools).toBe(true);
    expect(models[1]?.supportsDynamicTools).toBe(false);
  });

  it('leaves supportsThinkingType undefined when the field is absent or invalid', async () => {
    const absent = await fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeModelsResponse()) as unknown as typeof fetch,
    });
    expect(absent[0]?.supportsThinkingType).toBeUndefined();

    const invalid = await fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              data: [
                {
                  id: 'floyd-for-coding',
                  context_length: 262144,
                  supports_reasoning: true,
                  supports_thinking_type: 'maybe',
                },
              ],
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } },
          ),
      ) as unknown as typeof fetch,
    });
    expect(invalid[0]?.supportsThinkingType).toBeUndefined();
  });

  it('maps the three states onto capabilities, overriding supports_reasoning', async () => {
    const config: ManagedFloydConfigShape = { providers: {} };

    await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeThinkingTypeModelsResponse()) as unknown as typeof fetch,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    // 'only' → thinking locked on; supports_dynamic_tools adds dynamically_loaded_tools.
    expect(config.models?.['floyd-code/floyd-for-coding']?.capabilities).toEqual([
      'thinking',
      'always_thinking',
      'image_in',
      'video_in',
      'tool_use',
      'dynamically_loaded_tools',
    ]);
    // 'no' → no thinking capability despite supports_reasoning=true.
    expect(config.models?.['floyd-code/floyd-plain']?.capabilities).toEqual(['tool_use']);
    // 'both' → plain toggleable thinking.
    expect(config.models?.['floyd-code/floyd-toggle']?.capabilities).toEqual([
      'thinking',
      'tool_use',
    ]);
  });

  it('forces default thinking on when the selected default model is thinking-only', async () => {
    const config: ManagedFloydConfigShape = { providers: {}, thinking: { enabled: false } };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeThinkingTypeModelsResponse()) as unknown as typeof fetch,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('floyd-code/floyd-for-coding');
    expect(result.defaultThinking).toBe(true);
    expect(config.thinking?.enabled).toBe(true);
  });

  it('forces default thinking on when preserving a thinking-only managed default', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
        },
      },
      defaultModel: 'floyd-code/floyd-for-coding',
      thinking: { enabled: false },
      models: {
        'floyd-code/floyd-for-coding': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'floyd-for-coding',
          maxContextSize: 262144,
          capabilities: ['thinking'],
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeThinkingTypeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('floyd-code/floyd-for-coding');
    expect(result.defaultThinking).toBe(true);
    expect(config.thinking?.enabled).toBe(true);
  });

  it('forces default thinking off when preserving a no-thinking managed default', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        [FLOYD_CODE_PROVIDER_NAME]: {
          type: 'floyd',
          apiKey: '',
        },
      },
      defaultModel: 'floyd-code/floyd-plain',
      thinking: { enabled: true },
      models: {
        'floyd-code/floyd-plain': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'floyd-plain',
          maxContextSize: 128000,
          capabilities: ['thinking'],
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeThinkingTypeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('floyd-code/floyd-plain');
    expect(result.defaultThinking).toBe(false);
    expect(config.thinking?.enabled).toBe(false);
  });

  it('keeps a preserved non-managed default thinking selection untouched', async () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        custom: {
          type: 'floyd',
          apiKey: 'sk-existing',
        },
      },
      defaultModel: 'custom-default',
      thinking: { enabled: false },
      models: {
        'custom-default': {
          provider: 'custom',
          model: 'custom-model',
          maxContextSize: 1000,
        },
      },
    };

    const result = await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeThinkingTypeModelsResponse()) as unknown as typeof fetch,
      preserveDefaultModel: true,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    expect(result.defaultModel).toBe('custom-default');
    expect(result.defaultThinking).toBe(false);
    expect(config.thinking?.enabled).toBe(false);
  });
});

describe('support_efforts / default_effort', () => {
  function makeEffortModelsResponse(): Response {
    return new Response(
      JSON.stringify({
        data: [
          {
            id: 'floyd-for-coding',
            context_length: 262144,
            supports_reasoning: true,
            supports_thinking_type: 'both',
            think_efforts: {
              support: true,
              valid_efforts: ['low', 'high', 'max'],
              default_effort: 'high',
            },
            display_name: 'Floyd For Coding',
          },
          {
            // Empty / non-string entries are filtered; absent fields stay undefined.
            id: 'floyd-plain',
            context_length: 128000,
            supports_reasoning: true,
            think_efforts: { support: true, valid_efforts: ['low', '', 42] },
          },
        ],
      }),
      { status: 200, headers: { 'Content-Type': 'application/json' } },
    );
  }

  it('parses think_efforts from the models endpoint', async () => {
    const models = await fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeEffortModelsResponse()) as unknown as typeof fetch,
    });

    expect(models[0]?.supportEfforts).toEqual(['low', 'high', 'max']);
    expect(models[0]?.defaultEffort).toBe('high');
    // The empty string and number are filtered out of valid_efforts.
    expect(models[1]?.supportEfforts).toEqual(['low']);
    expect(models[1]?.defaultEffort).toBeUndefined();
  });

  it('ignores think_efforts entirely when support is not true', async () => {
    const models = await fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      fetchImpl: async () =>
        new Response(
          JSON.stringify({
            data: [
              {
                id: 'floyd-no-effort',
                context_length: 128000,
                supports_reasoning: true,
                think_efforts: {
                  support: false,
                  valid_efforts: ['low', 'high'],
                  default_effort: 'high',
                },
              },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    });

    // support !== true gates the whole object — valid_efforts / default_effort
    // are ignored.
    expect(models[0]?.supportEfforts).toBeUndefined();
    expect(models[0]?.defaultEffort).toBeUndefined();
  });

  it('ignores legacy flat fields even when think_efforts is absent', async () => {
    // The legacy support_efforts / default_effort fields are no longer read;
    // only the nested think_efforts object is honored.
    const models = await fetchManagedFloydCodeModels({
      accessToken: 'oauth-access-token',
      fetchImpl: async () =>
        new Response(
          JSON.stringify({
            data: [
              {
                id: 'floyd-k2',
                context_length: 128000,
                supports_reasoning: true,
                support_efforts: ['low', 'high'],
                default_effort: 'high',
              },
            ],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    });

    expect(models[0]?.supportEfforts).toBeUndefined();
    expect(models[0]?.defaultEffort).toBeUndefined();
  });

  it('writes supportEfforts and defaultEffort onto the provisioned model entry', async () => {
    const config: ManagedFloydConfigShape = { providers: {} };

    await provisionManagedFloydCodeConfig({
      accessToken: 'oauth-access-token',
      fetchImpl: vi.fn(async () => makeEffortModelsResponse()) as unknown as typeof fetch,
      adapter: {
        read: () => config,
        write: vi.fn(),
        apply: applyManagedFloydCodeConfig,
      },
    });

    const alias = config.models?.['floyd-code/floyd-for-coding'];
    expect(alias?.['supportEfforts']).toEqual(['low', 'high', 'max']);
    expect(alias?.['defaultEffort']).toBe('high');
  });
});

describe('selective merge', () => {
  const baseOptions = {
    baseUrl: 'https://api.example.test/coding/v1',
    oauthKey: 'test-key',
  };

  it('preserves non-managed user fields but drops stale managed fields', () => {
    const config: ManagedFloydConfigShape = {
      providers: {},
      models: {
        'floyd-code/floyd-k2': {
          provider: 'floyd-code',
          model: 'floyd-k2',
          maxContextSize: 262144,
          capabilities: ['thinking'],
          maxOutputSize: 4096,
          supportEfforts: ['low', 'high', 'max'],
        } as Record<string, unknown>,
      },
    };

    applyManagedFloydCodeConfig(config, {
      ...baseOptions,
      models: [
        {
          id: 'floyd-k2',
          contextLength: 262144,
          supportsReasoning: true,
          supportsImageIn: false,
          supportsVideoIn: false,
          supportsThinkingType: 'both',
        },
      ],
    });

    const alias = config.models?.['floyd-code/floyd-k2'];
    expect(alias?.['maxOutputSize']).toBe(4096);
    expect(alias?.['supportEfforts']).toBeUndefined();
    expect(alias?.['maxContextSize']).toBe(262144);
  });

  it('preserves overrides when upstream declares managed fields', () => {
    const config: ManagedFloydConfigShape = {
      providers: {},
      models: {
        'floyd-code/floyd-k2': {
          provider: 'floyd-code',
          model: 'floyd-k2',
          maxContextSize: 262144,
          overrides: { supportEfforts: ['low'] },
        } as Record<string, unknown>,
      },
    };

    applyManagedFloydCodeConfig(config, {
      ...baseOptions,
      models: [
        {
          id: 'floyd-k2',
          contextLength: 262144,
          supportsReasoning: true,
          supportsImageIn: false,
          supportsVideoIn: false,
          supportEfforts: ['low', 'high', 'max'],
          defaultEffort: 'high',
        },
      ],
    });

    const alias = config.models?.['floyd-code/floyd-k2'];
    expect(alias?.['supportEfforts']).toEqual(['low', 'high', 'max']);
    expect(alias?.['defaultEffort']).toBe('high');
    expect(alias?.['overrides']).toEqual({ supportEfforts: ['low'] });
  });

  it('removes managed models that upstream no longer lists', () => {
    const config: ManagedFloydConfigShape = {
      providers: {},
      models: {
        'floyd-code/floyd-k2': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'floyd-k2',
          maxContextSize: 262144,
        },
        'floyd-code/removed': {
          provider: FLOYD_CODE_PROVIDER_NAME,
          model: 'removed',
          maxContextSize: 128000,
        },
      },
    };

    applyManagedFloydCodeConfig(config, {
      ...baseOptions,
      models: [
        {
          id: 'floyd-k2',
          contextLength: 262144,
          supportsReasoning: true,
          supportsImageIn: false,
          supportsVideoIn: false,
        },
      ],
    });

    expect(config.models?.['floyd-code/floyd-k2']).toBeDefined();
    expect(config.models?.['floyd-code/removed']).toBeUndefined();
  });
});

describe('applyManagedApiKeyProviderModels', () => {
  it('merges upstream models without touching provider, services, or defaults', () => {
    const config: ManagedFloydConfigShape = {
      providers: {
        'my-floyd': {
          type: 'floyd',
          baseUrl: 'https://api.example.test/coding/v1',
          apiKey: 'sk-distributed-key',
        },
      },
      models: {
        'my-floyd/floyd-k2': {
          provider: 'my-floyd',
          model: 'floyd-k2',
          maxContextSize: 262144,
          displayName: 'Old K2',
          maxOutputSize: 4096,
        } as Record<string, unknown>,
        'my-floyd/floyd-old': {
          provider: 'my-floyd',
          model: 'floyd-old',
          maxContextSize: 128000,
        },
        'other/m1': {
          provider: 'other',
          model: 'm1',
          maxContextSize: 128000,
        },
      },
      defaultModel: 'my-floyd/floyd-k2',
      thinking: { enabled: false },
    };

    applyManagedApiKeyProviderModels(
      config,
      'my-floyd',
      [makeModelInfo('floyd-k2', { displayName: 'Fresh K2' }), makeModelInfo('floyd-k2.5')],
      'my-floyd/',
    );

    // The provider record is user-owned: no rewrite, no oauth, no apiKey reset.
    expect(config.providers['my-floyd']).toEqual({
      type: 'floyd',
      baseUrl: 'https://api.example.test/coding/v1',
      apiKey: 'sk-distributed-key',
    });
    // Defaults and services are the orchestrator's / OAuth branch's business.
    expect(config.defaultModel).toBe('my-floyd/floyd-k2');
    expect(config.thinking).toEqual({ enabled: false });
    expect(config.services).toBeUndefined();
    // Upstream-owned fields merge; hand-written extras survive.
    const alias = config.models?.['my-floyd/floyd-k2'];
    expect(alias?.['displayName']).toBe('Fresh K2');
    expect(alias?.['maxOutputSize']).toBe(4096);
    // New upstream model added; dropped one removed; other providers untouched.
    expect(config.models?.['my-floyd/floyd-k2.5']).toBeDefined();
    expect(config.models?.['my-floyd/floyd-old']).toBeUndefined();
    expect(config.models?.['other/m1']).toBeDefined();
  });

  it('writes protocol routing fields for anthropic-protocol models', () => {
    const config: ManagedFloydConfigShape = { providers: {}, models: {} };

    applyManagedApiKeyProviderModels(
      config,
      'my-floyd',
      [makeModelInfo('floyd-for-coding', { protocol: 'anthropic', supportsReasoning: true })],
      'my-floyd/',
    );

    const alias = config.models?.['my-floyd/floyd-for-coding'];
    expect(alias?.['provider']).toBe('my-floyd');
    expect(alias?.['protocol']).toBe('anthropic');
    expect(alias?.['betaApi']).toBe(true);
    expect(alias?.['adaptiveThinking']).toBe(true);
    expect(alias?.['capabilities']).toEqual(['thinking', 'tool_use']);
  });

  it('rejects models without a positive context length', () => {
    const config: ManagedFloydConfigShape = { providers: {}, models: {} };

    expect(() => {
      applyManagedApiKeyProviderModels(
        config,
        'my-floyd',
        [makeModelInfo('bad', { contextLength: 0 })],
        'my-floyd/',
      );
    }).toThrow('context_length');
  });
});

function makeModelInfo(
  id: string,
  overrides: Partial<ManagedFloydCodeModelInfo> = {},
): ManagedFloydCodeModelInfo {
  return {
    id,
    contextLength: 200000,
    supportsReasoning: false,
    supportsImageIn: false,
    supportsVideoIn: false,
    ...overrides,
  };
}

const FLOYD_BASE_URL = 'https://api.example.test/coding/v1';

describe('managed protocol routing', () => {
  it('reads protocol from the /models response', async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [{ id: 'floyd-for-coding', context_length: 262144, protocol: 'anthropic' }],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    ) as unknown as typeof fetch;

    const models = await fetchManagedFloydCodeModels({ accessToken: 't', fetchImpl });
    expect(models).toHaveLength(1);
    expect(models[0]?.protocol).toBe('anthropic');
  });

  it('maps the server "response" protocol value to openai_responses', async () => {
    const fetchImpl = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: [{ id: 'k3', context_length: 1048576, protocol: 'response' }],
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        ),
    ) as unknown as typeof fetch;

    const models = await fetchManagedFloydCodeModels({ accessToken: 't', fetchImpl });
    expect(models).toHaveLength(1);
    expect(models[0]?.protocol).toBe('openai_responses');
  });

  it('records openai_responses protocol without anthropic routing fields', () => {
    const config: ManagedFloydConfigShape = { providers: {} };
    applyManagedFloydCodeConfig(config, {
      baseUrl: FLOYD_BASE_URL,
      models: [makeModelInfo('k3', { protocol: 'openai_responses', supportsReasoning: true })],
    });

    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      type: 'floyd',
      baseUrl: FLOYD_BASE_URL,
      apiKey: '',
    });
    const alias = config.models?.['floyd-code/k3'];
    expect(alias?.protocol).toBe('openai_responses');
    expect(alias?.betaApi).toBeUndefined();
    expect(alias?.adaptiveThinking).toBeUndefined();
  });

  it('keeps the provider on the floyd REST base and records the model protocol when anthropic', () => {
    const config: ManagedFloydConfigShape = { providers: {} };
    applyManagedFloydCodeConfig(config, {
      baseUrl: FLOYD_BASE_URL,
      models: [makeModelInfo('floyd-for-coding', { protocol: 'anthropic' })],
    });

    // The provider stays on the floyd wire + REST base; the anthropic transport
    // is resolved per-model at runtime, not baked into the provider config, so
    // the REST base keeps flowing to OAuth key derivation and plugin env.
    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      type: 'floyd',
      baseUrl: FLOYD_BASE_URL,
      apiKey: '',
    });
    expect(config.models?.['floyd-code/floyd-for-coding']).toMatchObject({
      provider: FLOYD_CODE_PROVIDER_NAME,
      protocol: 'anthropic',
      betaApi: true,
    });
  });

  it('keeps the floyd protocol and baseUrl when the model has no anthropic protocol', () => {
    const config: ManagedFloydConfigShape = { providers: {} };
    applyManagedFloydCodeConfig(config, {
      baseUrl: FLOYD_BASE_URL,
      models: [makeModelInfo('floyd-for-coding')],
    });

    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      type: 'floyd',
      baseUrl: FLOYD_BASE_URL,
      apiKey: '',
    });
    expect(config.models?.['floyd-code/floyd-for-coding']?.provider).toBe(FLOYD_CODE_PROVIDER_NAME);
    expect(config.models?.['floyd-code/floyd-for-coding']?.protocol).toBeUndefined();
  });

  it('drops the model protocol on refresh when the server stops declaring anthropic', () => {
    const config: ManagedFloydConfigShape = { providers: {} };
    applyManagedFloydCodeConfig(config, {
      baseUrl: FLOYD_BASE_URL,
      models: [makeModelInfo('floyd-for-coding', { protocol: 'anthropic' })],
    });
    expect(config.models?.['floyd-code/floyd-for-coding']?.protocol).toBe('anthropic');

    applyManagedFloydCodeConfig(config, {
      baseUrl: FLOYD_BASE_URL,
      models: [makeModelInfo('floyd-for-coding')],
    });
    // The provider never leaves the floyd wire / REST base across refreshes —
    // only the per-model protocol annotation changes.
    expect(config.providers[FLOYD_CODE_PROVIDER_NAME]).toMatchObject({
      type: 'floyd',
      baseUrl: FLOYD_BASE_URL,
    });
    expect(config.models?.['floyd-code/floyd-for-coding']?.protocol).toBeUndefined();
  });
});
