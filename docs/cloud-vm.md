# megafloyd on a Linux VM

One-command install for a headless Debian or Ubuntu machine.
The installer lives at `scripts/install-megafloyd.sh`.
The Linux CI smoke test lives at `.github/workflows/cloud-ci.yml`.

Evidence tags used below: **VERIFIED** means the command was run in this
session and the output is quoted. **HYPOTHESIS** means the behavior is
standard for Debian/Ubuntu but was not executed on a real Linux host in this
session.

## What the installer does

1. Detects `amd64` or `arm64`.
2. Installs missing `git`, `curl`, and `tar` with `apt-get` when it runs as
   root and `--skip-prereqs` is not set.
3. Finds a Go toolchain that satisfies the `go` directive in
   `floyd-go/go.mod` (currently 1.26.6). If none is suitable, it installs
   that version into `<prefix>/libexec/megafloyd/toolchain` and leaves any
   system Go alone.
4. Builds from a checkout, clones the repository, or installs a prebuilt
   binary.
5. Installs the binary to `<prefix>/libexec/megafloyd/megafloyd`.
6. Installs the launcher to `<prefix>/bin/megafloyd`.
7. Creates the `megafloyd` service user and `/home/megafloyd/.megafloyd`
   with `config/`, `data/`, and `cache/` unless `--no-service-user` is set.
8. Prints the post-install commands.

Existing binaries and launchers are moved to
`<prefix>/libexec/megafloyd/backup/<label>.<timestamp>`. Nothing is deleted.
No ports are opened and no systemd unit is installed.

## Requirements

- Debian 12+ or Ubuntu 22.04+ (`apt-get`, `useradd`; systemd is optional).
- Root or `sudo` for the default `/usr/local` prefix and for the service
  user.
- Outbound HTTPS to `github.com` for a repository clone and to `go.dev`
  only when a Go toolchain must be downloaded.

The repository is private. Authenticate `git` on the VM first, for example
with `gh auth login`, or clone on a workstation and copy the checkout to the
VM. The installer script itself does not read, write, or store credentials.

## Flags

Run `bash scripts/install-megafloyd.sh --help` for the full list.

| Flag | Meaning |
| --- | --- |
| `--source-dir DIR` | Build from an existing checkout |
| `--repo URL` | Clone this repository instead (default `https://github.com/CaptainPhantasy/MEGAFLOYD.git`) |
| `--ref REF` | Branch, tag, or commit for `--repo` (default `main`) |
| `--binary PATH` | Install a prebuilt binary instead of building |
| `--go-version VERSION` | Go toolchain to install when no suitable `go` exists |
| `--prefix DIR` | Install prefix (default `/usr/local`) |
| `--service-user NAME` | Service user name (default `megafloyd`) |
| `--no-service-user` | Skip service-user and home creation |
| `--skip-prereqs` | Never install missing `git`, `curl`, or `tar` |
| `--dry-run` | Print every action without changing anything |
| `--unsupported-os` | Proceed on a non-Linux OS for layout testing |
| `-h`, `--help` | Show help |

## Recommended run: dry run first

```bash
git clone https://github.com/CaptainPhantasy/MEGAFLOYD.git
cd MEGAFLOYD
bash scripts/install-megafloyd.sh --dry-run --source-dir "$PWD"
```

Then the real install:

```bash
sudo bash scripts/install-megafloyd.sh --source-dir "$PWD"
```

If the VM has no checkout and can authenticate to GitHub:

```bash
sudo bash scripts/install-megafloyd.sh --repo https://github.com/CaptainPhantasy/MEGAFLOYD.git --ref main
```

Prebuilt binary (cross-compiled on another machine):

```bash
sudo bash scripts/install-megafloyd.sh --binary ./megafloyd
```

User-local install without a service user:

```bash
bash scripts/install-megafloyd.sh --prefix "$HOME/.local" --no-service-user --source-dir "$PWD"
```

## Environment contract

The launcher sets the `MEGAFLOYD_*` namespace and mirrors the values into the
names the current chassis reads. User-supplied values win (`:-` defaults), so
any single path can be overridden.

| Variable | Default | Read by |
|---|---|---|
| `MEGAFLOYD_HOME` | `$HOME/.megafloyd` | launcher |
| `MEGAFLOYD_CONFIG_DIR` | `$MEGAFLOYD_HOME/config` | launcher |
| `MEGAFLOYD_DATA_DIR` | `$MEGAFLOYD_HOME/data` | launcher |
| `MEGAFLOYD_CACHE_DIR` | `$MEGAFLOYD_HOME/cache` | launcher |
| `FLOYD_GLOBAL_CONFIG` | `$MEGAFLOYD_CONFIG_DIR` | `internal/config/load.go:1216` **VERIFIED** |
| `FLOYD_GLOBAL_DATA` | `$MEGAFLOYD_DATA_DIR` | `internal/config/load.go:1263` **VERIFIED** |
| `FLOYD_CACHE_DIR` | `$MEGAFLOYD_CACHE_DIR` | `internal/config/load.go:1239` **VERIFIED** |
| `FLOYD_APP` | `megafloyd` | chassis app name |

The launcher passes `--data-dir "$MEGAFLOYD_DATA_DIR"` unless the invocation
already contains `-D` or `--data-dir`. The flag is defined at
`floyd-go/internal/cmd/root.go:56` (**VERIFIED**).

When the installed binary is named `megafloyd`, the CLI presents itself as
`megafloyd` instead of `floyd` (`internal/cmd/identity.go:31`,
**VERIFIED**). **VERIFIED** end to end with the chassis's own `dirs`
command:

