"""Frozen-threshold source/tag diagnostics; never retune an operating point."""
from collections import defaultdict

from decision_protocol import confusion, scenario_key
from tag_protocol import normalize_rows, TAG_VERSION


def error_slices(rows, scores, threshold):
    if not rows or len(rows) != len(scores):
        raise ValueError("missing or unmatched diagnostic rows")
    normalized = normalize_rows(rows)
    # Validate every score and label before constructing any subgroups.
    overall = confusion([r.get("label") for r in normalized], scores, threshold)
    indices = {"by_source": defaultdict(list), "by_tag": defaultdict(list)}
    for i, row in enumerate(normalized):
        scenario_key(row)  # also validates group/source identities
        indices["by_source"][row.get("source", "")].append(i)
        for tag in row["tags"]:
            indices["by_tag"][tag].append(i)
    report = dict(tag_version=TAG_VERSION, overall=overall,
                  comparison="strict >", threshold=threshold,
                  overlapping_tag_slices=True)
    for axis, groups in indices.items():
        report[axis] = []
        for name, members in sorted(groups.items()):
            metric = confusion([normalized[i]["label"] for i in members],
                               [scores[i] for i in members], threshold)
            report[axis].append(dict(key=name, rows=len(members),
                scenarios=len({scenario_key(normalized[i]) for i in members}),
                **metric))
    return report
