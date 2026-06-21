#!/usr/bin/env bash
# Multi-run eBPF Performance Overhead Benchmark
# Runs multiple complete benchmark cycles to reduce variance

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="${BENCH_STAMP:-$(date -u +%Y%m%d-%H%M%S)}"
OUT_DIR="${EBPF_BENCH_OUTDIR:-$ROOT_DIR/reports/ebpf-overhead-multi-$STAMP}"
BENCH_TOOL="$ROOT_DIR/scripts/benchmark-syscalls"
NUM_CYCLES="${BENCH_CYCLES:-3}"  # Number of complete baseline+ebpf cycles

# Colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[multi-bench]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[multi-bench]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[multi-bench]${NC} $*"
}

log_error() {
    echo -e "${RED}[multi-bench]${NC} $*"
}

# Check if eBPF maps are loaded
check_ebpf_loaded() {
    # Try API first (works without root)
    if curl -s http://localhost:8080/system/bootstrap-health 2>/dev/null | grep -q '"attachedCount"'; then
        return 0
    fi

    # Fallback to filesystem check (requires root)
    if [[ -d /sys/fs/bpf/agent-ebpf/maps ]] && \
       [[ "$(ls -A /sys/fs/bpf/agent-ebpf/maps 2>/dev/null | wc -l)" -gt 0 ]]; then
        return 0
    fi
    return 1
}

# Check if backend is running
check_backend_running() {
    if curl -s http://localhost:8080/system/bootstrap-health >/dev/null 2>&1; then
        return 0
    fi
    return 1
}

# Run a single benchmark
run_single_benchmark() {
    local mode=$1
    local cycle=$2
    local output_file=$3

    log "Running ${mode} benchmark (cycle ${cycle}/${NUM_CYCLES})..."

    "$BENCH_TOOL" --output "$output_file" \
        --runs 5 \
        --warmup 2 \
        --iterations 10000

    log_success "Cycle ${cycle} ${mode} completed"
}

# Aggregate multiple runs
aggregate_results() {
    local mode=$1
    shift
    local files=("$@")
    local output="$OUT_DIR/${mode}_aggregated.json"

    log "Aggregating ${#files[@]} ${mode} results..."

    python3 - "$mode" "$output" "${files[@]}" << 'EOF'
import json
import sys
from collections import defaultdict

mode = sys.argv[1]
output = sys.argv[2]
files = sys.argv[3:]

# Load all results
all_data = []
for f in files:
    with open(f) as file:
        all_data.append(json.load(file))

if not all_data:
    sys.exit(1)

# Group by operation
op_data = defaultdict(lambda: {
    'times': [],
    'iterations': 0
})

for data in all_data:
    for op in data['operations']:
        op_data[op['name']]['times'].append(op['avg_time_us'])
        op_data[op['name']]['iterations'] += op['iterations']

# Calculate aggregated stats
aggregated_ops = []
for name, data in sorted(op_data.items()):
    times = data['times']
    avg = sum(times) / len(times)

    # Calculate variance and std dev
    variance = sum((t - avg) ** 2 for t in times) / len(times)
    stddev = variance ** 0.5

    # Calculate coefficient of variation (CV)
    cv = (stddev / avg * 100) if avg > 0 else 0

    aggregated_ops.append({
        'name': name,
        'avg_time_us': avg,
        'min_time_us': min(times),
        'max_time_us': max(times),
        'stddev_us': stddev,
        'cv_percent': cv,
        'iterations': data['iterations'],
        'cycles': len(times),
        'individual_times': times
    })

result = {
    'timestamp': all_data[0]['timestamp'],
    'cycles': len(all_data),
    'runs_per_cycle': all_data[0]['runs'],
    'warmup_per_cycle': all_data[0]['warmup'],
    'operations': aggregated_ops
}

with open(output, 'w') as f:
    json.dump(result, f, indent=2)

print(f"Aggregated results saved to: {output}")
EOF

    log_success "Aggregation complete: $output"
}

