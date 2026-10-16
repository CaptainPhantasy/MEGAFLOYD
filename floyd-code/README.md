# Floyd Code CLI

[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE) <br>
[Documentation](docs/en/index.md) · [Issues](https://github.com/CaptainPhantasy/f7/issues) · [中文](README.zh-CN.md)

![Demo of using Floyd Code](./docs/media/intro.gif)

## What is Floyd Code CLI

Floyd Code CLI is an AI coding agent that runs in your terminal — it can read and edit code, run shell commands, search files, fetch web pages, and choose the next step based on the feedback it receives. It runs against any model provider you configure, so you can point it at the API you already use.

## Install

No prebuilt binary or npm package is published for Floyd Code yet — install it by building from source. Node.js 24.15.0 or later and pnpm 10.33.0 are required.

```sh
git clone https://github.com/CaptainPhantasy/f7.git
cd f7
pnpm install
pnpm dev:cli
```

> On Windows, install [Git for Windows](https://gitforwindows.org/) before first launch because Floyd Code CLI uses Git Bash as its shell environment. If Git Bash is installed in a custom location, set `FLOYD_SHELL_PATH` to the absolute path of `bash.exe`.

The CLI executable is `f7`. In a source checkout use `pnpm dev:cli` wherever the examples below use `f7`; see [Develop](#develop) for the full set of build commands.

For provider configuration and usage, see the [documentation](docs/en/index.md).

## Quick Start

Open a project and start the interactive UI:

```sh
cd your-project
f7
```

On first launch, point Floyd Code CLI at the provider you want to use by adding a `[providers.*]` entry to `~/.floyd-code/config.toml`, or run `/provider` inside the TUI to add one interactively. Then try your first task:

```
Take a look at this project and explain its main directories.
```

## Key Features

- **Blazing-fast startup.** The TUI is ready in milliseconds, so starting a session never feels heavy.
- **Purpose-built TUI.** A carefully tuned interface, optimized end to end for long, focused agent sessions.
- **Video input.** Drop a screen recording or demo clip into the chat and let the agent watch what is hard to describe in words — turn a reference clip into a LUT, a long video into a short, a screen recording into working code, and more.
- **AI-native MCP configuration.** Add, edit, and authenticate Model Context Protocol servers conversationally with `/mcp-config`, without hand-editing JSON.
- **Rich plugin ecosystem.** Install skills, MCP servers, and data sources from the marketplace or any GitHub repo, with each install's trust level surfaced up front.
- **Subagents for focused, parallel work.** Dispatch built-in `coder`, `explore`, and `plan` subagents in isolated contexts while keeping the main conversation clean.
- **Lifecycle hooks.** Run local commands at key points to gate risky tool calls, audit decisions, trigger desktop notifications, or connect to your own automation.
- **Editor & IDE integration (ACP).** Drive a Floyd Code CLI session straight from Zed, JetBrains, or any [Agent Client Protocol](https://agentclientprotocol.com/) client with `f7 acp`.

## Use it in your editor (ACP)

Floyd Code CLI speaks the [Agent Client Protocol](https://agentclientprotocol.com/), so ACP-compatible editors and IDEs (Zed, JetBrains, …) can drive a session over stdio. Point your editor at the `f7 acp` subcommand.

For Zed, add this to `~/.config/zed/settings.json`:

```json
{
  "agent_servers": {
    "Floyd Code CLI": {
      "type": "custom",
      "command": "f7",
      "args": ["acp"],
      "env": {}
    }
  }
}
```

Then open a new conversation in Zed's Agent panel. See [Using in IDEs](docs/en/guides/ides.md) for JetBrains setup and troubleshooting, and the [`f7 acp` reference](docs/en/reference/floyd-acp.md) for the full capability matrix.

## Docs

- [Getting Started](docs/en/guides/getting-started.md)
- [Interaction and approvals](docs/en/guides/interaction.md)
- [Sessions](docs/en/guides/sessions.md)
- [Using in IDEs (ACP)](docs/en/guides/ides.md)
- [Configuration](docs/en/configuration/config-files.md)
- [Command reference](docs/en/reference/floyd-command.md)

## Develop

Requirements: Node.js ≥ 24.15.0, pnpm 10.33.0.

```sh
git clone https://github.com/CaptainPhantasy/f7.git
cd f7
pnpm install
```

```sh
pnpm dev:cli    # run the CLI in dev mode
pnpm test       # run tests
pnpm typecheck  # TypeScript check
pnpm lint       # oxlint
pnpm build      # build all packages
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full contribution guide.

## Community

- [Issues](https://github.com/CaptainPhantasy/f7/issues)
- For security vulnerabilities, see [SECURITY.md](SECURITY.md).

## Acknowledgements

Our TUI is built on top of [`pi-tui`](https://github.com/earendil-works/pi-mono/tree/main/packages/tui). We thank the authors of `pi-tui` for their valuable work.

## License

Released under the [MIT License](LICENSE).
