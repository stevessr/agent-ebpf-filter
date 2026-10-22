"""Bounded, reproducible AutoML selection and independent shadow-review gates.

Only PrivacyAlign action labels are accepted. A passing statistical gate is a
research triage signal, never authorization to install a model or block calls.
"""
from itertools import product
import math

from decision_protocol import confusion, upper_binomial95, scenario_key
from error_analysis import error_slices


DEFAULT_TRIALS = [
    dict(name="mini_pair_baseline", learning_rate=3e-5, pair_weight=0.2,
         hard_negative_weight=1.0, positive_weight=1.0, pair_strategy="all"),
    dict(name="mini_hard_negative", learning_rate=3e-5, pair_weight=0.2,
         hard_negative_weight=3.0, positive_weight=1.0, pair_strategy="all"),
]

# Two additional controlled objectives, available only on explicit request.
QUALITY_TRIALS = DEFAULT_TRIALS + [
    dict(name="mini_hardest_pair", learning_rate=3e-5, pair_weight=0.4,
         hard_negative_weight=3.0, positive_weight=1.0, pair_strategy="hardest"),
    dict(name="mini_missed_positive", learning_rate=3e-5, pair_weight=0.3,
         hard_negative_weight=3.0, positive_weight=2.0, pair_strategy="hardest"),
]


def _number(value, minimum, maximum, label):
    if type(value) not in (float, int) or not math.isfinite(value):
        raise ValueError("invalid " + label)
    value = float(value)
    if not minimum <= value <= maximum:
        raise ValueError("out of range " + label)
    return value


def trial_plan(space=None, *, max_trials=12, quality_search=False):
    """Explicit finite grid; refuse budget overruns rather than truncate trials.

    Baseline and 3x hard-negative candidates share all other hyperparameters.
    An explicit grid orders values deterministically; no held-out data enters it.
    """
    if type(max_trials) is not int or not 1 <= max_trials <= 24:
        raise ValueError("max_trials must be in [1,24]")
    if type(quality_search) is not bool:
        raise ValueError("quality_search must be bool")
    if quality_search and space is not None:
        raise ValueError("choose quality_search or custom search space")
    if space is None:
        trials = [dict(v) for v in (QUALITY_TRIALS if quality_search else DEFAULT_TRIALS)]
    else:
        if not isinstance(space, dict):
            raise ValueError("search space must be JSON object")
        required = {"learning_rates", "pair_weights", "hard_negative_weights"}
        allowed = required | {"positive_weights", "pair_strategies"}
        if not required.issubset(space) or not set(space).issubset(allowed):
            raise ValueError("search space requires three named axes and optional positive/pair axes")
        axes = (
            ("learning_rates", 1e-6, 1e-3),
            ("pair_weights", 0.0, 1.0),
            ("hard_negative_weights", 1.0, 6.0),
            ("positive_weights", 1.0, 6.0),
        )
        values = []
        for name, lo, hi in axes:
            axis = space.get(name, [1.0])
            if not isinstance(axis, list) or not 1 <= len(axis) <= 8:
                raise ValueError("invalid search axis: " + name)
            checked = [_number(v, lo, hi, name) for v in axis]
            if len(set(checked)) != len(checked):
                raise ValueError("duplicate search value: " + name)
            values.append(sorted(checked))
        strategies = space.get("pair_strategies", ["all"])
        if not isinstance(strategies, list) or not 1 <= len(strategies) <= 2 or (
            any(x not in ("all", "hardest") for x in strategies)
        ) or len(set(strategies)) != len(strategies):
            raise ValueError("invalid pair_strategies")
        count = math.prod(len(axis) for axis in values) * len(strategies)
        if count > max_trials:
            raise ValueError("trial budget exceeded; narrow the search space")
        trials = [
            dict(name=f"mini_auto_{i:02d}", learning_rate=lr,
                 pair_weight=pw, hard_negative_weight=hw,
                 positive_weight=posw, pair_strategy=strategy)
            for i, (lr, pw, hw, posw, strategy) in enumerate(
                product(*values, sorted(strategies)), 1
            )
        ]
    if len(trials) > max_trials:
        raise ValueError("trial budget exceeded")
    return trials


def candidate_rank(metrics):
    """Prefer validation cross-fit low-FPR quality, never calibration/test labels."""
    try:
        if "crossfit" in metrics:
            held = metrics["crossfit"]
            ci = held["cluster_recall95"]
            fprs = [held["fpr"]] + [fold["metrics"]["fpr"] for fold in held.get("folds", [])]
            worst_fpr = max(_number(value, 0, 1, 'fold FPR') for value in fprs)
            feasible = float(worst_fpr <= _number(held["target_fpr"], 0, 1, 'target FPR'))
            # If all candidates fail the FPR budget, minimize the violation
            # before comparing recall; one shifted fold cannot hide in pooling.
            vals = (feasible, 1.0 if feasible else 1.0-worst_fpr,
                    ci["low"], held["recall"], metrics["AP"], 1.0-held["fpr"])
        else:
            ci = metrics["cluster_recall95"]
            vals = (ci["low"], metrics["recall"], metrics["AP"])
    except (KeyError, TypeError) as exc:
        raise ValueError("missing validation metric") from exc
    return tuple(_number(v, 0.0, 1.0, "validation metric") for v in vals)


