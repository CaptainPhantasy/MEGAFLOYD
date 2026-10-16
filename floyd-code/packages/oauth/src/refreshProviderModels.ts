import {
  applyCustomRegistryProvider,
  fetchCustomRegistry,
  isReservedProviderId,
  oauthManagedProviderMessage,
  readCustomRegistrySource,
  removeCustomRegistryProvider,
  reservedProviderIdMessage,
  type CustomRegistrySource,
} from './custom-registry';
import {
  applyManagedApiKeyProviderModels,
  applyManagedFloydCodeConfig,
  fetchManagedFloydCodeModels,
  FLOYD_CODE_PLATFORM_ID,
  FLOYD_CODE_PROVIDER_NAME,
  resolveFloydCodeRuntimeAuth,
  type ManagedFloydConfigShape,
  type ManagedFloydModelAlias,
  type ManagedFloydOAuthRef,
} from './managed-floyd-code';
import { isManagedFloydCodeBaseUrl } from './managed-usage';
import {
  applyOpenPlatformConfig,
  fetchOpenPlatformModels,
  filterModelsByPrefix,
  getOpenPlatformById,
  isOpenPlatformId,
} from './open-platform';
import {
  declaredProviderCredential,
  apiKeyEnvMissingMessage,
  credentialConflictMessage,
  nonEmptyString,
} from './provider-credential';
import { isRecord } from './utils';

/**
 * Host capabilities the refresh orchestrator needs. Intentionally typed against
 * {@link ManagedFloydConfigShape} (the oauth package's own minimal config shape)
 * rather than the SDK's full `FloydConfig`, so this module has no dependency on
 * the engine or the SDK and can be reused by both the CLI and the daemon.
 */
export interface RefreshProviderHost {
  getConfig(): Promise<ManagedFloydConfigShape>;
  removeProvider(providerId: string): Promise<ManagedFloydConfigShape>;
  setConfig(patch: ManagedFloydConfigShape): Promise<ManagedFloydConfigShape>;
  resolveOAuthToken(providerName: string, oauthRef?: ManagedFloydOAuthRef): Promise<string>;
  /**
   * Product User-Agent sent on custom-registry (api.json) fetches, e.g.
   * `floyd-code-cli/1.2.3`. When omitted the fetch falls back to the runtime
   * default (`User-Agent: node`).
   */
  readonly userAgent?: string;
}

export interface ProviderChange {
  readonly providerId: string;
  /** User-facing name when available. */
  readonly providerName: string;
  readonly added: number;
  readonly removed: number;
}

export interface RefreshResult {
  /** Providers whose model list actually changed. */
  readonly changed: readonly ProviderChange[];
  /** Providers whose model list stayed identical after refresh. */
  readonly unchanged: readonly string[];
  readonly failed: ReadonlyArray<{ readonly provider: string; readonly reason: string }>;
}

export type RefreshProviderScope = 'all' | 'oauth';

export interface RefreshProviderOptions {
  readonly scope?: RefreshProviderScope;
  /**
   * Refresh only this provider. When set, managed / open-platform branches
   * skip every other provider; for a custom-registry provider the registry
   * group it belongs to is fetched but only the target entry is applied.
   */
  readonly providerId?: string;
}

interface ProviderView {
  readonly type?: string;
  readonly baseUrl?: string;
  readonly apiKey?: string;
  readonly apiKeyEnv?: unknown;
  readonly oauth?: ManagedFloydOAuthRef;
  readonly source?: unknown;
  readonly env?: unknown;
}

/**
 * Resolves the Bearer key for `type: 'floyd'` providers: the inline `apiKey`
 * wins, then a declared `apiKeyEnv` naming an environment variable (read from
 * `process.env` at refresh time), with `env.FLOYD_API_KEY` as the documented
 * config-file fallback. A declared `apiKeyEnv` whose variable is unset or
 * empty throws — silently falling through to another key source could send
 * requests (and bill) under the wrong account.
 *
 * Credential conflicts (`apiKey`+`apiKeyEnv`, `apiKeyEnv`+`oauth`,
 * `apiKey`+`oauth`) throw here too: refresh must not honor a configuration the
 * chat path would refuse, and the open-platform rewrite must never pick one
 * side of a conflict to persist.
 */
