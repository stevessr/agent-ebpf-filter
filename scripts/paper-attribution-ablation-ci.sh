#!/usr/bin/env bash
# Run actual kernel-observed attribution under three isolated research modes.
set -euo pipefail
base="$PWD/reports/paper-attribution-ablation"
mkdir -p "$base"
git rev-parse HEAD >"$base/commit.txt"
uname -a >"$base/kernel.txt"
backend="$RUNNER_TEMP/paper-ablation-backend"
pid=""

cleanup() {
  if [[ -n "$pid" ]]; then
    sudo kill "$pid" >/dev/null 2>&1 || true
    wait "$pid" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

for mode in pid-only lifecycle-only combined; do
  label="$mode"
  if [[ "$mode" == "pid-only" ]]; then label=pid; fi
  if [[ "$mode" == "lifecycle-only" ]]; then label=lifecycle; fi
  dir="$base/$label"
  mkdir -p "$dir"
  printf '%s\n' "$mode" >"$dir/research-mode.txt"
  echo "[paper-ablation] Start isolated backend: $mode"
  sudo -E env DISABLE_AUTH=true GIN_MODE=release \
    AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE="$mode" "$backend" \
    >"$dir/backend.log" 2>&1 &
  pid=$!
  ready=0
  for i in $(seq 1 60); do
    if curl -fsS http://127.0.0.1:8080/system/bootstrap-health \
      >"$dir/bootstrap-health.json" 2>/dev/null && \
      jq -e '.attachedCount >= 1' "$dir/bootstrap-health.json" >/dev/null; then
      ready=1
      break
    fi
    if ! sudo kill -0 "$pid" 2>/dev/null; then
      echo "FAIL: $mode backend died" >&2
      tail -70 "$dir/backend.log" >&2 || true
      exit 1
    fi
    sleep 2
  done
  if [[ "$ready" != 1 ]]; then
    echo "FAIL: $mode has no verified attached kernel probes" >&2
    exit 1
  fi

  python3 scripts/attribution-concurrent-workload.py \
    --agents 2 3 4 --cycles 10 \
    --backend-url http://127.0.0.1:8080 \
    --capture-events-dir "$dir/snapshots" \
    --output "$dir/ground-truth.csv" \
    >"$dir/workload.log" 2>&1

  python3 scripts/paper-attribution-reconcile.py \
    --ground-truth "$dir/ground-truth.csv" --events-dir "$dir/snapshots" \
    --mode "$label" --output-dir "$dir/results" \
    >"$dir/scoring.log" 2>&1 || {
      echo "INCOMPLETE: $mode kernel event coverage; see $dir/scoring.log" >&2
      tail -70 "$dir/scoring.log" >&2
      exit 1
    }
  echo "[paper-ablation] $mode completed with observed predictions"
  cleanup
  pid=""
  sleep 1
done

python3 scripts/paper-attribution-ablation-summary.py --folder "$base" \
  --output "$base/summary.md"
