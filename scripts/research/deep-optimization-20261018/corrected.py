"""Versioned follow-up: preserve literal target suffixes and field denominators.

Earlier frozen families remain reproducible and unchanged. Earlier audits are
now consumed regression for this follow-up; selection still uses validation only.
"""
import argparse,copy,json,re,time
from pathlib import Path
import joblib,numpy as np
from sklearn.feature_extraction.text import TfidfVectorizer
from sklearn.linear_model import LogisticRegression
from sklearn.ensemble import ExtraTreesClassifier,HistGradientBoostingClassifier
from http_model import previous,template_key
from structured import extract
from privacy import measure,threshold,sha

def target(raw):
    # HTTP request-target data is not a browser URL: preserve literal '#'.
    if re.match(r'^[A-Za-z][A-Za-z0-9+.-]*://',raw):
        raw=re.sub(r'^[A-Za-z][A-Za-z0-9+.-]*://[^/?#]*','',raw,count=1)
        if not raw or raw[0] in '?#':raw='/'+raw
    return raw
def text(event):
    h=event['http'];return (target(h['target'])+'\n'+h['body'].rstrip('\r\n'))[:16384]
def go_features(events,exe):
    canonical=[]
    for e in events:
        r=copy.deepcopy(e);r['http']['target']=target(r['http']['target']);r['http']['body']=r['http']['body'].rstrip('\r\n');canonical.append(r)
    old,names=extract(canonical,exe);base=old[:,:137];nn=names[:137];idx={n:i for i,n in enumerate(nn)};cols=[base];out=list(nn)
    for i,n in enumerate(nn):
        if n.endswith(('_count','.bytes')):cols.append(np.log1p(base[:,i:i+1]));out.append('log1p:'+n)
        if n.endswith('_count') and n.startswith(('payload.','body.')):
            field=n.split('.')[0];den=np.maximum(1,base[:,idx[field+'.bytes']]);cols.append((base[:,i]/den)[:,None]);out.append('per_'+field+'_byte:'+n)
    hints=[n for n in nn if n.endswith('_hint')]
    for i,n in enumerate(hints):
        for other in hints[:i]:cols.append((base[:,idx[n]]*base[:,idx[other]])[:,None]);out.append(n+'*'+other)
    cols.append(old[:,-137:]);out+=['present:'+n for n in nn]
    return np.column_stack(cols).astype(np.float32),out
