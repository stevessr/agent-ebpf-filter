"""Cross-fit low-FPR evaluation for model selection, never production certification.

Calibration uses the other validation fold, with whole scenario groups isolated.
Independent final calibration and consumed regression data are never used here.
"""
from collections import defaultdict
import hashlib
import math

from decision_protocol import confusion, group_bootstrap_recall, operating_threshold, scenario_key


def group_folds(rows, seed=20261021):
    """Deterministic and size-balanced scenario-disjoint two-fold partition."""
    if not isinstance(rows, list) or len(rows) < 4:
        raise ValueError("need at least four validation examples")
    groups = defaultdict(list)
    for i, row in enumerate(rows):
        g = row.get("group")
        if not isinstance(g, str) or not g:
            raise ValueError("validation row missing group")
        if type(row.get("label")) is not int or row["label"] not in (0, 1):
            raise ValueError("validation row missing binary label")
        groups[scenario_key(row)].append(i)
    if len(groups) < 4:
        raise ValueError("need at least four independent scenario groups")
    order = sorted(groups, key=lambda g: (
        -len(groups[g]), hashlib.sha256(repr((seed, g)).encode()).hexdigest()
    ))
    folds = [[], []]
    for g in order:
        chosen = min(range(2), key=lambda i: (len(folds[i]), i))
        folds[chosen].extend(groups[g])
    # Preserve historic partitions whenever both folds already contain both
    # labels. Only repair a failing assignment; never split a scenario.
    if all({rows[i]["label"] for i in fold} == {0, 1} for fold in folds):
        return [sorted(fold) for fold in folds]

    # Size balancing alone can put all positives into one fold and all
    # negatives into the other, even though a valid two-fold split exists.
    # Anchor the rare label coverage first, then greedily balance leftovers.
    mixed, positive_only, negative_only = [], [], []
    for key in order:
        labels = {rows[i]["label"] for i in groups[key]}
        if labels == {0, 1}:
            mixed.append(key)
        elif labels == {1}:
            positive_only.append(key)
        else:
            negative_only.append(key)
    if len(mixed) >= 2:
        anchors = [[mixed[0]], [mixed[1]]]
    elif len(mixed) == 1 and positive_only and negative_only:
        anchors = [[mixed[0]], [positive_only[0], negative_only[0]]]
    elif not mixed and len(positive_only) >= 2 and len(negative_only) >= 2:
        anchors = [[positive_only[0], negative_only[0]],
                   [positive_only[1], negative_only[1]]]
    else:
        raise ValueError("two folds with both labels require independent positive and negative scenarios")

    seeded = set(anchors[0] + anchors[1])
    folds = [[i for key in anchor for i in groups[key]] for anchor in anchors]
    for key in order:
        if key in seeded:
            continue
        chosen = min(range(2), key=lambda i: (len(folds[i]), i))
        folds[chosen].extend(groups[key])
    return [sorted(fold) for fold in folds]


def crossfit_validation(rows, scores, *, target_fpr=0.01, seed=20261021):
    """Out-of-fold threshold decisions, never fitted on the scored group."""
    if len(rows) != len(scores):
        raise ValueError("validation length mismatch")
    if type(target_fpr) not in (int, float) or not math.isfinite(target_fpr) or not 0 <= target_fpr < 1:
        raise ValueError("invalid target_fpr")
    if any(type(s) not in (int, float) or not math.isfinite(s) for s in scores):
        raise ValueError("non-finite score")
    folds = group_folds(rows, seed)
    decisions = [None] * len(rows)
    reports = []
    for k, evaluation in enumerate(folds):
        threshold_indices = folds[1-k]
        calibration_y = [rows[i]["label"] for i in threshold_indices]
        evaluation_y = [rows[i]["label"] for i in evaluation]
        if not 0 < sum(calibration_y) < len(calibration_y):
            raise ValueError("each threshold fold needs both labels")
        if not 0 < sum(evaluation_y) < len(evaluation_y):
            raise ValueError("each evaluation fold needs both labels")
        op = operating_threshold(calibration_y, [scores[i] for i in threshold_indices], target_fpr)
        for i in evaluation:
            decisions[i] = int(scores[i] > op["threshold"])
        reports.append(dict(fold=k, threshold=op["threshold"],
                            metrics=confusion(evaluation_y, [scores[i] for i in evaluation], op["threshold"]),
                            threshold_examples=len(threshold_indices),
                            evaluated_examples=len(evaluation),
                            threshold_negative_count=op["negatives"],
                            evaluated_negative_count=evaluation_y.count(0)))
    if any(d is None for d in decisions):
        raise AssertionError("missing out-of-fold decision")
    stats = confusion([r["label"] for r in rows], decisions, 0.5)
    stats["cluster_recall95"] = group_bootstrap_recall(
        rows, decisions, 0.5, replicates=400, seed=seed
    )
    stats["folds"] = reports
    stats["target_fpr"] = target_fpr
    stats["measured_out_of_fold"] = True
    stats["proven_independent"] = False
    return stats

