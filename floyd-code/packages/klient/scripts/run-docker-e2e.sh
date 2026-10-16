#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
PACKAGE_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
REPO_ROOT="$(cd -- "${PACKAGE_DIR}/../.." && pwd)"

workspace_slug="$(
  basename -- "${REPO_ROOT}" \
    | tr '[:upper:]' '[:lower:]' \
    | tr -cs 'a-z0-9_.-' '-' \
    | sed -e 's/^[^a-z0-9]*//' -e 's/[^a-z0-9]*$//' \
    | cut -c1-48
)"
if [[ -z "${workspace_slug}" ]]; then
  workspace_slug="workspace"
fi
workspace_hash="$(printf '%s' "${REPO_ROOT}" | cksum | awk '{print $1}')"
RUN_ID="${FLOYD_SERVER_E2E_RUN_ID:-${workspace_slug}-${workspace_hash}}"

BASE_IMAGE="${FLOYD_SERVER_E2E_BASE_IMAGE:-floyd-server-e2e-base:${RUN_ID}}"
IMAGE="${FLOYD_SERVER_E2E_IMAGE:-floyd-server-e2e:${RUN_ID}}"
CONTAINER="${FLOYD_SERVER_E2E_CONTAINER:-floyd-server-e2e-${RUN_ID}}"
STATE_ROOT="${FLOYD_SERVER_E2E_STATE_ROOT:-${HOME}/.floyd-code-server-dev}"
PORT="${FLOYD_SERVER_E2E_PORT:-58627}"

FLOYD_HOME_HOST="${FLOYD_SERVER_E2E_FLOYD_HOME_HOST:-${STATE_ROOT}/docker-e2e/${RUN_ID}/floyd-code-home}"
FLOYD_HOME_CONTAINER="/data/docker-e2e/floyd-code-home"
SEED_HOME_HOST="${FLOYD_SERVER_E2E_SEED_FLOYD_HOME_HOST:-${STATE_ROOT}/floyd-home/floyd-code-home}"

if [[ -n "${FLOYD_SERVER_E2E_REPORT_DIR_HOST:-}" ]]; then
  REPORT_DIR_HOST="${FLOYD_SERVER_E2E_REPORT_DIR_HOST}"
  REPORT_ROOT_HOST="$(dirname -- "${REPORT_DIR_HOST}")"
  REPORT_DIR_NAME="$(basename -- "${REPORT_DIR_HOST}")"
else
  REPORT_ROOT_HOST="${FLOYD_SERVER_E2E_REPORT_ROOT_HOST:-${STATE_ROOT}/server-e2e-reports/docker/${RUN_ID}}"
  REPORT_DIR_NAME="latest"
  REPORT_DIR_HOST="${REPORT_ROOT_HOST}/${REPORT_DIR_NAME}"
fi
REPORT_ROOT_CONTAINER="/data/server-e2e-reports/docker"
REPORT_DIR_CONTAINER="${REPORT_ROOT_CONTAINER}/${REPORT_DIR_NAME}"
TMPDIR_CONTAINER="/data/docker-e2e/tmp"

NM_ROOT="${STATE_ROOT}/docker-e2e/${RUN_ID}/nm"

workspace_node_modules=(
  "root:/workspace/floyd-code/node_modules"
  "apps_floyd-code:/workspace/floyd-code/apps/floyd-code/node_modules"
  "apps_floyd-web:/workspace/floyd-code/apps/floyd-web/node_modules"
  "apps_vis:/workspace/floyd-code/apps/vis/node_modules"
  "apps_vis_server:/workspace/floyd-code/apps/vis/server/node_modules"
  "apps_vis_web:/workspace/floyd-code/apps/vis/web/node_modules"
  "docs:/workspace/floyd-code/docs/node_modules"
  "pkg_kap-server:/workspace/floyd-code/packages/kap-server/node_modules"
  "pkg_server-e2e:/workspace/floyd-code/packages/klient/node_modules"
  "pkg_kaos:/workspace/floyd-code/packages/kaos/node_modules"
  "pkg_kosong:/workspace/floyd-code/packages/kosong/node_modules"
  "pkg_migration-legacy:/workspace/floyd-code/packages/migration-legacy/node_modules"
  "pkg_node-sdk:/workspace/floyd-code/packages/node-sdk/node_modules"
  "pkg_oauth:/workspace/floyd-code/packages/oauth/node_modules"
  "pkg_protocol:/workspace/floyd-code/packages/protocol/node_modules"
  "pkg_services:/workspace/floyd-code/packages/services/node_modules"
  "pkg_telemetry:/workspace/floyd-code/packages/telemetry/node_modules"
)

mkdir -p "${STATE_ROOT}" "${FLOYD_HOME_HOST}" "${REPORT_DIR_HOST}" "${NM_ROOT}"
for mount in "${workspace_node_modules[@]}"; do
  mkdir -p "${NM_ROOT}/${mount%%:*}"
done