# Generate comparison report
generate_comparison() {
    log "Generating comparison report..."

    python3 - "$OUT_DIR" "$STAMP" << 'EOF'
import json
import sys

OUT_DIR = sys.argv[1]
STAMP = sys.argv[2]

baseline = json.load(open(f"{OUT_DIR}/baseline_aggregated.json"))
ebpf = json.load(open(f"{OUT_DIR}/ebpf_aggregated.json"))

summary = {
    "timestamp": STAMP,
    "cycles": baseline['cycles'],
    "baseline_file": f"{OUT_DIR}/baseline_aggregated.json",
    "ebpf_file": f"{OUT_DIR}/ebpf_aggregated.json",
    "operations": []
}

print("\n" + "="*80)
print("Multi-Cycle eBPF Performance Overhead Report")
print("="*80)
print(f"Cycles per mode: {baseline['cycles']}")
print(f"Total measurements: {baseline['cycles'] * baseline['runs_per_cycle']} runs/operation/mode")
print("="*80 + "\n")

print(f"{'Operation':<15} {'Baseline (μs)':<15} {'eBPF (μs)':<15} {'Overhead':<12} {'CV %':<10}")
print("-"*80)

for op in baseline['operations']:
    op_name = op['name']
    baseline_time = op['avg_time_us']
    baseline_cv = op['cv_percent']

    ebpf_op = next(x for x in ebpf['operations'] if x['name'] == op_name)
    ebpf_time = ebpf_op['avg_time_us']
    ebpf_cv = ebpf_op['cv_percent']

    overhead = ((ebpf_time - baseline_time) / baseline_time) * 100
    overhead_str = f"{overhead:+.2f}%"

    # Average CV
    avg_cv = (baseline_cv + ebpf_cv) / 2

    summary['operations'].append({
        'name': op_name,
        'baseline_us': baseline_time,
        'baseline_cv': baseline_cv,
        'ebpf_us': ebpf_time,
        'ebpf_cv': ebpf_cv,
        'overhead_percent': overhead,
        'avg_cv': avg_cv
    })

    print(f"{op_name:<15} {baseline_time:<15.3f} {ebpf_time:<15.3f} {overhead_str:<12} {avg_cv:<10.2f}")

print("-"*80)

# Statistics
overheads = [op['overhead_percent'] for op in summary['operations']]
cvs = [op['avg_cv'] for op in summary['operations']]

avg_overhead = sum(overheads) / len(overheads)
min_overhead = min(overheads)
max_overhead = max(overheads)
avg_cv = sum(cvs) / len(cvs)

print(f"\nAverage overhead:     {avg_overhead:+.2f}%")
print(f"Min overhead:         {min_overhead:+.2f}%")
print(f"Max overhead:         {max_overhead:+.2f}%")
print(f"Average CV:           {avg_cv:.2f}%")

print("\n" + "="*80)
print("Variance Analysis")
print("="*80)

high_variance_ops = [op for op in summary['operations'] if op['avg_cv'] > 10]
if high_variance_ops:
    print(f"\nHigh variance operations (CV > 10%):")
    for op in high_variance_ops:
        print(f"  - {op['name']}: {op['avg_cv']:.2f}% CV")
else:
    print("\n✓ All operations show consistent results (CV < 10%)")

print("\n" + "="*80)

# Save summary
with open(f"{OUT_DIR}/summary.json", 'w') as f:
    json.dump(summary, f, indent=2)

print(f"\nSummary saved to: {OUT_DIR}/summary.json\n")
EOF

    log_success "Report generated"
}

main() {
    mkdir -p "$OUT_DIR"

    log "Multi-Cycle eBPF Performance Overhead Benchmark"
    log "Output directory: $OUT_DIR"
    log "Number of cycles: $NUM_CYCLES"
    echo ""

    # Check if backend is running
    if ! check_backend_running; then
        log_error "Backend is not running or not accessible"
        log "Please start the backend first:"
        log "  cd backend && go run ."
        exit 1
    fi

    # Check eBPF status
    if ! check_ebpf_loaded; then
        log_warn "eBPF programs may not be loaded"
        log_warn "Results may not reflect actual eBPF overhead"
    else
        log_success "eBPF programs detected"
    fi

    echo ""

    # Arrays to store result files
    baseline_files=()
    ebpf_files=()

    # Run multiple cycles
    for cycle in $(seq 1 $NUM_CYCLES); do
        log "═══════════════════════════════════════════════════════════"
        log "Starting cycle ${cycle}/${NUM_CYCLES}"
        log "═══════════════════════════════════════════════════════════"

        # Baseline
        baseline_file="$OUT_DIR/baseline_cycle${cycle}.json"
        run_single_benchmark "baseline" "$cycle" "$baseline_file"
        baseline_files+=("$baseline_file")

        sleep 2  # Brief pause between runs

        # eBPF
        ebpf_file="$OUT_DIR/ebpf_cycle${cycle}.json"
        run_single_benchmark "ebpf" "$cycle" "$ebpf_file"
        ebpf_files+=("$ebpf_file")

        echo ""
    done

    # Aggregate results
    log "═══════════════════════════════════════════════════════════"
    log "Aggregating results from all cycles"
    log "═══════════════════════════════════════════════════════════"

    aggregate_results "baseline" "${baseline_files[@]}"
    aggregate_results "ebpf" "${ebpf_files[@]}"

    # Generate comparison
    generate_comparison

    log_success "Multi-cycle benchmark completed!"
    log_success "Results: $OUT_DIR"
    echo ""
    log "View detailed report:"
    log "  python3 scripts/visualize-benchmark.py $OUT_DIR/summary.json"
}

main "$@"
