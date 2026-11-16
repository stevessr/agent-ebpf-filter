"""Pure-Python, versioned research training contracts; no model or test data imports."""
from collections import defaultdict
import hashlib
import json
import math
import random
import unicodedata


def scenario_key(row):
    """Source-local group names are identities, not normalizable behavior tags."""
    group = row.get('group')
    source = row.get('source', '')
    if not isinstance(group, str) or not group.strip():
        raise ValueError('missing scenario group')
    if not isinstance(source, str) or ('source' in row and not source.strip()):
        raise ValueError('invalid source identity')
    return source, group


def _input_digest(context, action):
    # Conservatively flag Unicode/whitespace near-duplicates for leakage review;
    # never case-fold executable inputs or change what the encoder receives.
    fields = [' '.join(unicodedata.normalize('NFKC', s).split())
              for s in (context, action)]
    return hashlib.sha256(json.dumps(fields, ensure_ascii=False).encode()).hexdigest()


def audit_splits(splits):
    """Reject label/identity leakage between train, validation, calibration and test."""
    required = ('train', 'validation', 'calibration')
    if 'test' in splits:
        required += ('test',)
    if set(splits) != set(required):
        raise ValueError('expected train/validation/calibration with optional test')
    owners, payloads, identities, report = {}, {}, {}, {}
    payload_labels = {}
    for split in required:
        rows = splits[split]
        groups = set()
        for r in rows:
            if r.get('label') not in (0, 1) or type(r.get('label')) is not int:
                raise ValueError('invalid binary label')
            g = scenario_key(r)
            if g in owners and owners[g] != split:
                raise ValueError('scenario leakage')
            owners[g] = split
            groups.add(g)
            # Prepared PrivacyAlign IDs refer to a source action; a reused ID
            # with mutated text is leakage too, even if its payload differs.
            row_id = r.get('id')
            if row_id is not None:
                if not isinstance(row_id, str) or not row_id.strip():
                    raise ValueError('invalid source action ID')
                identity = (g[0], row_id)
                if identity in identities:
                    prior_split, prior_payload = identities[identity]
                    if prior_split != split:
                        raise ValueError('source action ID leakage')
                    if prior_payload != (r.get('context'), r.get('candidate'), r['label']):
                        raise ValueError('conflicting source action ID')
                identities[identity] = (split, (r.get('context'), r.get('candidate'), r['label']))
            # Hash only independent input fields, not dataset id/label/split.
            c = r.get('context')
            a = r.get('candidate')
            if not isinstance(c, str) or not isinstance(a, str):
                raise ValueError('missing context/action input')
            digest = _input_digest(c, a)
            if digest in payload_labels and payload_labels[digest] != r['label']:
                raise ValueError('conflicting context/action labels')
            payload_labels[digest] = r['label']
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
        groups[scenario_key(r)].append(i)
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


def _finite_number(value):
    """Reject booleans, numeric strings and overflowing integers as scores."""
    if type(value) not in (int, float):
        return False
    try:
        return math.isfinite(value)
    except OverflowError:
        return False


def operating_threshold(labels, scores, target_fpr=0.01):
    """Maximize observed recall with <= floor(target_fpr * negative count) FPs.

    Uses strict score > threshold and handles ties without exceeding FP budget.
    This is empirical calibration, not a population FPR guarantee.
    """
    if not _finite_number(target_fpr) or not 0 <= target_fpr < 1:
        raise ValueError('invalid target_fpr')
    if len(labels) != len(scores) or not labels:
        raise ValueError('missing or unmatched calibration rows')
    if any(type(y) is not int or y not in (0, 1) for y in labels):
        raise ValueError('non-binary labels')
    if any(not _finite_number(s) for s in scores):
        raise ValueError('non-finite or non-numeric scores')
    negatives = sorted((float(s) for y, s in zip(labels, scores) if y == 0), reverse=True)
    if not negatives:
        raise ValueError('no calibration negatives')
    budget = math.floor(target_fpr * len(negatives))
    # At threshold==negatives[budget], <=budget negatives are strictly above it.
    return dict(threshold=negatives[budget], negatives=len(negatives), fp_budget=budget,
                target_fpr=target_fpr, comparison='strict >')


def upper_binomial95(fp, negatives):
    """Exact one-sided Clopper-Pearson upper bound, with no scipy dependency."""
    if type(fp) is not int or type(negatives) is not int or not (0 <= fp <= negatives) or negatives <= 0:
        raise ValueError('invalid false-positive counts')
    if fp == negatives:
        return 1.0
    if fp == 0:
        return 1.0 - 0.05 ** (1.0 / negatives)
    # Cache binomial coefficients once, not on every bisection iteration.
    combinations = [math.lgamma(negatives + 1) - math.lgamma(k + 1)
                    - math.lgamma(negatives - k + 1) for k in range(fp + 1)]
    # Log-sum-exp avoids underflow for large negative audit cohorts.
    def cdf(p):
        if p == 1:
            return 0.0
        if p == 0:
            return 1.0
        lp, lq = math.log(p), math.log1p(-p)
        terms = [coefficient + k * lp + (negatives - k) * lq
                 for k, coefficient in enumerate(combinations)]
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


def lower_binomial95(successes, total):
    """Exact one-sided recall lower bound via binomial failure symmetry."""
    if type(successes) is not int or type(total) is not int or not 0 <= successes <= total or total <= 0:
        raise ValueError('invalid recall counts')
    return 1.0 - upper_binomial95(total - successes, total)


def confusion(labels, scores, threshold):
    if not _finite_number(threshold):
        raise ValueError('invalid evaluation threshold')
    if len(labels) != len(scores):
        raise ValueError('length mismatch')
    tp = fp = tn = fn = 0
    for y, score in zip(labels, scores):
        if type(y) is not int or y not in (0, 1) or not _finite_number(score):
            raise ValueError('invalid evaluation row')
        if score > threshold:
            if y == 1: tp += 1
            else: fp += 1
        else:
            if y == 1: fn += 1
            else: tn += 1
    negatives = tn + fp
    positives = tp + fn
    predicted_positive = tp + fp
    total = positives + negatives
    precision = tp / predicted_positive if predicted_positive else 0.0
    recall = tp / positives if positives else None
    specificity = tn / negatives if negatives else None
    return dict(tp=tp, fp=fp, tn=tn, fn=fn,
                accuracy=(tp + tn) / total if total else None,
                precision=precision,
                recall=recall,
                recall_lower95=lower_binomial95(tp, positives) if positives else None,
                specificity=specificity,
                balanced_accuracy=(recall + specificity) / 2 if recall is not None and specificity is not None else None,
                f1=2 * precision * recall / (precision + recall) if recall is not None and precision + recall else 0.0,
                fpr=fp / negatives if negatives else None,
                fpr_upper95=upper_binomial95(fp, negatives) if negatives else None)


def group_bootstrap_recall(rows, scores, threshold, *, replicates=400, seed=20261021):
    """Group-clustered CI; never treat same-scenario candidates as independent."""
    if len(rows) != len(scores) or type(replicates) is not int or not 1 <= replicates <= 10000:
        raise ValueError('invalid bootstrap arguments')
    groups = defaultdict(list)
    for r, p in zip(rows, scores):
        if type(r.get('label')) is not int or r['label'] not in (0, 1) or not _finite_number(p):
            raise ValueError('invalid bootstrap row')
        groups[scenario_key(r)].append((r['label'], float(p)))
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

