#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! command -v bun >/dev/null 2>&1; then
    echo "--- [Dev] Missing required command: bun ---" >&2
    echo "--- [Dev] Install Bun, run 'make predev', then retry 'make dev'. ---" >&2
    exit 127
fi

if [ ! -d "$ROOT/frontend/node_modules" ]; then
    echo "--- [Dev] Installing frontend dependencies ---"
    (cd "$ROOT/frontend" && bun install)
fi

cd "$ROOT/frontend"
exec bun run dev
