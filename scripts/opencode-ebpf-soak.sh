#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_BIN="${AGENT_SOAK_BACKEND_BIN:-$ROOT_DIR/backend/agent-ebpf-filter}"
MODEL="${OPENCODE_SOAK_MODEL:-opencode/big-pickle}"
BACKEND_TIMEOUT="${OPENCODE_SOAK_BACKEND_TIMEOUT:-60}"
MODEL_TIMEOUT="${OPENCODE_SOAK_MODEL_TIMEOUT:-180}"
EVENT_TIMEOUT="${OPENCODE_SOAK_EVENT_TIMEOUT:-45}"
ARTIFACT_DIR="${OPENCODE_SOAK_ARTIFACT_DIR:-}"
KEEP_ARTIFACTS="${OPENCODE_SOAK_KEEP_ARTIFACTS:-0}"
EBPF_PIN_ROOT=/sys/fs/bpf/agent-ebpf
pin_root_exists() {
  if (( EUID == 0 )); then
    [[ -e "$EBPF_PIN_ROOT" || -L "$EBPF_PIN_ROOT" ]]
  else
    sudo -n test -e "$EBPF_PIN_ROOT" 2>/dev/null || sudo -n test -L "$EBPF_PIN_ROOT" 2>/dev/null
  fi
}

backend_pid_running() {
  if (( EUID == 0 )); then
    [[ -e "/proc/$1/exe" ]]
  else
    sudo -n test -e "/proc/$1/exe" 2>/dev/null
  fi
}

backend_signal() {
  if (( EUID == 0 )); then
    kill "-$1" "$2" 2>/dev/null
  else
    sudo -n kill "-$1" "$2" 2>/dev/null
  fi
}

if [[ ! -x "$BACKEND_BIN" ]]; then
  echo "[soak] backend binary is missing or not executable: $BACKEND_BIN (run make ebpf-bootstrap first)" >&2
  exit 1
fi
for command in curl python3 opencode timeout mountpoint; do
  if ! command -v "$command" >/dev/null 2>&1; then
    echo "[soak] required command is missing: $command" >&2
    exit 1
  fi
done
if (( EUID != 0 )) && ! command -v sudo >/dev/null 2>&1; then
  echo "[soak] root privileges are required to attach eBPF tracepoints" >&2
  exit 1
fi
if (( EUID != 0 )) && ! sudo -n true 2>/dev/null; then
  echo "[soak] sudo must be available without an interactive prompt" >&2
  exit 1
fi
if [[ ! -r /sys/kernel/btf/vmlinux ]]; then
  echo "[soak] kernel BTF is unavailable at /sys/kernel/btf/vmlinux" >&2
  exit 1
fi
if ! mountpoint -q /sys/fs/bpf; then
  echo "[soak] bpffs is not mounted at /sys/fs/bpf" >&2
  exit 1
fi
if pin_root_exists; then
  echo "[soak] refusing to reuse or remove existing eBPF pins: $EBPF_PIN_ROOT" >&2
  exit 1
fi

if [[ -n "$ARTIFACT_DIR" ]]; then
  mkdir -p "$ARTIFACT_DIR"
  RUN_DIR="$(mktemp -d "$ARTIFACT_DIR/run.XXXXXX")"
  KEEP_ARTIFACTS=1
else
  RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/agent-ebpf-opencode-soak.XXXXXX")"
fi
BACKEND_HOME="$RUN_DIR/backend-home"
WORKDIR="$RUN_DIR/workspace"
OPENCODE_HOME="$RUN_DIR/opencode-home"
BACKEND_LOG="$RUN_DIR/backend.log"
BACKEND_PID_FILE="$RUN_DIR/backend.pid"
OPENCODE_LOG="$RUN_DIR/opencode.jsonl"
SUMMARY="$RUN_DIR/summary.txt"
START_FIFO="$RUN_DIR/opencode-start.fifo"
BACKEND_LAUNCH_PID=""
REGISTERED_PID=""
PIN_ROOT_RESERVED=1
BACKEND_STOPPED=0
API_KEY="$(python3 -c 'import secrets; print(secrets.token_hex(32))')"
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"
BASE_URL="http://127.0.0.1:$PORT"

mkdir -p "$BACKEND_HOME" "$WORKDIR" "$OPENCODE_HOME"
mkfifo "$START_FIFO"

