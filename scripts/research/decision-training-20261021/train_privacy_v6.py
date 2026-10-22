"""Four equal-budget workers, validation-only selection, then frozen regression."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil

from train_privacy_v2 import (PairBert, BertTokenizerFast, average_precision_score,
    audit_splits, confusion, encode, error_slices, leaderboard, load_file,
    load_rows, operating_threshold, predict, research_gate, sha, torch, train_variant)
from evidence_encoding import plan_rows
from json_contract import read_json, read_jsonl
from low_fpr_ranking import low_fpr_ranking
from tag_protocol import TAG_VERSION
from v6_protocol import VERSION, SEED, EPOCHS, BATCH_SIZE, NAMES, spec_for, validate_workers


def write_json(path, value):
    path.write_text(json.dumps(value, indent=2, sort_keys=True, allow_nan=False) + '\n')


def common_record(assets):
    source = os.environ.get('GITHUB_SHA', '')
    if not re.fullmatch('[0-9a-f]{40}', source):
        raise ValueError('immutable GITHUB_SHA required')
    directory = Path(__file__).resolve().parent
    request = read_json(directory / 'public-v6-request.json')
    expected = dict(version=VERSION, seed=SEED, epochs=EPOCHS, batch_size=BATCH_SIZE,
                    candidates=[spec_for(n) for n in NAMES], mode='shadow_only')
    if request != expected:
        raise ValueError('request differs from fixed v6 budget/specifications')
    return dict(request=request, source_commit=source,
        source_sha256={s: sha(assets / 'agent_privacy_data' / (s + '.jsonl'))
                       for s in ('train', 'validation', 'calibration')},
        implementation_sha256={p.name: sha(p) for p in sorted(directory.glob('*.py'))},
        legacy_implementation_sha256=sha(directory.parent / 'deep-optimization-20261018' / 'privacy.py'),
        pretrained_sha256={n: sha(assets / 'base_mini' / n) for n in ('config.json', 'pytorch_model.bin')},
        vocab_sha256=sha(assets / 'base_model' / 'vocab.txt'),
        tag_version=TAG_VERSION, selection='validation standardized partial ROC AUC at max_fpr=0.01; then empirical recall; AP; lower empirical FPR',
        calibration='separate split; strict score > threshold; floor(1% * negatives) FP budget',
        test='previously consumed regression, opened after all candidate thresholds freeze',
        production_approved=False, max_sequence_tokens=384, threads=2)


def encode_view(rows, tok, view):
    if view == 'legacy':
        return encode(rows, tok)
    if view != 'evidence_pair.v1':
        raise ValueError('unknown encoding view')
    planned, _ = plan_rows(rows, tok)
    return {key: torch.tensor(value, dtype=torch.long) for key, value in planned.items()}


def check_partial_auc():
    # Full research environment checks our tied-score integration against sklearn.
    from sklearn.metrics import roc_auc_score
    cases = [([0, 1, 0, 1], [0.0, 1.0, .1, .9]),
             ([0, 1, 0, 1], [.5, .5, .5, .5]),
             ([0, 1, 0, 1, 0, 1], [.9, .8, .8, .5, .3, .2])]
    for labels, scores in cases:
        actual = low_fpr_ranking(labels, scores)['standardized_partial_auc']
        expected = roc_auc_score(labels, scores, max_fpr=.01)
        if abs(actual - expected) > 1e-12:
            raise AssertionError('partial AUC differs from sklearn')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('mode', choices=['worker', 'aggregate'])
    parser.add_argument('--candidate', choices=NAMES)
    parser.add_argument('--assets', type=Path, required=True)
    parser.add_argument('--out', type=Path, required=True)
    parser.add_argument('--workers', type=Path)
    args = parser.parse_args()
    if args.out.exists() and any(args.out.iterdir()):
        parser.error('use an empty output directory')
    args.out.mkdir(parents=True, exist_ok=True)
    torch.set_num_threads(2)
    torch.use_deterministic_algorithms(True)
    check_partial_auc()
    common = common_record(args.assets)
    datasets = {s: load_rows(args.assets, s) for s in ('train', 'validation', 'calibration')}
    development_audit = audit_splits(datasets)
    mini = args.assets / 'base_mini'
    tok = BertTokenizerFast(vocab_file=str(args.assets / 'base_model' / 'vocab.txt'), do_lower_case=True)
    if args.mode == 'worker':
        if not args.candidate:
            parser.error('worker requires candidate')
        spec = spec_for(args.candidate)
        encoded = {s: encode_view(rows, tok, spec['context_view']) for s, rows in datasets.items()}
        # Both encodings audited without labels; only training/validation text.
        encoding_audit = {}
        for split in ('train', 'validation'):
            _, encoding_audit[split] = plan_rows(datasets[split], tok)
            old = encode(datasets[split], tok)
            encoding_audit[split]['legacy_mean_shared_tokens'] = sum(
                len(set(a.tolist()) & set(c.tolist()) - set(tok.all_special_ids))
                for ids, tt, mask in zip(old['input_ids'], old['token_type_ids'], old['attention_mask'])
                for a, c in [(ids[(tt == 1) & mask.bool()], ids[(tt == 0) & mask.bool()])]
            ) / len(datasets[split])
        write_json(args.out / 'encoding_audit.json', encoding_audit)
        write_json(args.out / 'protocol.json', dict(common=common, development_audit=development_audit))
        trial, scores = train_variant(spec, mini, args.out, datasets, encoded,
            epochs=EPOCHS, batch_size=BATCH_SIZE, seed=SEED,
            selection_mode='partial_auc', capture_initial_digest=True)
        labels = [r['label'] for r in datasets['calibration']]
        operation = operating_threshold(labels, scores.tolist())
        trial['calibration_threshold'] = operation
        trial['calibration'] = confusion(labels, scores.tolist(), operation['threshold'])
        (args.out / 'calibration_predictions.jsonl').write_text(''.join(json.dumps(dict(
            id=r['id'], group=r['group'], source=r.get('source', ''), label=r['label'], score=float(s)),
            allow_nan=False) + '\n' for r, s in zip(datasets['calibration'], scores)))
        write_json(args.out / 'worker.json', dict(common=common, trial=trial))
        print('WORKER_V6', json.dumps(dict(name=trial['name'], epoch=trial['selected_epoch'],
            validation=trial['validation'], calibration=trial['calibration'],
            initial_state_sha256=trial['initial_state_sha256'], checkpoint_sha256=trial['checkpoint_sha256'])), flush=True)
        return
    if not args.workers:
        parser.error('aggregate requires workers')
    files = sorted(args.workers.rglob('worker.json'))
    records = [read_json(path) for path in files]
    if validate_workers(records) != common:
        raise ValueError('worker provenance differs from aggregation source/assets')
    trials = []
    for path, record in zip(files, records):
        trial = record['trial']
        name = trial['name']
        checkpoint = path.parent / (name + '.safetensors')
        if sha(checkpoint) != trial['checkpoint_sha256']:
            raise ValueError('worker checkpoint digest mismatch')
        cp = read_jsonl(path.parent / 'calibration_predictions.jsonl')
        expected = [dict(id=r['id'], group=r['group'], source=r.get('source', ''), label=r['label'])
                    for r in datasets['calibration']]
        if [{k: row.get(k) for k in ('id', 'group', 'source', 'label')} for row in cp] != expected:
            raise ValueError('calibration row identities/order differ')
        scores = [row['score'] for row in cp]
        labels = [r['label'] for r in datasets['calibration']]
        operation = operating_threshold(labels, scores)
        stats = confusion(labels, scores, operation['threshold'])
        if operation != trial['calibration_threshold'] or stats != trial['calibration']:
            raise ValueError('worker calibration does not reproduce')
        shutil.copy2(checkpoint, args.out / checkpoint.name)
        trials.append(trial)
    ranking = leaderboard(trials)
    selected = ranking[0]['name']
    freeze = dict(common=common, selected=selected, leaderboard=ranking,
        candidates={t['name']: dict(selected_epoch=t['selected_epoch'],
            threshold=t['calibration_threshold']['threshold'],
            checkpoint_sha256=t['checkpoint_sha256'], calibration=t['calibration'],
            validation=t['validation']) for t in trials},
        initial_state_sha256=trials[0]['initial_state_sha256'],
        mode='shadow_only', production_approved=False)
    write_json(args.out / 'trials.json', trials)
    write_json(args.out / 'freeze.json', freeze)
    # All four checkpoints, epoch selections and thresholds are immutable now.
    datasets['test'] = load_rows(args.assets, 'test')
    audit = audit_splits(datasets)
    labels = [r['label'] for r in datasets['test']]
    regressions = {}
    for trial in trials:
        name = trial['name']
        model = PairBert(mini, pretrained=False)
        model.load_state_dict(load_file(str(args.out / (name + '.safetensors'))))
        scores = predict(model, encode_view(datasets['test'], tok, spec_for(name)['context_view'])).tolist()
        threshold = freeze['candidates'][name]['threshold']
        regressions[name] = dict(metrics=confusion(labels, scores, threshold),
            AP=float(average_precision_score(labels, scores)), selected_epoch=trial['selected_epoch'],
            threshold=threshold, checkpoint_sha256=trial['checkpoint_sha256'])
        write_json(args.out / (name + '_error_slices.json'), error_slices(datasets['test'], scores, threshold))
        (args.out / (name + '_test_predictions.jsonl')).write_text(''.join(json.dumps(dict(
            id=r['id'], group=r['group'], source=r.get('source', ''), tags=r['tags'],
            tag_version=TAG_VERSION, label=r['label'], score=s), allow_nan=False) + '\n'
            for r, s in zip(datasets['test'], scores)))
        del model
    result = dict(version=VERSION, selected=selected, freeze_sha256=sha(args.out / 'freeze.json'),
        initial_state_sha256=freeze['initial_state_sha256'], source_commit=common['source_commit'],
        leaderboard=ranking, candidates=regressions,
        calibration={t['name']: t['calibration'] for t in trials},
        shadow_review_gate=research_gate(freeze['candidates'][selected]['calibration']),
        split_audit=audit, test_source_sha256=sha(args.assets / 'agent_privacy_data' / 'test.jsonl'),
        actual_neural_epochs=4 * EPOCHS, actual_fits=4, mode='shadow_only', production_approved=False,
        note='Consumed test is regression only. Partial AUC interpolates the empirical validation ROC; it does not certify population FPR.')
    write_json(args.out / 'results.json', result)
    print('FROZEN_V6', json.dumps(result, sort_keys=True), flush=True)


if __name__ == '__main__':
    main()
