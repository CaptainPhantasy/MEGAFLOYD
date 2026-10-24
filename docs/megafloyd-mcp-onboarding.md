# MCP gateway onboarding from a workstation harness

Prepared 2026-10-24 from the canonical Mac MEGAFLOYD configuration, the installed gateway tool schemas, and `/srv/organized/SERVER-MODEL.md` on inference.

A harness does not need to run on inference. Give its MCP client the existing SSH stdio connection, complete the normal MCP handshake, discover the gateway's tools, and call the required backend through the gateway. Existing mesh and SSH access are sufficient; there is no gateway registration, application login, or onboarding token to obtain.

1. **Use the SSH access already provided to the workstation.**

   The verified inference mesh address is `100.78.109.105`. The current Mac configuration uses `floyd@100.78.109.105`. Another member uses the SSH account and access assigned to that member; do not copy another member's private keys or application state. Retain normal SSH host-key verification.

2. **Configure an MCP stdio server in the harness.**

   The current canonical Mac entry is in `/Users/floyd/.megafloyd/config/floyd.json`. Its transport settings are:

   ```json
   {
     "mcp": {
       "mcp-gateway": {
         "type": "stdio",
         "command": "/usr/bin/ssh",
         "args": [
           "-T",
           "-o", "BatchMode=yes",
           "-o", "StrictHostKeyChecking=yes",
           "-o", "ConnectTimeout=8",
           "floyd@100.78.109.105",
           "exec env MCP_GATEWAY_ROOT=/srv/organized/services/MCP_SERVER_ROOM /home/linuxbrew/.linuxbrew/bin/node /srv/organized/services/MCP_SERVER_ROOM/mcp-gateway/dist/index.js"
         ],
         "timeout": 30
       }
     }
   }
   ```

   This is the Go MEGAFLOYD configuration shape. Other harnesses may use `mcpServers`, another configuration file, or a settings screen. Preserve the command and argument boundaries when translating it into that harness's supported stdio configuration. `timeout` above is MEGAFLOYD's configuration field; it is separate from the gateway's per-call `timeout_ms` argument.

   The harness launches SSH, and SSH runs the installed Node gateway on inference. MCP messages travel through stdin and stdout. Do not allocate a terminal or inject banners into stdout. Keep the client connection open for the session. This procedure uses the existing gateway installation; it does not require a new HTTP gateway or a tunnel service.

3. **Let the MCP client initialize the connection and discover tools.**

   A normal MCP client sends `initialize` with its supported protocol version and capabilities, accepts the negotiated response, sends `notifications/initialized`, then requests `tools/list`. SDK methods may call the last operation `list_tools` or `listTools`.

   Successful SSH `hostname` output proves SSH access. A successful MCP initialization and `tools/list` prove gateway transport and discovery. Neither result alone proves that the harness's model can select and execute those tools.

4. **Use the gateway's discovery sequence, then execute one selected tool.**

   These are the gateway tool names and their argument objects. The MCP client wraps each as a `tools/call` request. Harnesses may display prefixed names; canonical Go MEGAFLOYD uses names such as `mcp_mcp-gateway_health_check`.

   | Gateway tool | Example arguments | Purpose |
   |---|---|---|
   | `list_servers` | `{"harness_id":"megafloyd-macmini"}` | Compact backend inventory. |
   | `health_check` | `{"server":"environment-ecosystem","harness_id":"megafloyd-macmini"}` | Live connection and tool discovery for this specific backend. |
   | `search_tools` | `{"query":"environment briefing","limit":5,"harness_id":"megafloyd-macmini"}` | Find a suitable backend tool by intent. |
   | `describe_tool` | `{"server":"environment-ecosystem","tool":"get_environment_briefing","harness_id":"megafloyd-macmini"}` | Read that tool's exact input schema before calling it. |

   For example, after checking its schema, call the briefing tool with:

   ```json
   {
     "name": "call_tool",
     "arguments": {
       "server": "environment-ecosystem",
       "tool": "get_environment_briefing",
       "arguments": {
         "harness_id": "megafloyd-macmini",
         "offset": 0,
         "limit": 50
       },
       "harness_id": "megafloyd-macmini",
       "timeout_ms": 30000
     }
   }
   ```

   The outer object above is the parameters object of the MCP `tools/call` request. The nested `arguments` object is passed unchanged to the selected backend. `harness_id` is an advisory identifier, not a credential. Choose one stable identifier for the calling harness.

   Use `health_check` with a **specific server** for an authoritative live check. Omitting `server` returns a fleet summary from an index that may be stale. Fetch only the schemas needed for the task, rather than loading every backend definition.