def probabilities(bundle,events,exe=None):
    if bundle['front']=='corrected_go':
        if sha(exe)!=bundle['extractor_sha256']:raise ValueError('extractor checksum mismatch')
        x,names=go_features(events,exe)
        if names!=bundle['feature_names']:raise ValueError('engineered feature contract changed')
        x=x[:,bundle['indices']]
    else:x=bundle['vectorizer'].transform([text(e) for e in events])
    return bundle['model'].predict_proba(x)[:,1]
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',type=Path,required=True);ap.add_argument('--data',type=Path,required=True);a=ap.parse_args();start=time.perf_counter();dirs={n:a.root/n for n in ['http_v2','http_structured_v3']}
    for d in dirs.values():d.mkdir(exist_ok=True)
    protocol=dict(version='request-model-origin-preserving.v2/field-ratios.v3',changes=['retain literal # suffix in HTTP request-target','strip only dataset body CR/LF separators, retain spaces','body count ratios divide by body.bytes, target ratios by payload.bytes','Go selection uses minimum feature importance across source-specific train-only forests'],lexical_candidates=['char3-5 logistic C2','word1-2 logistic C2'],go_leaf_candidates=[15,31,63],selection='validation worse-source recall at empirical 1% FPR, then macro AP',calibration='max source calibration 99th negative percentile; strict >',earlier_audits_status='consumed regression, not independent for follow-up',mode='shadow_only',production_approved=False)
    for d in dirs.values():(d/'protocol.json').write_text(json.dumps(protocol,indent=2)+'\n')
    r,_=previous.ecml(a.data);c,_=previous.csic(a.data);r,_=previous.dedupe(r+c);events=[e for e,y,l in r];y=np.array([y for e,y,l in r]);sources=np.array([e['source'] for e,y,l in r]);groups=np.array([template_key(e) for e in events]);parts,seed=previous.old.split_groups(y,groups)
    def validation(m,x):
        ii=parts['validation'];p=m.predict_proba(x[ii])[:,1];vs={}
        for s in sorted(set(sources)):
            keep=sources[ii]==s;vs[s]=measure(y[ii][keep],p[keep],threshold(y[ii][keep],p[keep]))
        return vs,[min(v['recall'] for v in vs.values()),float(np.mean([v['AP'] for v in vs.values()]))]
    def freeze(family,trials,bundles):
        d=dirs[family];k=max(range(len(trials)),key=lambda i:tuple(trials[i]['rank']));b=bundles[k];ii=parts['calibration'];p=probabilities(b,[events[i] for i in ii],a.root/'behavior-features');ts={s:threshold(y[ii][sources[ii]==s],p[sources[ii]==s]) for s in sorted(set(sources))};t=max(ts.values());mp=d/'model.joblib';joblib.dump(b,mp);f=dict(artifact_sha256=sha(mp),threshold=t,selection=trials[k],source_thresholds=ts,protocol_sha256=sha(d/'protocol.json'),mode='shadow_only',production_approved=False);(d/'freeze.json').write_text(json.dumps(f,indent=2)+'\n');(d/'trials.json').write_text(json.dumps(trials,indent=2)+'\n');ii=parts['test'];p=probabilities(b,[events[i] for i in ii],a.root/'behavior-features');result=dict(freeze=f,actual_fits=2 if family=='http_v2' else 5,consumed_regression={s:measure(y[ii][sources[ii]==s],p[sources[ii]==s],t) for s in sorted(set(sources))},elapsed_seconds=time.perf_counter()-start);(d/'results.json').write_text(json.dumps(result,indent=2)+'\n');print('RESULT',family,json.dumps(result),flush=True)
    trials=[];bundles=[];strings=[text(e) for e in events]
    for front in ['char','word']:
        v=TfidfVectorizer(analyzer=front,ngram_range=(3,5) if front=='char' else (1,2),min_df=3,max_features=50000,sublinear_tf=True,dtype=np.float32);v.fit([strings[i] for i in parts['train']]);x=v.transform(strings);m=LogisticRegression(C=2,max_iter=600,solver='liblinear',random_state=20261018).fit(x[parts['train']],y[parts['train']]);vs,rank=validation(m,x);trials.append(dict(front=front,validation=vs,rank=rank));bundles.append(dict(front='corrected_'+front,vectorizer=v,model=m));print('FIT lexical',front,json.dumps(trials[-1]),flush=True)
    freeze('http_v2',trials,bundles);del x,v,m,strings,trials,bundles
    x,names=go_features(events,a.root/'behavior-features');varying=np.flatnonzero(np.var(x[parts['train']],axis=0)>1e-10);importances=[]
    for s in sorted(set(sources)):
        ii=parts['train'][sources[parts['train']]==s];selector=ExtraTreesClassifier(n_estimators=96,max_depth=14,min_samples_leaf=20,n_jobs=2,random_state=20261018).fit(x[ii][:,varying],y[ii]);importances.append(selector.feature_importances_)
    importance=np.min(importances,axis=0);ranked=sorted(range(len(varying)),key=lambda j:(-importance[j],names[varying[j]]));selected=[]
    for j in ranked:
        i=int(varying[j])
        if importance[j]>0 and all(abs(np.corrcoef(x[parts['train'],i],x[parts['train'],k])[0,1])<.98 for k in selected):selected.append(i)
        if len(selected)>=32:break
    (dirs['http_structured_v3']/'features.json').write_text(json.dumps(dict(engineered_feature_count=len(names),fit_rows=len(parts['train']),selected=[dict(index=i,name=names[i]) for i in selected],selector='minimum source-conditional train-only forest importance; correlation < .98'),indent=2)+'\n');xx=x[:,selected];trials=[];bundles=[]
    for leaves in [15,31,63]:
        m=HistGradientBoostingClassifier(max_iter=200,max_leaf_nodes=leaves,learning_rate=.06,min_samples_leaf=30,l2_regularization=3,early_stopping=False,random_state=20261018).fit(xx[parts['train']],y[parts['train']]);vs,rank=validation(m,xx);trials.append(dict(leaves=leaves,validation=vs,rank=rank));bundles.append(dict(front='corrected_go',model=m,indices=selected,feature_names=names,extractor_sha256=sha(a.root/'behavior-features')));print('FIT Go',leaves,json.dumps(trials[-1]),flush=True)
    freeze('http_structured_v3',trials,bundles)
if __name__=='__main__':main()
