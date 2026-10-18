import json,hashlib,itertools
import numpy as np
from paths import ROOT,R2
SPLITS=['train','validation','calibration','audit','previous_confirmation','regression']
def sha(s):return hashlib.sha256(s.encode()).hexdigest()
def rows(task,s):return [json.loads(l) for l in (ROOT/'data'/task/(s+'.jsonl')).read_text().splitlines()]
def main():
    stats={}
    for task in ['privacy','code']:
        old={s:[json.loads(l) for l in (R2/'data'/task/(s+'.jsonl')).read_text().splitlines()] for s in ['train','validation','calibration','confirmation','regression']}
        keep=np.array([int(sha('round3-audit:'+r['group'])[:8],16)%100>=20 for r in old['train']])
        ds={**{s:old[s] for s in ['validation','calibration','regression']},'previous_confirmation':old['confirmation'],'train':[r for r,k in zip(old['train'],keep) if k],'audit':[r for r,k in zip(old['train'],keep) if not k]}
        groups={s:{r['group'] for r in rs} for s,rs in ds.items()}
        assert all(not groups[a]&groups[b] for a,b in itertools.combinations(groups,2))
        dst=ROOT/'data'/task;dst.mkdir(parents=True,exist_ok=True)
        for s,rs in ds.items():(dst/(s+'.jsonl')).write_text(''.join(json.dumps(r)+'\n' for r in rs))
        stats[task]={s:{'n':len(rs),'positive':sum(r['label'] for r in rs),'groups':len(groups[s])} for s,rs in ds.items()}
        for base in ['tiny','new']:
            dest=ROOT/'features'/f'{task}_{base}';dest.mkdir(parents=True,exist_ok=True)
            for s,rs in ds.items():
                orig='train' if s=='audit' else 'confirmation' if s=='previous_confirmation' else s
                d=dict(np.load(R2/'artifacts'/f'{task}_{base}'/(orig+'.npz'),allow_pickle=False))
                ix=np.flatnonzero(~keep if s=='audit' else keep) if orig=='train' else np.arange(len(rs))
                d={k:v[ix] for k,v in d.items()};assert d['ids'].tolist()==[r['id'] for r in rs]
                np.savez_compressed(dest/(s+'.npz'),**d)
    stats['protocol']='audit newly held out for this round only; not external or previously unexposed corpus'
    stats['data_sha256']={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in (ROOT/'data').rglob('*.jsonl')}
    (ROOT/'artifacts/data_manifest.json').write_text(json.dumps(stats,indent=2));print(json.dumps(stats,indent=2))
if __name__=='__main__':main()