function resolveProviderApiKey(provider: ProviderView, providerName: string): string | undefined {
  const declared = declaredProviderCredential(provider, providerName);
  if (declared.kind === 'conflict') {
    throw new Error(declared.message);
  }
  if (declared.kind === 'inline') {
    return declared.apiKey;
  }
  if (declared.kind === 'env') {
    const value = nonEmptyString(process.env[declared.apiKeyEnv]);
    if (value === undefined) {
      throw new Error(apiKeyEnvMissingMessage(providerName, declared.apiKeyEnv));
    }
    return value;
  }
  if (isRecord(provider.env)) {
    const fromEnv = nonEmptyString(provider.env['FLOYD_API_KEY']);
    if (fromEnv !== undefined) {
      if (provider.oauth !== undefined) {
        throw new Error(credentialConflictMessage('Provider', providerName, 'apiKey', 'oauth'));
      }
      return fromEnv;
    }
  }
  return undefined;
}

function readProvider(
  config: ManagedFloydConfigShape,
  providerId: string,
): ProviderView | undefined {
  const provider = config.providers[providerId];
  if (provider === undefined) return undefined;
  return provider as ProviderView;
}

function readModel(
  config: ManagedFloydConfigShape,
  alias: string,
): ManagedFloydModelAlias | undefined {
  const model = config.models?.[alias];
  if (model === undefined) return undefined;
  return model as ManagedFloydModelAlias;
}

function customRegistrySourceKey(source: CustomRegistrySource): string {
  return JSON.stringify([source.url]);
}

function customRegistrySourceCredentialKey(source: CustomRegistrySource): string {
  return JSON.stringify([source.url, source.apiKey]);
}

async function fetchCustomRegistryFromSources(
  sources: readonly CustomRegistrySource[],
  userAgent?: string,
): Promise<{
  readonly entries: Awaited<ReturnType<typeof fetchCustomRegistry>>;
  readonly source: CustomRegistrySource;
}> {
  let lastError: unknown;
  for (const source of sources) {
    try {
      return {
        entries: await fetchCustomRegistry(source, { userAgent }),
        source,
      };
    } catch (error) {
      lastError = error;
    }
  }
  if (lastError instanceof Error) throw lastError;
  if (typeof lastError === 'string') throw new Error(lastError);
  throw new Error('No custom registry sources configured.');
}

function collectModelIdsForAliases(
  config: ManagedFloydConfigShape,
  aliasKeys: ReadonlySet<string>,
): Set<string> {
  const ids = new Set<string>();
  for (const aliasKey of aliasKeys) {
    const alias = readModel(config, aliasKey);
    if (alias !== undefined && alias.model.length > 0) {
      ids.add(alias.model);
    }
  }
  return ids;
}

function providerAliasKeys(config: ManagedFloydConfigShape, providerId: string): Set<string> {
  const keys = new Set<string>();
  for (const [alias, raw] of Object.entries(config.models ?? {})) {
    if ((raw as ManagedFloydModelAlias).provider === providerId) keys.add(alias);
  }
  return keys;
}

function generatedProviderAliasKeys(
  config: ManagedFloydConfigShape,
  providerId: string,
  aliasPrefix: string,
): Set<string> {
  const keys = new Set<string>();
  for (const [alias, raw] of Object.entries(config.models ?? {})) {
    const model = raw as ManagedFloydModelAlias;
    if (model.provider === providerId && alias.startsWith(aliasPrefix)) {
      keys.add(alias);
    }
  }
  return keys;
}

function computeChanges(oldIds: Set<string>, newIds: Set<string>): { added: number; removed: number } {
  let added = 0;
  for (const id of newIds) {
    if (!oldIds.has(id)) added++;
  }
  let removed = 0;
  for (const id of oldIds) {
    if (!newIds.has(id)) removed++;
  }
  return { added, removed };
}

