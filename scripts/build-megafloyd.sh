#!/usr/bin/env bash
#
# Reproducible cross-platform build for the megafloyd binary.
#
# Builds the Go chassis in floyd-go/ with CGO disabled, stamps the release
# version and commit into internal/version via -ldflags, and writes each
# artifact to dist/<os>-<arch>/megafloyd.
#
# Reproducibility: -trimpath strips local paths, -buildvcs=false keeps the
# build independent of worktree VCS state, and the version/commit inputs are
# explicit so the same source and inputs yield the same compile-time values.
#
# Usage:
#   scripts/build-megafloyd.sh                  # all four shipped targets
#   scripts/build-megafloyd.sh --host           # host os/arch only
#   scripts/build-megafloyd.sh -t linux/amd64   # repeat -t for more
#   MEGAFLOYD_VERSION=v1.2.3 scripts/build-megafloyd.sh
#
# Environment:
#   MEGAFLOYD_VERSION   Version string stamped into the binary.
#   GOEXPERIMENT        Defaults to greenteagc (matches floyd-go/Taskfile.yaml).
#
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)
GO_DIR="$REPO_ROOT/floyd-go"
DIST_DIR="$REPO_ROOT/dist"
MODULE_PATH="github.com/legacy-ai/floyd"
DEFAULT_TARGETS=(darwin/arm64 darwin/amd64 linux/amd64 linux/arm64)

targets=()
version=""
dist_dir="$DIST_DIR"
strip=true

usage() {
	cat <<'EOF'
Build the megafloyd binary for darwin/arm64, darwin/amd64, linux/amd64 and
linux/arm64 with CGO disabled.

Usage:
  scripts/build-megafloyd.sh [options]

Options:
  -t, --target OS/ARCH   Build one target; repeatable. Default: all four.
      --host             Build only the host OS/ARCH.
      --all              Build all four shipped targets (the default).
  -o, --out DIR          Output directory. Default: <repo>/dist.
  -v, --version STRING   Version to stamp. Default: MEGAFLOYD_VERSION, then
                         git describe --tags --always --dirty, then "devel".
      --no-strip         Keep debug symbols (omit -s -w from ldflags).
  -h, --help             Show this help.

Artifacts:
  dist/<os>-<arch>/megafloyd   Binary per target.
  dist/SHA256SUMS              Checksums for every artifact built.

The version is readable at runtime with `megafloyd --version` (also `-v`).
EOF
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

while (($#)); do
	case "$1" in
	-t | --target)
		[[ $# -ge 2 ]] || die "--target needs an OS/ARCH value"
		targets+=("$2")
		shift 2
		;;
	--host)
		targets+=("$(go env GOOS)/$(go env GOARCH)")
		shift
		;;
	--all)
		targets=("${DEFAULT_TARGETS[@]}")
		shift
		;;
	-o | --out)
		[[ $# -ge 2 ]] || die "--out needs a directory"
		dist_dir=$2
		shift 2
		;;
	-v | --version)
		[[ $# -ge 2 ]] || die "--version needs a value"
		version=$2
		shift 2
		;;
	--no-strip)
		strip=false
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		printf 'error: unknown option: %s\n\n' "$1" >&2
		usage >&2
		exit 2
		;;
	esac
done

command -v go >/dev/null 2>&1 || die "go toolchain not found in PATH"
[[ -f "$GO_DIR/go.mod" ]] || die "missing $GO_DIR/go.mod; is this the megafloyd repo?"

if ((${#targets[@]} == 0)); then
	targets=("${DEFAULT_TARGETS[@]}")
fi

for target in "${targets[@]}"; do
	[[ $target == */* && ${target%%/*} != "" && ${target##*/} != "" ]] ||
		die "invalid target '$target' (want OS/ARCH, e.g. linux/amd64)"
done

if [[ -z $version ]]; then
	version=${MEGAFLOYD_VERSION:-}
fi
if [[ -z $version ]]; then
	if ! version=$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null); then
		version=devel
	fi
fi
[[ -n $version ]] || version=devel

commit=$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo unknown)

ldflags="-X ${MODULE_PATH}/internal/version.Version=${version}"
ldflags+=" -X ${MODULE_PATH}/internal/version.Commit=${commit}"
if [[ $strip == true ]]; then
	ldflags="-s -w $ldflags"
fi

export CGO_ENABLED=0
if [[ -z ${GOEXPERIMENT:-} ]]; then
	export GOEXPERIMENT=greenteagc
fi

mkdir -p "$dist_dir"
dist_dir=$(cd -- "$dist_dir" && pwd)

artifacts=()
for target in "${targets[@]}"; do
	goos=${target%%/*}
	goarch=${target##*/}
	out="$dist_dir/${goos}-${goarch}/megafloyd"
	if [[ $goos == windows ]]; then
		out+=".exe"
	fi
	mkdir -p "$(dirname -- "$out")"
	printf '==> building %s -> %s\n' "$target" "$out"
	(
		cd "$GO_DIR"
		GOOS="$goos" GOARCH="$goarch" go build \
			-trimpath \
			-buildvcs=false \
			-ldflags "$ldflags" \
			-o "$out" \
			.
	)
	artifacts+=("$out")
done

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

checksums="$dist_dir/SHA256SUMS"
: >"$checksums"
for artifact in "${artifacts[@]}"; do
	relative=${artifact#"$dist_dir"/}
	printf '%s  %s\n' "$(sha256 "$artifact")" "$relative" >>"$checksums"
done

printf '\nversion: %s\ncommit:  %s\n' "$version" "$commit"
printf 'artifacts:\n'
for artifact in "${artifacts[@]}"; do
	size=$(wc -c <"$artifact" | tr -d ' ')
	printf '  %-45s %8s bytes\n' "${artifact#"$dist_dir"/}" "$size"
done
printf '  %-45s\n' "SHA256SUMS"
