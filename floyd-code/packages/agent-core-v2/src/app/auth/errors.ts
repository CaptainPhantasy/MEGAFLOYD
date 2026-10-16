import { registerErrorDomain, type ErrorDomain } from '#/_base/errors/codes';

export const AuthErrors = {
  codes: {
    AUTH_LOGIN_REQUIRED: 'auth.login_required',
    AUTH_PROVISIONING_REQUIRED: 'auth.provisioning_required',
    AUTH_TOKEN_MISSING: 'auth.token_missing',
    AUTH_TOKEN_UNAUTHORIZED: 'auth.token_unauthorized',
    AUTH_MODEL_NOT_RESOLVED: 'auth.model_not_resolved',
  },
  info: {
    'auth.login_required': {
      title: 'Login required',
      retryable: false,
      public: true,
      action: 'Configure a provider in config.toml or via /provider; use /login only for an OAuth provider.',
    },
    'auth.provisioning_required': {
      title: 'Provider provisioning required',
      retryable: false,
      public: true,
      action: 'No provider is configured. Add one in config.toml or via /provider; use /login only for an OAuth provider.',
    },
    'auth.token_missing': {
      title: 'Provider credential missing',
      retryable: false,
      public: true,
      action: 'Set an API key for this provider in config.toml or via /provider; use /login only for an OAuth provider.',
    },
    'auth.token_unauthorized': {
      title: 'Provider credential unauthorized',
      retryable: false,
      public: true,
      action: 'Re-authenticate with the OAuth provider.',
    },
    'auth.model_not_resolved': {
      title: 'Model not resolved',
      retryable: false,
      public: true,
      action: 'Set a default model or configure the requested model alias.',
    },
  },
} as const satisfies ErrorDomain;

registerErrorDomain(AuthErrors);