interface ProviderModelSnapshot {
  readonly alias: string;
  readonly model: ManagedFloydModelAlias;
}

// Compare the full model metadata for the relevant aliases, not just model IDs:
// a registry can change capabilities (e.g. enabling reasoning) without changing
// any model ID. Spreading the whole alias keeps this in sync with the schema
// automatically; only `capabilities` needs normalizing because its order is not
// meaningful. `defaultModel` joins the snapshot so a lost selection flips the
// provider to changed and the re-selected default is written back.
function providerModelSnapshot(
  config: ManagedFloydConfigShape,
  providerId: string,
  aliasKeys: ReadonlySet<string>,
): string {
  const snapshots: ProviderModelSnapshot[] = [];
  for (const alias of aliasKeys) {
    const model = readModel(config, alias);
    if (model === undefined || model.provider !== providerId) continue;
    snapshots.push({
      alias,
      model: {
        ...model,
        capabilities: model.capabilities === undefined ? undefined : model.capabilities.toSorted(),
      },
    });
  }
  snapshots.sort((a, b) => a.alias.localeCompare(b.alias));
  return JSON.stringify({ defaultModel: config.defaultModel ?? null, models: snapshots });
}

function providerModelsEqual(
  config: ManagedFloydConfigShape,
  nextConfig: ManagedFloydConfigShape,
  providerId: string,
  aliasKeys: ReadonlySet<string>,
): boolean {
  return (
    providerModelSnapshot(config, providerId, aliasKeys) ===
    providerModelSnapshot(nextConfig, providerId, aliasKeys)
  );
}

function providerConfigSnapshot(config: ManagedFloydConfigShape, providerId: string): string {
  return JSON.stringify(config.providers[providerId] ?? null);
}

function providerConfigEqual(
  config: ManagedFloydConfigShape,
  nextConfig: ManagedFloydConfigShape,
  providerId: string,
): boolean {
  return providerConfigSnapshot(config, providerId) === providerConfigSnapshot(nextConfig, providerId);
}

function providerRefreshAliasKeys(
  config: ManagedFloydConfigShape,
  nextConfig: ManagedFloydConfigShape,
  providerId: string,
  aliasPrefix: string,
): Set<string> {
  const keys = generatedProviderAliasKeys(config, providerId, aliasPrefix);
  for (const key of providerAliasKeys(nextConfig, providerId)) keys.add(key);
  return keys;
}

function preserveUserProviderAliases(
  config: ManagedFloydConfigShape,
  providerId: string,
  refreshedAliasKeys: ReadonlySet<string>,
): Record<string, ManagedFloydModelAlias> {
  const preserved: Record<string, ManagedFloydModelAlias> = {};
  for (const [alias, raw] of Object.entries(config.models ?? {})) {
    const model = raw as ManagedFloydModelAlias;
    if (model.provider !== providerId || refreshedAliasKeys.has(alias)) continue;
    preserved[alias] = structuredClone(model);
  }
  return preserved;
}

function restoreProviderAliases(
  config: ManagedFloydConfigShape,
  aliases: Record<string, ManagedFloydModelAlias>,
): void {
  if (Object.keys(aliases).length === 0) return;
  config.models = {
    ...config.models,
    ...aliases,
  };
}

function restoreDefaultSelection(
  config: ManagedFloydConfigShape,
  defaultModel: string | undefined,
  defaultEnabled: boolean | undefined,
): void {
  if (defaultModel === undefined || readModel(config, defaultModel) === undefined) return;
  config.defaultModel = defaultModel;
  // A refresh may have just learned that the default model cannot disable
  // thinking — never restore a stale thinking-off selection onto it.
  const capabilities = readModel(config, defaultModel)?.capabilities ?? [];
  const enabled = capabilities.includes('always_thinking') ? true : defaultEnabled;
  if (enabled !== undefined) {
    config.thinking = { ...config.thinking, enabled };
  }
}

