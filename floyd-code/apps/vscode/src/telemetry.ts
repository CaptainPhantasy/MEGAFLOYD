import * as vscode from "vscode";

import {
  createFloydDeviceId,
  FLOYD_CODE_PROVIDER_NAME,
  FLOYD_REGION_PROFILES,
  resolveFloydRegion,
} from "@legacy-ai/floyd-code-oauth";
import {
  FloydAuthFacade,
  loadRuntimeConfigSafe,
  resolveConfigPath,
  resolveFloydHome,
  type FloydConfig,
} from "@legacy-ai/floyd-code-sdk";
import {
  initializeTelemetry,
  setTelemetryEnabled,
  shouldEnableTelemetry,
  shutdownTelemetry,
  track,
} from "@legacy-ai/floyd-telemetry";

const SHUTDOWN_TIMEOUT_MS = 2000;

export interface ExtensionTelemetryOptions {
  readonly version: string;
  readonly log: (message: string) => void;
}

export function activateExtensionTelemetry(options: ExtensionTelemetryOptions): vscode.Disposable {
  const homeDir = resolveFloydHome();
  let firstLaunch = false;
  const deviceId = createFloydDeviceId(homeDir, {
    onFirstLaunch: () => {
      firstLaunch = true;
    },
  });
  const configPath = resolveConfigPath({ homeDir });
  const config = readTelemetryConfig(configPath);
  const auth = new FloydAuthFacade({ homeDir, configPath });

  initializeTelemetry({
    homeDir,
    deviceId,
    enabled: config.telemetry !== false,
    initiallyEnabled: vscode.env.isTelemetryEnabled,
    appName: "floyd-code-vscode",
    version: options.version,
    uiMode: "vscode",
    model: config.defaultModel,
    endpoint: () => telemetryEndpoint(homeDir),
    getAccessToken: async () => (await auth.getCachedAccessToken(FLOYD_CODE_PROVIDER_NAME)) ?? null,
    onUnexpectedError: (error) => options.log(`Telemetry dropped a property: ${error.message}`),
  });

  const staticallyEnabled = shouldEnableTelemetry({ enabled: config.telemetry !== false });
  if (firstLaunch) track("first_launch");

  return vscode.env.onDidChangeTelemetryEnabled((enabled) => {
    if (staticallyEnabled) setTelemetryEnabled(enabled);
  });
}

export async function deactivateExtensionTelemetry(): Promise<void> {
  await shutdownTelemetry({ timeoutMs: SHUTDOWN_TIMEOUT_MS });
}

function readTelemetryConfig(
  configPath: string,
): Pick<FloydConfig, "telemetry" | "defaultModel"> {
  try {
    const { config, fileError } = loadRuntimeConfigSafe(configPath);
    if (fileError !== undefined) return {};
    return config;
  } catch {
    return {};
  }
}

function telemetryEndpoint(homeDir: string): string {
  const oauth = loadRuntimeConfigSafe(resolveConfigPath({ homeDir })).config.providers?.[
    FLOYD_CODE_PROVIDER_NAME
  ]?.oauth;
  const region = resolveFloydRegion({
    configuredOAuthHost: oauth?.oauthHost,
    configuredOAuthKey: oauth?.key,
    homeDir,
    readMarker: process.env["FLOYD_CODE_REGION_MARKER"] !== "off",
  });
  return FLOYD_REGION_PROFILES[region].telemetryEndpoint;
}
