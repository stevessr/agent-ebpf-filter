"""Public PrivacyAlign training: retrieved context, segment interaction, ranking.

Tests from previous experiments are consumed regression sets, not new holdouts.
No OpenCode actions or hand-created security labels are used for fitting.
"""
import argparse, hashlib, json, os, random, re, time
from pathlib import Path
os.environ['TOKENIZERS_PARALLELISM']='false'
import numpy as np
import torch
from torch import nn
from transformers import BertModel, BertConfig, BertTokenizerFast
from safetensors.torch import save_file, load_file
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score, roc_auc_score, confusion_matrix
from scipy.stats import beta
import joblib

SEED=20261018
def sha(p): return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def sigmoid(z): return 1/(1+np.exp(-np.clip(z,-60,60)))
def measure(y,p,t):
    tn,fp,fn,tp=confusion_matrix(y,p>t,labels=[0,1]).ravel()
    return dict(n=len(y),tn=int(tn),fp=int(fp),fn=int(fn),tp=int(tp),
        accuracy=float((tn+tp)/len(y)),fpr=float(fp/max(1,tn+fp)),recall=float(tp/max(1,tp+fn)),
        fpr_upper95=float(beta.ppf(.95,fp+1,tn)) if tn else 1.,
        AP=float(average_precision_score(y,p)) if sum(y) else None,
        AUROC=float(roc_auc_score(y,p)) if len(set(y))==2 else None)
def threshold(y,p):
    # Predeclared empirical 1% negative operating point; n~100 cannot certify 1%.
    return float(np.quantile(p[y==0],.99,method='higher'))
def compact(context,candidate):
    """Retain the user goal and retrieve relevant memory/trajectory sentences.

    No labels, split identity, model verdicts, or source filenames enter retrieval.
    """
    context=context[:65536];candidate=candidate[:32768]
    user,sep,rest=context.partition('\nUser memories: ')
    if not sep: return context[:12000]
    terms=set(re.findall(r'[a-zA-Z][a-zA-Z0-9_@.-]{2,}',candidate.lower()))
    chunks=[c for c in re.split(r'(?<=[.!?])\s+|\n',rest) if c.strip()]
    def score(c):
        words=set(re.findall(r'[a-zA-Z][a-zA-Z0-9_@.-]{2,}',c.lower()))
        return len(terms&words)/max(1,len(words))**.5
    selected=sorted(sorted(range(len(chunks)),key=lambda i:(-score(chunks[i]),i))[:8])
    return user+'\nRelevant prior context: '+'\n'.join(chunks[i] for i in selected)
def encode(rows,tok):
    ids=[];tt=[];masks=[]
    for r in rows:
        c=tok.encode(compact(r['context'],r['candidate']),add_special_tokens=False)
        a=tok.encode(r['candidate'],add_special_tokens=False)
        # 128-token goal/retrieved context and 253-token candidate, total 384.
        c=c[:128];a=a if len(a)<=253 else a[:190]+a[-63:]
        seq=[tok.cls_token_id]+c+[tok.sep_token_id]+a+[tok.sep_token_id];n=384-len(seq)
        ids.append(seq+[0]*n);tt.append([0]*(len(c)+2)+[1]*(len(a)+1)+[0]*n);masks.append([1]*len(seq)+[0]*n)
    return dict(input_ids=torch.tensor(ids),token_type_ids=torch.tensor(tt),attention_mask=torch.tensor(masks))
class PairBert(nn.Module):
    def __init__(self,base,pretrained=True):
        super().__init__();self.encoder=BertModel.from_pretrained(base,local_files_only=True) if pretrained else BertModel(BertConfig.from_json_file(str(Path(base)/'config.json')))
        h=self.encoder.config.hidden_size
        self.head=nn.Sequential(nn.LayerNorm(h*4),nn.Linear(h*4,128),nn.GELU(),nn.Dropout(.15),nn.Linear(128,1))
    def forward(self,input_ids,token_type_ids,attention_mask):
        h=self.encoder(input_ids=input_ids,token_type_ids=token_type_ids,attention_mask=attention_mask).last_hidden_state
        pools=[]
        for side in [0,1]:
            m=((token_type_ids==side)&attention_mask.bool()&(input_ids!=101)&(input_ids!=102)).unsqueeze(-1)
            pools.append((h*m).sum(1)/m.sum(1).clamp(min=1))
        c,a=pools
        return self.head(torch.cat([c,a,torch.abs(c-a),c*a],dim=-1)).squeeze(-1)
