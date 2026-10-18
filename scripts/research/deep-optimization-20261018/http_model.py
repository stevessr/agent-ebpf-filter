"""Multi-source public HTTP training; all request strings are offline data.

ECML + previously consumed CSIC test become development corpora explicitly.
Original CSIC normalTrafficTraining is held out for a fresh benign-only audit.
No claim that historical synthetic HTTP labels are kernel behavior outcomes.
"""
import argparse, hashlib, importlib.util, json, re, time
from pathlib import Path
from collections import Counter,defaultdict
from urllib.parse import unquote_plus, urlsplit
import numpy as np
import joblib
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression,SGDClassifier
from sklearn.ensemble import HistGradientBoostingClassifier
from scipy.sparse import hstack,csr_matrix
from privacy import measure,threshold,sha

HERE=Path(__file__).resolve().parent
spec=importlib.util.spec_from_file_location('previous',HERE.parent/'behavior-production-20261018/train_production.py');previous=importlib.util.module_from_spec(spec);spec.loader.exec_module(previous)
KEYWORDS=set('select union from where insert into delete update drop table sleep benchmark script svg iframe onerror onload javascript alert eval document cookie window location passwd shadow bin bash cmd powershell gopher exec system wget curl waitfor information_schema concat cast load_file outfile jndi ldap'.split())
def decode(s):
    for _ in range(2):s=unquote_plus(s,errors='replace')
    return s
def view(event,kind):
    h=event['http'];target=h['target'];p=urlsplit(target)
    # Remove authority from absolute-form request targets before learned features.
    target=p.path+('?' + p.query if p.query else '')
    if kind=='parameters':target=p.query
    text=target+'\n'+h['body'].strip()
    if kind=='syntax':
        text+='\n'+'\n'.join(x['value'] for x in h['headers'])
        text=decode(text).lower()
        text=re.sub(r'[a-z_][a-z_0-9]*',lambda m:m[0] if m[0] in KEYWORDS else 'w',text)
        text=re.sub(r'\d+','n',text)
    return text[:16384]
def structural(s):
    d=decode(s).lower();n=max(1,len(d))
    vals=[len(s),len(d),len(s)-len(d),len(d.split()),len(set(d)),sum(ord(c)>127 for c in d)]
    for c in "'\"<>;=()/\\%&|+*-{}":vals.extend([d.count(c),d.count(c)/n])
    vals.extend(d.count(w) for w in sorted(KEYWORDS));return np.log1p(np.maximum(0,vals))
def transform(bundle,events):
    strings=[view(e,bundle['view']) for e in events]
    if bundle['front']=='structural':return np.asarray([structural(s) for s in strings])
    return bundle['vectorizer'].transform(strings)
def fresh(root):
    p=root/'OriginalDataSets/csic_2010/normalTrafficTraining.txt';data=p.read_bytes().decode('latin1').replace('\r\n','\n');starts=list(re.finditer(r'^(?:GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH) .* HTTP/1\.[01]\n',data,re.M));out=[];rejected=0
    for i,m in enumerate(starts):
        h=previous.http(data[m.start():starts[i+1].start() if i+1<len(starts) else len(data)])
        if h is None:rejected+=1
        else:out.append((dict(source='fresh_csic_normal',http=h),0,'normal'))
    return out,dict(file_sha256=sha(p),parsed=len(out),rejected=rejected)
