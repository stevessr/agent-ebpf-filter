#!/bin/bash
set -euo pipefail

ROLE="${1:-}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

case "$ROLE" in
    backend)
        TARGET="$ROOT/scripts/dev-backend.sh"
        ;;
    frontend)
        TARGET="$ROOT/scripts/dev-frontend.sh"
        ;;
    *)
        echo "Usage: $0 {backend|frontend}" >&2
        exit 64
        ;;
esac

export AGENT_EBPF_DEV_ROOT="$ROOT"
export AGENT_EBPF_DEV_TARGET="$TARGET"

if [ "${AGENT_EBPF_DEV_RTK_WRAPPED:-0}" != "1" ] && command -v rtk >/dev/null 2>&1; then
    export AGENT_EBPF_DEV_RTK_WRAPPED=1
    exec rtk bash -lc 'cd "$AGENT_EBPF_DEV_ROOT" && exec "$AGENT_EBPF_DEV_TARGET"'
fi

cd "$ROOT"
exec "$TARGET"
