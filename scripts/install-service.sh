#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ORIGINAL_ARGS=("$@")

ACTION="install"
DRY_RUN=0
METHOD="${INSTALL_METHOD:-auto}"
PREFIX="${INSTALL_PREFIX:-/opt/agent-ebpf-filter}"
BINDIR="${INSTALL_BINDIR:-/usr/local/bin}"
SYSCONFDIR="${INSTALL_SYSCONFDIR:-/etc/agent-ebpf-filter}"
SERVICE_NAME="${INSTALL_SERVICE_NAME:-agent-ebpf-filter}"
ENABLE="${INSTALL_ENABLE:-1}"
START="${INSTALL_START:-1}"
RC_LOCAL="${INSTALL_RC_LOCAL:-/etc/rc.local}"
START_SCRIPT="${INSTALL_START_SCRIPT:-/usr/local/sbin/agent-ebpf-filter-service}"
ENV_FILE="$SYSCONFDIR/$SERVICE_NAME.env"

log() { printf '[install-service] %s\n' "$*"; }
warn() { printf '[install-service] WARN: %s\n' "$*" >&2; }
die() { printf '[install-service] ERROR: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<EOF
Usage: $0 [install|uninstall] [--method auto|systemd|rc.local] [--dry-run] [--no-enable] [--no-start]

Environment overrides:
  INSTALL_PREFIX        install tree for backend + frontend (default: /opt/agent-ebpf-filter)
  INSTALL_BINDIR        public binary directory (default: /usr/local/bin)
  INSTALL_SYSCONFDIR    environment file directory (default: /etc/agent-ebpf-filter)
  INSTALL_SERVICE_NAME  service name (default: agent-ebpf-filter)
  INSTALL_METHOD        auto, systemd, or rc.local (default: auto)
  INSTALL_ENABLE        enable on boot, 1/0 (default: 1)
  INSTALL_START         start/restart immediately, 1/0 (default: 1)
  INSTALL_REAL_HOME     runtime config home for the root service
EOF
}

while (($#)); do
  case "$1" in
    install|uninstall)
      ACTION="$1"
      shift
      ;;
    --method)
      [[ $# -ge 2 ]] || die "--method needs a value"
      METHOD="$2"
      shift 2
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --no-enable)
      ENABLE=0
      shift
      ;;
    --no-start)
      START=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

trim_path() {
  local value="$1"
  while [[ "$value" != "/" && "$value" == */ ]]; do
    value="${value%/}"
  done
  printf '%s' "$value"
}

PREFIX="$(trim_path "$PREFIX")"
BINDIR="$(trim_path "$BINDIR")"
SYSCONFDIR="$(trim_path "$SYSCONFDIR")"
ENV_FILE="$SYSCONFDIR/$SERVICE_NAME.env"

bool_enabled() {
  case "${1,,}" in
    1|yes|true|on|enable|enabled) return 0 ;;
    *) return 1 ;;
  esac
}

print_cmd() {
  printf '+'
  printf ' %q' "$@"
  printf '\n'
}

run() {
  if ((DRY_RUN)); then
    print_cmd "$@"
  else
    "$@"
  fi
}

try_run() {
  if ((DRY_RUN)); then
    print_cmd "$@"
  else
    "$@" || true
  fi
}

quote_env_value() {
  local escaped
  escaped="$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"
  printf "'%s'" "$escaped"
}

lookup_home_for_user() {
  local name="$1"
  [[ -n "$name" && "$name" != "root" ]] || return 1
  if command -v getent >/dev/null 2>&1; then
    getent passwd "$name" | awk -F: '{print $6; exit}'
    return 0
  fi
  eval "printf '%s' ~$name" 2>/dev/null
}

lookup_home_for_uid() {
  local uid="$1"
  [[ -n "$uid" && "$uid" != "0" ]] || return 1
  if command -v getent >/dev/null 2>&1; then
    getent passwd "$uid" | awk -F: '{print $6; exit}'
    return 0
  fi
  return 1
}

detect_real_home() {
  if [[ -n "${INSTALL_REAL_HOME:-}" ]]; then
    printf '%s' "$INSTALL_REAL_HOME"
    return 0
  fi

  local candidate=""
  if [[ -n "${SUDO_USER:-}" ]]; then
    candidate="$(lookup_home_for_user "$SUDO_USER" || true)"
    if [[ -n "$candidate" ]]; then printf '%s' "$candidate"; return 0; fi
  fi
  if [[ -n "${PKEXEC_UID:-}" ]]; then
    candidate="$(lookup_home_for_uid "$PKEXEC_UID" || true)"
    if [[ -n "$candidate" ]]; then printf '%s' "$candidate"; return 0; fi
  fi
  if [[ -n "${HOME:-}" && "$HOME" != "/root" ]]; then
    printf '%s' "$HOME"
    return 0
  fi
  if command -v logname >/dev/null 2>&1; then
    local login_user
    login_user="$(logname 2>/dev/null || true)"
    candidate="$(lookup_home_for_user "$login_user" || true)"
    if [[ -n "$candidate" ]]; then printf '%s' "$candidate"; return 0; fi
  fi
  printf '/root'
}

