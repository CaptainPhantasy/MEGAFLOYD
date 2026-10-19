# Doctor — Health Check The Installation

Run a full self-diagnostic of the floyd installation: config, providers, sessions, LSP, MCP, skills, and logs. Report what is healthy, what is degraded, and the fix for each finding.

## Usage
```
/doctor
```

## Procedure
1. Config and environment: call the `floyd_info` tool — active config paths, model setup, loaded skills, LSP servers, MCP servers.
2. Providers: run `floyd models` (CLI) with the `bash` tool to verify provider configuration resolves.
3. Sessions: run `floyd session list` — confirms the SQLite store (floyd.db) opens and reads.
4. Logs: call the `floyd_logs` tool and scan the tail for errors (failed connections, permission denials, LSP crashes).
5. Report as three lists: HEALTHY (component, evidence), DEGRADED (component, exact error, suggested fix), UNKNOWN (could not be checked, why).

## Notes
- The dedicated `floyd doctor` CLI subcommand arrives with wave 3 (SSOT §3D); when it exists, run it first and use these steps for anything it does not cover.
- Doctor never mutates: read-only checks, fixes proposed not applied.