```text
$ MEGAFLOYD_HOME=/tmp/mfl-home.cloudci /tmp/mfl-prefix.cloudci/bin/megafloyd dirs
/tmp/mfl-home.cloudci/config
/tmp/mfl-home.cloudci/data
/etc/floyd
```

The third line is the system config directory (`/etc/floyd/floyd.json` on
Unix). Project-level config files add more lines below it.

## After install

```bash
sudo -u megafloyd -H megafloyd --version
sudo -u megafloyd -H megafloyd login
sudo -u megafloyd -H megafloyd server
```

- `--version` is the CLI version flag (`-v` is the short form).
- The headless server subcommand is `server`, not `serve`.
- `login` and `logout` are registered in `internal/cmd/`.
- The exact authentication flow is documented in `docs/auth-integration.md`.

Home layout:

```text
/home/megafloyd/.megafloyd/
├── cache/
├── config/   # FLOYD_GLOBAL_CONFIG / MEGAFLOYD_CONFIG_DIR
└── data/     # FLOYD_GLOBAL_DATA / MEGAFLOYD_DATA_DIR, sessions, logs
```

## CI smoke test

`.github/workflows/cloud-ci.yml` runs on `ubuntu-latest` for every push, pull
request, and manual dispatch. It:

1. installs Go from the `go` directive in `floyd-go/go.mod` (1.26.6);
2. builds `linux/amd64` with `CGO_ENABLED=0` and `GOEXPERIMENT=greenteagc`;
3. runs the binary's `--version` and `--help`;
4. runs `scripts/install-megafloyd.sh --no-service-user --skip-prereqs`
   into a temporary prefix, capturing the installer output with `tee` and in
   the job summary;
5. runs the installed launcher's `--version` and `dirs`;
6. uploads the captured text files as the `cloud-ci-logs` artifact.

## Phone reachability

This installer deliberately creates no new infrastructure. It opens no ports,
starts no tunnel, and configures no DNS or certificates.

Phone access uses the existing remote and web surface over `server` mode.
If the VM is joined to the owner's existing tailnet, SSH and that surface are
reachable over the same route. Joining a tailnet is an owner action and is
outside this script.

## Upgrade

Re-run the same command. The old binary and launcher are moved to
`<prefix>/libexec/megafloyd/backup/` with a timestamp, then the new files are
installed and `--version` is checked.

## Uninstall

The installer only creates these paths:

```text
/usr/local/bin/megafloyd
/usr/local/libexec/megafloyd/
/home/megafloyd/.megafloyd/
```

Inspect them first. To remove the launcher and libexec tree without deleting
the service user's data, move them into a quarantine directory of your choice:

```bash
sudo install -d -m 0700 /var/backups/megafloyd
sudo mv /usr/local/bin/megafloyd /var/backups/megafloyd/
sudo mv /usr/local/libexec/megafloyd /var/backups/megafloyd/
```

`userdel -r megafloyd` deletes the service user's home and its sessions. It is
not part of this installer and is not run automatically.

## Verification performed for this document

**VERIFIED** on macOS arm64 (layout and launcher behavior):

```text
$ bash -n scripts/install-megafloyd.sh
BASH_N_EXIT=0

$ shellcheck scripts/install-megafloyd.sh
SHELLCHECK_EXIT=0

$ bash scripts/install-megafloyd.sh --dry-run --source-dir "$PWD"
DRYRUN_EXIT=0
```

**VERIFIED** with Go 1.26.6 from a cold cache:

```text
$ cd floyd-go && go build ./...
BUILD_EXIT=0

$ CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOEXPERIMENT=greenteagc \
    go build -trimpath -o /tmp/megafloyd-linux-amd64-greenteagc .
LINUX_GREENTEAGC_EXIT=0

$ file /tmp/megafloyd-linux-amd64-greenteagc
/tmp/megafloyd-linux-amd64-greenteagc: ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, Go BuildID=383l1x7MMDKWjAJI_Y_0/Y9trveiddYorf0lO5vmX/GFhqUbDks3Z2jGWVsas7/zQjFuupr2x-FEnTd2ifP, BuildID[sha1]=524d8e255416fd55fd37534290c41b165c44ecd8, with debug_info, not stripped
```

**VERIFIED** binary and installer behavior:

```text
$ /tmp/cloudci-bin/megafloyd --version
megafloyd version cloud-ci-local
MEGA_VERSION_EXIT=0

$ /tmp/cloudci-bin/megafloyd --help
MEGA_HELP_EXIT=0

$ bash scripts/install-megafloyd.sh --unsupported-os --no-service-user \
    --prefix /tmp/mfl-prefix.cloudci.9SKxTz --binary /tmp/cloudci-bin/megafloyd
[megafloyd-install] verified: megafloyd version cloud-ci-local
INSTALL_EXIT=0

$ MEGAFLOYD_HOME=/tmp/mfl-home.cloudci /tmp/mfl-prefix.cloudci.9SKxTz/bin/megafloyd --version
megafloyd version cloud-ci-local
LAUNCHER_VERSION_EXIT=0
```

**VERIFIED** workflow syntax (not its remote execution):

```text
$ yq eval '.' .github/workflows/cloud-ci.yml
YQ_EXIT=0

$ go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/cloud-ci.yml
ACTIONLINT_EXIT=0
```

**NOT VERIFIED in this session:** `apt-get` prerequisite installation,
`groupadd`/`useradd` service-user creation, the Linux Go tarball download,
and the full workflow on `ubuntu-latest`. The Linux path is covered by
`.github/workflows/cloud-ci.yml`; its first remote run is the evidence for
those steps.