REAL_HOME="$(detect_real_home)"

validate_common() {
  [[ "$ACTION" == "install" || "$ACTION" == "uninstall" ]] || die "ACTION must be install or uninstall"
  [[ "$METHOD" == "auto" || "$METHOD" == "systemd" || "$METHOD" == "rc.local" ]] || die "INSTALL_METHOD must be auto, systemd, or rc.local"
  [[ -n "$PREFIX" && "$PREFIX" == /* ]] || die "INSTALL_PREFIX must be an absolute path"
  [[ -n "$BINDIR" && "$BINDIR" == /* ]] || die "INSTALL_BINDIR must be an absolute path"
  [[ -n "$SYSCONFDIR" && "$SYSCONFDIR" == /* ]] || die "INSTALL_SYSCONFDIR must be an absolute path"
  [[ -n "$SERVICE_NAME" && "$SERVICE_NAME" != */* ]] || die "INSTALL_SERVICE_NAME must be a non-empty service name, not a path"
}

ensure_root() {
  if ((DRY_RUN)); then
    return 0
  fi
  if [[ "${EUID:-$(id -u)}" -eq 0 ]]; then
    return 0
  fi
  command -v sudo >/dev/null 2>&1 || die "need root privileges; install sudo or run this script as root"
  log "Re-running through sudo for system installation..."
  exec sudo env \
    "INSTALL_PREFIX=$PREFIX" \
    "INSTALL_BINDIR=$BINDIR" \
    "INSTALL_SYSCONFDIR=$SYSCONFDIR" \
    "INSTALL_SERVICE_NAME=$SERVICE_NAME" \
    "INSTALL_METHOD=$METHOD" \
    "INSTALL_ENABLE=$ENABLE" \
    "INSTALL_START=$START" \
    "INSTALL_RC_LOCAL=$RC_LOCAL" \
    "INSTALL_START_SCRIPT=$START_SCRIPT" \
    "INSTALL_REAL_HOME=$REAL_HOME" \
    "$0" "${ORIGINAL_ARGS[@]}"
}

systemd_running() {
  command -v systemctl >/dev/null 2>&1 && [[ -d /etc/systemd/system && -d /run/systemd/system ]]
}

resolve_method() {
  case "$METHOD" in
    systemd)
      command -v systemctl >/dev/null 2>&1 || die "systemctl not found; use INSTALL_METHOD=rc.local"
      [[ -d /etc/systemd/system ]] || die "/etc/systemd/system not found; use INSTALL_METHOD=rc.local"
      printf 'systemd'
      ;;
    rc.local)
      printf 'rc.local'
      ;;
    auto)
      if systemd_running; then
        printf 'systemd'
      else
        printf 'rc.local'
      fi
      ;;
  esac
}

require_file() {
  [[ -f "$1" ]] || die "missing $1; run make build first"
}

require_dir() {
  [[ -d "$1" ]] || die "missing $1; run make frontend/build first"
}

require_install_artifacts() {
  require_file "$ROOT_DIR/backend/agent-ebpf-filter"
  require_file "$ROOT_DIR/agent-wrapper"
  require_dir "$ROOT_DIR/frontend/dist"
}

install_payload() {
  log "Installing backend/frontend to $PREFIX and binaries to $BINDIR"
  run mkdir -p "$PREFIX/backend" "$PREFIX/frontend" "$BINDIR"
  run install -m 0755 "$ROOT_DIR/backend/agent-ebpf-filter" "$PREFIX/backend/agent-ebpf-filter"
  run install -m 0755 "$ROOT_DIR/agent-wrapper" "$PREFIX/agent-wrapper"
  run install -m 0644 "$ROOT_DIR/README.md" "$PREFIX/README.md"
  run install -m 0644 "$ROOT_DIR/AGENTS.md" "$PREFIX/AGENTS.md"
  run install -m 0755 "$ROOT_DIR/backend/agent-ebpf-filter" "$BINDIR/agent-ebpf-filter"
  run install -m 0755 "$ROOT_DIR/agent-wrapper" "$BINDIR/agent-wrapper"
  run rm -rf "$PREFIX/frontend/dist"
  run mkdir -p "$PREFIX/frontend"
  run cp -a "$ROOT_DIR/frontend/dist" "$PREFIX/frontend/dist"
}

