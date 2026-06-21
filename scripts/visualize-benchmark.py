#!/usr/bin/env python3
"""
Generate performance comparison charts from benchmark results
"""

import json
import sys
from pathlib import Path

def generate_text_chart(summary_file):
    """Generate ASCII bar chart from summary data"""
    with open(summary_file) as f:
        data = json.load(f)

    operations = data.get("operations", [])
    if not operations:
        print("No operations found in summary")
        return

    print("\n" + "="*80)
    print("eBPF Performance Overhead Visualization")
    print("="*80)
    print(f"Timestamp: {data.get('timestamp', 'N/A')}")
    print("="*80 + "\n")

    # Find max values for scaling
    max_baseline = max(op["baseline_us"] for op in operations)
    max_ebpf = max(op["ebpf_us"] for op in operations)
    max_overhead = max(op["overhead_percent"] for op in operations)

    # Chart 1: Execution time comparison
    print("Execution Time Comparison (μs)")
    print("-"*80)

    for op in operations:
        name = op["name"]
        baseline = op["baseline_us"]
        ebpf = op["ebpf_us"]

        # Calculate bar lengths (max 35 chars)
        baseline_bar_len = int((baseline / max_ebpf) * 35)
        ebpf_bar_len = int((ebpf / max_ebpf) * 35)

        print(f"{name:15} Baseline: {'█' * baseline_bar_len} {baseline:.3f}")
        print(f"{' ':15} eBPF:     {'█' * ebpf_bar_len} {ebpf:.3f}")
        print()

    print("\n" + "="*80)
    print("Overhead Percentage")
    print("-"*80)

    # Chart 2: Overhead percentage
    for op in operations:
        name = op["name"]
        overhead = op["overhead_percent"]

        # Calculate bar length (max 50 chars)
        bar_len = int((overhead / max_overhead) * 50)

        # Color code: green (<10%), yellow (10-30%), red (>30%)
        if overhead < 10:
            color = "🟢"
        elif overhead < 30:
            color = "🟡"
        else:
            color = "🔴"

        print(f"{name:15} {color} {'█' * bar_len} {overhead:+.2f}%")

    # Statistics
    avg_overhead = sum(op["overhead_percent"] for op in operations) / len(operations)

    print("\n" + "="*80)
    print("Summary Statistics")
    print("-"*80)
    print(f"Total operations:    {len(operations)}")
    print(f"Average overhead:    {avg_overhead:.2f}%")
    print(f"Min overhead:        {min(op['overhead_percent'] for op in operations):.2f}%")
    print(f"Max overhead:        {max(op['overhead_percent'] for op in operations):.2f}%")

    # Category analysis
    fast_ops = [op for op in operations if op["baseline_us"] < 0.1]
    medium_ops = [op for op in operations if 0.1 <= op["baseline_us"] < 1.0]
    slow_ops = [op for op in operations if op["baseline_us"] >= 1.0]

    print("\n" + "="*80)
    print("Category Analysis")
    print("-"*80)

    if fast_ops:
        avg_fast = sum(op["overhead_percent"] for op in fast_ops) / len(fast_ops)
        print(f"Fast syscalls (<0.1μs):    {len(fast_ops)} ops, {avg_fast:.2f}% avg overhead")
        print(f"  Examples: {', '.join(op['name'] for op in fast_ops[:3])}")

    if medium_ops:
        avg_medium = sum(op["overhead_percent"] for op in medium_ops) / len(medium_ops)
        print(f"Medium syscalls (0.1-1μs): {len(medium_ops)} ops, {avg_medium:.2f}% avg overhead")
        print(f"  Examples: {', '.join(op['name'] for op in medium_ops[:3])}")

    if slow_ops:
        avg_slow = sum(op["overhead_percent"] for op in slow_ops) / len(slow_ops)
        print(f"Slow syscalls (>1μs):      {len(slow_ops)} ops, {avg_slow:.2f}% avg overhead")
        print(f"  Examples: {', '.join(op['name'] for op in slow_ops[:3])}")

    print("\n" + "="*80)
    print("Interpretation")
    print("-"*80)

    if avg_overhead < 15:
        print("✅ Excellent: eBPF overhead is minimal and acceptable for production use.")
    elif avg_overhead < 30:
        print("✓  Good: eBPF overhead is reasonable for most use cases.")
    elif avg_overhead < 50:
        print("⚠  Moderate: Consider optimizing eBPF programs for performance-critical workloads.")
    else:
        print("⚠  High: eBPF overhead is significant. Review hook implementation.")

    print("\nNote: Fast syscalls show higher percentage overhead due to their extremely")
    print("      low baseline cost. Absolute time increase is still negligible.")
    print("="*80 + "\n")


def generate_csv(summary_file, output_csv):
    """Generate CSV for external analysis"""
    with open(summary_file) as f:
        data = json.load(f)

    operations = data.get("operations", [])

    with open(output_csv, 'w') as f:
        f.write("operation,baseline_us,ebpf_us,overhead_percent,absolute_overhead_us\n")
        for op in operations:
            abs_overhead = op["ebpf_us"] - op["baseline_us"]
            f.write(f"{op['name']},{op['baseline_us']},{op['ebpf_us']},{op['overhead_percent']},{abs_overhead}\n")

    print(f"CSV exported to: {output_csv}")


def main():
    if len(sys.argv) < 2:
        print("Usage: python3 visualize-benchmark.py <summary.json> [--csv output.csv]")
        sys.exit(1)

    summary_file = sys.argv[1]

    if not Path(summary_file).exists():
        print(f"Error: File not found: {summary_file}")
        sys.exit(1)

    # Generate text chart
    generate_text_chart(summary_file)

    # Generate CSV if requested
    if "--csv" in sys.argv:
        csv_idx = sys.argv.index("--csv")
        if csv_idx + 1 < len(sys.argv):
            output_csv = sys.argv[csv_idx + 1]
            generate_csv(summary_file, output_csv)


if __name__ == "__main__":
    main()
