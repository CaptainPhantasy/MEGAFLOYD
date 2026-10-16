export {
  DeviceCodeExpiredError,
  DeviceCodeTimeoutError,
  OAuthAccessDeniedError,
  OAuthConnectionError,
  OAuthError,
  OAuthUnauthorizedError,
  RetryableRefreshError,
} from './errors';

export type {
  DeviceAuthorization,
  DeviceHeaders,
  OAuthFlowConfig,
  OAuthStorageBackend,
  TokenInfo,
  TokenInfoWire,
} from './types';
export { tokenFromWire, tokenToWire } from './types';

export type { TokenStorage } from './storage';
export { FileTokenStorage } from './storage';

export type { DevicePollResult, RefreshOptions } from './oauth';
export { pollDeviceToken, refreshAccessToken, requestDeviceAuthorization } from './oauth';

export type { LoginOptions, OAuthManagerOptions, OAuthRefreshOutcome } from './oauth-manager';
export { OAuthManager, defaultRefreshThreshold, newInstanceId } from './oauth-manager';

export {
  assertFloydHostIdentity,
  createFloydDefaultHeaders,
  createFloydDeviceHeaders,
  createFloydDeviceId,
  createFloydUserAgent,
  FLOYD_CODE_CUSTOM_HEADERS_ENV,
  FLOYD_CODE_PLATFORM,
  parseFloydCodeCustomHeaders,
  readFloydDeviceId,
  replaceUserAgentProduct,
} from './identity';
export type { FloydHostIdentity, FloydIdentityOptions } from './identity';

export { FLOYD_CODE_FLOW_CONFIG } from './constants';

export {
  FLOYD_REGION_MARKER_FILENAME,
  FLOYD_REGION_PROFILES,
  floydCdnContentUrl,
  floydRegionLoginHosts,
  floydRegionProfile,
  floydRegionSchema,
  resolveFloydRegion,
} from './region';
export type { FloydRegion, FloydRegionProfile, ResolveFloydRegionOptions } from './region';

export {
  applyManagedApiKeyProviderModels,
  applyManagedFloydCodeLogoutConfig,
  applyManagedFloydCodeConfig,
  clearManagedFloydCodeConfig,
  fetchManagedFloydCodeModels,
  floydCodeEnvBaseUrl,
  floydCodeEnvOAuthHost,
  FLOYD_CODE_OAUTH_KEY,
  FLOYD_CODE_PLATFORM_ID,
  FLOYD_CODE_PROVIDER_NAME,
  ManagedFloydCodeModelsAuthError,
  provisionManagedFloydCodeConfig,
  resolveFloydCodeLoginAuth,
  resolveFloydCodeOAuthKey,
  resolveFloydCodeOAuthRef,
  resolveFloydCodeRuntimeAuth,
  toManagedModelAlias,
} from './managed-floyd-code';
export type {
  FetchManagedFloydCodeModelsOptions,
  ManagedFloydCodeApplyResult,
  ManagedFloydCodeCleanupResult,
  ManagedFloydCodeProtocol,
  ManagedFloydEnv,
  ManagedFloydLoginAuth,
  ManagedFloydCodeModelInfo,
  ManagedFloydCodeProvisionResult,
  ManagedFloydConfigAdapter,
  ManagedFloydConfigShape,
  ManagedFloydOAuthRef,
  ManagedFloydOAuthRefInput,
  ManagedFloydRuntimeAuth,
  ProvisionManagedFloydCodeConfigOptions,
} from './managed-floyd-code';

export {
  fetchManagedUserInfo,
  floydCodeUserInfoUrl,
  managedUserInfoPhoneSchema,
  managedUserInfoResultSchema,
  managedUserInfoSchema,
  parseManagedUserInfoPayload,
} from './managed-userinfo';
export type {
  FetchManagedUserInfoError,
  FetchManagedUserInfoResult,
  ManagedUserInfo,
  ManagedUserInfoPhone,
  ManagedUserInfoResult,
} from './managed-userinfo';

