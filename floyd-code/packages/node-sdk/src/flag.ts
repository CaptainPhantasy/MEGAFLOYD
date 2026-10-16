import type {
  FlagDefinitionInput,
  FlagId,
} from '@legacy-ai/agent-core-v2/app/flag/flagRegistry';

export type {
  ExperimentalFeatureState,
  ExperimentalFlagMap,
  ExperimentalFlagSource,
} from '@legacy-ai/agent-core-v2/app/flag/flag';
export type {
  FlagDefinitionInput,
  FlagId,
  FlagSurface,
} from '@legacy-ai/agent-core-v2/app/flag/flagRegistry';

export type FlagDefinition = FlagDefinitionInput & { readonly id: FlagId };