def template_key(event):
    return hashlib.sha256(previous.old.template(view(event,'payload')).encode()).hexdigest()
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--data',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True);start=time.perf_counter()
    protocol=dict(seed=20261018,training_sources=['ECML original train/test repartitioned','CSIC original normal/anomalous test (previously consumed, now development)'],fresh_benign_audit='CSIC original normalTrafficTraining, never used for fitting or threshold selection',views=['payload','parameters','syntax'],fronts=['char3-5 TFIDF 50000 train-only vocabulary','word1-2 TFIDF 50000 train-only vocabulary','structural counts and ratios'],backs=['logistic C=.5,2','histogram gradient boosting leaves15,31'],selection='maximize worst-source validation recall at source-wise empirical 1% FPR, then macro AP',threshold='max of source-wise calibration empirical 99th negative percentiles; strict >',mode='shadow_only',production_approved=False,limitations=['No independent attack-source holdout in this round.','Payload/parameter views exclude request headers; syntax includes headers but loses literal identifiers.','Fresh benign corpus shares generator/application with CSIC development; overlaps removed and reported.','Training input truncation at 16384 characters; not an abstention-safe production decoder.'])
    (a.out/'protocol.json').write_text(json.dumps(protocol,indent=2)+'\n')
    rr,audit_e=previous.ecml(a.data);cc,audit_c=previous.csic(a.data);raw=rr+cc;rows,dups=previous.dedupe(raw);del raw,rr,cc
    # Entire normalized templates stay together across both sources.
    groups=np.array([template_key(r[0]) for r in rows]);parts,seed=previous.old.split_groups(np.array([r[1] for r in rows]),groups);ys=np.array([r[1] for r in rows]);sources=np.array([r[0]['source'] for r in rows]);trials=[];models={}
    audit=dict(ecml=audit_e,csic=audit_c,deduplication=dups,split_seed=seed,parts={s:dict(n=len(ii),positive=int(ys[ii].sum()),groups=len(set(groups[ii])),sources=dict(Counter(sources[ii]))) for s,ii in parts.items()});(a.out/'data_audit.json').write_text(json.dumps(audit,indent=2)+'\n');print('DATA',json.dumps(audit),flush=True)
    for kind in protocol['views']:
        strings=[view(r[0],kind) for r in rows]
        for front in ['char','word','structural']:
            if front=='structural':xx=np.asarray([structural(s) for s in strings],dtype=np.float32);vector=None;params=[15,31]
            else:
                vector=TfidfVectorizer(analyzer=front,ngram_range=(3,5) if front=='char' else (1,2),min_df=3,max_features=50000,sublinear_tf=True,dtype=np.float32)
                vector.fit([strings[i] for i in parts['train']]);xx=vector.transform(strings);params=[.5,2]
            for param in params:
                name=f'{kind}_{front}_{param}'
                if front=='structural':m=HistGradientBoostingClassifier(max_iter=160,max_leaf_nodes=param,learning_rate=.08,l2_regularization=3,min_samples_leaf=40,early_stopping=False,random_state=20261018)
                else:m=LogisticRegression(C=param,max_iter=600,solver='liblinear',random_state=20261018)
                m.fit(xx[parts['train']],ys[parts['train']]);vi=parts['validation'];p=m.predict_proba(xx[vi])[:,1];vs={}
                for source in sorted(set(sources)):
                    ii=sources[vi]==source;t=threshold(ys[vi][ii],p[ii]);vs[source]=measure(ys[vi][ii],p[ii],t)
                rank=(min(v['recall'] for v in vs.values()),np.mean([v['AP'] for v in vs.values()]));bundle=dict(view=kind,front=front,vectorizer=vector,model=m);joblib.dump(bundle,a.out/f'{name}.joblib');trial=dict(name=name,view=kind,front=front,param=param,validation=vs,rank=[float(q) for q in rank],weights_sha256=sha(a.out/f'{name}.joblib'));trials.append(trial);print('FIT',json.dumps(trial),flush=True)
            del xx,vector
    selected=max(trials,key=lambda r:tuple(r['rank']));name=selected['name'];bundle=joblib.load(a.out/f'{name}.joblib');ci=parts['calibration'];cp=bundle['model'].predict_proba(transform(bundle,[rows[i][0] for i in ci]))[:,1];ts={source:threshold(ys[ci][sources[ci]==source],cp[sources[ci]==source]) for source in sorted(set(sources))};t=max(ts.values());freeze=dict(selected=name,threshold=t,source_thresholds=ts,comparison='strict >',artifact_sha256=selected['weights_sha256'],protocol_sha256=sha(a.out/'protocol.json'),selection=selected,mode='shadow_only',production_approved=False)
    (a.out/'trials.json').write_text(json.dumps(trials,indent=2)+'\n');(a.out/'freeze.json').write_text(json.dumps(freeze,indent=2)+'\n');print('FROZEN',json.dumps(freeze),flush=True)
    ti=parts['test'];pp=bundle['model'].predict_proba(transform(bundle,[rows[i][0] for i in ti]))[:,1];results=dict(freeze=freeze,consumed_regression={source:measure(ys[ti][sources[ti]==source],pp[sources[ti]==source],t) for source in sorted(set(sources))},actual_fits=len(trials))
    # Holdout is opened only after all models and thresholds are frozen.
    fr,fa=fresh(a.data);fr,fd=previous.dedupe(fr);seen={previous.key(r[0]) for r in rows};semantic={hashlib.sha256(view(r[0],'payload').encode()).hexdigest() for r in rows};overlap=Counter();keep=[]
    group_set=set(groups)
    for r in fr:
        if previous.key(r[0]) in seen:overlap['exact_request']+=1;continue
        if hashlib.sha256(view(r[0],'payload').encode()).hexdigest() in semantic:overlap['same_payload']+=1;continue
        if template_key(r[0]) in group_set:overlap['template_overlap_kept']+=1
        keep.append(r)
    preds=[]
    for off in range(0,len(keep),512):preds.extend(bundle['model'].predict_proba(transform(bundle,[r[0] for r in keep[off:off+512]]))[:,1].tolist())
    results['fresh_benign_audit']=dict(parse=fa,dedupe=fd,overlap=dict(overlap),metrics=measure(np.zeros(len(keep),dtype=int),np.asarray(preds),t));results['seconds']=time.perf_counter()-start
    (a.out/'results.json').write_text(json.dumps(results,indent=2)+'\n');(a.out/'fresh_predictions.jsonl').write_text(''.join(json.dumps(dict(id=previous.key(r[0]),label=0,score=p,above_threshold=p>t))+'\n' for r,p in zip(keep,preds)));print('RESULT',json.dumps(results),flush=True)
if __name__=='__main__':main()
