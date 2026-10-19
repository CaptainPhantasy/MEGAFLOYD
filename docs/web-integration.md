# Web integration — verified findings

Checked 2026-10-19. Read-only work: no remote change, no rebuild, no edit of web sources.

## Short answer

The existing web version is a prebuilt client bundle committed at
`floyd-code/apps/floyd-code/dist-web` (524 files, about 30 MB). It is served by the
Node server `kap-server` through the command `floyd web`, which looks for the bundle at
`<package root>/dist-web`.

Three local copies exist and are byte-identical.

The instance at `https://inference.tail58d565.ts.net:8443/` is **not** this bundle. It is
the Floyd MCP Room app. No deployment of `dist-web` was found on 8443 or 8446.

## 1. What the web version is

Entry files, read from `floyd-code/apps/floyd-code/dist-web`:

| File | Size | SHA-256 |
|---|---:|---|
| `index.html` | 1,189 B | `0197f7c85d934fd6fecd0d61db74a5c07d52c72fe2e80b2bc4653d73c3aac1a0` |
| `boot.js` | 1,108 B | `e6c18e65d115465ffa16b5fcf0940348d6d780928a46d871f165bdf30a68df25` |
| `favicon.ico` | 3,461 B | `ef6d2fd106548353f1f736448b0083be700bc19282cd93324977f44c424ad2c0` |
| `assets/index-DusVyqlT.js` | 3,732,157 B | main module |
| `assets/index-BxYISzcB.css` | 604,234 B | main stylesheet |

Facts taken from `index.html`:

- `<title>Floyd Code Web</title>`
- Absolute paths from origin root: `/boot.js`, `/assets/index-DusVyqlT.js`,
  `/assets/index-BxYISzcB.css`, `/favicon.ico`.
- `boot.js` is a classic external script because the server's Content Security Policy
  forbids inline scripts. It sets color scheme and font scale before the bundle loads.

The bundle is a client, not a standalone site. Its main module contains the same-origin
API literals `/api/v1` and `/api/v1/ws`, and reads a token from `location.hash`
(`#token=`). It needs the Floyd Code server behind it.

Source does not live in this repo. `floyd-code/AGENTS.md` says the UI is developed in
the code-app repo and shipped here as the committed, prebuilt bundle, synced with
`FLOYD_CODE_REPO=<checkout> pnpm run sync:web`.

Asset check (run in the worktree):

```
$ node apps/floyd-code/scripts/check-web-assets.mjs
Web assets OK: /Users/floyd/MEGAFLOYD-web/floyd-code/apps/floyd-code/dist-web (525 entries, synced from code-app)
exit 0
```

525 entries = 524 files plus the directory entry.

## 2. Where it lives locally

All three copies are identical. Hash is taken over the sorted per-file SHA-256 values,
so the directory path cannot affect it.

| Path | Bundle hash |
|---|---|
| `/Users/floyd/MEGAFLOYD-web/floyd-code/apps/floyd-code/dist-web` | `9aefa61c945de5f2de4b405b22e931c92b84c175aaef2fca371ded30bc910ba4` |
| `/Users/floyd/MEGAFLOYD-git/floyd-code/apps/floyd-code/dist-web` | `9aefa61c945de5f2de4b405b22e931c92b84c175aaef2fca371ded30bc910ba4` |
| `/Users/floyd/floyd-v6/KimiSource/kimi-code/apps/floyd-code/dist-web` | `9aefa61c945de5f2de4b405b22e931c92b84c175aaef2fca371ded30bc910ba4` |

In the megafloyd worktree the bundle is tracked, not ignored: 524 paths in
`git ls-files`. The commit that restored it is `62c0c0c`
(`fix(build): restore klient/Dockerfile and committed dist-web bundle`).

## 3. How it is served

Code path, read in this worktree:

- `floyd-code/apps/floyd-code/src/cli/sub/web/run.ts`
  `serverWebAssetsDir()` returns the native asset directory when present, otherwise
  `join(getHostPackageRoot(), 'dist-web')`. `getHostPackageRoot()` walks up from the
  running module to the nearest `package.json`.
- `floyd-code/packages/kap-server/src/start.ts` calls `registerWebAssetRoutes(app,
  opts.webAssetsDir)` when a directory is set.
- `floyd-code/packages/kap-server/src/routes/webAssets.ts`:
  - fails fast if `index.html` is missing;
  - registers `GET /` and `GET /*`;
  - reserves `/api` and `/documentation` paths for the API;
  - falls back to `index.html` for extensionless paths (single-page app routing);
  - sends hashed files under `assets/` with
    `Cache-Control: public, max-age=31536000, immutable`, and everything else with
    `no-cache`;
  - serves correct MIME types for html, js, css, json, svg, png, jpg, webp, ico, woff2.
- `floyd-code/packages/kap-server/src/middleware/securityHeaders.ts` sets
  `Content-Security-Policy: default-src 'self'; style-src 'self' 'unsafe-inline'; ...`.

Default bind, read from `src/cli/sub/web/shared.ts` and verified in `--help`:

- host `127.0.0.1`, port `58627`.
- `--host` binds `0.0.0.0` (all interfaces) and prints a bearer token.
- The opened URL carries the token in the fragment: `#token=<token>`.
- Non-loopback binds expect a TLS-terminating reverse proxy unless
  `--insecure-no-tls` is passed; remote shutdown and remote terminals are off by
  default.

A prebuilt Node server ships in the same package:

```
/Users/floyd/MEGAFLOYD-git/floyd-code/apps/floyd-code/dist/main.mjs   17,564,330 B
sha256 3e401fbfac6c3b5a05c2d9239b5838165c37c99977bc5f7ab981123f1470e94f

$ node dist/main.mjs --version
2.0.2

$ node dist/main.mjs web --help
Usage: f7 web [options] [command]
...
exit 0
```