async function rebaseSelectionAfterFetch(
  host: RefreshProviderHost,
  config: ManagedFloydConfigShape,
): Promise<ManagedFloydConfigShape> {
  const fresh = await host.getConfig();
  return { ...config, defaultModel: fresh.defaultModel, thinking: fresh.thinking };
}

// `apply*` may leave `defaultModel` pointing at an alias that no longer exists
// (e.g. the previously-selected model was dropped from the registry). The host's
// `setConfig` deep-merge cannot clear a key, so the matching `removeProvider`
// call handles disk cleanup while this drops the dangling reference in memory.
function clampDanglingDefault(config: ManagedFloydConfigShape): void {
  if (config.defaultModel !== undefined && readModel(config, config.defaultModel) === undefined) {
    config.defaultModel = undefined;
    config.thinking = undefined;
  }
}

function clearDefaultThinkingWhenDefaultRemoved(
  config: ManagedFloydConfigShape,
  previousDefaultModel: string | undefined,
): void {
  if (previousDefaultModel !== undefined && config.defaultModel === undefined) {
    config.thinking = undefined;
  }
}

function pickDefaultModel(
  config: ManagedFloydConfigShape,
  providerId: string,
  models: Array<{ id: string }>,
): string {
  const firstModel = models[0];
  if (firstModel === undefined) return '';

  const existingDefault = config.defaultModel;
  if (existingDefault !== undefined) {
    const alias = readModel(config, existingDefault);
    if (alias !== undefined && alias.provider === providerId) {
      const stillAvailable = models.find((m) => m.id === alias.model);
      if (stillAvailable !== undefined) {
        return stillAvailable.id;
      }
    }
  }
  return firstModel.id;
}

/**
 * Refresh remote model metadata for the configured providers and persist any
 * changes through the host. Handles four provider kinds, in order:
 *
 *  1. Managed Floyd Code (OAuth) — `GET /models` against the runtime endpoint.
 *  2. Open platforms (legacy-cn, legacy-ai, …) — platform catalog fetch.
 *  2.5. Managed-endpoint API-key providers — hand-written `type: 'floyd'`
 *     providers (including a hand-written `managed:floyd-code` without an oauth
 *     ref) whose baseUrl is exactly the managed Floyd Code endpoint; refreshed
 *     via `GET /models` with the configured API key as Bearer. Only model
 *     aliases are merged; the provider record is user-owned and never
 *     rewritten.
 *  3. Custom registries (models.dev-style, keyed by `provider.source`).
 *
 * Each branch diffs old vs new and only writes when something actually changed
 * (`removeProvider` then `setConfig`). Failures are collected per-provider and
 * never abort the whole refresh. Pass `providerId` to scope the refresh to a
 * single provider; pass `scope: 'oauth'` to refresh only the managed provider.
 */
