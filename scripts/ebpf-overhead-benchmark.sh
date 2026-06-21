#!/usr/bin/env bash
# eBPF Performance Overhead Benchmark
# Compares program performance with and without eBPF hooks

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="$(date -u +%Y%m%d-%H%M%S)"
OUT_DIR="${EBPF_BENCH_OUTDIR:-$ROOT_DIR/reports/ebpf-overhead-$STAMP}"
BENCH_TOOL="$ROOT_DIR/scripts/benchmark-syscalls"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[benchmark]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[benchmark]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[benchmark]${NC} $*"
}

log_error() {
    echo -e "${RED}[benchmark]${NC} $*"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root to control eBPF programs"
        exit 1
    fi
}

# Build benchmark tool if not exists
build_bench_tool() {
    if [[ ! -f "$BENCH_TOOL" ]]; then
        log "Building benchmark tool..."
        cd "$ROOT_DIR/scripts"
        go build -o benchmark-syscalls ./benchmark-syscalls.go
    fi
}

# Check if eBPF programs are loaded
check_ebpf_loaded() {
    if [[ -d /sys/fs/bpf/agent-ebpf/maps ]] && \
       [[ -d /sys/fs/bpf/agent-ebpf/links ]] && \
       [[ "$(ls -A /sys/fs/bpf/agent-ebpf/links 2>/dev/null | wc -l)" -gt 0 ]]; then
        return 0
    fi
    return 1
}

# Unload eBPF programs
unload_ebpf() {
    log "Unloading eBPF programs..."
    rm -rf /sys/fs/bpf/agent-ebpf/links/* 2>/dev/null || true
    rm -rf /sys/fs/bpf/agent-ebpf/maps/* 2>/dev/null || true
    sleep 2
    log_success "eBPF programs unloaded"
}

# Load eBPF programs
load_ebpf() {
    log "Loading eBPF programs..."
    cd "$ROOT_DIR/backend"
    go run ./app --ebpf-bootstrap
    sleep 2
    log_success "eBPF programs loaded"
}

# Run benchmark without eBPF
run_baseline_benchmark() {
    log "Running baseline benchmark (no eBPF)..."
    mkdir -p "$OUT_DIR"

    "$BENCH_TOOL" --output "$OUT_DIR/baseline.json" --runs 5 --warmup 2

    log_success "Baseline benchmark completed"
}

# Run benchmark with eBPF
run_ebpf_benchmark() {
    log "Running eBPF benchmark (with hooks)..."

    "$BENCH_TOOL" --output "$OUT_DIR/ebpf.json" --runs 5 --warmup 2

    log_success "eBPF benchmark completed"
}

# Generate comparison report
generate_report() {
    log "Generating comparison report..."

    python3 - <<EOF
import json
import sys

def load_results(path):
    with open(path) as f:
        return json.load(f)

def calc_overhead(baseline, ebpf):
    if baseline == 0:
        return 0
    return ((ebpf - baseline) / baseline) * 100

baseline = load_results("$OUT_DIR/baseline.json")
ebpf = load_results("$OUT_DIR/ebpf.json")

print("\n" + "="*70)
print("eBPF Performance Overhead Report")
print("="*70)
print(f"Timestamp: $STAMP")
print(f"Output Directory: $OUT_DIR")
print("="*70 + "\n")

print(f"{'Operation':<20} {'Baseline (μs)':<15} {'eBPF (μs)':<15} {'Overhead':<15}")
print("-"*70)

for op in baseline.get("operations", []):
    op_name = op["name"]
    baseline_time = op["avg_time_us"]

    # Find matching operation in eBPF results
    ebpf_op = next((x for x in ebpf.get("operations", []) if x["name"] == op_name), None)
    if not ebpf_op:
        continue

    ebpf_time = ebpf_op["avg_time_us"]
    overhead = calc_overhead(baseline_time, ebpf_time)

    overhead_str = f"+{overhead:.2f}%" if overhead > 0 else f"{overhead:.2f}%"
    print(f"{op_name:<20} {baseline_time:<15.2f} {ebpf_time:<15.2f} {overhead_str:<15}")

print("-"*70)
print(f"\nTotal operations: {len(baseline.get('operations', []))}")
print(f"Runs per operation: {baseline.get('runs', 'N/A')}")
print(f"Warmup runs: {baseline.get('warmup', 'N/A')}")

# Calculate average overhead
if baseline.get("operations") and ebpf.get("operations"):
    total_overhead = 0
    count = 0
    for op in baseline.get("operations", []):
        ebpf_op = next((x for x in ebpf.get("operations", []) if x["name"] == op["name"]), None)
        if ebpf_op:
            overhead = calc_overhead(op["avg_time_us"], ebpf_op["avg_time_us"])
            total_overhead += overhead
            count += 1

    if count > 0:
        avg_overhead = total_overhead / count
        print(f"\nAverage overhead: {avg_overhead:.2f}%")

print("\n" + "="*70)

# Save summary
summary = {
    "timestamp": "$STAMP",
    "baseline_file": "$OUT_DIR/baseline.json",
    "ebpf_file": "$OUT_DIR/ebpf.json",
    "operations": []
}

for op in baseline.get("operations", []):
    ebpf_op = next((x for x in ebpf.get("operations", []) if x["name"] == op["name"]), None)
    if ebpf_op:
        summary["operations"].append({
            "name": op["name"],
            "baseline_us": op["avg_time_us"],
            "ebpf_us": ebpf_op["avg_time_us"],
            "overhead_percent": calc_overhead(op["avg_time_us"], ebpf_op["avg_time_us"])
        })

with open("$OUT_DIR/summary.json", "w") as f:
    json.dump(summary, f, indent=2)

print(f"Summary saved to: $OUT_DIR/summary.json\n")
EOF

    log_success "Report generated"
}

# Main execution
main() {
    check_root
    build_bench_tool

    log "Starting eBPF overhead benchmark"
    log "Output directory: $OUT_DIR"

    # Check initial state
    ebpf_was_loaded=false
    if check_ebpf_loaded; then
        ebpf_was_loaded=true
        log_warn "eBPF programs are currently loaded"
    fi

    # Run baseline without eBPF
    if check_ebpf_loaded; then
        unload_ebpf
    fi
    run_baseline_benchmark

    # Run with eBPF
    load_ebpf
    run_ebpf_benchmark

    # Restore original state
    if [[ "$ebpf_was_loaded" == "false" ]]; then
        log "Restoring original state (unloading eBPF)..."
        unload_ebpf
    fi

    # Generate report
    generate_report

    log_success "Benchmark completed successfully"
    log_success "Results available in: $OUT_DIR"
}

main "$@"
