#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CGROUP_SANDBOX_PATH="${AGENT_CGROUP_SANDBOX_PATH:-/sys/fs/cgroup}"
OS_SMOKE_PRIVILEGE_CMD="${OS_SMOKE_PRIVILEGE_CMD:-}"
CGROUP_SANDBOX_MAPS_DIR="/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps"
LSM_ENFORCER_MAPS_DIR="/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps"
failures=0
warnings=0
privileged_runner_label=""

ok() { printf "[preflight] OK: %s\n" "$*"; }
warn() { warnings=$((warnings + 1)); printf "[preflight] WARN: %s\n" "$*"; }
fail() { failures=$((failures + 1)); printf "[preflight] FAIL: %s\n" "$*"; }

check_file() {
  local path="$1"
  if [[ -e "$path" ]]; then
    ok "$path exists"
  else
    fail "$path is missing"
  fi
}

printf "[preflight] kernel: %s\n" "$(uname -r 2>/dev/null || echo unknown)"
printf "[preflight] user: uid=%s euid=%s\n" "$(id -u 2>/dev/null || echo unknown)" "${EUID:-unknown}"

custom_privilege_prefix=()
if [[ -n "$OS_SMOKE_PRIVILEGE_CMD" ]]; then
  # shellcheck disable=SC2206 # simple argv splitting for operator-provided command prefix.
  custom_privilege_prefix=($OS_SMOKE_PRIVILEGE_CMD)
fi

privileged_runner=0
if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
  privileged_runner=1
  privileged_runner_label="root"
  ok "running as root; sudo is not required for smoke-start"
elif (( ${#custom_privilege_prefix[@]} > 0 )); then
  custom_uid="$("${custom_privilege_prefix[@]}" id -u 2>/tmp/agent-ebpf-preflight-privilege.err || true)"
  if [[ "$custom_uid" == "0" ]]; then
    privileged_runner=1
    privileged_runner_label="OS_SMOKE_PRIVILEGE_CMD"
    ok "custom OS_SMOKE_PRIVILEGE_CMD runs commands as root"
  else
    fail "OS_SMOKE_PRIVILEGE_CMD did not run a root command (uid=${custom_uid:-unknown}): $(tr "\n" " " </tmp/agent-ebpf-preflight-privilege.err)"
  fi
elif command -v sudo >/dev/null 2>&1; then
  if sudo -n true 2>/tmp/agent-ebpf-preflight-sudo.err; then
    privileged_runner=1
    privileged_runner_label="passwordless sudo"
    ok "passwordless sudo is available"
  else
    fail "passwordless sudo is unavailable: $(tr "\n" " " </tmp/agent-ebpf-preflight-sudo.err)"
  fi
else
  fail "sudo command is unavailable; run as root or install/configure passwordless sudo"
fi

run_privileged_preflight() {
  case "$privileged_runner_label" in
    root)
      "$@"
      ;;
    OS_SMOKE_PRIVILEGE_CMD)
      "${custom_privilege_prefix[@]}" "$@"
      ;;
    "passwordless sudo")
      sudo -n "$@"
      ;;
    *)
      return 1
      ;;
  esac
}

if [[ -f /sys/fs/cgroup/cgroup.controllers ]]; then
  ok "cgroup v2 is mounted at /sys/fs/cgroup"
else
  fail "cgroup v2 root /sys/fs/cgroup/cgroup.controllers is unavailable"
fi

if [[ -d "$CGROUP_SANDBOX_PATH" ]]; then
  ok "cgroup sandbox attach path exists: $CGROUP_SANDBOX_PATH"
  if [[ -e "$CGROUP_SANDBOX_PATH/cgroup.procs" ]]; then
    ok "cgroup sandbox attach path has cgroup.procs"
  else
    fail "cgroup sandbox attach path is not a cgroup directory: missing $CGROUP_SANDBOX_PATH/cgroup.procs"
  fi
  if command -v stat >/dev/null 2>&1; then
    cgroup_fs_type="$(stat -f -c %T "$CGROUP_SANDBOX_PATH" 2>/dev/null || true)"
    if [[ "$cgroup_fs_type" == "cgroup2fs" ]]; then
      ok "cgroup sandbox attach path is on cgroup v2 filesystem"
    else
      fail "cgroup sandbox attach path is on filesystem type ${cgroup_fs_type:-unknown}, expected cgroup2fs"
    fi
  else
    warn "stat command is unavailable; cannot confirm selected cgroup attach path filesystem type"
  fi
  if (( privileged_runner )); then
    tmp_cgroup="$CGROUP_SANDBOX_PATH/agent-ebpf-preflight-$$"
    if run_privileged_preflight mkdir "$tmp_cgroup" 2>/tmp/agent-ebpf-preflight-cgroup.err; then
      ok "can create a temporary cgroup below the sandbox attach path"
      if ! run_privileged_preflight rmdir "$tmp_cgroup" 2>/dev/null; then
        warn "temporary preflight cgroup needs manual cleanup: $tmp_cgroup"
      fi
    else
      fail "cannot create a temporary cgroup below $CGROUP_SANDBOX_PATH: $(tr "\n" " " </tmp/agent-ebpf-preflight-cgroup.err)"
    fi
  fi
else
  fail "cgroup sandbox attach path does not exist: $CGROUP_SANDBOX_PATH"
fi

if mountpoint -q /sys/fs/bpf; then
  ok "bpffs is mounted at /sys/fs/bpf"
else
  fail "bpffs is not mounted at /sys/fs/bpf"
fi

