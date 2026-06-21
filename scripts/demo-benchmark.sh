#!/usr/bin/env bash
# Demo: Complete benchmark workflow
# This script demonstrates the full benchmark process without requiring sudo

set -euo pipefail

cat << 'EOF'
╔══════════════════════════════════════════════════════════════════════╗
║              eBPF Benchmark Demo - Mock Results                      ║
╚══════════════════════════════════════════════════════════════════════╝

This demo shows what a complete benchmark run looks like.
For real results, use: ./scripts/benchmark-quickstart.sh

EOF

DEMO_DIR="reports/ebpf-overhead-demo"
mkdir -p "$DEMO_DIR"

echo "Step 1: Creating mock baseline results..."
cat > "$DEMO_DIR/baseline.json" << 'JSON'
{
  "timestamp": "2026-06-21T03:41:00Z",
  "runs": 5,
  "warmup": 2,
  "operations": [
    {
      "name": "getpid",
      "avg_time_us": 0.065,
      "min_time_us": 0.063,
      "max_time_us": 0.067,
      "stddev_us": 0.0014,
      "iterations": 50000
    },
    {
      "name": "open_close",
      "avg_time_us": 0.623,
      "min_time_us": 0.528,
      "max_time_us": 0.885,
      "stddev_us": 0.134,
      "iterations": 50000
    },
    {
      "name": "stat",
      "avg_time_us": 0.330,
      "min_time_us": 0.265,
      "max_time_us": 0.500,
      "stddev_us": 0.088,
      "iterations": 50000
    },
    {
      "name": "read_write",
      "avg_time_us": 5.295,
      "min_time_us": 4.845,
      "max_time_us": 5.670,
      "stddev_us": 0.293,
      "iterations": 50000
    },
    {
      "name": "socket_close",
      "avg_time_us": 1.262,
      "min_time_us": 1.166,
      "max_time_us": 1.367,
      "stddev_us": 0.073,
      "iterations": 50000
    }
  ]
}
JSON

echo "✓ Baseline results created"
echo ""

echo "Step 2: Creating mock eBPF results (with 15% average overhead)..."
cat > "$DEMO_DIR/ebpf.json" << 'JSON'
{
  "timestamp": "2026-06-21T03:45:00Z",
  "runs": 5,
  "warmup": 2,
  "operations": [
    {
      "name": "getpid",
      "avg_time_us": 0.104,
      "min_time_us": 0.100,
      "max_time_us": 0.108,
      "stddev_us": 0.0028,
      "iterations": 50000
    },
    {
      "name": "open_close",
      "avg_time_us": 0.685,
      "min_time_us": 0.580,
      "max_time_us": 0.950,
      "stddev_us": 0.145,
      "iterations": 50000
    },
    {
      "name": "stat",
      "avg_time_us": 0.360,
      "min_time_us": 0.290,
      "max_time_us": 0.545,
      "stddev_us": 0.095,
      "iterations": 50000
    },
    {
      "name": "read_write",
      "avg_time_us": 5.610,
      "min_time_us": 5.130,
      "max_time_us": 6.010,
      "stddev_us": 0.310,
      "iterations": 50000
    },
    {
      "name": "socket_close",
      "avg_time_us": 1.388,
      "min_time_us": 1.280,
      "max_time_us": 1.500,
      "stddev_us": 0.080,
      "iterations": 50000
    }
  ]
}
JSON

echo "✓ eBPF results created"
echo ""

echo "Step 3: Generating summary report..."
python3 - << 'PYTHON'
import json

baseline = json.load(open("reports/ebpf-overhead-demo/baseline.json"))
ebpf = json.load(open("reports/ebpf-overhead-demo/ebpf.json"))

summary = {
    "timestamp": "demo",
    "baseline_file": "reports/ebpf-overhead-demo/baseline.json",
    "ebpf_file": "reports/ebpf-overhead-demo/ebpf.json",
    "operations": []
}

for op in baseline["operations"]:
    ebpf_op = next(x for x in ebpf["operations"] if x["name"] == op["name"])
    overhead = ((ebpf_op["avg_time_us"] - op["avg_time_us"]) / op["avg_time_us"]) * 100
    summary["operations"].append({
        "name": op["name"],
        "baseline_us": op["avg_time_us"],
        "ebpf_us": ebpf_op["avg_time_us"],
        "overhead_percent": overhead
    })

with open("reports/ebpf-overhead-demo/summary.json", "w") as f:
    json.dump(summary, f, indent=2)
PYTHON

echo "✓ Summary created"
echo ""

echo "Step 4: Visualizing results..."
echo ""

python3 scripts/visualize-benchmark.py "$DEMO_DIR/summary.json"

echo ""
echo "────────────────────────────────────────────────────────────────────"
echo "Demo complete! Mock results saved to: $DEMO_DIR/"
echo ""
echo "To run real benchmarks with actual eBPF measurements:"
echo "  ./scripts/benchmark-quickstart.sh"
echo "────────────────────────────────────────────────────────────────────"
