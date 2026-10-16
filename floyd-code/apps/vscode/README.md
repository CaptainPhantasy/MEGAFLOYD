# Floyd Code

AI coding assistant for VS Code, built for long-context workflows and complex coding tasks.

## Features

- **Works alongside you**: Floyd autonomously explores your codebase, reads and writes code, and runs terminal commands with your permission
- **Thinking controls**: Toggle reasoning or choose a model-supported thinking effort
- **Provider-aware models**: Distinguish and select same-named models across configured providers
- **Native editor integration**: Review AI-proposed changes directly in VS Code's diff viewer
- **MCP support**: Extend capabilities with Model Context Protocol servers
- **Slash commands**: Quick actions like `/init` to analyze your project and `/compact` to manage context

## Install

Floyd Code requires VS Code 1.100.0 or later.

1. Build the extension from a source checkout of this repository — it is not published to a marketplace yet: run `pnpm install` at the repository root, then `pnpm --filter floyd-code run package:platform`, and install the resulting `.vsix` with `code --install-extension <file>.vsix`
2. Open a folder in VS Code
3. Click the Floyd icon in the Activity Bar
4. Configure a provider in the shared `config.toml`; no account or login is required

The extension runs the Floyd Code Node SDK in the VS Code Extension Host. When
the extension and the Floyd Code terminal app resolve to the same
`FLOYD_CODE_HOME`, they share `config.toml`, MCP configuration, login state, and
sessions. The system-level `FLOYD_CODE_HOME` environment variable is supported;
there is no separate VS Code setting for it. Do not run the same session from
both applications at the same time, because cross-process session locking is
not guaranteed.

After upgrading from version 0.5.x, the extension prompts before migrating any
legacy data it finds. Migration copies or merges data into the current Floyd Code
home and does not delete the legacy source. Legacy Floyd Code OAuth and MCP OAuth
credentials are not copied, so those connections must be authorized again.
See [the changelog](CHANGELOG.md) for the full compatibility notes.

## Docs

Floyd Code documentation lives in this repository, under `docs/en/` — see [the project repository](https://github.com/CaptainPhantasy/f7) for the documentation tree and its guides.

## License

[Apache-2.0](LICENSE)
