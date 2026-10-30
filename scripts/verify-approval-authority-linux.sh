#!/usr/bin/env bash
# Exercise the protected authority on an isolated hosted Linux test fixture.
set -euo pipefail

if [[ "$(uname -s)" != Linux ]]; then
  echo 'This integration check requires Linux per-message credentials.' >&2
  exit 77
fi
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
engine_uid="$(id -u nobody)"
engine_gid="$(id -g nobody)"
[[ "$engine_uid" != 0 ]]
sudo -n true
build_dir="$(mktemp -d "${RUNNER_TEMP:-/tmp}/megafloyd-approval-build.XXXXXX")"
fixture="$(sudo -n mktemp -d /run/megafloyd-approval-test.XXXXXX)"
sudo -n chmod 755 "$fixture"
CGO_ENABLED=0 go -C "$repo_root/floyd-go" build -o "$build_dir/authority" ./cmd/megafloyd-approval-authority
CGO_ENABLED=0 go -C "$repo_root/floyd-go" test -c -o "$build_dir/engine.test" ./internal/deletionguard
sudo -n install -o root -g root -m 755 "$build_dir/authority" "$fixture/authority"
sudo -n install -o root -g root -m 755 "$build_dir/engine.test" "$fixture/engine.test"
sudo -n install -d -o root -g root -m 700 "$fixture/state"
sudo -n install -d -o "$engine_uid" -g "$engine_gid" -m 700 "$fixture/workspace" "$fixture/tmp"
engine_sha="$(sha256sum "$fixture/engine.test" | cut -d ' ' -f 1)"
socket_path="$fixture/authority.sock"
broker_pid=''
broker_job=''
stop_broker() {
  if [[ -n "$broker_pid" ]]; then
    if sudo -n kill -0 "$broker_pid" 2>/dev/null; then
      sudo -n kill -TERM "$broker_pid"
    fi
    local stop_status=0
    wait "$broker_job" || stop_status=$?
    broker_pid=''
    broker_job=''
    return "$stop_status"
  fi
}
cleanup_broker() {
  local original_status=$?
  trap - EXIT
  local cleanup_status=0
  stop_broker || cleanup_status=$?
  if (( original_status != 0 || cleanup_status != 0 )); then
    if [[ -f "$build_dir/authority.log" ]]; then cat "$build_dir/authority.log" >&2; fi
  fi
  if (( original_status == 0 )); then original_status=$cleanup_status; fi
  exit "$original_status"
}
trap cleanup_broker EXIT
start_broker() {
  local phase="$1" old_inode=''
  if [[ -S "$socket_path" ]]; then old_inode="$(stat -c '%i' "$socket_path")"; fi
  # The runner opens its own diagnostic log; only the broker process is root.
  {
  sudo -n /bin/sh -c 'printf "%s\n" "$$" > "$1"; shift; exec "$@"' sh \
    "$fixture/pid-$phase" "$fixture/authority" \
    --socket "$socket_path" --state-dir "$fixture/state" \
    --allow-uid "$engine_uid" --allow-executable "$fixture/engine.test=$engine_sha"
  } >> "$build_dir/authority.log" 2>&1 &
  broker_job=$!
  for ((attempt=0; attempt<100; attempt++)); do
    if [[ -f "$fixture/pid-$phase" ]]; then broker_pid="$(cat "$fixture/pid-$phase")"; fi
    if [[ -n "$broker_pid" ]] && ! sudo -n kill -0 "$broker_pid" 2>/dev/null; then
      cat "$build_dir/authority.log" >&2
      wait "$broker_job"
      return 1
    fi
    if [[ -S "$socket_path" && "$(stat -c '%i' "$socket_path")" != "$old_inode" ]]; then
      if python3 - "$socket_path" <<'PY'
import socket, sys
with socket.socket(socket.AF_UNIX) as connection:
    connection.settimeout(1)
    connection.connect(sys.argv[1])
PY
      then return 0; fi
    fi
    sleep 0.1
  done
  cat "$build_dir/authority.log" >&2
  echo 'Protected authority did not become ready.' >&2
  return 1
}
run_phase() {
  sudo -n -u nobody env \
    TMPDIR="$fixture/tmp" \
    MEGAFLOYD_APPROVAL_TEST_SOCKET="$socket_path" \
    MEGAFLOYD_APPROVAL_TEST_PHASE="$1" \
    MEGAFLOYD_APPROVAL_TEST_FIXTURE="$fixture/workspace" \
    "$fixture/engine.test" -test.run '^TestProtectedAuthorityProcessIntegration$' -test.v
}
start_broker prepare
run_phase prepare
stop_broker
start_broker retry
run_phase retry
# A general-purpose process is not a pinned engine, and the OS must deny its
# direct access to the private store. This fixture never removes test targets.
sudo -n -u nobody python3 - "$socket_path" "$fixture/state/unprivileged-write-probe" <<'PY'
import json, socket, sys
with socket.socket(socket.AF_UNIX) as connection:
    connection.settimeout(3)
    connection.connect(sys.argv[1])
    response = json.loads(connection.recv(4096))
    assert response.get('error') == 'approval authority rejected peer identity', response
try:
    open(sys.argv[2], 'x').close()
except PermissionError:
    pass
else:
    raise AssertionError('unprivileged account accessed the protected store')
print('Unpinned client and direct private-store write both refused.')
PY
stop_broker
trap - EXIT
cat "$build_dir/authority.log"
echo "Protected Linux authority: separate UID, separate engine PIDs, broker restart, two confirmations, exact remainder, stale CAS, local-record rejection, no-repeat: PASS."
echo "Isolated fixtures retained at $fixture and $build_dir."
