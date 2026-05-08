#!/usr/bin/env bash
set -Eeuo pipefail
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
CONFIG_PATH="${AGORA_DEMO_CONTROLLER_CONFIG:-${REPO_ROOT}/etc/demo-controller.yaml}"
DEMO_ROOT="${AGORA_DEMO_ROOT:-${HOME}/.agora-demo}"
DEMO_ROOT="${DEMO_ROOT/#\~/${HOME}}"
RUN_DIR="${DEMO_ROOT}/run"
WORKER_RUN_DIR="${RUN_DIR}/workers"
PURGE=0

log() {
  printf '[demo-down] %s\n' "$*"
}

warn() {
  printf '[demo-down] warning: %s\n' "$*" >&2
}

extract_yaml_scalar() {
  local key="$1"
  local file="$2"
  [[ -f "${file}" ]] || return 0
  sed -nE "s/^[[:space:]]*${key}:[[:space:]]*\"?([^\"#]+)\"?.*$/\1/p" "${file}" | head -n 1 | sed -E 's/[[:space:]]+$//'
}

usage() {
  cat <<'USAGE'
usage: bin/demo-down.sh [--purge]

Stops demo processes started by bin/demo-up.sh. By default it preserves
$AGORA_DEMO_ROOT/envs and $AGORA_DEMO_ROOT/logs for warm restarts and
debugging. --purge removes the entire demo root after stopping processes.
USAGE
}

while (($#)); do
  case "$1" in
    --purge)
      PURGE=1
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      warn "unknown argument: $1"
      usage
      exit 2
      ;;
  esac
  shift
done

is_running() {
  local pid="$1"
  [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null
}

wait_for_exit() {
  local pid="$1"
  local timeout="$2"
  local deadline=$((SECONDS + timeout))
  while is_running "${pid}"; do
    if (( SECONDS >= deadline )); then
      return 1
    fi
    sleep 1
  done
  return 0
}

stop_pid() {
  local pid="$1"
  local label="$2"
  local timeout="$3"
  if ! is_running "${pid}"; then
    return 0
  fi
  log "stopping ${label} pid=${pid}"
  kill -TERM "${pid}" 2>/dev/null || true
  if ! wait_for_exit "${pid}" "${timeout}"; then
    warn "${label} did not stop within ${timeout}s; sending SIGKILL"
    kill -KILL "${pid}" 2>/dev/null || true
    wait_for_exit "${pid}" 2 || true
  fi
}

stop_pid_file() {
  local pidfile="$1"
  local label="$2"
  local timeout="$3"
  if [[ ! -f "${pidfile}" ]]; then
    return 0
  fi
  local pid
  pid="$(cat "${pidfile}" 2>/dev/null || true)"
  stop_pid "${pid}" "${label}" "${timeout}"
  rm -f "${pidfile}"
}

stop_workers() {
  local pidfile label
  for pidfile in "${WORKER_RUN_DIR}"/*.pid; do
    label="$(basename "${pidfile}" .pid)"
    stop_pid_file "${pidfile}" "${label}" 5
  done
}

stop_gateways() {
  stop_pid_file "${RUN_DIR}/llm-gateway.pid" "llm-gateway" 5
  stop_pid_file "${RUN_DIR}/mcp-gateway.pid" "mcp-gateway" 5
}

stop_controller() {
  local pidfile="${RUN_DIR}/controller.pid"
  if [[ -f "${pidfile}" ]]; then
    stop_pid_file "${pidfile}" "agora controller" 10
  fi
  stop_controller_by_config
}

stop_controller_by_config() {
  command -v pgrep >/dev/null 2>&1 || return 0
  local pid
  while IFS= read -r pid; do
    [[ -n "${pid}" ]] || continue
    stop_pid "${pid}" "agora controller" 10
  done < <(pgrep -f "agora controller ${CONFIG_PATH}" 2>/dev/null || true)
}

stop_controller_by_port_if_matching_config() {
  command -v ss >/dev/null 2>&1 || return 0
  local bind port pid cmd
  bind="$(extract_yaml_scalar bind_address "${CONFIG_PATH}")"
  [[ -n "${bind}" ]] || bind=":8080"
  port="$(printf '%s' "${bind}" | sed -nE 's/^.*:([0-9]+)$/\1/p')"
  [[ -n "${port}" ]] || return 0
  while IFS= read -r pid; do
    [[ -n "${pid}" ]] || continue
    cmd="$(ps -p "${pid}" -o args= 2>/dev/null || true)"
    if [[ "${cmd}" == *"agora controller ${CONFIG_PATH}"* ]]; then
      stop_pid "${pid}" "agora controller" 10
    fi
  done < <(ss -ltnp "sport = :${port}" 2>/dev/null | sed -nE 's/.*pid=([0-9]+).*/\1/p' | sort -u)
}

stop_orphaned_controller() {
  stop_controller_by_config
  stop_controller_by_port_if_matching_config
}

stop_demo_processes() {
  if [[ -d "${DEMO_ROOT}" ]]; then
    stop_workers
    stop_pid_file "${RUN_DIR}/pulse-agent.pid" "macro-pulse-pulse-agent" 5
    stop_gateways
  fi
  stop_controller
}

clean_run_dir() {
  [[ -d "${RUN_DIR}" ]] || return 0
  find "${RUN_DIR}" -type f -name '*.pid' -delete
  find "${RUN_DIR}" -mindepth 1 -depth -type d -empty -delete
  mkdir -p "${RUN_DIR}"
}

main() {
  cd "${REPO_ROOT}"
  stop_demo_processes
  if [[ -d "${DEMO_ROOT}" ]]; then
    clean_run_dir
  else
    stop_orphaned_controller
  fi

  if [[ "${PURGE}" == "1" ]]; then
    if [[ -d "${DEMO_ROOT}" ]]; then
      log "purging ${DEMO_ROOT}"
      rm -rf "${DEMO_ROOT}"
    else
      log "demo root already absent: ${DEMO_ROOT}"
    fi
  else
    if [[ -d "${DEMO_ROOT}" ]]; then
      log "stopped managed demo processes; preserved ${DEMO_ROOT}/envs and ${DEMO_ROOT}/logs"
    else
      log "stopped managed demo processes; demo root does not exist: ${DEMO_ROOT}"
    fi
  fi
}

main "$@"
