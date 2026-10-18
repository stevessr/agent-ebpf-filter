"""Finite automatic representation x decision-network search with frozen selection."""
import argparse,copy,hashlib,json,random,time,signal
import numpy as np
import torch
from torch import nn
from sklearn.preprocessing import StandardScaler
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import confusion_matrix,average_precision_score,roc_auc_score
from paths import ROOT,R1
from prepare import rows
torch.set_num_threads(2)
OUT=ROOT/'artifacts';SEED=20261019
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def metrics(y,p,t):
    tn,fp,fn,tp=map(int,confusion_matrix(y,p>=t,labels=[0,1]).ravel());rec=tp/max(1,tp+fn);fpr=fp/max(1,fp+tn)
    return {'n':len(y),'positive':int(sum(y)),'accuracy':(tn+tp)/len(y),'balanced_accuracy':(rec+1-fpr)/2,'F1':2*tp/max(1,2*tp+fp+fn),'recall':rec,'FPR':fpr,'AP':float(average_precision_score(y,p)),'ROC_AUC':float(roc_auc_score(y,p)) if len(np.unique(y))==2 else None,'threshold':float(t),'confusion':{'tn':tn,'fp':fp,'fn':fn,'tp':tp}}
def threshold(y,p,cap=.05):
    values=np.r_[np.unique(p),np.nextafter(float(max(p)),np.inf)];best=(-1,0,1.)
    for t in values:
        fpr=np.mean(p[y==0]>=t)
        if fpr<=cap:
            ba=(np.mean(p[y==1]>=t)+1-fpr)/2;best=max(best,(ba,-fpr,float(t)))
    return best[2]
def objective(m):return m['balanced_accuracy']-max(0,m['FPR']-.10)
def read(task,front,s):return dict(np.load(ROOT/'features'/f'{task}_{front}'/(s+'.npz'),allow_pickle=False))
class Residual(nn.Module):
    def __init__(self,d):
        super().__init__();self.input=nn.Sequential(nn.Linear(d,128),nn.GELU());self.blocks=nn.ModuleList([nn.Sequential(nn.LayerNorm(128),nn.Linear(128,128),nn.GELU(),nn.Dropout(.25),nn.Linear(128,128)) for _ in range(3)]);self.out=nn.Linear(128,1)
    def forward(self,x):
        z=self.input(x)
        for b in self.blocks:z=z+.3*b(z)
        return self.out(z).ravel()
class Gated(nn.Module):
    def __init__(self,d):
        super().__init__();self.gate=nn.Linear(d,2);self.experts=nn.ModuleList([nn.Sequential(nn.Linear(d,128),nn.GELU(),nn.Dropout(.3),nn.Linear(128,64),nn.GELU(),nn.Linear(64,1)) for _ in range(2)])
    def forward(self,x):return (self.gate(x).softmax(-1)*torch.cat([m(x) for m in self.experts],dim=1)).sum(1)
def head(kind,d):return (Residual if kind=='residual' else Gated)(d)
def probability(bundle,X):
    xx=((X-bundle['mean'])/bundle['scale']).astype('float32')
    if bundle['config']['back']=='linear':return 1/(1+np.exp(-np.clip(xx@bundle['coef']+bundle['bias'],-60,60)))
    m=head(bundle['config']['back'],xx.shape[1]);m.load_state_dict(bundle['state']);m.eval()
    with torch.no_grad():return np.concatenate([m(torch.from_numpy(xx[i:i+128])).sigmoid().numpy() for i in range(0,len(xx),128)])
