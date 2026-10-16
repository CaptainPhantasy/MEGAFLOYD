---
name: check-floyd-code-docs
description: Answer questions about the Floyd Code product using the official documentation — CLI usage, configuration, slash commands, features, membership and quota, API onboarding, third-party tool setup, and error codes. Use when the user asks how Floyd Code works, how to set something up, or what a Floyd Code error message means.
---

# Check Floyd Code docs (check-floyd-code-docs)

Answer Floyd Code **product** questions from the documentation that ships with Floyd Code, not from memory. This skill covers product usage ("how do I configure a provider", "what does this error mean", "how does quota work"); it is not for developing the Floyd Code repository itself.

## The single source of truth

The English documentation lives in the Floyd Code repository, under `docs/en/`:

```
docs/en/
```

Read pages from that tree with **Read** — and **Grep** to locate a term — before answering. All page paths below are relative to `docs/en/`. This project has no hosted documentation site, so never fetch a documentation URL: read the file from the checkout instead.

## Which page to read for which question

| Question topic | Page (relative to `docs/en/`) |
| --- | --- |
| What Floyd Code is; install, first session, upgrade | `guides/getting-started.md` |
| Configuring a provider, Base URL / API key, choosing a model, comparing providers | `configuration/providers.md` |
| `config.toml` fields, providers/models, environment variables, data locations, config overrides | `configuration/` — `config-files.md`, `providers.md`, `env-vars.md`, `data-locations.md`, `overrides.md` |
| Skills, MCP, hooks, plugins, themes, agents/sub-agents, Floyd Datasource | `customization/` — `skills.md`, `mcp.md`, `hooks.md`, `plugins.md`, `themes.md`, `agents.md`, `datasource.md` |
| Sessions and context, interaction and input, use cases, IDEs | `guides/` — `sessions.md`, `interaction.md`, `use-cases.md`, `ides.md` |
| The browser UI, remote control, migrating from another tool | `guides/` — `web.md`, `remote-control.md`, `migration.md` |
| Slash commands, keyboard shortcuts, builtin tools, `f7` command flags, ACP | `reference/` — `slash-commands.md`, `keyboard.md`, `tools.md`, `floyd-command.md`, `floyd-acp.md` |
| Membership, plan quota and rate limits, usage reporting | `reference/slash-commands.md` (`/usage`), `reference/server-api.md` (`GET /api/v1/oauth/usage`), `configuration/config-files.md` |
| Error codes and their meaning | `reference/server-api.md` |
| Importing instructions, skills, or MCP settings from Claude Code or Codex | `reference/slash-commands.md` (`/import-from-cc-codex`), `customization/agents.md` |
| Product news and recent changes, CLI changelog | `release-notes/changelog.md` |

If no row fits the question, start at `docs/en/index.md` and follow the links from there.

## How to answer

1. Pick the page from the table above.
2. **Read the page before answering** — answer strictly from its content, never from memory.
3. Cite the pages you used at the end of the answer, as repository-relative paths (`docs/en/...`).
4. If the page cannot be read, the checkout is not available, or the docs do not cover the question, say so plainly: answer from what you already know, name the `docs/en/` page that would cover it, and mark which parts you could not verify. **Never invent config keys, command names, model IDs, or product behaviors.**