cleanup() {
  local status=$?
  trap - EXIT

  if [[ -n "$REGISTERED_PID" ]]; then
    if kill -0 "$REGISTERED_PID" 2>/dev/null; then
      kill -TERM "$REGISTERED_PID" 2>/dev/null || true
      for _ in {1..20}; do
        kill -0 "$REGISTERED_PID" 2>/dev/null || break
        sleep 0.2
      done
      if kill -0 "$REGISTERED_PID" 2>/dev/null; then
        kill -KILL "$REGISTERED_PID" 2>/dev/null || true
        status=1
      fi
    fi
    wait "$REGISTERED_PID" 2>/dev/null || true
    curl --silent --show-error --max-time 3 -X POST \
      -H "X-API-KEY: $API_KEY" -H 'Content-Type: application/json' \
      --data "{\"pid\":$REGISTERED_PID}" "$BASE_URL/unregister" >/dev/null 2>&1 || true
  fi

  if [[ -n "$BACKEND_LAUNCH_PID" ]]; then
    local backend_pid=""
    for _ in {1..20}; do
      if [[ -s "$BACKEND_PID_FILE" ]]; then
        backend_pid="$(<"$BACKEND_PID_FILE")"
        break
      fi
      kill -0 "$BACKEND_LAUNCH_PID" 2>/dev/null || break
      sleep 0.2
    done
    if [[ "$backend_pid" =~ ^[0-9]+$ ]] && backend_pid_running "$backend_pid"; then
      backend_signal TERM "$backend_pid" || true
      for _ in {1..30}; do
        backend_pid_running "$backend_pid" || break
        sleep 0.2
      done
      if backend_pid_running "$backend_pid"; then
        backend_signal KILL "$backend_pid" || true
        status=1
        echo "[soak] backend required SIGKILL during cleanup" >&2
      fi
    fi
    wait "$BACKEND_LAUNCH_PID" 2>/dev/null || true
    BACKEND_LAUNCH_PID=""
    if [[ "$backend_pid" =~ ^[0-9]+$ ]]; then
      if backend_pid_running "$backend_pid"; then
        BACKEND_STOPPED=0
      else
        BACKEND_STOPPED=1
      fi
    elif pin_root_exists; then
      BACKEND_STOPPED=0
    else
      BACKEND_STOPPED=1
    fi
    if (( BACKEND_STOPPED == 0 )); then
      status=1
      echo "[soak] could not confirm backend exit; leaving eBPF pins in place" >&2
    fi
  else
    BACKEND_STOPPED=1
  fi

  if (( PIN_ROOT_RESERVED == 1 && BACKEND_STOPPED == 1 )) && pin_root_exists; then
    if (( EUID == 0 )); then
      rm -rf -- "$EBPF_PIN_ROOT"
    else
      sudo -n rm -rf -- "$EBPF_PIN_ROOT"
    fi
  fi

  if [[ -f "$BACKEND_LOG" ]]; then
    if (( status != 0 )); then
      echo "[soak] backend log (last 80 lines):" >&2
      tail -n 80 "$BACKEND_LOG" >&2 || true
      if [[ -f "$OPENCODE_LOG" ]]; then
        echo "[soak] OpenCode log (last 80 lines):" >&2
        tail -n 80 "$OPENCODE_LOG" >&2 || true
      fi
    fi
  fi

  if [[ "$KEEP_ARTIFACTS" != 1 ]]; then
    rm -rf -- "$RUN_DIR"
  else
    echo "[soak] artifacts: $RUN_DIR"
  fi
  exit "$status"
}
trap cleanup EXIT

write_project_config() {
  python3 - "$WORKDIR/opencode.json" "$WORKDIR" <<'PY'
import json
import sys

config_path, workdir = sys.argv[1:]
commands = [
    f"mkdir -p {workdir}/nested",
    f"touch {workdir}/nested/marker-one.txt",
    f"touch {workdir}/nested/marker-two.txt",
]
permissions = [{"action": "*", "resource": "*", "effect": "deny"}]
permissions.extend(
    {"action": "shell", "resource": command, "effect": "allow"}
    for command in commands
)
config = {
    "$schema": "https://opencode.ai/config.json",
    "permissions": [{"action": "*", "resource": "*", "effect": "deny"}],
    "agents": {
        "soak": {
            "description": "Execute only the three explicitly allowed temporary-file operations.",
            "mode": "primary",
            "system": (
                "Run only the three shell commands supplied by the user, exactly and in order, "
                "one command per tool call. Do not inspect files, access the network or backend, "
                "read environment variables, run extra commands, or modify anything else. "
                "After the commands finish, reply exactly SOAK_OK."
            ),
            "permissions": permissions,
        }
    },
}
with open(config_path, "w", encoding="utf-8") as output:
    json.dump(config, output, separators=(",", ":"))
    output.write("\n")
PY
}

curl_api() {
  curl --silent --show-error --fail-with-body --connect-timeout 2 --max-time 10 \
    -H "X-API-KEY: $API_KEY" "$@"
}

