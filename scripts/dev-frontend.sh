#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
if [ ! -d "$ROOT/frontend/node_modules" ]; then
    echo "--- [Dev] Installing frontend dependencies ---"
    (cd "$ROOT/frontend" && bun install)
fi

cd "$ROOT/frontend"
exec bun run dev
