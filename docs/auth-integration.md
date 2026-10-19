# Auth integration — how f7 authenticates, and what the Go chassis needs

Status: evidence record for the auth lane. Written 2026-10-19 on branch
`work/auth`. Everything marked **VERIFIED** was produced by a command run in
this session; the command and its output are quoted. Everything else is marked
**UNKNOWN** or **NOT CHECKED** and must not be treated as fact.

Scope: the device-code login flow, token storage, and refresh behaviour that
the existing f7 / floyd7 harness uses against the owner's inference server.
This document does not cover provider configuration, model listing, or the
web console beyond the three read-only probes named in the task.

No login, invite, or write request was sent. No token file was read.

## 1. Launcher environment (VERIFIED)

`cat /Users/floyd/.local/bin/floyd7`:

```sh
export FLOYD_CODE_BASE_URL='https://inference.tail58d565.ts.net:8446'
export FLOYD_CODE_OAUTH_HOST='https://inference.tail58d565.ts.net:8446'
export FLOYD_CODE_CDN_BASE='https://inference.tail58d565.ts.net:8444/code'
export FLOYD_DISABLE_TELEMETRY=1
export FLOYD_CODE_HOME="${FLOYD_CODE_HOME:-$HOME/.floyd7}"
exec "${FLOYD_COMPANY_INSTALL_ROOT:-$HOME/.local/share/floyd7-company}/bin/f7" "$@"
```

So the OAuth host and the managed API base are the same origin in this
deployment. The CDN is a different port (`8444`) and is not part of auth.

f7 resolves its data directory as `FLOYD_CODE_HOME` first, then
`~/.floyd-code` (`apps/floyd-code/src/utils/paths.ts:38`). The launcher pins
it to `~/.floyd7`.

## 2. Live server checks (VERIFIED)

Commands and exact results:

```text
GET /                                     http=200  text/html   644 bytes
GET /healthz                              http=200  application/json
{"ok": true}
GET /.well-known/openid-configuration     http=404  application/json
{"error": "not_found", "error_description": "No route for GET /.well-known/openid-configuration"}
```

The root document is the Floyd Code web console:

```html
<title>Floyd Code</title>
<nav aria-label="Main"><a href="/catalog">Providers</a><a href="/account">My account</a><a href="/admin">Administration</a></nav>
```

Consequences:

- The host is reachable over the tailnet and its certificate is trusted by
  this machine.
- There is **no OIDC discovery document**. Any client must hardcode the three
  device-flow paths below.
- Health checking for the Go chassis can use `GET /healthz`.

## 3. The device-code flow (VERIFIED IN SOURCE, NOT EXERCISED)

Source of truth: `packages/oauth/src/oauth.ts` in
`/Users/floyd/floyd-v6/KimiSource/kimi-code`. The same four auth files are
byte-identical in `/Users/floyd/MEGAFLOYD-git/floyd-code` (`diff -q` reported
`SAME` for `oauth.ts`, `constants.ts`, `identity.ts`,
`managed-floyd-code.ts`).

### 3.1 Endpoints

All three are `POST`, `Content-Type: application/x-www-form-urlencoded`,
`Accept: application/json`, against the OAuth host with any trailing slash
removed.

| Purpose | Path | Body fields | Evidence |
|---|---|---|---|
| Start device authorization | `/api/oauth/device_authorization` | `client_id` | `oauth.ts:129-134` |
| Poll for the token | `/api/oauth/token` | `client_id`, `device_code`, `grant_type=urn:ietf:params:oauth:grant-type:device_code` | `oauth.ts:179-188` |
| Refresh the token | `/api/oauth/token` | `client_id`, `grant_type=refresh_token`, `refresh_token` | `oauth.ts:245-260` |

Client ID, from `constants.ts:29`:

```text
17e5f671-d194-4dfb-9706-5516cb48c098
```

On a 200 from the authorization endpoint the client requires
`user_code`, `device_code`, and `verification_uri_complete` to be non-empty
strings. `verification_uri` is optional; `expires_in` may be null; `interval`
defaults to 5 seconds.

