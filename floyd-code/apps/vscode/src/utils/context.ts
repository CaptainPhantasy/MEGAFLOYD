import * as vscode from "vscode";
import type { FloydHarness } from "@legacy-ai/floyd-code-sdk";

export async function updateLoginContext(harness: FloydHarness): Promise<boolean> {
  const status = await harness.auth.status();
  const loggedIn = status.providers.some((provider) => provider.hasToken);
  await vscode.commands.executeCommand("setContext", "floyd.isLoggedIn", loggedIn);
  return loggedIn;
}
