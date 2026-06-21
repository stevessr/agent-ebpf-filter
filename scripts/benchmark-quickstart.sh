#!/usr/bin/env bash
# Quick Start: eBPF Overhead Benchmark
# This script guides you through the benchmarking process

set -euo pipefail

cat << 'BANNER'
╔══════════════════════════════════════════════════════════════════════╗
║          eBPF Performance Overhead Benchmark - Quick Start           ║
╚══════════════════════════════════════════════════════════════════════╝
BANNER

echo ""
echo "This benchmark measures the performance overhead of eBPF hooks by comparing"
echo "syscall execution times with and without eBPF programs loaded."
echo ""

# Check if baseline exists
LATEST_REPORT=$(ls -td reports/ebpf-overhead-*/ 2>/dev/null | head -1 || echo "")

if [[ -n "$LATEST_REPORT" ]] && [[ -f "$LATEST_REPORT/baseline.json" ]]; then
    echo "✓ Found existing baseline results: $LATEST_REPORT"
    STAMP=$(basename "$LATEST_REPORT" | sed 's/ebpf-overhead-//')

    if [[ -f "$LATEST_REPORT/ebpf.json" ]]; then
        echo "✓ Found existing eBPF results"
        echo ""
        echo "Complete benchmark results available!"
        echo "View report: cat $LATEST_REPORT/summary.json"
        exit 0
    else
        echo "⚠ Missing eBPF results"
        echo ""
        echo "To complete the benchmark, follow these steps:"
        echo ""
        echo "Step 1: Open a NEW terminal and start the backend with sudo:"
        echo "  cd backend && sudo go run ./app"
        echo ""
        echo "Step 2: Wait for backend to start (about 5-10 seconds)"
        echo "        Look for 'Listening on port 8080' message"
        echo ""
        echo "Step 3: In THIS terminal, run:"
        echo "  BENCH_STAMP=$STAMP ./scripts/run-benchmark-manual.sh"
        echo ""
        echo "Step 4: Stop the backend (Ctrl+C in the backend terminal)"
        echo ""
        exit 0
    fi
else
    echo "Starting fresh benchmark..."
    echo ""
    echo "═══════════════════════════════════════════════════════════════════════"
    echo "Step 1: Run BASELINE benchmark (without eBPF)"
    echo "═══════════════════════════════════════════════════════════════════════"
    echo ""
    echo "Make sure the backend is NOT running, then run:"
    echo "  ./scripts/run-benchmark-manual.sh"
    echo ""
    echo "This will create a timestamp-stamped directory in reports/"
    echo ""
    read -p "Press Enter to run baseline benchmark now..."

    ./scripts/run-benchmark-manual.sh

    # Extract timestamp from the latest report
    LATEST_REPORT=$(ls -td reports/ebpf-overhead-*/ 2>/dev/null | head -1)
    STAMP=$(basename "$LATEST_REPORT" | sed 's/ebpf-overhead-//')

    echo ""
    echo "═══════════════════════════════════════════════════════════════════════"
    echo "Step 2: Run eBPF benchmark (with hooks)"
    echo "═══════════════════════════════════════════════════════════════════════"
    echo ""
    echo "Now you need to start the backend with eBPF hooks enabled."
    echo ""
    echo "Option A - Manual (recommended for understanding):"
    echo "  1. Open a NEW terminal"
    echo "  2. cd backend && sudo go run ./app"
    echo "  3. Wait for 'Listening on port 8080'"
    echo "  4. Come back here and press Enter"
    echo ""
    echo "Option B - If you have passwordless sudo:"
    echo "  The script will try to start it for you"
    echo ""
    read -p "Press Enter when backend is running (or to try auto-start)..."

    # Try to start backend automatically
    if command -v sudo >/dev/null 2>&1; then
        echo "Attempting to start backend..."
        cd backend
        sudo -n true 2>/dev/null && {
            sudo go run ./app > /tmp/backend-bench.log 2>&1 &
            BACKEND_PID=$!
            sleep 8

            if kill -0 $BACKEND_PID 2>/dev/null; then
                echo "✓ Backend started (PID: $BACKEND_PID)"
                AUTO_STARTED=true
            else
                echo "✗ Failed to auto-start. Please start manually."
                AUTO_STARTED=false
            fi
        } || {
            echo "✗ Need sudo password. Please start backend manually in another terminal."
            AUTO_STARTED=false
        }
        cd ..
    else
        echo "✗ sudo not available. Please start backend manually."
        AUTO_STARTED=false
    fi

    if [[ "$AUTO_STARTED" != "true" ]]; then
        echo ""
        echo "Waiting for backend to be ready..."
        echo "Press Enter when you see 'Listening on port 8080' in the backend terminal"
        read
    fi

    # Run eBPF benchmark
    echo ""
    echo "Running eBPF benchmark..."
    BENCH_STAMP=$STAMP ./scripts/run-benchmark-manual.sh

    # Stop backend if we started it
    if [[ "${AUTO_STARTED:-false}" == "true" ]] && [[ -n "${BACKEND_PID:-}" ]]; then
        echo ""
        echo "Stopping backend..."
        sudo kill $BACKEND_PID 2>/dev/null || true
        sleep 2
    fi

    echo ""
    echo "═══════════════════════════════════════════════════════════════════════"
    echo "✓ Benchmark Complete!"
    echo "═══════════════════════════════════════════════════════════════════════"
    echo ""
    echo "Results saved to: $LATEST_REPORT"
    echo ""
    echo "View summary:"
    echo "  cat $LATEST_REPORT/summary.json"
    echo ""
    echo "View detailed report with overhead percentages:"
    echo "  python3 -c 'import json; d=json.load(open(\"$LATEST_REPORT/summary.json\")); print(json.dumps(d, indent=2))'"
    echo ""
fi
