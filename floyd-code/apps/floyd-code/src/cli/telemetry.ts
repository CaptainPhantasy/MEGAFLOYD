import { createFloydDeviceId, FLOYD_CODE_PROVIDER_NAME } from '@legacy-ai/floyd-code-oauth';
import {
  FloydAuthFacade,
  loadRuntimeConfigSafe,
  log,
  resolveConfigPath,
  resolveFloydHome,
  type FloydConfig,
  type TelemetryClient,
} from '@legacy-ai/floyd-code-sdk';

import type { PromptHarness } from './prompt-session';
import {
  initializeTelemetry,
  setTelemetryContext,
  track,
  withTelemetryContext,
} from '@legacy-ai/floyd-telemetry';

import { CLI_USER_AGENT_PRODUCT, WEB_UI_MODE } from '#/constant/app';
import { currentFloydProfile } from '#/utils/region';

import { createFloydCodeHostIdentity } from './version';

export interface CliTelemetryBootstrap {
  readonly homeDir: string;
  readonly deviceId: string;
  readonly firstLaunch: boolean;
}

export interface InitializeCliTelemetryOptions {
  readonly harness: PromptHarness;
  readonly bootstrap: CliTelemetryBootstrap;
  readonly config: Pick<FloydConfig, 'defaultModel' | 'telemetry'>;
  readonly version: string;
  readonly uiMode: string;
  readonly model?: string;
  readonly sessionId?: string;
}

export function createCliTelemetryBootstrap(): CliTelemetryBootstrap {
  let firstLaunch = false;
  const homeDir = resolveFloydHome();
  const deviceId = createFloydDeviceId(homeDir, {
    onFirstLaunch: () => {
      firstLaunch = true;
    },
  });
  return { homeDir, deviceId, firstLaunch };
}

export function initializeCliTelemetry(options: InitializeCliTelemetryOptions): void {
  initializeTelemetry({
    homeDir: options.harness.homeDir,
    deviceId: options.bootstrap.deviceId,
    enabled: options.config.telemetry !== false,
    appName: CLI_USER_AGENT_PRODUCT,
    version: options.version,
    uiMode: options.uiMode,
    model: options.model ?? options.config.defaultModel,
    sessionId: options.sessionId,
    endpoint: () => currentFloydProfile().telemetryEndpoint,
    getAccessToken: async () =>
      (await options.harness.auth.getCachedAccessToken(FLOYD_CODE_PROVIDER_NAME)) ?? null,
    onUnexpectedError: (error) => log.warn('telemetry property dropped', { error: String(error) }),
  });
  if (options.bootstrap.firstLaunch) {
    options.harness.track('first_launch');
  }
}

export interface InitializeServerTelemetryOptions {
  readonly version: string;
}

/**
 * Bootstrap telemetry for the `floyd web` host.
 *
 * Mirrors {@link initializeCliTelemetry}: mints the device id, reads config to
 * honor the `telemetry` toggle and pick up the default model, and attaches the
 * sink with `ui_mode = "web"`.
 *
 * The returned client wraps the `@legacy-ai/floyd-telemetry` module
 * functions, so the module-level `track` / `withTelemetryContext` (used to
 * fire the startup event) share the same underlying client + sink.
 */
export function initializeServerTelemetry(
  options: InitializeServerTelemetryOptions,
): TelemetryClient {
  const bootstrap = createCliTelemetryBootstrap();
  const configPath = resolveConfigPath({ homeDir: bootstrap.homeDir });
  const config = readServerTelemetryConfig(configPath);
  const auth = new FloydAuthFacade({
    homeDir: bootstrap.homeDir,
    configPath,
    identity: createFloydCodeHostIdentity(options.version),
  });

  initializeTelemetry({
    homeDir: bootstrap.homeDir,
    deviceId: bootstrap.deviceId,
    enabled: config.telemetry !== false,
    appName: CLI_USER_AGENT_PRODUCT,
    version: options.version,
    uiMode: WEB_UI_MODE,
    model: config.defaultModel,
    endpoint: () => currentFloydProfile().telemetryEndpoint,
    getAccessToken: async () => (await auth.getCachedAccessToken(FLOYD_CODE_PROVIDER_NAME)) ?? null,
    onUnexpectedError: (error) => log.warn('telemetry property dropped', { error: String(error) }),
  });

  return {
    track,
    withContext: withTelemetryContext,
    setContext: setTelemetryContext,
  };
}

function readServerTelemetryConfig(
  configPath: string,
): Pick<FloydConfig, 'telemetry' | 'defaultModel'> {
  try {
    const { config, fileError } = loadRuntimeConfigSafe(configPath);
    // A broken config fails the server on its own inside FloydCore; for
    // telemetry just degrade to "enabled, no model" so we never block startup.
    if (fileError !== undefined) return {};
    return config;
  } catch {
    return {};
  }
}
