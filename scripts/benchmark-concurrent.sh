#!/usr/bin/env bash
# Concurrent eBPF Performance Benchmark
# Tests eBPF overhead under different concurrency levels

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="${BENCH_STAMP:-$(date -u +%Y%m%d-%H%M%S)}"
OUT_DIR="${EBPF_BENCH_OUTDIR:-$ROOT_DIR/reports/ebpf-concurrent-$STAMP}"
BENCH_TOOL="$ROOT_DIR/scripts/benchmark-syscalls-extended"

# Concurrency levels to test (simulating real-world scenarios)
CONCURRENCY_LEVELS="${BENCH_CONCURRENCY:-1 4 8 16 32}"
CYCLES="${BENCH_CYCLES:-10}"
RUNS=3
WARMUP=1
ITERATIONS=5000

# Colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[concurrent-bench]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[concurrent-bench]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[concurrent-bench]${NC} $*"
}

log_error() {
    echo -e "${RED}[concurrent-bench]${NC} $*"
}

check_backend_running() {
    if ! curl -s http://localhost:8080/system/bootstrap-health >/dev/null 2>&1; then
        return 1
    fi
    return 0
}

check_ebpf_loaded() {
    local response
    response=$(curl -s http://localhost:8080/system/bootstrap-health 2>/dev/null || echo "")
    if echo "$response" | grep -q '"attachedCount"'; then
        local count
        count=$(echo "$response" | grep -o '"attachedCount":[0-9]*' | grep -o '[0-9]*')
        if [ -n "$count" ] && [ "$count" -gt 0 ]; then
            return 0
        fi
    fi
    return 1
}

run_benchmark_for_concurrency() {
    local concurrency=$1
    local mode=$2  # baseline or ebpf
    local output_file=$3

    log "Running ${mode} test with concurrency=${concurrency}..."
    "$BENCH_TOOL" \
        --output "$output_file" \
        --runs $RUNS \
        --warmup $WARMUP \
        --iterations $ITERATIONS \
        --concurrency "$concurrency" \
        2>&1 | sed 's/^/  /'
}

append_result_jsonl() {
    local mode=$1
    local concurrency=$2
    local cycle=$3
    local input_file=$4
    local jsonl_file=$5

    python3 - "$mode" "$concurrency" "$cycle" "$input_file" "$jsonl_file" << 'EOF'
import sys
import json

mode = sys.argv[1]
concurrency = int(sys.argv[2])
cycle = int(sys.argv[3])
input_file = sys.argv[4]
jsonl_file = sys.argv[5]

with open(input_file) as f:
    current = json.load(f)

current["mode"] = mode
current["cycle"] = cycle
current["concurrency"] = concurrency

with open(jsonl_file, "a") as f:
    f.write(json.dumps(current, separators=(",", ":")) + "\n")
EOF

    log "Appended cycle ${cycle} to $(basename "$jsonl_file")"
}

aggregate_results() {
    local mode=$1
    local concurrency=$2
    local cycle=$3
    local output_file=$4

    # Use Python to aggregate incremental results
    python3 - "$mode" "$concurrency" "$cycle" "$output_file" << 'EOF'
import sys
import json
import os

mode = sys.argv[1]
concurrency = sys.argv[2]
cycle = int(sys.argv[3])
output_file = sys.argv[4]

# Paths
out_dir = os.path.dirname(output_file)
agg_file = f"{out_dir}/agg_{mode}_c{concurrency}.json"

# Load current result
with open(output_file) as f:
    current = json.load(f)

# Initialize or load aggregate
if cycle == 1:
    aggregate = {
        "concurrency": int(concurrency),
        "cycles": 1,
        "operations": {}
    }
    for op in current["operations"]:
        aggregate["operations"][op["name"]] = {
            "count": 1,
            "sum": op["avg_time_us"],
            "sum_sq": op["avg_time_us"] ** 2
        }
else:
    with open(agg_file) as f:
        aggregate = json.load(f)

    aggregate["cycles"] = cycle
    for op in current["operations"]:
        name = op["name"]
        if name not in aggregate["operations"]:
            aggregate["operations"][name] = {"count": 0, "sum": 0, "sum_sq": 0}

        agg_op = aggregate["operations"][name]
        agg_op["count"] += 1
        agg_op["sum"] += op["avg_time_us"]
        agg_op["sum_sq"] += op["avg_time_us"] ** 2

# Save aggregate
with open(agg_file, 'w') as f:
    json.dump(aggregate, f, indent=2)

print(f"Aggregated cycle {cycle} for {mode} (concurrency={concurrency})")
EOF
}

compute_final_results() {
    local concurrency=$1
    local output_file=$2

    log "Computing final results for concurrency=${concurrency}..."

    python3 - "$concurrency" "$output_file" << 'EOF'
import sys
import json
import math
import os

concurrency = sys.argv[1]
output_file = sys.argv[2]

out_dir = os.path.dirname(output_file)
baseline_file = f"{out_dir}/agg_baseline_c{concurrency}.json"
ebpf_file = f"{out_dir}/agg_ebpf_c{concurrency}.json"

# Load aggregates
with open(baseline_file) as f:
    baseline = json.load(f)
with open(ebpf_file) as f:
    ebpf = json.load(f)

# Compute summary
summary = {
    "concurrency": int(concurrency),
    "cycles": baseline["cycles"],
    "operations": []
}

for op_name in baseline["operations"]:
    base_agg = baseline["operations"][op_name]
    ebpf_agg = ebpf["operations"].get(op_name)

    if not ebpf_agg:
        continue

    # Calculate means
    base_mean = base_agg["sum"] / base_agg["count"]
    ebpf_mean = ebpf_agg["sum"] / ebpf_agg["count"]

    # Calculate standard deviations
    base_var = (base_agg["sum_sq"] / base_agg["count"]) - (base_mean ** 2)
    ebpf_var = (ebpf_agg["sum_sq"] / ebpf_agg["count"]) - (ebpf_mean ** 2)
    base_stddev = math.sqrt(max(0, base_var))
    ebpf_stddev = math.sqrt(max(0, ebpf_var))

    overhead_us = ebpf_mean - base_mean
    overhead_percent = (overhead_us / base_mean * 100) if base_mean > 0 else 0

    summary["operations"].append({
        "name": op_name,
        "baseline_us": round(base_mean, 3),
        "ebpf_us": round(ebpf_mean, 3),
        "overhead_us": round(overhead_us, 3),
        "overhead_percent": round(overhead_percent, 2),
        "baseline_stddev": round(base_stddev, 3),
        "ebpf_stddev": round(ebpf_stddev, 3)
    })

# Sort by overhead
summary["operations"].sort(key=lambda x: x["overhead_percent"])

# Save summary
with open(output_file, 'w') as f:
    json.dump(summary, f, indent=2)

# Calculate overall statistics
overheads = [op["overhead_percent"] for op in summary["operations"]]
avg_overhead = sum(overheads) / len(overheads) if overheads else 0

print(f"Concurrency {concurrency}: Average overhead = {avg_overhead:.2f}%")
EOF
}

main() {
    mkdir -p "$OUT_DIR"

    # Clear screen only if running in a terminal
    if [ -t 1 ]; then
        clear 2>/dev/null || true
    fi

    log "═══════════════════════════════════════════════════════════════════"
    log "Concurrent eBPF Performance Benchmark"
    log "═══════════════════════════════════════════════════════════════════"
    log "Output directory: $OUT_DIR"
    log "Concurrency levels: $CONCURRENCY_LEVELS"
    log "Cycles per level: $CYCLES"
    log "Estimated time: ~$((CYCLES * $(echo "$CONCURRENCY_LEVELS" | wc -w) * 2 / 60)) minutes"
    echo ""

    # Check backend
    if ! check_backend_running; then
        log_error "Backend is not running or not accessible"
        log "Please start the backend first"
        exit 1
    fi
    log_success "Backend is running"
    echo ""

    # Test each concurrency level
    for concurrency in $CONCURRENCY_LEVELS; do
        log "========================================="
        log "Testing concurrency level: ${concurrency}"
        log "========================================="
        echo ""

        baseline_jsonl="$OUT_DIR/raw_baseline_c${concurrency}.jsonl"
        ebpf_jsonl="$OUT_DIR/raw_ebpf_c${concurrency}.jsonl"
        baseline_current="$OUT_DIR/.current_baseline_c${concurrency}.json"
        ebpf_current="$OUT_DIR/.current_ebpf_c${concurrency}.json"
        : > "$baseline_jsonl"
        : > "$ebpf_jsonl"

        # Run baseline tests
        log_warn "Phase 1: Baseline tests (concurrency=${concurrency})"
        for cycle in $(seq 1 $CYCLES); do
            log "Cycle $cycle/$CYCLES"
            run_benchmark_for_concurrency \
                "$concurrency" \
                "baseline" \
                "$baseline_current"

            append_result_jsonl "baseline" "$concurrency" "$cycle" \
                "$baseline_current" "$baseline_jsonl"
            aggregate_results "baseline" "$concurrency" "$cycle" \
                "$baseline_current"
        done
        rm -f "$baseline_current"
        log_success "Baseline complete for concurrency=${concurrency}; raw JSONL: $baseline_jsonl"
        echo ""

        # Run eBPF tests
        log_warn "Phase 2: eBPF tests (concurrency=${concurrency})"

        if ! check_ebpf_loaded; then
            log_error "eBPF programs not loaded!"
            exit 1
        fi

        for cycle in $(seq 1 $CYCLES); do
            log "Cycle $cycle/$CYCLES"
            run_benchmark_for_concurrency \
                "$concurrency" \
                "ebpf" \
                "$ebpf_current"

            append_result_jsonl "ebpf" "$concurrency" "$cycle" \
                "$ebpf_current" "$ebpf_jsonl"
            aggregate_results "ebpf" "$concurrency" "$cycle" \
                "$ebpf_current"
        done
        rm -f "$ebpf_current"
        log_success "eBPF tests complete for concurrency=${concurrency}; raw JSONL: $ebpf_jsonl"
        echo ""

        # Compute final results
        compute_final_results "$concurrency" "$OUT_DIR/summary_c${concurrency}.json"
        echo ""
    done

    # Generate comparison report
    log "Generating concurrency comparison report..."
    python3 "$ROOT_DIR/scripts/visualize-concurrent.py" "$OUT_DIR"

    log_success "═══════════════════════════════════════════════════════════════════"
    log_success "Benchmark complete!"
    log_success "Results: $OUT_DIR"
    log_success "═══════════════════════════════════════════════════════════════════"
}

main "$@"
