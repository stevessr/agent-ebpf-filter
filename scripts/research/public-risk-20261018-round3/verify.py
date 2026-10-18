import hashlib,itertools,json
import numpy as np
from paths import ROOT
from train import OUT,metrics,SEED
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def main():
    checks={};manifest=json.loads((OUT/'data_manifest.json').read_text())
    for p,h in manifest['data_sha256'].items():assert sha(ROOT/p)==h
    checks['data_hashes']=True
    for task,n in [('privacy',22),('code',12)]:
        trials=[json.loads(l) for l in (OUT/f'{task}_trials.jsonl').read_text().splitlines()];assert len(trials)==n;assert len({json.dumps(r['config'],sort_keys=True) for r in trials})==n
        best=-1e9
        for r in trials:
            assert r['status']==('keep' if r['objective']>best else 'discard');best=max(best,r['objective']);assert r['calibration']['FPR']<=.05
        freeze=json.loads((OUT/f'{task}_frozen.json').read_text());assert sha(OUT/f'{task}_winner.pt')==freeze['weights_sha256'];assert sha(OUT/f'{task}_trials.jsonl')==freeze['trials_sha256']
        for r in freeze['replications']:assert sha(OUT/f'{task}_seed{r["seed"]}.pt')==r['weights_sha256'] and r['calibration']['FPR']<=.05
        for r in json.loads((OUT/f'{task}_results.json').read_text()):
            for split,m in r['splits'].items():
                d=np.load(OUT/f'{r["model"]}_{split}_predictions.npz',allow_pickle=False);mm=metrics(d['y'],d['p'],float(d['threshold']));assert m['confusion']==mm['confusion'];assert np.isfinite(d['p']).all()
        for r in json.loads((OUT/f'{task}_controls_frozen.json').read_text())['replications']:assert sha(OUT/f'{task}_control_seed{r["seed"]}.pt')==r['weights_sha256']
    checks.update(trial_counts_and_unique_configs=True,keep_discard_monotonicity=True,calibration_FPR_cap=True,frozen_checkpoints=True,recomputed_confusion_matrices=True)
    white=[json.loads(l) for l in (ROOT/'white/records.jsonl').read_text().splitlines()];groups={s:{r['family'] for r in white if r['split']==s and r['accepted']} for s in ['train','white_audit']};assert not groups['train']&groups['white_audit']
    for r in white:
        p=ROOT/'white/runs'/r['id'];assert sha(p/'events.jsonl')==r['event_sha256']
        if r['accepted']:assert all(r['checks'].values()) and r['cost_reported']==0 and r['label_vulnerability'] is None
    for r in json.loads((OUT/'white_frozen.json').read_text())['replications']:assert sha(OUT/f'privacy_white_seed{r["seed"]}.pt')==r['weights_sha256'] and r['calibration']['FPR']<=.05
    checks.update(white_task_family_isolation=True,white_trace_hashes_and_acceptance=True,white_frozen_weights=True)
    for r in json.loads((OUT/'exports.json').read_text()):assert sha(ROOT/'release'/r['file'])==r['sha256'] and r['max_prediction_error']<1e-6
    checks['safe_tensor_export_parity']=True
    result={'checks':checks,'all_passed':all(checks.values()),'scope':'executable experiment-integrity checks, not a formal proof of model correctness','source_sha256':{p.name:sha(p) for p in ROOT.glob('*.py')}}
    (OUT/'verification.json').write_text(json.dumps(result,indent=2));print(json.dumps(result,indent=2))
if __name__=='__main__':main()
