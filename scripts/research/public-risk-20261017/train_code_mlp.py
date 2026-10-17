"""Targeted nonlinear lexical baseline after weak code-encoder validation results."""
import argparse,json,time
import numpy as np
import torch
from torch import nn
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.decomposition import TruncatedSVD
from sklearn.preprocessing import StandardScaler
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score,f1_score
from safetensors.torch import save_file,load_file
import joblib
from train_public import ROOT,SEED,seed,rows,sig,metrics,threshold

def net():return nn.Sequential(nn.Linear(256,128),nn.GELU(),nn.Dropout(.3),nn.Linear(128,64),nn.GELU(),nn.Dropout(.3),nn.Linear(64,1))
def forward(m,x):
    m.eval()
    with torch.no_grad():return m(x).ravel().numpy()
def main():
    p=argparse.ArgumentParser();p.add_argument('--stage',choices=['train','evaluate'],default='train');a=p.parse_args();torch.set_num_threads(2);out=ROOT/'artifacts'
    ds={s:[r for r in rows(s) if r['task']=='vulnerability'] for s in ['train','validation','calibration','test']};y={s:np.array([r['label'] for r in rr],dtype=np.float32) for s,rr in ds.items()}
    if a.stage=='train':
        start=time.perf_counter();v=TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=30000,sublinear_tf=True,dtype=np.float32);xt=v.fit_transform([r['text'] for r in ds['train']]);svd=TruncatedSVD(256,random_state=SEED,n_iter=5);sc=StandardScaler();x={'train':torch.from_numpy(sc.fit_transform(svd.fit_transform(xt)).astype('float32')),'validation':torch.from_numpy(sc.transform(svd.transform(v.transform([r['text'] for r in ds['validation']]))).astype('float32'))}
        print('train-only representation fitted',time.perf_counter()-start,flush=True);joblib.dump((v,svd,sc),out/'code_mlp_representation.joblib')
        best=None;configs={};yy=torch.from_numpy(y['train'])
        for lr in [1e-3,3e-4]:
            seed(SEED);m=net();opt=torch.optim.AdamW(m.parameters(),lr=lr,weight_decay=.01);bestap=-1;stale=0;hist=[];name='code_mlp_'+str(lr)
            for ep in range(30):
                m.train();ii=np.random.permutation(len(yy))
                for off in range(0,len(ii),128):
                    idx=ii[off:off+128];opt.zero_grad(set_to_none=True);loss=nn.functional.binary_cross_entropy_with_logits(m(x['train'][idx]).ravel(),yy[idx]);loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1);opt.step()
                ap=average_precision_score(y['validation'],sig(forward(m,x['validation'])));hist.append({'epoch':ep+1,'validation_AP':float(ap)});print(name,hist[-1],flush=True)
                if ap>bestap:bestap=ap;state={k:v.detach().clone() for k,v in m.state_dict().items()};bestep=ep+1;stale=0
                else:stale+=1
                if stale>=5:break
            configs[name]={'lr':lr,'completed_epochs':ep+1,'selected_epoch':bestep,'validation_AP':float(bestap),'history':hist}
            if best is None or bestap>best[0]:best=(bestap,name,state)
        save_file({k:v.contiguous() for k,v in best[2].items()},str(out/'code_mlp.safetensors'));(out/'code_mlp_training.json').write_text(json.dumps({'selected':best[1],'configs':configs,'parameters':sum(p.numel() for p in net().parameters()),'training_seconds':time.perf_counter()-start,'scope':'targeted additional architecture chosen after weak transformer code validation; no test-driven selection'},indent=2));return
    v,svd,sc=joblib.load(out/'code_mlp_representation.joblib');m=net();m.load_state_dict(load_file(str(out/'code_mlp.safetensors')));z={s:forward(m,torch.from_numpy(sc.transform(svd.transform(v.transform([r['text'] for r in ds[s]]))).astype('float32'))) for s in ['validation','calibration','test']}
    cal=LogisticRegression(C=1,max_iter=1000).fit(z['calibration'].reshape(-1,1),y['calibration']);prob={s:cal.predict_proba(z[s].reshape(-1,1))[:,1] for s in ['validation','test']};t=threshold(y['validation'],prob['validation']);t5=float(np.nextafter(np.quantile(prob['validation'][y['validation']==0],.95,method='higher'),np.inf))
    rng=np.random.default_rng(SEED);groups=sorted({r['group'] for r in ds['test']});ix={g:[i for i,r in enumerate(ds['test']) if r['group']==g] for g in groups};ci=[]
    for _ in range(500):
        ii=np.concatenate([ix[g] for g in rng.choice(groups,len(groups),replace=True)]);ci.append(f1_score(y['test'][ii],prob['test'][ii]>=t,zero_division=0))
    result={'test':metrics(y['test'],prob['test'],t),'validation':metrics(y['validation'],prob['validation'],t),'test_at_0_5':metrics(y['test'],prob['test'],.5),'test_at_validation_FPR5':metrics(y['test'],prob['test'],t5),'F1_group_bootstrap_95CI':np.quantile(ci,[.025,.975]).tolist(),'calibration':{'slope':float(cal.coef_[0,0]),'intercept':float(cal.intercept_[0])}}
    (out/'code_mlp_results.json').write_text(json.dumps(result,indent=2));print(json.dumps(result,indent=2))
if __name__=='__main__':main()
