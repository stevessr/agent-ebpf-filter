"""Low-FPR ROC ranking for validation selection, never an independent FPR gate."""
import math

from decision_protocol import confusion, operating_threshold


def low_fpr_ranking(labels, scores, target_fpr=0.01):
    if type(target_fpr) not in (int, float) or not math.isfinite(target_fpr) or not 0 < target_fpr < 1:
        raise ValueError('invalid partial-AUC FPR target')
    base = confusion(labels, scores, 0.0)
    positives, negatives = base['tp'] + base['fn'], base['tn'] + base['fp']
    if not positives or not negatives:
        raise ValueError('partial AUC needs both labels')
    ordered = sorted(zip(scores, labels), reverse=True)
    tp = fp = 0
    previous_x = previous_y = area = 0.0
    index = 0
    while index < len(ordered):
        value = ordered[index][0]
        while index < len(ordered) and ordered[index][0] == value:
            if ordered[index][1]: tp += 1
            else: fp += 1
            index += 1
        x, y = fp / negatives, tp / positives
        end = min(x, target_fpr)
        if x > previous_x:
            end_y = previous_y + (y - previous_y) * (end - previous_x) / (x - previous_x)
            area += (end - previous_x) * (previous_y + end_y) / 2
        previous_x, previous_y = x, y
        if x >= target_fpr:
            break
    chance_area = target_fpr * target_fpr / 2
    standardized = 0.5 * (1 + (area - chance_area) / (target_fpr - chance_area))
    op = operating_threshold(labels, scores, target_fpr)
    observed = confusion(labels, scores, op['threshold'])
    return dict(standardized_partial_auc=standardized, raw_partial_auc=area,
                target_fpr=target_fpr, empirical_recall=observed['recall'],
                empirical_fpr=observed['fpr'], negatives=negatives,
                fpr_resolution=1 / negatives, selection_only=True,
                independent_quality_qualification=False,
                note='Tied-score ROC groups and linear interpolation; validation-only selection, not population FPR certification.')
