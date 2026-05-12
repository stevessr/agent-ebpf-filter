#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_URL="${BACKEND_URL:-}"
TOKEN="${AGENT_ACCESS_TOKEN:-${TOKEN:-}}"
OS_SMOKE_START_BACKEND="${OS_SMOKE_START_BACKEND:-0}"
OS_SMOKE_BUILD_BACKEND="${OS_SMOKE_BUILD_BACKEND:-0}"
OS_SMOKE_BACKEND_CMD="${OS_SMOKE_BACKEND_CMD:-$ROOT_DIR/backend/agent-ebpf-filter}"
OS_SMOKE_BACKEND_LOG="${OS_SMOKE_BACKEND_LOG:-/tmp/agent-ebpf-os-smoke-backend.log}"
OS_SMOKE_PRIVILEGE_CMD="${OS_SMOKE_PRIVILEGE_CMD:-}"
CGROUP_SANDBOX_PATH="${AGENT_CGROUP_SANDBOX_PATH:-}"
listener_pids=()
backend_pid=""
tmp_exec=""
tmp_exec_alias=""
tmp_file=""
tmp_dir=""
fd_probe_pid=""
fd_ready_file=""
fd_trigger_file=""
fd_result_file=""
mmap_probe_pid=""
mmap_ready_file=""
mmap_trigger_file=""
mmap_result_file=""
mprotect_probe_pid=""
mprotect_ready_file=""
mprotect_trigger_file=""
mprotect_result_file=""
setattr_probe_pid=""
setattr_ready_file=""
setattr_trigger_file=""
setattr_result_file=""
fchmod_probe_pid=""
fchmod_ready_file=""
fchmod_trigger_file=""
fchmod_result_file=""
blocked_exec_paths=()
blocked_exec_names=()
blocked_file_names=()
blocked_cgroups=()
blocked_ips=()
blocked_ports=()
test_cgroups=()

refresh_backend_url() {
  if [[ -n "${BACKEND_URL:-}" ]]; then
    return 0
  fi
  if [[ -f "$ROOT_DIR/backend/.port" ]]; then
    BACKEND_URL="http://127.0.0.1:$(cat "$ROOT_DIR/backend/.port")"
  else
    BACKEND_URL="http://127.0.0.1:8080"
  fi
}

refresh_backend_url

curl_json() {
  local method="$1"
  local path="$2"
  local body="${3:-}"
  local args=(-fsS -X "$method" "$BACKEND_URL$path" -H "Content-Type: application/json")
  if [[ -n "$TOKEN" ]]; then
    args+=(-H "X-API-KEY: $TOKEN")
  fi
  if [[ -n "$body" ]]; then
    args+=(-d "$body")
  fi
  curl "${args[@]}"
}

assert_idempotent_unblock() {
  local path="$1"
  local body="$2"
  curl_json POST "$path" "$body" >/dev/null
  curl_json POST "$path" "$body" >/dev/null
}

require_backend() {
  if ! curl_json GET /sandbox/cgroup/status >/dev/null; then
    cat >&2 <<EOF
[os-smoke] Backend is not reachable at $BACKEND_URL.
[os-smoke] Start the backend with privileges first, for example:
  DISABLE_AUTH=true sudo -E ./backend/agent-ebpf-filter
EOF
    exit 1
  fi
}

wait_for_backend() {
  for _ in $(seq 1 100); do
    if [[ -z "${BACKEND_URL:-}" || "$BACKEND_URL" == "http://127.0.0.1:8080" ]]; then
      BACKEND_URL=""
      refresh_backend_url
    fi
    if curl_json GET /sandbox/cgroup/status >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
  done
  echo "[os-smoke] backend did not become reachable at $BACKEND_URL" >&2
  if [[ -f "$OS_SMOKE_BACKEND_LOG" ]]; then
    echo "[os-smoke] backend log tail ($OS_SMOKE_BACKEND_LOG):" >&2
    tail -80 "$OS_SMOKE_BACKEND_LOG" >&2 || true
  fi
  return 1
}

custom_privilege_prefix() {
  local -n _out="$1"
  _out=()
  if [[ -n "$OS_SMOKE_PRIVILEGE_CMD" ]]; then
    # shellcheck disable=SC2206 # simple argv splitting for operator-provided command prefix.
    _out=($OS_SMOKE_PRIVILEGE_CMD)
  fi
}

