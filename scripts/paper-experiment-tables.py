#!/usr/bin/env python3
"""Render Section 6 evidence tables from measured inputs, refusing invalid comparisons.

No table cell is inferred from code paths, expected behavior, or example numbers.
"""
from __future__ import annotations

import argparse
import csv
import importlib.util
import json
import math
import statistics
from pathlib import Path

CONCURRENCIES = (1, 4, 8, 16, 32)
MODES = ("pid_only", "lifecycle_hook_only", "trimodal")
CASES = (
    "tcp_connect", "udp_connect", "udp_sendto", "ipv4_mapped_ipv6",
    "existing_fd_read", "existing_fd_write", "mmap",
)
UNKNOWN = {"", "⊥", "\\bot", "unattributed", "none", "null"}


def percentile(values: list[float], percent: float) -> float:
    if not values:
        raise ValueError("cannot compute percentile of empty samples")
    values = sorted(values)
    pos = (len(values) - 1) * percent / 100
    low, high = math.floor(pos), math.ceil(pos)
    return values[low] + (values[high] - values[low]) * (pos - low)


def rows(path: Path) -> list[dict[str, str]]:
    with path.open(encoding="utf-8-sig", newline="") as stream:
        reader = csv.DictReader(stream)
        if reader.fieldnames is None:
            raise ValueError(f"{path}: missing CSV header")
        return list(reader)


def pct(numerator: int, denominator: int) -> str:
    return f"{100 * numerator / denominator:.2f}%" if denominator else "N/A"


def truthy(value: str) -> bool:
    return value.strip().lower() in {"1", "yes", "true"}


def wilson_interval(successes: int, trials: int) -> str:
    """Two-sided 95% Wilson binomial CI, distinct from the observed trial rate."""
    if trials == 0:
        return "N/A"
    if trials < 0 or successes < 0 or successes > trials:
        raise ValueError("invalid binomial observations")
    z = 1.959963984540054
    p = successes / trials
    denominator = 1 + z*z/trials
    center = (p + z*z/(2*trials))/denominator
    half = z*math.sqrt(p*(1-p)/trials + z*z/(4*trials*trials))/denominator
    lower, upper = max(0.0, center-half), min(1.0, center+half)
    return f"{100*lower:.2f}%–{100*upper:.2f}%"


def clean_agent(value: str) -> str:
    return value.strip().lower()


def matched_crossover_table(folder: Path) -> list[str]:
    """Read measured raw on/off phase CSVs through the strict crossover verifier."""
    source = Path(__file__).with_name("paper-matched-crossover-summary.py")
    spec = importlib.util.spec_from_file_location("paper_matched_crossover", source)
    if spec is None or spec.loader is None:
        raise ValueError("cannot import matched crossover validator")
    validator = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(validator)
    measured = validator.analyze(folder)
    lines = [
        "### Table 6.1 — Matched-daemon randomized read(2) concurrency crossover",
        "| C | Mean paired monitoring increase (%) | 95% paired bootstrap CI (%) | On P95 latency (µs) | Evidence |",
        "|---:|---:|---|---:|---|",
    ]
    for level in CONCURRENCIES:
        cell = measured["by_concurrency"][str(level)]
        low, high = cell["paired_ci95_percent"]
        lines.append(
            f"| {level} | {cell['paired_mean_change_percent']:.2f} | "
            f"[{low:.2f}, {high:.2f}] | {cell['on_p95_us']:.3f} | "
            f"Verified {measured['expected_link_count']} actual eBPF link IDs; "
            f"same-daemon balanced {cell['cycles']} pairs; read(2) only |"
        )
    return lines


def concurrency_table(folder: Path | None, matched_crossover: Path | None = None) -> list[str]:
    if matched_crossover is not None:
        return matched_crossover_table(matched_crossover)
    lines = [
        "### Table 6.1 — Concurrency scalability",
        "| C | Mean overhead (%) | P95 latency (µs) | Evidence |",
        "|---:|---:|---:|---|",
    ]
    for level in CONCURRENCIES:
        overhead, p95, status = "—", "—", "Not measured"
        if folder is not None:
            summary_file = folder / f"summary_c{level}.json"
            if summary_file.exists():
                data = json.loads(summary_file.read_text(encoding="utf-8"))
                if data.get("concurrency") != level:
                    raise ValueError(f"{summary_file}: unexpected concurrency")
                # Legacy script runs BOTH labeled phases with attached eBPF.
                # Never convert its two-run difference into an overhead claim.
                if data.get("comparison_valid") is True and (
                    data.get("baseline_state") == "detached"
                    and data.get("ebpf_state") == "attached"
                ):
                    values = [float(op["overhead_percent"]) for op in data["operations"]]
                    if not values:
                        raise ValueError(f"{summary_file}: no syscall measurements")
                    overhead = f"{statistics.mean(values):.2f}"
                    status = "Verified detached/attached comparison"
                else:
                    status = "Invalid baseline: both phases may be attached"
            latency_file = folder / f"latency_ebpf_c{level}.csv"
            if latency_file.exists():
                samples = [float(row["latency_us"]) for row in rows(latency_file)]
                if any(not math.isfinite(v) or v < 0 for v in samples):
                    raise ValueError(f"{latency_file}: invalid latency")
                if len(samples) < 20:
                    raise ValueError(f"{latency_file}: at least 20 independent samples needed")
                p95 = f"{percentile(samples, 95):.3f}"
                status += "; measured per-call P95"
        lines.append(f"| {level} | {overhead} | {p95} | {status} |")
    return lines


