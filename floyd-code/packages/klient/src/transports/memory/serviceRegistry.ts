/**
 * Service name → DI token registry for the in-process dispatcher. Only leaf
 * modules are imported (tokens + types) — never the engine root barrel, so
 * hosting klient in-process does not force the full registration side effects
 * beyond what the host already bootstrapped.
 */

import type { ServiceIdentifier } from '@legacy-ai/agent-core-v2/_base/di/instantiation';
import { ISessionIndex } from '@legacy-ai/agent-core-v2/app/sessionIndex/sessionIndex';
import { IWorkspaceService } from '@legacy-ai/agent-core-v2/app/workspace/workspace';
import { IConfigService } from '@legacy-ai/agent-core-v2/app/config/config';
import { IModelService } from '@legacy-ai/agent-core-v2/llm-adapter/model/model';
import { IModelCatalog } from '@legacy-ai/agent-core-v2/llm-adapter/model/catalog';
import { IProviderDiscoveryService } from '@legacy-ai/agent-core-v2/app/kosongConfig/discovery';
import { IModelsDevImportService } from '@legacy-ai/agent-core-v2/app/kosongConfig/modelsDevImport';
import { IProviderService } from '@legacy-ai/agent-core-v2/llm-adapter/provider/provider';
import {
  IAuthSummaryService,
  IOAuthService,
} from '@legacy-ai/agent-core-v2/app/auth/auth';
import { IFlagService } from '@legacy-ai/agent-core-v2/app/flag/flag';
import { IPluginService } from '@legacy-ai/agent-core-v2/app/plugin/plugin';
import { ICapabilityService } from '@legacy-ai/agent-core-v2/app/capability/capability';
import { IBootstrapService } from '@legacy-ai/agent-core-v2/app/bootstrap/bootstrap';
import { IEventService } from '@legacy-ai/agent-core-v2/app/event/event';
import { IFileService } from '@legacy-ai/agent-core-v2/app/file/fileService';
import { IHostFolderBrowser } from '@legacy-ai/agent-core-v2/app/hostFolderBrowser/hostFolderBrowser';
import { IWorkspaceInstanceManager } from '@legacy-ai/agent-core-v2/workspace/workspaceInstance/workspaceInstanceManager';
import { ISessionManager } from '@legacy-ai/agent-core-v2/app/sessionManager/sessionManager';
import { ISessionMetadata } from '@legacy-ai/agent-core-v2/session/sessionMetadata/sessionMetadata';
import { ISessionSkillCatalog } from '@legacy-ai/agent-core-v2/features/skill/session/skillCatalog';
import { ISessionTitleService } from '@legacy-ai/agent-core-v2/session/sessionTitle/sessionTitle';
import { IAgentLoopService } from '@legacy-ai/agent-core-v2/agent/loop/loop';
import { IAgentPromptChannel } from '@legacy-ai/agent-core-v2/agent/loop/promptChannel';
import { IAgentPermissionModeService } from '@legacy-ai/agent-core-v2/agent/permissionMode/permissionMode';
import { IAgentCommandService } from '@legacy-ai/agent-core-v2/agent/command/agentCommand';
import { IAgentRuntimeBindingService } from '@legacy-ai/agent-core-v2/agent/runtimeBinding/runtimeBinding';
import { IAgentContextMemoryService } from '@legacy-ai/agent-core-v2/agent/contextMemory/contextMemory';
import { ISessionTokenCountingService } from '@legacy-ai/agent-core-v2/session/tokenCounting/sessionTokenCounting';
import { ISessionActivityView } from '@legacy-ai/agent-core-v2/session/sessionActivity/sessionActivity';
import { IAgentPlanService } from '@legacy-ai/agent-core-v2/features/plan/plan';
import { IAgentProfileService } from '@legacy-ai/agent-core-v2/agent/profile/profile';
import { IAgentShellCommandService } from '@legacy-ai/agent-core-v2/agent/shellCommand/shellCommand';
import { IAgentTaskService } from '@legacy-ai/agent-core-v2/agent/task/task';
import { ISessionUsageService } from '@legacy-ai/agent-core-v2/session/usage/sessionUsage';
import { IAgentMcpService } from '@legacy-ai/agent-core-v2/agent/mcp/mcp';
import { IAgentFullCompactionService } from '@legacy-ai/agent-core-v2/agent/fullCompaction/fullCompaction';
import { IMcpManagementService } from '@legacy-ai/agent-core-v2/app/mcpManagement/mcpManagement';

/** Wire service name (decorator id string) → token. */
export const serviceTokens: Readonly<Record<string, ServiceIdentifier<unknown>>> = {
  sessionIndex: ISessionIndex,
  workspaceService: IWorkspaceService,
  configService: IConfigService,
  modelService: IModelService,
  modelResolver: IModelCatalog,
  providerDiscovery: IProviderDiscoveryService,
  modelsDevImport: IModelsDevImportService,
  providerService: IProviderService,
  oauthService: IOAuthService,
  authSummaryService: IAuthSummaryService,
  flagService: IFlagService,
  pluginService: IPluginService,
  capabilityService: ICapabilityService,
  hostFolderBrowser: IHostFolderBrowser,
  bootstrapService: IBootstrapService,
  fileService: IFileService,
  workspaceInstanceManager: IWorkspaceInstanceManager,
  sessionManager: ISessionManager,
  sessionMetadata: ISessionMetadata,
  sessionSkillCatalog: ISessionSkillCatalog,
  sessionTitleService: ISessionTitleService,
  agentPromptService: IAgentPromptChannel,
  agentLoopService: IAgentLoopService,
  agentPermissionModeService: IAgentPermissionModeService,
  agentCommandService: IAgentCommandService,
  agentRuntimeBindingService: IAgentRuntimeBindingService,
  agentContextMemoryService: IAgentContextMemoryService,
  agentTokenCountingService: ISessionTokenCountingService,
  sessionActivityView: ISessionActivityView,
  agentShellCommandService: IAgentShellCommandService,
  agentProfileService: IAgentProfileService,
  agentUsageService: ISessionUsageService,
  agentPlanService: IAgentPlanService,
  agentTaskService: IAgentTaskService,
  agentMcpService: IAgentMcpService,
  agentFullCompactionService: IAgentFullCompactionService,
  mcpManagementService: IMcpManagementService,
};

export { IEventService };