write_env_file() {
  local tmp
  tmp="$(mktemp)"
  {
    printf '# Managed by agent-ebpf-filter make install. Edit and restart %s after changes.\n' "$SERVICE_NAME"
    printf 'AGENT_REAL_HOME=%s\n' "$(quote_env_value "$REAL_HOME")"
    printf 'AGENT_WRAPPER_PATH=%s\n' "$(quote_env_value "$BINDIR/agent-wrapper")"
    printf 'GIN_MODE=release\n'
    printf '# Optional overrides:\n'
    printf '# AGENT_BACKEND_PORT=8080\n'
    printf '# AGENT_CGROUP_SANDBOX_PATH=/sys/fs/cgroup\n'
    printf '# DISABLE_AUTH=true\n'
  } >"$tmp"
  log "Writing environment file $ENV_FILE (AGENT_REAL_HOME=$REAL_HOME)"
  run mkdir -p "$SYSCONFDIR"
  run install -m 0644 "$tmp" "$ENV_FILE"
  rm -f "$tmp"
}

write_systemd_unit() {
  local unit="/etc/systemd/system/$SERVICE_NAME.service"
  local tmp
  tmp="$(mktemp)"
  cat >"$tmp" <<EOF
[Unit]
Description=Agent eBPF Filter service
Documentation=file://$PREFIX/README.md
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=$PREFIX
EnvironmentFile=-$ENV_FILE
Environment=AGENT_WRAPPER_PATH=$BINDIR/agent-wrapper
Environment=GIN_MODE=release
ExecStartPre=/bin/mkdir -p /sys/fs/bpf
ExecStartPre=/bin/sh -c 'grep -qs " /sys/fs/bpf bpf " /proc/mounts || mount -t bpf bpf /sys/fs/bpf'
ExecStart=$PREFIX/backend/agent-ebpf-filter
Restart=on-failure
RestartSec=3s
LimitMEMLOCK=infinity
NoNewPrivileges=false
KillMode=mixed

[Install]
WantedBy=multi-user.target
EOF
  log "Installing systemd unit $unit"
  run install -m 0644 "$tmp" "$unit"
  rm -f "$tmp"
}

install_systemd_service() {
  write_systemd_unit
  if systemd_running; then
    run systemctl daemon-reload
    if bool_enabled "$ENABLE"; then
      run systemctl enable "$SERVICE_NAME.service"
    fi
    if bool_enabled "$START"; then
      run systemctl restart "$SERVICE_NAME.service"
    fi
    log "Installed systemd service: $SERVICE_NAME.service"
  else
    warn "systemd is not running in this environment; unit file was installed but systemctl enable/start was skipped"
  fi
}

write_rc_start_script() {
  local tmp
  tmp="$(mktemp)"
  cat >"$tmp" <<EOF
#!/bin/sh
set -eu

NAME=$(quote_env_value "$SERVICE_NAME")
PREFIX=$(quote_env_value "$PREFIX")
BIN="\$PREFIX/backend/agent-ebpf-filter"
ENV_FILE=$(quote_env_value "$ENV_FILE")
PIDFILE="/run/\$NAME.pid"
LOGFILE="/var/log/\$NAME.log"

is_running() {
  [ -s "\$PIDFILE" ] || return 1
  pid="\$(cat "\$PIDFILE" 2>/dev/null || true)"
  [ -n "\$pid" ] || return 1
  kill -0 "\$pid" 2>/dev/null
}

start_service() {
  if is_running; then
    echo "\$NAME already running with pid \$(cat "\$PIDFILE")"
    return 0
  fi
  mkdir -p /run
  if [ -r "\$ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "\$ENV_FILE"
    set +a
  fi
  export AGENT_REAL_HOME="\${AGENT_REAL_HOME:-$REAL_HOME}"
  export AGENT_WRAPPER_PATH="\${AGENT_WRAPPER_PATH:-$BINDIR/agent-wrapper}"
  export GIN_MODE="\${GIN_MODE:-release}"
  cd "\$PREFIX"
  nohup "\$BIN" >>"\$LOGFILE" 2>&1 &
  echo "\$!" >"\$PIDFILE"
  echo "started \$NAME with pid \$(cat "\$PIDFILE")"
}

stop_service() {
  if ! is_running; then
    rm -f "\$PIDFILE"
    echo "\$NAME is not running"
    return 0
  fi
  pid="\$(cat "\$PIDFILE")"
  kill "\$pid" 2>/dev/null || true
  i=0
  while kill -0 "\$pid" 2>/dev/null && [ "\$i" -lt 30 ]; do
    i=\$((i + 1))
    sleep 1
  done
  if kill -0 "\$pid" 2>/dev/null; then
    kill -9 "\$pid" 2>/dev/null || true
  fi
  rm -f "\$PIDFILE"
}

case "\${1:-start}" in
  start) start_service ;;
  stop) stop_service ;;
  restart) stop_service; start_service ;;
  status) is_running && echo "\$NAME running with pid \$(cat "\$PIDFILE")" || { echo "\$NAME is not running"; exit 3; } ;;
  *) echo "Usage: \$0 {start|stop|restart|status}" >&2; exit 2 ;;
