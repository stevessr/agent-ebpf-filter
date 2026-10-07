#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
DEV_ENV_FILE="${DEV_ENV_FILE:-$ROOT/.env.dev}"
if [[ -f "$DEV_ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "$DEV_ENV_FILE"
    set +a
fi
DESKTOP_DIR="${RENEW_DESKTOP_DIR:-$ROOT/desktop/renew}"
[[ "$DESKTOP_DIR" = /* ]] || DESKTOP_DIR="$ROOT/$DESKTOP_DIR"
command -v go >/dev/null || { echo "Missing go; run make predev." >&2; exit 127; }

case "${1:-dev}" in
    prepare)
        [[ "$(go env GOOS)" = linux ]] || { echo "Bundled eBPF backend requires a Linux build." >&2; exit 1; }
        make --no-print-directory backend-bare CUDA_GO_TAGS=
        resources="$DESKTOP_DIR/resources/linux-$(go env GOARCH)"
        mkdir -p "$resources/backend"
        cp backend/agent-ebpf-filter "$resources/backend/agent-ebpf-filter"
        rm -rf "$resources/frontend"
        ;;
    build)
        cd "$DESKTOP_DIR"
        exec go tool mygo build
        ;;
    dev)
        make --no-print-directory backend-bare
        backend_port="${AGENT_BACKEND_PORT:-}"
        if [[ -z "$backend_port" && -f backend/.port ]]; then
            read -r backend_port < backend/.port || true
        fi
        backend_port="${backend_port:-8080}"
        export AGENT_BACKEND_URL="${AGENT_BACKEND_URL:-http://127.0.0.1:$backend_port}"
        export AGENT_RENEW_BACKEND_BIN="$ROOT/backend/agent-ebpf-filter"
        export AGENT_RENEW_DEV=true
        echo "[renew] Native UI -> backend: $AGENT_BACKEND_URL"
        echo "[renew] Desktop will reuse the backend or request authorization to start it."
        cd "$DESKTOP_DIR"
        exec go tool mygo dev
        ;;
    *) echo "Usage: $0 {dev|build|prepare}" >&2; exit 2 ;;
esac
