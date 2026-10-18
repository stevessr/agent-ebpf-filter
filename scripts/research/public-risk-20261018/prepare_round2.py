"""Round-specific group-isolated confirmation; old test is regression-only."""
import json,hashlib
from pathlib import Path
from paths import old_round
ROOT=Path(__file__).resolve().parent;OLD=old_round()
def h(s):return hashlib.sha256(s.encode()).hexdigest()
def load(p):return [json.loads(s) for s in p.read_text().splitlines()]
def cap_groups(rows,n):
 groups={}
 for r in rows:groups.setdefault(r['group'],[]).append(r)
 out=[]
 for g in sorted(groups,key=lambda g:h('round2-cap:'+g)):
  if len(out)>=n:break
  out+=groups[g]
 return out

def main():
 datasets={};stats={}
 for task,directory in [('privacy','agent_privacy_data'),('code','data')]:
  ds={s:load(OLD/directory/(s+'.jsonl')) for s in ['train','validation','calibration','test']}
  if task=='code':ds={s:[r for r in rs if r['task']=='vulnerability'] for s,rs in ds.items()}
  train=[];confirm=[]
  for r in ds['train']:
   (confirm if int(h('round2-confirm:'+r['group'])[:8],16)%100<20 else train).append(r)
  datasets[task]={'train':cap_groups(train,4500) if task=='code' else train,
   'validation':cap_groups(ds['validation'],800) if task=='code' else ds['validation'],
   'calibration':cap_groups(ds['calibration'],800) if task=='code' else ds['calibration'],
   'confirmation':confirm,'regression':ds['test']}
  stats[task]={}
  for s,rs in datasets[task].items():
   for r in rs:r['round2_split']=s
   stats[task][s]={'n':len(rs),'positive':sum(r['label'] for r in rs),'groups':len({r['group'] for r in rs})}
  gg={s:{r['group'] for r in rs} for s,rs in datasets[task].items()}
  assert all(not gg[a]&gg[b] for a in gg for b in gg if a!=b)
  dst=ROOT/'data'/task;dst.mkdir(parents=True,exist_ok=True)
  for s,rs in datasets[task].items():(dst/(s+'.jsonl')).write_text(''.join(json.dumps(r)+'\n' for r in rs))
 stats['protocol']={'confirmation':'reserved from old train before new-base/head training; not a new external benchmark','regression':'previously examined old test; exploratory comparison only','code_train_cap':4500,'code_dev_and_cal_cap':800,'seed':20261018}
 stats['sha256']={str(p.relative_to(ROOT)):hashlib.sha256(p.read_bytes()).hexdigest() for p in (ROOT/'data').rglob('*.jsonl')}
 (ROOT/'data/manifest.json').write_text(json.dumps(stats,indent=2));print(json.dumps(stats,indent=2))
if __name__=='__main__':main()
