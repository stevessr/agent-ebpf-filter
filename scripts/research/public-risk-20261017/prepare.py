"""Prepare public supervised tasks. Never executes corpus contents."""
import collections,gzip,hashlib,json,re
from pathlib import Path
import pyarrow.parquet as pq
ROOT=Path(__file__).resolve().parent

def digest(x):return hashlib.sha256(x.encode()).hexdigest()
def norm(x):return re.sub(r'\s+',' ',x).strip().lower()
def bucket(x):return int(digest(x)[:8],16)%100

def prepare():
 raw=ROOT/'raw';out=ROOT/'data';out.mkdir(exist_ok=True)
 allrows=[];stats={}
 # Keep official test prompts completely disjoint from training/calibration/selection.
 b={s:[json.loads(x) for x in gzip.open(raw/f'beaver_{s}.jsonl.gz','rt')] for s in ['train','test']}
 held={norm(r['prompt']) for r in b['test']};seen=set();removed=collections.Counter()
 for s,rs in b.items():
  for i,r in enumerate(rs):
   g=norm(r['prompt']);text='Question: '+r['prompt']+'\nCandidate response: '+r['response'];key=norm(text)
   if s=='train' and g in held:removed['official_test_prompt_overlap']+=1;continue
   if key in seen:removed['exact_text_duplicate']+=1;continue
   seen.add(key);v=bucket('privacy:'+g)
   split='test' if s=='test' else ('train' if v<75 else 'validation' if v<88 else 'calibration')
   allrows.append({'id':f'beaver-{s}-{i}','task':'privacy','group':digest(g),'split':split,'text':text,'label':int(r['category']['privacy_violation'])})
 stats['privacy_removed']=dict(removed)
 # Preserve official validation/test, remove commit and code overlaps from train.
 d={s:pq.read_table(raw/f'devign_{s}.parquet').to_pylist() for s in ['train','validation','test']}
 test_commits={r['project']+':'+r['commit_id'] for r in d['test']};test_code={norm(r['func']) for r in d['test']}
 val_commits={r['project']+':'+r['commit_id'] for r in d['validation']};val_code={norm(r['func']) for r in d['validation']}
 removed=collections.Counter();seen=set()
 for s in ['test','validation','train']:
  for r in d[s]:
   code=norm(r['func']);g=r['project']+':'+r['commit_id']
   if s!='test' and (g in test_commits or code in test_code):removed[s+'_test_overlap']+=1;continue
   if s=='train' and (g in val_commits or code in val_code):removed['train_validation_overlap']+=1;continue
   if code in seen:removed['exact_code_duplicate']+=1;continue
   seen.add(code);split=s
   if s=='train' and bucket('vulnerability:'+g)>=86:split='calibration'
   allrows.append({'id':f'devign-{s}-{r["id"]}','task':'vulnerability','group':digest(g),'split':split,'text':r['func'],'label':int(r['target'])})
 stats['vulnerability_removed']=dict(removed)
 # Public AgentHarm prompt classification. Not its official agent-execution benchmark.
 # All harmful/benign, hint/detail variants of each original behavior share a split.
 for label,name in [(1,'harm_public'),(0,'harm_benign')]:
  rs=json.loads((raw/(name+'.json')).read_text())['behaviors']
  for i,r in enumerate(rs):
   g=str(r['id_original']);v=bucket('harm:'+g)
   split='train' if v<60 else 'validation' if v<80 else 'test'
   allrows.append({'id':f'agentharm-{label}-{i}','task':'harmful_request','group':g,'split':split,'text':r['prompt'],'label':label,'category':r['category']})
 # Verify actual group isolation; metadata and targets never enter model inputs.
 for task in ['privacy','vulnerability','harmful_request']:
  rr=[r for r in allrows if r['task']==task]
  groups={s:{r['group'] for r in rr if r['split']==s} for s in ['train','validation','calibration','test']}
  for a,ga in groups.items():
   for b,gb in groups.items():
    if a!=b:assert not ga&gb,(task,a,b)
  stats[task]={s:{'n':sum(r['split']==s for r in rr),'positive':sum(r['label'] for r in rr if r['split']==s),'groups':len(groups[s])} for s in groups}
 for s in ['train','validation','calibration','test']:
  (out/(s+'.jsonl')).write_text(''.join(json.dumps(r,ensure_ascii=False)+'\n' for r in allrows if r['split']==s))
 used=['beaver_train.jsonl.gz','beaver_test.jsonl.gz','devign_train.parquet','devign_validation.parquet','devign_test.parquet','harm_public.json','harm_benign.json']
 stats['raw_sha256']={name:hashlib.sha256((raw/name).read_bytes()).hexdigest() for name in used}
 stats['prepared_sha256']={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in out.glob('*.jsonl')}
 (out/'manifest.json').write_text(json.dumps(stats,indent=2));print(json.dumps(stats,indent=2))
if __name__=='__main__':prepare()
