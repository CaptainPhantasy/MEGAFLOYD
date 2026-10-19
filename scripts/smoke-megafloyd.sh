#!/usr/bin/env bash
#
# Smoke-test a built megafloyd binary.
#
# Runs the binary's --version and --help. Any nonzero exit fails the smoke
# test. The default target is the host build produced by
# scripts/build-megafloyd.sh.
#
# Usage:
#   scripts/smoke-megafloyd.sh [path-to-binary]
#
# Environment:
#   MEGAFLOYD_BIN   Binary to test. Overrides the positional argument.
#
set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
REPO_ROOT=$(cd -- "$SCRIPT_DIR/.." && pwd)

host_binary() {
	local goos goarch
	goos=$(go env GOOS 2>/dev/null || uname -s | tr '[:upper:]' '[:lower:]')
	goarch=$(go env GOARCH 2>/dev/null || uname -m)
	case "$goarch" in
	x86_64) goarch=amd64 ;;
	aarch64) goarch=arm64 ;;
	esac
	printf '%s/dist/%s-%s/megafloyd\n' "$REPO_ROOT" "$goos" "$goarch"
}

bin=${MEGAFLOYD_BIN:-${1:-$(host_binary)}}

if [[ ! -f $bin ]]; then
	printf 'error: binary not found: %s\n' "$bin" >&2
	printf 'build it first with: scripts/build-megafloyd.sh --host\n' >&2
	exit 1
fi
if [[ ! -x $bin ]]; then
	printf 'error: binary is not executable: %s\n' "$bin" >&2
	exit 1
fi

run_check() {
	local flag=$1 output status

	set +e
	output=$("$bin" "$flag" 2>&1)
	status=$?
	set -e

	if ((status != 0)); then
		printf 'FAIL: %s %s exited %d\n' "$bin" "$flag" "$status" >&2
		printf '%s\n' "$output" >&2
		exit "$status"
	fi

	if [[ -z $output ]]; then
		printf 'FAIL: %s %s produced no output\n' "$bin" "$flag" >&2
		exit 1
	fi

	printf '%s' "$output"
}

printf '==> %s --version\n' "$bin"
version_output=$(run_check --version)
printf '%s\n' "$version_output"

printf '==> %s --help\n' "$bin"
run_check --help >/dev/null
printf 'help: OK\n'

printf 'smoke: OK (%s)\n' "$version_output"
