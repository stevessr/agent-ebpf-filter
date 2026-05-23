#!/bin/bash
set -uo pipefail
# Auto-reload script with eBPF-aware hot-reload and privilege handling
# On startup: always clean old BPF pins to force a fresh bootstrap
# On shutdown: clean BPF pins so no stale state lingers

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_ENV_FILE="${DEV_ENV_FILE:-$ROOT/.env.dev}"
if [ -f "$DEV_ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$DEV_ENV_FILE"
    set +a
fi
cd "$ROOT"

BACKEND_DIR="backend"
WRAPPER_PATH="${AGENT_WRAPPER_PATH:-$ROOT/agent-wrapper}"
BACKEND_BIN="$ROOT/backend/agent-ebpf-filter"
BPF_PIN_ROOT="/sys/fs/bpf/agent-ebpf"
PID=""

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "--- [Dev] Missing required command: $1 ---" >&2
        echo "--- [Dev] Run 'make predev' after installing the repo toolchain, then retry 'make dev'. ---" >&2
        exit 127
    fi
}

require_cmd go
require_cmd sudo
require_cmd find
require_cmd md5sum

cleanup() {
    echo "--- [Dev] Shutting down ---"
    [ -n "${PID:-}" ] && sudo kill "$PID" 2>/dev/null || true
    [ -n "${PID:-}" ] && wait "$PID" 2>/dev/null || true
    echo "--- [Dev] Cleaning BPF pins ---"
    sudo rm -rf "$BPF_PIN_ROOT" 2>/dev/null
    exit
}

trap cleanup SIGINT SIGTERM

get_checksum() {
    find "$BACKEND_DIR" proto/ \( -name "*.go" -o -name "*.c" -o -name "*.h" -o -name "*.proto" \) -exec md5sum {} + 2>/dev/null | md5sum
}

while true; do
    echo "--- [Dev] Preparing Environment ---"
    # Remove root-owned build artifacts to prevent "Operation not permitted"
    if [ -f "backend/ebpf/agenttracker_bpfel.o" ]; then
        find backend/ebpf/ -name "agenttracker_bpf*" -user root -exec sudo rm -f {} +
    fi

    # Always wipe old BPF pins on startup to force a fresh eBPF bootstrap
    echo "--- [Dev] Cleaning old BPF pins for fresh bootstrap ---"
    sudo rm -rf "$BPF_PIN_ROOT" 2>/dev/null

    echo "--- [Dev] Building Backend ---"
    (cd backend/ebpf && go generate) && (cd backend && go build -o agent-ebpf-filter .)

    if [ $? -eq 0 ]; then
        echo "--- [Dev] Launching Backend ---"
        # Export first, then preserve by name. This keeps paths containing spaces
        # out of sudo's command/env assignment parser entirely.
        export DISABLE_AUTH="${DISABLE_AUTH:-true}"
        export GIN_MODE="${GIN_MODE:-debug}"
        export AGENT_WRAPPER_PATH="$WRAPPER_PATH"
        sudo_backend_cmd=(
            sudo
            --preserve-env=DISABLE_AUTH,GIN_MODE,AGENT_WRAPPER_PATH
            --
            "$BACKEND_BIN"
        )
        "${sudo_backend_cmd[@]}" &
        PID=$!

        LAST_SUM=$(get_checksum)
        while true; do
            sleep 2
            CURRENT_SUM=$(get_checksum)
            if [ "$LAST_SUM" != "$CURRENT_SUM" ]; then
                echo "--- [Dev] Source code changed, restarting ---"
                sudo kill "$PID" 2>/dev/null || true
                wait "$PID" 2>/dev/null || true
                break
            fi
        done
    else
        echo "--- [Dev] Build FAILED, waiting for changes ---"
        LAST_SUM=$(get_checksum)
        while true; do
            sleep 2
            CURRENT_SUM=$(get_checksum)
            if [ "$LAST_SUM" != "$CURRENT_SUM" ]; then break; fi
        done
    fi
done
