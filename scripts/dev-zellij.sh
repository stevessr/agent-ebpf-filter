#!/bin/bash
set -euo pipefail

SESSION_NAME="${AGENT_EBPF_DEV_SESSION:-agent-ebpf-dev}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LAYOUT_TEMPLATE="$ROOT/layouts/dev.kdl"
LAYOUT_FILE="$(mktemp "${TMPDIR:-/tmp}/agent-ebpf-dev-zellij.XXXXXX.kdl")"
trap 'rm -f "$LAYOUT_FILE"' EXIT

if ! command -v zellij >/dev/null 2>&1; then
    echo "zellij is required for make dev. Please install zellij and try again."
    exit 1
fi

if zellij list-sessions -s | awk -v name="$SESSION_NAME" '$0 == name { found = 1 } END { exit found ? 0 : 1 }'; then
    exec zellij attach "$SESSION_NAME"
fi

sed "s|__ROOT__|$ROOT|g" "$LAYOUT_TEMPLATE" > "$LAYOUT_FILE"
zellij attach --create-background "$SESSION_NAME" options --default-layout "$LAYOUT_FILE"
exec zellij attach "$SESSION_NAME"
