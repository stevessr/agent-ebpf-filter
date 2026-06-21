#!/usr/bin/env python3
"""
Visualize concurrent eBPF benchmark results
Shows how overhead changes with concurrency
"""

import json
import sys
import os
import glob
from pathlib import Path

def load_summary(filepath):
    """Load a summary JSON file"""
    with open(filepath) as f:
        return json.load(f)

def format_concurrency_comparison(summaries):
    """Format comparison across concurrency levels"""
    lines = []
    lines.append("╔" + "═" * 70 + "╗")
    lines.append("║" + " " * 10 + "eBPF Overhead vs Concurrency Level" + " " * 26 + "║")
    lines.append("╚" + "═" * 70 + "╝")
    lines.append("")

    # Sort by concurrency
    summaries.sort(key=lambda x: x['concurrency'])

    # Overall statistics by concurrency
    lines.append("=" * 72)
    lines.append("Overall Average Overhead by Concurrency")
    lines.append("=" * 72)
    lines.append("")

    lines.append(f"{'Concurrency':<15} {'Avg Overhead':<15} {'Min':<10} {'Max':<10} {'Operations'}")
    lines.append("-" * 72)

    for summary in summaries:
        concurrency = summary['concurrency']
        ops = summary['operations']

        if not ops:
            continue

        overheads = [op['overhead_percent'] for op in ops]
        avg_overhead = sum(overheads) / len(overheads)
        min_overhead = min(overheads)
        max_overhead = max(overheads)

        symbol = "✨" if avg_overhead < 0 else "⚠️" if avg_overhead > 2 else "✓"

        lines.append(
            f"{concurrency:<15} {symbol} {avg_overhead:>6.2f}%{'':<6} "
            f"{min_overhead:>6.2f}%{'':<1} {max_overhead:>6.2f}%{'':<1} {len(ops)}"
        )

    lines.append("")
    lines.append("=" * 72)
    lines.append("Per-Operation Overhead Trends")
    lines.append("=" * 72)
    lines.append("")

    # Get all operation names
    all_ops = set()
    for summary in summaries:
        for op in summary['operations']:
            all_ops.add(op['name'])

    # Show each operation's trend across concurrency levels
    for op_name in sorted(all_ops):
        lines.append(f"\n📊 {op_name}")
        lines.append("-" * 72)
        lines.append(f"{'Concurrency':<15} {'Baseline':<15} {'eBPF':<15} {'Overhead'}")
        lines.append("-" * 72)

        for summary in summaries:
            concurrency = summary['concurrency']
            op_data = next((op for op in summary['operations'] if op['name'] == op_name), None)

            if not op_data:
                continue

            baseline = op_data['baseline_us']
            ebpf = op_data['ebpf_us']
            overhead = op_data['overhead_percent']

            symbol = "✨" if overhead < 0 else "⚠️" if overhead > 2 else "✓"

            # Visual bar
            bar_len = int(abs(overhead) * 2)
            bar_len = min(bar_len, 30)

            if overhead < 0:
                bar = "◄" + "█" * bar_len
            else:
                bar = "█" * bar_len + "►"

            lines.append(
                f"{concurrency:<15} {baseline:>7.3f} μs{'':<3} "
                f"{ebpf:>7.3f} μs{'':<3} "
                f"{symbol} {overhead:>6.2f}% {bar}"
            )

    return "\n".join(lines)

