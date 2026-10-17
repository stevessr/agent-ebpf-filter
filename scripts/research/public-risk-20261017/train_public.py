"""Public-data transformer finetuning; offline scores only, no enforcement."""
import argparse,copy,json,os,random,time,platform,hashlib
from pathlib import Path
import numpy as np
import torch
from torch import nn
from transformers import BertModel,BertTokenizerFast,BertConfig
from safetensors.torch import save_file,load_file
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import (accuracy_score,balanced_accuracy_score,f1_score,precision_score,recall_score,roc_auc_score,average_precision_score,brier_score_loss,confusion_matrix)
ROOT=Path(__file__).resolve().parent
TASKS=['privacy','vulnerability','harmful_request'];SEED=20261017
os.environ['TOKENIZERS_PARALLELISM']='false'
torch.set_num_threads(4)

def seed(n):random.seed(n);np.random.seed(n);torch.manual_seed(n)
def rows(s):return [json.loads(x) for x in (ROOT/'data'/f'{s}.jsonl').read_text().splitlines()]
def sig(x):return 1/(1+np.exp(-np.clip(x,-60,60)))
def logit(p):return np.log(np.clip(p,1e-7,1-1e-7)/np.clip(1-p,1e-7,1))
def threshold(y,p):
    ts=np.unique(np.r_[.5,np.quantile(p,np.linspace(0,1,201))])
    return float(max(ts,key=lambda t:(f1_score(y,p>=t,zero_division=0),-abs(t-.5))))
def metrics(y,p,t):
    tn,fp,fn,tp=confusion_matrix(y,p>=t,labels=[0,1]).ravel()
    return {'n':len(y),'positive':int(sum(y)),'threshold':float(t),'accuracy':accuracy_score(y,p>=t),
      'balanced_accuracy':balanced_accuracy_score(y,p>=t),'F1_positive':f1_score(y,p>=t,zero_division=0),
      'macro_F1':f1_score(y,p>=t,average='macro',zero_division=0),'precision':precision_score(y,p>=t,zero_division=0),
      'recall':recall_score(y,p>=t,zero_division=0),'FPR':float(fp/max(1,tn+fp)),
      'PR_AUC_AP':average_precision_score(y,p),'ROC_AUC':roc_auc_score(y,p),'Brier':brier_score_loss(y,p),
      'confusion':{'tn':int(tn),'fp':int(fp),'fn':int(fn),'tp':int(tp)}}

class RiskBert(nn.Module):
    def __init__(self,pretrained=True):
        super().__init__();self.encoder=(BertModel.from_pretrained(ROOT/'base_model',local_files_only=True) if pretrained else BertModel(BertConfig.from_json_file(str(ROOT/'base_model/config.json'))))
        self.head=nn.Sequential(nn.Dropout(.2),nn.Linear(128,64),nn.GELU(),nn.Dropout(.2),nn.Linear(64,len(TASKS)))
    def forward(self,input_ids,attention_mask,token_type_ids):
        h=self.encoder(input_ids=input_ids,attention_mask=attention_mask,token_type_ids=token_type_ids).last_hidden_state
        mask=attention_mask.unsqueeze(-1);h=(h*mask).sum(1)/mask.sum(1).clamp(min=1)
        return self.head(h)

def encoded(rs,tok,maxlen):
    e=tok([r['text'] for r in rs],padding='max_length',truncation=True,max_length=maxlen,return_tensors='pt')
    return {k:v for k,v in e.items() if k in ['input_ids','attention_mask','token_type_ids']}
def infer(m,e,ids=None):
    m.eval();ps=[]
    if ids is None:ids=np.arange(len(e['input_ids']))
    with torch.no_grad():
        for off in range(0,len(ids),128):ps.append(m(**{k:v[ids[off:off+128]] for k,v in e.items()}).numpy())
    return np.concatenate(ps)
def task_probs(z,rs):return {t:z[[i for i,r in enumerate(rs) if r['task']==t],j] for j,t in enumerate(TASKS)}