The three paths were **not** probed. The task allowed only `GET /`,
`/.well-known/openid-configuration`, and `/healthz`. Their existence is
verified from client source in two independent trees, not from a server
response.

### 3.2 Token response and on-disk format

A token response is accepted only when `access_token` is a non-empty string,
`refresh_token` is a non-empty string, and `expires_in` is a finite number
greater than zero (`oauth.ts:30-55`). `scope` defaults to `""`,
`token_type` defaults to `"Bearer"`. `expires_at` is computed as
`floor(now/1000) + expires_in`.

The persisted JSON is snake_case (`types.ts:65-93`):

```json
{
  "access_token": "...",
  "refresh_token": "...",
  "expires_at": 1794070096,
  "scope": "...",
  "token_type": "Bearer",
  "expires_in": 2592000
}
```

A token whose `access_token` is empty is treated as a revoked tombstone: the
user was logged in before and must log in again.

### 3.3 Credential slot naming (VERIFIED by matching the live file)

`resolveFloydCodeOAuthKey` (`managed-floyd-code.ts:324-341`) returns:

- `oauth/floyd-code` when the normalized OAuth host and base URL both equal
  the built-in defaults (both empty in this build);
- otherwise `oauth/floyd-code-env-` plus the first 16 hex characters of
  `sha256(JSON.stringify({oauthHost, baseUrl}))`, with key order
  `oauthHost` then `baseUrl`, and trailing slashes stripped.

For this deployment both values are
`https://inference.tail58d565.ts.net:8446`. The computed digest:

```text
$ printf '%s' '{"oauthHost":"https://inference.tail58d565.ts.net:8446","baseUrl":"https://inference.tail58d565.ts.net:8446"}' | shasum -a 256 | cut -c1-16
e45bd67489796b47
```

The live credential directory contains exactly one file:

```text
$ ls -1 ~/.floyd7/credentials
floyd-code-env-e45bd67489796b47.json
```

and `config.toml` refers to the same key three times:

```text
key = "oauth/floyd-code-env-e45bd67489796b47"
```

That is an independent confirmation that the Go derivation is correct, not a
guess.

Storage name derivation (`toolkit.ts:461-476`): `oauth/floyd-code` and
`floyd-code` both map to `floyd-code`; any other key drops the `oauth/` prefix;
keys containing `/` or starting with `.` are rejected. The storage layer then
writes `<name>.json`.

### 3.4 Storage behaviour

`packages/oauth/src/storage.ts`:

- directory created `0700` and re-tightened on every save;
- file written `0600`, via `<name>.json.tmp.<pid>.<random>` then fsync then
  rename;
- a missing, unreadable, or corrupt file loads as "no token" rather than an
  error;
- name sanitization rejects anything that is not a plain base name or that
  starts with a dot.

Live file mode, without reading its contents:

```text
$ stat -f 'mode=%Sp size=%z mtime=%Sm' ~/.floyd7/credentials/floyd-code-env-e45bd67489796b47.json
mode=-rw------- size=255 mtime=Oct  8 12:48:16 2026
```

### 3.5 Device identity headers

`identity.ts:18,77-92` and `types.ts:49-56`. Every auth request carries:

| Header | Donor value |
|---|---|
| `X-Msh-Platform` | `floyd_code_cli` (`FLOYD_CODE_PLATFORM`) |
| `X-Msh-Version` | host CLI version (`2.0.2` for this build) |
| `X-Msh-Device-Name` | hostname, ASCII-only, `unknown` fallback |
| `X-Msh-Device-Model` | donor: OS product name/version + arch |
| `X-Msh-Os-Version` | donor: `os.release()` |
| `X-Msh-Device-Id` | UUID persisted at `<home>/device_id`, mode 0600 |
| `User-Agent` | `floyd-code-cli/<version>` |

The version and platform are required non-empty ASCII; the donor refuses to
send a blank platform rather than inventing one.

The **server's requirement** for these headers is not verified. The client
sends them on every auth request, so the Go client should do the same.

### 3.6 Login behaviour

