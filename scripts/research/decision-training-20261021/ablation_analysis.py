"""Verify archived v6 predictions and describe paired consumed-regression deltas.

No fitting, epoch selection, threshold search or automatic promotion occurs.
Scenario bootstrap intervals describe this consumed corpus, not new generalization.
"""
import argparse
from collections import defaultdict
import hashlib
import json
import math
from pathlib import Path
import random

from automl_protocol import leaderboard
from decision_protocol import confusion, scenario_key
from error_analysis import error_slices
from json_contract import read_json, read_jsonl
from v6_protocol import NAMES, VERSION, validate_workers


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def same_numbers(a, b):
    if isinstance(a, dict) and isinstance(b, dict):
        return set(a) == set(b) and all(same_numbers(a[k], b[k]) for k in a)
    if isinstance(a, list) and isinstance(b, list):
        return len(a) == len(b) and all(same_numbers(x,y) for x,y in zip(a,b))
    if type(a) in (int,float) and type(b) in (int,float):
        return math.isclose(a,b,rel_tol=1e-12,abs_tol=1e-12)
    return a == b


def average_precision(labels, scores):
    # Precision at each distinct score, weighted by newly retrieved positives.
    total = sum(labels)
    if not total:
        return None
    pairs = sorted(zip(scores,labels), reverse=True)
    tp = seen = 0
    area = 0.0
    index = 0
    while index < len(pairs):
        score = pairs[index][0]
        new_positive = 0
        while index < len(pairs) and pairs[index][0] == score:
            new_positive += pairs[index][1]
            seen += 1
            index += 1
        tp += new_positive
        area += new_positive / total * tp / seen
    return area


def paired_changes(rows, baseline_scores, scores, baseline_threshold, threshold, *, replicates=2000, seed=20261022):
    labels=[r['label'] for r in rows]
    base=confusion(labels,baseline_scores,baseline_threshold)
    candidate=confusion(labels,scores,threshold)
    if type(replicates) is not int or not 1 <= replicates <= 10000:
        raise ValueError('invalid bootstrap budget')
    if len(rows) != len(scores) or len(rows) != len(baseline_scores):
        raise ValueError('unmatched paired predictions')
    groups=defaultdict(lambda: [0,0,0,0])
    transitions=dict(newly_detected_positive=0,lost_positive=0,new_false_positive=0,removed_false_positive=0)
    for row,b,c in zip(rows,baseline_scores,scores):
        y=row['label'];old=int(b > baseline_threshold);new=int(c > threshold)
        counts=groups[scenario_key(row)]
        counts[0 if y else 1] += 1
        counts[2 if y else 3] += new-old
        if y and new > old: transitions['newly_detected_positive'] += 1
        if y and new < old: transitions['lost_positive'] += 1
        if not y and new > old: transitions['new_false_positive'] += 1
        if not y and new < old: transitions['removed_false_positive'] += 1
    units=[groups[key] for key in sorted(groups)]
    rng=random.Random(seed)
    recall,fpr=[],[]
    for _ in range(replicates):
        total=[0,0,0,0]
        for _ in units:
            sampled=units[rng.randrange(len(units))]
            total=[x+y for x,y in zip(total,sampled)]
        if total[0]: recall.append(total[2]/total[0])
        if total[1]: fpr.append(total[3]/total[1])
    def interval(values):
        if not values: return None
        values.sort();n=len(values)
        return dict(low=values[max(0,math.ceil(.025*n)-1)],
                    high=values[min(n-1,math.ceil(.975*n)-1)],valid_replicates=n)
    return dict(transitions=transitions,
        recall_delta=candidate['recall']-base['recall'] if candidate['recall'] is not None else None,
        fpr_delta=candidate['fpr']-base['fpr'] if candidate['fpr'] is not None else None,
        recall_delta_cluster95=interval(recall),fpr_delta_cluster95=interval(fpr),
        replicates=replicates,seed=seed,scenario_units=len(units),
        baseline_threshold=baseline_threshold,threshold=threshold,
        thresholds_previously_frozen=True,regression_only=True)


# The first v6 export wrote absent source as "". Restore absence only for this
# exact archived writer and prepared test digest; do not relax source validation.
LEGACY_WRITER = 'd24182f4fedc62f49e8a886c2b5db959e134e942'
LEGACY_TEST = '1abecfd4716b428cecb55466a7a709125fc9589098e6d6aefcf4389033f32882'


def archived_rows(rows, source_commit, version, test_sha256):
    repaired = 0
    restored = []
    for row in rows:
        copy = dict(row)
        if copy.get('source') == '':
            if (source_commit, version, test_sha256) != (LEGACY_WRITER, VERSION, LEGACY_TEST):
                raise ValueError('empty source outside pinned legacy export')
            copy.pop('source')
            repaired += 1
        scenario_key(copy)
        restored.append(copy)
    return restored, repaired


