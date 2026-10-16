# Using Floyd Code CLI in IDEs

Floyd Code CLI supports integration into IDEs via the [Agent Client Protocol (ACP)](https://agentclientprotocol.com/), letting you use AI-assisted coding directly inside your editor.

## Prerequisites

Before configuring your IDE, make sure Floyd Code CLI is installed and that you have configured at least one provider with a default model.

The ACP server is exposed as the `f7 acp` subcommand. The IDE launches it as a child process and communicates over stdin/stdout using JSON-RPC. Each time the IDE creates a session, the CLI reuses its existing provider configuration — there is nothing to set up again.

::: tip Path note
Child processes launched from an IDE GUI on macOS typically do **not** inherit the terminal shell's `PATH`. If `f7` is not in a system directory like `/usr/local/bin`, use the absolute path in your IDE configuration. Run `which f7` in a terminal to find the active path.
:::

## Using Floyd Code CLI in Zed

[Zed](https://zed.dev/) is a modern editor with native ACP support.

Add the following to Zed's config file at `~/.config/zed/settings.json`:

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

Configuration fields:

- `type`: fixed value `"custom"`
- `command`: path to the Floyd Code CLI executable. If `f7` is not on `PATH`, use the full path (e.g. `/Users/you/.local/bin/f7`).
- `args`: startup arguments. The `acp` subcommand switches the CLI into ACP mode.
- `env`: additional environment variables; usually leave this empty. Zed injects a default environment automatically.

After saving, open a new conversation in Zed's Agent panel and it will launch a `Floyd Code CLI` ACP subprocess using the configuration above. MCP servers declared in Zed's `agent_servers` section are also forwarded to the CLI side via the ACP protocol.

## Using Floyd Code CLI in JetBrains IDEs

JetBrains IDEs (IntelliJ IDEA, PyCharm, WebStorm, etc.) support ACP through the AI chat plugin.

If you do not have a JetBrains AI subscription, you can enable `llm.enable.mock.response` in the Registry to access the AI chat panel in ACP-only scenarios. Press Shift twice and search for "Registry" to open it.

In the AI chat panel menu, click **Configure ACP agents** and add the following configuration:

```json
{
  "agent_servers": {
    "Floyd Code CLI": {
      "command": "~/.local/bin/f7",
      "args": ["acp"],
      "env": {}
    }
  }
}
```

JetBrains is strict about the `command` field — always use an **absolute path**, which you can get by running `which f7` in a terminal. After saving, `Floyd Code CLI` will appear in the AI chat's agent selector.

## Using Floyd Code CLI in Paseo

[Paseo](https://paseo.sh/) is a self-hosted orchestrator that runs and supervises agent CLIs from your desktop, web, and mobile. It connects to Floyd Code CLI over ACP, the same way an IDE does.

Pick **Floyd Code CLI** from Paseo's built-in ACP provider catalog, or add a custom provider in `~/.paseo/config.json`:

```json
{
  "agents": {
    "providers": {
      "floyd": {
        "extends": "acp",
        "label": "Floyd Code CLI",
        "command": ["f7", "acp"]
      }
    }
  }
}
```

Paseo's generic ACP adapter does not run the CLI's own setup, so configure a provider and a default model first (see [Prerequisites](#prerequisites)) — otherwise session creation fails with `Authentication required`.

## Troubleshooting

- **Session disconnects immediately / IDE shows "agent exited"**: usually a wrong `command` path or no usable provider credentials. Run `f7 acp` in a terminal first to verify — if it blocks waiting for stdin, the CLI itself is fine and the problem is in the IDE configuration; if it exits immediately with an error, follow the error message (most commonly the configured provider has no valid API key).
- **IDE shows "auth required"**: the CLI has no usable credentials. Exit the IDE and check the provider configuration with `f7 provider list` in a terminal, then restart the IDE.
- **MCP tools not visible**: check the [`f7 acp` reference](../reference/floyd-acp.md) capability table to confirm that the MCP transport type configured in your IDE is supported. The Floyd Code CLI ACP server currently supports `http`, `stdio`, and `sse` transports; `acp` transport MCP servers are silently dropped and a warning is written to the log.

## Next steps

- [`f7 acp` reference](../reference/floyd-acp.md) — ACP capability matrix and method coverage details
- [`f7` command reference](../reference/floyd-command.md) — full subcommand list
