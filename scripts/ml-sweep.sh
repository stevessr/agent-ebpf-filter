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
#   --datasets <csv>
#   --points-per-param <N>
#   --workers <N>
#   --verbose-train-logs
#   --resume
#   --outdir <path>

set -euo pipefail

SCRIPT_PATH="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
REPO_ROOT="$(cd "$(dirname "$SCRIPT_PATH")/.." && pwd)"
cd "${REPO_ROOT}/backend"

MODE="${ML_SWEEP_MODE:-quick}"
REPEATS="${ML_SWEEP_REPEATS:-5}"
STABILITY_TOP="${ML_SWEEP_STABILITY_TOP:-1}"
OUTDIR="${ML_SWEEP_OUTDIR:-}"
MODEL_FILTER="${ML_SWEEP_MODELS:-}"
DATASET_FILTER="${ML_SWEEP_DATASETS:-}"
POINTS_PER_PARAM="${ML_SWEEP_POINTS_PER_PARAM:-}"
WORKERS="${ML_SWEEP_WORKERS:-1}"
QUIET_LOGS="${ML_SWEEP_QUIET_LOGS:-1}"
RESUME="${ML_SWEEP_RESUME:-}"

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
        --datasets)
            DATASET_FILTER="${2:-}"
            shift 2
            ;;
        --points-per-param)
            POINTS_PER_PARAM="${2:-}"
            shift 2
            ;;
        --workers)
            WORKERS="${2:-}"
            shift 2
            ;;
        --verbose-train-logs)
            QUIET_LOGS=0
            shift
            ;;
        --resume)
            RESUME=1
            shift
            ;;
        --outdir)
            OUTDIR="${2:-}"
            shift 2
            ;;
        --help|-h)
            sed -n '1,17p' "$SCRIPT_PATH"
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
    OUTDIR="${REPO_ROOT}/reports/ml-sweep-${TIMESTAMP}"
elif [[ "${OUTDIR}" != /* ]]; then
    OUTDIR="${REPO_ROOT}/${OUTDIR}"
fi

echo "============================================"
echo " ML Sweep Benchmark"
echo " Mode: ${MODE}"
echo " Models: ${MODEL_FILTER:-all}"
echo " Datasets: ${DATASET_FILTER:-default}"
echo " Points/param: ${POINTS_PER_PARAM:-1000}"
echo " Workers: ${WORKERS}"
echo " Quiet training logs: ${QUIET_LOGS}"
echo " Resume: ${RESUME:-0}"
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
export ML_SWEEP_POINTS_PER_PARAM="${POINTS_PER_PARAM:-1000}"
export ML_SWEEP_WORKERS="${WORKERS}"
export ML_SWEEP_QUIET_LOGS="${QUIET_LOGS}"
export ML_SWEEP_RESUME="${RESUME:-0}"

if [ -n "${MODEL_FILTER}" ]; then
    export ML_SWEEP_MODELS="${MODEL_FILTER}"
fi
if [ -n "${DATASET_FILTER}" ]; then
    export ML_SWEEP_DATASETS="${DATASET_FILTER}"
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
