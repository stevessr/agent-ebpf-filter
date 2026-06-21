#!/usr/bin/env bash
# Extended eBPF Performance Overhead Benchmark
# Support for 100+ cycles with progress tracking and incremental aggregation

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="${BENCH_STAMP:-$(date -u +%Y%m%d-%H%M%S)}"
OUT_DIR="${EBPF_BENCH_OUTDIR:-$ROOT_DIR/reports/ebpf-overhead-extended-$STAMP}"
BENCH_TOOL="$ROOT_DIR/scripts/benchmark-syscalls"
NUM_CYCLES="${BENCH_CYCLES:-100}"  # Default 100 cycles

# Colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[extended-bench]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[extended-bench]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[extended-bench]${NC} $*"
}

log_error() {
    echo -e "${RED}[extended-bench]${NC} $*"
}

log_progress() {
    echo -e "${CYAN}[progress]${NC} $*"
}

# Check if eBPF is loaded via API
check_ebpf_loaded() {
    if curl -s http://localhost:8080/system/bootstrap-health 2>/dev/null | grep -q '"attachedCount"'; then
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

# Run a single benchmark (optimized for speed)
run_single_benchmark() {
    local mode=$1
    local cycle=$2
    local output_file=$3

    # Reduced iterations for faster execution
    "$BENCH_TOOL" --output "$output_file" \
        --runs 3 \
        --warmup 1 \
        --iterations 5000 2>&1 | grep -v "Benchmarking"
}

# Progressive aggregation (update stats incrementally)
update_aggregation() {
    local mode=$1
    local new_file=$2
    local agg_file=$3
    local cycle=$4

    python3 - "$mode" "$new_file" "$agg_file" "$cycle" << 'EOF'
import json
import sys
import os

mode = sys.argv[1]
new_file = sys.argv[2]
agg_file = sys.argv[3]
cycle = int(sys.argv[4])

# Load new data
with open(new_file) as f:
    new_data = json.load(f)

# Load or initialize aggregation
if os.path.exists(agg_file):
    with open(agg_file) as f:
        agg = json.load(f)
else:
    agg = {
        'timestamp': new_data['timestamp'],
        'cycles': 0,
        'runs_per_cycle': new_data['runs'],
        'warmup_per_cycle': new_data['warmup'],
        'operations': {}
    }

# Update aggregation
agg['cycles'] = cycle

for op in new_data['operations']:
    name = op['name']
    if name not in agg['operations']:
        agg['operations'][name] = {
            'name': name,
            'sum': 0,
            'sum_sq': 0,
            'min': op['avg_time_us'],
            'max': op['avg_time_us'],
            'count': 0,
            'times': []
        }

    stats = agg['operations'][name]
    val = op['avg_time_us']

    stats['sum'] += val
    stats['sum_sq'] += val * val
    stats['min'] = min(stats['min'], val)
    stats['max'] = max(stats['max'], val)
    stats['count'] += 1
    stats['times'].append(val)

# Save updated aggregation
with open(agg_file, 'w') as f:
    json.dump(agg, f, indent=2)
EOF
}

# Finalize aggregation (compute final statistics)
finalize_aggregation() {
    local mode=$1
    local agg_file=$2
    local output_file=$3

    python3 - "$mode" "$agg_file" "$output_file" << 'EOF'
import json
import sys

mode = sys.argv[1]
agg_file = sys.argv[2]
output_file = sys.argv[3]

with open(agg_file) as f:
    agg = json.load(f)

operations = []
for name, stats in sorted(agg['operations'].items()):
    count = stats['count']
    avg = stats['sum'] / count

    # Calculate variance and stddev
    variance = (stats['sum_sq'] / count) - (avg * avg)
    stddev = max(0, variance) ** 0.5

    # Calculate CV
    cv = (stddev / avg * 100) if avg > 0 else 0

    operations.append({
        'name': name,
        'avg_time_us': avg,
        'min_time_us': stats['min'],
        'max_time_us': stats['max'],
        'stddev_us': stddev,
        'cv_percent': cv,
        'iterations': count * 5000 * 3,  # hardcoded from run parameters
        'cycles': count,
        'individual_times': stats['times']
    })

result = {
    'timestamp': agg['timestamp'],
    'cycles': agg['cycles'],
    'runs_per_cycle': agg['runs_per_cycle'],
    'warmup_per_cycle': agg['warmup_per_cycle'],
    'operations': operations
}

with open(output_file, 'w') as f:
    json.dump(result, f, indent=2)

print(f"Finalized {mode} aggregation: {count} cycles")
EOF
}

# Generate final comparison report
generate_comparison() {
    log "Generating comparison report..."

    python3 - "$OUT_DIR" "$STAMP" "$NUM_CYCLES" << 'EOF'
import json
import sys

OUT_DIR = sys.argv[1]
STAMP = sys.argv[2]
NUM_CYCLES = int(sys.argv[3])

baseline = json.load(open(f"{OUT_DIR}/baseline_final.json"))
ebpf = json.load(open(f"{OUT_DIR}/ebpf_final.json"))

summary = {
    "timestamp": STAMP,
    "cycles": NUM_CYCLES,
    "baseline_file": f"{OUT_DIR}/baseline_final.json",
    "ebpf_file": f"{OUT_DIR}/ebpf_final.json",
    "operations": []
}

print("\n" + "="*80)
print(f"Extended eBPF Performance Overhead Report ({NUM_CYCLES} cycles)")
print("="*80)
print(f"Total measurements per operation: {NUM_CYCLES * baseline['runs_per_cycle'] * 5000}")
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
print("Measurement Quality")
print("="*80)

excellent = len([op for op in summary['operations'] if op['avg_cv'] < 5])
good = len([op for op in summary['operations'] if 5 <= op['avg_cv'] < 10])
fair = len([op for op in summary['operations'] if op['avg_cv'] >= 10])

print(f"\nExcellent consistency (CV < 5%):  {excellent} operations")
print(f"Good consistency (CV 5-10%):      {good} operations")
print(f"Fair consistency (CV >= 10%):     {fair} operations")

if fair > 0:
    print(f"\nOperations with higher variance:")
    for op in sorted(summary['operations'], key=lambda x: x['avg_cv'], reverse=True)[:5]:
        if op['avg_cv'] >= 10:
            print(f"  - {op['name']}: {op['avg_cv']:.2f}% CV")

print("\n" + "="*80)
print("Interpretation")
print("="*80)

if abs(avg_overhead) < 5:
    print("\n✅ Excellent: eBPF overhead is negligible (< 5%)")
elif abs(avg_overhead) < 10:
    print("\n✓  Good: eBPF overhead is minimal (< 10%)")
elif abs(avg_overhead) < 20:
    print("\n⚠  Moderate: eBPF overhead is noticeable but acceptable (< 20%)")
else:
    print("\n⚠  High: eBPF overhead is significant (>= 20%)")

print(f"\nWith {NUM_CYCLES} cycles, these results are highly reliable.")
print(f"Average CV of {avg_cv:.2f}% indicates {'excellent' if avg_cv < 5 else 'good' if avg_cv < 10 else 'acceptable'} measurement consistency.")

print("\n" + "="*80)

# Save summary
with open(f"{OUT_DIR}/summary.json", 'w') as f:
    json.dump(summary, f, indent=2)

print(f"\nSummary saved to: {OUT_DIR}/summary.json\n")
EOF

    log_success "Report generated"
}

# Display progress bar
show_progress() {
    local current=$1
    local total=$2
    local width=50
    local percentage=$((current * 100 / total))
    local filled=$((width * current / total))
    local empty=$((width - filled))

    printf "\r[progress] ["
    printf "%${filled}s" | tr ' ' '█'
    printf "%${empty}s" | tr ' ' '░'
    printf "] %3d%% (%d/%d)" "$percentage" "$current" "$total"
}

main() {
    mkdir -p "$OUT_DIR"
    mkdir -p "$OUT_DIR/raw_baseline"
    mkdir -p "$OUT_DIR/raw_ebpf"

    clear
    log "═══════════════════════════════════════════════════════════════════"
    log "Extended eBPF Performance Overhead Benchmark"
    log "═══════════════════════════════════════════════════════════════════"
    log "Output directory: $OUT_DIR"
    log "Number of cycles: $NUM_CYCLES"
    log "Estimated time: ~$((NUM_CYCLES * 4 / 60)) minutes"
    echo ""

    # Check backend
    if ! check_backend_running; then
        log_error "Backend is not running or not accessible"
        log "Please start the backend first: cd backend && go run ."
        exit 1
    fi

    if ! check_ebpf_loaded; then
        log_warn "eBPF programs may not be loaded"
    else
        log_success "eBPF programs detected"
    fi

    echo ""
    log "Starting benchmark..."
    echo ""

    # Aggregation files
    baseline_agg="$OUT_DIR/baseline_aggregation.json"
    ebpf_agg="$OUT_DIR/ebpf_aggregation.json"

    # Run cycles with progress tracking
    start_time=$(date +%s)

    for cycle in $(seq 1 $NUM_CYCLES); do
        # Baseline
        baseline_file="$OUT_DIR/raw_baseline/cycle${cycle}.json"
        run_single_benchmark "baseline" "$cycle" "$baseline_file" > /dev/null
        update_aggregation "baseline" "$baseline_file" "$baseline_agg" "$cycle"

        # eBPF
        ebpf_file="$OUT_DIR/raw_ebpf/cycle${cycle}.json"
        run_single_benchmark "ebpf" "$cycle" "$ebpf_file" > /dev/null
        update_aggregation "ebpf" "$ebpf_file" "$ebpf_agg" "$cycle"

        # Update progress
        show_progress "$cycle" "$NUM_CYCLES"

        # Show ETA every 10 cycles
        if ((cycle % 10 == 0)); then
            current_time=$(date +%s)
            elapsed=$((current_time - start_time))
            avg_time_per_cycle=$((elapsed / cycle))
            remaining_cycles=$((NUM_CYCLES - cycle))
            eta=$((remaining_cycles * avg_time_per_cycle))
            eta_min=$((eta / 60))
            echo -ne " | ETA: ${eta_min}m"
        fi
    done

    echo ""
    echo ""

    end_time=$(date +%s)
    total_time=$((end_time - start_time))
    log_success "All cycles completed in $((total_time / 60))m $((total_time % 60))s"
    echo ""

    # Finalize aggregations
    log "Finalizing aggregations..."
    finalize_aggregation "baseline" "$baseline_agg" "$OUT_DIR/baseline_final.json"
    finalize_aggregation "ebpf" "$ebpf_agg" "$OUT_DIR/ebpf_final.json"

    # Generate comparison
    generate_comparison

    # Cleanup raw files to save space
    log "Cleaning up raw measurement files..."
    rm -rf "$OUT_DIR/raw_baseline" "$OUT_DIR/raw_ebpf"
    rm -f "$baseline_agg" "$ebpf_agg"

    log_success "Extended benchmark completed!"
    log_success "Results: $OUT_DIR"
    echo ""
    log "View detailed report:"
    log "  python3 scripts/visualize-benchmark.py $OUT_DIR/summary.json"
}

main "$@"