`oauth-manager.ts` and `apps/floyd-code/src/cli/sub/login-flow.ts`:

- one login attempt has a hard deadline of 15 minutes
  (`DEFAULT_DEVICE_CODE_TIMEOUT_MS`);
- the CLI prints `verification_uri_complete` (falling back to
  `verification_uri`) and the `user_code`, then opens a browser;
- polling waits `interval` seconds, default 5; on `slow_down` the interval
  grows by 5 seconds for the rest of the attempt;
- `authorization_pending` and `slow_down` keep polling;
- `access_denied` ends the login with an error;
- `expired_token` restarts the outer loop and requests a fresh device code,
  unless the 15-minute deadline has passed;
- any other error code is surfaced as a failure.

### 3.7 Refresh behaviour

`oauth-manager.ts:27-36,459-464` and `oauth.ts:232-296`:

- refresh is lazy, on demand; there is no background loop;
- refresh happens when `expires_at - now < max(300, expires_in * 0.5)`
  seconds;
- one in-process refresh at a time; concurrent callers share the result;
- a POSIX file lock coordinates refresh across processes
  (`<configDir>/oauth/<storageName>.lock`), and storage is re-read before and
  after the refresh so a refresh from another process is noticed;
- refresh retries up to three attempts on transport errors and on
  `429/500/502/503/504`, with 1 s then 2 s backoff;
- `401`, `403`, or `invalid_grant` is terminal: the user must log in again.

### 3.8 How the token is used

Managed API calls send `Authorization: Bearer <access_token>` against the
managed base URL (`managed-floyd-code.ts:503-510`). A `401`, `402`, or `403`
from the managed API is treated as an auth failure.

## 4. What the Go chassis needs

### 4.1 Configuration

| Setting | Source | Value for this deployment |
|---|---|---|
| OAuth host | `MEGAFLOYD_AUTH_HOST`, else `FLOYD_CODE_OAUTH_HOST`, else `FLOYD_OAUTH_HOST` | `https://inference.tail58d565.ts.net:8446` |
| Managed base URL | `MEGAFLOYD_AUTH_BASE_URL`, else `FLOYD_CODE_BASE_URL` | same origin |
| Client ID | `MEGAFLOYD_AUTH_CLIENT_ID`, else the constant | `17e5f671-d194-4dfb-9706-5516cb48c098` |
| Platform | `MEGAFLOYD_AUTH_PLATFORM`, else `floyd_code_cli` | `floyd_code_cli` until the server is proven to accept another value |
| Version | `MEGAFLOYD_VERSION`, else `FLOYD_CODE_VERSION` | must be non-empty; `2.0.2` is the donor build |
| Home | `MEGAFLOYD_HOME`, else `~/.megafloyd` | deliberately not `~/.floyd7` |
| Credentials dir | `<home>/credentials` or `MEGAFLOYD_AUTH_CREDENTIALS_DIR` | mirrors f7 layout |

Using `~/.floyd7` by accident would let two harnesses share one refresh token.
Because the Go side does not yet implement the cross-process lock, the safe
default is a separate home. Deliberate reuse of an existing login requires the
lock to be ported first.

### 4.2 Login

1. POST to `{OAuthHost}/api/oauth/device_authorization` with `client_id`, plus
   the `X-Msh-*` and `User-Agent` headers.
2. Require `user_code`, `device_code`, `verification_uri_complete`.
3. Show `verification_uri_complete` and `user_code` to the user; optionally
   open the URL. The URL is server-generated and its exact form is UNKNOWN.
4. Poll `{OAuthHost}/api/oauth/token` with the device-code grant every
   `interval` seconds, growing by 5 on `slow_down`, until success, denial, or
   the 15-minute deadline.
5. Persist the token under the derived storage name.
6. `logout` removes the file.

### 4.3 Storage

- Derive the key with the algorithm in §3.3; do not hardcode the hash, because
  a host change moves the slot.
- Write `<home>/credentials/<storageName>.json`, `0600`, in a `0700`
  directory, atomically.
- Treat a corrupt or missing file as "no token".
- Keep `ExpiresAt` in Unix seconds and `ExpiresIn` in seconds, as on the wire.

