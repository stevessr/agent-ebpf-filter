"""Observable lexical source/candidate relations; no policy labels or authorization claims."""
import json,re,hashlib
from pathlib import Path
import numpy as np
from paths import ROOT

def evidence(r):
 request,_,rest=r['context'].partition('\nUser memories: ');memory,_,trace=rest.partition('\nPrior tool trajectory: ');candidate=r['candidate'].lower();req=request.lower();source=(memory+' '+trace).lower()
 words=lambda s:set(re.findall(r'\b[a-z]{4,}\b',s))
 numbers=lambda s:set(re.findall(r'\b\d[\d,./-]{2,}\b',s))
 emails=lambda s:set(re.findall(r'[\w.+-]+@[\w.-]+\.[a-z]{2,}',s))
 cw,rw,sw=words(candidate),words(req),words(source);cn,rn,sn=numbers(candidate),numbers(req),numbers(source);ce,re_,se=emails(candidate),emails(req),emails(source)
 v=[min(len(candidate),20000)/20000,len(cw&rw)/max(1,len(cw)),len(cw&(sw-rw))/max(1,len(cw)),len(cw-sw-rw)/max(1,len(cw)),min(len(cn),20)/20,min(len((cn&sn)-rn),20)/20,min(len(cn-sn-rn),20)/20,min(len(ce),10)/10,min(len(ce&re_),10)/10,min(len((ce&se)-re_),10)/10,float(bool(re.search(r'\b\d{3}-\d{2}-\d{4}\b',candidate))),min(len(re.findall(r'https?://',candidate)),10)/10]
 for terms in [('password','api_key','access_token','secret'),('medical','diagnos','therapy','health'),('salary','income','bank','account'),('address','phone','social security','ssn')]:v.append(min(sum(candidate.count(t) for t in terms),10)/10)
 return np.array(v,dtype=np.float32)
def main(base):
 folder=ROOT/'artifacts'/f'privacy_{base}';meta=json.loads((folder/'extraction.json').read_text())
 if meta.get('evidence_augmented'):raise RuntimeError('Already augmented')
 for s in ['train','validation','calibration','confirmation','regression']:
  p=folder/(s+'.npz');d=dict(np.load(p,allow_pickle=False));rows=[json.loads(x) for x in (ROOT/'data/privacy'/(s+'.jsonl')).read_text().splitlines()];assert d['ids'].tolist()==[r['id'] for r in rows];d['X']=np.c_[d['X'],np.stack([evidence(r) for r in rows])];np.savez_compressed(p,**d)
 meta['evidence_augmented']=True;meta['evidence_dimension']=16;meta['evidence_source']='candidate + preceding request/memories/history only; exact lexical matches are not authorization proof';(folder/'extraction.json').write_text(json.dumps(meta,indent=2));print(base,'evidence augmentation complete')
if __name__=='__main__':
 import sys;main(sys.argv[1])
