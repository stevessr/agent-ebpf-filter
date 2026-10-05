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
for command in go bun curl; do
    command -v "$command" >/dev/null || { echo "Missing $command; run make predev." >&2; exit 127; }
done
if [[ ! -d frontend/node_modules ]]; then
    (cd frontend && bun install)
fi

case "${1:-dev}" in
    prepare)
        [[ "$(go env GOOS)" = linux ]] || { echo "Bundled eBPF backend requires a Linux build." >&2; exit 1; }
        # Portable packages default to CPU; do not require the build host's CUDA.
        make --no-print-directory backend-bare CUDA_GO_TAGS=
        (cd frontend && bun run build)
        resources="$DESKTOP_DIR/resources/linux-$(go env GOARCH)"
        mkdir -p "$resources/backend" "$resources/frontend"
        cp backend/agent-ebpf-filter "$resources/backend/agent-ebpf-filter"
        # Replace the generated assets, so packages cannot contain stale chunks.
        python3 - "$ROOT/frontend/dist" "$resources/frontend/dist" <<'PY'
import shutil, sys
from pathlib import Path
destination = Path(sys.argv[2])
if destination.exists():
    shutil.rmtree(destination)
shutil.copytree(sys.argv[1], destination)
PY
        ;;
    build)
        cd "$DESKTOP_DIR"
        exec go tool mygo build # mygo.json buildCommand prepares all resources.
        ;;
    dev)
        make --no-print-directory backend-bare
        backend_port="${AGENT_BACKEND_PORT:-}"
        if [[ -z "$backend_port" && -f backend/.port ]]; then
            read -r backend_port < backend/.port || true
        fi
        backend_port="${backend_port:-8080}"
        export AGENT_BACKEND_URL="${AGENT_BACKEND_URL:-http://127.0.0.1:$backend_port}"
        export VITE_AGENT_BACKEND_URL="$AGENT_BACKEND_URL"
        export AGENT_RENEW_BACKEND_BIN="$ROOT/backend/agent-ebpf-filter"
        export AGENT_RENEW_FRONTEND_DIST="$ROOT/frontend/dist"
        export AGENT_RENEW_DEV=true
        frontend_port="${RENEW_FRONTEND_PORT:-5173}"
        export AGENT_RENEW_UI_URL="http://127.0.0.1:$frontend_port"
        pids=()
        # Separate child process groups let cleanup reach Vite/MyGo descendants,
        # without signalling a reused system backend or our invoking shell.
        set -m
        # shellcheck disable=SC2329 # Invoked by EXIT/INT/TERM traps.
        cleanup() {
            trap - EXIT INT TERM
            for pid in "${pids[@]}"; do kill -TERM -- "-$pid" 2>/dev/null || true; done
            for pid in "${pids[@]}"; do wait "$pid" 2>/dev/null || true; done
        }
        trap cleanup EXIT
        trap 'exit 130' INT
        trap 'exit 143' TERM
        (cd "$ROOT/frontend" && exec bun run dev --host 127.0.0.1 --port "$frontend_port" --strictPort) &
        frontend_pid=$!
        pids+=("$frontend_pid")
        ready=false
        for ((attempt = 0; attempt < 60; attempt++)); do
            kill -0 "$frontend_pid" 2>/dev/null || { echo "Renew WebUI exited before startup." >&2; exit 1; }
            if curl --noproxy '*' -fsS --max-time 1 "$AGENT_RENEW_UI_URL/renew" | grep -q '/@vite/client'; then
                ready=true
                break
            fi
            sleep 0.5
        done
        [[ "$ready" = true ]] || { echo "Timed out waiting for Renew WebUI." >&2; exit 1; }
        echo "[renew] WebUI: $AGENT_RENEW_UI_URL/renew; backend: $AGENT_BACKEND_URL"
        echo "[renew] Desktop will reuse the backend or request authorization to start it."
        (cd "$DESKTOP_DIR" && exec go tool mygo dev) &
        desktop_pid=$!
        pids+=("$desktop_pid")
        # Stop the whole session if either the desktop or frontend exits.
        status=0
        wait -n "$frontend_pid" "$desktop_pid" || status=$?
        exit "$status"
        ;;
    *) echo "Usage: $0 {dev|build|prepare}" >&2; exit 2 ;;
esac
