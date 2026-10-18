import argparse,json,hashlib
from pathlib import Path
import numpy as np
ROOT=Path(__file__).resolve().parent

def main():
 cert={'kind':'executable experimental invariants, not formal model correctness','group_isolation':{},'source_hashes_verified':True,'features_finite':{},'frozen_weights_verified':{},'selection_constraints_verified':{}}
 manifest=json.loads((ROOT/'data/manifest.json').read_text())
 for file,digest in manifest['sha256'].items():assert hashlib.sha256((ROOT/file).read_bytes()).hexdigest()==digest,file
 for task in ['privacy','code']:
  group={}
  for s in ['train','validation','calibration','confirmation','regression']:
   rows=[json.loads(x) for x in (ROOT/'data'/task/(s+'.jsonl')).read_text().splitlines()];group[s]={r['group'] for r in rows}
  assert all(not group[a]&group[b] for a in group for b in group if a!=b);cert['group_isolation'][task]=True
  for base in ['tiny','new']:
   folder=ROOT/'artifacts'/f'{task}_{base}'
   if not folder.exists():continue
   for p in folder.glob('*.npz'):
    v=dict(np.load(p,allow_pickle=False));assert np.isfinite(v['X']).all();assert len(v['X'])==len(v['y'])==len(v['ids'])==len(v['groups'])
    source=[json.loads(x) for x in (ROOT/'data'/task/(p.stem+'.jsonl')).read_text().splitlines()];assert v['ids'].tolist()==[r['id'] for r in source];assert v['y'].tolist()==[r['label'] for r in source]
   cert['features_finite'][task+'_'+base]=True
   freeze=ROOT/'artifacts'/f'{task}_{base}_frozen.json'
   if freeze.exists():
    f=json.loads(freeze.read_text());assert f['confirmation_accessed'] is False and f['regression_accessed'] is False
    assert hashlib.sha256((ROOT/'artifacts'/f'{task}_{base}_winner.joblib').read_bytes()).hexdigest()==f['weights_sha256'];cert['frozen_weights_verified'][task+'_'+base]=True
    best=-float('inf')
    for r in f['trials']:
     if r['status'] in ['seed_replication','error']:continue
     assert r['calibration']['FPR']<=.1+1e-12
     expected='keep' if r['objective']>best else 'discard';assert r['status']==expected
     best=max(best,r['objective'])
    assert abs(best-f['selection_objective'])<1e-12;cert['selection_constraints_verified'][task+'_'+base]=True
    result=ROOT/'artifacts'/f'{task}_{base}_results.json'
    if result.exists():
     rr=json.loads(result.read_text());assert rr['frozen']==f
     for rep in rr['replications']:
      for part in rep['splits'].values():
       for name in ['metrics','low_FPR']:
        mm=part[name];assert sum(mm['confusion'].values())==mm['n']
 (ROOT/'artifacts/certificate.json').write_text(json.dumps(cert,indent=2));print(json.dumps(cert,indent=2))
if __name__=='__main__':main()