export async function refreshProviderModels(
  host: RefreshProviderHost,
  options: RefreshProviderOptions = {},
): Promise<RefreshResult> {
  const changed: ProviderChange[] = [];
  const unchanged: string[] = [];
  const failed: Array<{ provider: string; reason: string }> = [];
  const scope = options.scope ?? 'all';
  const targetId = options.providerId;

  let config = await host.getConfig();

  // ---------------------------------------------------------------------------
  // 1. Managed Floyd Code (OAuth)
  // ---------------------------------------------------------------------------
  const managedProvider = readProvider(config, FLOYD_CODE_PROVIDER_NAME);
  const managedWanted = targetId === undefined || targetId === FLOYD_CODE_PROVIDER_NAME;
  if (
    managedWanted &&
    managedProvider !== undefined &&
    managedProvider.type === 'floyd' &&
    managedProvider.oauth !== undefined
  ) {
    try {
      const declared = declaredProviderCredential(managedProvider, FLOYD_CODE_PROVIDER_NAME);
      if (declared.kind === 'conflict') {
        throw new Error(declared.message);
      }
      const auth = resolveFloydCodeRuntimeAuth({
        configuredBaseUrl: managedProvider.baseUrl,
        configuredOAuthRef: managedProvider.oauth,
      });
      const accessToken = await host.resolveOAuthToken(FLOYD_CODE_PROVIDER_NAME, auth.oauthRef);
      const models = await fetchManagedFloydCodeModels({
        accessToken,
        baseUrl: auth.baseUrl,
      });
      if (models.length > 0) {
        config = await rebaseSelectionAfterFetch(host, config);
        const next = structuredClone(config);
        applyManagedFloydCodeConfig(next, {
          models,
          baseUrl: auth.baseUrl,
          oauthKey: auth.oauthRef.key,
          oauthHost: auth.oauthRef.oauthHost,
          preserveDefaultModel: true,
        });
        const refreshedAliasKeys = providerRefreshAliasKeys(
          config,
          next,
          FLOYD_CODE_PROVIDER_NAME,
          `${FLOYD_CODE_PLATFORM_ID}/`,
        );
        restoreProviderAliases(
          next,
          preserveUserProviderAliases(config, FLOYD_CODE_PROVIDER_NAME, refreshedAliasKeys),
        );
        restoreDefaultSelection(next, config.defaultModel, config.thinking?.enabled);
        clampDanglingDefault(next);
        clearDefaultThinkingWhenDefaultRemoved(next, config.defaultModel);

        if (providerModelsEqual(config, next, FLOYD_CODE_PROVIDER_NAME, refreshedAliasKeys)) {
          unchanged.push(FLOYD_CODE_PROVIDER_NAME);
        } else {
          const { added, removed } = computeChanges(
            collectModelIdsForAliases(config, refreshedAliasKeys),
            collectModelIdsForAliases(next, refreshedAliasKeys),
          );
          await host.removeProvider(FLOYD_CODE_PROVIDER_NAME);
          config = await host.setConfig({
            providers: next.providers,
            models: next.models,
            defaultModel: next.defaultModel,
            thinking: next.thinking,
          });
          changed.push({
            providerId: FLOYD_CODE_PROVIDER_NAME,
            providerName: 'Floyd Code',
            added,
            removed,
          });
        }
      }
    } catch (error) {
      failed.push({
        provider: FLOYD_CODE_PROVIDER_NAME,
        reason: error instanceof Error ? error.message : String(error),
      });
    }
  }

  // The oauth scope stops here, but a targeted refresh of the managed provider
  // must fall through: branch 2 no-ops on a non-open-platform id, branch 2.5
  // handles a hand-written `managed:floyd-code` that carries an API key instead
  // of an oauth ref, and branch 3 no-ops when no registry group contains it.
  if (scope === 'oauth') {
    return { changed, unchanged, failed };
  }

  // ---------------------------------------------------------------------------
  // 2. Open Platforms (legacy-cn, legacy-ai, …)
  // ---------------------------------------------------------------------------
  const openPlatformIds = Object.keys(config.providers).filter((id) => {
    if (!isOpenPlatformId(id)) return false;
    const provider = readProvider(config, id);
    return provider !== undefined && readCustomRegistrySource(provider) === undefined;
  });
  for (const providerId of openPlatformIds) {
    if (targetId !== undefined && targetId !== providerId) continue;
    const platform = getOpenPlatformById(providerId);
    if (platform === undefined) continue;

    const providerConfig = readProvider(config, providerId);
    if (providerConfig === undefined) continue;

    const configuredBaseUrl = nonEmptyString(providerConfig.baseUrl);
    const refreshPlatform =
      configuredBaseUrl === undefined || nonEmptyString(platform.baseUrl) !== undefined
        ? platform
        : { ...platform, baseUrl: configuredBaseUrl };

    try {
      const declared = declaredProviderCredential(providerConfig, providerId);
      const apiKey = resolveProviderApiKey(providerConfig, providerId);
      if (apiKey === undefined) continue;
      let models = await fetchOpenPlatformModels(refreshPlatform, apiKey);
      models = filterModelsByPrefix(models, refreshPlatform);
      if (models.length === 0) continue;

      config = await rebaseSelectionAfterFetch(host, config);
      const selectedModelId = pickDefaultModel(config, providerId, models);
      const selectedModel = models.find((m) => m.id === selectedModelId);
      if (selectedModel === undefined) continue;
      const next = structuredClone(config);
      applyOpenPlatformConfig(next, {
        platform: refreshPlatform,
        models,
        selectedModel,
        thinking: false,
        credential: declared.kind === 'env' ? { apiKeyEnv: declared.apiKeyEnv } : { apiKey },
      });
      const refreshedAliasKeys = providerRefreshAliasKeys(
        config,
        next,
        providerId,
        `${providerId}/`,
      );
      restoreProviderAliases(next, preserveUserProviderAliases(config, providerId, refreshedAliasKeys));
      restoreDefaultSelection(next, config.defaultModel, config.thinking?.enabled);
      clampDanglingDefault(next);
      clearDefaultThinkingWhenDefaultRemoved(next, config.defaultModel);

      if (providerModelsEqual(config, next, providerId, refreshedAliasKeys)) {
        unchanged.push(providerId);
      } else {
        const { added, removed } = computeChanges(
          collectModelIdsForAliases(config, refreshedAliasKeys),
          collectModelIdsForAliases(next, refreshedAliasKeys),
        );
        await host.removeProvider(providerId);
        config = await host.setConfig({
          providers: next.providers,
          models: next.models,
          defaultModel: next.defaultModel,
          thinking: next.thinking,
        });
        changed.push({
          providerId,
          providerName: platform.name,
          added,
          removed,
        });
      }
    } catch (error) {
      failed.push({
        provider: providerId,
        reason: error instanceof Error ? error.message : String(error),
      });
    }
  }

  // ---------------------------------------------------------------------------
  // 2.5. Managed-endpoint API-key providers (hand-configured distributed keys)
  // ---------------------------------------------------------------------------
  // A hand-written `type: 'floyd'` provider whose baseUrl is exactly the managed
  // Floyd Code endpoint, carrying an API key (inline, via `apiKeyEnv`, or via
  // `env.FLOYD_API_KEY`) instead of an oauth ref, gets its model list refreshed
  // from `{baseUrl}/models` just like the OAuth branch. Strict baseUrl matching
  // keeps proxies / gateways with an untrusted `/models` schema out.
  for (const providerId of Object.keys(config.providers)) {
    if (isOpenPlatformId(providerId)) continue;
    if (targetId !== undefined && targetId !== providerId) continue;
    const provider = readProvider(config, providerId);
    if (provider === undefined) continue;
    if (provider.type !== 'floyd') continue;
    const earlyDeclared = declaredProviderCredential(provider, providerId);
    if (earlyDeclared.kind === 'conflict') {
      if (providerId !== FLOYD_CODE_PROVIDER_NAME || provider.oauth === undefined) {
        failed.push({ provider: providerId, reason: earlyDeclared.message });
      }
      continue;
    }
    if (provider.oauth !== undefined) continue;
    if (readCustomRegistrySource(provider) !== undefined) continue;
    if (!isManagedFloydCodeBaseUrl(provider.baseUrl)) continue;

    try {
      const apiKey = resolveProviderApiKey(provider, providerId);
      if (apiKey === undefined) continue;
      const models = await fetchManagedFloydCodeModels({
        accessToken: apiKey,
        baseUrl: provider.baseUrl,
        credentialKind: 'apiKey',
      });
      if (models.length === 0) continue;

      config = await rebaseSelectionAfterFetch(host, config);
      // A hand-written `managed:floyd-code` shares the OAuth branch's
      // `floyd-code/` alias prefix so the two shapes merge cleanly if the user
      // later logs in via OAuth; ordinary providers use their own id.
      const aliasPrefix =
        providerId === FLOYD_CODE_PROVIDER_NAME ? `${FLOYD_CODE_PLATFORM_ID}/` : `${providerId}/`;
      const next = structuredClone(config);
      applyManagedApiKeyProviderModels(next, providerId, models, aliasPrefix);
      const refreshedAliasKeys = providerRefreshAliasKeys(config, next, providerId, aliasPrefix);
      restoreProviderAliases(
        next,
        preserveUserProviderAliases(config, providerId, refreshedAliasKeys),
      );
      restoreDefaultSelection(next, config.defaultModel, config.thinking?.enabled);
      clampDanglingDefault(next);
      clearDefaultThinkingWhenDefaultRemoved(next, config.defaultModel);

      if (providerModelsEqual(config, next, providerId, refreshedAliasKeys)) {
        unchanged.push(providerId);
      } else {
        const { added, removed } = computeChanges(
          collectModelIdsForAliases(config, refreshedAliasKeys),
          collectModelIdsForAliases(next, refreshedAliasKeys),
        );
        await host.removeProvider(providerId);
        config = await host.setConfig({
          providers: next.providers,
          models: next.models,
          defaultModel: next.defaultModel,
          thinking: next.thinking,
          // The v1 `removeProvider` RPC clears `defaultProvider` when it points
          // at this provider; the clone still holds the original value, so
          // write it back — a refresh must not silently drop the fallback.
          defaultProvider: next['defaultProvider'],
        });
        changed.push({
          providerId,
          providerName: providerId,
          added,
          removed,
        });
      }
    } catch (error) {
      failed.push({
        provider: providerId,
        reason: error instanceof Error ? error.message : String(error),
      });
    }
  }

  // ---------------------------------------------------------------------------
  // 3. Custom Registry providers (grouped by URL, with API-key candidates)
  // ---------------------------------------------------------------------------
  const customSources = new Map<
    string,
    {
      readonly sources: CustomRegistrySource[];
      readonly sourceKeys: Set<string>;
      readonly providerIds: string[];
    }
  >();
  for (const providerId of Object.keys(config.providers)) {
    const provider = readProvider(config, providerId);
    if (provider === undefined) continue;
    if (provider.oauth !== undefined) continue;
    const source = readCustomRegistrySource(provider);
    if (source === undefined) continue;
    const key = customRegistrySourceKey(source);
    const sourceKey = customRegistrySourceCredentialKey(source);
    const entry = customSources.get(key);
    if (entry !== undefined) {
      if (!entry.sourceKeys.has(sourceKey)) {
        entry.sources.push(source);
        entry.sourceKeys.add(sourceKey);
      }
      entry.providerIds.push(providerId);
    } else {
      customSources.set(key, {
        sources: [source],
        sourceKeys: new Set([sourceKey]),
        providerIds: [providerId],
      });
    }
  }

  for (const { sources, providerIds } of customSources.values()) {
    // When scoped to a single provider, only refresh the registry group it
    // belongs to and only apply the target entry (siblings under the same URL
    // are left untouched).
    if (targetId !== undefined && !providerIds.includes(targetId)) continue;
    try {
      const { entries, source } = await fetchCustomRegistryFromSources(sources, host.userAgent);
      if (Object.keys(entries).length === 0) {
        throw new Error(`Custom registry at ${source.url} contained no usable providers.`);
      }
      config = await rebaseSelectionAfterFetch(host, config);
      // Build the whole batch on one clone so that several changed providers
      // from the same source do not overwrite each other's aliases, and so the
      // config we compare is exactly the config we persist.
      const next = structuredClone(config);
      const changedProviders: Array<{
        readonly providerId: string;
        readonly providerName: string;
        readonly added: number;
        readonly removed: number;
      }> = [];
      const providersToRemoveBeforeSet = new Set<string>();
      let hasUnreportedConfigChange = false;
      const remoteEntries = Object.values(entries);
      const remoteEntriesByProviderId = new Map(
        remoteEntries.map((entry) => [entry.id, entry]),
      );
      const providerIdsToSync = new Set(providerIds);
      const rejectedProviderIds = new Set<string>();
      // Only pull in newly-appeared providers from the registry when running an
      // unscoped refresh; a scoped refresh must not add siblings.
      if (targetId === undefined) {
        for (const entry of remoteEntries) {
          if (isReservedProviderId(entry.id)) {
            rejectedProviderIds.add(entry.id);
            failed.push({
              provider: entry.id,
              reason: reservedProviderIdMessage(entry.id),
            });
            continue;
          }
          const existing = readProvider(config, entry.id);
          if (existing?.oauth !== undefined) {
            rejectedProviderIds.add(entry.id);
            failed.push({
              provider: entry.id,
              reason: oauthManagedProviderMessage(entry.id),
            });
            continue;
          }
          providerIdsToSync.add(entry.id);
        }
      }

      for (const providerId of providerIdsToSync) {
        if (rejectedProviderIds.has(providerId)) continue;
        if (targetId !== undefined && providerId !== targetId) continue;
        const entry = remoteEntriesByProviderId.get(providerId);
        if (entry === undefined) {
          if (isReservedProviderId(providerId)) {
            failed.push({
              provider: providerId,
              reason: reservedProviderIdMessage(providerId),
            });
            continue;
          }
          const oldIds = collectModelIdsForAliases(config, providerAliasKeys(config, providerId));
          removeCustomRegistryProvider(next, providerId);
          changedProviders.push({
            providerId,
            providerName: providerId,
            added: 0,
            removed: oldIds.size,
          });
          providersToRemoveBeforeSet.add(providerId);
          continue;
        }

        const existingProvider = readProvider(config, providerId);
        if (existingProvider?.oauth !== undefined) {
          failed.push({
            provider: providerId,
            reason: oauthManagedProviderMessage(providerId),
          });
          continue;
        }
        const declared = declaredProviderCredential(existingProvider ?? {}, providerId);
        if (declared.kind === 'conflict') {
          failed.push({ provider: providerId, reason: declared.message });
          continue;
        }
        const existed = existingProvider !== undefined;
        try {
          applyCustomRegistryProvider(next, entry, source);
        } catch (error) {
          failed.push({
            provider: providerId,
            reason: error instanceof Error ? error.message : String(error),
          });
          continue;
        }
        const refreshedAliasKeys = providerRefreshAliasKeys(config, next, providerId, `${providerId}/`);
        if (existed) {
          restoreProviderAliases(next, preserveUserProviderAliases(config, providerId, refreshedAliasKeys));
        }

        if (
          existed &&
          providerModelsEqual(config, next, providerId, refreshedAliasKeys) &&
          providerConfigEqual(config, next, providerId)
        ) {
          unchanged.push(providerId);
        } else if (existed && providerModelsEqual(config, next, providerId, refreshedAliasKeys)) {
          unchanged.push(providerId);
          providersToRemoveBeforeSet.add(providerId);
          hasUnreportedConfigChange = true;
        } else {
          const { added, removed } = computeChanges(
            collectModelIdsForAliases(config, refreshedAliasKeys),
            collectModelIdsForAliases(next, refreshedAliasKeys),
          );
          changedProviders.push({
            providerId,
            providerName: entry.name || providerId,
            added,
            removed,
          });
          if (existed) providersToRemoveBeforeSet.add(providerId);
        }
      }

      if (changedProviders.length > 0 || hasUnreportedConfigChange) {
        restoreDefaultSelection(next, config.defaultModel, config.thinking?.enabled);
        clampDanglingDefault(next);
        clearDefaultThinkingWhenDefaultRemoved(next, config.defaultModel);
        for (const providerId of providersToRemoveBeforeSet) {
          await host.removeProvider(providerId);
        }
        config = await host.setConfig({
          providers: next.providers,
          models: next.models,
          defaultModel: next.defaultModel,
          thinking: next.thinking,
        });
        for (const change of changedProviders) {
          changed.push({
            providerId: change.providerId,
            providerName: change.providerName,
            added: change.added,
            removed: change.removed,
          });
        }
      }
    } catch (error) {
      const reportedIds = targetId !== undefined ? [targetId] : providerIds;
      for (const providerId of reportedIds) {
        failed.push({
          provider: providerId,
          reason: error instanceof Error ? error.message : String(error),
        });
      }
    }
  }

  return { changed, unchanged, failed };
}
