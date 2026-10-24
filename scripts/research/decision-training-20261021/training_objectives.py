"""Differentiable training objectives; requires only PyTorch, no data or models."""
import math

import torch
from torch.nn import functional as F

from decision_protocol import scenario_key


def pairwise_loss(logits, labels, rows, indices, *, strategy="all"):
    """Opposite-label comparisons are restricted to the same scenario."""
    if strategy not in ("all", "hardest"):
        raise ValueError("invalid pair strategy")
    groups = {}
    for i, global_idx in enumerate(indices):
        groups.setdefault(scenario_key(rows[global_idx]), []).append(i)
    losses = []
    for positions in groups.values():
        positives = [i for i in positions if labels[i].item() == 1]
        negatives = [i for i in positions if labels[i].item() == 0]
        if not positives or not negatives:
            continue
        if strategy == "hardest":
            # Hardest benign candidate within the same user/task scenario.
            # The max retains gradients through the selected negative.
            hardest = torch.stack([logits[i] for i in negatives]).max()
            losses.extend(F.softplus(0.25 + hardest - logits[pos]) for pos in positives)
        else:
            for pos in positives:
                for neg in negatives:
                    losses.append(F.softplus(0.25 - logits[pos] + logits[neg]))
    return torch.stack(losses).mean() if losses else logits.sum() * 0


def training_loss(logits, labels, rows, indices, *,
                  hard_negative_weight, pair_weight,
                  positive_weight=1.0, pair_strategy="all", tail_weight=0.0):
    """Asymmetric supervised BCE + optional hardest-pair ranking.

    Positive upweighting tests reduced misses; hard negatives test low FPR.
    Model selection, NOT these guesses, determines which variant to retain.
    """
    per_row = F.binary_cross_entropy_with_logits(logits, labels, reduction='none')
    weight = torch.ones_like(per_row)
    if positive_weight > 1:
        weight[labels == 1] = positive_weight
    if hard_negative_weight > 1:
        neg = torch.nonzero(labels == 0, as_tuple=False).flatten()
        if len(neg):
            count = max(1, math.ceil(len(neg) / 4))
            worst = neg[torch.topk(logits.detach()[neg], count).indices]
            weight[worst] = hard_negative_weight
    bce = (per_row * weight).sum() / weight.sum()
    result = bce + pair_weight * pairwise_loss(
        logits, labels, rows, indices, strategy=pair_strategy
    )
    if tail_weight:
        if not math.isfinite(tail_weight) or not 0 < tail_weight <= 1:
            raise ValueError('invalid tail loss weight')
        result = result + tail_weight * tail_ranking_loss(logits, labels, rows, indices)
    return result


def tail_ranking_loss(logits, labels, rows, indices, *, fraction=0.1):
    """Train-batch negative-tail surrogate, restricted to each source/domain.

    Cross-scenario binary ranking is not causal attribution. At small batches
    the top 10% is one negative; this does not estimate a population 1% tail.
    """
    if type(fraction) not in (int, float) or not math.isfinite(fraction) or not 0 < fraction <= 1:
        raise ValueError('invalid tail fraction')
    sources = {}
    for position, index in enumerate(indices):
        row = rows[index]
        if row.get('domain', 'privacy_action') != 'privacy_action':
            raise ValueError('tail ranking requires privacy action labels')
        sources.setdefault(scenario_key(row)[0], []).append(position)
    losses = []
    for positions in sources.values():
        positive = [i for i in positions if labels[i].item() == 1]
        negative = [i for i in positions if labels[i].item() == 0]
        if not positive or not negative:
            continue
        negative = torch.tensor(negative, device=logits.device)
        count = max(1, math.ceil(len(negative) * fraction))
        hardest = negative[torch.topk(logits.detach()[negative], count).indices]
        losses.append(F.softplus(0.25 + logits[hardest].unsqueeze(0)
                                 - logits[positive].unsqueeze(1)).flatten())
    return torch.cat(losses).mean() if losses else logits.sum() * 0