esac
EOF
  log "Installing rc.local helper $START_SCRIPT"
  run install -m 0755 "$tmp" "$START_SCRIPT"
  rm -f "$tmp"
}

rc_block() {
  cat <<EOF
# >>> agent-ebpf-filter service >>>
$START_SCRIPT start
# <<< agent-ebpf-filter service <<<
EOF
}

update_rc_local_block() {
  local block tmp stripped
  block="$(rc_block)"
  if ((DRY_RUN)); then
    log "Would update $RC_LOCAL with:"
    printf '%s\n' "$block"
    return 0
  fi

  tmp="$(mktemp)"
  stripped="$(mktemp)"
  mkdir -p "$(dirname "$RC_LOCAL")"
  if [[ -f "$RC_LOCAL" ]]; then
    awk '
      $0 == "# >>> agent-ebpf-filter service >>>" {skip=1; next}
      $0 == "# <<< agent-ebpf-filter service <<<" {skip=0; next}
      !skip {print}
    ' "$RC_LOCAL" >"$stripped"
  else
    printf '#!/bin/sh\n' >"$stripped"
  fi

  awk -v block="$block" '
    BEGIN { inserted = 0 }
    !inserted && $0 ~ /^[[:space:]]*exit[[:space:]]+0[[:space:]]*$/ {
      print block
      inserted = 1
    }
    { print }
    END {
      if (!inserted) {
        print block
      }
    }
  ' "$stripped" >"$tmp"

  install -m 0755 "$tmp" "$RC_LOCAL"
  rm -f "$tmp" "$stripped"
}

install_rc_local_service() {
  write_rc_start_script
  if bool_enabled "$ENABLE"; then
    update_rc_local_block
    log "Installed rc.local boot entry in $RC_LOCAL"
  fi
  if bool_enabled "$START"; then
    run "$START_SCRIPT" restart
  fi
}

safe_remove_payload() {
  case "$PREFIX" in
    ""|"/"|"/usr"|"/usr/local"|"/opt"|"/etc"|"/bin"|"/sbin")
      die "refusing to remove unsafe INSTALL_PREFIX=$PREFIX"
      ;;
  esac
  log "Removing installed files"
  run rm -f "$BINDIR/agent-wrapper" "$BINDIR/agent-ebpf-filter"
  run rm -rf "$PREFIX"
  run rm -f "$ENV_FILE"
  try_run rmdir "$SYSCONFDIR"
}

uninstall_systemd_service() {
  local unit="/etc/systemd/system/$SERVICE_NAME.service"
  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
    try_run systemctl stop "$SERVICE_NAME.service"
    try_run systemctl disable "$SERVICE_NAME.service"
  fi
  run rm -f "$unit"
  if command -v systemctl >/dev/null 2>&1 && [[ -d /run/systemd/system ]]; then
    try_run systemctl daemon-reload
    try_run systemctl reset-failed "$SERVICE_NAME.service"
  fi
}

remove_rc_local_block() {
  if [[ ! -f "$RC_LOCAL" ]]; then
    return 0
  fi
  if ((DRY_RUN)); then
    log "Would remove managed block from $RC_LOCAL"
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  awk '
    $0 == "# >>> agent-ebpf-filter service >>>" {skip=1; next}
    $0 == "# <<< agent-ebpf-filter service <<<" {skip=0; next}
    !skip {print}
  ' "$RC_LOCAL" >"$tmp"
  install -m 0755 "$tmp" "$RC_LOCAL"
  rm -f "$tmp"
}

uninstall_rc_local_service() {
  if [[ -x "$START_SCRIPT" ]]; then
    try_run "$START_SCRIPT" stop
  fi
  remove_rc_local_block
  run rm -f "$START_SCRIPT"
}

install_action() {
  require_install_artifacts
  local resolved
  resolved="$(resolve_method)"
  install_payload
  write_env_file
  case "$resolved" in
    systemd) install_systemd_service ;;
    rc.local) install_rc_local_service ;;
  esac
  log "Done. Dashboard should be available on the backend port (default 8080; exact port is written to $PREFIX/.port)."
}

uninstall_action() {
  case "$METHOD" in
    systemd)
      uninstall_systemd_service
      ;;
    rc.local)
      uninstall_rc_local_service
      ;;
    auto)
      uninstall_systemd_service
      uninstall_rc_local_service
      ;;
  esac
  safe_remove_payload
  log "Uninstall complete."
}

validate_common
ensure_root

case "$ACTION" in
  install) install_action ;;
  uninstall) uninstall_action ;;
esac