build_backend_privilege_prefix() {
  local -n _out="$1"
  local custom=()
  local custom_uid
  custom_privilege_prefix custom
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    _out=(env)
  elif (( ${#custom[@]} > 0 )); then
    custom_uid="$("${custom[@]}" id -u 2>/dev/null || true)"
    if [[ "$custom_uid" != "0" ]]; then
      echo "[os-smoke] OS_SMOKE_PRIVILEGE_CMD did not run commands as root (uid=${custom_uid:-unknown})." >&2
      echo "[os-smoke] Example: rtk env OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start" >&2
      exit 1
    fi
    _out=("${custom[@]}" env)
  elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    _out=(sudo -E env)
  else
    echo "[os-smoke] OS_SMOKE_START_BACKEND=1 requires root, passwordless sudo, or OS_SMOKE_PRIVILEGE_CMD." >&2
    echo "[os-smoke] Example: rtk env OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start" >&2
    echo "[os-smoke] Alternatively start the backend manually, then rerun this script." >&2
    exit 1
  fi
}

start_backend_if_requested() {
  if [[ "$OS_SMOKE_START_BACKEND" != "1" ]]; then
    return 0
  fi

  local run_prefix=()
  build_backend_privilege_prefix run_prefix

  if [[ "$OS_SMOKE_BUILD_BACKEND" == "1" || ! -x "$OS_SMOKE_BACKEND_CMD" ]]; then
    echo "[os-smoke] building backend binary"
    (cd "$ROOT_DIR" && make backend)
  fi
  if [[ ! -x "$OS_SMOKE_BACKEND_CMD" ]]; then
    echo "[os-smoke] backend binary is not executable: $OS_SMOKE_BACKEND_CMD" >&2
    echo "[os-smoke] Run: rtk make backend" >&2
    exit 1
  fi

  rm -f "$ROOT_DIR/backend/.port"
  BACKEND_URL=""
  : >"$OS_SMOKE_BACKEND_LOG"
  echo "[os-smoke] starting privileged backend: $OS_SMOKE_BACKEND_CMD"
  "${run_prefix[@]}" DISABLE_AUTH=true GIN_MODE=debug "$OS_SMOKE_BACKEND_CMD" >"$OS_SMOKE_BACKEND_LOG" 2>&1 &
  backend_pid=$!
  wait_for_backend
}

enable_policy_management() {
  curl_json PUT /config/runtime '{"policyManagementEnabled":true}' >/dev/null
}

make_temp_executable() {
  local path
  path="$(mktemp /tmp/agent-ebpf-lsm-exec.XXXXXX)"
  cat >"$path" <<'EOF'
#!/usr/bin/env sh
exit 0
EOF
  chmod +x "$path"
  printf '%s' "$path"
}

python_connect() {
  local host="$1"
  local port="$2"
  python3 - "$host" "$port" <<'PY'
import socket
import sys

host = sys.argv[1]
port = int(sys.argv[2])
with socket.create_connection((host, port), timeout=1):
    pass
PY
}

python_udp_connect() {
  local host="$1"
  local port="$2"
  python3 - "$host" "$port" <<'PY'
import socket
import sys

host = sys.argv[1]
port = int(sys.argv[2])
family = socket.AF_INET6 if ":" in host else socket.AF_INET
s = socket.socket(family, socket.SOCK_DGRAM)
s.settimeout(1)
s.connect((host, port))
s.close()
PY
}

python_udp_sendto() {
  local host="$1"
  local port="$2"
  python3 - "$host" "$port" <<'PY'
import socket
import sys

host = sys.argv[1]
port = int(sys.argv[2])
family = socket.AF_INET6 if ":" in host else socket.AF_INET
s = socket.socket(family, socket.SOCK_DGRAM)
s.settimeout(1)
s.sendto(b"agent-ebpf-os-smoke", (host, port))
s.close()
PY
}

start_tcp_listener() {
  local host="${1:-127.0.0.1}"
  python3 - "$host" <<'PY' &
import socket
import sys
import time

host = sys.argv[1]
family = socket.AF_INET6 if ":" in host else socket.AF_INET
s = socket.socket(family)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
if family == socket.AF_INET6:
    s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
s.bind((host, 0))
s.listen(16)
print(s.getsockname()[1], flush=True)
deadline = time.time() + 45
while time.time() < deadline:
    try:
        s.settimeout(1)
        conn, _ = s.accept()
        conn.close()
    except socket.timeout:
        pass
PY
}

wait_for_listener_port() {
  local port_file="$1"
  local pid="$2"
  local label="$3"
  for _ in $(seq 1 50); do
    if [[ -s "$port_file" ]]; then
      cat "$port_file"
      return 0
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      wait "$pid" || true
      echo "[os-smoke] $label listener exited before reporting a port" >&2
      return 1
    fi
    sleep 0.1
  done
  echo "[os-smoke] timed out waiting for $label listener port" >&2
  return 1
}

ipv6_loopback_available() {
  python3 - <<'PY'
import socket

try:
    s = socket.socket(socket.AF_INET6)
    s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
    s.bind(("::1", 0))
    s.close()
except OSError:
    raise SystemExit(1)
PY
}

ipv4_mapped_loopback_available() {
  python3 - <<'PY'
import socket

server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
server.bind(("127.0.0.1", 0))
server.listen(1)
port = server.getsockname()[1]

client = socket.socket(socket.AF_INET6, socket.SOCK_STREAM)
client.settimeout(1)
try:
    client.connect(("::ffff:127.0.0.1", port))
    conn, _ = server.accept()
    conn.close()
finally:
    client.close()
    server.close()
PY
}

cleanup() {
  cleanup_policy_entries
  [[ -n "$tmp_exec" ]] && rm -f "$tmp_exec"
  [[ -n "$tmp_exec_alias" ]] && rm -f "$tmp_exec_alias"
  [[ -n "$tmp_file" ]] && rm -f "$tmp_file"
  [[ -n "$fd_probe_pid" ]] && kill "$fd_probe_pid" 2>/dev/null || true
  [[ -n "$fd_ready_file" ]] && rm -f "$fd_ready_file"
  [[ -n "$fd_trigger_file" ]] && rm -f "$fd_trigger_file"
  [[ -n "$fd_result_file" ]] && rm -f "$fd_result_file"
  [[ -n "$mmap_probe_pid" ]] && kill "$mmap_probe_pid" 2>/dev/null || true
  [[ -n "$mmap_ready_file" ]] && rm -f "$mmap_ready_file"
  [[ -n "$mmap_trigger_file" ]] && rm -f "$mmap_trigger_file"
  [[ -n "$mmap_result_file" ]] && rm -f "$mmap_result_file"
  [[ -n "$mprotect_probe_pid" ]] && kill "$mprotect_probe_pid" 2>/dev/null || true
  [[ -n "$mprotect_ready_file" ]] && rm -f "$mprotect_ready_file"
  [[ -n "$mprotect_trigger_file" ]] && rm -f "$mprotect_trigger_file"
  [[ -n "$mprotect_result_file" ]] && rm -f "$mprotect_result_file"
  [[ -n "$setattr_probe_pid" ]] && kill "$setattr_probe_pid" 2>/dev/null || true
  [[ -n "$setattr_ready_file" ]] && rm -f "$setattr_ready_file"
  [[ -n "$setattr_trigger_file" ]] && rm -f "$setattr_trigger_file"
  [[ -n "$setattr_result_file" ]] && rm -f "$setattr_result_file"
  [[ -n "$fchmod_probe_pid" ]] && kill "$fchmod_probe_pid" 2>/dev/null || true
  [[ -n "$fchmod_ready_file" ]] && rm -f "$fchmod_ready_file"
  [[ -n "$fchmod_trigger_file" ]] && rm -f "$fchmod_trigger_file"
  [[ -n "$fchmod_result_file" ]] && rm -f "$fchmod_result_file"
  [[ -n "$tmp_dir" ]] && rmdir "$tmp_dir" >/dev/null 2>&1 || true
  for pid in "${listener_pids[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  if [[ -n "$backend_pid" ]]; then
    kill "$backend_pid" 2>/dev/null || true
    run_privileged kill "$backend_pid" 2>/dev/null || true
  fi
  for cg in "${test_cgroups[@]}"; do
    run_privileged rmdir "$cg" >/dev/null 2>&1 || true
  done
}

cleanup_policy_entries() {
  for path in "${blocked_exec_paths[@]}"; do
    curl_json POST /sandbox/lsm/unblock-exec-path "{\"path\":\"$path\"}" >/dev/null 2>&1 || true
  done
  for name in "${blocked_exec_names[@]}"; do
    curl_json POST /sandbox/lsm/unblock-exec-name "{\"name\":\"$name\"}" >/dev/null 2>&1 || true
  done
  for name in "${blocked_file_names[@]}"; do
    curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$name\"}" >/dev/null 2>&1 || true
  done
  for cgroup_id in "${blocked_cgroups[@]}"; do
    curl_json POST /sandbox/cgroup/unblock-cgroup "{\"cgroupId\":\"$cgroup_id\"}" >/dev/null 2>&1 || true
  done
  for ip in "${blocked_ips[@]}"; do
    curl_json POST /sandbox/cgroup/unblock-ip "{\"ip\":\"$ip\"}" >/dev/null 2>&1 || true
  done
  for port in "${blocked_ports[@]}"; do
    curl_json POST /sandbox/cgroup/unblock-port "{\"port\":$port}" >/dev/null 2>&1 || true
  done
}

run_privileged() {
  local custom=()
  custom_privilege_prefix custom
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    "$@"
  elif (( ${#custom[@]} > 0 )); then
    "${custom[@]}" "$@"
  elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    sudo -n "$@"
  else
    return 1
  fi
}

write_privileged_file() {
  local value="$1"
  local path="$2"
  local custom=()
  custom_privilege_prefix custom
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    printf '%s' "$value" >"$path"
  elif (( ${#custom[@]} > 0 )); then
    printf '%s' "$value" | "${custom[@]}" tee "$path" >/dev/null
  elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then
    printf '%s' "$value" | sudo -n tee "$path" >/dev/null
  else
    return 1
  fi
}

json_field() {
  local field="$1"
  python3 -c 'import json, sys; print(json.load(sys.stdin).get(sys.argv[1], ""))' "$field"
}

loopback_ipv4_available() {
  local host="$1"
  python3 - "$host" <<'PY'
import socket
import sys

host = sys.argv[1]
try:
    s = socket.socket(socket.AF_INET)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind((host, 0))
    s.close()
except OSError:
    raise SystemExit(1)
PY
}

start_connect_probe() {
  local host="$1"
  local port="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$host" "$port" "$trigger_file" "$result_file" <<'PY' &
import os
import socket
import sys
import time

host = sys.argv[1]
port = int(sys.argv[2])
trigger_file = sys.argv[3]
result_file = sys.argv[4]

deadline = time.time() + 20
while time.time() < deadline and not os.path.exists(trigger_file):
    time.sleep(0.05)

if not os.path.exists(trigger_file):
    status = "timeout"
else:
    try:
        with socket.create_connection((host, port), timeout=1):
            status = "ok"
    except OSError:
        status = "blocked"

with open(result_file, "w", encoding="utf-8") as fh:
    fh.write(status + "\n")
PY
}

start_udp_sendto_probe() {
  local host="$1"
  local port="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$host" "$port" "$trigger_file" "$result_file" <<'PY' &
import os
import socket
import sys
import time

host = sys.argv[1]
port = int(sys.argv[2])
trigger_file = sys.argv[3]
result_file = sys.argv[4]

deadline = time.time() + 20
while time.time() < deadline and not os.path.exists(trigger_file):
    time.sleep(0.05)

if not os.path.exists(trigger_file):
    status = "timeout"
else:
    try:
        family = socket.AF_INET6 if ":" in host else socket.AF_INET
        s = socket.socket(family, socket.SOCK_DGRAM)
        s.settimeout(1)
        s.sendto(b"agent-ebpf-cgroup-pid-sendmsg", (host, port))
        s.close()
        status = "ok"
    except OSError:
        status = "blocked"

with open(result_file, "w", encoding="utf-8") as fh:
    fh.write(status + "\n")
PY
}

start_connected_udp_send_probe() {
  local host="$1"
  local port="$2"
  local ready_file="$3"
  local trigger_file="$4"
  local result_file="$5"
  python3 - "$host" "$port" "$ready_file" "$trigger_file" "$result_file" <<'PY' &
import os
import socket
import sys
import time

host = sys.argv[1]
port = int(sys.argv[2])
ready_file = sys.argv[3]
trigger_file = sys.argv[4]
result_file = sys.argv[5]

family = socket.AF_INET6 if ":" in host else socket.AF_INET
s = socket.socket(family, socket.SOCK_DGRAM)
s.settimeout(1)
s.connect((host, port))

with open(ready_file, "w", encoding="utf-8") as ready:
    ready.write("ready\n")

deadline = time.time() + 20
while time.time() < deadline and not os.path.exists(trigger_file):
    time.sleep(0.05)

if not os.path.exists(trigger_file):
    status = "timeout"
else:
    try:
        s.send(b"agent-ebpf-cgroup-existing-udp-send")
        status = "ok"
    except OSError:
        status = "blocked"

s.close()
with open(result_file, "w", encoding="utf-8") as fh:
    fh.write(status + "\n")
PY
}

start_existing_fd_probe() {
  local path="$1"
  local ready_file="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$path" "$ready_file" "$trigger_file" "$result_file" <<'PY' &
import os
import sys
import time

path = sys.argv[1]
ready_file = sys.argv[2]
trigger_file = sys.argv[3]
result_file = sys.argv[4]

with open(path, "r+b", buffering=0) as fh:
    with open(ready_file, "w", encoding="utf-8") as ready:
        ready.write("ready\n")

    deadline = time.time() + 20
    while time.time() < deadline and not os.path.exists(trigger_file):
        time.sleep(0.05)

    if not os.path.exists(trigger_file):
        status = "timeout"
    else:
        try:
            fh.seek(0)
            fh.read(1)
            fh.seek(0, os.SEEK_END)
            fh.write(b"blocked-existing-fd\n")
            status = "ok"
        except OSError:
            status = "blocked"

with open(result_file, "w", encoding="utf-8") as result:
    result.write(status + "\n")
PY
}

start_existing_fd_setattr_probe() {
  local path="$1"
  local ready_file="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$path" "$ready_file" "$trigger_file" "$result_file" <<'PY' &
import os
import sys
import time

path = sys.argv[1]
ready_file = sys.argv[2]
trigger_file = sys.argv[3]
result_file = sys.argv[4]

fd = os.open(path, os.O_RDWR)
try:
    with open(ready_file, "w", encoding="utf-8") as ready:
        ready.write("ready\n")

    deadline = time.time() + 20
    while time.time() < deadline and not os.path.exists(trigger_file):
        time.sleep(0.05)

    if not os.path.exists(trigger_file):
        status = "timeout"
    else:
        try:
            os.ftruncate(fd, 0)
            status = "ok"
        except OSError:
            status = "blocked"
finally:
    os.close(fd)

with open(result_file, "w", encoding="utf-8") as result:
    result.write(status + "\n")
PY
}

start_existing_fd_fchmod_probe() {
  local path="$1"
  local ready_file="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$path" "$ready_file" "$trigger_file" "$result_file" <<'PY' &
import os
import sys
import time

path = sys.argv[1]
ready_file = sys.argv[2]
trigger_file = sys.argv[3]
result_file = sys.argv[4]

fd = os.open(path, os.O_RDWR)
try:
    with open(ready_file, "w", encoding="utf-8") as ready:
        ready.write("ready\n")

    deadline = time.time() + 20
    while time.time() < deadline and not os.path.exists(trigger_file):
        time.sleep(0.05)

    if not os.path.exists(trigger_file):
        status = "timeout"
    else:
        try:
            os.fchmod(fd, 0o640)
            status = "ok"
        except OSError:
            status = "blocked"
finally:
    os.close(fd)

with open(result_file, "w", encoding="utf-8") as result:
    result.write(status + "\n")
PY
}

start_existing_fd_mmap_probe() {
  local path="$1"
  local ready_file="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$path" "$ready_file" "$trigger_file" "$result_file" <<'PY' &
import mmap
import os
import sys
import time

path = sys.argv[1]
ready_file = sys.argv[2]
trigger_file = sys.argv[3]
result_file = sys.argv[4]

with open(path, "rb", buffering=0) as fh:
    with open(ready_file, "w", encoding="utf-8") as ready:
        ready.write("ready\n")

    deadline = time.time() + 20
    while time.time() < deadline and not os.path.exists(trigger_file):
        time.sleep(0.05)

    if not os.path.exists(trigger_file):
        status = "timeout"
    else:
        try:
            mm = mmap.mmap(fh.fileno(), 0, access=mmap.ACCESS_READ)
            try:
                _ = mm[:1]
            finally:
                mm.close()
            status = "ok"
        except OSError:
            status = "blocked"

with open(result_file, "w", encoding="utf-8") as result:
    result.write(status + "\n")
PY
}

start_existing_map_mprotect_probe() {
  local path="$1"
  local ready_file="$2"
  local trigger_file="$3"
  local result_file="$4"
  python3 - "$path" "$ready_file" "$trigger_file" "$result_file" <<'PY' &
import ctypes
import errno
import os
import sys
import time

path = sys.argv[1]
ready_file = sys.argv[2]
trigger_file = sys.argv[3]
result_file = sys.argv[4]

PROT_READ = 0x1
PROT_WRITE = 0x2
MAP_SHARED = 0x01
MAP_FAILED = ctypes.c_void_p(-1).value

libc = ctypes.CDLL(None, use_errno=True)
libc.mmap.restype = ctypes.c_void_p
libc.mmap.argtypes = [
    ctypes.c_void_p,
    ctypes.c_size_t,
    ctypes.c_int,
    ctypes.c_int,
    ctypes.c_int,
    ctypes.c_long,
]
libc.mprotect.restype = ctypes.c_int
libc.mprotect.argtypes = [ctypes.c_void_p, ctypes.c_size_t, ctypes.c_int]
libc.munmap.restype = ctypes.c_int
libc.munmap.argtypes = [ctypes.c_void_p, ctypes.c_size_t]

fd = os.open(path, os.O_RDWR)
addr = MAP_FAILED
size = max(os.path.getsize(path), 1)
try:
    addr = libc.mmap(None, size, PROT_READ, MAP_SHARED, fd, 0)
    if addr == MAP_FAILED:
        with open(ready_file, "w", encoding="utf-8") as ready:
            ready.write(f"setup_error:{ctypes.get_errno()}\n")
        sys.exit(0)

    with open(ready_file, "w", encoding="utf-8") as ready:
        ready.write("ready\n")

    deadline = time.time() + 20
    while time.time() < deadline and not os.path.exists(trigger_file):
        time.sleep(0.05)

    if not os.path.exists(trigger_file):
        status = "timeout"
    elif libc.mprotect(ctypes.c_void_p(addr), size, PROT_READ | PROT_WRITE) == 0:
        status = "ok"
    else:
        err = ctypes.get_errno()
        status = "blocked" if err in (errno.EACCES, errno.EPERM) else f"error:{err}"
finally:
    if addr != MAP_FAILED:
        libc.munmap(ctypes.c_void_p(addr), size)
    os.close(fd)

with open(result_file, "w", encoding="utf-8") as result:
    result.write(status + "\n")
PY
}

wait_for_probe_result() {
  local result_file="$1"
  local label="$2"
  for _ in $(seq 1 80); do
    if [[ -s "$result_file" ]]; then
      cat "$result_file"
      return 0
    fi
    sleep 0.1
  done
  echo "[os-smoke] timed out waiting for $label probe result" >&2
  return 1
}

run_probe_in_cgroup() {
  local cgroup_dir="$1"
  local host="$2"
  local port="$3"
  local label="$4"
  local trigger_file result_file probe_pid result

  trigger_file="$(mktemp /tmp/agent-ebpf-cgroup-trigger.XXXXXX)"
  result_file="$(mktemp /tmp/agent-ebpf-cgroup-result.XXXXXX)"
  rm -f "$trigger_file" "$result_file"
  start_connect_probe "$host" "$port" "$trigger_file" "$result_file"
  probe_pid=$!
  if ! write_privileged_file "$probe_pid" "$cgroup_dir/cgroup.procs"; then
    kill "$probe_pid" 2>/dev/null || true
    rm -f "$trigger_file" "$result_file"
    echo "[os-smoke] could not move probe PID $probe_pid into $cgroup_dir" >&2
    return 1
  fi
  : >"$trigger_file"
  result="$(wait_for_probe_result "$result_file" "$label")"
  wait "$probe_pid" || true
  rm -f "$trigger_file" "$result_file"
  printf '%s' "$result"
}

run_udp_sendto_probe_in_cgroup() {
  local cgroup_dir="$1"
  local host="$2"
  local port="$3"
  local label="$4"
  local trigger_file result_file probe_pid result

  trigger_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-trigger.XXXXXX)"
  result_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-result.XXXXXX)"
  rm -f "$trigger_file" "$result_file"
  start_udp_sendto_probe "$host" "$port" "$trigger_file" "$result_file"
  probe_pid=$!
  if ! write_privileged_file "$probe_pid" "$cgroup_dir/cgroup.procs"; then
    kill "$probe_pid" 2>/dev/null || true
    rm -f "$trigger_file" "$result_file"
    echo "[os-smoke] could not move UDP sendto probe PID $probe_pid into $cgroup_dir" >&2
    return 1
  fi
  : >"$trigger_file"
  result="$(wait_for_probe_result "$result_file" "$label")"
  wait "$probe_pid" || true
  rm -f "$trigger_file" "$result_file"
  printf '%s' "$result"
}

run_cgroup_pid_block_smoke() {
  local host="127.0.0.1"
  local label="PID-cgroup"
  local cgroup_root="${CGROUP_SANDBOX_PATH:-/sys/fs/cgroup}"
  local cgroup_dir port_file listener_pid port result trigger_file result_file probe_pid block_resp cgroup_id

  cgroup_dir="$cgroup_root/agent-ebpf-os-smoke-$$-$RANDOM"
  if ! run_privileged mkdir "$cgroup_dir" >/dev/null 2>&1; then
    echo "[os-smoke] cannot create cgroup under $cgroup_root; skipped PID cgroup block smoke"
    return 0
  fi
  test_cgroups+=("$cgroup_dir")

  echo "[os-smoke] cgroup/connect PID cgroup block"
  port_file="$(mktemp /tmp/agent-ebpf-cgroup-port.XXXXXX)"
  start_tcp_listener "$host" >"$port_file"
  listener_pid=$!
  listener_pids+=("$listener_pid")
  port="$(wait_for_listener_port "$port_file" "$listener_pid" "$label")"
  rm -f "$port_file"

  result="$(run_probe_in_cgroup "$cgroup_dir" "$host" "$port" "$label-baseline")"
  if [[ "$result" != "ok" ]]; then
    echo "[os-smoke] expected baseline PID-cgroup connect to succeed, got $result" >&2
    exit 1
  fi
  result="$(run_udp_sendto_probe_in_cgroup "$cgroup_dir" "$host" "$port" "$label-udp-baseline")"
  if [[ "$result" != "ok" ]]; then
    echo "[os-smoke] expected baseline PID-cgroup UDP sendto to succeed, got $result" >&2
    exit 1
  fi

  trigger_file="$(mktemp /tmp/agent-ebpf-cgroup-trigger.XXXXXX)"
  result_file="$(mktemp /tmp/agent-ebpf-cgroup-result.XXXXXX)"
  rm -f "$trigger_file" "$result_file"
  start_connect_probe "$host" "$port" "$trigger_file" "$result_file"
  probe_pid=$!
  if ! write_privileged_file "$probe_pid" "$cgroup_dir/cgroup.procs"; then
    kill "$probe_pid" 2>/dev/null || true
    echo "[os-smoke] could not move blocked probe PID $probe_pid into $cgroup_dir" >&2
    exit 1
  fi

  block_resp="$(curl_json POST /sandbox/cgroup/block-pid "{\"pid\":$probe_pid}")"
  cgroup_id="$(printf '%s' "$block_resp" | json_field cgroupId)"
  if [[ -z "$cgroup_id" ]]; then
    echo "[os-smoke] block-pid response missing cgroupId: $block_resp" >&2
    kill "$probe_pid" 2>/dev/null || true
    exit 1
  fi
  blocked_cgroups+=("$cgroup_id")
  : >"$trigger_file"
  result="$(wait_for_probe_result "$result_file" "$label-blocked")"
  wait "$probe_pid" || true
  rm -f "$trigger_file" "$result_file"
  if [[ "$result" != "blocked" ]]; then
    echo "[os-smoke] expected PID-cgroup connect to be blocked, got $result" >&2
    curl_json POST /sandbox/cgroup/unblock-cgroup "{\"cgroupId\":\"$cgroup_id\"}" >/dev/null || true
    exit 1
  fi
  echo "[os-smoke] cgroup/sendmsg PID cgroup UDP sendto block"
  result="$(run_udp_sendto_probe_in_cgroup "$cgroup_dir" "$host" "$port" "$label-udp-blocked")"
  if [[ "$result" != "blocked" ]]; then
    echo "[os-smoke] expected PID-cgroup UDP sendto to be blocked, got $result" >&2
    curl_json POST /sandbox/cgroup/unblock-cgroup "{\"cgroupId\":\"$cgroup_id\"}" >/dev/null || true
    exit 1
  fi

  echo "[os-smoke] cgroup/connect PID cgroup unblock-pid"
  trigger_file="$(mktemp /tmp/agent-ebpf-cgroup-trigger.XXXXXX)"
  result_file="$(mktemp /tmp/agent-ebpf-cgroup-result.XXXXXX)"
  rm -f "$trigger_file" "$result_file"
  start_connect_probe "$host" "$port" "$trigger_file" "$result_file"
  probe_pid=$!
  if ! write_privileged_file "$probe_pid" "$cgroup_dir/cgroup.procs"; then
    kill "$probe_pid" 2>/dev/null || true
    rm -f "$trigger_file" "$result_file"
    echo "[os-smoke] could not move unblock-pid probe PID $probe_pid into $cgroup_dir" >&2
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-pid "{\"pid\":$probe_pid}"
  : >"$trigger_file"
  result="$(wait_for_probe_result "$result_file" "$label-unblock-pid")"
  wait "$probe_pid" || true
  rm -f "$trigger_file" "$result_file"
  if [[ "$result" != "ok" ]]; then
    echo "[os-smoke] expected PID-cgroup connect to succeed after unblock-pid, got $result" >&2
    curl_json POST /sandbox/cgroup/unblock-cgroup "{\"cgroupId\":\"$cgroup_id\"}" >/dev/null || true
    exit 1
  fi
  result="$(run_udp_sendto_probe_in_cgroup "$cgroup_dir" "$host" "$port" "$label-udp-unblock-pid")"
  if [[ "$result" != "ok" ]]; then
    echo "[os-smoke] expected PID-cgroup UDP sendto to succeed after unblock-pid, got $result" >&2
    curl_json POST /sandbox/cgroup/unblock-cgroup "{\"cgroupId\":\"$cgroup_id\"}" >/dev/null || true
    exit 1
  fi

  assert_idempotent_unblock /sandbox/cgroup/unblock-cgroup "{\"cgroupId\":\"$cgroup_id\"}"
  result="$(run_probe_in_cgroup "$cgroup_dir" "$host" "$port" "$label-unblocked")"
  if [[ "$result" != "ok" ]]; then
    echo "[os-smoke] expected PID-cgroup connect to succeed after unblock, got $result" >&2
    exit 1
  fi

  kill "$listener_pid" 2>/dev/null || true
}

run_ip_block_smoke() {
  local host="$1"
  local label="$2"
  local port_file listener_pid port

  echo "[os-smoke] cgroup/connect IP destination block ($label)"
  port_file="$(mktemp /tmp/agent-ebpf-ip.XXXXXX)"
  start_tcp_listener "$host" >"$port_file"
  listener_pid=$!
  listener_pids+=("$listener_pid")
  port="$(wait_for_listener_port "$port_file" "$listener_pid" "$label")"
  rm -f "$port_file"

  python_connect "$host" "$port"
  curl_json POST /sandbox/cgroup/block-ip "{\"ip\":\"$host\"}" >/dev/null
  blocked_ips+=("$host")
  if python_connect "$host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected $label connect to $host:$port to be blocked by IP, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}"
  python_connect "$host" "$port"

  kill "$listener_pid" 2>/dev/null || true
}

run_ipv4_mapped_ip_block_smoke() {
  local mapped_host="::ffff:127.0.0.1"
  local block_ip="127.0.0.1"
  local label="IPv4-mapped-IPv6"
  local port_file listener_pid port

  echo "[os-smoke] cgroup/connect IPv4 block applies to IPv4-mapped IPv6 destination ($label)"
  port_file="$(mktemp /tmp/agent-ebpf-ipv4-mapped.XXXXXX)"
  start_tcp_listener "$block_ip" >"$port_file"
  listener_pid=$!
  listener_pids+=("$listener_pid")
  port="$(wait_for_listener_port "$port_file" "$listener_pid" "$label")"
  rm -f "$port_file"

  python_connect "$mapped_host" "$port"
  curl_json POST /sandbox/cgroup/block-ip "{\"ip\":\"$block_ip\"}" >/dev/null
  blocked_ips+=("$block_ip")
  if python_connect "$mapped_host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected IPv4 block $block_ip to deny mapped IPv6 connect to $mapped_host:$port, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-ip "{\"ip\":\"$block_ip\"}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-ip "{\"ip\":\"$block_ip\"}"
  python_connect "$mapped_host" "$port"

  kill "$listener_pid" 2>/dev/null || true
}

run_port_block_smoke() {
  local host="$1"
  local label="$2"
  local port_file listener_pid port

  echo "[os-smoke] cgroup/connect port block ($label)"
  port_file="$(mktemp /tmp/agent-ebpf-port.XXXXXX)"
  start_tcp_listener "$host" >"$port_file"
  listener_pid=$!
  listener_pids+=("$listener_pid")
  port="$(wait_for_listener_port "$port_file" "$listener_pid" "$label")"
  rm -f "$port_file"

  python_connect "$host" "$port"
  curl_json POST /sandbox/cgroup/block-port "{\"port\":$port}" >/dev/null
  blocked_ports+=("$port")
  if python_connect "$host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected $label connect to port $port to be blocked, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-port "{\"port\":$port}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-port "{\"port\":$port}"
  python_connect "$host" "$port"

  kill "$listener_pid" 2>/dev/null || true
}

run_udp_ip_block_smoke() {
  local host="$1"
  local label="$2"
  local port="${3:-46991}"

  echo "[os-smoke] cgroup/connect UDP IP destination block ($label)"
  python_udp_connect "$host" "$port"
  curl_json POST /sandbox/cgroup/block-ip "{\"ip\":\"$host\"}" >/dev/null
  blocked_ips+=("$host")
  if python_udp_connect "$host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected UDP $label connect to $host:$port to be blocked by IP, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}"
  python_udp_connect "$host" "$port"
}

run_udp_port_block_smoke() {
  local host="$1"
  local label="$2"
  local port="${3:-46992}"

  echo "[os-smoke] cgroup/connect UDP port block ($label)"
  python_udp_connect "$host" "$port"
  curl_json POST /sandbox/cgroup/block-port "{\"port\":$port}" >/dev/null
  blocked_ports+=("$port")
  if python_udp_connect "$host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected UDP $label connect to port $port to be blocked, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-port "{\"port\":$port}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-port "{\"port\":$port}"
  python_udp_connect "$host" "$port"
}

run_udp_sendto_ip_block_smoke() {
  local host="$1"
  local label="$2"
  local port="${3:-46993}"

  echo "[os-smoke] cgroup/sendmsg UDP sendto IP destination block ($label)"
  python_udp_sendto "$host" "$port"
  curl_json POST /sandbox/cgroup/block-ip "{\"ip\":\"$host\"}" >/dev/null
  blocked_ips+=("$host")
  if python_udp_sendto "$host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected UDP sendto $label to $host:$port to be blocked by IP, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}"
  python_udp_sendto "$host" "$port"
}

run_udp_sendto_port_block_smoke() {
  local host="$1"
  local label="$2"
  local port="${3:-46994}"

  echo "[os-smoke] cgroup/sendmsg UDP sendto port block ($label)"
  python_udp_sendto "$host" "$port"
  curl_json POST /sandbox/cgroup/block-port "{\"port\":$port}" >/dev/null
  blocked_ports+=("$port")
  if python_udp_sendto "$host" "$port" 2>/dev/null; then
    echo "[os-smoke] expected UDP sendto $label to port $port to be blocked, but it succeeded" >&2
    curl_json POST /sandbox/cgroup/unblock-port "{\"port\":$port}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-port "{\"port\":$port}"
  python_udp_sendto "$host" "$port"
}

run_udp_existing_connected_send_ip_block_smoke() {
  local host="$1"
  local label="$2"
  local port="${3:-46995}"
  local ready_file trigger_file result_file probe_pid result

  echo "[os-smoke] cgroup/sendmsg existing UDP connected-socket IP block ($label)"
  ready_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-ready.XXXXXX)"
  trigger_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-trigger.XXXXXX)"
  result_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-result.XXXXXX)"
  rm -f "$ready_file" "$trigger_file" "$result_file"
  start_connected_udp_send_probe "$host" "$port" "$ready_file" "$trigger_file" "$result_file"
  probe_pid=$!
  if [[ "$(wait_for_probe_result "$ready_file" "existing UDP connected-socket ready")" != "ready" ]]; then
    echo "[os-smoke] existing UDP connected-socket probe did not become ready" >&2
    kill "$probe_pid" 2>/dev/null || true
    exit 1
  fi

  curl_json POST /sandbox/cgroup/block-ip "{\"ip\":\"$host\"}" >/dev/null
  blocked_ips+=("$host")
  : >"$trigger_file"
  result="$(wait_for_probe_result "$result_file" "existing UDP connected-socket IP block")"
  wait "$probe_pid" || true
  rm -f "$ready_file" "$trigger_file" "$result_file"
  if [[ "$result" != "blocked" ]]; then
    echo "[os-smoke] expected existing UDP connected-socket $label send to $host:$port to be blocked by IP, got $result" >&2
    curl_json POST /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-ip "{\"ip\":\"$host\"}"
  python_udp_sendto "$host" "$port"
}

run_udp_existing_connected_send_port_block_smoke() {
  local host="$1"
  local label="$2"
  local port="${3:-46996}"
  local ready_file trigger_file result_file probe_pid result

  echo "[os-smoke] cgroup/sendmsg existing UDP connected-socket port block ($label)"
  ready_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-ready.XXXXXX)"
  trigger_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-trigger.XXXXXX)"
  result_file="$(mktemp /tmp/agent-ebpf-cgroup-udp-result.XXXXXX)"
  rm -f "$ready_file" "$trigger_file" "$result_file"
  start_connected_udp_send_probe "$host" "$port" "$ready_file" "$trigger_file" "$result_file"
  probe_pid=$!
  if [[ "$(wait_for_probe_result "$ready_file" "existing UDP connected-socket ready")" != "ready" ]]; then
    echo "[os-smoke] existing UDP connected-socket probe did not become ready" >&2
    kill "$probe_pid" 2>/dev/null || true
    exit 1
  fi

  curl_json POST /sandbox/cgroup/block-port "{\"port\":$port}" >/dev/null
  blocked_ports+=("$port")
  : >"$trigger_file"
  result="$(wait_for_probe_result "$result_file" "existing UDP connected-socket port block")"
  wait "$probe_pid" || true
  rm -f "$ready_file" "$trigger_file" "$result_file"
  if [[ "$result" != "blocked" ]]; then
    echo "[os-smoke] expected existing UDP connected-socket $label send to port $port to be blocked, got $result" >&2
    curl_json POST /sandbox/cgroup/unblock-port "{\"port\":$port}" >/dev/null || true
    exit 1
  fi
  assert_idempotent_unblock /sandbox/cgroup/unblock-port "{\"port\":$port}"
  python_udp_sendto "$host" "$port"
}

trap cleanup EXIT
start_backend_if_requested
require_backend
enable_policy_management

echo "[os-smoke] backend: $BACKEND_URL"

echo "[os-smoke] checking cgroup/connect status"
cgroup_status="$(curl_json GET /sandbox/cgroup/status)"
echo "$cgroup_status" | grep -q '"available":true' || {
  echo "$cgroup_status"
  echo "[os-smoke] cgroup sandbox is not available" >&2
  exit 1
}
echo "$cgroup_status" | grep -q '"attached":true' || {
  echo "$cgroup_status"
  echo "[os-smoke] cgroup sandbox is not fully attached" >&2
  exit 1
}
CGROUP_SANDBOX_PATH="$(printf '%s' "$cgroup_status" | json_field cgroupPath)"
if [[ -z "$CGROUP_SANDBOX_PATH" || "$CGROUP_SANDBOX_PATH" == "None" ]]; then
  CGROUP_SANDBOX_PATH="${AGENT_CGROUP_SANDBOX_PATH:-/sys/fs/cgroup}"
fi

echo "[os-smoke] checking BPF LSM status"
lsm_status="$(curl_json GET /sandbox/lsm/status)"
echo "$lsm_status" | grep -q '"available":true' || {
  echo "$lsm_status"
  echo "[os-smoke] BPF LSM enforcer is not available" >&2
  exit 1
}
echo "$lsm_status" | grep -q '"attached":true' || {
  echo "$lsm_status"
  echo "[os-smoke] BPF LSM enforcer is not fully attached" >&2
  exit 1
}

tmp_exec="$(make_temp_executable)"

echo "[os-smoke] BPF LSM exec block: $tmp_exec"
curl_json POST /sandbox/lsm/block-exec-path "{\"path\":\"$tmp_exec\"}" >/dev/null
blocked_exec_paths+=("$tmp_exec")
if "$tmp_exec" 2>/dev/null; then
  echo "[os-smoke] expected exec to be blocked, but it ran" >&2
  curl_json POST /sandbox/lsm/unblock-exec-path "{\"path\":\"$tmp_exec\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-exec-path "{\"path\":\"$tmp_exec\"}"
"$tmp_exec"

tmp_exec_name="$(basename "$tmp_exec")"
echo "[os-smoke] BPF LSM exec basename block: $tmp_exec_name"
curl_json POST /sandbox/lsm/block-exec-name "{\"name\":\"$tmp_exec_name\"}" >/dev/null
blocked_exec_names+=("$tmp_exec_name")
if "$tmp_exec" 2>/dev/null; then
  echo "[os-smoke] expected exec basename to be blocked, but it ran" >&2
  curl_json POST /sandbox/lsm/unblock-exec-name "{\"name\":\"$tmp_exec_name\"}" >/dev/null || true
  exit 1
fi
tmp_exec_alias="$(mktemp -u /tmp/agent-ebpf-lsm-exec-alias.XXXXXX)"
ln -s "$tmp_exec" "$tmp_exec_alias"
echo "[os-smoke] BPF LSM exec basename symlink-alias block: $tmp_exec_alias -> $tmp_exec_name"
if "$tmp_exec_alias" 2>/dev/null; then
  echo "[os-smoke] expected exec basename to block symlink alias execution, but alias ran" >&2
  rm -f "$tmp_exec_alias"
  tmp_exec_alias=""
  curl_json POST /sandbox/lsm/unblock-exec-name "{\"name\":\"$tmp_exec_name\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-exec-name "{\"name\":\"$tmp_exec_name\"}"
"$tmp_exec"
"$tmp_exec_alias"
rm -f "$tmp_exec_alias"
tmp_exec_alias=""

tmp_file="$(mktemp /tmp/agent-ebpf-lsm-file.XXXXXX)"
basename_file="$(basename "$tmp_file")"
echo "secret" >"$tmp_file"
fd_ready_file="$(mktemp /tmp/agent-ebpf-lsm-fd-ready.XXXXXX)"
fd_trigger_file="$(mktemp /tmp/agent-ebpf-lsm-fd-trigger.XXXXXX)"
fd_result_file="$(mktemp /tmp/agent-ebpf-lsm-fd-result.XXXXXX)"
rm -f "$fd_ready_file" "$fd_trigger_file" "$fd_result_file"
start_existing_fd_probe "$tmp_file" "$fd_ready_file" "$fd_trigger_file" "$fd_result_file"
fd_probe_pid=$!
if [[ "$(wait_for_probe_result "$fd_ready_file" "BPF LSM existing-fd ready")" != "ready" ]]; then
  echo "[os-smoke] existing file descriptor probe did not become ready" >&2
  kill "$fd_probe_pid" 2>/dev/null || true
  exit 1
fi
setattr_ready_file="$(mktemp /tmp/agent-ebpf-lsm-setattr-ready.XXXXXX)"
setattr_trigger_file="$(mktemp /tmp/agent-ebpf-lsm-setattr-trigger.XXXXXX)"
setattr_result_file="$(mktemp /tmp/agent-ebpf-lsm-setattr-result.XXXXXX)"
rm -f "$setattr_ready_file" "$setattr_trigger_file" "$setattr_result_file"
start_existing_fd_setattr_probe "$tmp_file" "$setattr_ready_file" "$setattr_trigger_file" "$setattr_result_file"
setattr_probe_pid=$!
if [[ "$(wait_for_probe_result "$setattr_ready_file" "BPF LSM existing-fd setattr ready")" != "ready" ]]; then
  echo "[os-smoke] existing file descriptor setattr probe did not become ready" >&2
  kill "$setattr_probe_pid" 2>/dev/null || true
  exit 1
fi
fchmod_ready_file="$(mktemp /tmp/agent-ebpf-lsm-fchmod-ready.XXXXXX)"
fchmod_trigger_file="$(mktemp /tmp/agent-ebpf-lsm-fchmod-trigger.XXXXXX)"
fchmod_result_file="$(mktemp /tmp/agent-ebpf-lsm-fchmod-result.XXXXXX)"
rm -f "$fchmod_ready_file" "$fchmod_trigger_file" "$fchmod_result_file"
start_existing_fd_fchmod_probe "$tmp_file" "$fchmod_ready_file" "$fchmod_trigger_file" "$fchmod_result_file"
fchmod_probe_pid=$!
if [[ "$(wait_for_probe_result "$fchmod_ready_file" "BPF LSM existing-fd fchmod ready")" != "ready" ]]; then
  echo "[os-smoke] existing file descriptor fchmod probe did not become ready" >&2
  kill "$fchmod_probe_pid" 2>/dev/null || true
  exit 1
fi
mmap_ready_file="$(mktemp /tmp/agent-ebpf-lsm-mmap-ready.XXXXXX)"
mmap_trigger_file="$(mktemp /tmp/agent-ebpf-lsm-mmap-trigger.XXXXXX)"
mmap_result_file="$(mktemp /tmp/agent-ebpf-lsm-mmap-result.XXXXXX)"
rm -f "$mmap_ready_file" "$mmap_trigger_file" "$mmap_result_file"
start_existing_fd_mmap_probe "$tmp_file" "$mmap_ready_file" "$mmap_trigger_file" "$mmap_result_file"
mmap_probe_pid=$!
if [[ "$(wait_for_probe_result "$mmap_ready_file" "BPF LSM existing-fd mmap ready")" != "ready" ]]; then
  echo "[os-smoke] existing file descriptor mmap probe did not become ready" >&2
  kill "$mmap_probe_pid" 2>/dev/null || true
  exit 1
fi
mprotect_ready_file="$(mktemp /tmp/agent-ebpf-lsm-mprotect-ready.XXXXXX)"
mprotect_trigger_file="$(mktemp /tmp/agent-ebpf-lsm-mprotect-trigger.XXXXXX)"
mprotect_result_file="$(mktemp /tmp/agent-ebpf-lsm-mprotect-result.XXXXXX)"
rm -f "$mprotect_ready_file" "$mprotect_trigger_file" "$mprotect_result_file"
start_existing_map_mprotect_probe "$tmp_file" "$mprotect_ready_file" "$mprotect_trigger_file" "$mprotect_result_file"
mprotect_probe_pid=$!
if [[ "$(wait_for_probe_result "$mprotect_ready_file" "BPF LSM existing-map mprotect ready")" != "ready" ]]; then
  echo "[os-smoke] existing file mapping mprotect probe did not become ready" >&2
  kill "$mprotect_probe_pid" 2>/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM file_open block: $basename_file"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_file\"}" >/dev/null
blocked_file_names+=("$basename_file")
if cat "$tmp_file" >/dev/null 2>&1; then
  echo "[os-smoke] expected file open to be blocked, but cat succeeded" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM file_open write block: $basename_file"
if printf 'blocked-write\n' >>"$tmp_file" 2>/dev/null; then
  echo "[os-smoke] expected file write open to be blocked, but append succeeded" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM file_permission existing-fd block: $basename_file"
: >"$fd_trigger_file"
fd_result="$(wait_for_probe_result "$fd_result_file" "BPF LSM existing-fd block")"
wait "$fd_probe_pid" || true
rm -f "$fd_ready_file" "$fd_trigger_file" "$fd_result_file"
fd_probe_pid=""
fd_ready_file=""
fd_trigger_file=""
fd_result_file=""
if [[ "$fd_result" != "blocked" ]]; then
  echo "[os-smoke] expected existing file descriptor I/O to be blocked, got $fd_result" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM mmap_file existing-fd block: $basename_file"
: >"$mmap_trigger_file"
mmap_result="$(wait_for_probe_result "$mmap_result_file" "BPF LSM existing-fd mmap block")"
wait "$mmap_probe_pid" || true
rm -f "$mmap_ready_file" "$mmap_trigger_file" "$mmap_result_file"
mmap_probe_pid=""
mmap_ready_file=""
mmap_trigger_file=""
mmap_result_file=""
if [[ "$mmap_result" != "blocked" ]]; then
  echo "[os-smoke] expected existing file descriptor mmap to be blocked, got $mmap_result" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM file_mprotect existing-map block: $basename_file"
: >"$mprotect_trigger_file"
mprotect_result="$(wait_for_probe_result "$mprotect_result_file" "BPF LSM existing-map mprotect block")"
wait "$mprotect_probe_pid" || true
rm -f "$mprotect_ready_file" "$mprotect_trigger_file" "$mprotect_result_file"
mprotect_probe_pid=""
mprotect_ready_file=""
mprotect_trigger_file=""
mprotect_result_file=""
if [[ "$mprotect_result" != "blocked" ]]; then
  echo "[os-smoke] expected existing file mapping mprotect to be blocked, got $mprotect_result" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM inode_setattr existing-fd ftruncate block: $basename_file"
: >"$setattr_trigger_file"
setattr_result="$(wait_for_probe_result "$setattr_result_file" "BPF LSM existing-fd setattr block")"
wait "$setattr_probe_pid" || true
rm -f "$setattr_ready_file" "$setattr_trigger_file" "$setattr_result_file"
setattr_probe_pid=""
setattr_ready_file=""
setattr_trigger_file=""
setattr_result_file=""
if [[ "$setattr_result" != "blocked" ]]; then
  echo "[os-smoke] expected existing file descriptor ftruncate to be blocked, got $setattr_result" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM inode_setattr existing-fd fchmod block: $basename_file"
: >"$fchmod_trigger_file"
fchmod_result="$(wait_for_probe_result "$fchmod_result_file" "BPF LSM existing-fd fchmod block")"
wait "$fchmod_probe_pid" || true
rm -f "$fchmod_ready_file" "$fchmod_trigger_file" "$fchmod_result_file"
fchmod_probe_pid=""
fchmod_ready_file=""
fchmod_trigger_file=""
fchmod_result_file=""
if [[ "$fchmod_result" != "blocked" ]]; then
  echo "[os-smoke] expected existing file descriptor fchmod to be blocked, got $fchmod_result" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM inode_setattr block: $basename_file"
if chmod 600 "$tmp_file" 2>/dev/null; then
  echo "[os-smoke] expected inode setattr to be blocked, but chmod succeeded" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
echo "[os-smoke] BPF LSM inode_unlink block: $basename_file"
if rm -f "$tmp_file" 2>/dev/null; then
  echo "[os-smoke] expected file unlink to be blocked, but rm succeeded" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
tmp_rename_target="$tmp_file.renamed"
echo "[os-smoke] BPF LSM inode_rename block: $basename_file"
if mv "$tmp_file" "$tmp_rename_target" 2>/dev/null; then
  echo "[os-smoke] expected file rename to be blocked, but mv succeeded" >&2
  mv "$tmp_rename_target" "$tmp_file" 2>/dev/null || true
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_file\"}"
cat "$tmp_file" >/dev/null
printf 'allowed-write\n' >>"$tmp_file"
chmod 600 "$tmp_file"
mv "$tmp_file" "$tmp_rename_target"
mv "$tmp_rename_target" "$tmp_file"
tmp_rename_target="$(mktemp -u /tmp/agent-ebpf-lsm-rename-dst.XXXXXX)"
basename_rename_target="$(basename "$tmp_rename_target")"
echo "[os-smoke] BPF LSM inode_rename destination block: $basename_rename_target"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_rename_target\"}" >/dev/null
blocked_file_names+=("$basename_rename_target")
if mv "$tmp_file" "$tmp_rename_target" 2>/dev/null; then
  echo "[os-smoke] expected rename into blocked destination basename to be blocked, but mv succeeded" >&2
  mv "$tmp_rename_target" "$tmp_file" 2>/dev/null || true
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_rename_target\"}" >/dev/null || true
  exit 1
fi
if [[ -e "$tmp_rename_target" ]]; then
  echo "[os-smoke] expected inode_rename destination block to prevent a lingering target, but $tmp_rename_target exists" >&2
  rm -f "$tmp_rename_target"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_rename_target\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_rename_target\"}"
mv "$tmp_file" "$tmp_rename_target"
mv "$tmp_rename_target" "$tmp_file"
rm -f "$tmp_file"
tmp_file=""

tmp_create="$(mktemp -u /tmp/agent-ebpf-lsm-create.XXXXXX)"
basename_create="$(basename "$tmp_create")"
echo "[os-smoke] BPF LSM inode_create block: $basename_create"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_create\"}" >/dev/null
blocked_file_names+=("$basename_create")
if : >"$tmp_create" 2>/dev/null; then
  echo "[os-smoke] expected file create to be blocked, but shell create succeeded" >&2
  rm -f "$tmp_create"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_create\"}" >/dev/null || true
  exit 1
fi
if [[ -e "$tmp_create" ]]; then
  echo "[os-smoke] expected inode_create to prevent a lingering file, but $tmp_create exists" >&2
  rm -f "$tmp_create"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_create\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_create\"}"
: >"$tmp_create"
rm -f "$tmp_create"

tmp_link_src="$(mktemp /tmp/agent-ebpf-lsm-link-src.XXXXXX)"
tmp_link_dst="$(mktemp -u /tmp/agent-ebpf-lsm-link-dst.XXXXXX)"
basename_link_dst="$(basename "$tmp_link_dst")"
echo "[os-smoke] BPF LSM inode_link block: $basename_link_dst"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_link_dst\"}" >/dev/null
blocked_file_names+=("$basename_link_dst")
if ln "$tmp_link_src" "$tmp_link_dst" 2>/dev/null; then
  echo "[os-smoke] expected hard link creation to be blocked, but ln succeeded" >&2
  rm -f "$tmp_link_src" "$tmp_link_dst"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_link_dst\"}" >/dev/null || true
  exit 1
fi
if [[ -e "$tmp_link_dst" ]]; then
  echo "[os-smoke] expected inode_link to prevent a lingering link, but $tmp_link_dst exists" >&2
  rm -f "$tmp_link_src" "$tmp_link_dst"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_link_dst\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_link_dst\"}"
ln "$tmp_link_src" "$tmp_link_dst"
rm -f "$tmp_link_src" "$tmp_link_dst"

tmp_link_src="$(mktemp /tmp/agent-ebpf-lsm-link-src-blocked.XXXXXX)"
tmp_link_dst="$(mktemp -u /tmp/agent-ebpf-lsm-link-alias.XXXXXX)"
basename_link_src="$(basename "$tmp_link_src")"
echo "[os-smoke] BPF LSM inode_link source block: $basename_link_src"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_link_src\"}" >/dev/null
blocked_file_names+=("$basename_link_src")
if ln "$tmp_link_src" "$tmp_link_dst" 2>/dev/null; then
  echo "[os-smoke] expected hard link from blocked source basename to be blocked, but ln succeeded" >&2
  rm -f "$tmp_link_src" "$tmp_link_dst"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_link_src\"}" >/dev/null || true
  exit 1
fi
if [[ -e "$tmp_link_dst" ]]; then
  echo "[os-smoke] expected inode_link source block to prevent a lingering alias, but $tmp_link_dst exists" >&2
  rm -f "$tmp_link_src" "$tmp_link_dst"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_link_src\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_link_src\"}"
ln "$tmp_link_src" "$tmp_link_dst"
rm -f "$tmp_link_src" "$tmp_link_dst"

tmp_symlink_src="$(mktemp /tmp/agent-ebpf-lsm-symlink-src.XXXXXX)"
tmp_symlink_dst="$(mktemp -u /tmp/agent-ebpf-lsm-symlink-dst.XXXXXX)"
basename_symlink_dst="$(basename "$tmp_symlink_dst")"
echo "[os-smoke] BPF LSM inode_symlink block: $basename_symlink_dst"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_symlink_dst\"}" >/dev/null
blocked_file_names+=("$basename_symlink_dst")
if ln -s "$tmp_symlink_src" "$tmp_symlink_dst" 2>/dev/null; then
  echo "[os-smoke] expected symlink creation to be blocked, but ln -s succeeded" >&2
  rm -f "$tmp_symlink_src" "$tmp_symlink_dst"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_symlink_dst\"}" >/dev/null || true
  exit 1
fi
if [[ -e "$tmp_symlink_dst" || -L "$tmp_symlink_dst" ]]; then
  echo "[os-smoke] expected inode_symlink to prevent a lingering symlink, but $tmp_symlink_dst exists" >&2
  rm -f "$tmp_symlink_src" "$tmp_symlink_dst"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_symlink_dst\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_symlink_dst\"}"
ln -s "$tmp_symlink_src" "$tmp_symlink_dst"
rm -f "$tmp_symlink_src" "$tmp_symlink_dst"

tmp_mkdir="$(mktemp -u /tmp/agent-ebpf-lsm-mkdir.XXXXXX)"
basename_mkdir="$(basename "$tmp_mkdir")"
echo "[os-smoke] BPF LSM inode_mkdir block: $basename_mkdir"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_mkdir\"}" >/dev/null
blocked_file_names+=("$basename_mkdir")
if mkdir "$tmp_mkdir" 2>/dev/null; then
  echo "[os-smoke] expected mkdir to be blocked, but mkdir succeeded" >&2
  rmdir "$tmp_mkdir" 2>/dev/null || true
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_mkdir\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_mkdir\"}"
mkdir "$tmp_mkdir"
rmdir "$tmp_mkdir"

tmp_fifo="$(mktemp -u /tmp/agent-ebpf-lsm-fifo.XXXXXX)"
basename_fifo="$(basename "$tmp_fifo")"
echo "[os-smoke] BPF LSM inode_mknod block: $basename_fifo"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_fifo\"}" >/dev/null
blocked_file_names+=("$basename_fifo")
if mkfifo "$tmp_fifo" 2>/dev/null; then
  echo "[os-smoke] expected FIFO mknod to be blocked, but mkfifo succeeded" >&2
  rm -f "$tmp_fifo"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_fifo\"}" >/dev/null || true
  exit 1
fi
if [[ -e "$tmp_fifo" ]]; then
  echo "[os-smoke] expected inode_mknod to prevent a lingering FIFO, but $tmp_fifo exists" >&2
  rm -f "$tmp_fifo"
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_fifo\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_fifo\"}"
mkfifo "$tmp_fifo"
rm -f "$tmp_fifo"

tmp_dir="$(mktemp -d /tmp/agent-ebpf-lsm-dir.XXXXXX)"
basename_dir="$(basename "$tmp_dir")"
echo "[os-smoke] BPF LSM inode_rmdir block: $basename_dir"
curl_json POST /sandbox/lsm/block-file-name "{\"name\":\"$basename_dir\"}" >/dev/null
blocked_file_names+=("$basename_dir")
if rmdir "$tmp_dir" 2>/dev/null; then
  echo "[os-smoke] expected directory removal to be blocked, but rmdir succeeded" >&2
  curl_json POST /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_dir\"}" >/dev/null || true
  exit 1
fi
assert_idempotent_unblock /sandbox/lsm/unblock-file-name "{\"name\":\"$basename_dir\"}"
rmdir "$tmp_dir"
tmp_dir=""

run_port_block_smoke 127.0.0.1 IPv4
run_udp_port_block_smoke 127.0.0.1 IPv4-UDP
run_udp_sendto_port_block_smoke 127.0.0.1 IPv4-UDP-sendto
run_udp_existing_connected_send_port_block_smoke 127.0.0.1 IPv4-UDP-existing-connected
run_cgroup_pid_block_smoke
if loopback_ipv4_available 127.0.0.2; then
  run_ip_block_smoke 127.0.0.2 IPv4-loopback-alias
  run_udp_ip_block_smoke 127.0.0.2 IPv4-UDP-loopback-alias
  run_udp_sendto_ip_block_smoke 127.0.0.2 IPv4-UDP-sendto-loopback-alias
  run_udp_existing_connected_send_ip_block_smoke 127.0.0.2 IPv4-UDP-existing-connected-loopback-alias
else
  echo "[os-smoke] 127.0.0.2 loopback alias unavailable; skipped IPv4 destination block smoke"
fi
if ipv4_mapped_loopback_available; then
  run_ipv4_mapped_ip_block_smoke
  run_udp_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-loopback
  run_udp_sendto_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-sendto-loopback
  run_udp_existing_connected_send_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-existing-connected-loopback
else
  echo "[os-smoke] IPv4-mapped IPv6 loopback unavailable; skipped mapped IPv6 IP block smoke"
fi
if ipv6_loopback_available; then
  run_ip_block_smoke ::1 IPv6-loopback
  run_port_block_smoke ::1 IPv6
  run_udp_ip_block_smoke ::1 IPv6-UDP-loopback
  run_udp_port_block_smoke ::1 IPv6-UDP
  run_udp_sendto_ip_block_smoke ::1 IPv6-UDP-sendto-loopback
  run_udp_sendto_port_block_smoke ::1 IPv6-UDP-sendto
  run_udp_existing_connected_send_ip_block_smoke ::1 IPv6-UDP-existing-connected-loopback
  run_udp_existing_connected_send_port_block_smoke ::1 IPv6-UDP-existing-connected
else
  echo "[os-smoke] IPv6 loopback unavailable; skipped connect6 destination/port smoke"
fi

echo "[os-smoke] OS-level enforcement smoke test passed"
