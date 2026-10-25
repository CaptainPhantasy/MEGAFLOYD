#!/usr/bin/env bash
# install-megafloyd.sh — one-command installer for megafloyd on Debian/Ubuntu VMs.
#
# What it does:
#   1. detects the CPU architecture (amd64 / arm64)
#   2. installs build prerequisites if they are missing (root + apt only)
#   3. installs a pinned Go toolchain only when no suitable `go` is present
#   4. builds megafloyd from a repo clone, or installs a prebuilt binary
#   5. installs the binary to <prefix>/libexec/megafloyd/megafloyd
#   6. installs the launcher to <prefix>/bin/megafloyd
#   7. creates a dedicated service user and its ~/.megafloyd home
#
# Safety:
#   * `--dry-run` prints every change without making it.
#   * Existing binaries and launchers are moved to a timestamped backup
#     directory, never deleted.
#   * No network ports are opened, no systemd unit is installed, and no
#     secrets are written.
set -euo pipefail

readonly PROG="megafloyd"
readonly DEFAULT_REPO="https://github.com/CaptainPhantasy/MEGAFLOYD.git"
readonly DEFAULT_REF="main"
readonly DEFAULT_PREFIX="/usr/local"
readonly DEFAULT_SERVICE_USER="megafloyd"
readonly FALLBACK_GO_VERSION="1.26.6"
readonly GO_MOD_SUBDIR="floyd-go"
readonly VERSION_PKG="github.com/legacy-ai/floyd/internal/version"

PREFIX="$DEFAULT_PREFIX"
SERVICE_USER="$DEFAULT_SERVICE_USER"
CREATE_SERVICE_USER=1
SOURCE_DIR=""
BINARY_PATH=""
REPO_URL="$DEFAULT_REPO"
REF="$DEFAULT_REF"
GO_VERSION=""
DRY_RUN=0
SKIP_PREREQS=0
UNSUPPORTED_OS=0

OS=""
ARCH=""
TMP_DIR=""
SRC_DIR=""
GO_BIN=""
LIBEXEC=""
LAUNCHER=""
BIN_DEST=""
BACKUP_DIR=""
SERVICE_HOME=""

log() { printf '[%s-install] %s\n' "$PROG" "$*"; }
warn() { printf '[%s-install] WARNING: %s\n' "$PROG" "$*" >&2; }
die() {
	printf '[%s-install] ERROR: %s\n' "$PROG" "$*" >&2
	exit 1
}

# run executes a command, or prints it verbatim when --dry-run is set.
run() {
	if [ "$DRY_RUN" -eq 1 ]; then
		printf '  +'
		printf ' %q' "$@"
		printf '\n'
		return 0
	fi
	"$@"
}

usage() {
	cat <<'EOF'
install-megafloyd.sh — install megafloyd on a Debian/Ubuntu VM.

USAGE
  sudo bash scripts/install-megafloyd.sh [options]

BUILD SOURCES (pick one)
  --source-dir DIR      build from an existing repo checkout
  --repo URL            clone this repository (default: private MEGAFLOYD repo)
  --ref REF             branch, tag or commit to build (default: main)
  --binary PATH         install a prebuilt megafloyd binary instead of building
  --go-version VERSION  Go toolchain to install if none is suitable
                        (default: the go directive in floyd-go/go.mod, else 1.26.6)

INSTALL LOCATION
  --prefix DIR          install prefix (default: /usr/local)
  --service-user NAME   dedicated service user (default: megafloyd)
  --no-service-user     do not create a service user or its home directory

OTHER
  --skip-prereqs        do not install missing git/curl/tar packages
  --dry-run             print every action without changing anything
  --unsupported-os      proceed on a non-Linux OS (layout testing only)
  -h, --help            show this help

EXAMPLES
  # inspect the plan first
  bash scripts/install-megafloyd.sh --dry-run --source-dir "$PWD"

  # build and install from this checkout, as root
  sudo bash scripts/install-megafloyd.sh --source-dir "$PWD"

  # install a binary built on another machine
  sudo bash scripts/install-megafloyd.sh --binary ./megafloyd

After install:
  sudo -u megafloyd -H megafloyd --version
  sudo -u megafloyd -H megafloyd login
  sudo -u megafloyd -H megafloyd server
EOF
}