if [[ -w /sys/fs/bpf ]]; then
  ok "/sys/fs/bpf is writable by this user"
elif (( privileged_runner )) && [[ "$privileged_runner_label" == "OS_SMOKE_PRIVILEGE_CMD" ]] && "${custom_privilege_prefix[@]}" test -w /sys/fs/bpf 2>/dev/null; then
  ok "/sys/fs/bpf is writable through OS_SMOKE_PRIVILEGE_CMD"
elif (( privileged_runner )) && command -v sudo >/dev/null 2>&1 && sudo -n test -w /sys/fs/bpf 2>/dev/null; then
  ok "/sys/fs/bpf is writable through passwordless sudo"
else
  fail "/sys/fs/bpf is not writable by this user; live attach needs root/passwordless sudo, OS_SMOKE_PRIVILEGE_CMD, or a privileged container"
fi

if [[ -r /sys/kernel/security/lsm ]]; then
  lsm_list="$(cat /sys/kernel/security/lsm)"
  printf "[preflight] active LSMs: %s\n" "$lsm_list"
  if [[ ",$lsm_list," == *,bpf,* ]]; then
    ok "BPF LSM is enabled"
  else
    fail "BPF LSM is not listed; enable it with kernel support and an lsm=...bpf boot configuration"
  fi
else
  warn "/sys/kernel/security/lsm is not readable; cannot confirm BPF LSM enablement"
fi

if command -v llvm-objdump >/dev/null 2>&1; then
  ok "llvm-objdump is available"
else
  fail "llvm-objdump is missing; static object checks need LLVM tools"
fi

if command -v clang >/dev/null 2>&1; then
  ok "clang is available"
else
  fail "clang is missing; bpf2go eBPF generation needs clang"
fi

if command -v go >/dev/null 2>&1; then
  ok "go is available ($(go version 2>/dev/null || true))"
else
  fail "go is missing"
fi

if command -v curl >/dev/null 2>&1; then
  ok "curl is available"
else
  fail "curl is missing; the smoke script uses it to call the backend APIs"
fi

if command -v python3 >/dev/null 2>&1; then
  ok "python3 is available"
else
  fail "python3 is missing; the smoke script uses it for loopback listeners and probes"
fi

check_file "$ROOT_DIR/backend/ebpf/agentcgroupsandbox_bpfel.o"
check_file "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o"

check_object_section() {
  local object="$1"
  local section="$2"
  if [[ ! -e "$object" ]] || ! command -v llvm-objdump >/dev/null 2>&1; then
    return 0
  fi
  if llvm-objdump -h "$object" | grep -q "$section"; then
    ok "$object includes $section"
  else
    fail "$object is missing expected eBPF section $section; run: rtk make os-enforcement-check"
  fi
}

check_object_section "$ROOT_DIR/backend/ebpf/agentcgroupsandbox_bpfel.o" "cgroup/connect4"
check_object_section "$ROOT_DIR/backend/ebpf/agentcgroupsandbox_bpfel.o" "cgroup/connect6"
check_object_section "$ROOT_DIR/backend/ebpf/agentcgroupsandbox_bpfel.o" "cgroup/sendmsg4"
check_object_section "$ROOT_DIR/backend/ebpf/agentcgroupsandbox_bpfel.o" "cgroup/sendmsg6"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/bprm_check_security"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/file_open"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/file_permission"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/mmap_file"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/file_mprotect"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_setattr"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_create"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_link"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_unlink"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_symlink"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_mkdir"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_rmdir"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_mknod"
check_object_section "$ROOT_DIR/backend/ebpf/agentlsmenforcer_bpfel.o" "lsm/inode_rename"

check_pinned_map_set() {
  local label="$1"
  local dir="$2"
  shift 2
  if [[ ! -d "$dir" ]]; then
    ok "$label pinned maps are not present yet; backend will bootstrap them"
    return 0
  fi

  local missing=()
  local name
  for name in "$@"; do
    if [[ ! -e "$dir/$name" ]]; then
      missing+=("$name")
    fi
  done
  if (( ${#missing[@]} == 0 )); then
    ok "$label pinned maps include expected entries"
    return 0
  fi

  if [[ "$label" == "cgroup sandbox" && " ${missing[*]} " == *" ip6_blocklist "* && ${#missing[@]} -eq 1 ]]; then
    warn "$label pinned maps are from an older build and miss ip6_blocklist; privileged startup will create this compatibility map"
  else
    warn "$label pinned map set is incomplete: ${missing[*]}; privileged startup may need to reset stale pins"
  fi
}

check_pinned_map_set "cgroup sandbox" "$CGROUP_SANDBOX_MAPS_DIR" \
  cgroup_blocklist ip_blocklist ip6_blocklist port_blocklist cgroup_sandbox_stats
check_pinned_map_set "BPF LSM enforcer" "$LSM_ENFORCER_MAPS_DIR" \
  lsm_blocked_exec_paths lsm_blocked_exec_names lsm_blocked_file_names lsm_enforcer_stats_map

if [[ -x "$ROOT_DIR/backend/agent-ebpf-filter" ]]; then
  ok "backend binary is executable"
else
  warn "backend binary is not built yet; run: rtk make backend"
fi

if (( failures > 0 )); then
  printf "[preflight] result: %d failure(s), %d warning(s). Live OS-enforcement smoke is not ready.\n" "$failures" "$warnings"
  printf "[preflight] next: fix the failures, then run: rtk make os-enforcement-smoke-start\n"
  exit 1
fi

printf "[preflight] result: ready with %d warning(s). Run: rtk make os-enforcement-smoke-start\n" "$warnings"
