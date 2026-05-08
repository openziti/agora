#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_PATH="${AGORA_DEMO_CONTROLLER_CONFIG:-${REPO_ROOT}/etc/demo-controller.yaml}"
DEMO_ROOT="${AGORA_DEMO_ROOT:-${HOME}/.agora-demo}"
DEMO_ROOT="${DEMO_ROOT/#\~/${HOME}}"
RUN_DIR="${DEMO_ROOT}/run"
WORKER_RUN_DIR="${RUN_DIR}/workers"
LOG_DIR="${DEMO_ROOT}/logs"
SEED_SQL="${DEMO_ROOT}/seed-history.sql"
ADMIN_TOKEN="${AGORA_DEMO_ADMIN_TOKEN:-}"
CONTROLLER_URL="${AGORA_DEMO_CONTROLLER_URL:-}"
DEMO_DSN="${AGORA_DEMO_DSN:-}"
DEMO_EMAIL="demo@agora.local"
DEMO_PASSWORD="Agora-Demo-1"
STARTED_ANY=0
CLEANING_UP=0

WORKERS=(
  "macro-pulse-equity-feed:equity-feed@markets-co:equity-feed"
  "macro-pulse-fx-feed:fx-feed@markets-co:fx-feed"
  "macro-pulse-commodities-feed:commodities-feed@markets-co:commodities-feed"
  "macro-pulse-weather-feed:weather-feed@weather-co:weather-feed"
  "macro-pulse-search-trends:search-trends@signals-co:search-trends"
  "macro-pulse-news-pulse:news-pulse@signals-co:news-pulse"
  "macro-pulse-correlator:correlator@analytics-co:correlator"
  "macro-pulse-narrator:narrator@analytics-co:narrator"
)

REQUIRED_BINS=(
  agora
  demo-bootstrap
  macro-pulse-bootstrap
  macro-pulse-equity-feed
  macro-pulse-fx-feed
  macro-pulse-commodities-feed
  macro-pulse-weather-feed
  macro-pulse-search-trends
  macro-pulse-news-pulse
  macro-pulse-correlator
  macro-pulse-narrator
  macro-pulse-pulse-agent
)

log() {
  printf '[demo-up] %s\n' "$*"
}

warn() {
  printf '[demo-up] warning: %s\n' "$*" >&2
}

fail() {
  printf '[demo-up] error: %s\n' "$*" >&2
  cleanup_started
  exit 1
}

cleanup_started() {
  if [[ "${STARTED_ANY}" == "1" && "${CLEANING_UP}" == "0" ]]; then
    CLEANING_UP=1
    warn "startup failed; stopping processes launched under ${DEMO_ROOT}"
    AGORA_DEMO_ROOT="${DEMO_ROOT}" AGORA_DEMO_CONTROLLER_CONFIG="${CONFIG_PATH}" "${REPO_ROOT}/bin/demo-down.sh" || true
  fi
}

on_error() {
  local status=$?
  cleanup_started
  exit "${status}"
}
trap on_error ERR

extract_yaml_scalar() {
  local key="$1"
  local file="$2"
  sed -nE "s/^[[:space:]]*${key}:[[:space:]]*\"?([^\"#]+)\"?.*$/\1/p" "${file}" | head -n 1 | sed -E 's/[[:space:]]+$//'
}

extract_yaml_sequence_first() {
  local key="$1"
  local file="$2"
  sed -nE "/^[[:space:]]*${key}:[[:space:]]*$/,/^[[:space:]]*[A-Za-z0-9_]+:[[:space:]]*/ {
    s/^[[:space:]]*-[[:space:]]*['\"]?([^'\"#]+)['\"]?.*$/\1/p
  }" "${file}" | head -n 1 | sed -E 's/[[:space:]]+$//'
}

controller_url_from_bind() {
  local bind="$1"
  local host port
  port="$(printf '%s' "${bind}" | sed -nE 's/^.*:([0-9]+)$/\1/p')"
  [[ -n "${port}" ]] || fail "could not derive controller port from bind_address '${bind}' in ${CONFIG_PATH}"
  host="$(printf '%s' "${bind}" | sed -E 's/:?[0-9]+$//')"
  case "${host}" in
    ""|"0.0.0.0"|"::"|"[::]") host="localhost" ;;
  esac
  printf 'http://%s:%s' "${host}" "${port}"
}

