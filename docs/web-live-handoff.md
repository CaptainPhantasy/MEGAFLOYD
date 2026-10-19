# Web live handoff — verified 2026-10-19

Worktree: `/Users/floyd/MEGAFLOYD-x-web`, branch `fix/web-live`.
Verified at commit `5c16e6a` (`fix(web): track a self-contained Node server bundle`).

This closes the two web-lane audit items:

- the Node server entry point and its runtime files are now tracked and
  present in a fresh checkout;
- the Go `megafloydweb` engine has been exercised against that tracked
  bundle with isolated `HOME`/data directories, and the separate
  session-store gap is documented from live API responses.

## 1. What ships

Tracked in `floyd-code/apps/floyd-code`:

| Path | Bytes | SHA-256 | Git mode |
|---|---:|---|---|
| `dist/main.mjs` | 17,816,667 | `e2cf8b5590b3a8ff6b6c425d5900035b81a7a366ab1baa89f0b641d980e50570` | `100755` |
| `dist/search-worker.mjs` | 469,244 | `2622e9dc5a2dde20bb2e5362ca70484db6e29d14001532c944f78461f878f0bd` | `100644` |
| `dist-web/**` | 524 files | already tracked | — |
| `package.json` | 4,597 | already tracked | — |

`dist-web` is the prebuilt web client. `package.json` must stay beside
`dist/` because `getHostPackageRoot()` finds the package root by walking
up to the nearest `package.json`. `dist/main.mjs` resolves the bundle at
`<package root>/dist-web`.

No `node_modules` is tracked. The bundle has no external package imports;
its only non-relative imports are Node built-ins (`node:*`, `fs`, `path`,
`http`). `ws` and `qrcode` are bundled. `ws` still probes two optional
native accelerators, `bufferutil` and `utf-8-validate`, inside `try`/`catch`
guards; their absence is handled by the JavaScript fallback.

## 2. Why `dist/main.mjs` was invisible to git

`floyd-code/.gitignore` excluded every `dist/` directory. The root
`.gitignore` has the same rule, and a deep `.gitignore` wins over the
root one. No negation existed, so the file was never staged.

```
$ git check-ignore -v floyd-code/apps/floyd-code/dist/main.mjs
floyd-code/.gitignore:2:dist/	floyd-code/apps/floyd-code/dist/main.mjs
```

Fix in `floyd-code/.gitignore`: re-include the directory, ignore its
contents, then re-include the two runtime files (last matching pattern
wins):

```
!apps/floyd-code/dist/
apps/floyd-code/dist/*
!apps/floyd-code/dist/main.mjs
!apps/floyd-code/dist/search-worker.mjs
```

Negative control after the fix:

```
$ git check-ignore --non-matching --verbose \
    floyd-code/apps/floyd-code/dist/main.mjs \
    floyd-code/apps/floyd-code/dist/other.bin
floyd-code/.gitignore:10:!apps/floyd-code/dist/main.mjs	floyd-code/apps/floyd-code/dist/main.mjs
floyd-code/.gitignore:9:apps/floyd-code/dist/*	floyd-code/apps/floyd-code/dist/other.bin
```

`main.mjs` matches the negation; `other.bin` is still ignored.

## 3. The prebuilt bundle was not self-contained

The bundle copied from the sibling worktree at the same commit failed at
startup:

```
Error [ERR_MODULE_NOT_FOUND]: Cannot find package 'ws' imported from
  .../floyd-code/apps/floyd-code/dist/main.mjs
```

`--version` and `web --help` worked because they never load the server
path. A scan of the bundle found exactly two external runtime packages:
`ws` and `qrcode` (both listed in `apps/floyd-code/package.json`
`dependencies`). tsdown externalizes package dependencies by default, so
a plain `pnpm build` produced an entry that needed `node_modules`.

Fix: `apps/floyd-code/tsdown.config.ts` now force-bundles those two:

```ts
deps: {
  onlyBundle: false,
  alwaysBundle: [/^qrcode$/, /^ws$/],
},
```

Build command and result:

```
$ corepack pnpm --filter @legacy-ai/floyd-code build
ℹ dist/main.mjs  17.82 MB
✔ Build complete in 9074ms
ℹ dist/search-worker.mjs  469.24 kB
✔ Build complete in 150ms
Web assets OK: .../dist-web (525 entries, synced from code-app)
node build exit=0
```

The rebuilt `main.mjs` has no `ws` or `qrcode` import. `search-worker.mjs`
is byte-identical to the prebuilt copy.

## 4. Fresh-clone proof

A fresh tree was extracted from the commit, not from the working
directory. `git archive` includes only tracked files, so `node_modules`
cannot leak in.

```
$ CLONE=$(mktemp -d)
$ git archive HEAD | tar -x -C "$CLONE"
extract exit=0
node_modules:        0
main.mjs: -rwxr-xr-x  17816667  .../floyd-code/apps/floyd-code/dist/main.mjs
dist-web files:      524
package.json:         present
```

The Go engine was then pointed at that tree. The probe called
`megafloydweb.StartWeb` with `HomeDir`, `DataDir`, `CWD`, `SessionID`,
`SkipTailscale: true`, `DisableServe: true`, and explicit `ServerPath` /
`BundlePath`. It hit the server over loopback.

```
START elapsed=4.278s pid=90510 reused=false
HEALTH status=200 body={"code":0,"msg":"success","data":{"ok":true},...}
PAGE status=200 content_type="text/html; charset=utf-8" bytes=1189 title="Floyd Code Web"
ASSET status=200 content_type="text/javascript; charset=utf-8" cache="public, max-age=31536000, immutable"
SESSION_PAGE status=200 content_type="text/html; charset=utf-8" title="Floyd Code Web"
LOG_TAIL_BEGIN
  ▐█▛█▛█▌  Floyd server ready  2.0.2
  Local:    http://127.0.0.1:53948/#token=...
  Token:    ...
LOG_TAIL_END
```

