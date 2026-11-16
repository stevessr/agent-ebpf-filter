#!/usr/bin/env bash
set -Eeuo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_BIN="${AGENT_SOAK_BACKEND_BIN:-$ROOT_DIR/backend/agent-ebpf-filter}"
MODEL="${OPENCODE_SOAK_MODEL:-opencode/big-pickle}"
SOAK_EXECUTOR="${AGENT_SOAK_EXECUTOR:-opencode}"  # registered or opencode
case "$SOAK_EXECUTOR" in
  registered|opencode) ;;
  *) echo "[soak] invalid executor: $SOAK_EXECUTOR" >&2; exit 2 ;;
esac
BACKEND_TIMEOUT="${OPENCODE_SOAK_BACKEND_TIMEOUT:-60}"
MODEL_TIMEOUT="${OPENCODE_SOAK_MODEL_TIMEOUT:-180}"
EVENT_TIMEOUT="${OPENCODE_SOAK_EVENT_TIMEOUT:-45}"
ARTIFACT_DIR="${OPENCODE_SOAK_ARTIFACT_DIR:-}"
KEEP_ARTIFACTS="${OPENCODE_SOAK_KEEP_ARTIFACTS:-0}"
# The soak's assertion window must remain observable even under bounded
# /events/recent retention. Drop unrelated high-rate network/IO/alert types
# from this isolated test backend; NEVER disable OPENAT (1), MKDIR (3), OPEN
# (11), GENERIC_SYSCALL (25; openat2), or process/exec provenance (0,26,27).
# Production capture, monitoring and configured policy are unaffected.
SOAK_DISABLED_EVENT_TYPES="2,5,6,7,8,9,10,20,21,22,28,29,30,31,32,33,34,43"
SOAK_IGNORED_PATHS="/proc,/var/cache/apt,/var/lib/apt,/var/log/journal"
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
required_commands=(curl python3 timeout mountpoint)
if [[ "$SOAK_EXECUTOR" == "opencode" ]]; then
  required_commands+=(opencode)
