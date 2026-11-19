#!/usr/bin/env python3
"""Join *observed* backend kernel event predictions against known Agent child PIDs.

Consumes per-cohort real /events/summaries snapshots and actual subprocess
ground truth. Reports sensor coverage separately from attribution correctness.
Does NOT synthesize a PID-only or lifecycle-only ablation from combined output.
"""
from __future__ import annotations

import argparse
import csv
import json
from collections import defaultdict
from pathlib import Path

HEADER = ("event_id", "agent_count", "cycle", "command", "truth_agent",
          "predicted_agent", "pid", "event_type", "received_at_ms")
TOOL_NAMES = {"git": "git", "python": "python3", "node": "node"}


def load_truth(path: Path) -> dict[tuple[int, int], list[dict]]:
    entries: dict[tuple[int, int], list[dict]] = defaultdict(list)
    with path.open(encoding="utf-8-sig", newline="") as file:
        for row in csv.DictReader(file):
            if any(key not in row for key in
                   ("agent_count", "cycle", "child_pid", "started_ns", "finished_ns",
                    "agent_id", "command", "returncode")):
                raise ValueError(f"{path}: incomplete ground truth header")
            count, cycle = int(row["agent_count"]), int(row["cycle"])
            if count not in (2, 3, 4) or cycle < 1:
                raise ValueError(f"{path}: invalid cohort")
            if row["command"] not in TOOL_NAMES or int(row["returncode"]) != 0:
                raise ValueError(f"{path}: invalid/failed tool invocation")
            if int(row["finished_ns"]) < int(row["started_ns"]):
                raise ValueError(f"{path}: reversed process lifetime")
            entries[(count, cycle)].append(row)
    return entries


def join_live_events(truth: dict, snapshots: Path, skew_before_ms: int = 50,
                     skew_after_ms: int = 1500):
    """
    Return (labeled kernel events, diagnostics).

    "No event" is missing sensor evidence; it is NOT a model abstention.
    A single event must have one unambiguous PID/time owner.
    """
    emitted: dict[str, dict] = {}
    diagnostics = defaultdict(int)
    required = set(truth)
    seen_cohorts = set()
    seen_processes = set()
    for snapshot in sorted(snapshots.glob("events_agents*_cycle*.json")):
        payload = json.loads(snapshot.read_text(encoding="utf-8"))
        cohort = (int(payload["agent_count"]), int(payload["cycle"]))
        if cohort in seen_cohorts:
            raise ValueError(f"duplicate event snapshot for cohort {cohort}")
        seen_cohorts.add(cohort)
        candidates = truth.get(cohort)
        if not candidates:
            raise ValueError(f"{snapshot}: snapshot lacks ground-truth cohort")
        events = payload.get("events")
        if not isinstance(events, list):
            raise ValueError(f"{snapshot}: invalid events array")
        diagnostics["snapshot_event_rows"] += len(events)
        for event in events:
            event_id = str(event.get("eventId") or "").strip()
            if not event_id:
                diagnostics["missing_event_ids"] += 1
                continue
            try:
                pid = int(event.get("pid") or 0)
                received = int(event.get("receivedAtMs") or 0)
            except (TypeError, ValueError):
                diagnostics["invalid_pid_or_time"] += 1
                continue
            # The event source is a backend receive timestamp, not the
            # kernel syscall timestamp. Use bounded allowance only.
            comm = str(event.get("comm") or "").strip()
            ppid = int(event.get("ppid") or 0)
            matched = [
                row for row in candidates
                if int(row["child_pid"]) == pid
                # The backend privacy layer can redact process names. PID,
                # process lifetime and an observed PPID are independent ground
                # truth; never read the MODEL's agentRunId to choose an owner.
                and (comm == TOOL_NAMES[row["command"]] or comm == "[REDACTED]")
                and (not ppid or not row.get("root_pid")
                     or int(row["root_pid"]) == ppid)
                and int(row["started_ns"]) // 1_000_000 - skew_before_ms <= received
                <= int(row["finished_ns"]) // 1_000_000 + skew_after_ms
            ]
            if len(matched) != 1:
                diagnostics["ambiguous_events" if matched else "unmatched_events"] += 1
                continue
            owner = matched[0]
            record = {
                "event_id": event_id,
                "agent_count": cohort[0], "cycle": cohort[1],
                "command": owner["command"],
                "truth_agent": owner["agent_id"],
                "predicted_agent": str(event.get("agentRunId") or "").strip() or "⊥",
                "pid": pid,
                "event_type": str(event.get("type") or ""),
                "received_at_ms": received,
            }
            # The same backend record can appear in multiple snapshots.
            # It must never acquire a different owner or prediction.
            if event_id in emitted and emitted[event_id] != record:
                raise ValueError(f"conflicting duplicated eventId: {event_id}")
            emitted[event_id] = record
            seen_processes.add((cohort, owner["child_pid"], owner["agent_id"]))
    diagnostics["expected_cohorts"] = len(required)
    diagnostics["snapshots_found"] = len(seen_cohorts)
    diagnostics["missing_cohorts"] = len(required - seen_cohorts)
    diagnostics["ground_truth_tools"] = sum(len(rows) for rows in truth.values())
    diagnostics["tools_with_matched_events"] = len(seen_processes)
    diagnostics["tools_without_matched_events"] = (
        diagnostics["ground_truth_tools"] - len(seen_processes)
    )
    diagnostics["unique_observed_events"] = len(emitted)
    return sorted(emitted.values(), key=lambda r: (r["agent_count"], r["cycle"], r["pid"], r["event_id"])), dict(diagnostics)


def rates(records):
    result = {}
    for count in (2, 3, 4):
        cohort = [row for row in records if row["agent_count"] == count]
        correct = sum(row["predicted_agent"] == row["truth_agent"] for row in cohort)
        unknown = sum(row["predicted_agent"] == "⊥" for row in cohort)
        wrong = len(cohort) - correct - unknown
        result[count] = {"events": len(cohort), "correct": correct,
                         "false_attribution": wrong, "unattributed": unknown}
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--ground-truth", type=Path, required=True)
    parser.add_argument("--events-dir", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--mode", choices=("combined", "pid", "lifecycle"), default="combined")
    args = parser.parse_args()
    truth = load_truth(args.ground_truth)
    records, diagnostic = join_live_events(truth, args.events_dir)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    with (args.output_dir / f"{args.mode}-events.csv").open("w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=HEADER)
        writer.writeheader()
        writer.writerows(records)
    summary = {"mode": args.mode, "data_kind": "observed_backend_event_predictions",
               "diagnostics": diagnostic, "by_concurrency": rates(records)}
    (args.output_dir / f"{args.mode}-summary.json").write_text(
        json.dumps(summary, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    print(json.dumps(summary, indent=2, ensure_ascii=False))
    if diagnostic["missing_cohorts"]:
        raise SystemExit("INCOMPLETE: missing per-cohort event captures; no publication-grade result")
    if not records:
        raise SystemExit("INCOMPLETE: zero observed kernel events matched actual child lifetimes")


if __name__ == "__main__":
    main()
