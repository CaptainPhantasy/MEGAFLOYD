import {
  createFloydDefaultHeaders,
  FLOYD_CODE_FLOW_CONFIG,
  FLOYD_CODE_PROVIDER_NAME,
  FloydOAuthToolkit,
  floydCodeBaseUrl,
  parseFloydCodeCustomHeaders,
  resolveFloydCodeOAuthRef,
  type FloydHostIdentity,
  type ManagedFloydOAuthRef,
} from '@legacy-ai/floyd-code-oauth';
import type {
  ProviderConfig as KosongProviderConfig,
  ProviderRequestAuth,
} from '@legacy-ai/kosong';
import { APIStatusError, UNKNOWN_CAPABILITY } from '@legacy-ai/kosong';
import { resolveFloydHome } from '@legacy-ai/agent-core-v2';

import { ErrorCodes, FloydError } from '#/errors';
import type { Logger } from '#/logging/index';
import type { ModelProvider, ResolvedRuntimeProvider } from '#/model-provider';
import { mapOAuthTokenError } from '#/oauth-error';

export interface FloydForCodingProviderOptions extends FloydHostIdentity {
  readonly homeDir?: string;
  readonly model?: string;
  readonly baseUrl?: string;
  readonly promptCacheKey?: string;
  readonly defaultHeaders?: Record<string, string>;
}

export class FloydForCodingProvider implements ModelProvider {
  private readonly model: string;
  private readonly baseUrl: string;
  private readonly promptCacheKey: string | undefined;
  private readonly defaultHeaders: Record<string, string> | undefined;
  private readonly toolkit: FloydOAuthToolkit;
  private readonly homeDir: string;
  private readonly identity: FloydHostIdentity;
  private readonly oauthRef: ManagedFloydOAuthRef;

  constructor(options: FloydForCodingProviderOptions) {
    this.model = options.model ?? 'floyd-for-coding';
    this.baseUrl = options.baseUrl ?? floydCodeBaseUrl();
    this.promptCacheKey = options.promptCacheKey;
    this.defaultHeaders = options.defaultHeaders;
    this.homeDir = resolveFloydHome(options.homeDir);
    this.identity = {
      productName: options.productName,
      version: options.version,
      platform: options.platform,
      userAgentSuffix: options.userAgentSuffix,
    };
    this.oauthRef = resolveFloydCodeOAuthRef({
      oauthHost: FLOYD_CODE_FLOW_CONFIG.oauthHost,
      baseUrl: this.baseUrl,
    });
    this.toolkit = new FloydOAuthToolkit({
      homeDir: this.homeDir,
      identity: this.identity,
    });
  }

  get defaultModel(): string {
    return this.model;
  }

  resolveProviderConfig(model: string): ResolvedRuntimeProvider {
    if (model !== this.model) {
      throw new FloydError(
        ErrorCodes.CONFIG_INVALID,
        `Model "${model}" is not supported by FloydForCodingProvider.`,
      );
    }

    const provider: KosongProviderConfig = {
      type: 'floyd',
      model: this.model,
      baseUrl: this.baseUrl,
      generationKwargs: this.promptCacheKey
        ? { prompt_cache_key: this.promptCacheKey }
        : undefined,
      defaultHeaders: {
        ...parseFloydCodeCustomHeaders(),
        ...createFloydDefaultHeaders({
          homeDir: this.homeDir,
          ...this.identity,
        }),
        ...this.defaultHeaders,
      },
    };

    return {
      providerName: 'floyd-for-coding',
      provider,
      modelCapabilities: UNKNOWN_CAPABILITY,
      type: 'floyd',
      protocol: undefined,
    };
  }

  resolveAuth(_model: string, _options?: { readonly log?: Logger }) {
    return async <T>(request: (auth: ProviderRequestAuth) => Promise<T>): Promise<T> => {
      let auth = await this.buildAuth(false);
      for (let refreshed = false; ; refreshed = true) {
        try {
          return await request(auth);
        } catch (error) {
          const is401 = error instanceof APIStatusError && error.statusCode === 401;
          if (!is401) throw error;
          if (refreshed) {
            throw new FloydError(
              ErrorCodes.AUTH_LOGIN_REQUIRED,
              'OAuth token was rejected after refresh. Run /login to re-authenticate.',
              { cause: error },
            );
          }
          auth = await this.buildAuth(true);
        }
      }
    };
  }

  private async buildAuth(force: boolean): Promise<ProviderRequestAuth> {
    try {
      const apiKey = await this.toolkit.ensureFresh(FLOYD_CODE_PROVIDER_NAME, {
        force,
        oauthRef: this.oauthRef,
      });
      return { apiKey };
    } catch (error) {
      // Classify OAuth token failures into the public FloydError protocol so the
      // turn surfaces `auth.login_required` / `provider.connection_error`
      // instead of collapsing everything to `internal`. Unrecognized errors are
      // rethrown raw (see mapOAuthTokenError).
      throw mapOAuthTokenError(error, FLOYD_CODE_PROVIDER_NAME) ?? error;
    }
  }
}
