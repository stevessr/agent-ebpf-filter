"""Bounded, validation-only head search with append-only trial certificates."""
import argparse,json,time,hashlib,random,copy,signal
from pathlib import Path
import numpy as np
import torch
from torch import nn
from sklearn.preprocessing import StandardScaler
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import (balanced_accuracy_score,accuracy_score,f1_score,precision_score,recall_score,average_precision_score,roc_auc_score,confusion_matrix)
import joblib
ROOT=Path(__file__).resolve().parent;SEED=20261018;torch.set_num_threads(2)
def read(task,base,s):return dict(np.load(ROOT/'artifacts'/f'{task}_{base}'/(s+'.npz'),allow_pickle=False))
def sig(z):return 1/(1+np.exp(-np.clip(z,-60,60)))
def metrics(y,p,t):
 tn,fp,fn,tp=confusion_matrix(y,p>=t,labels=[0,1]).ravel()
 return {'n':len(y),'positive':int(y.sum()),'threshold':float(t),'accuracy':accuracy_score(y,p>=t),'balanced_accuracy':balanced_accuracy_score(y,p>=t),'F1':f1_score(y,p>=t,zero_division=0),'macro_F1':f1_score(y,p>=t,average='macro',zero_division=0),'precision':precision_score(y,p>=t,zero_division=0),'recall':recall_score(y,p>=t,zero_division=0),'FPR':float(fp/max(1,fp+tn)),'AP':average_precision_score(y,p),'ROC_AUC':roc_auc_score(y,p),'confusion':{'tn':int(tn),'fp':int(fp),'fn':int(fn),'tp':int(tp)}}
def threshold(y,p,cap=.1):
 ts=np.unique(np.r_[np.quantile(p,np.linspace(0,1,201)),np.nextafter(p.max(),np.inf)])
 # Explicit constraints avoid the almost-all-positive F1 solution from round one.
 good=[]
 for t in ts:
  fp=float(np.mean(p[y==0]>=t))
  if fp<=cap:good.append((balanced_accuracy_score(y,p>=t),-fp,float(t)))
 return max(good)[2]
def take(X,view,d):
 if view in ['candidate','mean']:return X[:,:d]
 if view=='context':return X[:,:4*d]
 if view=='retrieval':return X[:,:6*d]
 return X
class Head(nn.Module):
 def __init__(self,dim,hidden=128):
  super().__init__();self.layers=nn.Sequential(nn.Linear(dim,hidden),nn.GELU(),nn.Dropout(.3),nn.Linear(hidden,64),nn.GELU(),nn.Dropout(.3),nn.Linear(64,1))
 def forward(self,x):return self.layers(x).ravel()
def score(bundle,X):
 x=bundle['scaler'].transform(take(X,bundle['config']['view'],bundle['d'])).astype('float32')
 if bundle['config']['kind']=='linear':return bundle['model'].predict_proba(x)[:,1]
 m=Head(x.shape[1]);m.load_state_dict(bundle['state']);m.eval()
 with torch.no_grad():return np.concatenate([torch.sigmoid(m(torch.from_numpy(x[i:i+256]))).numpy() for i in range(0,len(x),256)])