def fit_head(task,cfg,seed):
    t0=time.time();random.seed(seed);np.random.seed(seed);torch.manual_seed(seed)
    ds={s:read(task,cfg['front'],s) for s in ['train','validation','calibration']}
    if cfg.get('white_augmentation'):
        extra=read(task,cfg['front'],'white_train');ds['train']={k:np.concatenate([ds['train'][k],extra[k]]) for k in ds['train']}
    sc=StandardScaler().fit(ds['train']['X']);xx={s:torch.tensor(sc.transform(v['X']),dtype=torch.float32) for s,v in ds.items()};y=torch.tensor(ds['train']['y'],dtype=torch.float32)
    bundle={'config':cfg,'seed':seed,'mean':sc.mean_,'scale':sc.scale_};hist=[]
    if cfg['back']=='linear':
        m=LogisticRegression(C=1,class_weight='balanced',max_iter=1200,random_state=seed).fit(xx['train'].numpy(),y.numpy());bundle.update(coef=m.coef_[0],bias=float(m.intercept_[0]))
    else:
        m=head(cfg['back'],xx['train'].shape[1]);opt=torch.optim.AdamW(m.parameters(),lr=cfg['lr'],weight_decay=.02);best=-1;stale=0;pos=(len(y)-y.sum())/y.sum()
        for ep in range(50):
            m.train()
            for ii in torch.randperm(len(y)).split(128):
                opt.zero_grad(set_to_none=True);loss=nn.functional.binary_cross_entropy_with_logits(m(xx['train'][ii]),y[ii],pos_weight=pos);loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1);opt.step()
            m.eval()
            with torch.no_grad():p=m(xx['validation']).sigmoid().numpy()
            ap=float(average_precision_score(ds['validation']['y'],p));hist.append({'epoch':ep+1,'validation_AP':ap})
            if ap>best:best=ap;state=copy.deepcopy(m.state_dict());stale=0
            else:stale+=1
            if stale>=8 or time.time()-t0>60:break
        bundle['state']=state
    pv=probability(bundle,ds['validation']['X']);pc=probability(bundle,ds['calibration']['X']);t=threshold(ds['calibration']['y'],pc)
    bundle['threshold']=t;mv=metrics(ds['validation']['y'],pv,t)
    record={'config':cfg,'seed':seed,'validation':mv,'calibration':metrics(ds['calibration']['y'],pc,t),'objective':objective(mv),'history':hist,'seconds':time.time()-t0,'training':'frozen front, trained decision head'}
    return bundle,record

