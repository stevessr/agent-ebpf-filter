"""New experiment: train-only hard-negative objective for PrivacyAlign agent actions.

Requires the public prepared assets and local BERT-Mini, as in privacy.py.
No prior frozen weights or consumed test predictions are used for fitting.
"""
import argparse
import hashlib
import json
import math
import random
import sys
from pathlib import Path

# Reuse the exact tokenizer/model definitions from the frozen v1 study.
sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'deep-optimization-20261018'))

import numpy as np
import torch
from torch.nn import functional as F
from safetensors.torch import load_file, save_file
from transformers import BertTokenizerFast
from sklearn.metrics import average_precision_score

from decision_protocol import (audit_splits, confusion, group_batches,
                               group_bootstrap_recall, operating_threshold)
from automl_protocol import candidate_rank, leaderboard, research_gate, trial_plan
from privacy import PairBert, encode

SEED = 20261021


def load_rows(assets, split):
    return [json.loads(s) for s in (assets / 'agent_privacy_data' / (split + '.jsonl')).read_text().splitlines()]


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def pairwise_loss(logits, labels, rows, indices):
    """Only within-scenario opposite-label comparisons, never unrelated runs."""
    groups = {}
    for i, global_idx in enumerate(indices):
        groups.setdefault(rows[global_idx]['group'], []).append(i)
    losses = []
    for positions in groups.values():
        positives = [i for i in positions if labels[i].item() == 1]
        negatives = [i for i in positions if labels[i].item() == 0]
        for pos in positives:
            for neg in negatives:
                losses.append(F.softplus(0.25 - logits[pos] + logits[neg]))
    return torch.stack(losses).mean() if losses else logits.sum() * 0


def training_loss(logits, labels, rows, indices, *, hard_negative_weight, pair_weight):
    per_row = F.binary_cross_entropy_with_logits(logits, labels, reduction='none')
    weight = torch.ones_like(per_row)
    if hard_negative_weight > 1:
        neg = torch.nonzero(labels == 0, as_tuple=False).flatten()
        if len(neg):
            # Select the highest-scored benign actions in THIS training batch.
            count = max(1, math.ceil(len(neg) / 4))
            worst = neg[torch.topk(logits.detach()[neg], count).indices]
            weight[worst] = hard_negative_weight
    bce = (per_row * weight).sum() / weight.sum()
    return bce + pair_weight * pairwise_loss(logits, labels, rows, indices)


def predict(model, encoded, batch_size=24):
    model.eval()
    logits = []
    with torch.inference_mode():
        for i in range(0, len(encoded['input_ids']), batch_size):
            logits.append(model(**{k: v[i:i + batch_size] for k, v in encoded.items()}).cpu())
    return torch.sigmoid(torch.cat(logits)).numpy()


def validation(rows, scores, *, seed):
    labels = [r['label'] for r in rows]
    operation = operating_threshold(labels, scores.tolist())
    metrics = confusion(labels, scores.tolist(), operation['threshold'])
    metrics['AP'] = float(average_precision_score(labels, scores))
    metrics['cluster_recall95'] = group_bootstrap_recall(rows, scores.tolist(), operation['threshold'],
                                                          replicates=400, seed=seed)
    return operation, metrics