def fit(ds,cfg,d,seed):
 t0=time.perf_counter();np.random.seed(seed);random.seed(seed);torch.manual_seed(seed)
 scaler=StandardScaler();x={s:take(v['X'],cfg['view'],d) for s,v in ds.items()};x['train']=scaler.fit_transform(x['train']).astype('float32')
 for s in ['validation','calibration']:x[s]=scaler.transform(x[s]).astype('float32')
 bundle={'config':cfg,'d':d,'scaler':scaler,'seed':seed,'representation_fit':'train-only StandardScaler'}
 if cfg['kind']=='linear':
  model=LogisticRegression(C=cfg['C'],class_weight='balanced',max_iter=1000,random_state=seed).fit(x['train'],ds['train']['y']);bundle['model']=model
  pv=model.predict_proba(x['validation'])[:,1];pc=model.predict_proba(x['calibration'])[:,1];history=[];epochs=None
 else:
  m=Head(x['train'].shape[1]);opt=torch.optim.AdamW(m.parameters(),lr=cfg['lr'],weight_decay=.01);tx=torch.from_numpy(x['train']);yy=torch.tensor(ds['train']['y'],dtype=torch.float32);vx=torch.from_numpy(x['validation']);cx=torch.from_numpy(x['calibration']);best=-1;stale=0;history=[];state=None
  for ep in range(60):
   m.train();ids=np.random.permutation(len(yy))
   for off in range(0,len(ids),128):
    ii=ids[off:off+128];opt.zero_grad(set_to_none=True);loss=nn.functional.binary_cross_entropy_with_logits(m(tx[ii]),yy[ii]);loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1);opt.step()
   m.eval()
   with torch.no_grad():pv=torch.sigmoid(m(vx)).numpy()
   ap=average_precision_score(ds['validation']['y'],pv);history.append({'epoch':ep+1,'validation_AP':float(ap)})
   if ap>best:best=ap;state={k:v.detach().clone() for k,v in m.state_dict().items()};stale=0;bestep=ep+1
   else:stale+=1
   if stale>=7 or time.perf_counter()-t0>=30:break
  m.load_state_dict(state);bundle['state']=state;epochs=bestep
  with torch.no_grad():pv=torch.sigmoid(m(vx)).numpy();pc=torch.sigmoid(m(cx)).numpy()
 assert np.isfinite(pv).all() and np.isfinite(pc).all()
 t=threshold(ds['calibration']['y'],pc,.1);t5=threshold(ds['calibration']['y'],pc,.05)
 bundle['threshold']=t;bundle['low_FPR_threshold']=t5
 mv=metrics(ds['validation']['y'],pv,t);objective=mv['balanced_accuracy']-.5*max(0,mv['FPR']-.2)
 return bundle,{'config':cfg,'seed':seed,'validation':mv,'objective':float(objective),'calibration':metrics(ds['calibration']['y'],pc,t),'selected_epoch':epochs,'history':history,'wall_seconds':time.perf_counter()-t0,'budget_seconds':30,'finite_certificate':True,'threshold_source':'calibration-only','training_source':'train-only'}
def configs(task):
 if task=='privacy':return [{'kind':'linear','view':v,'C':c} for v in ['candidate','retrieval','evidence'] for c in [.1,1]]+[{'kind':'mlp','view':v,'lr':.001} for v in ['context','evidence']]
 return [{'kind':'linear','view':v,'C':c} for v in ['mean','meanmax'] for c in [.1,1,10]]+[{'kind':'mlp','view':'meanmax','lr':lr} for lr in [.001,.0003]]
def search(task,base):
 if (ROOT/'artifacts'/f'{task}_{base}_results.json').exists():raise RuntimeError('Evaluation already performed: create a new declared round before searching again')
 if (ROOT/'artifacts'/f'{task}_{base}_trials.jsonl').exists():raise RuntimeError('Trial ledger already exists: preserve it and start a separate declared run')
 ds={s:read(task,base,s) for s in ['train','validation','calibration']};meta=json.loads((ROOT/'artifacts'/f'{task}_{base}'/'extraction.json').read_text());d=meta['hidden_size'];out=ROOT/'artifacts';best=-float('inf');selected=None
 records=[]
 for i,cfg in enumerate(configs(task)):
  def timeout(_signum,_frame):raise TimeoutError('30 second head-trial budget exceeded')
  signal.signal(signal.SIGALRM,timeout);signal.alarm(30)
  try:
   bundle,r=fit(ds,cfg,d,SEED)
  except Exception as e:
   signal.alarm(0);r={'config':cfg,'task':task,'base':base,'trial':i,'status':'error','error':str(e)};records.append(r)
   with (out/f'{task}_{base}_trials.jsonl').open('a') as f:f.write(json.dumps(r)+'\n')
   print('FAILED',r,flush=True);continue
  finally:signal.alarm(0)
  r.update({'task':task,'base':base,'trial':i,'status':'keep' if r['objective']>best else 'discard'})
  if r['objective']>best:best=r['objective'];selected=cfg;joblib.dump(bundle,out/f'{task}_{base}_winner.joblib')
  records.append(r)
  with (out/f'{task}_{base}_trials.jsonl').open('a') as f:f.write(json.dumps(r)+'\n')
  print(task,base,i,cfg,'objective',round(r['objective'],4),r['status'],flush=True)
 if selected is None:raise RuntimeError('No valid trial survived; no model selected')
 # Seed checks repeat only the selected configuration, no architecture re-selection.
 for seed in [SEED+1,SEED+2]:
  bundle,r=fit(ds,selected,d,seed);joblib.dump(bundle,out/f'{task}_{base}_seed{seed}.joblib');records.append({**r,'task':task,'base':base,'status':'seed_replication'})
 winner=joblib.load(out/f'{task}_{base}_winner.joblib')
 frozen={'task':task,'base':base,'selected_config':selected,'selection_objective':best,'seed':SEED,'weights_sha256':hashlib.sha256((out/f'{task}_{base}_winner.joblib').read_bytes()).hexdigest(),'feature_extraction':meta,'trials':records,'confirmation_accessed':False,'regression_accessed':False,'deploy_gate':{'min_recall':.7,'max_FPR':.05}}
 (out/f'{task}_{base}_frozen.json').write_text(json.dumps(frozen,indent=2));print('FROZEN',task,base,selected,flush=True)