### 4.1 CLI binary proof

The built binary was also run through its real `run --web` path. `tailscale`
was removed from `PATH` on purpose, so the engine could not change the
machine's tailnet Serve configuration; it only recorded a warning.

```
$ go build -o /tmp/megafloyd-x-web-bin .
$ env -i PATH="$TMPBIN:/usr/bin:/bin" HOME="$HOME_DIR" \
    MEGAFLOYD_HOME="$HOME_DIR/.megafloyd" FLOYD_CODE_HOME="$HOME_DIR/web" \
    MEGAFLOYD_WEB_PORT=58733 \
    MEGAFLOYD_WEB_SCRIPT=.../dist/main.mjs \
    MEGAFLOYD_WEB_BUNDLE=.../dist-web \
    /tmp/megafloyd-x-web-bin run --web
Web UI: http://127.0.0.1:58733/sessions/#token=...
Server: started on 127.0.0.1:58733 (loopback)
Log:    $HOME_DIR/.megafloyd/logs/web-server.log
Warning: tailscale is not available: ... executable file not found in $PATH
cli exit=0

$ curl -sS -w 'http=%{http_code}\n' http://127.0.0.1:58733/api/v1/healthz
http=200
{"code":0,"msg":"success","data":{"ok":true},...}

$ curl -sS -w 'http=%{http_code} bytes=%{size_download}\n' http://127.0.0.1:58733/
http=200 bytes=1189
<title>Floyd Code Web</title>
```

The server log contained the ready banner and token. The listener was
stopped afterwards; the port was empty.

The same probe ran against the worktree before this one and produced the
same health, page, asset, and session results.

## 5. Session-store gap (precise)

The Go session ID and the Node session ID are different namespaces, and
there is no mirror between their stores.

- Go creates a bare UUID:
  `floyd-go/internal/session/session.go:98` —
  `ID: uuid.New().String()`.
- Node creates `session_<uuid>`:
  `floyd-code/packages/agent-core-v2/src/workspace/sessionLifecycle/sessionLifecycleService.ts:904` —
  ``return `session_${randomUUID()}`;``.
- The handoff URL is built from the Go ID:
  `floyd-go/internal/megafloydweb/url.go` — `/sessions/<sessionID>`.
- Go persists sessions in `<data-dir>/floyd.db`
  (`floyd-go/internal/db/connect.go:93`), default data directory `.floyd`
  (`floyd-go/internal/config/config.go:24`).
- Node persists its sessions under `FLOYD_CODE_HOME`, default
  `~/.floyd-code` (`floyd-code/apps/floyd-code/src/utils/paths.ts:38`).

Live evidence from the handoff with Go session
`11111111-1111-4111-8111-111111111111`:

```
SESSIONS_BEFORE status=200 body={"code":0,"msg":"success","data":{"items":[],"has_more":false},...}
GO_SESSION_LOOKUP status=200 body={"code":40401,"msg":"session 11111111-1111-4111-8111-111111111111 does not exist","data":null,...}
NODE_CREATE status=200 body={"code":0,...,"data":{"id":"session_e3cb3f4b-1c7f-4eeb-958c-eb149b81967a",...}}
SESSIONS_AFTER status=200 body={"code":0,...,"data":{"items":[{"id":"session_e3cb3f4b-...",...}]}}
```

Consequences:

- `/sessions/<go-id>` still returns HTTP 200 because the web asset route
  falls back to `index.html` for extensionless paths. The page renders
  the shell, but the session does not exist from the server's point of
  view, so the client has no transcript to load.
- The session list is empty before a Node-created session exists; the Go
  session never appears in it.
- Creating a session through the Node API produces `session_<uuid>`, a
  second ID that the Go TUI never sees.
- No import, mirror, or dual-write code exists in either direction.

This is a storage and identity gap, not a URL-format gap. Changing the URL
alone would not make the Go transcript available.

## 6. Verification boundary

Exercised:

- Go build of every package (`go build ./...`, exit 0).
- `go test -race ./internal/megafloydweb/...` (exit 0, 1.316 s).
- Real server start, health route, `/`, hashed asset route, session page.
- Authenticated `GET /api/v1/meta`, `GET /api/v1/sessions`,
  `GET /api/v1/sessions/<id>`, `POST /api/v1/sessions`.
- Server log capture and shutdown; the port was closed afterwards.
- Fresh tree with zero `node_modules`.

Known operational gap:

- The engine starts the Node server detached and has no stop command. The
  verification killed the detached PID directly. A user-facing stop path
  (for example a `/web stop` command) is not implemented.

Not exercised:

- Tailscale Serve registration and remote URLs (`SkipTailscale` and
  `DisableServe` were set to avoid changing the machine's tailnet
  configuration).
- WebSocket transcript streaming.
- A long-running server or browser-side rendering.
- Non-macOS platforms.

## 7. Commands

```
git check-ignore -v floyd-code/apps/floyd-code/dist/main.mjs
corepack pnpm install --frozen-lockfile
corepack pnpm --filter @legacy-ai/floyd-code build
go build ./...
go build -o /tmp/megafloyd-x-web-bin .
go test -race ./internal/megafloydweb/...
go run mvdan.cc/gofumpt@latest -l floyd-go/internal/megafloydweb
corepack pnpm exec oxlint apps/floyd-code/tsdown.config.ts
git archive HEAD | tar -x -C "$CLONE"
```