fi
for command in "${required_commands[@]}"; do
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
  # Keep the focused file-capture profile above. Do not inherit Renew's
  # Daily ignored-path defaults, which could hide test marker events.
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
OPENCODE_LAUNCH_PID=""
EVENT_MONITOR_PID=""
CONTROL_PID=""
CONTROL_REGISTERED_PID=""
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

  if [[ -n "$OPENCODE_LAUNCH_PID" ]]; then
    kill -TERM "$OPENCODE_LAUNCH_PID" 2>/dev/null || true
    wait "$OPENCODE_LAUNCH_PID" 2>/dev/null || true
    OPENCODE_LAUNCH_PID=""
  fi
  if [[ -n "$EVENT_MONITOR_PID" ]]; then
    kill -TERM "$EVENT_MONITOR_PID" 2>/dev/null || true
    wait "$EVENT_MONITOR_PID" 2>/dev/null || true
    EVENT_MONITOR_PID=""
  fi
  if [[ -n "$CONTROL_PID" ]]; then
    kill -TERM "$CONTROL_PID" 2>/dev/null || true
    wait "$CONTROL_PID" 2>/dev/null || true
    CONTROL_PID=""
  fi
  if [[ -n "$CONTROL_REGISTERED_PID" ]]; then
    curl --silent --show-error --max-time 3 -X POST \
      -H "X-API-KEY: $API_KEY" -H 'Content-Type: application/json' \
      --data "{\"pid\":$CONTROL_REGISTERED_PID}" "$BASE_URL/unregister" >/dev/null 2>&1 || true
    CONTROL_REGISTERED_PID=""
  fi
  if [[ -n "$REGISTERED_PID" ]]; then
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
      if [[ -f "$RUN_DIR/checker.log" ]]; then
        echo "[soak] eBPF live sampler diagnostic:" >&2
        tail -n 50 "$RUN_DIR/checker.log" >&2 || true
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
command = (
    f"mkdir -p {workdir}/nested && "
    f"touch {workdir}/nested/marker-one.txt && "
    f"touch {workdir}/nested/marker-two.txt"
)
global_permissions = [{"action": "*", "resource": "*", "effect": "deny"}]
global_permissions.append(
    {"action": "read", "resource": config_path, "effect": "allow"}
)
permissions = list(global_permissions)
# OpenCode checks every subcommand in an && chain independently. Keep
# exact allow rules for each component as well as the compound resource;
# do not grant wildcard shell access to the isolated soak agent.
allowed_shell = [
    command,
    f"mkdir -p {workdir}/nested",
    f"touch {workdir}/nested/marker-one.txt",
    f"touch {workdir}/nested/marker-two.txt",
]
permissions.extend(
    {"action": "shell", "resource": allowed, "effect": "allow"}
    for allowed in allowed_shell
)
config = {
    "$schema": "https://opencode.ai/config.json",
    "permissions": global_permissions,
    "agents": {
        "soak": {
            "description": "Execute the three temporary-file operations in one ordered shell call.",
            "mode": "primary",
            "system": (
                "Call the shell tool exactly once with the user's full &&-joined command, "
                "never split or parallelize it. Do not inspect files, access the network "
                "or backend, read environment variables, run extra commands, or modify anything else. "
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
    "AGENT_RUNTIME_DISABLED_EVENT_TYPES=$SOAK_DISABLED_EVENT_TYPES"
    "AGENT_RUNTIME_IGNORED_PATHS=$SOAK_IGNORED_PATHS"
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

# Poll while the agent is running. The kernel may emit hundreds of
# process_exit/housekeeping events after a short tool call; relying on one
# post-run /events/recent snapshot can miss genuine openat evidence.
python3 "$ROOT_DIR/scripts/check-opencode-soak.py" \
  --base-url "$BASE_URL" --api-key "$API_KEY" \
  --expect "$RUN_DIR/control-marker.txt" \
  --expect "$WORKDIR/nested/marker-one.txt" \
  --expect "$WORKDIR/nested/marker-two.txt" \
  --report "$RUN_DIR/capture-report.json" \
  --done-file "$RUN_DIR/opencode-done" --post-run-timeout "$EVENT_TIMEOUT" \
  --limit 1000 --poll-interval 0.25 \
  --timeout "$((MODEL_TIMEOUT + EVENT_TIMEOUT))" \
  >"$RUN_DIR/checker.log" 2>&1 &
EVENT_MONITOR_PID=$!

# Baseline: one registered Bash process performs an open/create syscall itself,
# without an LLM or subprocess attribution. If this is absent from the eBPF
# stream the failure is in kernel capture/configuration, not the model.
CONTROL_FIFO="$RUN_DIR/control-start.fifo"
mkfifo "$CONTROL_FIFO"
(
  IFS= read -r _ < "$CONTROL_FIFO"
  # The standard daily profile deliberately disables unscoped "bash" comm
  # events. Rename only this explicitly registered, isolated Bash baseline
  # before its writes, or the reader would discard every control sample.
  # /proc/self/comm changes the kernel task name without changing its PID.
  printf 'soak-file-probe\n' > /proc/self/comm
  for _ in {1..12}; do
    : > "$RUN_DIR/control-marker.txt"
    sleep 0.2
  done
) &
CONTROL_PID=$!
CONTROL_REGISTERED_PID="$CONTROL_PID"
curl_api -X POST -H 'Content-Type: application/json' \
  --data "{\"pid\":$CONTROL_PID,\"tag\":\"Agent CLI\",\"cwd\":\"$RUN_DIR\"}" \
  "$BASE_URL/register" >/dev/null
printf 'go\n' > "$CONTROL_FIFO"
wait "$CONTROL_PID"
CONTROL_PID=""
curl_api -X POST -H 'Content-Type: application/json' \
  --data "{\"pid\":$CONTROL_REGISTERED_PID}" "$BASE_URL/unregister" >/dev/null
CONTROL_REGISTERED_PID=""
echo "[soak] registered-shell baseline completed (capture evidence checked independently)"

# The registered executor is the mandatory quota-independent kernel
# integration check. It exercises real mkdir/touch syscalls from an explicitly
# registered Agent PID and tests fork inheritance, path capture and the graph.
# It is NOT a substitute for optional live OpenCode model/tool testing.
if [[ "$SOAK_EXECUTOR" == "registered" ]]; then
  (
    cd "$WORKDIR"
    IFS= read -r _ < "$START_FIFO"
    # Bash is excluded from normal daily capture; give the explicitly
    # registered test process a unique comm before any test file activity.
    printf 'soak-worker\n' > /proc/self/comm
    mkdir -p "$WORKDIR/nested"
    touch "$WORKDIR/nested/marker-one.txt"
    touch "$WORKDIR/nested/marker-two.txt"
  ) >"$OPENCODE_LOG" 2>&1 &
  OPENCODE_LAUNCH_PID=$!
  REGISTERED_PID="$OPENCODE_LAUNCH_PID"
  curl_api -X POST -H 'Content-Type: application/json' \
    --data "{\"pid\":$REGISTERED_PID,\"tag\":\"Agent CLI\",\"cwd\":\"$WORKDIR\"}" \
    "$BASE_URL/register" >/dev/null
  echo "[soak] registered deterministic tool-runner PID $REGISTERED_PID"
  printf 'start\n' > "$START_FIFO"
else
# A model can schedule multiple tool calls concurrently despite a sequential
# instruction. Use ONE shell call with explicit && barriers, so kernel evidence
# covers all three operations without a model-dependent scheduling race.
# Per-tool-call causality is exercised separately in AgentSight regression CI.
printf -v PROMPT 'Call the shell tool exactly ONCE, with the following exact command string (do not split it into separate tool calls):\nmkdir -p %s/nested && touch %s/nested/marker-one.txt && touch %s/nested/marker-two.txt\nOnly after the tool reports exit 0, reply exactly SOAK_OK. Do not inspect files, run extra commands, read secrets or environment variables, access the network, or call the backend.' "$WORKDIR" "$WORKDIR" "$WORKDIR"
(
  cd "$WORKDIR"
  IFS= read -r _ < "$START_FIFO"
  exec env \
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
OPENCODE_LAUNCH_PID=$!
printf 'start\n' > "$START_FIFO"

for _ in {1..100}; do
  launch_children=()
  children_path="/proc/$OPENCODE_LAUNCH_PID/task/$OPENCODE_LAUNCH_PID/children"
  if [[ -r "$children_path" ]]; then
    read -r -a launch_children < "$children_path" || true
    for candidate_pid in "${launch_children[@]}"; do
      if [[ -r "/proc/$candidate_pid/cmdline" ]]; then
        REGISTERED_PID="$candidate_pid"
        break 2
      fi
    done
  fi
  if ! kill -0 "$OPENCODE_LAUNCH_PID" 2>/dev/null; then
    break
  fi
  sleep 0.05
done
if [[ -z "$REGISTERED_PID" ]]; then
  echo "[soak] could not identify the OpenCode process under timeout supervisor" >&2
  exit 1
fi

curl_api -X POST -H 'Content-Type: application/json' \
  --data "{\"pid\":$REGISTERED_PID,\"tag\":\"Agent CLI\",\"cwd\":\"$WORKDIR\"}" \
  "$BASE_URL/register" >/dev/null
echo "[soak] registered OpenCode PID $REGISTERED_PID"

fi

set +e
wait "$OPENCODE_LAUNCH_PID"
opencode_status=$?
set -e
OPENCODE_LAUNCH_PID=""
: > "$RUN_DIR/opencode-done"
if [[ -n "$REGISTERED_PID" ]]; then
  curl_api -X POST -H 'Content-Type: application/json' \
    --data "{\"pid\":$REGISTERED_PID}" "$BASE_URL/unregister" >/dev/null
  REGISTERED_PID=""
fi
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

set +e
wait "$EVENT_MONITOR_PID"
event_status=$?
set -e
EVENT_MONITOR_PID=""
cat "$RUN_DIR/checker.log"
curl_api -o "$RUN_DIR/post-run-health.json" "$BASE_URL/api/v1/health"
if (( event_status != 0 )); then
  exit "$event_status"
fi

# Verify that the captured kernel records also form a usable process/activity
# topology. Export only aggregate statistics, never raw event payloads.
curl_api -o "$RUN_DIR/execution-graph.json" "$BASE_URL/events/graph?limit=1000"
python3 - "$RUN_DIR/execution-graph.json" "$RUN_DIR/graph-report.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as source:
    graph = json.load(source)
nodes, edges = graph.get("nodes"), graph.get("edges")
if not isinstance(nodes, list) or not isinstance(edges, list):
    raise SystemExit("execution graph returned invalid node/edge arrays")
node_ids = {n.get("id") for n in nodes if isinstance(n, dict)}
dangling = [
    e.get("id")
    for e in edges if isinstance(e, dict)
    and (e.get("source") not in node_ids or e.get("target") not in node_ids)
]
processes = sum(n.get("kind") == "process" for n in nodes if isinstance(n, dict))
if not nodes or not edges or not processes or dangling:
    raise SystemExit(
        f"execution graph invalid: nodes={len(nodes)} edges={len(edges)} "
        f"processes={processes} dangling={len(dangling)}"
    )
report = {
    "status": "pass",
    "source": graph.get("source"),
    "eventCount": graph.get("eventCount"),
    "nodes": len(nodes),
    "edges": len(edges),
    "processNodes": processes,
    "nodeCounts": graph.get("nodeCounts", {}),
    "edgeCounts": graph.get("edgeCounts", {}),
    "truncated": graph.get("truncated", False),
    "omittedEventCount": graph.get("omittedEventCount", 0),
    "omittedNodeCount": graph.get("omittedNodeCount", 0),
    "omittedEdgeCount": graph.get("omittedEdgeCount", 0),
    "danglingEdges": 0,
}
with open(sys.argv[2], "w", encoding="utf-8") as output:
    json.dump(report, output, indent=2, sort_keys=True)
    output.write("\n")
print(f"[soak] graph verified: {len(nodes)} nodes, {len(edges)} edges, "
      f"{processes} process nodes; no dangling edges")
PY
rm -f -- "$RUN_DIR/execution-graph.json"
printf 'OpenCode model: %s\nBackend: %s\nEvent assertions: passed\nGraph assertions: passed\n' "$MODEL" "$BASE_URL" >>"$SUMMARY"
echo "[soak] OpenCode free-model exec was observed in kernel events and graph topology"

