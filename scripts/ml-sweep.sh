#!/bin/bash
# ML Sweep Benchmark: runs all model types and generates comparison reports
# Usage: ./scripts/ml-sweep.sh [mode] [models...]
#   mode: comprehensive|quick|full (default: quick)
#   models: optional filters like random_forest,logistic,knn

set -euo pipefail

cd "$(dirname "$0")/../backend"

MODE="${1:-quick}"
shift 2>/dev/null || true

MODEL_FILTER=""
if [ $# -gt 0 ]; then
    MODEL_FILTER="$*"
fi

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
OUTDIR="../reports/ml-sweep-${TIMESTAMP}"

echo "============================================"
echo " ML Sweep Benchmark"
echo " Mode: ${MODE}"
echo " Models: ${MODEL_FILTER:-all}"
echo " Output: ${OUTDIR}"
echo "============================================"
echo ""

if [ ! -f "${HOME}/.config/agent-ebpf-filter/ml_training_data.bin" ]; then
    echo "WARNING: No training data found at ~/.config/agent-ebpf-filter/ml_training_data.bin"
    echo "The sweep will run against whatever data is available."
    echo ""
fi

export ML_SWEEP=1
export ML_SWEEP_MODE="${MODE}"
export ML_SWEEP_REPEATS="${ML_SWEEP_REPEATS:-5}"
export ML_SWEEP_STABILITY_TOP="${ML_SWEEP_STABILITY_TOP:-1}"
export ML_SWEEP_OUTDIR="${OUTDIR}"

if [ -n "${MODEL_FILTER}" ]; then
    export ML_SWEEP_MODELS="${MODEL_FILTER}"
fi

echo "Running model comparison..."
START_TS=$(date +%s)

go test -run TestMLSweep -v -count=1 -timeout=3600s 2>&1 | tee "${OUTDIR}-raw.log"

END_TS=$(date +%s)
DURATION=$((END_TS - START_TS))

echo ""
echo "============================================"
echo " Sweep Complete!"
echo " Duration: ${DURATION}s"
echo " Report: ${OUTDIR}/index.html"
echo " Results: ${OUTDIR}/results.csv"
echo " Stability: ${OUTDIR}/stability-summary.csv"
echo "============================================"
