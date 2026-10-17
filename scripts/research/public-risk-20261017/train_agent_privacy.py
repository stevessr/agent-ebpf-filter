"""Full transformer adaptation on public, human-labelled candidate tool actions."""
import argparse,json,random,time
from pathlib import Path
import numpy as np
import torch
from torch import nn
from transformers import BertTokenizerFast
from safetensors.torch import save_file,load_file
from sklearn.linear_model import LogisticRegression
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.metrics import average_precision_score,f1_score
import joblib
from train_public import RiskBert,seed,sig,metrics,threshold,ROOT,SEED

def read(s):return [json.loads(x) for x in (ROOT/'agent_privacy_data'/f'{s}.jsonl').read_text().splitlines()]
def truncate(ids,n):return ids if len(ids)<=n else ids[:n//3]+ids[-(n-n//3):]
def encode(rs,tok):
    seq=[];types=[];mask=[];truncated={'context':0,'candidate':0}
    for r in rs:
        context=tok.encode(r['context'],add_special_tokens=False);candidate=tok.encode(r['candidate'],add_special_tokens=False)
        truncated['context']+=len(context)>190;truncated['candidate']+=len(candidate)>126
        c=truncate(context,190);a=truncate(candidate,126);ids=[tok.cls_token_id]+c+[tok.sep_token_id]+a+[tok.sep_token_id]
        tt=[0]*(len(c)+2)+[1]*(len(a)+1);n=320-len(ids)
        seq.append(ids+[tok.pad_token_id]*n);types.append(tt+[0]*n);mask.append([1]*len(ids)+[0]*n)
    return {'input_ids':torch.tensor(seq),'token_type_ids':torch.tensor(types),'attention_mask':torch.tensor(mask)},truncated

def predict(m,e):
    m.eval();zz=[]
    with torch.no_grad():
        for off in range(0,len(e['input_ids']),64):zz.append(m(**{k:v[off:off+64] for k,v in e.items()})[:,0].numpy())
    return np.concatenate(zz)

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--base-only',action='store_true');parser.add_argument('--resume',action='store_true');args=parser.parse_args()
    torch.set_num_threads(2)
    ds={s:read(s) for s in ['train','validation','calibration','test']};out=ROOT/'artifacts';tok=BertTokenizerFast(vocab_file=str(ROOT/'base_model/vocab.txt'),do_lower_case=True)
    # Test representations/labels are used only after all checkpoints are selected below.
    enc={};trunc={}
    for s in ['train','validation','calibration']:enc[s],trunc[s]=encode(ds[s],tok)
    y={s:np.array([r['label'] for r in rr]) for s,rr in ds.items()};report={'scope':'PrivacyAlign candidate final-action leakage, majority human vote, uncertain ties excluded','models':{},'training':{},'truncation':trunc,'max_tokens':320}
    nnmodels={}
    # Domain transfer is a predeclared optimization: BeaverTails/Devign/AgentHarm encoder -> PrivacyAlign.
    for name,transfer,weight in [('agent_privacy_base',False,False),('agent_privacy_transfer_balanced',True,True)]:
        if args.base_only and transfer:break
        cached=out/f'{name}_training.json'
        if args.resume and cached.exists() and (out/f'{name}.safetensors').exists():
            m=RiskBert();m.load_state_dict(load_file(str(out/f'{name}.safetensors')));nnmodels[name]=m;report['training'][name]=json.loads(cached.read_text());continue
        seed(SEED);m=RiskBert()
        if transfer:m.load_state_dict(load_file(str(out/'bert_balanced.safetensors')))
        opt=torch.optim.AdamW(m.parameters(),lr=1e-4,weight_decay=.01)
        yy=torch.tensor(y['train'],dtype=torch.float32);pw=torch.tensor(float((len(yy)-yy.sum())/yy.sum()) if weight else 1.)
        best=-1;history=[];start=time.perf_counter()
        for ep in range(5):
            m.train();ids=np.random.permutation(len(yy));losses=[]
            for off in range(0,len(ids),32):
                ii=ids[off:off+32];opt.zero_grad(set_to_none=True);z=m(**{k:v[ii] for k,v in enc['train'].items()})[:,0]
                loss=nn.functional.binary_cross_entropy_with_logits(z,yy[ii],pos_weight=pw);loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1);opt.step();losses.append(loss.item())
            ap=average_precision_score(y['validation'],sig(predict(m,enc['validation'])))
            history.append({'epoch':ep+1,'loss':float(np.mean(losses)),'validation_AP':float(ap),'elapsed_seconds':time.perf_counter()-start});print(name,json.dumps(history[-1]),flush=True)
            if ap>best:best=ap;state={k:v.detach().clone() for k,v in m.state_dict().items()};bestep=ep+1
        save_file({k:v.contiguous() for k,v in state.items()},str(out/f'{name}.safetensors'));m.load_state_dict(state);nnmodels[name]=m
        report['training'][name]={'epochs':5,'selected_epoch':bestep,'lr':1e-4,'pos_weight':pw.item(),'transfer_from':'bert_balanced' if transfer else 'google/bert_uncased_L-2_H-128_A-2','history':history,'training_seconds':time.perf_counter()-start}
        cached.write_text(json.dumps(report['training'][name],indent=2))
    if args.base_only:return
    enc['test'],trunc['test']=encode(ds['test'],tok)
    predictions=[];raw_logits={}
    def assess(name,z):
        raw_logits[name]=z
        cal=LogisticRegression(C=1,max_iter=1000).fit(z['calibration'].reshape(-1,1),y['calibration'])
        p={s:cal.predict_proba(z[s].reshape(-1,1))[:,1] for s in ['validation','test']};t=threshold(y['validation'],p['validation'])
        m=metrics(y['test'],p['test'],t);ci=[];rng=np.random.default_rng(SEED);groups=sorted({r['group'] for r in ds['test']});ix={g:[i for i,r in enumerate(ds['test']) if r['group']==g] for g in groups}
        for _ in range(500):
            ii=np.concatenate([ix[g] for g in rng.choice(groups,len(groups),replace=True)]);ci.append(f1_score(y['test'][ii],p['test'][ii]>=t,zero_division=0))
        report['models'][name]={'test':m,'test_at_0_5':metrics(y['test'],p['test'],.5),'validation':metrics(y['validation'],p['validation'],t),'F1_group_bootstrap_95CI':np.quantile(ci,[.025,.975]).tolist(),'calibration':{'slope':float(cal.coef_[0,0]),'intercept':float(cal.intercept_[0])}}
        t5=float(np.nextafter(np.quantile(p['validation'][y['validation']==0],.95,method='higher'),np.inf))
        report['models'][name]['test_at_validation_FPR5']=metrics(y['test'],p['test'],t5)
        for r,pp in zip(ds['test'],p['test']):predictions.append({'model':name,'id':r['id'],'group':r['group'],'label':r['label'],'score':float(pp),'prediction':int(pp>=t)})
    for name,m in nnmodels.items():assess(name,{s:predict(m,enc[s]) for s in ['validation','calibration','test']})
    # No same-domain learning: measures transfer of the auxiliary model without adaptation.
    m=RiskBert();m.load_state_dict(load_file(str(out/'bert_balanced.safetensors')));assess('agent_privacy_frozen_auxiliary',{s:predict(m,enc[s]) for s in ['validation','calibration','test']})
    v=TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=30000,sublinear_tf=True);xx={'train':v.fit_transform([r['text'] for r in ds['train']])}
    xx.update({s:v.transform([r['text'] for r in ds[s]]) for s in ['validation','calibration','test']});best=None
    for c in [.25,1,4]:
        cl=LogisticRegression(C=c,class_weight='balanced',max_iter=1000,random_state=SEED).fit(xx['train'],y['train']);ap=average_precision_score(y['validation'],cl.predict_proba(xx['validation'])[:,1])
        if best is None or ap>best[0]:best=(ap,c,cl)
    report['tfidf_C']=best[1];joblib.dump((v,best[2]),out/'tfidf_agent_privacy.joblib');assess('agent_privacy_tfidf',{s:best[2].decision_function(xx[s]) for s in ['validation','calibration','test']})
    # Candidate-only ablation reduces irrelevant shared context; select C on validation.
    v=TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=30000,sublinear_tf=True);xx={'train':v.fit_transform([r['candidate'] for r in ds['train']])}
    xx.update({s:v.transform([r['candidate'] for r in ds[s]]) for s in ['validation','calibration','test']});best=None
    for c in [.25,1,4]:
        cl=LogisticRegression(C=c,class_weight='balanced',max_iter=1000,random_state=SEED).fit(xx['train'],y['train']);ap=average_precision_score(y['validation'],cl.predict_proba(xx['validation'])[:,1])
        if best is None or ap>best[0]:best=(ap,c,cl)
    report['candidate_only_tfidf_C']=best[1];joblib.dump((v,best[2]),out/'tfidf_agent_privacy_candidate.joblib');assess('agent_privacy_candidate_only_tfidf',{s:best[2].decision_function(xx[s]) for s in ['validation','calibration','test']})
    blend=None
    for neural in ['agent_privacy_base','agent_privacy_transfer_balanced']:
        for sparse in ['agent_privacy_tfidf','agent_privacy_candidate_only_tfidf']:
            for alpha in [0.,.25,.5,.75,1.]:
                pv=alpha*sig(raw_logits[neural]['validation'])+(1-alpha)*sig(raw_logits[sparse]['validation'])
                ap=average_precision_score(y['validation'],pv)
                if blend is None or ap>blend[0]:blend=(ap,neural,sparse,alpha)
    ap,neural,sparse,alpha=blend;report['blend_config']={'neural':neural,'sparse':sparse,'alpha':alpha,'validation_AP':float(ap)}
    z={}
    for s in ['validation','calibration','test']:
        pp=np.clip(alpha*sig(raw_logits[neural][s])+(1-alpha)*sig(raw_logits[sparse][s]),1e-7,1-1e-7);z[s]=np.log(pp/(1-pp))
    assess('agent_privacy_validation_selected_blend',z)
    prior=float(y['train'].mean());prior_logit=float(np.log(prior/(1-prior)));assess('agent_privacy_constant_prevalence',{s:np.full(len(y[s]),prior_logit) for s in ['validation','calibration','test']})
    (out/'agent_privacy_results.json').write_text(json.dumps(report,indent=2));(out/'agent_privacy_test_predictions.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in predictions))
    print(json.dumps({k:v['test'] for k,v in report['models'].items()},indent=2),flush=True)
if __name__=='__main__':main()
