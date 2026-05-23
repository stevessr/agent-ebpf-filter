#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_ENV_FILE="${DEV_ENV_FILE:-$ROOT/.env.dev}"
if [ -f "$DEV_ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$DEV_ENV_FILE"
    set +a
fi

SESSION_NAME="${AGENT_EBPF_DEV_SESSION:-agent-ebpf-dev}"
LAYOUT_TEMPLATE="$ROOT/layouts/dev.kdl"
SESSION_STAMP_SAFE="${SESSION_NAME//[^A-Za-z0-9_.-]/_}"
LAUNCHER_DIR="/tmp/agent-ebpf-dev-zellij-${SESSION_STAMP_SAFE}"
LAYOUT_FILE="$LAUNCHER_DIR/dev.kdl"
BACKEND_LAUNCHER="$LAUNCHER_DIR/backend.sh"
FRONTEND_LAUNCHER="$LAUNCHER_DIR/frontend.sh"

escape_kdl_string() {
    local value="$1"
    value="${value//\\/\\\\}"
    value="${value//\"/\\\"}"
    printf '%s' "$value"
}

if ! command -v zellij >/dev/null 2>&1; then
    echo "zellij is required for make dev. Please install zellij and try again."
    exit 1
fi

mkdir -p "$LAUNCHER_DIR"

cat > "$BACKEND_LAUNCHER" <<EOF
#!/bin/bash
set -euo pipefail
cd $(printf '%q' "$ROOT")
exec ./scripts/dev-pane.sh backend
EOF

cat > "$FRONTEND_LAUNCHER" <<EOF
#!/bin/bash
set -euo pipefail
cd $(printf '%q' "$ROOT")
exec ./scripts/dev-pane.sh frontend
EOF

chmod +x "$BACKEND_LAUNCHER" "$FRONTEND_LAUNCHER"

BACKEND_LAUNCHER_KDL="$(escape_kdl_string "$BACKEND_LAUNCHER")"
FRONTEND_LAUNCHER_KDL="$(escape_kdl_string "$FRONTEND_LAUNCHER")"
LAYOUT_CONTENT="$(<"$LAYOUT_TEMPLATE")"
restore_patsub_replacement=0
if shopt -q patsub_replacement 2>/dev/null; then
    restore_patsub_replacement=1
    shopt -u patsub_replacement
fi
LAYOUT_CONTENT="${LAYOUT_CONTENT//__BACKEND_LAUNCHER__/$BACKEND_LAUNCHER_KDL}"
printf '%s\n' "${LAYOUT_CONTENT//__FRONTEND_LAUNCHER__/$FRONTEND_LAUNCHER_KDL}" > "$LAYOUT_FILE"
if [ "$restore_patsub_replacement" = "1" ]; then
    shopt -s patsub_replacement
fi
session_exists() {
    zellij list-sessions -s | awk -v name="$SESSION_NAME" '$0 == name { found = 1 } END { exit found ? 0 : 1 }'
}

if session_exists; then
    echo "Recreating Zellij session '$SESSION_NAME' from the current layout."
    zellij delete-session --force "$SESSION_NAME" >/dev/null 2>&1 || {
        zellij kill-session "$SESSION_NAME" >/dev/null 2>&1 || true
        zellij delete-session "$SESSION_NAME" >/dev/null 2>&1 || true
    }
fi

zellij attach --forget --create-background "$SESSION_NAME" options \
    --session-serialization false \
    --default-layout "$LAYOUT_FILE"

if [ ! -t 0 ] || [ ! -t 1 ]; then
    echo "Zellij session '$SESSION_NAME' started in the background."
    echo "Attach from an interactive terminal with: zellij attach '$SESSION_NAME'"
    exit 0
fi

exec zellij attach "$SESSION_NAME"