5. **Optionally read and acknowledge the environment briefing.**

   Briefing and acknowledgment are recommended context setup, never an access gate. Tools work before acknowledgment. If a briefing returns a non-null `nextOffset`, request the next page with that offset and the same `harness_id` until `nextOffset` is null. Then call `call_tool` with:

   ```json
   {
     "server": "environment-ecosystem",
     "tool": "acknowledge_environment",
     "arguments": {
       "harness_id": "megafloyd-macmini",
       "snapshot_id": "COPY snapshotId FROM THE BRIEFING",
       "content_hash": "COPY contentHash FROM THE BRIEFING"
     },
     "harness_id": "megafloyd-macmini"
   }
   ```

   Replace the two indicated values with the exact returned values. An `onboarding_hint` is advice, not a permission denial. For tools that create files, supply the member's chosen output folder as required by that tool's schema and the server law.

6. **Verify the actual harness tool path.**

   Confirm that the harness exposes the discovered gateway tools to its model. Then use its normal tool workflow to request one specific backend health check and inspect the returned result. Keep ordinary tool permission handling in place. A shell command that manually invokes SSH, or a separate MCP test client, establishes a narrower result than a successful model-driven call through the canonical harness.

   In Go MEGAFLOYD, the direct manual-shell API does not invoke the model or the model-facing bash tool's permission check. It is therefore insufficient by itself to prove removal of the agent's SSH restriction. A shell-only history entry also does not suppress title generation on the next real model turn; title generation can make an additional provider call.

If a harness says SSH is forbidden, first identify the rule and the execution layer it actually controls. A model's belief, a repository instruction, the model-facing bash tool's command list, and the host platform's execution policy are different things. An MCP client's ability to launch its configured stdio transport must be checked separately from the model-facing bash tool.

The owner-managed SSH ban in canonical MEGAFLOYD source has been removed while retaining ordinary tool permissions. An external harness's real platform prohibition cannot be overridden by this guide, a repository instruction, or the model deciding to ignore it. Its operator must change the policy or provide a transport that the platform supports and permits. Renaming SSH, hiding it in a wrapper, or using an unapproved tunnel is not a valid onboarding fix.

For browser access to the personal shared-Go interface from another machine, keep the web adapter bound to the Mac's loopback address. Use an authorized SSH forward from the remote workstation, for example:

```sh
ssh -N -L 58628:127.0.0.1:58627 floyd@Mac-mini.local
```

Then open `http://127.0.0.1:58628` on that workstation. Use the calling member's assigned SSH account, keep host-key verification enabled, and substitute the actual adapter port shown by MegaFloyd if it differs from `58627`. Choose another unused local port if `58628` is occupied. This browser forward uses the existing SSH access boundary; it is separate from the MCP stdio transport above and creates no new gateway service or application login gate.

**Evidence status, updated 2026-10-24:** the rebuilt canonical Go `intermediate-D` candidate completed an actual browser-submitted BeamFree turn in 13.311 seconds, using two assistant steps and two ordinary web permission approvals. The bash SSH tool returned the hostname `inference` followed by a newline, with `is_error: false`. The gateway `health_check` for `environment-ecosystem` returned `ok: true`, `live: true`, `reachable: true`, 10 tools, and 118 ms latency. The Go terminal attached to the same session displayed both tool results and the correct final response. An earlier browser attempt was canceled by the test guard after successful SSH because native MCP arguments arrived as a JSON string; the adapter was corrected and directly checked before this successful second attempt. These results establish the intermediate candidate's normal SSH and MCP tool paths and shared terminal/web session.

The final combined installed release is 2026.10.24, BuildID `megafloyd-20261024T234343Z`. Actual desktop/browser/terminal handoffs use the same Go coordinator and preserved 23-message session. The active model remains the owner-selected DeepSeek V4.1 Flash/high. The final package checks made no new model calls; the live agent SSH/gateway execution above retains its explicitly named intermediate-D provenance.

Source references: Mac `/Users/floyd/.megafloyd/config/floyd.json`; inference `/srv/organized/services/MCP_SERVER_ROOM/mcp-gateway/src/index.ts` and its installed `dist/index.js`; connected gateway metadata for `list_servers`, `health_check`, `search_tools`, `describe_tool`, and `call_tool`; canonical `/Volumes/SanDisk1Tb/MEGAFLOYD/floyd-go/internal/agent/tools/mcp-tools.go`, `internal/backend/agent.go`, and `internal/agent/agent.go`.
