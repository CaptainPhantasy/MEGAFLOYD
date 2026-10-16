export { FloydHarness } from '#/floyd-harness';
export type { FloydHarnessRuntimeOptions } from '#/floyd-harness';
export { Session } from '#/session';
export { FloydAuthFacade } from '#/auth';
export {
  createFloydHarness,
  SDKRpcClientV2,
  type SDKRpcClientV2Options,
} from '#/sdk-rpc-client-v2';
export {
  createFloydConfigRpc,
  FloydConfigRpcClient,
  type FloydConfigRpc,
  type FloydConfigValidationIssue,
  type FloydConfigValidationPathSegment,
  type ResolveFloydConfigPathInput,
  type ValidateFloydConfigTomlInput,
} from '#/config-rpc';
export { SDKRpcClientBase } from '#/rpc';
export { FloydForCodingProvider } from '#/floyd-code-model-provider';
export type { FloydForCodingProviderOptions } from '#/floyd-code-model-provider';
export { removeProviderFromConfig } from '#/v2/config-mapper';

export {
  applyCatalogProvider,
  catalogBaseUrl,
  catalogModelToAlias,
  catalogProviderModels,
  CatalogFetchError,
  RegistryImportError,
  DEFAULT_CATALOG_URL,
  fetchCatalog,
  inferWireType,
  loadBuiltInCatalog,
  resolveCatalogImport,
} from '#/catalog';
export type {
  ApplyCatalogProviderOptions,
  Catalog,
  CatalogImportInvalidReason,
  CatalogImportResolution,
  CatalogModel,
  CatalogProviderEntry,
  FetchCatalogOptions,
} from '#/catalog';

export {
  ErrorCodes,
  FloydError,
  type FloydErrorCode,
  type FloydErrorInfo,
  type FloydErrorOptions,
  type FloydErrorPayload,
  FLOYD_ERROR_INFO,
  fromFloydErrorPayload,
  isFloydError,
  toFloydErrorPayload,
} from '#/errors';

export {
  flushDiagnosticLogs,
  flushDiagnosticLogsSync,
  log,
  redact,
  resolveGlobalLogPath,
} from '#/logging/index';
export { resolveFloydHome } from '@legacy-ai/agent-core-v2';
export type { LogContext, LogLevel, LogPayload, Logger } from '#/logging/index';

export { effectiveModelAlias, loadRuntimeConfigSafe } from '#/config/index';
export { resolveConfigPath } from '@legacy-ai/agent-core-v2';
export { limitAgentReplayByTurns } from '#/replay';
export { parseAgentFileText, resolveAgentPath } from '@legacy-ai/agent-core-v2';
export { SECONDARY_DERIVED_MODEL_ALIAS } from '#/config/index';
export { PRIMARY_SUBAGENT_MODEL_CHOICE } from '@legacy-ai/agent-core-v2/session/subagent/configSection';

export { installGlobalProxyDispatcher } from '#/proxy';

export {
  buildImageCompressionCaption,
  buildUnsupportedImageNotice,
  gateImageFormatParts,
  isModelAcceptedImageMime,
  normalizeImageMime,
  parseImageDataUrl,
  persistOriginalImage,
  sessionMediaOriginalsDir,
  IMAGE_BYTE_BUDGET,
  MAX_IMAGE_EDGE_PX,
} from '@legacy-ai/agent-core-v2';
export { compressBase64ForModel, compressImageForModel, ImageLimits } from '#/image';
export type {
  CompressImageOptions,
  CompressImageResult,
  CompressBase64Result,
  ImageCompressionCaptionInput,
  ImageCompressionTelemetry,
} from '#/image';

export type {
  ExperimentalFeatureState,
  ExperimentalFlagMap,
  ExperimentalFlagSource,
  FlagDefinition,
  FlagDefinitionInput,
  FlagId,
  FlagSurface,
} from '#/flag';

export {
  buildDaemonFileUrl,
  buildMediaPathTag,
  isDaemonFileUrl,
  matchSingleMediaPathTag,
  parseDaemonFileUrl,
} from '@legacy-ai/agent-core-v2/agent/media/mediaRef';
export type {
  DaemonFileRef,
  MediaKind,
} from '@legacy-ai/agent-core-v2/agent/media/mediaRef';

export type {
  FloydAuthCompleteFeedbackUploadInput,
  FloydAuthCompleteFeedbackUploadPart,
  FloydAuthCreateFeedbackUploadUrlInput,
  FloydAuthCreateFeedbackUploadUrlOk,
  FloydAuthCreateFeedbackUploadUrlResult,
  FloydAuthFeedbackUploadPart,
  FloydAuthLoginResult,
  FloydAuthLogoutResult,
  FloydAuthSubmitFeedbackInput,
} from '#/auth';

export * from '#/events';
export type * from '#/types';