def fit_neural(name,ds,enc,epochs,lr,weighted):
    seed(SEED);m=RiskBert();n=sum(p.numel() for p in m.parameters())
    opt=torch.optim.AdamW(m.parameters(),lr=lr,weight_decay=.01)
    yy=torch.tensor([r['label'] for r in ds['train']],dtype=torch.float32)
    tt=torch.tensor([TASKS.index(r['task']) for r in ds['train']])
    pw=[]
    for j in range(len(TASKS)):
        pos=yy[tt==j].sum().item();count=(tt==j).sum().item();pw.append(min(5.,((count-pos)/max(1,pos))**.5) if weighted else 1.)
    pw=torch.tensor(pw)
    # More optimization exposure for the small AgentHarm training portion, no new examples.
    base=list(range(len(yy)));harm=[i for i,r in enumerate(ds['train']) if r['task']=='harmful_request']
    base+=harm*7
    best=-1;beststate=None;hist=[];start=time.perf_counter()
    for ep in range(epochs):
        m.train();random.shuffle(base);losses=[]
        for off in range(0,len(base),64):
            idx=base[off:off+64];opt.zero_grad(set_to_none=True)
            z=m(**{k:v[idx] for k,v in enc['train'].items()})[torch.arange(len(idx)),tt[idx]]
            loss=nn.functional.binary_cross_entropy_with_logits(z,yy[idx],pos_weight=pw[tt[idx]])
            loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1.0);opt.step();losses.append(loss.item())
        vz=task_probs(infer(m,enc['validation']),ds['validation'])
        ap={t:average_precision_score([r['label'] for r in ds['validation'] if r['task']==t],sig(vz[t])) for t in TASKS}
        avg=float(np.mean(list(ap.values())))
        hist.append({'epoch':ep+1,'loss':float(np.mean(losses)),'validation_AP':ap,'macro_AP':avg,'elapsed_seconds':time.perf_counter()-start})
        print(name,json.dumps(hist[-1]),flush=True)
        if avg>best:best=avg;beststate={k:v.detach().clone() for k,v in m.state_dict().items()};bestep=ep+1
    m.load_state_dict(beststate);save_file({k:v.contiguous() for k,v in beststate.items()},str(ROOT/'artifacts'/f'{name}.safetensors'))
    (ROOT/'artifacts'/f'{name}_history.json').write_text(json.dumps(hist,indent=2))
    return m,{'parameters':n,'epochs_completed':epochs,'selected_epoch':bestep,'learning_rate':lr,'pos_weight':pw.tolist(),'training_seconds':time.perf_counter()-start,'selection':'mean validation AP across three tasks'}

