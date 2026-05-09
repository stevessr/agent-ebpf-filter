#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%d-%H%M%S)"
OUT_DIR="${RUNTIME_REPLAY_OUTDIR:-$ROOT_DIR/reports/runtime-replay-$STAMP}"
OUT_FILE="${RUNTIME_REPLAY_OUT:-$OUT_DIR/summary.json}"

mkdir -p "$OUT_DIR"

echo "[runtime-replay] writing summary to: $OUT_FILE"
(cd "$ROOT_DIR/backend" && RUNTIME_REPLAY_OUT="$OUT_FILE" go test ./... -run TestRuntimeReplaySuite -count=1 -v)

echo "[runtime-replay] completed"
echo "[runtime-replay] summary: $OUT_FILE"
