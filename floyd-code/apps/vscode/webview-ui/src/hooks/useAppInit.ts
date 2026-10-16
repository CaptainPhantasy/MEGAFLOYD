import { useState, useEffect, useCallback } from "react";
import { bridge, Events } from "@/services";
import { requiresManagedProviderLogin, useSettingsStore } from "@/stores";
import type { ExtensionConfig } from "shared/types";

export type AppStatus =
  | "loading"
  | "no-workspace"
  | "runtime-error"
  | "no-provider"
  | "managed-provider-unconfigured"
  | "no-models"
  | "ready";

export type ConfigErrorStatus =
  | "loading"
  | "no-workspace"
  | "runtime-error"
  | "managed-provider-unconfigured"
  | "no-models";

export type AppViewResolution =
  | { readonly view: "login" }
  | {
      readonly view: "status";
      readonly status: ConfigErrorStatus;
      /** True when the status screen must offer a path to the provider-setup screen. */
      readonly canGoToLogin: boolean;
    }
  | { readonly view: "main" };

/**
 * Pure view router for App. `no-provider` (nothing configured yet) owns the
 * provider-setup screen; `no-models` is that setup skipped or a config.toml
 * without models, and keeps a path back to it, because Reload alone cannot
 * change the on-disk state and the user would otherwise be stranded.
 * `managed-provider-unconfigured` is routed to the status screen instead: its
 * fix is a config.toml edit, not a screen this build can complete, so that
 * screen must carry the instructions.
 */
export function resolveAppView(input: {
  readonly status: AppStatus;
  readonly modelsCount: number;
  readonly skippedLogin: boolean;
  readonly showLogin: boolean;
}): AppViewResolution {
  const { status, modelsCount, skippedLogin, showLogin } = input;
  if (showLogin || (status === "no-provider" && !skippedLogin)) {
    return { view: "login" };
  }
  if (skippedLogin && modelsCount === 0) {
    return { view: "status", status: "no-models", canGoToLogin: true };
  }
  if (status !== "ready" && status !== "no-provider") {
    return {
      view: "status",
      status,
      canGoToLogin: status === "no-models" || status === "managed-provider-unconfigured",
    };
  }
  return { view: "main" };
}

export interface AppInitState {
  status: AppStatus;
  errorMessage: string | null;
  modelsCount: number;
  refresh: () => void;
}

export function useAppInit(): AppInitState {
  const [state, setState] = useState<Omit<AppInitState, "refresh">>({
    status: "loading",
    errorMessage: null,
    modelsCount: 0,
  });
  const [initKey, setInitKey] = useState(0);
  const { initModels, setExtensionConfig, setWireSlashCommands, setWorkspaceRoot } = useSettingsStore();

  const refresh = useCallback(() => {
    setState({ status: "loading", errorMessage: null, modelsCount: 0 });
    setInitKey((k) => k + 1);
  }, []);

  useEffect(() => {
    return bridge.on<{ config: ExtensionConfig; changedKeys: string[] }>(Events.ExtensionConfigChanged, ({ config }) => {
      setExtensionConfig(config);
    });
  }, [setExtensionConfig, refresh]);

  useEffect(() => {
    let cancelled = false;

    async function init() {
      try {
        const workspace = await bridge.checkWorkspace();
        if (cancelled) {
          return;
        }

        if (!workspace.hasWorkspace) {
          setState({ status: "no-workspace", errorMessage: null, modelsCount: 0 });
          return;
        }

        setWorkspaceRoot(workspace.workspaceRoot ?? workspace.path ?? null);

        const [extensionConfig, slashCommands] = await Promise.all([
          bridge.getExtensionConfig(),
          bridge.getSlashCommands(),
        ]);
        if (cancelled) {
          return;
        }

        setExtensionConfig(extensionConfig);
        setWireSlashCommands(slashCommands);

        const [loginStatus, floydConfig] = await Promise.all([bridge.checkLoginStatus(), bridge.getModels()]);
        if (cancelled) {
          return;
        }

        console.log("[AppInit] Login status:", loginStatus, "floydConfig:", floydConfig);

        initModels(floydConfig.models, floydConfig.defaultModel, floydConfig.defaultThinking, floydConfig.defaultThinkingEffort);

        const modelsCount = floydConfig.models?.length ?? 0;

        if (modelsCount === 0) {
          setState({ status: "no-provider", errorMessage: null, modelsCount: 0 });
          return;
        }

        if (requiresManagedProviderLogin(floydConfig.models, floydConfig.defaultModel, loginStatus.loggedIn)) {
          setState({ status: "managed-provider-unconfigured", errorMessage: null, modelsCount });
          return;
        }

        setState({ status: "ready", errorMessage: null, modelsCount });
      } catch (error) {
        if (!cancelled) {
          setState({
            status: "runtime-error",
            errorMessage: error instanceof Error ? error.message : "Failed to initialize",
            modelsCount: 0,
          });
        }
      }
    }

    void init();
    return () => {
      cancelled = true;
    };
  }, [initKey, initModels, setExtensionConfig, setWireSlashCommands]);

  return { ...state, refresh };
}
