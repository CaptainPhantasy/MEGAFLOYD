/**
 * Scenario: App-level view routing after init.
 * Responsibilities: the provider-setup screen must stay reachable while nothing
 * is configured — including the setup-skipped state, where Reload alone can
 * never change the on-disk state and the user would otherwise be stranded — and
 * a `managed:floyd-code` model this build cannot authenticate must land on the
 * status screen carrying the fix instead of on a sign-in screen that no longer
 * exists.
 * Wiring: resolveAppView is pure; the bridge and toast boundaries are mocked away.
 * Run: pnpm exec vitest run --config apps/vscode/vitest.config.ts test/app-init.test.ts
 */
import { describe, expect, it, vi } from "vitest";

vi.mock("@/services", () => ({
  bridge: {},
  Events: {},
}));
vi.mock("@/components/ui/sonner", () => ({
  toast: { error: vi.fn(), warning: vi.fn() },
}));

import { resolveAppView, type AppStatus } from "../webview-ui/src/hooks/useAppInit";

function resolve(
  status: AppStatus,
  options: { modelsCount?: number; skippedLogin?: boolean; showLogin?: boolean } = {},
) {
  return resolveAppView({
    status,
    modelsCount: options.modelsCount ?? 0,
    skippedLogin: options.skippedLogin ?? false,
    showLogin: options.showLogin ?? false,
  });
}

describe("resolveAppView", () => {
  it("routes a brand-new user (nothing configured) to the provider-setup screen", () => {
    expect(resolve("no-provider")).toEqual({ view: "login" });
  });

  it("routes a skipped setup without models to no-models with a setup path", () => {
    expect(resolve("no-provider", { skippedLogin: true })).toEqual({
      view: "status",
      status: "no-models",
      canGoToLogin: true,
    });
  });

  it("keeps a setup path in the no-models state", () => {
    // Reload alone cannot change the on-disk state, so the provider-setup
    // screen must stay one click away from here.
    expect(resolve("no-models")).toEqual({
      view: "status",
      status: "no-models",
      canGoToLogin: true,
    });
    expect(resolve("no-models", { skippedLogin: true })).toEqual({
      view: "status",
      status: "no-models",
      canGoToLogin: true,
    });
  });

  it("routes a managed provider this build cannot authenticate to its own status screen", () => {
    // Regression: this state used to land on the sign-in screen, which no
    // longer offers any sign-in and cannot resolve a managed provider.
    expect(resolve("managed-provider-unconfigured", { modelsCount: 3 })).toEqual({
      view: "status",
      status: "managed-provider-unconfigured",
      canGoToLogin: true,
    });
  });

  it("keeps the managed status screen after skipping back out of setup", () => {
    expect(resolve("managed-provider-unconfigured", { modelsCount: 3, skippedLogin: true })).toEqual({
      view: "status",
      status: "managed-provider-unconfigured",
      canGoToLogin: true,
    });
  });

  it("routes to the setup screen when the user asks for it from any state", () => {
    expect(resolve("no-models", { showLogin: true })).toEqual({ view: "login" });
    expect(resolve("ready", { showLogin: true, modelsCount: 1 })).toEqual({ view: "login" });
    expect(resolve("managed-provider-unconfigured", { showLogin: true, modelsCount: 3 })).toEqual({
      view: "login",
    });
  });

  it("routes a skipped setup with models to the main view", () => {
    expect(resolve("ready", { skippedLogin: true, modelsCount: 2 })).toEqual({
      view: "main",
    });
  });

  it("routes ready to the main view", () => {
    expect(resolve("ready", { modelsCount: 1 })).toEqual({ view: "main" });
  });

  it("routes non-setup error statuses to status screens without a setup path", () => {
    for (const status of ["loading", "no-workspace", "runtime-error"] as const) {
      expect(resolve(status)).toEqual({ view: "status", status, canGoToLogin: false });
    }
  });
});
