import type { BearerTokenProvider } from '@legacy-ai/floyd-code-oauth';

import { createOAuthCredentialProvider } from '#/credentials/credentials';
import type { LlmCredentialProvider } from '#/llm/requester/requester';

export function createFloydOAuthCredentialProvider(tokens: BearerTokenProvider): LlmCredentialProvider {
  return createOAuthCredentialProvider((options) => tokens.getAccessToken(options));
}