start_backend() {
  : > "$BACKEND_PID_FILE"
  local -a backend_env=(
    "AGENT_BACKEND_HOST=127.0.0.1"
    "AGENT_BACKEND_PORT=$PORT"
    "AGENT_REAL_HOME=$BACKEND_HOME"
    "AGENT_ACCESS_TOKEN=$API_KEY"
    "AGENT_RUNTIME_TLS_CAPTURE_ENABLED=false"
    "AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED=false"
    "DISABLE_AUTH=false"
    "GIN_MODE=release"
  )
  local -a backend_command=(
    "${backend_env[@]}"
    sh -c 'echo "$$" > "$1"; exec "$2"' _
    "$BACKEND_PID_FILE" "$BACKEND_BIN"
  )
  if (( EUID == 0 )); then
    env "${backend_command[@]}" >"$BACKEND_LOG" 2>&1 &
  else
    sudo -n env "${backend_command[@]}" >"$BACKEND_LOG" 2>&1 &
  fi
  BACKEND_LAUNCH_PID=$!
}

write_project_config
start_backend

printf '[soak] waiting for authenticated backend on %s\n' "$BASE_URL"
ready=0
for _ in $(seq 1 "$BACKEND_TIMEOUT"); do
  if curl_api -o "$RUN_DIR/health.json" "$BASE_URL/api/v1/health" 2>/dev/null; then
    ready=1
    break
  fi
  if ! kill -0 "$BACKEND_LAUNCH_PID" 2>/dev/null; then
    echo "[soak] backend exited before becoming ready" >&2
    break
  fi
  sleep 1
done
if (( ready == 0 )); then
  echo "[soak] backend did not become ready within ${BACKEND_TIMEOUT}s" >&2
  exit 1
fi

curl_api -o "$RUN_DIR/bootstrap-health.json" "$BASE_URL/system/bootstrap-health"
python3 - "$RUN_DIR/bootstrap-health.json" >"$SUMMARY" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    status = json.load(source)
attached = status.get("attachedCount", 0)
compiled = status.get("compiledCount", 0)
if not isinstance(attached, int) or not isinstance(compiled, int) or attached < 1 or compiled < 1:
    raise SystemExit(f"eBPF bootstrap attached {attached} of {compiled} tracepoints")
print(f"eBPF bootstrap: {status.get('status')} ({attached}/{compiled} tracepoints); kernel={status.get('kernelRelease')}")
PY
cat "$SUMMARY"

printf -v PROMPT 'Run exactly these three shell commands, one per tool call and in order:\n1) `mkdir -p %s/nested`\n2) `touch %s/nested/marker-one.txt`\n3) `touch %s/nested/marker-two.txt`\nDo not inspect files, use other tools, access the network, call the backend, read environment variables, or modify anything outside the temporary workspace. Reply exactly SOAK_OK after all three commands succeed.' "$WORKDIR" "$WORKDIR" "$WORKDIR"
(
  cd "$WORKDIR"
  IFS= read -r _ < "$START_FIFO"
  exec env \
    -u OPENCODE_API_KEY \
    -u OPENCODE_CONFIG \
    -u OPENCODE_CONFIG_CONTENT \
    -u OPENCODE_CONFIG_DIR \
    -u OPENCODE_PERMISSION \
    -u OPENCODE_CLI_CONFIG_CONTENT \
    HOME="$OPENCODE_HOME" \
    XDG_CONFIG_HOME="$OPENCODE_HOME/config" \
    XDG_DATA_HOME="$OPENCODE_HOME/data" \
    XDG_CACHE_HOME="$OPENCODE_HOME/cache" \
    XDG_STATE_HOME="$OPENCODE_HOME/state" \
    timeout --signal=TERM --kill-after=10s "${MODEL_TIMEOUT}s" \
      opencode run --standalone --model "$MODEL" --agent soak --format json --auto "$PROMPT"
) >"$OPENCODE_LOG" 2>&1 &
REGISTERED_PID=$!

curl_api -X POST -H 'Content-Type: application/json' \
  --data "{\"pid\":$REGISTERED_PID,\"tag\":\"Agent CLI\",\"cwd\":\"$WORKDIR\"}" \
  "$BASE_URL/register" >/dev/null
printf 'start\n' > "$START_FIFO"

set +e
wait "$REGISTERED_PID"
opencode_status=$?
set -e
REGISTERED_PID=""
if (( opencode_status != 0 )); then
  echo "[soak] OpenCode exited with status $opencode_status (model: $MODEL)" >&2
  exit 1
fi
for marker in "$WORKDIR/nested/marker-one.txt" "$WORKDIR/nested/marker-two.txt"; do
  if [[ ! -f "$marker" ]]; then
    echo "[soak] OpenCode did not create expected marker: $marker" >&2
    exit 1
  fi
done

python3 "$ROOT_DIR/scripts/check-opencode-soak.py" \
  --base-url "$BASE_URL" --api-key "$API_KEY" \
  --expect "$WORKDIR/nested/marker-one.txt" \
  --expect "$WORKDIR/nested/marker-two.txt" \
  --timeout "$EVENT_TIMEOUT"
printf 'OpenCode model: %s\nBackend: %s\nEvent assertions: passed\n' "$MODEL" "$BASE_URL" >>"$SUMMARY"
echo "[soak] OpenCode free-model exec was observed in kernel events"