def evaluate(task,base):
 out=ROOT/'artifacts';freeze=json.loads((out/f'{task}_{base}_frozen.json').read_text());paths=[out/f'{task}_{base}_winner.joblib']+[out/f'{task}_{base}_seed{s}.joblib' for s in [SEED+1,SEED+2]];results=[];preds=[]
 for path in paths:
  bundle=joblib.load(path);rr={'seed':bundle['seed'],'splits':{}}
  for split in ['confirmation','regression']:
   ds=read(task,base,split);p=score(bundle,ds['X']);m=metrics(ds['y'],p,bundle['threshold']);m5=metrics(ds['y'],p,bundle['low_FPR_threshold']);ci=[];groups=np.unique(ds['groups']);ix={g:np.flatnonzero(ds['groups']==g) for g in groups};rng=np.random.default_rng(SEED)
   for _ in range(500):
    ii=np.concatenate([ix[g] for g in rng.choice(groups,len(groups),replace=True)]);ci.append(balanced_accuracy_score(ds['y'][ii],p[ii]>=bundle['threshold']))
   rr['splits'][split]={'metrics':m,'low_FPR':m5,'balanced_accuracy_group_bootstrap_95CI':np.quantile(ci,[.025,.975]).tolist(),'deployment_gate_passed':bool(m['recall']>=.7 and m['FPR']<=.05),'low_FPR_gate_passed':bool(m5['recall']>=.7 and m5['FPR']<=.05)}
   for i,pp in enumerate(p):preds.append({'task':task,'base':base,'seed':bundle['seed'],'split':split,'id':str(ds['ids'][i]),'label':int(ds['y'][i]),'score':float(pp),'prediction':int(pp>=bundle['threshold'])})
  results.append(rr)
 report={'frozen':freeze,'replications':results,'confirmation_seed_mean':{k:float(np.mean([r['splits']['confirmation']['metrics'][k] for r in results])) for k in ['balanced_accuracy','accuracy','F1','recall','FPR','AP']},'confirmation_seed_std':{k:float(np.std([r['splits']['confirmation']['metrics'][k] for r in results],ddof=1)) for k in ['balanced_accuracy','accuracy','F1','recall','FPR','AP']}}
 (out/f'{task}_{base}_results.json').write_text(json.dumps(report,indent=2));(out/f'{task}_{base}_predictions.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in preds));print(task,base,json.dumps(report['confirmation_seed_mean']),flush=True)
if __name__=='__main__':
 a=argparse.ArgumentParser();a.add_argument('--task',choices=['privacy','code'],required=True);a.add_argument('--base',choices=['tiny','new'],required=True);a.add_argument('--stage',choices=['search','evaluate'],required=True);v=a.parse_args();(search if v.stage=='search' else evaluate)(v.task,v.base)
