"""Frozen-threshold source/tag diagnostics; never retune an operating point."""
from collections import defaultdict

from decision_protocol import confusion, scenario_key, upper_binomial95, lower_binomial95
from tag_protocol import normalize_rows, TAG_VERSION


def _scenario_metrics(rows, scores, threshold):
    """All positive actions must be found for a scenario to count as detected.

    Row labels/scores are validated by the caller. This conservative unit avoids
    certifying recall from repeated candidates in one positive scenario.
    """
    positive, missed, negative, bad = set(), set(), set(), set()
    for row, score in zip(rows, scores):
        key = scenario_key(row)
        if row['label'] == 1:
            positive.add(key)
            if score <= threshold:
                missed.add(key)
        else:
            negative.add(key)
            if score > threshold:
                bad.add(key)
    found = len(positive) - len(missed)
    return dict(negative_scenarios=len(negative), false_positive_scenarios=len(bad),
                scenario_fpr=len(bad) / len(negative) if negative else None,
                scenario_fpr_upper95=upper_binomial95(len(bad), len(negative)) if negative else None,
                positive_scenarios=len(positive), fully_detected_positive_scenarios=found,
                scenario_recall=found / len(positive) if positive else None,
                scenario_recall_lower95=lower_binomial95(found, len(positive)) if positive else None)


def error_slices(rows, scores, threshold):
    if not rows or len(rows) != len(scores):
        raise ValueError("missing or unmatched diagnostic rows")
    normalized = normalize_rows(rows)
    # Validate every score and label before constructing any subgroups.
    overall = confusion([r.get("label") for r in normalized], scores, threshold)
    overall.update(_scenario_metrics(normalized, scores, threshold))
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
            metric.update(_scenario_metrics([normalized[i] for i in members],
                                            [scores[i] for i in members], threshold))
            report[axis].append(dict(key=name, rows=len(members),
                scenarios=len({scenario_key(normalized[i]) for i in members}),
                **metric))
    return report
