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

if ! command -v rustup >/dev/null 2>&1; then
    echo "--- [Dev] Missing required command: rustup ---" >&2
    echo "--- [Dev] Install Rust via rustup, then run 'make predev-webui'. ---" >&2
    exit 127
fi

if ! command -v trunk >/dev/null 2>&1; then
    echo "--- [Dev] Missing required command: trunk ---" >&2
    echo "--- [Dev] Run 'make predev-webui' first. ---" >&2
    exit 127
fi

cd "$ROOT/webui"
exec trunk serve
