"""Export numeric-only trained heads and verify against local fitted models."""
import json,hashlib
from pathlib import Path
import numpy as np
import torch
import joblib
from autosearch import ROOT,Head,take,score,read,sig

def json_score(head,X):
 if X.shape[1]!=head['raw_feature_dim'] or not np.isfinite(X).all():raise ValueError('Feature contract mismatch')
 d=head['hidden_size'];x=take(X,head['config']['view'],d)
 x=((x-np.array(head['scaler_mean']))/np.array(head['scaler_scale'])).astype('float32')
 if head['config']['kind']=='linear':return sig(x@np.array(head['coef'])+head['intercept'])
 m=Head(x.shape[1]);m.load_state_dict({k:torch.tensor(v,dtype=torch.float32) for k,v in head['state'].items()});m.eval()
 with torch.no_grad():return torch.sigmoid(m(torch.from_numpy(x))).numpy()
def main():
 exported={}
 for task in ['privacy','code']:
  for base in ['tiny','new']:
   p=ROOT/'artifacts'/f'{task}_{base}_winner.joblib';b=joblib.load(p);data=read(task,base,'validation')
   head={'task':task,'base':base,'config':b['config'],'hidden_size':b['d'],'raw_feature_dim':int(data['X'].shape[1]),'scaler_mean':b['scaler'].mean_.tolist(),'scaler_scale':b['scaler'].scale_.tolist(),'threshold':b['threshold'],'low_FPR_threshold':b['low_FPR_threshold'],'automatic_enforcement':False,'calibrated_incident_probability':False}
   if b['config']['kind']=='linear':head.update(coef=b['model'].coef_[0].tolist(),intercept=float(b['model'].intercept_[0]))
   else:head['state']={k:v.tolist() for k,v in b['state'].items()}
   out=ROOT/'artifacts'/f'{task}_{base}.head.json';out.write_text(json.dumps(head))
   loaded=json.loads(out.read_text());expected=score(b,data['X'][:10]);actual=json_score(loaded,data['X'][:10]);delta=float(np.max(np.abs(expected-actual)));assert delta<1e-5,delta
   exported[out.name]={'sha256':hashlib.sha256(out.read_bytes()).hexdigest(),'prediction_max_abs_error':delta,'numeric_only':True}
 (ROOT/'artifacts/head_exports.json').write_text(json.dumps(exported,indent=2));print(json.dumps(exported,indent=2))
if __name__=='__main__':main()
