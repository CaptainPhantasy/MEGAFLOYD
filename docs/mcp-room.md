# MCP room — home and cloud config

Decision first: use the tailnet URL at home and the owner-published URL in
the cloud. No secret goes in any config file. Tags are VERIFIED (command run,
output quoted) or UNVERIFIED (not tested).

## What the room is

The MCP room server owned on the inference box. SSOT.md §6 (VERIFIED, read
from the repo): "MCP room ownership confirmed on inference box
(`/srv/organized/services/MCP_SERVER_ROOM`, LAN git remote, owner handling the
public publish)". SSOT.md §3 item 32 requires the room URL in config: tailnet
at home, public URL in cloud.

## Endpoint check — home (VERIFIED, 2026-10-19)

```console
$ curl -sS -m 10 -o /dev/null -w 'GET /mcp -> %{http_code}\n' \
    https://inference.tail58d565.ts.net:8443/mcp
GET /mcp -> 405

$ curl -sS -m 10 -X OPTIONS -o /dev/null \
    -w 'OPTIONS /mcp -> %{http_code} %{header_json}\n' \
    https://inference.tail58d565.ts.net:8443/mcp
OPTIONS /mcp -> 405 allow={"allow":["POST"]}
```

Read: the host answers, and only `POST` is allowed on `/mcp`. That is the
shape of a streamable-HTTP MCP endpoint. NOT tested: an MCP `initialize`
handshake, the protocol version, and the tool list. No login, invite, or write
was attempted.

## How the chassis configures MCP (VERIFIED from source)

- `floydrc` is Bash. Global file: `$XDG_CONFIG_HOME/floyd/floydrc`
  (normally `~/.config/floyd/floydrc`). Project file: `./.floydrc` or
  `./floydrc`, which override global. Evidence:
  `floyd-go/docs/config/README.md:88-100`.
- The builtin is `mcp add <name>` with `--type`, `--url`, `--header`,
  `--timeout`, `--enabled-tools`, `--disabled-tools`, `--sessionless`,
  `--oauth`. Evidence: `floyd-go/internal/shellconfig/mcp.go:25,45`.
- Repeated `mcp add` calls with the same name update one entry. Later lines
  win. Evidence: comment on `handleMCP`, `internal/shellconfig/mcp.go:12-23`.
- The parsed shape is `Config.MCP map[string]MCPConfig`
  (`internal/config/config.go:712`) with `MCPConfig` fields at
  `internal/config/config.go:193`.
- A header whose value expands to empty is dropped, not sent blank. Evidence:
  `floyd-go/docs/config/README.md:312-313`.
- A floydrc that defines nothing returns no config; that is valid, not an
  error. Evidence: `internal/shellconfig/load.go:62-65`.

## Files in this change

| File | Purpose |
| --- | --- |
| `floyd-go/.floydrc.example-megafloyd` | Both environments, switched by `MEGAFLOYD_ENV` |
| `floyd-go/docs/config/examples/mcp-room-home.floydrc` | Home only, copy-paste |
| `floyd-go/docs/config/examples/mcp-room-cloud.floydrc` | Cloud only, copy-paste |

## Home values (VERIFIED)

```bash
mcp add room \
	--type http \
	--url "https://inference.tail58d565.ts.net:8443/mcp" \
	--timeout 30
```

## Cloud values (UNVERIFIED — owner publishes)

The public host is not published yet, so no hostname is guessed. Set
`MEGAFLOYD_MCP_ROOM_URL` to the URL the owner publishes, for example
`https://<owner-published-host>/mcp` (placeholder, not a real host).

```bash
room_url="${MEGAFLOYD_MCP_ROOM_URL:-}"

if [[ -n "$room_url" ]]; then
	mcp add room --type http --url "$room_url" --timeout 60
fi
```

With the variable unset, no room is registered and megafloyd still starts.
This is deliberate: a Codespace must not refuse to start because a public
endpoint is not published yet. Once the room is required in cloud, change the
guard to `${MEGAFLOYD_MCP_ROOM_URL:?...}`.

## Secrets

