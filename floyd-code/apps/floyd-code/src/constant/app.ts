import { ErrorCodes, type HostUiCapability } from '@legacy-ai/floyd-code-sdk';

export const PRODUCT_NAME = 'Floyd Code';
export const CLI_COMMAND_NAME = 'f7';
export const PROCESS_NAME = 'floyd-code';

// Used in telemetry app names and HTTP User-Agent headers.
export const CLI_USER_AGENT_PRODUCT = 'floyd-code-cli';
export const CLI_UI_MODE = 'shell';
// UI surfaces the TUI renders; declared to the engine at bootstrap so features that need a
// host-side surface (the NotifyUser update panel) are offered to this process only.
export const TUI_HOST_UI_CAPABILITIES: readonly HostUiCapability[] = ['update_panel'];
// Telemetry ui_mode for the `floyd web` host. Same product
// as the CLI (CLI_USER_AGENT_PRODUCT); the surface is distinguished by ui_mode.
export const WEB_UI_MODE = 'web';
// User-Agent suffix for the `floyd web` host: its requests go out as
// `floyd-code-cli/<version> (web)` so upstream can tell web-UI traffic
// apart from direct CLI runs without changing the product token or platform.
export const WEB_USER_AGENT_SUFFIX = 'web';

// Give telemetry a short flush window without making CLI exit feel stuck.
export const CLI_SHUTDOWN_TIMEOUT_MS = 3000;

// Upper bound on headless (`floyd -p`) shutdown. A wedged cleanup step (e.g. a
// SessionEnd hook, an MCP shutdown, or a connection blackholed by a restrictive
// firewall) must not keep a completed run alive indefinitely — once this elapses
// we stop waiting on cleanup and let the run return.
export const PROMPT_CLEANUP_TIMEOUT_MS = 8000;

// Grace after a headless run has fully completed (turn done, cleanup attempted)
// before force-exiting. `floyd -p` otherwise relies on the event loop draining to
// exit; a stray ref'd handle (socket/timer/child) left over from the run would
// wedge it. The guard timer is unref'd, so a healthy run still exits naturally
// well before this fires.
export const HEADLESS_FORCE_EXIT_GRACE_MS = 2000;

// Max time to wait for buffered stdout/stderr to flush before arming the
// force-exit fallback. A slow/piped consumer's still-draining stdio is a
// legitimate ref'd handle — flushing first prevents the fallback from
// truncating completed output. Bounded so a permanently-stuck consumer can't
// re-introduce the hang.
export const HEADLESS_STDIO_DRAIN_TIMEOUT_MS = 10000;

// Published npm package name; this can differ from the executable command.
export const NPM_PACKAGE_NAME = '@legacy-ai/floyd-code';

// App-owned data paths. SDK/core runtime config is intentionally not routed here.
export const FLOYD_CODE_HOME_ENV = 'FLOYD_CODE_HOME';
export const FLOYD_CODE_DATA_DIR_NAME = '.floyd-code';
export const FLOYD_CODE_LOG_DIR_NAME = 'logs';
export const FLOYD_CODE_CACHE_DIR_NAME = 'cache';
export const FLOYD_CODE_UPDATE_DIR_NAME = 'updates';
export const FLOYD_CODE_BIN_DIR_NAME = 'bin';
export const FLOYD_CODE_UPDATE_STATE_FILE_NAME = 'latest.json';
export const FLOYD_CODE_UPDATE_INSTALL_STATE_FILE_NAME = 'install.json';
export const FLOYD_CODE_UPDATE_INSTALL_LOCK_FILE_NAME = 'install.lock';
export const FLOYD_CODE_UPDATE_ROLLOUT_LOG_FILE_NAME = 'rollout.log';
export const FLOYD_CODE_PLUGIN_UPDATE_NOTICE_STATE_FILE_NAME = 'plugin-notices.json';
// Native staged update: the staged binary + metadata live next to the running
// executable (`<exe dir>/.staging/`); the re-exec guard env breaks the
// swap → re-exec → swap loop.
export const FLOYD_CODE_NATIVE_STAGING_DIR_NAME = '.staging';
export const FLOYD_CODE_NATIVE_STAGED_STATE_FILE_NAME = 'staged.json';
export const FLOYD_CODE_UPDATE_REEXEC_ENV = 'FLOYD_CODE_UPDATE_REEXEC';
export const FLOYD_CODE_INPUT_HISTORY_DIR_NAME = 'user-history';
export const FLOYD_CODE_BANNER_DIR_NAME = 'banner';
export const FLOYD_CODE_BANNER_STATE_FILE_NAME = 'state.json';
export const FLOYD_CODE_SURVEY_STATE_FILE_NAME = 'feedback-survey-state.json';
export const FLOYD_CODE_RECOMMENDED_EFFORT_STATE_FILE_NAME = 'recommended-effort-state.json';