parse_args() {
	while [ "$#" -gt 0 ]; do
		case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--dry-run)
			DRY_RUN=1
			;;
		--prefix)
			[ "$#" -ge 2 ] || die "--prefix needs a directory"
			PREFIX="$2"
			shift
			;;
		--prefix=*)
			PREFIX="${1#*=}"
			;;
		--service-user)
			[ "$#" -ge 2 ] || die "--service-user needs a name"
			SERVICE_USER="$2"
			shift
			;;
		--service-user=*)
			SERVICE_USER="${1#*=}"
			;;
		--no-service-user)
			CREATE_SERVICE_USER=0
			;;
		--source-dir)
			[ "$#" -ge 2 ] || die "--source-dir needs a directory"
			SOURCE_DIR="$2"
			shift
			;;
		--source-dir=*)
			SOURCE_DIR="${1#*=}"
			;;
		--binary)
			[ "$#" -ge 2 ] || die "--binary needs a path"
			BINARY_PATH="$2"
			shift
			;;
		--binary=*)
			BINARY_PATH="${1#*=}"
			;;
		--repo)
			[ "$#" -ge 2 ] || die "--repo needs a URL"
			REPO_URL="$2"
			shift
			;;
		--repo=*)
			REPO_URL="${1#*=}"
			;;
		--ref)
			[ "$#" -ge 2 ] || die "--ref needs a branch, tag or commit"
			REF="$2"
			shift
			;;
		--ref=*)
			REF="${1#*=}"
			;;
		--go-version)
			[ "$#" -ge 2 ] || die "--go-version needs a version"
			GO_VERSION="$2"
			shift
			;;
		--go-version=*)
			GO_VERSION="${1#*=}"
			;;
		--skip-prereqs)
			SKIP_PREREQS=1
			;;
		--unsupported-os)
			UNSUPPORTED_OS=1
			;;
		*)
			die "unknown option: $1 (try --help)"
			;;
		esac
		shift
	done

	if [ -n "$SOURCE_DIR" ] && [ -n "$BINARY_PATH" ]; then
		die "--source-dir and --binary are mutually exclusive"
	fi
	case "$PREFIX" in
	/*) ;;
	*) die "--prefix must be an absolute path" ;;
	esac
	LIBEXEC="$PREFIX/libexec/$PROG"
	LAUNCHER="$PREFIX/bin/$PROG"
	BIN_DEST="$LIBEXEC/$PROG"
	BACKUP_DIR="$LIBEXEC/backup"
}

detect_platform() {
	OS="$(uname -s)"
	case "$(uname -m)" in
	x86_64 | amd64) ARCH="amd64" ;;
	aarch64 | arm64) ARCH="arm64" ;;
	*) die "unsupported CPU architecture: $(uname -m)" ;;
	esac

	if [ "$OS" != "Linux" ]; then
		if [ "$UNSUPPORTED_OS" -eq 1 ]; then
			warn "non-Linux OS detected ($OS); continuing because --unsupported-os was passed"
		elif [ "$DRY_RUN" -eq 1 ]; then
			warn "this installer targets Linux; detected $OS — dry run continues"
		else
			die "this installer targets Debian/Ubuntu Linux; detected $OS (use --unsupported-os only for layout testing)"
		fi
	fi
	log "platform: ${OS}/${ARCH}"
}

make_tmp_dir() {
	TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/megafloyd-install.XXXXXX")"
	log "work directory: $TMP_DIR"
}

cleanup() {
	[ -n "$TMP_DIR" ] || return 0
	[ -d "$TMP_DIR" ] || return 0
	case "$TMP_DIR" in
	/tmp/megafloyd-install.* | /var/tmp/megafloyd-install.*)
		rm -rf -- "$TMP_DIR"
		;;
	*)
		warn "leaving unexpected work directory in place: $TMP_DIR"
		;;
	esac
}

require_privileges() {
	if [ "$(id -u)" -eq 0 ]; then
		return 0
	fi
	if [ "$DRY_RUN" -eq 1 ]; then
		warn "not running as root; the dry run only prints the plan"
		return 0
	fi
	if [ "$CREATE_SERVICE_USER" -eq 1 ]; then
		die "run as root (sudo), or pass --no-service-user with a writable --prefix"
	fi
	if [ ! -w "$PREFIX" ] && [ ! -w "$(dirname "$PREFIX")" ]; then
		die "install prefix is not writable: $PREFIX"
	fi
}

# version_ge returns success when $1 >= $2 using version sort.
version_ge() {
	local lowest
	lowest="$(printf '%s\n%s\n' "$2" "$1" | sort -V | sed -n '1p')"
	[ "$lowest" = "$2" ]
}

install_prereqs() {
	local missing=()
	local cmd
	for cmd in git curl tar; do
		if ! command -v "$cmd" >/dev/null 2>&1; then
			missing+=("$cmd")
		fi
	done
	if [ "${#missing[@]}" -eq 0 ]; then
		return 0
	fi
	log "missing prerequisites: ${missing[*]}"
	if [ "$SKIP_PREREQS" -eq 1 ]; then
		die "install these commands first or drop --skip-prereqs: ${missing[*]}"
	fi
	if ! command -v apt-get >/dev/null 2>&1; then
		if [ "$DRY_RUN" -eq 1 ]; then
			warn "apt-get not found; a real run would stop here"
			return 0
		fi
		die "apt-get not found; this installer supports Debian/Ubuntu only"
	fi
	run apt-get update
	run env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${missing[@]}"
}

required_go_version() {
	if [ -n "$GO_VERSION" ]; then
		printf '%s\n' "$GO_VERSION"
		return 0
	fi
	if [ -n "$SRC_DIR" ] && [ -f "$SRC_DIR/$GO_MOD_SUBDIR/go.mod" ]; then
		local from_mod
		from_mod="$(awk '$1 == "go" { print $2; exit }' "$SRC_DIR/$GO_MOD_SUBDIR/go.mod")"
		if [ -n "$from_mod" ]; then
			printf '%s\n' "$from_mod"
			return 0
		fi
	fi
	printf '%s\n' "$FALLBACK_GO_VERSION"
}

install_go() {
	local want="$1"
	local dest="$LIBEXEC/toolchain"
	local tarball="$TMP_DIR/go.tar.gz"
	local goos="linux"
	local url

	# The product target is Linux. The Darwin branch exists only so the
	# layout-testing escape hatch can build a working toolchain.
	if [ "$OS" = "Darwin" ]; then
		goos="darwin"
	fi
	url="https://go.dev/dl/go${want}.${goos}-${ARCH}.tar.gz"

	log "installing Go ${want} (${goos}-${ARCH}) into ${dest}"
	run mkdir -p "$dest"
	run curl -fL --retry 3 --proto '=https' --tlsv1.2 -o "$tarball" "$url"
	run tar -C "$dest" -xzf "$tarball"
	GO_BIN="$dest/go/bin/go"
}

ensure_go() {
	local want
	want="$(required_go_version)"
	log "required Go version: ${want}"

	if command -v go >/dev/null 2>&1; then
		local have
		have="$(go version | awk '{ print $3 }')"
		have="${have#go}"
		if version_ge "$have" "$want"; then
			GO_BIN="$(command -v go)"
			log "using existing Go ${have}: ${GO_BIN}"
			return 0
		fi
		warn "existing Go ${have} is older than ${want}"
	fi

	install_go "$want"
}

resolve_source() {
	if [ -n "$BINARY_PATH" ]; then
		if [ "$DRY_RUN" -eq 0 ]; then
			[ -f "$BINARY_PATH" ] || die "binary not found: $BINARY_PATH"
			BINARY_PATH="$(cd "$(dirname "$BINARY_PATH")" && pwd -P)/$(basename "$BINARY_PATH")"
		fi
		log "prebuilt binary: $BINARY_PATH"
		return 0
	fi

	if [ -n "$SOURCE_DIR" ]; then
		[ -d "$SOURCE_DIR" ] || die "source directory not found: $SOURCE_DIR"
		[ -d "$SOURCE_DIR/$GO_MOD_SUBDIR" ] || die "not a megafloyd checkout (missing $GO_MOD_SUBDIR/): $SOURCE_DIR"
		SRC_DIR="$(cd "$SOURCE_DIR" && pwd -P)"
		log "source checkout: $SRC_DIR"
		return 0
	fi

	SRC_DIR="$TMP_DIR/src"
	log "cloning $REPO_URL at ref $REF"
	if [ "$DRY_RUN" -eq 0 ]; then
		if ! git clone --depth 1 --branch "$REF" "$REPO_URL" "$SRC_DIR"; then
			warn "shallow clone of '$REF' failed; cloning all refs into a second directory"
			SRC_DIR="$TMP_DIR/src-full"
			run git clone "$REPO_URL" "$SRC_DIR"
			run git -C "$SRC_DIR" checkout "$REF"
		fi
	else
		run git clone --depth 1 --branch "$REF" "$REPO_URL" "$SRC_DIR"
	fi
}

detect_version() {
	if git -C "$SRC_DIR" rev-parse --git-dir >/dev/null 2>&1; then
		git -C "$SRC_DIR" describe --tags --always --dirty 2>/dev/null || printf 'dev\n'
	else
		printf 'dev\n'
	fi
}

build_binary() {
	if [ -n "$BINARY_PATH" ]; then
		BUILT_BIN="$BINARY_PATH"
		return 0
	fi
	BUILT_BIN="$TMP_DIR/$PROG"
	local version
	version="$(detect_version)"
	local ldflags="-s -w -X ${VERSION_PKG}.Version=${version}"
	log "building ${PROG} ${version} for ${OS}/${ARCH}"

	if [ "$DRY_RUN" -eq 1 ]; then
		printf '  + cd %q && CGO_ENABLED=0 GOEXPERIMENT=greenteagc GOTOOLCHAIN=local %q build -trimpath -ldflags %q -o %q .\n' \
			"$SRC_DIR/$GO_MOD_SUBDIR" "$GO_BIN" "$ldflags" "$BUILT_BIN"
		return 0
	fi

	(
		cd "$SRC_DIR/$GO_MOD_SUBDIR"
		env CGO_ENABLED=0 GOEXPERIMENT=greenteagc GOTOOLCHAIN=local \
			"$GO_BIN" build -trimpath -ldflags "$ldflags" -o "$BUILT_BIN" .
	)
	log "built: $BUILT_BIN"
}

backup_if_exists() {
	local path="$1" label="$2" ts dest
	if [ ! -e "$path" ]; then
		return 0
	fi
	ts="$(date +%Y%m%d%H%M%S)"
	dest="$BACKUP_DIR/${label}.${ts}"
	log "backing up $path -> $dest"
	run mkdir -p "$BACKUP_DIR"
	run mv "$path" "$dest"
}

write_launcher() {
	local staged="$TMP_DIR/${PROG}-launcher"
	cat >"$staged" <<EOF
#!/usr/bin/env bash
# ${PROG} launcher — generated by scripts/install-megafloyd.sh
set -euo pipefail

MEGAFLOYD_BIN="\${MEGAFLOYD_BIN:-$BIN_DEST}"

if [ -z "\${HOME:-}" ]; then
	HOME="\$(getent passwd "\$(id -u)" | cut -d: -f6)"
	export HOME
fi

MEGAFLOYD_HOME="\${MEGAFLOYD_HOME:-\$HOME/.$PROG}"
MEGAFLOYD_CONFIG_DIR="\${MEGAFLOYD_CONFIG_DIR:-\$MEGAFLOYD_HOME/config}"
MEGAFLOYD_DATA_DIR="\${MEGAFLOYD_DATA_DIR:-\$MEGAFLOYD_HOME/data}"
MEGAFLOYD_CACHE_DIR="\${MEGAFLOYD_CACHE_DIR:-\$MEGAFLOYD_HOME/cache}"
export MEGAFLOYD_HOME MEGAFLOYD_CONFIG_DIR MEGAFLOYD_DATA_DIR MEGAFLOYD_CACHE_DIR

# Compatibility mirrors: the current chassis reads these names.
export FLOYD_GLOBAL_CONFIG="\$MEGAFLOYD_CONFIG_DIR"
export FLOYD_GLOBAL_DATA="\$MEGAFLOYD_DATA_DIR"
export FLOYD_CACHE_DIR="\$MEGAFLOYD_CACHE_DIR"
export FLOYD_APP="$PROG"
export MEGAFLOYD_ACTIVE=1

mkdir -p "\$MEGAFLOYD_CONFIG_DIR" "\$MEGAFLOYD_DATA_DIR" "\$MEGAFLOYD_CACHE_DIR" 2>/dev/null || true

if [ ! -x "\$MEGAFLOYD_BIN" ]; then
	printf '%s launcher: missing executable %s\n' "$PROG" "\$MEGAFLOYD_BIN" >&2
	exit 127
fi

# Let the runtime select <cwd>/.megafloyd unless --data-dir is explicit.
exec "\$MEGAFLOYD_BIN" "\$@"
EOF
	run install -d -m 0755 "$PREFIX/bin" "$LIBEXEC"
	run install -m 0755 "$staged" "$LAUNCHER"
}

install_binary() {
	run mkdir -p "$PREFIX/bin" "$LIBEXEC" "$BACKUP_DIR"
	backup_if_exists "$BIN_DEST" "binary"
	run install -m 0755 "$BUILT_BIN" "$BIN_DEST"
	backup_if_exists "$LAUNCHER" "launcher"
	write_launcher
	log "installed binary: $BIN_DEST"
	log "installed launcher: $LAUNCHER"
}

setup_service_user() {
	if [ "$CREATE_SERVICE_USER" -eq 0 ]; then
		log "service user creation skipped (--no-service-user)"
		return 0
	fi

	if id -u "$SERVICE_USER" >/dev/null 2>&1; then
		SERVICE_HOME="$(getent passwd "$SERVICE_USER" | cut -d: -f6)"
		log "service user exists: $SERVICE_USER (home: $SERVICE_HOME)"
	else
		SERVICE_HOME="/home/$SERVICE_USER"
		if ! getent group "$SERVICE_USER" >/dev/null 2>&1; then
			run groupadd --system "$SERVICE_USER"
		fi
		run mkdir -p /home
		run useradd --system --gid "$SERVICE_USER" --create-home \
			--home-dir "$SERVICE_HOME" --shell /usr/sbin/nologin "$SERVICE_USER"
		log "created service user: $SERVICE_USER (home: $SERVICE_HOME)"
	fi

	run mkdir -p "$SERVICE_HOME/.$PROG/config" \
		"$SERVICE_HOME/.$PROG/data" \
		"$SERVICE_HOME/.$PROG/cache"
	run chown -R "$SERVICE_USER:$SERVICE_USER" "$SERVICE_HOME/.$PROG"
	run chmod 0700 "$SERVICE_HOME/.$PROG"
	log "app home ready: $SERVICE_HOME/.$PROG"
}

verify_install() {
	if [ "$DRY_RUN" -eq 1 ]; then
		log "dry run: skipping post-install verification"
		return 0
	fi
	local reported
	reported="$("$BIN_DEST" --version 2>&1)" || die "installed binary failed to report its version"
	log "verified: $reported"
}

print_next_steps() {
	local run_as="megafloyd"
	if [ "$CREATE_SERVICE_USER" -eq 0 ] || [ "$(id -u)" -ne 0 ]; then
		run_as="$(id -un)"
	fi
	local home_display="$SERVICE_HOME"
	if [ -z "$home_display" ]; then
		home_display="$HOME/.$PROG"
	fi
	if [ -n "$SERVICE_HOME" ]; then
		home_display="$SERVICE_HOME/.$PROG"
	fi

	cat <<EOF

megafloyd is installed.

  binary    $BIN_DEST
  launcher  $LAUNCHER
  app home  $home_display
  user      $run_as

Next steps (headless VM):
  1. Check the build:
       sudo -u $run_as -H $PROG --version
  2. Sign in to the owner's auth server:
       sudo -u $run_as -H $PROG login
  3. Start the headless server:
       sudo -u $run_as -H $PROG server

Environment overrides (all optional):
  MEGAFLOYD_HOME, MEGAFLOYD_CONFIG_DIR, MEGAFLOYD_DATA_DIR, MEGAFLOYD_CACHE_DIR

This installer did not open any network ports and did not install a service
unit. Use the owner's existing remote/web path for phone access; see
docs/cloud-vm.md.
EOF
}

main() {
	parse_args "$@"
	detect_platform
	make_tmp_dir
	trap cleanup EXIT

	if [ "$DRY_RUN" -eq 1 ]; then
		log "dry run: no system changes will be made"
	fi

	require_privileges
	install_prereqs
	resolve_source
	if [ -z "$BINARY_PATH" ]; then
		ensure_go
	fi
	build_binary
	install_binary
	setup_service_user
	verify_install
	print_next_steps
}

main "$@"
