import { ErrorCodes, FloydError } from '#/errors';

import { parseBooleanEnv } from './resolve';
import {
  validateConfig,
  type FloydConfig,
  type ModelAlias,
  type ProviderConfig,
  type ProviderType,
  type ThinkingConfig,
} from './schema';

export const ENV_MODEL_PROVIDER_KEY = '__floyd_env__';
export const ENV_MODEL_ALIAS_KEY = '__floyd_env_model__';

const ALLOWED_TYPES: readonly ProviderType[] = ['floyd', 'anthropic', 'openai'];

const DEFAULT_BASE_URL: Partial<Record<ProviderType, string>> = {
  openai: 'https://api.openai.com/v1',
};

const DEFAULT_MAX_CONTEXT_SIZE = 262144;

const DEFAULT_CAPABILITIES = ['image_in', 'thinking'];

type Env = Readonly<Record<string, string | undefined>>;

function trimmed(value: string | undefined): string | undefined {
  const t = value?.trim();
  return t === undefined || t.length === 0 ? undefined : t;
}

function fail(message: string): never {
  throw new FloydError(ErrorCodes.CONFIG_INVALID, message);
}

function parsePositiveInt(raw: string, varName: string): number {
  if (!/^\d+$/.test(raw) || Number(raw) <= 0) {
    fail(`${varName} must be a positive integer, got "${raw}".`);
  }
  return Number(raw);
}

function parseProviderType(raw: string | undefined): ProviderType {
  if (raw === undefined) return 'floyd';
  const normalized = raw.toLowerCase() as ProviderType;
  if (!ALLOWED_TYPES.includes(normalized)) {
    fail(
      `FLOYD_MODEL_PROVIDER_TYPE must be one of ${ALLOWED_TYPES.join(', ')}, got "${raw}".`,
    );
  }
  return normalized;
}

function resolveBaseUrl(type: ProviderType, explicit: string | undefined): string | undefined {
  if (explicit !== undefined) return explicit;
  const fallback = DEFAULT_BASE_URL[type];
  if (fallback !== undefined) return fallback;
  if (type === 'floyd') {
    fail(
      'FLOYD_MODEL_NAME is set but FLOYD_MODEL_BASE_URL is missing, and provider type "floyd" has no default endpoint. Set FLOYD_MODEL_BASE_URL to your OpenAI-compatible base URL, pick another FLOYD_MODEL_PROVIDER_TYPE (anthropic, openai), or declare the provider under [providers.*] in ~/.floyd-code/config.toml.',
    );
  }
  return undefined;
}

function parseCapabilities(raw: string | undefined): string[] | undefined {
  if (raw === undefined) return undefined;
  const caps = raw
    .split(',')
    .map((c) => c.trim().toLowerCase())
    .filter((c) => c.length > 0);
  return caps.length === 0 ? undefined : caps;
}

function parseBooleanVar(raw: string | undefined, varName: string): boolean | undefined {
  const value = trimmed(raw);
  if (value === undefined) return undefined;
  const parsed = parseBooleanEnv(value);
  if (parsed === undefined) {
    fail(`${varName} must be a boolean (true/false/1/0/yes/no/on/off), got "${raw}".`);
  }
  return parsed;
}

