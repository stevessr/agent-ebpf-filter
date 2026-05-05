#!/bin/bash
# ML Sweep Report Generator
# Parses sweep results and generates a comprehensive comparison table
set -euo pipefail

REPORT_DIR="${1:-../reports}"
cd "$(dirname "$0")/.."

# Find latest sweep
LATEST=$(ls -td "${REPORT_DIR}"/ml-sweep-* 2>/dev/null | head -1)
if [ -z "${LATEST}" ]; then
    echo "No sweep reports found in ${REPORT_DIR}/ml-sweep-*/"
    exit 1
fi

echo "══════════════════════════════════════════════════════════════"
echo "  ML Model Benchmark Report"
echo "  $(basename "${LATEST}")"
echo "══════════════════════════════════════════════════════════════"
echo ""

# Parse results.csv
CSV="${LATEST}/results.csv"
if [ -f "${CSV}" ]; then
    echo "── Model Accuracy & Speed ────────────────────────────────────"
    echo ""
    printf "%-25s %-12s %-12s %-14s %-14s %-12s\n" "Model" "Params" "Val.Acc" "Train(s)" "Inf(μs/sample)" "Throughput"
    printf "%-25s %-12s %-12s %-14s %-14s %-12s\n" "─────" "──────" "───────" "────────" "──────────────" "─────────"
    
    # Sort by validation accuracy descending, skip header
    tail -n +2 "${CSV}" 2>/dev/null | sort -t, -k3 -rn | while IFS=',' read -r name params valAcc trainDur infUs throughput rest; do
        # Sanitize and format
        if [ -n "${valAcc}" ]; then
            accPct=$(echo "${valAcc} * 100" | bc 2>/dev/null || echo "${valAcc}")
            printf "%-25s %-12s %-12s %-14s %-14s %-12s\n" "${name}" "${params}" "${valAcc}" "${trainDur}s" "${infUs}μs" "${throughput}"
        fi
    done
    echo ""
fi

# Parse stability summary
STABLE="${LATEST}/stability-summary.csv"
if [ -f "${STABLE}" ]; then
    echo "── Stability (mean ± std across repeats) ─────────────────────"
    echo ""
    printf "%-25s %-20s %-20s\n" "Model" "Accuracy (mean±std)" "Inference (mean±std)"
    printf "%-25s %-20s %-20s\n" "─────" "───────────────────" "───────────────────"
    tail -n +2 "${STABLE}" 2>/dev/null | while IFS=',' read -r name config accMean accStd infMean infStd rest; do
        printf "%-25s %-20s %-20s\n" "${name}" "${accMean}±${accStd}" "${infMean}±${infStd}μs"
    done
    echo ""
fi

# Find overall best
BEST_JSON="${LATEST}/best.json"
if [ -f "${BEST_JSON}" ]; then
    echo "── Best Configuration ────────────────────────────────────────"
    python3 -c "
import json
with open('${BEST_JSON}') as f:
    data = json.load(f)
print(f\"  Model: {data.get('model', 'N/A')}\")
print(f\"  Validation Accuracy: {data.get('accuracy', 0)*100:.1f}%\")
print(f\"  Parameters: {data.get('params', 'N/A')}\")
print(f\"  Train Duration: {data.get('trainDuration', 0):.2f}s\")
print(f\"  Inference: {data.get('inferenceUs', 0):.1f}μs/sample\")
"
fi

echo ""
echo "── HTML Report ──────────────────────────────────────────────"
echo "  Open: file://$(realpath "${LATEST}/index.html" 2>/dev/null || echo 'not found')"
echo ""
echo "── Raw Data ─────────────────────────────────────────────────"
echo "  CSV:   ${CSV}"
echo "  Best:  ${BEST_JSON}"
echo ""