def leaderboard(trials):
    if not trials:
        raise ValueError("empty AutoML trials")
    names = [trial["name"] for trial in trials]
    if len(set(names)) != len(names):
        raise ValueError("duplicate AutoML candidate")
    return sorted(
        (dict(name=t["name"], rank=list(candidate_rank(t["validation"])),
              checkpoint_sha256=t["checkpoint_sha256"],
              hyperparameters=t["hyperparameters"])
         for t in trials),
        key=lambda t: tuple(-v for v in t["rank"]) + (t["name"],)
    )


def shadow_review_metrics(rows, scores, threshold):
    """Evaluate independent action-review labels with *scenario*-level FP bound.

    Inputs must be separately collected and human reviewed. The source
    authenticity cannot be established by this function; a reviewer must
    verify provenance and independence before interpreting the statistics.
    """
    if len(rows) != len(scores) or not rows:
        raise ValueError("missing shadow review examples")
    if type(threshold) not in (float, int) or not math.isfinite(threshold):
        raise ValueError("invalid threshold")
    labels, negative_groups, bad_groups, sources, identities = [], set(), set(), set(), set()
    source_scenarios = {}
    for row, score in zip(rows, scores):
        if not isinstance(row, dict) or type(row.get("label")) is not int or row["label"] not in (0, 1):
            raise ValueError("invalid reviewed action label")
        if row.get("domain") != "privacy_action":
            raise ValueError("cross-domain labels are not interchangeable")
        if row.get("split") != "independent_review":
            raise ValueError("not an independent-review partition")
        for field in ("group", "source", "annotation_source", "id"):
            if not isinstance(row.get(field), str) or not row[field].strip():
                raise ValueError("missing review provenance: " + field)
        identity = (row["source"], row["id"])
        if identity in identities:
            raise ValueError("duplicate review ID")
        identities.add(identity)
        if type(score) not in (float, int) or not math.isfinite(score):
            raise ValueError("invalid reviewed score")
        labels.append(row["label"])
        sources.add(row["source"])
        source_negative, source_bad = source_scenarios.setdefault(row['source'], (set(), set()))
        if row["label"] == 0:
            negative_groups.add(scenario_key(row))
            source_negative.add(scenario_key(row))
            if score > threshold:
                bad_groups.add(scenario_key(row))
                source_bad.add(scenario_key(row))
    result = confusion(labels, scores, threshold)
    if not result["tp"] + result["fn"] or not negative_groups:
        raise ValueError("review needs confirmed positives and negatives")
    result["negative_scenarios"] = len(negative_groups)
    result["false_positive_scenarios"] = len(bad_groups)
    result["scenario_fpr_upper95"] = upper_binomial95(len(bad_groups), len(negative_groups))
    result["source_count"] = len(sources)
    result["reviewed_rows"] = len(rows)
    result["slices"] = error_slices(rows, scores, threshold)
    for metric in result["slices"]["by_source"]:
        negative, bad = source_scenarios[metric['key']]
        metric['negative_scenarios'] = len(negative)
        metric['scenario_fpr_upper95'] = upper_binomial95(len(bad), len(negative)) if negative else None
    return result


def research_gate(calibration, independent_review=None, *,
                  max_fpr_upper95=0.01, min_review_recall=0.20):
    """Fail closed on missing independent evidence; *never* approve enforcement."""
    max_fpr_upper95 = _number(max_fpr_upper95, 0.0, 0.1, "FPR bound")
    min_review_recall = _number(min_review_recall, 0.0, 1.0, "minimum recall")
    reasons = []
    cal_upper = calibration.get("fpr_upper95")
    if cal_upper is None or _number(cal_upper, 0, 1, "calibration bound") > max_fpr_upper95:
        reasons.append("calibration FPR upper bound exceeds budget")
    if independent_review is None:
        reasons.append("new, independently annotated shadow review missing")
    else:
        if _number(independent_review["fpr_upper95"], 0, 1, 'shadow FPR bound') > max_fpr_upper95:
            reasons.append("shadow row FPR upper bound exceeds budget")
        if _number(independent_review["scenario_fpr_upper95"], 0, 1, 'scenario FPR bound') > max_fpr_upper95:
            reasons.append("shadow scenario FPR upper bound exceeds budget")
        if _number(independent_review["recall"], 0, 1, 'shadow recall') < min_review_recall:
            reasons.append("shadow recall below budget")
        for metric in independent_review.get('slices', {}).get('by_source', []):
            if any(metric.get(key) is None or _number(metric[key], 0, 1, 'source FPR bound') > max_fpr_upper95
                   for key in ('fpr_upper95', 'scenario_fpr_upper95')):
                reasons.append('shadow source FPR upper bound exceeds budget')
                break
    return dict(shadow_review_candidate=not reasons,
                production_approved=False, mode="shadow_only",
                requires_human_provenance_review=True,
                max_fpr_upper95=max_fpr_upper95,
                min_review_recall=min_review_recall,
                blockers=reasons)

