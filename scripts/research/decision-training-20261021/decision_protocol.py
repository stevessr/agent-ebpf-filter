"""Pure-Python, versioned research training contracts; no model or test data imports."""
from collections import defaultdict
import hashlib
import json
import math
import random


def audit_splits(splits):
    """Reject label/identity leakage between train, validation, calibration and test."""
    required = ('train', 'validation', 'calibration')
    if 'test' in splits:
        required += ('test',)
    if set(splits) != set(required):
        raise ValueError('expected train/validation/calibration with optional test')
    owners, payloads, report = {}, {}, {}
    for split in required:
        rows = splits[split]
        groups = set()
        for r in rows:
            if r.get('label') not in (0, 1) or type(r.get('label')) is not int:
                raise ValueError('invalid binary label')
            g = r.get('group')
            if not isinstance(g, str) or not g:
                raise ValueError('missing scenario group')
            if g in owners and owners[g] != split:
                raise ValueError('scenario leakage: ' + g)
            owners[g] = split
            groups.add(g)
            # Hash only independent input fields, not dataset id/label/split.
            c = r.get('context')
            a = r.get('candidate')
            if not isinstance(c, str) or not isinstance(a, str):
                raise ValueError('missing context/action input')
            digest = hashlib.sha256(json.dumps((c, a), ensure_ascii=False).encode()).hexdigest()
            if digest in payloads and payloads[digest] != split:
                raise ValueError('duplicate context/action across splits')
            payloads[digest] = split
        report[split] = dict(rows=len(rows), groups=len(groups), positives=sum(r['label'] for r in rows))
        if not rows or not 0 < report[split]['positives'] < len(rows):
            raise ValueError('split must have both labels: ' + split)
    return report


def group_batches(rows, batch_size, seed):
    """Each scenario stays intact, including both candidate labels; each row used once."""
    if batch_size < 2:
        raise ValueError('batch size must be at least 2')
    groups = defaultdict(list)
    for i, r in enumerate(rows):
        groups[r['group']].append(i)
    if any(len(g) > batch_size for g in groups.values()):
        raise ValueError('scenario exceeds batch size: increase --batch-size')
    keys = sorted(groups)
    random.Random(seed).shuffle(keys)
    batches, current = [], []
    for k in keys:
        g = groups[k]
        if current and len(current) + len(g) > batch_size:
            batches.append(current)
            current = []
        current.extend(g)
    if current:
        batches.append(current)
    return batches


def operating_threshold(labels, scores, target_fpr=0.01):
    """Maximize observed recall with <= floor(target_fpr * negative count) FPs.

    Uses strict score > threshold and handles ties without exceeding FP budget.
    This is empirical calibration, not a population FPR guarantee.
    """
    if not 0 <= target_fpr < 1:
        raise ValueError('invalid target_fpr')
    if len(labels) != len(scores) or not labels:
        raise ValueError('missing or unmatched calibration rows')
    if any(type(y) is not int or y not in (0, 1) for y in labels):
        raise ValueError('non-binary labels')
    if any(not math.isfinite(float(s)) for s in scores):
        raise ValueError('non-finite scores')
    negatives = sorted((float(s) for y, s in zip(labels, scores) if y == 0), reverse=True)
    if not negatives:
        raise ValueError('no calibration negatives')
    budget = math.floor(target_fpr * len(negatives))
    # At threshold==negatives[budget], <=budget negatives are strictly above it.
    return dict(threshold=negatives[budget], negatives=len(negatives), fp_budget=budget,
                target_fpr=target_fpr, comparison='strict >')


def upper_binomial95(fp, negatives):
    """Exact one-sided Clopper-Pearson upper bound, with no scipy dependency."""
    if not (0 <= fp <= negatives) or negatives <= 0:
        raise ValueError('invalid false-positive counts')
    if fp == negatives:
        return 1.0
    if fp == 0:
        return 1.0 - 0.05 ** (1.0 / negatives)
    # Log-sum-exp avoids underflow for large negative audit cohorts.
    def cdf(p):
        if p == 1:
            return 0.0
        if p == 0:
            return 1.0
        lp, lq = math.log(p), math.log1p(-p)
        terms = [math.lgamma(negatives + 1) - math.lgamma(k + 1)
                 - math.lgamma(negatives - k + 1) + k * lp
                 + (negatives - k) * lq for k in range(fp + 1)]
        maximum = max(terms)
        return math.exp(maximum) * sum(math.exp(term - maximum) for term in terms)
    lo, hi = 0.0, 1.0
    for _ in range(64):
        mid = (lo + hi) / 2
        if cdf(mid) > 0.05:
            lo = mid
        else:
            hi = mid
    return hi


def confusion(labels, scores, threshold):
    if len(labels) != len(scores):
        raise ValueError('length mismatch')
    tp = fp = tn = fn = 0
    for y, score in zip(labels, scores):
        if y not in (0, 1) or not math.isfinite(float(score)):
            raise ValueError('invalid evaluation row')
        if score > threshold:
            if y == 1: tp += 1
            else: fp += 1
        else:
            if y == 1: fn += 1
            else: tn += 1
    negatives = tn + fp
    positives = tp + fn
    return dict(tp=tp, fp=fp, tn=tn, fn=fn,
                recall=tp / positives if positives else None,
                fpr=fp / negatives if negatives else None,
                fpr_upper95=upper_binomial95(fp, negatives) if negatives else None)


def group_bootstrap_recall(rows, scores, threshold, *, replicates=400, seed=20261021):
    """Group-clustered CI; never treat same-scenario candidates as independent."""
    if len(rows) != len(scores) or replicates < 1:
        raise ValueError('invalid bootstrap arguments')
    groups = defaultdict(list)
    for r, p in zip(rows, scores):
        groups[r['group']].append((r['label'], float(p)))
    if not groups:
        raise ValueError('empty bootstrap input')
    units = [groups[g] for g in sorted(groups)]
    rng = random.Random(seed)
    samples = []
    for _ in range(replicates):
        tp = positives = 0
        for _ in range(len(units)):
            for label, score in units[rng.randrange(len(units))]:
                if label == 1:
                    positives += 1
                    tp += int(score > threshold)
        if positives:
            samples.append(tp / positives)
    if not samples:
        raise ValueError('no positive bootstrap strata')
    samples.sort()
    return dict(low=samples[max(0, math.ceil(0.025 * len(samples)) - 1)],
                high=samples[min(len(samples)-1, math.ceil(0.975 * len(samples))-1)],
                valid_replicates=len(samples), total_replicates=replicates)
