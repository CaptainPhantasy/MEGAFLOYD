# megafloyd in a GitHub Codespace

This lane builds and runs `megafloyd` on GitHub's servers. A new person needs
a GitHub account and access to the private repository. Nothing here contacts
the owner's home machines, home LAN, tailnet, or a model host on the owner's
network.

## Files in this lane

| Path | Purpose |
| --- | --- |
| `.devcontainer/devcontainer.json` | Go 1.26 image, `MEGAFLOYD_*` paths, forwarded port 8080, post-create hook |
| `.devcontainer/post-create.sh` | Creates the megafloyd home, writes `env.sh`, builds `megafloyd`, links it on `PATH`, prints its version |
| `docs/cloud-codespace.md` | This document |

No Dockerfile is needed. `mcr.microsoft.com/devcontainers/go:1.26-bookworm`
exists on the Microsoft container registry; its tag list included
`1.26-bookworm` on 2026-10-19.

## 1. Create the Codespace

1. Open `https://github.com/CaptainPhantasy/MEGAFLOYD` in a browser.
2. Select the branch that contains this lane: `work/cloud-devcontainer`
   while it is under review, or `main` after it is merged. The branch must
   already exist on GitHub; a Codespace is built from a remote branch, not
   from a local commit.
3. Choose **Code -> Codespaces -> Create codespace on
   work/cloud-devcontainer**.
4. Leave the defaults. GitHub builds the container and runs
   `.devcontainer/post-create.sh`.

The first creation takes a few minutes. The log ends with lines like:

```
megafloyd: go version go1.26.6 linux/amd64
megafloyd: downloading Go modules
megafloyd: building version devel
megafloyd: built /home/vscode/.megafloyd/bin/megafloyd
floyd version devel
```

`devel` is expected when the clone has no reachable tag. If the clone has
tags, `git describe` supplies a value such as `v1.2.3-4-gabc1234`.

The binary is named `megafloyd`. The CLI still prints the upstream command
name `floyd` in its version line, so `floyd version devel` is correct.

## 2. Check the build

Open a terminal in the Codespace and run:

```bash
megafloyd --version
# floyd version devel

megafloyd --help
# prints USAGE, EXAMPLES, and the command list

cd floyd-go
go build ./...
# exit 0, no output
```

`megafloyd` is on `PATH` because `remoteEnv` adds
`/home/vscode/.megafloyd/bin`. If it is missing in an old terminal, run
`export PATH="$PATH:/home/vscode/.megafloyd/bin"` or use the full path
`/home/vscode/.megafloyd/bin/megafloyd`. Sourcing `~/.megafloyd/env.sh` sets
the config, data, cache, and log paths, not `PATH`.

## 3. Give it a model

Floyd reads provider keys from the environment. The full table is in
`floyd-go/README.md`; common names are `ANTHROPIC_API_KEY`,
`OPENAI_API_KEY`, `GEMINI_API_KEY`, and `OPENROUTER_API_KEY`.

Add a key as a Codespaces secret:

1. GitHub avatar -> **Settings** -> **Codespaces** -> **Secrets** ->
   **New secret**.
2. Name it exactly, for example `ANTHROPIC_API_KEY`.
3. Paste the key as the value and grant access to this repository.
4. Stop and restart the Codespace so the secret is present at startup.

Never put a key in `devcontainer.json`, `post-create.sh`, this document, or
any other committed file. The in-app model picker can also store a key in
Floyd's own config, which lives under the megafloyd home and is not committed.

### The `MEGAFLOYD_*` placeholders

`devcontainer.json` sets these to empty strings:

- `MEGAFLOYD_INFERENCE_URL`
- `MEGAFLOYD_AUTH_URL`
- `MEGAFLOYD_MCP_ROOM_URL`

They are reserved names. This build does not read them, and they do not point
Floyd at a provider. Use the provider keys above or a `floydrc`/`floyd.json`
provider entry instead. If you later add a Codespaces secret with one of
these exact names, first remove the empty key from `containerEnv`; otherwise
the empty value can shadow the secret.

