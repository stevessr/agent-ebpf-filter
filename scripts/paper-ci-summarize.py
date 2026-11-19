#!/usr/bin/env python3
"""Summarize raw CI-host files; do not conflate off/on means with causal eBPF overhead."""
import csv
import json
import math
import pathlib
import statistics
import subprocess

root = pathlib.Path("reports/paper-ci")
root.mkdir(parents=True, exist_ok=True)
sha = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
lines = [
    "# Exploratory real-host measurements",
    "",
    f"Commit: \`{sha}\`",
    "",
    "Host manifest: \`reports/paper-ci/host.txt\`",
    "",
    "| C | Off samples | On samples | Off read mean (µs) | On read mean (µs) | On P95 (µs) | Observed difference (%) |",
    "|---:|---:|---:|---:|---:|---:|---:|",
]


def samples(mode, concurrency):
    all_values = []
    files = sorted((root / mode).glob(f"read_c{concurrency}-r*.csv"))
    for path in files:
        with path.open(encoding="utf-8", newline="") as source:
            all_values.extend(float(row["latency_us"]) for row in csv.DictReader(source))
    return all_values, len(files)


for concurrency in (1, 4, 8, 16, 32):
    off, off_runs = samples("off", concurrency)
    on, on_runs = samples("on", concurrency)
    if not off or not on or off_runs != 10 or on_runs != 10:
        lines.append(f"| {concurrency} | {len(off)} | {len(on)} | — | — | — | — |")
        continue
    if any(not math.isfinite(x) or x < 0 for x in off + on):
        raise ValueError("nonfinite/negative samples")
    a, b = statistics.mean(off), statistics.mean(on)
    values = sorted(on)
    idx = .95 * (len(values) - 1)
    p95 = values[math.floor(idx)] * (1 - idx % 1) + values[math.ceil(idx)] * (idx % 1)
    lines.append(
        f"| {concurrency} | {len(off)} | {len(on)} | {a:.4f} | {b:.4f} | {p95:.4f} | {(b/a - 1)*100:+.2f} |"
    )

lines += [
    "",
    "**Interpretation:** real read(2) timing on a single GitHub runner,",
    "10 repetitions × 20,000 calls in each state. The uninstrumented control",
    "ran before startup, while the attached state ran with the backend active.",
    "This is not a randomized or daemon-load-matched experiment;",
    "the difference is *not established causal eBPF overhead*.",
    "P95 applies to read(2), not to all Agent syscalls or workflow latency.",
    "",
]
p = root / "health.json"
if p.exists():
    data = json.loads(p.read_text())
    lines.append(
        f"Verified bootstrap attachedCount={data.get('attachedCount', 'missing')}, "
        f"compiledCount={data.get('compiledCount', 'missing')}, "
        f"skippedCount={data.get('skippedCount', 'missing')}."
    )
else:
    lines.append("Project eBPF attachment status: NOT VERIFIED.")

p = root / "context/ground-truth.csv"
if p.exists():
    with p.open(encoding="utf-8", newline="") as source:
        entries = list(csv.DictReader(source))
    lines.append(f"\nRegistered Agent same-command process lifetimes: **{len(entries)}**.")
    for n in (2, 3, 4):
        count = sum(int(row["agent_count"]) == n for row in entries)
        lines.append(f"- {n} Agent roots: {count} tool-process records.")
    lines.append("No isolated ablation-mode predictions were captured; correctness/false/⊥ rates remain unmeasured.")
else:
    lines.append("\nAgent run registration evidence not available; inspect context/workload.log.")

p = root / "enforcement/enforcement.csv"
if p.exists():
    with p.open(encoding="utf-8", newline="") as source:
        events = list(csv.DictReader(source))
    lines += ["", "| Kernel-deny probe | Verified EPERM/EACCES + both controls | All recorded trials |",
              "|---|---:|---:|"]
    from collections import Counter
    types = sorted(set(row["case"] for row in events))
    for name in types:
        cohort = [row for row in events if row["case"] == name]
        success = sum(
            row["control_ok"] == "true" and row["denied"] == "true"
            and row["errno"] in ("1", "13") and row["restored"] == "true"
            for row in cohort
        )
        lines.append(f"| {name} | {success} | {len(cohort)} |")
    lines.append("This includes only completed rows; a crashed/incomplete test must be identified from trials.log.")
else:
    lines.append("\nLive BPF LSM/cgroup denial evidence: **not available** (skip or failure recorded separately).")

text = "\n".join(lines) + "\n"
(root / "summary.md").write_text(text, encoding="utf-8")
print(text)