// Managed Floyd auth provider key shared with OAuth/SDK config.
export const DEFAULT_OAUTH_PROVIDER_NAME = 'managed:floyd-code';

// SDK/core error code that tells the TUI to show a login-required startup
// notice. Derived from sdk's ErrorCodes so a future rename in core
// auto-propagates instead of silently breaking the startup recovery path.
export const OAUTH_LOGIN_REQUIRED_CODE = ErrorCodes.AUTH_LOGIN_REQUIRED;

const FLOYD_CODE_REPOSITORY_URL = 'https://github.com/CaptainPhantasy/f7';

export const FEEDBACK_ISSUE_URL = `${FLOYD_CODE_REPOSITORY_URL}/issues`;
// Entry page offered to signed-out users. This build has no hosted account
// console, so it points at the project repository.
export function floydCodeSignupUrl(): string {
  const base = (process.env.FLOYD_CODE_BASE_URL ?? '').replace(/\/+$/, '');
  return base.length === 0 ? FLOYD_CODE_REPOSITORY_URL : `${base}/account`;
}

// Sent in the feedback `version` field so the backend can distinguish this
// TypeScript client from clients that send a bare version.
export const FEEDBACK_VERSION_PREFIX = 'floyd-code-';

// Telemetry event name; keep stable for dashboard queries.
export const FEEDBACK_TELEMETRY_EVENT = 'feedback_submitted';

export const FLOYD_CODE_CDN_BASE_ENV = 'FLOYD_CODE_CDN_BASE';
// No vendor CDN ships with this build, so the base is empty unless an operator
// configures one: no version check, install script, or binary download can
// reach a host the project does not control.
export function floydCodeCdnBase(): string {
  return (process.env[FLOYD_CODE_CDN_BASE_ENV] ?? '').replace(/\/+$/, '');
}
export function floydCodeCdnLatestUrl(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? '' : `${base}/latest`;
}
// Rollout manifest consumed by update checks; the plain-text `/latest` above
// stays unchanged forever — already-shipped clients hard-fail on non-semver
// bodies.
export function floydCodeCdnLatestJsonUrl(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? '' : `${base}/latest.json`;
}
// Per-release native artifacts: `/binaries/<version>/manifest.json` +
// `/binaries/<version>/floyd-code-<target>[.exe]` — the bare platform binary.
export function floydCodeCdnBinariesBase(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? '' : `${base}/binaries`;
}
// The marketplace env override name lives in the shared agent-core-v2 plugin
// domain (kap-server consumes it from there). Deep-path import: this module is
// evaluated on every CLI invocation, so it must not pull in the engine root.
export { FLOYD_CODE_PLUGIN_MARKETPLACE_URL_ENV } from '@legacy-ai/agent-core-v2/app/plugin/marketplace';
// The env override above takes priority at the call site; with none set the
// catalog URL is empty and the marketplace falls back to built-in entries.
export function floydCodePluginMarketplaceUrl(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? '' : `${base}/plugins/marketplace.json`;
}
// Bound on each background "latest release" lookup when the TUI fills in
// marketplace versions. Without it a stalled connection to github.com hangs
// the version phase for undici's default header timeout (300s).
export const MARKETPLACE_VERSION_LOOKUP_TIMEOUT_MS = 5000;
export const INTERACTIVE_UPDATE_CHECK_TIMEOUT_MS = 10_000;
// Official plugins whose usage bills against the user's plan quota. Installing
// one of these shows a quota note after the install result.
export const QUOTA_CONSUMING_PLUGIN_IDS: readonly string[] = ['floyd-datasource'];
export function floydCodeInstallShUrl(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? '' : `${base}/install.sh`;
}
export function floydCodeInstallPs1Url(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? '' : `${base}/install.ps1`;
}
// Official download page, referenced by prompt copy that steers users away
// from third-party install sources.
export function floydCodeOfficialInstallUrl(): string {
  const base = floydCodeCdnBase();
  return base.length === 0 ? FLOYD_CODE_REPOSITORY_URL : base;
}

// Install commands, split by platform. Use these for prompt copy and spawn calls only; do not assemble the strings elsewhere.
export function nativeInstallCommandUnix(): string {
  return `npm install -g ${NPM_PACKAGE_NAME}`;
}
export function nativeInstallCommandWin(): string {
  return `npm install -g ${NPM_PACKAGE_NAME}`;
}
