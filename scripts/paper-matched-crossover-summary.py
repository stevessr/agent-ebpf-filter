#!/usr/bin/env python3
"""Measured paired crossover inference with explicit kernel link-state evidence.

Difference estimates are per-read *total monitored backend cost*, not a
per-BPF-program instruction cost. Bootstrapped CIs resample randomized
on/off *pairs*, not millions of correlated syscall samples.
"""
from __future__ import annotations

import argparse
import csv
import json
import math
from pathlib import Path
import random
import statistics

CONCURRENCIES = (1, 4, 8, 16, 32)


def quantile(values: list[float], fraction: float) -> float:
    if not 0 <= fraction <= 1 or not values:
        raise ValueError("invalid percentile input")
    sorted_values = sorted(values)
    position = (len(sorted_values) - 1) * fraction
    left, right = math.floor(position), math.ceil(position)
    return sorted_values[left] + (sorted_values[right] - sorted_values[left]) * (position - left)


def pair_bootstrap_ci(changes: list[float], seed: int, rounds: int = 5000) -> tuple[float, float]:
    if len(changes) < 2:
        raise ValueError("at least two independent paired cycles required")
    rng = random.Random(seed)
    estimates = [statistics.mean(rng.choices(changes, k=len(changes))) for _ in range(rounds)]
    return quantile(estimates, .025), quantile(estimates, .975)


def read_samples(path: Path, expected: int) -> list[float]:
    if not path.is_file():
        raise ValueError(f"missing measured sample file: {path}")
    with path.open(encoding="utf-8", newline="") as file:
        reader = csv.DictReader(file)
        if reader.fieldnames != ["latency_us"]:
            raise ValueError(f"{path}: unexpected CSV schema")
        data = [float(row["latency_us"]) for row in reader]
    if len(data) != expected or any(not math.isfinite(x) or x < 0 for x in data):
        raise ValueError(f"{path}: invalid sample count or latency values")
    return data


def analyze(directory: Path) -> dict:
    raw = json.loads((directory / "manifest.json").read_text(encoding="utf-8"))
    if raw.get("kind") != "matched_daemon_real_kernel_link_crossover":
        raise ValueError("not a controlled crossover trial manifest")
    if raw.get("restored_tracepoint_links") is not True:
        raise ValueError("kernel tracing not verified restored after experiment")
    cycles = raw["cycles_per_concurrency"]
    samples = raw["samples_per_phase"]
    nlinks = raw["expected_link_count"]
    if cycles < 2 or cycles % 2 or samples < 1000 or nlinks < 100:
        raise ValueError("invalid experimental manifest setup")
    if len(raw["phases"]) != len(CONCURRENCIES) * cycles * 2:
        raise ValueError("missing or duplicate experiment phase records")
    cohort: dict[tuple[int, int], dict[str, list[float]]] = {}
    names = set()
    for phase in raw["phases"]:
        c, pair, mode = int(phase["concurrency"]), int(phase["pair"]), phase["mode"]
        if c not in CONCURRENCIES or not 1 <= pair <= cycles or mode not in ("off", "on"):
            raise ValueError("unsupported crossover phase")
        if phase.get("actual_kernel_link_ids_verified") is not True or phase.get("backend_healthy") is not True:
            raise ValueError("no actual kernel or backend state evidence")
        expected_links = nlinks if mode == "on" else 0
        if phase["kernel_tracepoint_links"] != expected_links:
            raise ValueError("kernel link state not correctly switched")
        key = (c, pair)
        if mode in cohort.get(key, {}):
            raise ValueError(f"duplicate mode in crossover pair: {key}, {mode}")
        name = phase["file"]
        if (not isinstance(name, str) or Path(name).name != name or
                not name.startswith(f"c{c}-pair{pair}-") or
                not name.endswith(f"-{mode}.csv") or name in names):
            raise ValueError("unsafe/duplicated measurement filename")
        names.add(name)
        cohort.setdefault(key, {})[mode] = read_samples(directory / name, samples)
    result = {"kind": raw["kind"], "kernel": raw["kernel"],
              "sha": raw["tested_sha"], "seed": raw["seed"],
              "cycles_per_concurrency": cycles,
              "samples_per_phase": samples,
              "expected_link_count": nlinks,
              "by_concurrency": {}}
    for c in CONCURRENCIES:
        changes, on, off = [], [], []
        for pair in range(1, cycles + 1):
            modes = cohort.get((c, pair), {})
            if set(modes) != {"off", "on"}:
                raise ValueError(f"C={c} pair={pair}: missing genuine matched control")
            a, b = statistics.mean(modes["off"]), statistics.mean(modes["on"])
            if a <= 0:
                raise ValueError(f"C={c} pair={pair}: zero baseline latency")
            changes.append((b / a - 1) * 100)
            off.extend(modes["off"])
            on.extend(modes["on"])
        lower, upper = pair_bootstrap_ci(changes, int(raw["seed"]) + c)
        result["by_concurrency"][str(c)] = {
            "cycles": cycles, "off_samples": len(off), "on_samples": len(on),
            "off_mean_us": statistics.mean(off), "on_mean_us": statistics.mean(on),
            "on_p95_us": quantile(on, .95),
            "paired_mean_change_percent": statistics.mean(changes),
            "paired_ci95_percent": [lower, upper],
            "all_pairs": changes,
        }
    return result


def report(data: dict) -> str:
    lines = [
        "# Real kernel-link state matched-daemon randomized crossover",
        "",
        f"Commit: `{data['sha']}`; kernel: `{data['kernel']}`; "
        f"seed: {data['seed']}; verified tracepoint links: {data['expected_link_count']}.",
        "",
        "| C | Off mean read(2) µs | On mean read(2) µs | On P95 µs | Paired mean change | Paired bootstrap 95% CI |",
        "|---:|---:|---:|---:|---:|---|",
    ]
    for c in CONCURRENCIES:
        p = data["by_concurrency"][str(c)]
        lo, hi = p["paired_ci95_percent"]
        lines.append(
            f"| {c} | {p['off_mean_us']:.4f} | {p['on_mean_us']:.4f} | "
            f"{p['on_p95_us']:.4f} | {p['paired_mean_change_percent']:+.2f}% | "
            f"[{lo:+.2f}%, {hi:+.2f}%] |"
        )
    lines += [
        "",
        "Each level includes randomized AB/BA ordered pairs with the SAME",
        "running user-space backend, verified BPF tracepoint link ID removal",
        "and reattachment, and independent real read(2) samples.",
        "The 95% percentile bootstrap resamples *paired cycles* (n="
        f"{data['cycles_per_concurrency']}) rather than individual correlated calls.",
        "",
        "**Interpretation:** This isolates the effect of enabling the project's",
        "kernel tracepoint instrumentation + event processing at the tested",
        "host/load, *not* raw kernel eBPF instruction runtime, and not all",
        "syscall types or full Agent workflow cost. Differences may still",
        "include order/caching, pin/rebootstrap side effects and CI CPU",
        "frequency variation. P95 is a per-read timer observation.",
    ]
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--folder", type=Path, required=True)
    args = parser.parse_args()
    data = analyze(args.folder)
    (args.folder / "summary.json").write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    (args.folder / "summary.md").write_text(report(data), encoding="utf-8")
    print(report(data))


if __name__ == "__main__":
    main()
