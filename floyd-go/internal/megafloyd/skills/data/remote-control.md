# Remote-Control — Operate This Session From Another Device

Drive the current workspace from a phone or remote machine: start the headless server, verify reachability, and connect a remote client to the shared workspace.

## Usage
```
/remote-control [start|status|stop]
```

## Procedure
1. Start the server: run `floyd server` with the `bash` tool (serve mode; Unix socket or TCP, see internal/server). It exposes the same backend the TUI uses — sessions, permissions, LSP, MCP — over the socket with SSE events.
2. Verify: `floyd stats` or `floyd session list` through the same workspace to confirm the backend is live; check the socket path in the server output.
3. Connect the remote client from the other device against the same working directory — clients on the same `--cwd` join one workspace and share sessions and the permission queue (multi-client workspaces, internal/workspace).
4. `status`: report server process, socket, connected clients. `stop`: terminate the server process cleanly.

## Notes
- The phone-facing web surface and cloud path are wave-4 work (SSOT §3C-31); until then remote-control means the socket/TCP server plus a remote client on the same network or tailnet.
- Never expose the socket on a public interface; keep it to localhost, tailnet, or an authenticated tunnel.
