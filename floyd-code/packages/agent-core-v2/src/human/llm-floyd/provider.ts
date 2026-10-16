import { createProvider } from '#/llm/provider/definition';
import { anthropicBetaBase } from '#/llm/requester/bases/anthropic/requester';
import { openAIBase } from '#/llm/requester/bases/openai/requester';
import { openAIResponsesBase } from '#/llm/requester/bases/openai-responses/requester';

import { floydAnthropicTrait, floydConnection, floydOpenAITrait } from './trait';
import { classifyFloydQuotaError } from './errors';
import { floydMediaContribution } from './media';

export const floydProvider = createProvider({
  id: 'floyd',
  protocols: {
    openai: {
      base: openAIBase,
      trait: floydOpenAITrait,
      connection: floydConnection,
      classifyError: classifyFloydQuotaError,
    },
    anthropic: {
      base: anthropicBetaBase,
      trait: floydAnthropicTrait,
      connection: floydConnection,
      classifyError: classifyFloydQuotaError,
    },
    openai_responses: {
      base: openAIResponsesBase,
      connection: floydConnection,
      classifyError: classifyFloydQuotaError,
    },
  },
  media: floydMediaContribution,
});