def analyze(root, expected_source):
    freezes=list(root.rglob('freeze.json'))
    if len(freezes) != 1:
        raise ValueError('one final freeze required')
    directory=freezes[0].parent
    frozen=read_json(freezes[0]);result=read_json(directory/'results.json')
    if frozen.get('mode') != 'shadow_only' or frozen.get('production_approved') is not False:
        raise ValueError('invalid mode')
    if frozen['common']['source_commit'] != expected_source or result['source_commit'] != expected_source:
        raise ValueError('training source differs from request')
    if digest(freezes[0]) != result['freeze_sha256']:
        raise ValueError('freeze bytes differ from regression record')
    workers=[read_json(p) for p in root.rglob('worker.json')]
    if validate_workers(workers) != frozen['common']:
        raise ValueError('worker provenance differs from freeze')
    ranking=leaderboard([w['trial'] for w in workers])
    if ranking != frozen['leaderboard'] or ranking[0]['name'] != frozen['selected'] or result['selected'] != frozen['selected']:
        raise ValueError('validation-only ranking does not reproduce')
    predictions={}
    verified={}
    source_repairs={}
    for name in NAMES:
        selected=frozen['candidates'][name]
        if digest(directory/(name+'.safetensors')) != selected['checkpoint_sha256']:
            raise ValueError('checkpoint digest mismatch')
        rows=read_jsonl(directory/(name+'_test_predictions.jsonl'))
        rows, source_repairs[name] = archived_rows(rows, expected_source,
            frozen['common']['request']['version'], result.get('test_source_sha256'))
        stats=confusion([r['label'] for r in rows],[r['score'] for r in rows],selected['threshold'])
        ap=average_precision([r['label'] for r in rows],[r['score'] for r in rows])
        if not same_numbers(stats,result['candidates'][name]['metrics']) or not same_numbers(ap,result['candidates'][name]['AP']):
            raise ValueError('archived regression metrics do not reproduce')
        predictions[name]=rows
        slices=error_slices(rows,[r['score'] for r in rows],selected['threshold'])
        verified[name]=dict(metrics=stats,AP=ap,selected_epoch=selected['selected_epoch'],
            threshold=selected['threshold'],tag_coverage=slices['tag_coverage'],
            scenario_metrics={k:v for k,v in slices['overall'].items() if 'scenario' in k})
    baseline='v6_legacy_pair';base=predictions[baseline]
    identities=lambda rows:[(r.get('source',''),r['id'],r['group'],r['label']) for r in rows]
    if len({(r.get('source',''),r['id']) for r in base}) != len(base):
        raise ValueError('duplicate regression identities')
    changes={}
    for name in NAMES:
        rows=predictions[name]
        if identities(rows) != identities(base):
            raise ValueError('candidate predictions do not align by source/ID/scenario/label')
        changes[name]=paired_changes(base,[r['score'] for r in base],[r['score'] for r in rows],
            frozen['candidates'][baseline]['threshold'],frozen['candidates'][name]['threshold'])
    audits=[read_json(p) for p in root.rglob('encoding_audit.json')]
    if len(audits) != 4 or any(a != audits[0] for a in audits):
        raise ValueError('workers disagree on label-free encoding audit')
    return dict(version='privacy_v6_archive_audit.v1',source_commit=expected_source,
        selected=frozen['selected'],initial_state_sha256=frozen['initial_state_sha256'],
        freeze_sha256=result['freeze_sha256'],all_weight_digests_verified=True,
        validation_selection_reproduced=True,all_regression_metrics_reproduced=True,
        candidates=verified,paired_against_baseline=changes,encoding_audit=audits[0],
        pinned_legacy_empty_source_omissions=source_repairs,
        production_approved=False,mode='shadow_only',
        note='Paired scenario bootstrap describes a consumed regression set; no independent quality claim or model/threshold reselection.')


def main():
    p=argparse.ArgumentParser()
    p.add_argument('--artifacts',type=Path,required=True)
    p.add_argument('--expected-source',required=True)
    p.add_argument('--out',type=Path,required=True)
    args=p.parse_args()
    if args.out.exists(): p.error('refusing to overwrite existing archive audit')
    report=analyze(args.artifacts,args.expected_source)
    args.out.parent.mkdir(parents=True,exist_ok=True)
    args.out.write_text(json.dumps(report,indent=2,sort_keys=True,allow_nan=False)+'\n')
    print('V6_AUDIT',json.dumps(report,sort_keys=True,allow_nan=False),flush=True)


if __name__=='__main__':
    main()