class EndToEnd(nn.Module):
    def __init__(self,back):
        super().__init__();from transformers import BertModel
        self.encoder=BertModel.from_pretrained(R1/'base_model',local_files_only=True);self.back=back;h=self.encoder.config.hidden_size
        if back=='cnn':self.convs=nn.ModuleList([nn.Conv1d(h,64,k,padding=k//2) for k in [3,5]]);d=128
        else:d=h
        self.head=nn.Sequential(nn.Dropout(.2),nn.Linear(d,64),nn.GELU(),nn.Linear(64,1))
    def forward(self,ids,mask):
        h=self.encoder(input_ids=ids,attention_mask=mask).last_hidden_state
        if self.back=='cnn':z=torch.cat([nn.functional.gelu(c(h.transpose(1,2))).masked_fill(mask[:,None,:]==0,-1e4).max(2).values for c in self.convs],dim=1)
        else:z=(h*mask[:,:,None]).sum(1)/mask.sum(1)[:,None].clamp(min=1)
        return self.head(z).ravel()
def tokens(task,s):
    p=ROOT/'tokens'/f'{task}_{s}.npz'
    if p.exists():return dict(np.load(p,allow_pickle=False))
    from transformers import BertTokenizerFast
    tok=BertTokenizerFast(vocab_file=str(R1/'base_model/vocab.txt'),do_lower_case=True);rr=rows(task,s);ids=[]
    for r in rr:
        if task=='privacy':a=tok.encode(r['context'],add_special_tokens=False)[:96];b=tok.encode(r['candidate'],add_special_tokens=False);b=b if len(b)<=157 else b[:80]+b[-77:];ii=[101]+a+[102]+b+[102]
        else:
            b=tok.encode(r['text'],add_special_tokens=False);b=b if len(b)<=254 else b[:96]+b[-158:];ii=[101]+b+[102]
        ids.append(ii)
    pack=tok.pad({'input_ids':ids},padding='max_length',max_length=256,return_tensors='np');d={'ids':pack['input_ids'],'mask':pack['attention_mask'],'y':np.array([r['label'] for r in rr]),'groups':np.array([r['group'] for r in rr])};p.parent.mkdir(exist_ok=True);np.savez_compressed(p,**d);return d
def epredict(m,d):
    m.eval();out=[]
    with torch.no_grad():
        for off in range(0,len(d['y']),32):out.append(m(torch.tensor(d['ids'][off:off+32]),torch.tensor(d['mask'][off:off+32])).sigmoid().numpy())
    return np.concatenate(out)
def fit_e2e(task,cfg,seed):
    t0=time.time();random.seed(seed);np.random.seed(seed);torch.manual_seed(seed);ds={s:tokens(task,s) for s in ['train','validation','calibration']}
    if cfg.get('white_augmentation'):
        extra=tokens(task,'white_train');ds['train']={k:np.concatenate([ds['train'][k],extra[k]]) for k in ds['train']}
    m=EndToEnd(cfg['back']);opt=torch.optim.AdamW(m.parameters(),lr=1e-4,weight_decay=.01);y=torch.tensor(ds['train']['y'],dtype=torch.float32);pos=(len(y)-y.sum())/y.sum();history=[];best=-1
    for ep in range(3):
        m.train()
        for ii in torch.randperm(len(y)).split(24):
            opt.zero_grad(set_to_none=True);z=m(torch.tensor(ds['train']['ids'][ii]),torch.tensor(ds['train']['mask'][ii]));loss=nn.functional.binary_cross_entropy_with_logits(z,y[ii],pos_weight=pos);loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1);opt.step()
        pv=epredict(m,ds['validation']);ap=float(average_precision_score(ds['validation']['y'],pv));history.append({'epoch':ep+1,'validation_AP':ap});print(task,cfg,ep+1,round(ap,4),'seconds',round(time.time()-t0),flush=True)
        if ap>best:best=ap;state=copy.deepcopy(m.state_dict())
    m.load_state_dict(state);pv=epredict(m,ds['validation']);pc=epredict(m,ds['calibration']);t=threshold(ds['calibration']['y'],pc);mv=metrics(ds['validation']['y'],pv,t)
    return {'config':cfg,'seed':seed,'state':state,'threshold':t},{'config':cfg,'seed':seed,'validation':mv,'calibration':metrics(ds['calibration']['y'],pc,t),'objective':objective(mv),'history':history,'seconds':time.time()-t0,'training':'all encoder and head parameters trainable','parameters':sum(p.numel() for p in m.parameters())}
def fit(task,cfg,seed):return (fit_e2e if cfg['front']=='finetuned_tiny' else fit_head)(task,cfg,seed)
def configurations(task):
    fronts=['tiny','new','guard','hybrid'] if task=='privacy' else ['tiny','new']
    return [{'front':f,'back':b,**({'lr':lr} if b!='linear' else {})} for f in fronts for b in ['linear','residual','gated'] for lr in ([0] if b=='linear' else [.001,.0003])]
def search(task,stage):
    ledger=OUT/f'{task}_trials.jsonl';assert not (OUT/f'{task}_frozen.json').exists(),'Already frozen'
    prior=[json.loads(l) for l in ledger.read_text().splitlines()] if ledger.exists() else []
    configs=configurations(task) if stage in ['heads','heads_ready'] else [{'front':'finetuned_tiny','back':b} for b in ['mean','cnn']]
    if stage=='heads_ready':configs=[c for c in configs if all((ROOT/'features'/f'{task}_{c["front"]}'/(s+'.npz')).exists() for s in ['train','validation','calibration'])]
    seen={json.dumps(r['config'],sort_keys=True) for r in prior};best=max([r.get('objective',-1e9) for r in prior],default=-1e9)
    for cfg in configs:
        if json.dumps(cfg,sort_keys=True) in seen:continue
        bundle,r=fit(task,cfg,SEED);r['status']='keep' if r['objective']>best else 'discard'
        if r['objective']>best:best=r['objective'];torch.save(bundle,OUT/f'{task}_winner.pt')
        # Save matched front-specific controls for fair architecture comparisons.
        same=[x for x in prior if x['config']['front']==cfg['front']]
        if not same or r['objective']>max(x['objective'] for x in same):torch.save(bundle,OUT/f'{task}_{cfg["front"]}_best.pt')
        with ledger.open('a') as f:f.write(json.dumps(r)+'\n')
        prior.append(r);print(task,cfg,round(r['objective'],4),r['status'],flush=True)
def freeze(task):
    records=[json.loads(l) for l in (OUT/f'{task}_trials.jsonl').read_text().splitlines()];assert len(records)==len(configurations(task))+2
    p=OUT/f'{task}_winner.pt';bundle=torch.load(p,weights_only=False);rep=[]
    for seed in [SEED+1,SEED+2]:
        b,r=fit(task,bundle['config'],seed);q=OUT/f'{task}_seed{seed}.pt';torch.save(b,q);rep.append({**r,'weights_sha256':sha(q)})
    frozen={'task':task,'selected':bundle['config'],'weights_sha256':sha(p),'trial_count':len(records),'trials_sha256':sha(OUT/f'{task}_trials.jsonl'),'replications':rep,'audit_used_for_selection':False,'gate':{'recall':.7,'FPR':.05},'time':time.time()}
    (OUT/f'{task}_frozen.json').write_text(json.dumps(frozen,indent=2));print('FROZEN',task,bundle['config'],flush=True)
def predict_bundle(task,b,s):
    if b['config']['front']=='finetuned_tiny':
        d=tokens(task,s);m=EndToEnd(b['config']['back']);m.load_state_dict(b['state']);p=epredict(m,d)
    else:d=read(task,b['config']['front'],s);p=probability(b,d['X'])
    return d,p
def evaluate(task):
    frozen=json.loads((OUT/f'{task}_frozen.json').read_text());assert sha(OUT/f'{task}_winner.pt')==frozen['weights_sha256']
    paths=[OUT/f'{task}_winner.pt']+[OUT/f'{task}_seed{s}.pt' for s in [SEED+1,SEED+2]]+sorted(OUT.glob(f'{task}_*_best.pt'))+sorted(OUT.glob(f'{task}_control_seed*.pt'));result=[];pred=[]
    for path in paths:
        b=torch.load(path,weights_only=False);rr={'model':path.stem,'config':b['config'],'seed':b['seed'],'splits':{}}
        for s in ['audit','previous_confirmation','regression']:
            d,p=predict_bundle(task,b,s);assert np.isfinite(p).all();mm=metrics(d['y'],p,b['threshold']);mm['gate_passed']=mm['recall']>=.7 and mm['FPR']<=.05;groups=np.unique(d['groups']);ix={g:np.flatnonzero(d['groups']==g) for g in groups};rng=np.random.default_rng(SEED);ci=[]
            for _ in range(500):
                ii=np.concatenate([ix[g] for g in rng.choice(groups,len(groups),replace=True)]);yy=d['y'][ii];pp=p[ii]>=b['threshold']
                if len(np.unique(yy))==2:ci.append((np.mean(pp[yy==1])+np.mean(~pp[yy==0]))/2)
            mm['BA_group_95CI']=np.quantile(ci,[.025,.975]).tolist();rr['splits'][s]=mm
            np.savez_compressed(OUT/f'{path.stem}_{s}_predictions.npz',y=d['y'],p=p,groups=d['groups'],threshold=b['threshold'])
        result.append(rr)
    (OUT/f'{task}_results.json').write_text(json.dumps(result,indent=2));print('EVALUATED',task,flush=True)
if __name__=='__main__':
    ap=argparse.ArgumentParser();ap.add_argument('task',choices=['privacy','code']);ap.add_argument('stage',choices=['heads','heads_ready','e2e','freeze','evaluate']);a=ap.parse_args()
    if a.stage in ['heads','heads_ready','e2e']:search(a.task,a.stage)
    elif a.stage=='freeze':freeze(a.task)
    else:evaluate(a.task)