def format_scalability_analysis(summaries):
    """Analyze scalability characteristics"""
    lines = []
    lines.append("\n" + "=" * 72)
    lines.append("Scalability Analysis")
    lines.append("=" * 72)
    lines.append("")

    # Calculate overhead growth rate
    summaries.sort(key=lambda x: x['concurrency'])

    concurrency_levels = []
    avg_overheads = []

    for summary in summaries:
        ops = summary['operations']
        if ops:
            overheads = [op['overhead_percent'] for op in ops]
            avg_overhead = sum(overheads) / len(overheads)
            concurrency_levels.append(summary['concurrency'])
            avg_overheads.append(avg_overhead)

    if len(avg_overheads) >= 2:
        # Calculate growth rate
        overhead_range = max(avg_overheads) - min(avg_overheads)
        concurrency_range = max(concurrency_levels) - min(concurrency_levels)

        lines.append(f"Concurrency range tested: {min(concurrency_levels)} → {max(concurrency_levels)}")
        lines.append(f"Overhead range: {min(avg_overheads):.2f}% → {max(avg_overheads):.2f}%")
        lines.append(f"Overhead variation: {overhead_range:.2f}%")
        lines.append("")

        # Classify scalability
        if overhead_range < 1.0:
            classification = "✅ Excellent - Nearly constant overhead regardless of concurrency"
        elif overhead_range < 3.0:
            classification = "✓ Good - Overhead scales well with concurrency"
        elif overhead_range < 5.0:
            classification = "⚠️ Moderate - Some overhead growth with concurrency"
        else:
            classification = "❌ Poor - Significant overhead growth with concurrency"

        lines.append(f"Scalability: {classification}")
        lines.append("")

        # Show trend
        lines.append("Trend:")
        for i, (conc, overhead) in enumerate(zip(concurrency_levels, avg_overheads)):
            bar_len = int((overhead - min(avg_overheads) + 1) * 5)
            bar = "█" * bar_len
            lines.append(f"  C={conc:<3} {overhead:>6.2f}% {bar}")

    return "\n".join(lines)

def format_recommendations(summaries):
    """Generate recommendations based on results"""
    lines = []
    lines.append("\n" + "=" * 72)
    lines.append("Recommendations")
    lines.append("=" * 72)
    lines.append("")

    summaries.sort(key=lambda x: x['concurrency'])

    # Find best and worst concurrency levels
    best_concurrency = None
    worst_concurrency = None
    best_overhead = float('inf')
    worst_overhead = float('-inf')

    for summary in summaries:
        ops = summary['operations']
        if ops:
            overheads = [op['overhead_percent'] for op in ops]
            avg_overhead = sum(overheads) / len(overheads)

            if avg_overhead < best_overhead:
                best_overhead = avg_overhead
                best_concurrency = summary['concurrency']

            if avg_overhead > worst_overhead:
                worst_overhead = avg_overhead
                worst_concurrency = summary['concurrency']

    if best_concurrency and worst_concurrency:
        lines.append(f"📈 Best performance: Concurrency = {best_concurrency} ({best_overhead:+.2f}% overhead)")
        lines.append(f"📉 Worst performance: Concurrency = {worst_concurrency} ({worst_overhead:+.2f}% overhead)")
        lines.append("")

        if abs(worst_overhead - best_overhead) < 1.0:
            lines.append("✅ eBPF overhead is stable across all tested concurrency levels.")
            lines.append("   Safe to use in highly concurrent environments.")
        elif abs(worst_overhead - best_overhead) < 3.0:
            lines.append("✓ eBPF overhead shows minor variation with concurrency.")
            lines.append("   Generally safe for production use.")
        else:
            lines.append("⚠️ eBPF overhead increases noticeably with concurrency.")
            lines.append("   Consider profiling specific workloads before deployment.")

    return "\n".join(lines)

def main():
    if len(sys.argv) < 2:
        print("Usage: visualize-concurrent.py <output_directory>")
        sys.exit(1)

    out_dir = sys.argv[1]

    # Find all summary files
    summary_files = glob.glob(os.path.join(out_dir, "summary_c*.json"))

    if not summary_files:
        print(f"No summary files found in {out_dir}")
        sys.exit(1)

    # Load all summaries
    summaries = []
    for filepath in summary_files:
        summary = load_summary(filepath)
        summaries.append(summary)

    # Generate report
    report = []
    report.append(format_concurrency_comparison(summaries))
    report.append(format_scalability_analysis(summaries))
    report.append(format_recommendations(summaries))

    full_report = "\n".join(report)

    # Print to stdout
    print(full_report)

    # Save to file
    report_file = os.path.join(out_dir, "concurrent_report.txt")
    with open(report_file, 'w') as f:
        f.write(full_report)

    print(f"\n📄 Full report saved to: {report_file}")

if __name__ == "__main__":
    main()