# Seed only auth/config into the isolated docker-e2e home. Never copy server
# locks, sessions, uploaded files, or reports from the compose server home.
if [[ -f "${SEED_HOME_HOST}/config.toml" && ! -f "${FLOYD_HOME_HOST}/config.toml" ]]; then
  cp "${SEED_HOME_HOST}/config.toml" "${FLOYD_HOME_HOST}/config.toml"
fi
if [[ -d "${SEED_HOME_HOST}/credentials" && ! -d "${FLOYD_HOME_HOST}/credentials" ]]; then
  cp -R "${SEED_HOME_HOST}/credentials" "${FLOYD_HOME_HOST}/credentials"
fi

if [[ "${FLOYD_SERVER_E2E_SKIP_BUILD:-0}" != "1" ]]; then
  docker build -t "${BASE_IMAGE}" -f "${REPO_ROOT}/Dockerfile" "${REPO_ROOT}"
  docker build \
    -t "${IMAGE}" \
    -f "${PACKAGE_DIR}/Dockerfile" \
    --build-arg "BASE_IMAGE=${BASE_IMAGE}" \
    "${REPO_ROOT}"
fi

docker rm -f "${CONTAINER}" >/dev/null 2>&1 || true

read -r -d '' container_script <<'EOS' || true
set -euo pipefail

cd /workspace/floyd-code
mkdir -p "${FLOYD_CODE_HOME}/server" "${FLOYD_SERVER_E2E_REPORT_DIR}" "${TMPDIR}" /data/server-e2e-reports/docker
rm -f "${FLOYD_CODE_HOME}/server/lock"

if [[ ! -e /workspace/floyd-code/node_modules/.modules.yaml || ! -e /workspace/floyd-code/packages/klient/node_modules/ws ]]; then
  echo "[server-e2e:docker] installing pnpm deps"
  pnpm install --frozen-lockfile
else
  echo "[server-e2e:docker] pnpm deps already present"
fi

server_log="/data/server-e2e-reports/docker/server.log"
: > "${server_log}"

echo "[server-e2e:docker] starting server on container-local ${FLOYD_SERVER_URL}"
pnpm dev:server -- \
  --host 127.0.0.1 \
  --port "${FLOYD_SERVER_E2E_PORT}" \
  --log-level debug \
  --debug-endpoints \
  >"${server_log}" 2>&1 &
server_pid=$!

cleanup() {
  status=$?
  if kill -0 "${server_pid}" >/dev/null 2>&1; then
    kill "${server_pid}" >/dev/null 2>&1 || true
    wait "${server_pid}" >/dev/null 2>&1 || true
  fi
  exit "${status}"
}
trap cleanup EXIT INT TERM

ready=0
for attempt in $(seq 1 90); do
  if curl -fsS "${FLOYD_SERVER_URL}/api/v1/meta" >/tmp/server-meta.json 2>/tmp/server-curl.err; then
    ready=1
    echo "[server-e2e:docker] server ready: $(cat /tmp/server-meta.json)"
    break
  fi
  if ! kill -0 "${server_pid}" >/dev/null 2>&1; then
    echo "[server-e2e:docker] server exited before readiness" >&2
    tail -n 200 "${server_log}" >&2 || true
    exit 1
  fi
  sleep 1
done

if [[ "${ready}" != "1" ]]; then
  echo "[server-e2e:docker] server did not become ready within 90s" >&2
  cat /tmp/server-curl.err >&2 || true
  tail -n 200 "${server_log}" >&2 || true
  exit 1
fi

cd /workspace/floyd-code/packages/klient
pnpm test
EOS

docker_args=(
  run
  --rm
  --init
  --name "${CONTAINER}"
  --workdir /workspace/floyd-code/packages/klient
  --env "FLOYD_CODE_HOME=${FLOYD_HOME_CONTAINER}"
  --env "FLOYD_SERVER_E2E_PORT=${PORT}"
  --env "FLOYD_SERVER_URL=http://127.0.0.1:${PORT}"
  --env "FLOYD_SERVER_E2E_REPORT_DIR=${REPORT_DIR_CONTAINER}"
  --env "TMPDIR=${TMPDIR_CONTAINER}"
  --env "TERM=xterm-256color"
  --env "TZ=Asia/Shanghai"
  --env "npm_config_store_dir=/workspace/floyd-code/node_modules/.pnpm-store"
  --env "npm_config_package_import_method=copy"
  --volume "${REPO_ROOT}:/workspace/floyd-code:ro"
  --volume "${FLOYD_HOME_HOST}:${FLOYD_HOME_CONTAINER}"
  --volume "${REPORT_ROOT_HOST}:${REPORT_ROOT_CONTAINER}"
)

for mount in "${workspace_node_modules[@]}"; do
  docker_args+=(--volume "${NM_ROOT}/${mount%%:*}:${mount#*:}")
done

echo "[server-e2e:docker] running ${IMAGE} without host port publishing"
set +e
docker "${docker_args[@]}" "${IMAGE}" bash -lc "${container_script}"
status=$?
set -e

echo "[server-e2e:docker] report: ${REPORT_DIR_HOST}/index.html"
echo "[server-e2e:docker] server log: ${REPORT_ROOT_HOST}/server.log"
exit "${status}"