No token is stored in any file here. Optional credential:
`MEGAFLOYD_MCP_ROOM_TOKEN`. When set, the config adds
`Authorization: Bearer <token>`; when unset, no header is added at all.

```bash
if [[ -n "${MEGAFLOYD_MCP_ROOM_TOKEN:-}" ]]; then
	mcp add room --type http --url "$room_url" --timeout 60 \
		--header Authorization "Bearer ${MEGAFLOYD_MCP_ROOM_TOKEN}"
fi
```

Supply the token from the launcher or a password manager, e.g.
`MEGAFLOYD_MCP_ROOM_TOKEN="$(op read ...)"` before starting megafloyd. Never
commit it. Whether the room needs a token at all is UNVERIFIED.

## Install

1. Pick the file for the environment.
2. Copy it to the global floydrc: `~/.config/floyd/floydrc`.
   Or to a project as `./.floydrc`.
3. Start megafloyd. The room appears in the MCP status/dialog.

`~/.megafloyd` as the megafloyd home is a separate task (TASK=pkg). Until the
launcher sets `FLOYD_GLOBAL_CONFIG` to that directory, the chassis default
above applies.

## Validation evidence (VERIFIED, 2026-10-19)

Each example was executed through the chassis's own loader
(`shellconfig.LoadShellConfig`) and then through the shipped CLI.

Home:

```console
$ go run ./.mcpvalidate .floydrc.example-megafloyd \
    docs/config/examples/mcp-room-home.floydrc
OK .floydrc.example-megafloyd
{
  "room": {
    "type": "http",
    "url": "https://inference.tail58d565.ts.net:8443/mcp",
    "timeout": 30
  }
}
OK docs/config/examples/mcp-room-home.floydrc
{ ...same... }
```

Cloud, URL unset:

```console
$ MEGAFLOYD_ENV=cloud go run ./.mcpvalidate .floydrc.example-megafloyd \
    docs/config/examples/mcp-room-cloud.floydrc
OK .floydrc.example-megafloyd (empty: no config produced)
OK docs/config/examples/mcp-room-cloud.floydrc (empty: no config produced)
```

Cloud, URL set:

```console
$ MEGAFLOYD_ENV=cloud MEGAFLOYD_MCP_ROOM_URL=https://room.example.invalid/mcp \
    go run ./.mcpvalidate .floydrc.example-megafloyd \
    docs/config/examples/mcp-room-cloud.floydrc
OK .floydrc.example-megafloyd
{
  "room": {
    "type": "http",
    "url": "https://room.example.invalid/mcp",
    "timeout": 60
  }
}
```

Cloud with token set adds:

```json
"headers": { "Authorization": "Bearer <token>" }
```

End-to-end CLI load, using the global config directory override:

```console
$ tmp="$(mktemp -d)"
$ cp .floydrc.example-megafloyd "$tmp/floydrc"
$ FLOYD_GLOBAL_CONFIG="$tmp" FLOYD_GLOBAL_DATA="$tmp/data" go run . models >/dev/null
$ echo $?
0
```

Negative control (proves the check is sensitive to MCP syntax):

```console
$ printf 'mcp add room --type http --bogus x\n' > "$tmp/floydrc"
$ FLOYD_GLOBAL_CONFIG="$tmp" FLOYD_GLOBAL_DATA="$tmp/data" go run . models
exit=1
  ERROR
  Failed to load config from paths [...]: failed to load shell config
  .../floydrc
```

`./.mcpvalidate` is a temporary helper used only for this verification and is
not committed.

## Unknowns

1. Does the room require auth? Not tested. No login or write was attempted.
2. Is the room sessionless (no `Mcp-Session-Id`)? Not tested. Leave
   `--sessionless` unset; auto-detection only covers known GitHub URLs
   (`internal/config/config.go:488-494`).
3. MCP handshake, protocol version, and tool list: not exercised.
4. Public cloud URL: not published. UNVERIFIED by design.
5. Whether the cloud room should stay disabled until the auth-server identity
   lands (SSOT §3 item 34). Owner decision.
