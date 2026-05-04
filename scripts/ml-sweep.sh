#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODE="${ML_SWEEP_MODE:-quick}"
MODELS="${ML_SWEEP_MODELS:-}"
OUTDIR="${ML_SWEEP_OUTDIR:-$ROOT_DIR/reports/ml-sweep-$(date +%Y%m%d-%H%M%S)}"
REPEATS="${ML_SWEEP_REPEATS:-100}"
STABILITY_TOP="${ML_SWEEP_STABILITY_TOP:-1}"

usage() {
  cat <<'EOF'
Usage: scripts/ml-sweep.sh [--mode quick|full] [--models m1,m2] [--outdir path] [--repeats N] [--stability-top N]

Environment variables:
  ML_SWEEP_MODE       quick (default) or full
  ML_SWEEP_MODELS     comma-separated model filter, e.g. random_forest,knn
  ML_SWEEP_OUTDIR     output directory for CSV/SVG/HTML artifacts
  ML_SWEEP_REPEATS    repeat count for the stability phase (default: 100)
  ML_SWEEP_STABILITY_TOP  top grid points per profile to repeat (default: 1)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --models)
      MODELS="${2:-}"
      shift 2
      ;;
    --outdir)
      OUTDIR="${2:-}"
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
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$OUTDIR" in
  /*) ;;
  *) OUTDIR="$ROOT_DIR/$OUTDIR" ;;
esac

mkdir -p "$OUTDIR"

export ML_SWEEP=1
export ML_SWEEP_MODE="$MODE"
export ML_SWEEP_MODELS="$MODELS"
export ML_SWEEP_OUTDIR="$OUTDIR"
export ML_SWEEP_REPEATS="$REPEATS"
export ML_SWEEP_STABILITY_TOP="$STABILITY_TOP"

echo "[ml-sweep] root=$ROOT_DIR"
echo "[ml-sweep] mode=$ML_SWEEP_MODE"
if [[ -n "$ML_SWEEP_MODELS" ]]; then
  echo "[ml-sweep] models=$ML_SWEEP_MODELS"
fi
echo "[ml-sweep] outdir=$ML_SWEEP_OUTDIR"
echo "[ml-sweep] repeats=$ML_SWEEP_REPEATS top=$ML_SWEEP_STABILITY_TOP"

cd "$ROOT_DIR/backend"
go test -run TestMLSweep -count=1
