# @legacy-ai/floyd-code

> The Starting Point for Next-Gen Agents

[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

## What is Floyd Code CLI

Floyd Code CLI is an AI coding agent that runs in your terminal. It can read and edit code, run shell commands, search files, fetch web pages, and choose the next step based on the feedback it receives. It runs against any model provider you configure, so you can point it at the API you already use.

## Install

No prebuilt binary or npm package is published for Floyd Code yet — install it by building from source. Node.js 24.15.0 or later and pnpm 10.33.0 are required.

```sh
git clone https://github.com/CaptainPhantasy/f7.git
cd f7
pnpm install
pnpm dev:cli
```

> On Windows, install [Git for Windows](https://gitforwindows.org/) before first launch because Floyd Code CLI uses Git Bash as its shell environment. If Git Bash is installed in a custom location, set `FLOYD_SHELL_PATH` to the absolute path of `bash.exe`.

The CLI executable is `f7`. In a source checkout use `pnpm dev:cli` wherever the examples below use `f7`; the full set of build commands is in the [main repository README](https://github.com/CaptainPhantasy/f7#develop).

## Quick Start

Open a project and start the interactive UI:

```sh
cd your-project
f7
```

On first launch, point Floyd Code CLI at the provider you want to use by adding a `[providers.*]` entry to `~/.floyd-code/config.toml`, or run `/provider` inside the TUI to add one interactively. Then try a first task:

```
Take a look at this project and explain the main directories.
```

## Key Features

- **Blazing-fast startup.** The TUI is ready in milliseconds, so opening a session never feels heavy.
- **Polished TUI.** A carefully tuned interface designed for long, focused agent sessions.
- **Video input.** Drop a screen recording or demo clip into the chat — let the agent watch instead of typing out what's hard to describe in words.
- **AI-native MCP configuration.** Add, edit, and authenticate Model Context Protocol servers conversationally via `/mcp-config` — no hand-editing JSON.
- **Subagents for focused, parallel work.** Dispatch built-in `coder`, `explore`, and `plan` subagents in isolated context windows; the main conversation stays clean.
- **Lifecycle hooks.** Run local commands at key points — gate risky tool calls, audit decisions, fire desktop notifications, wire into your own automation.

## Documentation

- Full docs: https://github.com/CaptainPhantasy/f7/blob/main/docs/en/index.md
- 中文文档: https://github.com/CaptainPhantasy/f7/blob/main/docs/zh/index.md
- Getting Started: https://github.com/CaptainPhantasy/f7/blob/main/docs/en/guides/getting-started.md

## Repository & Issues

- Source: https://github.com/CaptainPhantasy/f7
- Issues: https://github.com/CaptainPhantasy/f7/issues
- Security: see SECURITY.md in the main repository

## License

MIT
