#!/usr/bin/env python3
"""Curate reviewed command-action JSONL without inventing labels from attack traces.

Inputs must contain: command/commandLine, label, scenario_id, run_id, and
label_source=human_reviewed|policy_decision or explicitly opted-in assistant_reviewed. This script does not execute commands.
Output training files retain explicit labels for backend/cmd/ml-train.
"""
import argparse
import collections
import hashlib
import json
from pathlib import Path
import sys


VALID_LABELS = {"ALLOW": "ALLOW", "BLOCK": "BLOCK", "safe": "ALLOW", "unsafe": "BLOCK"}
VALID_LABEL_SOURCES = {"human_reviewed", "policy_decision", "assistant_reviewed"}


def option_pairs(options, flag):
    parsed = {}
    for entry in options:
        key, sep, value = entry.partition("=")
        if not sep or not key.strip() or not value.strip() or key in parsed:
            raise ValueError(f"{flag}: expected unique NAME=VALUE, got {entry!r}")
        parsed[key.strip()] = value.strip()
    return parsed


def read_source(name, path, license_id, allow_assistant_reviewed=False):
    if not license_id:
        raise ValueError(f"{name}: a source license is required")
    source = Path(path)
    if not source.is_file() or source.stat().st_size > 100 * 1024 * 1024:
        raise ValueError(f"{name}: input must be a file no larger than 100 MiB")
    raw = source.read_bytes()
    checksum = hashlib.sha256(raw).hexdigest()
    records = []
    for line_number, line in enumerate(raw.splitlines(), 1):
        if not line.strip():
            continue
        try:
            obj = json.loads(line)
        except (ValueError, UnicodeDecodeError) as exc:
            raise ValueError(f"{name}:{line_number}: invalid JSONL") from exc
        if not isinstance(obj, dict):
            raise ValueError(f"{name}:{line_number}: expected JSON object")
        command = obj.get("commandLine") or obj.get("command")
        label = obj.get("label")
        source_kind = obj.get("label_source")
        scenario = obj.get("scenario_id")
        run_id = obj.get("run_id")
        if not isinstance(command, str) or not command.strip() or "\x00" in command or len(command.encode()) > 4096:
            raise ValueError(f"{name}:{line_number}: missing or invalid command (max 4096 bytes)")
        if not isinstance(label, str) or label not in VALID_LABELS:
            raise ValueError(f"{name}:{line_number}: explicit binary ALLOW/BLOCK action label required")
        if source_kind not in VALID_LABEL_SOURCES:
            raise ValueError(f"{name}:{line_number}: explicit reviewed action provenance required")
        if source_kind == "assistant_reviewed":
            if not allow_assistant_reviewed:
                raise ValueError(f"{name}:{line_number}: assistant-reviewed labels require explicit --allow-ai-reviewed opt-in")
            if obj.get("review_status") != "approved_for_candidate_only":
                raise ValueError(f"{name}:{line_number}: AI labels must be individually approved for candidate-only use")
        if not isinstance(scenario, str) or not scenario.strip():
            raise ValueError(f"{name}:{line_number}: scenario_id is required")
        if not isinstance(run_id, str) or not run_id.strip():
            raise ValueError(f"{name}:{line_number}: run_id is required")
        # Ignore unreviewed metadata and upstream feature vectors: Go re-extracts
        # the canonical 128 runtime features from the reviewed command.
        records.append({
            "command": command.strip(),
            "label": VALID_LABELS[label],
            "dataset": name,
            "scenario_id": scenario.strip(),
            "run_id": run_id.strip(),
            "label_source": source_kind,
            "review_status": obj.get("review_status", ""),
        })
    return records, {"name": name, "file": source.name, "license": license_id, "sha256": checksum, "inputRows": len(records)}


def build_groups(rows, mode):
    """Group across ALL sources; 'both' closes transitive scenario/run leakage.

    A single run may mention multiple scenarios and a scenario may span many
    runs. Connected components are the only sound independent unit if neither
    identifier is allowed to cross validation boundaries. Union roots are
    lexicographically stable, so input order cannot alter membership.
    """
    groups = collections.defaultdict(list)
    if mode != "both":
        field = "run_id" if mode == "run" else "scenario_id"
        kind = "run" if mode == "run" else "scenario"
        for row in rows:
            groups[(kind, row[field])].append(row)
        return groups

    parent = {}

    def find(key):
        parent.setdefault(key, key)
        if parent[key] != key:
            parent[key] = find(parent[key])
        return parent[key]

    for row in rows:
        scenario = ("scenario", row["scenario_id"])
        run = ("run", row["run_id"])
        a, b = find(scenario), find(run)
        if a != b:
            parent[max(a, b)] = min(a, b)

    for row in rows:
        key = find(("scenario", row["scenario_id"]))
        groups[key].append(row)
    return groups


