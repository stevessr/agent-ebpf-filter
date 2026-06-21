#!/usr/bin/env bash
# Manual eBPF Performance Overhead Benchmark
# Run this script in two scenarios:
# 1. Without eBPF backend running
# 2. With eBPF backend running (in another terminal)

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAMP="${BENCH_STAMP:-$(date -u +%Y%m%d-%H%M%S)}"
OUT_DIR="${EBPF_BENCH_OUTDIR:-$ROOT_DIR/reports/ebpf-overhead-$STAMP}"
BENCH_TOOL="$ROOT_DIR/scripts/benchmark-syscalls"

# Colors
BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[benchmark]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[benchmark]${NC} $*"
}

log_warn() {
    echo -e "${YELLOW}[benchmark]${NC} $*"
}

# Check if backend is running
check_backend_running() {
    if pgrep -f "agent-ebpf-filter.*backend" > /dev/null 2>&1 || \
       lsof -ti:8080 > /dev/null 2>&1 || \
       [[ -f "$ROOT_DIR/backend/.port" ]]; then
        return 0
    fi
    return 1
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

main() {
    mkdir -p "$OUT_DIR"

    log "eBPF Performance Overhead Benchmark (Manual Mode)"
    log "Output directory: $OUT_DIR"
    echo ""

    # Detect current state
    if check_backend_running; then
        log_warn "Backend appears to be running"
    fi

    if check_ebpf_loaded; then
        log_warn "eBPF programs are loaded"
        MODE="ebpf"
        OUTPUT_FILE="$OUT_DIR/ebpf.json"
    else
        log "eBPF programs are NOT loaded"
        MODE="baseline"
        OUTPUT_FILE="$OUT_DIR/baseline.json"
    fi

    echo ""
    log "Running in $MODE mode..."
    log "Output: $OUTPUT_FILE"
    echo ""

    # Run benchmark
    "$BENCH_TOOL" --output "$OUTPUT_FILE" --runs 5 --warmup 2 --iterations 10000

    log_success "Benchmark completed"
    echo ""

    # Check if we have both results
    if [[ -f "$OUT_DIR/baseline.json" ]] && [[ -f "$OUT_DIR/ebpf.json" ]]; then
        log_success "Both baseline and eBPF results are available!"
        log "Generating comparison report..."
        generate_report
    else
        log_warn "Only $MODE results available."
        if [[ "$MODE" == "baseline" ]]; then
            log "To complete the benchmark:"
            log "  1. Start the backend: cd backend && sudo go run ./app"
            log "  2. Run this script again: BENCH_STAMP=$STAMP $0"
        else
            log "To complete the benchmark:"
            log "  1. Stop the backend"
            log "  2. Unload eBPF: sudo rm -rf /sys/fs/bpf/agent-ebpf/links/*"
            log "  3. Run this script again: BENCH_STAMP=$STAMP $0"
        fi
    fi
}

generate_report() {
    python3 - <<'EOF'
import json
import sys

def load_results(path):
    with open(path) as f:
        return json.load(f)

def calc_overhead(baseline, ebpf):
    if baseline == 0:
        return 0
    return ((ebpf - baseline) / baseline) * 100

baseline = load_results("$OUT_DIR/baseline.json".replace("$OUT_DIR", "$OUT_DIR"))
ebpf = load_results("$OUT_DIR/ebpf.json".replace("$OUT_DIR", "$OUT_DIR"))

print("\n" + "="*70)
print("eBPF Performance Overhead Report")
print("="*70)
print(f"Timestamp: $STAMP".replace("$STAMP", "$STAMP"))
print(f"Output Directory: $OUT_DIR".replace("$OUT_DIR", "$OUT_DIR"))
print("="*70 + "\n")

print(f"{'Operation':<20} {'Baseline (μs)':<15} {'eBPF (μs)':<15} {'Overhead':<15}")
print("-"*70)

for op in baseline.get("operations", []):
    op_name = op["name"]
    baseline_time = op["avg_time_us"]

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
    "timestamp": "$STAMP".replace("$STAMP", "$STAMP"),
    "baseline_file": "$OUT_DIR/baseline.json".replace("$OUT_DIR", "$OUT_DIR"),
    "ebpf_file": "$OUT_DIR/ebpf.json".replace("$OUT_DIR", "$OUT_DIR"),
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

with open("$OUT_DIR/summary.json".replace("$OUT_DIR", "$OUT_DIR"), "w") as f:
    json.dump(summary, f, indent=2)

print(f"Summary saved to: $OUT_DIR/summary.json\n".replace("$OUT_DIR", "$OUT_DIR"))
EOF

    log_success "Report generated: $OUT_DIR/summary.json"
}

main "$@"