### 4.4 Refresh

- Refresh lazily, at the threshold in §3.7.
- Serialize refreshes inside one process.
- TODO before sharing a home with f7: a cross-process lock equivalent to the
  donor's `<configDir>/oauth/<storageName>.lock`.
- On `401/403/invalid_grant`, delete or tombstone the token and ask for a new
  login.
- Retry transport failures and `429/5xx` up to three attempts with backoff.

### 4.5 Wiring into the existing chassis

`internal/oauth.Token` already has `AccessToken`, `RefreshToken`, `ExpiresIn`,
`ExpiresAt`. The skeleton converts between that type and the f7 wire type, so
the existing provider plumbing can consume the result without a new credential
store. Nothing is wired into `internal/cmd/` or `internal/ui/`; login command
wiring is out of scope for this lane.

## 5. Design skeleton

`floyd-go/internal/megafloydauth/` — interfaces plus verified, non-network
pieces. Network calls fail closed with `ErrNotImplemented`.

| File | Contents |
|---|---|
| `doc.go` | package status and evidence pointer |
| `config.go` | environment precedence, defaults, home and credentials path |
| `keys.go` | credential key hash and storage-name mapping |
| `token.go` | wire token, validity, refresh threshold, `oauth.Token` conversion |
| `store.go` | `Store` interface and atomic file store |
| `identity.go` | device id minting and `X-Msh-*` headers |
| `flow.go` | `DeviceFlow`, `TokenSource`, poll result types, unimplemented flow |
| `megafloydauth_test.go` | key-vector, storage, env, expiry, header, fail-closed tests |

Verified state of the package:

```text
$ cd floyd-go && go build ./...
build_exit=0        (empty error log)

$ go test ./internal/megafloydauth/ -count=1
ok  github.com/legacy-ai/floyd/internal/megafloydauth  0.119s
```

Known deviations from the donor, all recorded rather than hidden:

- `User-Agent` product is `megafloyd`, not `floyd-code-cli`.
- `X-Msh-Device-Model` is `GOOS GOARCH`; `X-Msh-Os-Version` is `GOOS`. The
  donor sends richer values from `node:os`.
- No cross-process refresh lock.
- File storage only; the donor also names a `keyring` storage option.

## 6. Unknowns (do not treat as verified)

| # | Unknown | Why it matters |
|---|---|---|
| 1 | The three `/api/oauth/*` routes were never called from this session | Endpoint existence rests on two identical client source trees, not a server response |
| 2 | Whether `X-Msh-*` headers are required or validated | If required, wrong values could reject login |
| 3 | Whether the server accepts platform `megafloyd` or UA `megafloyd/<v>` | Platform/UA may identify the client family |
| 4 | Exact `verification_uri_complete` shape | A prior note in this repo claims an `/approve?user_code=` page; not re-checked this session |
| 5 | Real token lifetime and scope | The token file was not read (secret) |
| 6 | Whether refresh rotates the refresh token | Determines whether two processes may share a token |
| 7 | Server rate limits and error codes beyond RFC 8628 | Retry policy may need adjustment |
| 8 | Whether `keyring` storage is needed for any surface | The donor supports it; the Go side does not |
| 9 | Whether the console's account pages use the same token | Out of scope; only three GETs were allowed |

## 7. Reproduce

```sh
cat /Users/floyd/.local/bin/floyd7
curl -sS -m 20 -w 'http=%{http_code}\n' https://inference.tail58d565.ts.net:8446/healthz
curl -sS -m 20 -w 'http=%{http_code}\n' https://inference.tail58d565.ts.net:8446/.well-known/openid-configuration
printf '%s' '{"oauthHost":"https://inference.tail58d565.ts.net:8446","baseUrl":"https://inference.tail58d565.ts.net:8446"}' | shasum -a 256 | cut -c1-16
ls -1 ~/.floyd7/credentials
cd /Users/floyd/MEGAFLOYD-auth/floyd-go && go build ./... && go test ./internal/megafloydauth/ -count=1
```
