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

prepare_embedded_backend() {
    [[ "$(go env GOOS)" = linux ]] || {
        echo "The embedded eBPF backend currently requires a Linux build." >&2
        exit 1
    }

    command -v protoc >/dev/null || {
        echo "Missing protoc; install protobuf-compiler." >&2
        exit 127
    }
    command -v protoc-gen-go >/dev/null || {
        echo "Missing protoc-gen-go; run make predev-go." >&2
        exit 127
    }

    echo "[renew] Generating protobuf bindings for the embedded backend..."
    mkdir -p backend/pb
    protoc --go_out=backend/pb --go_opt=paths=source_relative -I proto         proto/tracker_common.proto         proto/tracker_events.proto         proto/tracker_registration.proto         proto/tracker_system.proto         proto/tracker_config.proto         proto/tracker_shell.proto

    echo "[renew] Generating eBPF objects for the embedded backend..."
    (
        cd backend/ebpf
        go generate
        go generate gen_tls.go
        go generate gen_cgroup.go
        go generate gen_lsm.go
    )

    # Remove stale sidecars left by builds from before the single-binary layout.
    if [[ -d "$DESKTOP_DIR/resources" ]]; then
        find "$DESKTOP_DIR/resources" -mindepth 2 -maxdepth 2 -type d -name backend -exec rm -rf {} + 2>/dev/null || true
    fi
}

case "${1:-dev}" in
    prepare)
        prepare_embedded_backend
        ;;
    build)
        cd "$DESKTOP_DIR"
        exec go tool mygo build
        ;;
    dev)
        prepare_embedded_backend
        backend_port="${AGENT_BACKEND_PORT:-8080}"
        export AGENT_BACKEND_URL="${AGENT_BACKEND_URL:-http://127.0.0.1:$backend_port}"
        export AGENT_RENEW_DEV=true
        echo "[renew] Native single-binary UI/backend; backend: $AGENT_BACKEND_URL"
        cd "$DESKTOP_DIR"
        exec go tool mygo dev
        ;;
    *)
        echo "Usage: $0 {dev|build|prepare}" >&2
        exit 2
        ;;
esac