controller_port() {
  printf '%s' "${CONTROLLER_URL}" | sed -nE 's#^https?://(\[[^]]+\]|[^:/]+):([0-9]+).*$#\2#p'
}

port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn "sport = :${port}" 2>/dev/null | awk 'NR > 1 { found = 1 } END { exit found ? 0 : 1 }'
    return
  fi
  if command -v lsof >/dev/null 2>&1; then
    lsof -iTCP:"${port}" -sTCP:LISTEN -Pn >/dev/null 2>&1
    return
  fi
  return 1
}

ensure_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command '${1}' is not on PATH"
}

ensure_pid_slot_free() {
  local pidfile="$1"
  [[ -f "${pidfile}" ]] || return 0
  local pid
  pid="$(cat "${pidfile}" 2>/dev/null || true)"
  if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
    fail "process from ${pidfile} is still running as pid ${pid}; run bin/demo-down.sh first"
  fi
  rm -f "${pidfile}"
}

prepare_paths() {
  [[ -f "${CONFIG_PATH}" ]] || fail "controller config not found: ${CONFIG_PATH}"
  mkdir -p "${RUN_DIR}" "${WORKER_RUN_DIR}" "${LOG_DIR}"
  local bind port config_admin_token
  config_admin_token="$(extract_yaml_sequence_first admin_tokens "${CONFIG_PATH}")"
  if [[ -z "${ADMIN_TOKEN}" ]]; then
    ADMIN_TOKEN="${config_admin_token}"
  fi
  [[ -n "${ADMIN_TOKEN}" ]] || fail "admin_tokens not found in ${CONFIG_PATH}; set AGORA_DEMO_ADMIN_TOKEN to override"
  if [[ -n "${AGORA_ADMIN_TOKEN:-}" && -z "${AGORA_DEMO_ADMIN_TOKEN:-}" && "${AGORA_ADMIN_TOKEN}" != "${ADMIN_TOKEN}" ]]; then
    warn "ignoring ambient AGORA_ADMIN_TOKEN for demo startup; using admin_tokens from ${CONFIG_PATH}"
  fi
  bind="$(extract_yaml_scalar bind_address "${CONFIG_PATH}")"
  [[ -n "${bind}" ]] || bind=":8080"
  if [[ -z "${CONTROLLER_URL}" ]]; then
    CONTROLLER_URL="$(controller_url_from_bind "${bind}")"
  fi
  CONTROLLER_URL="${CONTROLLER_URL%/}"
  if [[ -z "${DEMO_DSN}" ]]; then
    DEMO_DSN="$(extract_yaml_scalar dsn "${CONFIG_PATH}")"
  fi
  [[ -n "${DEMO_DSN}" ]] || fail "store.dsn not found in ${CONFIG_PATH}"
  port="$(controller_port)"
  [[ -n "${port}" ]] || fail "could not derive controller port from ${CONTROLLER_URL}"
  ensure_pid_slot_free "${RUN_DIR}/controller.pid"
  if port_in_use "${port}"; then
    fail "port ${port} is already listening; stop the conflicting process or set AGORA_DEMO_CONTROLLER_URL/config"
  fi
}

build_ui() {
  log "building dashboard UI"
  (cd "${REPO_ROOT}/ui" && npm ci && npm run build)
  [[ -d "${REPO_ROOT}/ui/dist" ]] || fail "ui/dist was not created"
  if ! find "${REPO_ROOT}/ui/dist" -mindepth 1 -print -quit | grep -q .; then
    fail "ui/dist is empty after npm run build"
  fi
}

