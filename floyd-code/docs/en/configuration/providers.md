# Providers and models

Floyd Code CLI connects to whichever LLM platforms you choose, several at a time — no account with us is required. The usual path is an OpenAI-compatible endpoint: your own gateway, a hosted API, or a server on your own machine. An Anthropic or Google API key works the same way. Each provider corresponds to a specific API protocol; models are declared on top of providers with their own name, context length, and capabilities. This page explains how to configure each type of provider in `config.toml`.

## Supported provider types

The `type` field in the `providers` table determines which protocol implementation to use:

| Type | Protocol | Typical use |
| --- | --- | --- |
| [`floyd`](#floyd) | OpenAI-compatible | Legacy AI managed service; supports video upload |
| [`anthropic`](#anthropic) | Anthropic Messages | Claude model family |
| [`openai`](#openai) | OpenAI Chat Completions | OpenAI and compatible services, DeepSeek, Qwen, etc. |
| [`openai_responses`](#openai_responses) | OpenAI Responses API | OpenAI's newer Responses interface |
| [`google-genai`](#google-genai) | Google GenAI | Gemini API |
| [`vertexai`](#vertexai) | Google GenAI on Vertex | Google Cloud Vertex AI |

All providers communicate with models in streaming mode by default. Capabilities such as thinking, vision, and tool use are matched automatically by model name prefix, so you typically do not need to declare them manually.

**Credential priority**: `api_key` or `api_key_env` (mutually exclusive alternatives — set exactly one) > `[providers.<name>.env]` sub-table key (only when neither is present) > if all are absent, startup fails with an error. Except for the explicitly declared `api_key_env`, the CLI does not fall back to shell environment variables for credentials. See [Config overrides: provider credentials](./overrides.md#provider-credentials).

## `/provider` — interactive provider management

Prefer not to edit TOML by hand? Type `/provider` in the TUI to open the **provider manager**, where you can interactively add or remove providers.

![The /provider provider manager](../../media/provider-manager.jpg)

The manager displays providers as a list of entries grouped by source. Navigation:

- ↑/↓ to move the cursor, ←/→ to page
- `d` to delete the current provider (with `[y/N]` confirmation)
- Press Enter on the `[ Add New Platform ]` row to add a new provider

Two paths when adding:

- **Known third-party provider**: fetches the model catalog from [models.dev](https://models.dev/), select a provider → enter an API key → select a default model. Vendors whose protocol the catalog does not declare (e.g. xai, openrouter, and other vendor-specific SDKs) are imported as OpenAI-compatible with a "guessed" note; when the catalog provides no usable endpoint, a base URL prompt appears first; proprietary protocols (Amazon Bedrock, Cohere) and unrecognized explicit protocols are refused. Deprecated and alpha-status models are excluded from the import list. If the public catalog is unreachable, the CLI falls back to a built-in snapshot of the catalog, so the import still works offline or in blocked networks
- **Custom registry (api.json)**: paste a custom registry URL and, for private registries, a Bearer token; the CLI automatically creates the `providers` / `models` entries. When a registry entry declares the `env` field (the name of the environment variable holding the API key), the CLI prints it as a hint — set `api_key_env` in `config.toml` yourself to use it. The binding is never automatic: the registry chooses both the variable name and the endpoint the credential is sent to, so it must not decide which of your secrets is read. For private registries the Bearer token itself is still stored as `source.apiKey` so the registry can be refetched on refresh. On later startup, providers from the same registry URL are refreshed together, so upstream provider additions, removals, and model metadata changes are synced.

::: warning
Floyd Code OAuth managed accounts logged in via `/login` do not appear in `/provider`. Use `/login` and `/logout` to manage them.
:::

The same operations are also available in non-interactive environments via the shell command: [`f7 provider`](../reference/floyd-command.md#f7-provider).

## `floyd`

The OpenAI-compatible protocol used by the Legacy AI managed service, with one extra capability: video upload. That capability is the only thing it adds over [`openai`](#openai), so prefer that type unless your endpoint needs video upload.

- Default `base_url`: none — this build ships no managed endpoint, so `base_url` (or `FLOYD_BASE_URL`) is required
- Credential key names: `FLOYD_API_KEY`, `FLOYD_BASE_URL`
- Additional capability: supports video upload

```toml
[providers.floyd]
type = "floyd"
base_url = "https://your-gateway.example/v1"
api_key = "YOUR_API_KEY"
```

> A managed login (`/login`) writes `base_url` and credentials for you, but only against a deployment whose OAuth host you configured first (`FLOYD_CODE_OAUTH_HOST`); this build ships none. Without one, set both by hand here.

## `anthropic`

For connecting to the Claude API. Standard Claude models automatically enable vision, tool use, and Thinking (where supported); custom or uncovered models need `capabilities` declared explicitly on `[models.<alias>]`.

- Default `base_url`: follows Anthropic SDK default
- Credential key names: `ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`
- Default `max_tokens`: inferred per model. To override, set `max_output_size` on the model alias

```toml
[providers.anthropic]
type = "anthropic"
api_key = "sk-ant-xxxxx"

[models."claude-opus-4-7"]
provider = "anthropic"
model = "claude-opus-4-7"
max_context_size = 200000
# max_output_size = 32000  # optional; omit to use the model-inferred default
```

## `openai`

For connecting to the OpenAI Chat Completions protocol, as well as any third-party service compatible with that protocol (override `base_url` as needed).

Third-party reasoning models (DeepSeek, Qwen, One API, etc.) work out of the box: the CLI automatically handles the `reasoning_content` field and `reasoning_effort` injection. If your gateway returns reasoning content under a non-standard field name, set `reasoning_key` on the model alias to override.

- Default `base_url`: `https://api.openai.com/v1`
- Credential key names: `OPENAI_API_KEY`, `OPENAI_BASE_URL`

```toml
[providers.openai]
type = "openai"
base_url = "https://api.openai.com/v1"
api_key = "sk-xxxxx"
```

Any other service speaking this protocol is the same shape — point `base_url` at its endpoint and declare the model ids it serves:

```toml
default_model = "my-gateway/gpt-4.1"

[providers.my-gateway]
type = "openai"
base_url = "https://your-gateway.example/v1"
api_key = "YOUR_API_KEY"

[models."my-gateway/gpt-4.1"]
provider = "my-gateway"
model = "gpt-4.1"
max_context_size = 1047576
```

## `openai_responses`

Corresponds to OpenAI's newer Responses API, always operating in streaming mode. Configuration is the same as `openai`.

- Default `base_url`: `https://api.openai.com/v1`
- Credential key names: `OPENAI_API_KEY`, `OPENAI_BASE_URL`

```toml
[providers.openai-responses]
type = "openai_responses"
base_url = "https://api.openai.com/v1"
api_key = "sk-xxxxx"
```

## `google-genai`

For connecting directly to the Google Gemini API. Thinking, vision, and multimodal capabilities are auto-detected by model name.

- Credential key name: `GOOGLE_API_KEY`

```toml
[providers.gemini]
type = "google-genai"
api_key = "xxxxx"
```

To route through a Gemini-compatible proxy or gateway, set `base_url` (or the `GOOGLE_GEMINI_BASE_URL` env var); when omitted, the SDK default `https://generativelanguage.googleapis.com` is used.

> Give the **host root only**. The Google GenAI SDK appends the API version and path itself (e.g. `/v1beta/models/<model>:generateContent`), so a trailing `/v1beta` would produce a doubled `/v1beta/v1beta/…`.

```toml
[providers.gemini]
type = "google-genai"
api_key = "xxxxx"
base_url = "https://your-gateway.example"
```

## `vertexai`

Shares the same implementation as `google-genai`; setting `type = "vertexai"` switches to the Vertex AI access path.

Authentication follows the standard Google Cloud ADC flow (`gcloud auth application-default login` or a `GOOGLE_APPLICATION_CREDENTIALS` service account JSON); this part is unrelated to Floyd Code. **The project ID and region must be written in the `[providers.vertexai.env]` sub-table**. Simply `export GOOGLE_CLOUD_PROJECT` in the shell will not be read by the CLI.

```toml
[providers.vertexai]
type = "vertexai"

[providers.vertexai.env]
GOOGLE_CLOUD_PROJECT = "my-gcp-project"
GOOGLE_CLOUD_LOCATION = "us-central1"
```

```sh
gcloud auth application-default login   # one-time authentication
f7
```

To route Vertex requests through a custom (e.g. proxied) endpoint, set `base_url` (or the `GOOGLE_VERTEX_BASE_URL` env var); when omitted, the SDK default regional `*-aiplatform.googleapis.com` host is used. As with `google-genai`, give the host root only. The SDK appends `/v1beta1/publishers/google/models/…` itself.

## OAuth and credential injection

The Legacy AI managed service authenticates with OAuth rather than a static API key: once a managed account is authenticated, the built-in toolchain writes and refreshes the credential, so nothing in `config.toml` needs to be set by hand — provided you first pointed the client at a deployment (`FLOYD_CODE_OAUTH_HOST`), since this build ships no managed host. Providers you configure yourself always use `api_key` or `api_key_env`.

## Next steps

- [Configuration files](./config-files.md) — full field reference for the `providers` and `models` tables
- [Config overrides](./overrides.md) — credential resolution priority rules for providers
- [Environment variables](./env-vars.md) — credential key names per provider type