def main():
    a=argparse.ArgumentParser();a.add_argument('--stage',choices=['train','evaluate'],default='train');args=a.parse_args()
    out=ROOT/'artifacts';out.mkdir(exist_ok=True);ds={s:rows(s) for s in ['train','validation','calibration','test']}
    if args.stage=='train':
        tok=BertTokenizerFast(vocab_file=str(ROOT/'base_model/vocab.txt'),do_lower_case=True)
        enc={s:encoded(ds[s],tok,192) for s in ['train','validation']}
        configs={}
        for name,epochs,lr,w in [('bert_unweighted',3,3e-4,False),('bert_balanced',5,1e-4,True)]:
            _,configs[name]=fit_neural(name,ds,enc,epochs,lr,w)
            (out/'training_config.json').write_text(json.dumps(configs,indent=2))
        print('TRAINING_FINISHED',flush=True)
        return
    # Only after neural hyperparameters/checkpoints are selected, access final test labels.
    tok=BertTokenizerFast(vocab_file=str(ROOT/'base_model/vocab.txt'),do_lower_case=True)
    enc={s:encoded(ds[s],tok,192) for s in ['validation','calibration','test']}
    report={'seed':SEED,'scope':'public auxiliary text/code tasks, NOT online agent action decisions','tasks':TASKS,'models':{},
        'environment':{'python':platform.python_version(),'torch':torch.__version__,'threads':torch.get_num_threads(),'device':'CPU'},
        'training_config':json.loads((out/'training_config.json').read_text())}
    predrows=[]
    def assess(name,zby):
        result={}
        for task in TASKS:
            rs={s:[r for r in ds[s] if r['task']==task] for s in ['validation','calibration','test']}
            y={s:np.array([r['label'] for r in rrs]) for s,rrs in rs.items()}
            z={s:zby[s][task] for s in rs}
            calibrator=None
            if len(y['calibration']):
                calibrator=LogisticRegression(C=1,max_iter=1000).fit(z['calibration'].reshape(-1,1),y['calibration'])
                p={s:calibrator.predict_proba(z[s].reshape(-1,1))[:,1] for s in ['validation','test']}
            else:p={s:sig(z[s]) for s in ['validation','test']}
            t=threshold(y['validation'],p['validation'])
            result[task]={'test':metrics(y['test'],p['test'],t),'test_at_0_5':metrics(y['test'],p['test'],.5),'validation':metrics(y['validation'],p['validation'],t),
              'calibration':None if calibrator is None else {'slope':float(calibrator.coef_[0,0]),'intercept':float(calibrator.intercept_[0])}}
            t5=float(np.nextafter(np.quantile(p['validation'][y['validation']==0],.95,method='higher'),np.inf))
            result[task]['test_at_validation_FPR5']=metrics(y['test'],p['test'],t5)
            # Original behavior/prompt/commit clustered uncertainty, not IID row bootstrap.
            rng=np.random.default_rng(SEED);groups=sorted({r['group'] for r in rs['test']});ix={g:[i for i,r in enumerate(rs['test']) if r['group']==g] for g in groups};cis=[]
            for _ in range(500):
                sample=np.concatenate([ix[g] for g in rng.choice(groups,len(groups),replace=True)])
                cis.append(f1_score(y['test'][sample],p['test'][sample]>=t,zero_division=0))
            result[task]['F1_group_bootstrap_95CI']=np.quantile(cis,[.025,.975]).tolist()
            for r,pp in zip(rs['test'],p['test']):predrows.append({'model':name,'id':r['id'],'task':task,'group':r['group'],'label':r['label'],'score':float(pp),'prediction':int(pp>=t)})
        report['models'][name]=result
    neural_raw={}
    for name in ['bert_unweighted','bert_balanced']:
        m=RiskBert();m.load_state_dict(load_file(str(out/f'{name}.safetensors')))
        zby={s:task_probs(infer(m,enc[s]),ds[s]) for s in enc}
        neural_raw[name]=zby
        assess(name,zby)
    # Fit vocabulary/IDF only on training. C selected per task on validation AP.
    baseline_config={};zb={s:{} for s in ['validation','calibration','test']}
    import joblib
    for task in TASKS:
        rr={s:[r for r in ds[s] if r['task']==task] for s in ds}
        v=TfidfVectorizer(ngram_range=(1,2),min_df=2,max_features=40000,sublinear_tf=True)
        xt=v.fit_transform([r['text'] for r in rr['train']]);xs={s:v.transform([r['text'] for r in rr[s]]) for s in ['validation','test']}
        if rr['calibration']:xs['calibration']=v.transform([r['text'] for r in rr['calibration']])
        yy=[r['label'] for r in rr['train']];best=None
        for c in [.25,1,4]:
            cl=LogisticRegression(C=c,class_weight='balanced',max_iter=1000,random_state=SEED).fit(xt,yy)
            ap=average_precision_score([r['label'] for r in rr['validation']],cl.predict_proba(xs['validation'])[:,1])
            if best is None or ap>best[0]:best=(ap,c,cl)
        baseline_config[task]={'C':best[1],'validation_AP':best[0]};cl=best[2]
        for s in zb:zb[s][task]=cl.decision_function(xs[s]) if s in xs else np.empty(0)
        joblib.dump((v,cl),out/f'tfidf_{task}.joblib')
    report['baseline_config']=baseline_config;assess('tfidf_logistic',zb)
    # Validation-only blend search; includes pure baseline/neural endpoints.
    mixed={s:{} for s in zb};blend_config={}
    for task in TASKS:
        yv=np.array([r['label'] for r in ds['validation'] if r['task']==task]);best=None
        for name,zby in neural_raw.items():
            for alpha in [0.,.25,.5,.75,1.]:
                pv=alpha*sig(zby['validation'][task])+(1-alpha)*sig(zb['validation'][task])
                ap=average_precision_score(yv,pv)
                if best is None or ap>best[0]:best=(ap,name,alpha)
        _,name,alpha=best;blend_config[task]={'neural':name,'alpha':alpha,'validation_AP':float(best[0])}
        for s in mixed:mixed[s][task]=logit(alpha*sig(neural_raw[name][s][task])+(1-alpha)*sig(zb[s][task]))
    report['blend_config']=blend_config;assess('validation_selected_blend',mixed)
    constant={s:{} for s in ['validation','calibration','test']}
    for task in TASKS:
        prevalence=np.mean([r['label'] for r in ds['train'] if r['task']==task])
        for s in constant:constant[s][task]=np.full(sum(r['task']==task for r in ds[s]),logit(prevalence))
    assess('constant_prevalence',constant)
    (out/'results.json').write_text(json.dumps(report,indent=2))
    (out/'test_predictions.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in predrows))
    print(json.dumps({n:{t:r['test'] for t,r in v.items()} for n,v in report['models'].items()},indent=2),flush=True)
if __name__=='__main__':main()