The one `MEGAFLOYD_*` key the code reads is `MEGAFLOYD_POSTHOG_KEY`
(`floyd-go/internal/event/event.go`). It is not set here, so telemetry is
off. If you opt in, add it as a Codespaces secret, never as a file.

## 4. Run it

```bash
megafloyd                 # interactive TUI in the browser terminal
megafloyd server          # headless server on the default Unix socket
megafloyd --help          # every command and flag
```

## 5. Ports

| Port | Default | What it is |
| --- | --- | --- |
| none | yes | `megafloyd` and `megafloyd server` use a Unix socket: `$XDG_RUNTIME_DIR/floyd-<uid>.sock`, with `/tmp/floyd-<uid>.sock` as the fallback |
| 8080 | forwarded, nothing listening | Optional TCP server: `megafloyd server --host tcp://0.0.0.0:8080`, then `curl http://localhost:8080/v1/health` |
| 6060 | not forwarded | pprof, only when `FLOYD_PROFILE` is non-empty; it binds `localhost:6060` inside the container |
| 40704-40713 | not forwarded | MCP OAuth callback ports. A browser outside the Codespace cannot reach these `localhost` ports, so interactive MCP login may not complete; prefer token-based MCP auth or forward the port |

Port 8080 is reserved in `devcontainer.json` so a later TCP surface can use
it without editing the file. Nothing listens there by default. Keep the port
visibility **Private** unless the owner deliberately publishes it.

## 6. Why this needs no home network

- The container image comes from the public Microsoft registry.
- Go modules come from the public module proxy (`proxy.golang.org`); the
  Go toolchain named in `go.mod` comes from the same proxy.
- The repository comes from GitHub, using the GitHub account that created
  the Codespace.
- Model traffic must go to a public provider API, authenticated with a
  Codespaces secret, or to a public endpoint the owner publishes.
- No tailnet, VPN, SSH tunnel, home host, or home power is involved.

The only thing that can break the model path is a provider URL that is
reachable only on the owner's private network. This lane does not configure
one.

## 7. Rebuild and troubleshoot

Re-run the post-create script after changing it or pulling new code:

```bash
bash .devcontainer/post-create.sh
```

The script is idempotent: it rewrites `~/.megafloyd/env.sh`, adds the
`.bashrc` source line only once, and overwrites the binary.

| Symptom | Check |
| --- | --- |
| `megafloyd: command not found` | Open a new terminal; check `echo $PATH`; use `/home/vscode/.megafloyd/bin/megafloyd` |
| `go: downloading go1.26.6` | The image ships an earlier 1.26 patch; Go fetches the exact patch from the public proxy |
| Build fails on a module | Run `cd floyd-go && go mod download` and check that `proxy.golang.org` is reachable |
| Version prints `devel` | The clone has no reachable tag; expected outside a tagged release |
| `curl localhost:8080` is refused | Start `megafloyd server --host tcp://0.0.0.0:8080`; the default is a Unix socket |
| MCP OAuth never returns | The browser redirects to `localhost` on your machine, not the container; use token auth or forward the callback port |

## 8. Verification performed

Checked on 2026-10-19:

- `python3 -m json.tool .devcontainer/devcontainer.json` — exit 0.
- `jq empty .devcontainer/devcontainer.json` — exit 0.
- `bash -n .devcontainer/post-create.sh` — exit 0.
- `shellcheck .devcontainer/post-create.sh` — exit 0.
- `gitleaks` scan of the changed files — no leaks reported.
- The exact build command from `post-create.sh`, run locally — exit 0, and
  the binary printed `floyd version devcontainer-test`.
- `post-create.sh` in an isolated `HOME` — exit 0; it created `env.sh`, the
  binary, and the `.bashrc` source line.
- `megafloyd server --host tcp://127.0.0.1:18080` — `GET /v1/health`
  returned HTTP 200.
- The Microsoft registry tag list included `1.26-bookworm`.

Not verified here: a live Codespaces container creation. That requires
GitHub-hosted provisioning and cannot be exercised from this machine.
