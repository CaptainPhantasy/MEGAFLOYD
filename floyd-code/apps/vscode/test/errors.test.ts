/**
 * Scenario: Webview-facing error copy on a build that has no sign-in.
 * Responsibilities: every mapped code must name something the user can actually
 * do — add a provider and model to the shared Floyd Code config.toml, check a
 * provider's API key, or set the OAuth host knob — and must never send them to a
 * sign-in, account, or subscription this build has no server for. Error codes
 * keep their phase, so severity routing is unchanged.
 * Wiring: pure module — no bridge or Webview involved.
 * Run: pnpm exec vitest run --config apps/vscode/vitest.config.ts test/errors.test.ts
 */
import { describe, expect, it } from "vitest";

import { ERROR_MESSAGES, classifyError, getUserMessage } from "../shared/errors";

const UNSUPPORTED_COPY = /sign[ -]?in|signed in|log[ -]?in|account|subscription/i;

describe("ERROR_MESSAGES", () => {
  it("never points at a sign-in, account, or subscription", () => {
    const offenders = Object.entries(ERROR_MESSAGES)
      .filter(([, message]) => UNSUPPORTED_COPY.test(message))
      .map(([code]) => code);
    expect(offenders).toEqual([]);
  });

  it("names the config.toml fix for the no-model codes", () => {
    for (const code of ["LLM_NOT_SET", "model.not_configured"]) {
      expect(getUserMessage(code)).toBe(
        "No model is configured. Add a provider and model to your shared Floyd Code config.toml.",
      );
    }
  });

  it("names the OAuth host knob for auth.login_required", () => {
    expect(getUserMessage("auth.login_required")).toBe(
      "OAuth provider credentials were rejected. Check FLOYD_CODE_OAUTH_HOST, or add your own provider to config.toml.",
    );
  });

  it("points provider.auth_error at the provider's own credentials", () => {
    expect(getUserMessage("provider.auth_error")).toBe(
      "The provider rejected the credentials. Check its api_key in config.toml, or set FLOYD_CODE_OAUTH_HOST for an OAuth provider.",
    );
  });

  it("keeps the fallback chain and error phases intact", () => {
    expect(getUserMessage("unknown.code")).toBe("An unknown error occurred.");
    expect(getUserMessage("unknown.code", "raw message")).toBe("raw message");
    expect(classifyError("auth.login_required")).toBe("preflight");
    expect(classifyError("model.not_configured")).toBe("preflight");
    expect(classifyError("provider.auth_error")).toBe("runtime");
  });
});