`package.json` maps this file to both `f7` and `floyd-code`. Local Node is v26.9.0.
The app package requires Node `>=22.19.0`; the monorepo root requires `>=24.15.0`.

## 4. What is actually deployed — verified, and not the bundle

`https://inference.tail58d565.ts.net:8443/` — HTTP 200, read-only GET:

- `<title>Floyd MCP Room | Inference</title>`
- Next.js / Vinext RSC app. Assets: `/assets/index-DdwB_S-J.css`,
  `/assets/index-DbUNluQT.js`, `/assets/room-workbench-CSoMGf4N.js`,
  `/assets/rolldown-runtime-S-ySWqyJ.js`.
- `/agent-manifest.json` → 200, 7,067 B; `/llms.txt` → 200, 3,903 B.
- Bundle entry probes: `/assets/index-DusVyqlT.js` → 404, `/assets/index-BxYISzcB.css`
  → 404, `/index.html` → 404, `/web` → 404, `/code` → 404, `/app` → 404,
  `/healthz` → 404.

`https://inference.tail58d565.ts.net:8446/` — HTTP 200, read-only GET:

- `<title>Floyd Code</title>`, but a different app: assets `/platform.css` and
  `/platform.js`, server-rendered pages (`/catalog`, `/account`, `/admin`).
- Response headers: `server: Caddy`, `server: floyd-auth/1.0 Python/3.14.4`.
- `/healthz` → 200 `{"ok": true}`; `/favicon.ico` → 404.
- All dist-web entry probes (`/assets/index-DusVyqlT.js`, `/assets/index-BxYISzcB.css`,
  `/boot.js`, `/index.html`, `/web`, `/app`, `/code`) → 404.

Conclusion: no local directory matches a deployed `dist-web` build, because no
deployment of that bundle was found. The note in `findings.md` I.1 (that 8446 serves the
Floyd Code Web SPA) is not reproduced by today's observation; this document supersedes
it on that point.

The identity of the "working phone web UI" is therefore **not verified**. 8443 is the
MCP Room; 8446 is the auth/console app. Neither serves `dist-web`.

## 5. No-rebuild plan

Goal: ship the same bytes, unchanged.

### 5.1 Keep the bundle layout intact

Copy all 524 files into a stable directory in megafloyd. Proposed path:
`web/dist-web/`. Nothing is renamed or rewritten.

```
web/dist-web/index.html
web/dist-web/boot.js
web/dist-web/favicon.ico
web/dist-web/assets/...            # 521 files, hashed names
```

The server must mount this directory at the origin root. `index.html` requests
`/assets/...` with a leading slash, so serving it from a sub-path such as `/web/`
would break every asset URL.

### 5.2 Run the existing prebuilt server

Copy the prebuilt server beside it:

```
web/package.json                   # copy of apps/floyd-code/package.json
web/dist/main.mjs                  # 17.5 MB, self-contained CLI bundle
web/dist/search-worker.mjs
web/dist-web/...
```

Start it:

```
node web/dist/main.mjs web --host --port 58627 --no-open
```

`getHostPackageRoot()` finds `web/package.json`, so the server resolves
`web/dist-web` without any flag or environment variable.

For remote access, keep the bind loopback-only and put Caddy / Tailscale in front.
If the proxy forwards a host name the server does not know, add
`--allowed-host <name>`. Do not use `--dangerous-bypass-auth` on a reachable interface.

### 5.3 What not to do

- Do not serve the bundle with a plain static file server. It calls `/api/v1` and
  `/api/v1/ws` on the same origin; without the Floyd Code server it cannot function.
- Do not rebuild the bundle. Its source is not in this repo.
- Do not rename hashed assets or move them out of `assets/`. The immutable cache
  header depends on the hash pattern.
- Do not inline scripts into `index.html`. The server's CSP allows only `'self'`.

### 5.4 Go binary status

The Go chassis has no web-serving code: searching `floyd-go` for `dist-web` or a web UI
returned only unrelated image/URL matches. Wiring the web UI into the `megafloyd` Go
binary is new work, not a no-rebuild step. Two options, both needing a decision:

1. Run the Node server as a sidecar and have the launcher start both.
2. Add a Go reverse proxy or static-plus-API bridge later.

## 6. Commands run

```
curl -sS -D - https://inference.tail58d565.ts.net:8443/            # 200, MCP Room HTML
curl -sS https://inference.tail58d565.ts.net:8443/agent-manifest.json   # 200, 7067 B
# per-path probes on 8443 and 8446 listed in section 4
curl -sS https://inference.tail58d565.ts.net:8446/healthz          # {"ok": true}

node floyd-code/apps/floyd-code/scripts/check-web-assets.mjs
# Web assets OK: ... (525 entries, synced from code-app), exit 0

node dist/main.mjs --version                                       # 2.0.2
node dist/main.mjs web --help                                      # exit 0

# identical-copy proof
for d in <three paths>; do (cd "$d" && find . -type f -print0 | sort -z \
  | xargs -0 shasum -a 256 | awk '{print $1}' | shasum -a 256); done
# all three: 9aefa61c945de5f2de4b405b22e931c92b84c175aaef2fca371ded30bc910ba4
```

## 7. Open items

- Find the actual deployment of `dist-web`, if one exists. Not found on 8443 or 8446.
- Confirm that the phone UI the owner uses is this bundle. Not proven by today's probe.
- Decide Node sidecar versus Go proxy before wiring the web UI into `megafloyd`.