export function applyEnvModelConfig(config: FloydConfig, env: Env = process.env): FloydConfig {
  const model = trimmed(env['FLOYD_MODEL_NAME']);
  if (model === undefined) return config;

  const apiKey = trimmed(env['FLOYD_MODEL_API_KEY']);
  if (apiKey === undefined) {
    fail('FLOYD_MODEL_NAME is set but FLOYD_MODEL_API_KEY is missing.');
  }

  const maxContextRaw = trimmed(env['FLOYD_MODEL_MAX_CONTEXT_SIZE']);
  const maxContextSize =
    maxContextRaw === undefined
      ? DEFAULT_MAX_CONTEXT_SIZE
      : parsePositiveInt(maxContextRaw, 'FLOYD_MODEL_MAX_CONTEXT_SIZE');

  const type = parseProviderType(trimmed(env['FLOYD_MODEL_PROVIDER_TYPE']));
  const baseUrl = resolveBaseUrl(type, trimmed(env['FLOYD_MODEL_BASE_URL']));

  const provider: ProviderConfig = {
    type,
    apiKey,
    ...(baseUrl !== undefined ? { baseUrl } : {}),
  };

  const maxOutputRaw = trimmed(env['FLOYD_MODEL_MAX_OUTPUT_SIZE']);
  const maxOutputSize =
    maxOutputRaw !== undefined
      ? parsePositiveInt(maxOutputRaw, 'FLOYD_MODEL_MAX_OUTPUT_SIZE')
      : undefined;
  const capabilities = parseCapabilities(env['FLOYD_MODEL_CAPABILITIES']) ?? DEFAULT_CAPABILITIES;
  const displayName = trimmed(env['FLOYD_MODEL_DISPLAY_NAME']);
  const reasoningKey = trimmed(env['FLOYD_MODEL_REASONING_KEY']);
  const adaptiveThinking = parseBooleanVar(
    env['FLOYD_MODEL_ADAPTIVE_THINKING'],
    'FLOYD_MODEL_ADAPTIVE_THINKING',
  );

  const alias: ModelAlias = {
    provider: ENV_MODEL_PROVIDER_KEY,
    model,
    maxContextSize,
    capabilities,
    ...(displayName !== undefined ? { displayName } : {}),
    ...(maxOutputSize !== undefined ? { maxOutputSize } : {}),
    ...(reasoningKey !== undefined ? { reasoningKey } : {}),
    ...(adaptiveThinking !== undefined ? { adaptiveThinking } : {}),
  };

  const thinkingEffort = trimmed(env['FLOYD_MODEL_THINKING_EFFORT']);
  const thinking: ThinkingConfig | undefined =
    thinkingEffort !== undefined ? { ...config.thinking, effort: thinkingEffort } : config.thinking;

  const merged: FloydConfig = {
    ...config,
    providers: { ...config.providers, [ENV_MODEL_PROVIDER_KEY]: provider },
    models: { ...config.models, [ENV_MODEL_ALIAS_KEY]: alias },
    defaultModel: ENV_MODEL_ALIAS_KEY,
    ...(thinking !== undefined ? { thinking } : {}),
  };

  return validateConfig(merged);
}

export function stripEnvModelConfig(config: FloydConfig): FloydConfig {
  const hasProvider = ENV_MODEL_PROVIDER_KEY in config.providers;
  const hasModel = config.models !== undefined && ENV_MODEL_ALIAS_KEY in config.models;
  const defaultIsEnv = config.defaultModel === ENV_MODEL_ALIAS_KEY;
  if (!hasProvider && !hasModel && !defaultIsEnv) return config;

  const providers = { ...config.providers };
  delete providers[ENV_MODEL_PROVIDER_KEY];

  let models = config.models;
  if (models !== undefined && ENV_MODEL_ALIAS_KEY in models) {
    models = { ...models };
    delete models[ENV_MODEL_ALIAS_KEY];
  }

  return {
    ...config,
    providers,
    ...(models !== undefined ? { models } : {}),
    ...(defaultIsEnv ? { defaultModel: rawDefaultModel(config) } : {}),
    thinking: rawThinking(config),
  };
}

function rawDefaultModel(config: FloydConfig): string | undefined {
  const raw = config.raw?.['default_model'];
  return typeof raw === 'string' ? raw : undefined;
}

function rawThinking(config: FloydConfig): ThinkingConfig | undefined {
  const raw = config.raw?.['thinking'];
  return typeof raw === 'object' && raw !== null && !Array.isArray(raw)
    ? (raw as ThinkingConfig)
    : undefined;
}
