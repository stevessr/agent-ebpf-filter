#!/usr/bin/env bash
# Monitor extended benchmark progress

REPORT_DIR=$(ls -td /home/steve/文档/vibe\ coding/agent-ebpf-filiter/reports/ebpf-overhead-extended-* 2>/dev/null | head -1)

if [[ -z "$REPORT_DIR" ]]; then
    echo "No extended benchmark found"
    exit 1
fi

echo "Monitoring: $REPORT_DIR"
echo ""

while true; do
    clear
    echo "╔════════════════════════════════════════════════════════════╗"
    echo "║         Extended Benchmark Progress Monitor                ║"
    echo "╚════════════════════════════════════════════════════════════╝"
    echo ""

    # Check if raw directories exist
    if [[ -d "$REPORT_DIR/raw_baseline" ]]; then
        baseline_count=$(ls "$REPORT_DIR/raw_baseline" 2>/dev/null | wc -l)
        ebpf_count=$(ls "$REPORT_DIR/raw_ebpf" 2>/dev/null | wc -l)

        # Determine total expected (from env or default)
        expected=${BENCH_CYCLES:-100}

        # Calculate progress
        current=$((baseline_count < ebpf_count ? baseline_count : ebpf_count))
        percentage=$((current * 100 / expected))

        echo "Progress: ${current}/${expected} cycles (${percentage}%)"
        echo ""

        # Draw progress bar
        width=50
        filled=$((width * current / expected))
        empty=$((width - filled))

        printf "["
        printf "%${filled}s" | tr ' ' '█'
        printf "%${empty}s" | tr ' ' '░'
        printf "]\n\n"

        # Status
        echo "Baseline files: $baseline_count"
        echo "eBPF files:     $ebpf_count"
        echo ""

        # Check if aggregation files exist
        if [[ -f "$REPORT_DIR/baseline_aggregation.json" ]]; then
            echo "✓ Aggregation in progress"
        fi

        # Check if complete
        if [[ -f "$REPORT_DIR/summary.json" ]]; then
            echo ""
            echo "════════════════════════════════════════════════════════════"
            echo "✅ BENCHMARK COMPLETE!"
            echo "════════════════════════════════════════════════════════════"
            echo ""
            echo "View results:"
            echo "  cat $REPORT_DIR/summary.json"
            echo ""
            echo "Or visualize:"
            echo "  python3 scripts/visualize-benchmark.py $REPORT_DIR/summary.json"
            echo ""
            exit 0
        fi

    else
        echo "Waiting for benchmark to start..."
        echo ""
        echo "If this persists, check if the benchmark is running:"
        echo "  ps aux | grep benchmark-extended"
    fi

    echo ""
    echo "Press Ctrl+C to stop monitoring (benchmark will continue)"
    echo "Last update: $(date '+%H:%M:%S')"

    sleep 5
done
