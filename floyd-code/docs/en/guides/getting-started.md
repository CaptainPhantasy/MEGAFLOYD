# Getting started

## What is Floyd Code CLI

Floyd Code CLI is an AI agent that runs in the terminal, helping you carry out software development tasks and day-to-day terminal operations — reading and modifying code, running shell commands, searching files, fetching web pages, and autonomously planning and adjusting its next steps based on feedback as it works.

It fits scenarios such as:

- **Writing and modifying code**: implementing new features, fixing bugs, completing refactors
- **Understanding a project**: exploring an unfamiliar codebase and answering questions about architecture and implementation
- **Automating tasks**: batch-processing files, running builds and tests, chaining multiple scripts together

The CLI is written in TypeScript and runs on Node.js.

## Installation

Floyd Code CLI runs from a checkout of this repository: no prebuilt binary and no npm package is published yet. Node.js 24.15.0 or later and pnpm 10.33.0 are required.

::: tip Before you install
Floyd Code CLI is a fully interactive TUI application. For the best visual experience, run it in a terminal with true-color and ligature support, such as [Kitty](https://sw.kovidgoyal.net/kitty/) or [Ghostty](https://ghostty.org/).
:::

Check both versions first:

```sh
node --version
pnpm --version
```

Clone the repository and install the workspace:

```sh
git clone https://github.com/CaptainPhantasy/f7.git
cd f7
pnpm install
```

> On Windows, install [Git for Windows](https://gitforwindows.org/) before first launch. Floyd Code CLI uses the bundled Git Bash as its shell environment; if Git Bash is installed in a custom location, set `FLOYD_SHELL_PATH` to the absolute path of `bash.exe`.

The executable is `f7`. Inside the checkout, `pnpm dev:cli` starts the same CLI, so run that wherever the examples on this page show `f7`.

## First launch

Move into your project directory and run `f7` to start the interactive UI:

```sh
cd your-project
f7
```

To run a single instruction without entering the interactive UI, use `-p`:

```sh
f7 -p "Take a look at this project's directory structure"
```

To resume the previous session, add `-c`:

```sh
f7 -c
```

Floyd Code CLI needs no account — it works with whichever model provider you point it at. In the interactive UI, `/provider` walks you through it: pick a known third-party provider, paste its API key, then choose the default model. You can equally write the provider into `~/.floyd-code/config.toml` yourself:

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

Any OpenAI-compatible endpoint works, as do the Anthropic API and Google's Gemini API. [Providers and models](../configuration/providers.md) covers each type, and how to import a whole provider from a catalog or registry instead of typing the fields by hand.

::: tip Where credentials live
Keys are read from `config.toml`, not from the shell environment — the one exception is `api_key_env`, which points at a variable name you choose. See [Environment variables](../configuration/env-vars.md), [Configuration files](../configuration/config-files.md), and [Configuration overrides](../configuration/overrides.md).
:::

## Your first conversation

With a provider configured, describe a task in natural language. A good starting point is to let Floyd Code CLI familiarize itself with the project:

```
Take a look at this project's directory structure and briefly describe what each directory is for.
```

Floyd Code CLI automatically calls file-reading, search, and other tools to browse the relevant content before responding. Read-only operations are executed automatically by default without requiring confirmation. For operations that modify files or run shell commands, it asks for your confirmation before proceeding.

You can also describe a more concrete task directly:

```
Add a function in src/utils that converts any string to kebab-case, and add a unit test for it.
```

Floyd Code CLI plans the steps, modifies the code, runs the tests, and tells you what it did at each step.

::: tip Not sure what to do? Type `/help`
Type `/help` at any time to open the built-in command and keyboard shortcut panel. Use `↑`/`↓` to browse and `Esc` to close. To exit, type `/exit`, press `Ctrl-C` twice, or press `Ctrl-D` with the input box empty.
:::

## Common commands and keyboard shortcuts

For a first-time user, the following is all you need to know:

**Session commands**

| Command | Description |
| --- | --- |
| `/new` | Start a new session, clearing the current context |
| `/sessions` | Browse session history and choose one to resume |
| `/model` | Switch the current model |
| `/compact` | Manually compress the context to free up tokens |
| `/fork` | Fork the current session into an independent copy with full history (you stay in the current session) |

**Most-used keyboard shortcuts**

| Shortcut | Description |
| --- | --- |
| `Esc` | Interrupt streaming output / close a popup |
| `Ctrl-C` | Interrupt output; press twice while idle to exit |
| `Shift-Tab` | Toggle Plan mode |
| `Ctrl-S` | Inject a message mid-stream without waiting for the current response to finish |
| `Ctrl-O` | Collapse / expand tool output and compaction summaries |

For the full list, type `/help` or visit [Slash commands reference](../reference/slash-commands.md) and [Keyboard shortcuts](../reference/keyboard.md).

## Where data is stored

Floyd Code CLI stores its local data under `~/.floyd-code/` by default — config files, session records, logs, and the update cache. To move it elsewhere, point to a new path via the `FLOYD_CODE_HOME` environment variable. For the full directory layout, see [Data locations](../configuration/data-locations.md) and [Environment variables](../configuration/env-vars.md).

## Upgrade and uninstall

After installation, verify that the CLI starts:

```sh
f7 --version
```

**Upgrade**: in a source checkout, `git pull` and run `pnpm install` again. `f7 upgrade` covers packaged installs: it checks for the latest version and presents update options.

**Uninstall**: remove the checkout directory.

## Next steps

- [Interaction and input](./interaction.md) — input box operations, approval flow, Plan mode, and Ask When Needed mode explained
- [Sessions and context](./sessions.md) — resuming sessions, compressing context, exporting sessions
- [Common use cases](./use-cases.md) — prompt examples for typical tasks