def train_variant(spec, mini, out, datasets, encoded, *, epochs, batch_size, seed):
    variant = spec['name']
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)
    model = PairBert(mini, pretrained=True)
    optimizer = torch.optim.AdamW(model.parameters(), lr=spec['learning_rate'], weight_decay=0.01)
    labels = torch.tensor([r['label'] for r in datasets['train']], dtype=torch.float32)
    best_rank = None
    best_epoch = 0
    history = []
    checkpoint = out / (variant + '.safetensors')
    for epoch in range(1, epochs + 1):
        model.train()
        batches = group_batches(datasets['train'], batch_size, seed + epoch)
        losses = []
        for indices in batches:
            optimizer.zero_grad(set_to_none=True)
            logits = model(**{k: v[indices] for k, v in encoded['train'].items()})
            loss = training_loss(logits, labels[indices], datasets['train'], indices,
                                 hard_negative_weight=spec['hard_negative_weight'],
                                 pair_weight=spec['pair_weight'])
            loss.backward()
            torch.nn.utils.clip_grad_norm_(model.parameters(), 1.0)
            optimizer.step()
            losses.append(float(loss.item()))
        scores = predict(model, encoded['validation'])
        operation, stats = validation(datasets['validation'], scores, seed=seed)
        rank = candidate_rank(stats)
        history.append(dict(epoch=epoch, loss=float(np.mean(losses)), validation=stats,
                            empirical_validation_threshold=operation))
        print(variant, json.dumps(history[-1], sort_keys=True), flush=True)
        if best_rank is None or rank > best_rank:
            best_rank = rank
            best_epoch = epoch
            save_file({k: v.detach().cpu().contiguous() for k, v in model.state_dict().items()}, str(checkpoint))
    model.load_state_dict(load_file(str(checkpoint)))
    calibration = predict(model, encoded['calibration'])
    return dict(name=variant, selected_epoch=best_epoch, epoch_history=history,
                validation=history[best_epoch - 1]['validation'],
                hyperparameters=spec,
                checkpoint_sha256=sha(checkpoint)), calibration


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--assets', type=Path, required=True)
    p.add_argument('--mini', type=Path, required=True)
    p.add_argument('--out', type=Path, required=True)
    p.add_argument('--epochs', type=int, default=6)
    p.add_argument('--batch-size', type=int, default=16)
    p.add_argument('--search-space', type=Path, help='Optional bounded JSON parameter grid')
    p.add_argument('--max-trials', type=int, default=12)
    args = p.parse_args()
    if args.epochs < 1 or args.batch_size < 2:
        p.error('positive epochs and batch-size >= 2 required')
    space = json.loads(args.search_space.read_text()) if args.search_space else None
    plan = trial_plan(space, max_trials=args.max_trials)
    args.out.mkdir(parents=True, exist_ok=True)
    if any((args.out / name).exists() for name in ('protocol.json', 'freeze.json', 'trials.json')):
        p.error('output already contains a training run; use a new directory')
    torch.set_num_threads(2)
    torch.use_deterministic_algorithms(True)
    datasets = {s: load_rows(args.assets, s) for s in ['train', 'validation', 'calibration']}
    # Reject duplicate identities and context/action leakage among development
    # splits before optimization, without opening consumed test labels.
    development_audit = audit_splits(datasets)
    tok = BertTokenizerFast(vocab_file=str(args.assets / 'base_model' / 'vocab.txt'), do_lower_case=True)
    encoded = {split: encode(rows, tok) for split, rows in datasets.items()}
    protocol = dict(version='privacy_hard_negative.v2', seed=SEED, epochs=args.epochs,
                    batch_size=args.batch_size, variants=plan,
                    max_trials=args.max_trials,
                    objective='BCE + configurable within-scenario pair loss; optional weighted top-quartile batch negatives',
                    selection='validation group-bootstrap 95% recall lower bound, then empirical recall@1% FPR, then AP',
                    calibration='separate calibration split; strict > with floor(1% * negatives) FP budget',
                    heldout='previously consumed PrivacyAlign test, regression only',
                    production_approved=False, mode='shadow_only')
    protocol['development_split_audit'] = development_audit
    protocol['search_space_sha256'] = sha(args.search_space) if args.search_space else None
    protocol['development_source_sha256'] = {s: sha(args.assets / 'agent_privacy_data' / (s + '.jsonl')) for s in datasets}
    protocol['vocab_sha256'] = sha(args.assets / 'base_model' / 'vocab.txt')
    (args.out / 'protocol.json').write_text(json.dumps(protocol, indent=2) + '\n')
    trials = []
    for spec in plan:
        trial, cp = train_variant(spec, args.mini, args.out, datasets, encoded,
                                  epochs=args.epochs, batch_size=args.batch_size, seed=SEED)
        op = operating_threshold([r['label'] for r in datasets['calibration']], cp.tolist())
        trial['calibration'] = confusion([r['label'] for r in datasets['calibration']], cp.tolist(), op['threshold'])
        trial['calibration_threshold'] = op
        trials.append(trial)
    ranking = leaderboard(trials)
    (args.out / 'leaderboard.json').write_text(json.dumps(ranking, indent=2) + '\n')
    selected = next(t for t in trials if t['name'] == ranking[0]['name'])
    freeze = dict(selected=selected['name'], selected_epoch=selected['selected_epoch'],
                  threshold=selected['calibration_threshold']['threshold'],
                  empirical_calibration=selected['calibration'],
                  upper95_within_1pct=(selected['calibration']['fpr_upper95'] <= 0.01),
                  artifact_sha256=selected['checkpoint_sha256'], protocol_sha256=sha(args.out / 'protocol.json'),
                  selection_rank=ranking[0]['rank'],
                  shadow_review_gate=research_gate(selected['calibration']),
                  mode='shadow_only', production_approved=False)
    (args.out / 'trials.json').write_text(json.dumps(trials, indent=2) + '\n')
    (args.out / 'freeze.json').write_text(json.dumps(freeze, indent=2) + '\n')
    # Regression labels are accessed only after the training/calibration freeze exists.
    datasets['test'] = load_rows(args.assets, 'test')
    audit = audit_splits(datasets)
    model = PairBert(args.mini, pretrained=False)
    model.load_state_dict(load_file(str(args.out / (selected['name'] + '.safetensors'))))
    scores = predict(model, encode(datasets['test'], tok))
    result = dict(freeze=freeze, split_audit=audit, regression=confusion(
        [r['label'] for r in datasets['test']], scores.tolist(), freeze['threshold']),
        regression_AP=float(average_precision_score([r['label'] for r in datasets['test']], scores)),
        test_source_sha256=sha(args.assets / 'agent_privacy_data' / 'test.jsonl'),
        note='Regression set consumed in prior experiments; no independent production-quality claim.')
    (args.out / 'results.json').write_text(json.dumps(result, indent=2) + '\n')
    (args.out / 'test_predictions.jsonl').write_text(''.join(json.dumps(dict(
        id=r['id'], group=r['group'], label=r['label'], score=float(s))) + '\n'
        for r, s in zip(datasets['test'], scores)))
    print('FROZEN', json.dumps(result), flush=True)


if __name__ == '__main__':
    main()