build_go_binaries() {
  log "installing Go demo binaries"
  (cd "${REPO_ROOT}" && go install ./cmd/... ./examples/macro-pulse/cmd/...)
  local missing=()
  local bin
  for bin in "${REQUIRED_BINS[@]}"; do
    if ! command -v "${bin}" >/dev/null 2>&1; then
      missing+=("${bin}")
    fi
  done
  if (( ${#missing[@]} > 0 )); then
    printf '[demo-up] missing installed binaries on PATH:\n' >&2
    printf '  %s\n' "${missing[@]}" >&2
    fail 'go install completed, but one or more binaries are not on PATH; check GOBIN/GOPATH/bin'
  fi
}

run_migrations() {
  log "running store migrations with ${CONFIG_PATH}"
  AGORA_ADMIN_TOKEN="${ADMIN_TOKEN}" agora admin store migrate "${CONFIG_PATH}" up
}

start_controller() {
  local pidfile="${RUN_DIR}/controller.pid"
  local logfile="${LOG_DIR}/controller.log"
  ensure_pid_slot_free "${pidfile}"
  log "starting controller at ${CONTROLLER_URL}"
  AGORA_ADMIN_TOKEN="${ADMIN_TOKEN}" agora controller "${CONFIG_PATH}" >>"${logfile}" 2>&1 &
  echo "$!" > "${pidfile}"
  STARTED_ANY=1
  wait_ready "${CONTROLLER_URL}/ready" 10 "controller" "${logfile}"
}

wait_ready() {
  local url="$1"
  local timeout="$2"
  local label="$3"
  local logfile="$4"
  local deadline=$((SECONDS + timeout))
  until curl -fsS "${url}" >/dev/null 2>&1; do
    if (( SECONDS >= deadline )); then
      fail "${label} did not become ready within ${timeout}s; see ${logfile}"
    fi
    sleep 1
  done
}

is_first_run() {
  [[ ! -d "${DEMO_ROOT}/envs" ]] && return 0
  ! find "${DEMO_ROOT}/envs" -mindepth 1 -maxdepth 1 -type d -print -quit 2>/dev/null | grep -q .
}

run_bootstrap() {
  local first_run="$1"
  local seed_arg="--seed-history=0"
  if [[ "${first_run}" == "1" ]]; then
    ensure_command psql
    seed_arg="--seed-history=7d"
  fi
  log "provisioning demo topology"
  AGORA_DEMO_ROOT="${DEMO_ROOT}" demo-bootstrap \
    --controller="${CONTROLLER_URL}" \
    --admin-token="${ADMIN_TOKEN}" \
    "${seed_arg}"

  if [[ "${first_run}" == "1" ]]; then
    [[ -s "${SEED_SQL}" ]] || fail "expected seed history SQL at ${SEED_SQL}"
    log "loading synthetic history from ${SEED_SQL}"
    psql "${DEMO_DSN}" -f "${SEED_SQL}"
  else
    log "warm restart detected; preserved env roots reused and seed history not reloaded"
  fi
}

start_background() {
  local name="$1"
  local pidfile="$2"
  local logfile="$3"
  local envroot="$4"
  shift 4
  ensure_pid_slot_free "${pidfile}"
  mkdir -p "$(dirname "${pidfile}")" "$(dirname "${logfile}")"
  log "starting ${name}"
  AGORA_ENV_ROOT="${envroot}" AGORA_API_ENDPOINT="${CONTROLLER_URL}" "$@" >>"${logfile}" 2>&1 &
  local pid=$!
  echo "${pid}" > "${pidfile}"
  STARTED_ANY=1
  sleep 0.2
  if ! kill -0 "${pid}" 2>/dev/null; then
    fail "${name} exited during startup; see ${logfile}"
  fi
}

wait_for_worker_advertisement() {
  local email="$1"
  local ad_name="$2"
  local logfile="$3"
  local envroot="${DEMO_ROOT}/envs/${email}"
  local deadline=$((SECONDS + 10))
  until AGORA_ENV_ROOT="${envroot}" AGORA_API_ENDPOINT="${CONTROLLER_URL}" agora advertise list --status active --json 2>/dev/null | grep -q "\"name\": \"${ad_name}\""; do
    if (( SECONDS >= deadline )); then
      fail "worker advertisement '${ad_name}' was not published within 10s; see ${logfile}"
    fi
    sleep 1
  done
}

start_workers() {
  local item bin email ad envroot pidfile logfile
  for item in "${WORKERS[@]}"; do
    IFS=: read -r bin email ad <<<"${item}"
    envroot="${DEMO_ROOT}/envs/${email}"
    [[ -f "${envroot}/environment.json" ]] || fail "missing env root for ${email}: ${envroot}"
    pidfile="${WORKER_RUN_DIR}/${bin}.pid"
    logfile="${LOG_DIR}/${bin}.log"
    start_background "${bin}" "${pidfile}" "${logfile}" "${envroot}" "${bin}"
    wait_for_worker_advertisement "${email}" "${ad}" "${logfile}"
    log "worker ${bin} published ${ad}"
  done
}

start_pulse_agent() {
  local bin="${AGORA_PULSE_AGENT_BIN:-macro-pulse-pulse-agent}"
  ensure_command "${bin}"
  local envroot="${DEMO_ROOT}/envs/${DEMO_EMAIL}"
  [[ -f "${envroot}/environment.json" ]] || fail "missing orchestrator env root: ${envroot}"
  [[ -f "${REPO_ROOT}/etc/demo-profile.yaml" ]] || fail "missing etc/demo-profile.yaml"
  start_background "macro-pulse-pulse-agent" "${RUN_DIR}/pulse-agent.pid" "${LOG_DIR}/macro-pulse-pulse-agent.log" "${envroot}" "${bin}" --loop --profile="${REPO_ROOT}/etc/demo-profile.yaml"
}

start_gateway() {
  local name="$1"
  local bin="$2"
  local config="${DEMO_ROOT}/gateways/${name}.yaml"
  local envroot
  if ! command -v "${bin}" >/dev/null 2>&1; then
    warn "${bin} not found; ${name} gateway advertisement will be absent"
    return 0
  fi
  [[ -f "${config}" ]] || {
    warn "${name} gateway config missing at ${config}; skipping"
    return 0
  }
  envroot="$(sed -nE 's/^[[:space:]]*env_root:[[:space:]]*"?([^"]+)"?.*$/\1/p' "${config}" | head -n 1)"
  [[ -n "${envroot}" ]] || {
    warn "${name} gateway config has no env_root; skipping"
    return 0
  }
  local pidfile="${RUN_DIR}/${name}-gateway.pid"
  local logfile="${LOG_DIR}/${name}-gateway.log"
  ensure_pid_slot_free "${pidfile}"
  log "starting ${name}-gateway"
  env \
    "AGORA_ENV_ROOT=${envroot}" \
    "AGORA_API_ENDPOINT=${CONTROLLER_URL}" \
    "AGORA_GATEWAY_CONFIG=${config}" \
    "AGORA_${name^^}_GATEWAY_CONFIG=${config}" \
    "${bin}" --network=agora >>"${logfile}" 2>&1 &
  echo "$!" > "${pidfile}"
  STARTED_ANY=1
  sleep 1
  if ! kill -0 "$(cat "${pidfile}")" 2>/dev/null; then
    warn "${name} gateway exited during startup; see ${logfile}"
    rm -f "${pidfile}"
  else
    log "${name} gateway started with config ${config}"
  fi
}

start_gateways() {
  start_gateway "llm" "${AGORA_LLM_GATEWAY_BIN:-llm-gateway}"
  start_gateway "mcp" "${AGORA_MCP_GATEWAY_BIN:-mcp-gateway}"
}

open_browser() {
  [[ "${AGORA_DEMO_OPEN_BROWSER:-1}" == "0" ]] && return 0
  if command -v xdg-open >/dev/null 2>&1; then
    xdg-open "${CONTROLLER_URL}/" >/dev/null 2>&1 &
    return 0
  fi
  if command -v open >/dev/null 2>&1; then
    open "${CONTROLLER_URL}/" >/dev/null 2>&1 &
    return 0
  fi
  warn "no browser opener found; visit ${CONTROLLER_URL}/"
}

print_credentials() {
  log "run bin/demo-down.sh to stop managed processes"
  printf '\n'
  printf 'Agora demo is running at %s/\n' "${CONTROLLER_URL}"
  printf '   Email:    %s\n' "${DEMO_EMAIL}"
  printf '   Password: %s\n' "${DEMO_PASSWORD}"
}

main() {
  cd "${REPO_ROOT}"
  prepare_paths
  local first_run=0
  if is_first_run; then
    first_run=1
  fi
  build_ui
  build_go_binaries
  run_migrations
  start_controller
  run_bootstrap "${first_run}"
  start_workers
  start_pulse_agent
  start_gateways
  open_browser
  trap - ERR
  print_credentials
}

main "$@"
