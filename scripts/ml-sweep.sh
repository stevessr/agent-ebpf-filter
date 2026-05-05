#!/bin/bash
# ML Sweep Benchmark: runs all model types and generates comparison reports
# Usage:
#   ./scripts/ml-sweep.sh [mode] [models...]
#   ./scripts/ml-sweep.sh --mode full --repeats 100 --stability-top 1 --models random_forest,logistic
# Supported options:
#   --mode <quick|full|comprehensive>
#   --repeats <N>
#   --stability-top <N>
#   --models <csv>
#   --outdir <path>

set -euo pipefail

cd "$(dirname "$0")/../backend"

MODE="${ML_SWEEP_MODE:-quick}"
REPEATS="${ML_SWEEP_REPEATS:-5}"
STABILITY_TOP="${ML_SWEEP_STABILITY_TOP:-1}"
OUTDIR="${ML_SWEEP_OUTDIR:-}"
MODEL_FILTER="${ML_SWEEP_MODELS:-}"

POSITIONAL=()
while [ $# -gt 0 ]; do
    case "$1" in
        --mode)
            MODE="${2:-}"
            shift 2
            ;;
        --repeats)
            REPEATS="${2:-}"
            shift 2
            ;;
        --stability-top)
            STABILITY_TOP="${2:-}"
            shift 2
            ;;
        --models)
            MODEL_FILTER="${2:-}"
            shift 2
            ;;
        --outdir)
            OUTDIR="${2:-}"
            shift 2
            ;;
        --help|-h)
            sed -n '1,24p' "$0"
            exit 0
            ;;
        --)
            shift
            while [ $# -gt 0 ]; do
                POSITIONAL+=("$1")
                shift
            done
            ;;
        -*)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

if [ ${#POSITIONAL[@]} -gt 0 ]; then
    if [[ "${POSITIONAL[0]}" =~ ^(quick|full|comprehensive)$ ]]; then
        MODE="${POSITIONAL[0]}"
        POSITIONAL=("${POSITIONAL[@]:1}")
    fi
fi

if [ -z "${MODEL_FILTER}" ] && [ ${#POSITIONAL[@]} -gt 0 ]; then
    MODEL_FILTER="$(IFS=, ; echo "${POSITIONAL[*]}")"
fi

if [ -z "${OUTDIR}" ]; then
    TIMESTAMP=$(date +%Y%m%d-%H%M%S)
    OUTDIR="../reports/ml-sweep-${TIMESTAMP}"
fi

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
export ML_SWEEP_REPEATS="${REPEATS}"
export ML_SWEEP_STABILITY_TOP="${STABILITY_TOP}"
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