def attribution_table(path: Path | None) -> list[str]:
    lines = [
        "### Table 6.2 — Attribution ablation",
        "| Concurrent agents | Method | Correct | False attribution | Unattributed (⊥) | N |",
        "|---:|---|---:|---:|---:|---:|",
    ]
    evidence = rows(path) if path and path.exists() else []
    seen: set[str] = set()
    for row in evidence:
        required = ("event_id", "agent_count", "command", "truth_agent", *MODES)
        if any(key not in row for key in required):
            raise ValueError(f"{path}: missing required attribution columns")
        if row["event_id"] in seen:
            raise ValueError(f"{path}: repeated event_id {row['event_id']}")
        seen.add(row["event_id"])
        if int(row["agent_count"]) not in (2, 3, 4):
            raise ValueError(f"{path}: agent_count must be 2, 3 or 4")
        if row["command"] not in ("git", "python", "node"):
            raise ValueError(f"{path}: unexpected command {row['command']}")
        if clean_agent(row["truth_agent"]) in UNKNOWN:
            raise ValueError(f"{path}: truth_agent cannot be unknown")
    for agent_count in (2, 3, 4):
        subset = [x for x in evidence if int(x["agent_count"]) == agent_count]
        for mode, label in (
            ("pid_only", "PID only"),
            ("lifecycle_hook_only", "Lifecycle hook only"),
            ("trimodal", "Three modalities"),
        ):
            correct = wrong = unknown = 0
            for row in subset:
                actual, assigned = clean_agent(row["truth_agent"]), clean_agent(row[mode])
                if assigned in UNKNOWN:
                    unknown += 1
                elif assigned == actual:
                    correct += 1
                else:
                    wrong += 1
            n = len(subset)
            if correct + wrong + unknown != n:
                raise AssertionError("attribution categories must partition the samples")
            lines.append(
                f"| {agent_count} | {label} | {pct(correct, n)} | "
                f"{pct(wrong, n)} | {pct(unknown, n)} | {n} |"
            )
    return lines


def enforcement_table(path: Path | None) -> list[str]:
    lines = [
        "### Table 6.3 — Kernel enforcement end-to-end",
        "| Probe | Deterministic deny rate | Pass / trials | Apply P50/P95 (µs) | Revoke P50/P95 (µs) | 95% Wilson CI |",
        "|---|---:|---:|---|---|---|",
    ]
    evidence = rows(path) if path and path.exists() else []
    needed = ("case", "control_ok", "denied", "errno", "restored", "apply_us", "revoke_us")
    for row in evidence:
        if any(key not in row for key in needed):
            raise ValueError(f"{path}: missing required enforcement columns")
        if row["case"] not in CASES:
            raise ValueError(f"{path}: unsupported case {row['case']}")
        for key in ("apply_us", "revoke_us"):
            value = float(row[key])
            if not math.isfinite(value) or value < 0:
                raise ValueError(f"{path}: invalid {key}")
    for case in CASES:
        items = [x for x in evidence if x["case"] == case]
        success = sum(
            truthy(x["control_ok"]) and truthy(x["denied"])
            and x["errno"].strip().upper() in {"1", "13", "EPERM", "EACCES"}
            and truthy(x["restored"])
            for x in items
        )
        n = len(items)
        apply = [float(x["apply_us"]) for x in items]
        revoke = [float(x["revoke_us"]) for x in items]
        def fmt(values: list[float]) -> str:
            return f"{percentile(values, 50):.1f} / {percentile(values, 95):.1f}" if values else "—"
        lines.append(f"| {case} | {pct(success, n)} | {success}/{n} | {fmt(apply)} | {fmt(revoke)} | {wilson_interval(success, n)} |")
    lines.extend([
        "",
        "A successful trial requires a working no-policy control, an actual denied",
        "operation with errno EPERM/EACCES, and success again after revocation.",
        "Apply/revoke columns are API round-trip times, **not** proven kernel",
        "policy propagation time. 100% is reportable only when every recorded",
        "trial satisfies all four conditions; a 100% observed rate is not proof",
        "of universal 100% blocking. The final column uses two-sided 95%",
        "Wilson binomial confidence intervals.",
    ])
    return lines


def build(concurrency: Path | None, attribution: Path | None, enforcement: Path | None,
          matched_crossover: Path | None = None) -> str:
    sections = [
        "# Reproducible research tables",
        "",
        "> Values are populated from captured evidence only. “—” means unavailable;",
        "> 0 trials never imply 0% or 100% effectiveness.",
        "",
        *concurrency_table(concurrency, matched_crossover),
        "",
        *attribution_table(attribution),
        "",
        *enforcement_table(enforcement),
        "",
    ]
    return "\n".join(sections)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--concurrency-dir", type=Path)
    parser.add_argument("--matched-crossover-dir", type=Path,
                        help="Strictly validated raw on/off, same-daemon, kernel-ID crossover")
    parser.add_argument("--attribution-csv", type=Path)
    parser.add_argument("--enforcement-csv", type=Path)
    parser.add_argument("--output", type=Path, default=Path("reports/paper-experiments/tables.md"))
    args = parser.parse_args()
    result = build(args.concurrency_dir, args.attribution_csv, args.enforcement_csv,
                   args.matched_crossover_dir)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(result, encoding="utf-8")
    print(f"Wrote evidence-aware tables to {args.output}")


if __name__ == "__main__":
    main()