def split_rows(rows, group_by, seed, validation_fraction, test_fraction, min_per_class):
    if not 0 < validation_fraction < 0.5 or not 0 < test_fraction < 0.5 or validation_fraction + test_fraction >= 0.8:
        raise ValueError("validation/test fractions must be >0 and <0.5, with sum <0.8")
    groups = build_groups(rows, group_by)
    command_labels = {}
    for row in rows:
        command = row["command"]
        label = row["label"]
        previous = command_labels.setdefault(command, label)
        if previous != label:
            raise ValueError("conflicting action labels for the same command across sources or runs")
    if len(groups) < 3:
        raise ValueError("at least three independent scenarios/runs are required for train/validation/test")
    # Both held-out partitions need independent evidence of ALLOW and BLOCK.
    # Without it, security precision/recall are undefined, not zero.
    group_support = collections.Counter(
        action for members in groups.values()
        for action in {row["label"] for row in members}
    )
    for action in ("ALLOW", "BLOCK"):
        if group_support[action] < 3:
            raise ValueError(
                f"at least three independent groups containing {action} are required "
                "for train/validation/test"
            )

    n_test = max(1, round(len(groups) * test_fraction))
    n_val = max(1, round(len(groups) * validation_fraction))
    if n_test + n_val >= len(groups):
        raise ValueError("not enough independent groups for training")

    # Deterministic group-level stratification uses ground-truth labels only.
    # It never uses model predictions or test accuracy to choose a partition.
    separator = chr(31)
    for attempt in range(128):
        salt = str(seed) if attempt == 0 else f"{seed}{separator}{attempt}"
        ranked = sorted(groups, key=lambda key: (
            hashlib.sha256((salt + separator + separator.join(key)).encode()).digest(), key
        ))
        by_split = {
            "test": ranked[:n_test],
            "validation": ranked[n_test:n_test+n_val],
            "train": ranked[n_test+n_val:],
        }
        retained_commands = set()
        outputs = {}
        excluded = {}
        # Prefer training ownership of a command repeated across groups.
        for split in ("train", "validation", "test"):
            selected = []
            dropped = 0
            for group in sorted(by_split[split]):
                for row in groups[group]:
                    if row["command"] in retained_commands:
                        dropped += 1
                        continue
                    retained_commands.add(row["command"])
                    selected.append(row)
            outputs[split] = sorted(selected, key=lambda row: (
                row["dataset"], row["scenario_id"], row["run_id"], row["command"]))
            excluded[split] = dropped
        counts = {
            split: collections.Counter(row["label"] for row in selected)
            for split, selected in outputs.items()
        }
        if all(
            all(counts[split][action] >= (min_per_class if split == "train" else 1)
                for action in ("ALLOW", "BLOCK"))
            for split in ("train", "validation", "test")
        ):
            return outputs, by_split, excluded
    raise ValueError(
        "cannot build ALLOW/BLOCK-supported independent train/validation/test "
        "partitions after 128 deterministic group-stratification attempts"
    )

def build_corpus(sources, licenses, output_dir, group_by="both", seed=20261103,
                 validation_fraction=0.2, test_fraction=0.2, min_per_class=20,
                 allow_assistant_reviewed=False):
    if not sources or set(sources) != set(licenses):
        raise ValueError("provide a matching --license NAME=LICENSE for every --source NAME=FILE")
    all_rows, provenance = [], []
    for name in sorted(sources):
        rows, entry = read_source(name, sources[name], licenses[name], allow_assistant_reviewed)
        all_rows.extend(rows)
        provenance.append(entry)
    splits, grouped, excluded = split_rows(
        all_rows, group_by, seed, validation_fraction, test_fraction, min_per_class)
    out = Path(output_dir)
    out.mkdir(parents=True, exist_ok=True)
    manifest = {
        "schema": "agent-ebpf-reviewed-corpus.v1",
        "candidateOnly": True,
        "groupBy": group_by,
        "seed": seed,
        "sourceProvenance": provenance,
        "assistantReviewedAccepted": allow_assistant_reviewed,
        "split": {},
        "warning": "Labels require review; attack-window membership is not an action label. Holdout group IDs are not proof of independent real-world sessions.",
    }
    for split, rows in splits.items():
        payload = "".join(json.dumps(row, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n" for row in rows)
        raw = payload.encode("utf-8")
        (out / f"{split}.jsonl").write_bytes(raw)
        manifest["split"][split] = {
            "rows": len(rows),
            "groups": len(grouped[split]),
            "deduplicatedOrPurgedRows": excluded[split],
            "byAction": dict(sorted(collections.Counter(row["label"] for row in rows).items())),
            "sha256": hashlib.sha256(raw).hexdigest(),
        }
    (out / "manifest.json").write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8")
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", action="append", required=True, help="NAME=path/to/reviewed.jsonl; repeatable")
    parser.add_argument("--license", action="append", required=True, help="NAME=SPDX-or-source-license; repeatable")
    parser.add_argument("--output-dir", required=True)
    parser.add_argument("--group-by", choices=("both", "scenario", "run"), default="both",
                        help="both (default): transitive scenario/run disjointness; scenario or run: weaker single-key isolation")
    parser.add_argument("--seed", type=int, default=20261103)
    parser.add_argument("--validation-fraction", type=float, default=0.2)
    parser.add_argument("--test-fraction", type=float, default=0.2)
    parser.add_argument("--min-per-class", type=int, default=20)
    parser.add_argument("--allow-ai-reviewed", action="store_true",
                        help="opt in to assistant_reviewed labels as candidate training data; never count these as human ground truth")
    args = parser.parse_args()
    try:
        if args.min_per_class < 1:
            raise ValueError("--min-per-class must be >=1")
        result = build_corpus(
            option_pairs(args.source, "--source"), option_pairs(args.license, "--license"),
            args.output_dir, args.group_by, args.seed,
            args.validation_fraction, args.test_fraction, args.min_per_class,
            args.allow_ai_reviewed)
    except (ValueError, OSError) as exc:
        parser.exit(2, f"corpus rejected: {exc}\n")
    print(json.dumps(result["split"], ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
