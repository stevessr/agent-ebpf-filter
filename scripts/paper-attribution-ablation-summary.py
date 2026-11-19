#!/usr/bin/env python3
"""Compare measured research backends, never retroactively mask a combined trace."""
import argparse
import json
from pathlib import Path

MODES = ("pid", "lifecycle", "combined")
COUNTS = (2, 3, 4)


def report(folder: Path) -> str:
    lines = [
        "# Independent real-backend attribution ablation",
        "",
        "| Concurrent Agent roots | Mode | Observed events | Correct | False attribution | Unattributed (⊥) | Tool-process event coverage |",
        "|---:|---|---:|---:|---:|---:|---:|",
    ]
    for mode in MODES:
        path = folder / mode / "results" / f"{mode}-summary.json"
        if not path.exists():
            raise ValueError(f"missing measured mode: {path}")
        data = json.loads(path.read_text(encoding="utf-8"))
        diag = data["diagnostics"]
        if data.get("data_kind") != "observed_backend_event_predictions":
            raise ValueError(f"{path}: not captured backend predictions")
        if diag["expected_cohorts"] != 30 or diag["missing_cohorts"] != 0:
            raise ValueError(f"{path}: incomplete 2-4 Agent cohorts")
        if diag["ground_truth_tools"] != 270:
            raise ValueError(f"{path}: ground truth must cover 270 subprocesses")
        if diag["unique_observed_events"] < 1:
            raise ValueError(f"{path}: zero matched events; no rates can be claimed")
        for count in COUNTS:
            cell = data["by_concurrency"][str(count)]
            n = cell["events"]
            correct = cell["correct"]
            false = cell["false_attribution"]
            unknown = cell["unattributed"]
            if correct + false + unknown != n:
                raise ValueError(f"{path}: event classification not a partition")
            # Per-agent-mode sensor coverage is measured separately; do not
            # treat a missing kernel observation as a model-level abstention.
            expected = count * 10 * 3
            actual = sum(
                1 for item in read_records(folder / mode / "results" / f"{mode}-events.csv")
                if int(item["agent_count"]) == count
            )
            if actual != n:
                raise ValueError(f"{path}: row counts disagree with actual raw events")
            def fraction(a):
                return f"{100*a/n:.2f}%" if n else "N/A"
            lines.append(
                f"| {count} | {mode} | {n} | {fraction(correct)} | "
                f"{fraction(false)} | {fraction(unknown)} | "
                f"See per-mode ground truth ({expected} tool processes) |"
            )
    lines += [
        "",
        "These are separate executions of actual kernel event pipelines under",
        "controlled research modes (not three predictions on the same event).",
        "Denominators may differ because kernel capture coverage differs.",
        "The tested workload uses explicit Agent root registration, process",
        "lifecycle, and shared-cgroup evidence. It does NOT inject genuine SDK",
        "tool semantic hooks; therefore it cannot establish an incremental",
        "benefit of a third native-hook semantic modality. A separate real",
        "Agent framework workload is required for that claim.",
    ]
    return "\n".join(lines) + "\n"


def read_records(path: Path) -> list[dict]:
    import csv
    with path.open(encoding="utf-8", newline="") as stream:
        return list(csv.DictReader(stream))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--folder", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    text = report(args.folder)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(text, encoding="utf-8")
    print(text)


if __name__ == "__main__":
    main()