def predict(m,e):
    m.eval();z=[]
    with torch.inference_mode():
        for off in range(0,len(e['input_ids']),24):z.extend(m(**{k:v[off:off+24] for k,v in e.items()}).tolist())
    return sigmoid(np.asarray(z))
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--assets',type=Path,required=True);ap.add_argument('--mini',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True);torch.set_num_threads(2)
    ds={s:[json.loads(x) for x in (a.assets/'agent_privacy_data'/f'{s}.jsonl').read_text().splitlines()] for s in ['train','validation','calibration','test']}
    y={s:np.asarray([r['label'] for r in rows]) for s,rows in ds.items()}
    protocol=dict(seed=SEED,training='public PrivacyAlign original prepared train only',selection='validation recall at empirical 1% FPR, then AP; calibration separately fixes threshold',test_status='previously consumed regression; never independent',encoders=['BERT-Tiny 2x128','BERT-Mini 4x256'],epochs=6,lr=3e-5,batch=16,loss='unweighted BCE + 0.2 within-scenario opposite-label pair ranking',sparse_C=[.25,1,4],new_labeled_examples=0,production_approved=False)
    (a.out/'protocol.json').write_text(json.dumps(protocol,indent=2)+'\n')
    tok=BertTokenizerFast(vocab_file=str(a.assets/'base_model/vocab.txt'),do_lower_case=True)
    e={s:encode(ds[s],tok) for s in ['train','validation','calibration']};trials=[];predictions={};start=time.perf_counter()
    # Keep opposite-label scenario pairs in the same optimizer batch.
    groups={}
    for i,r in enumerate(ds['train']):groups.setdefault(r['group'],[]).append(i)
    for name,base in [('tiny_pair',a.assets/'base_model'),('mini_pair',a.mini)]:
        random.seed(SEED);np.random.seed(SEED);torch.manual_seed(SEED)
        m=PairBert(base);opt=torch.optim.AdamW(m.parameters(),lr=3e-5,weight_decay=.01);history=[];best=None
        yy=torch.tensor(y['train'],dtype=torch.float32)
        for ep in range(6):
            m.train();gg=list(groups.values());random.shuffle(gg);batches=[];batch=[]
            for g in gg:
                if len(batch)+len(g)>16:batches.append(batch);batch=[]
                batch+=g
            if batch:batches.append(batch)
            losses=[]
            for ii in batches:
                opt.zero_grad(set_to_none=True);z=m(**{k:v[ii] for k,v in e['train'].items()});loss=nn.functional.binary_cross_entropy_with_logits(z,yy[ii]);pairs=[]
                for j,i in enumerate(ii):
                    for k in range(j):
                        if ds['train'][i]['group']==ds['train'][ii[k]]['group'] and y['train'][i]!=y['train'][ii[k]]:
                            pairs.append(nn.functional.softplus(-(z[j]-z[k])*(yy[i]-yy[ii[k]])))
                if pairs:loss=loss+.2*torch.stack(pairs).mean()
                loss.backward();nn.utils.clip_grad_norm_(m.parameters(),1);opt.step();losses.append(loss.item())
            p=predict(m,e['validation']);t=threshold(y['validation'],p);v=measure(y['validation'],p,t);rank=(v['recall'],v['AP']);row=dict(epoch=ep+1,loss=float(np.mean(losses)),validation=v,elapsed_seconds=time.perf_counter()-start);history.append(row);print(name,json.dumps(row),flush=True)
            if best is None or rank>best[0]:
                best=(rank,ep+1);save_file({k:v.detach().contiguous() for k,v in m.state_dict().items()},str(a.out/f'{name}.safetensors'))
        m.load_state_dict(load_file(str(a.out/f'{name}.safetensors')));predictions[name]={s:predict(m,e[s]) for s in ['validation','calibration']};trials.append(dict(name=name,base=str(base),epochs=history,selected_epoch=best[1],validation=history[best[1]-1]['validation'],weights_sha256=sha(a.out/f'{name}.safetensors')))
        del m,opt
    # Alternative front/back ends; fitted vocabulary sees train only.
    for view in ['candidate','retrieved_pair']:
        texts={s:[r['candidate'] if view=='candidate' else compact(r['context'],r['candidate'])+'\nACTION '+r['candidate'] for r in rows] for s,rows in ds.items() if s!='test'}
        for front,opts in [('word',dict(analyzer='word',ngram_range=(1,2))),('char',dict(analyzer='char',ngram_range=(3,5)))]:
            vector=TfidfVectorizer(**opts,min_df=2,max_features=40000,sublinear_tf=True,dtype=np.float32);x={'train':vector.fit_transform(texts['train'])};x.update({s:vector.transform(texts[s]) for s in ['validation','calibration']})
            for c in [.25,1,4]:
                name=f'{view}_{front}_C{c}';m=LogisticRegression(C=c,max_iter=1500,random_state=SEED).fit(x['train'],y['train']);pp={s:m.predict_proba(x[s])[:,1] for s in ['validation','calibration']};t=threshold(y['validation'],pp['validation']);v=measure(y['validation'],pp['validation'],t);joblib.dump((vector,m),a.out/f'{name}.joblib');predictions[name]=pp;trials.append(dict(name=name,view=view,front=front,C=c,validation=v,weights_sha256=sha(a.out/f'{name}.joblib')));print(name,json.dumps(v),flush=True)
    selected=max(trials,key=lambda r:(r['validation']['recall'],r['validation']['AP']));name=selected['name'];t=threshold(y['calibration'],predictions[name]['calibration']);freeze=dict(selected=name,threshold=t,comparison='strict >',artifact_sha256=selected['weights_sha256'],protocol_sha256=sha(a.out/'protocol.json'),selection=selected,calibration=measure(y['calibration'],predictions[name]['calibration'],t),source_hashes={s:sha(a.assets/'agent_privacy_data'/f'{s}.jsonl') for s in ds},mode='shadow_only',production_approved=False)
    (a.out/'trials.json').write_text(json.dumps(trials,indent=2)+'\n');(a.out/'freeze.json').write_text(json.dumps(freeze,indent=2)+'\n');print('FROZEN',name,t,flush=True)
    # All selection is now fixed before consumed-test regression evaluation.
    test=ds['test']
    if name in ['tiny_pair','mini_pair']:
        base=a.assets/'base_model' if name=='tiny_pair' else a.mini;m=PairBert(base,False);m.load_state_dict(load_file(str(a.out/f'{name}.safetensors')));p=predict(m,encode(test,tok))
    else:
        vector,m=joblib.load(a.out/f'{name}.joblib');texts=[r['candidate'] if selected['view']=='candidate' else compact(r['context'],r['candidate'])+'\nACTION '+r['candidate'] for r in test];p=m.predict_proba(vector.transform(texts))[:,1]
    result=dict(freeze=freeze,consumed_test=measure(y['test'],p,t),actual_neural_epochs=12,actual_fits=14,seconds=time.perf_counter()-start)
    (a.out/'results.json').write_text(json.dumps(result,indent=2)+'\n');(a.out/'test_predictions.jsonl').write_text(''.join(json.dumps(dict(id=r['id'],group=r['group'],label=r['label'],score=float(q),above_threshold=bool(q>t)))+'\n' for r,q in zip(test,p)));print(json.dumps(result),flush=True)
if __name__=='__main__':main()