export {
  boosterWalletInfoSchema,
  fetchManagedUsage,
  formatDuration,
  isManagedFloydCode,
  isManagedFloydCodeBaseUrl,
  floydCodeBaseUrl,
  floydCodeUsageUrl,
  managedQuotaEntrySchema,
  managedQuotaSchema,
  managedQuotaUsagesSchema,
  managedUsageResultSchema,
  parseManagedUsagePayload,
} from './managed-usage';
export type {
  BoosterWalletInfo,
  FetchManagedUsageError,
  FetchManagedUsageResult,
  ManagedQuota,
  ManagedQuotaEntry,
  ManagedQuotaUsages,
  ManagedUsageResult,
} from './managed-usage';

export { fetchChatTitle, floydCodeToolsUrl } from './managed-tools';
export type {
  FetchChatTitleError,
  FetchChatTitleOk,
  FetchChatTitleResult,
} from './managed-tools';

export { fetchSubmitFeedback, floydCodeFeedbackUrl } from './managed-feedback';
export type {
  FetchSubmitFeedbackError,
  FetchSubmitFeedbackOk,
  FetchSubmitFeedbackResult,
  SubmitFeedbackBody,
} from './managed-feedback';

export {
  fetchCompleteFeedbackUpload,
  fetchCreateFeedbackUploadUrl,
  floydCodeFeedbackUploadCompleteUrl,
  floydCodeFeedbackUploadUrl,
} from './managed-feedback-upload';
export type {
  CompleteFeedbackUploadBody,
  CreateFeedbackUploadUrlBody,
  CreateFeedbackUploadUrlResponse,
  FetchCompleteFeedbackUploadResult,
  FetchCreateFeedbackUploadUrlResult,
  FetchFeedbackUploadError,
} from './managed-feedback-upload';

export {
  applyOpenPlatformConfig,
  capabilitiesForModel,
  fetchOpenPlatformModels,
  filterModelsByPrefix,
  getOpenPlatformById,
  isOpenPlatformId,
  OPEN_PLATFORMS,
  OpenPlatformApiError,
  removeOpenPlatformConfig,
} from './open-platform';
export type {
  ApplyOpenPlatformResult,
  OpenPlatformDefinition,
} from './open-platform';

export {
  applyCustomRegistryEntries,
  applyCustomRegistryProvider,
  capabilitiesFromCustomEntry,
  credentialEnvHints,
  CustomRegistryApiError,
  CUSTOM_REGISTRY_DEFAULT_CAPABILITIES,
  CUSTOM_REGISTRY_DEFAULT_MAX_CONTEXT,
  customRegistryReplacementKeys,
  fetchCustomRegistry,
  removeCustomRegistryEntries,
  removeCustomRegistryProvider,
} from './custom-registry';
export type {
  CustomRegistryModelEntry,
  CustomRegistryProviderEntry,
  CustomRegistryProviderType,
  CustomRegistryRemoval,
  CustomRegistryReplacementKeys,
  CustomRegistrySource,
  FetchCustomRegistryOptions,
} from './custom-registry';

export {
  apiKeyEnvMissingMessage,
  credentialConflictMessage,
  declaredProviderCredential,
  reconcileProviderCredentialUpdate,
} from './provider-credential';
export type {
  DeclaredProviderCredential,
  ProviderCredentialReconciliation,
  ProviderCredentialUpdate,
  ProviderCredentialView,
} from './provider-credential';

export { FloydOAuthToolkit, resolveFloydTokenStorageName } from './toolkit';
export type {
  AuthManagedUsageResult,
  AuthManagedUserInfoResult,
  AuthProviderStatus,
  AuthStatus,
  BearerTokenProvider,
  FloydOAuthLoginOptions,
  FloydOAuthLoginResult,
  FloydOAuthLogoutResult,
  FloydOAuthTokenRef,
  FloydOAuthToolkitOptions,
} from './toolkit';

export { refreshProviderModels } from './refreshProviderModels';
export type {
  ProviderChange,
  RefreshProviderHost,
  RefreshProviderOptions,
  RefreshProviderScope,
  RefreshResult,
} from './refreshProviderModels';

export type { OAuthTokenTransactionOptions } from './oauth-token-transaction';
export { OAuthTokenTransaction } from './oauth-token-transaction';
