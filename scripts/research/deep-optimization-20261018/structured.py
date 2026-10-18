"""Automatic feature selection from the actual Go v2 behavior cleaner.

Separate candidate family after the lexical round's generalization failure.
It does not overwrite the already frozen lexical model or reclassify old audits.
"""
import argparse,json,subprocess,tempfile,time
from pathlib import Path
import numpy as np
import joblib
from sklearn.ensemble import ExtraTreesClassifier,HistGradientBoostingClassifier
from http_model import previous,template_key,fresh
from privacy import measure,threshold,sha

def extract(events,exe):
    with tempfile.TemporaryFile(mode='w+') as f:
        for e in events:f.write(json.dumps(e)+'\n')
        f.seek(0);p=subprocess.Popen([str(exe)],stdin=f,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True);values=[];observed=[];names=None
        for line in p.stdout:
            row=json.loads(line)
            if row['schema']!='kernel-http-behavior.v2' or row['kind']!='http_request':raise ValueError('unexpected cleaner schema')
            features=row['features'];nn=sorted(features)
            if names is None:names=nn
            if names!=nn:raise ValueError('feature order changed')
            values.append([features[n]['value'] for n in names]);observed.append([int(features[n]['present']) for n in names])
        err=p.stderr.read();code=p.wait();p.stdout.close();p.stderr.close()
        if code:raise RuntimeError(err)
    if len(values)!=len(events):raise ValueError('cleaner coverage mismatch')
    xx,en=previous.old.engineer(np.asarray(values,dtype=np.float32),names)
    return np.column_stack([xx,np.asarray(observed,dtype=np.float32)]).astype(np.float32),en+['present:'+n for n in names]
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--data',type=Path,required=True);ap.add_argument('--extractor',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir(exist_ok=True);start=time.perf_counter()
    protocol=dict(schema='kernel-http-behavior.v2',extractor_sha256=sha(a.extractor),sources='same ECML + CSIC development corpora and template-disjoint splits as lexical round',selection='train-only variance filter + ExtraTrees importance + correlation<.98; 32 maximum features; validation worst-source recall at empirical 1% FPR',leaf_candidates=[15,31,63],feature_label_scope='HTTP request anomaly only; unavailable syscalls stay missing, never inferred',old_holdouts_status='all previously consumed; regression only',mode='shadow_only',production_approved=False)
    (a.out/'protocol.json').write_text(json.dumps(protocol,indent=2)+'\n')
    r,_=previous.ecml(a.data);c,_=previous.csic(a.data);r,_=previous.dedupe(r+c);groups=np.array([template_key(e) for e,y,l in r]);y=np.array([y for e,y,l in r]);sources=np.array([e['source'] for e,y,l in r]);parts,seed=previous.old.split_groups(y,groups);x,names=extract([e for e,y,l in r],a.extractor);print('FEATURES',x.shape,flush=True)
    varying=np.flatnonzero(np.var(x[parts['train']],axis=0)>1e-10);selector=ExtraTreesClassifier(n_estimators=96,max_depth=14,min_samples_leaf=20,n_jobs=2,random_state=20261018).fit(x[parts['train']][:,varying],y[parts['train']]);importance=dict(zip(varying,selector.feature_importances_));selected=[]
    for j in sorted(varying,key=lambda j:(-importance[j],names[j])):
        if all(abs(np.corrcoef(x[parts['train'],j],x[parts['train'],k])[0,1])<.98 for k in selected):selected.append(int(j))
        if len(selected)>=32:break
    (a.out/'features.json').write_text(json.dumps(dict(base_feature_count=137,engineered_feature_count=len(names),varying=len(varying),selected=[dict(index=j,name=names[j],train_importance=float(importance[j])) for j in selected],fit_rows=len(parts['train']),fit_partition='train'),indent=2)+'\n');xx=x[:,selected];trials=[];models=[]
    for leaves in [15,31,63]:
        m=HistGradientBoostingClassifier(max_iter=200,max_leaf_nodes=leaves,learning_rate=.06,min_samples_leaf=30,l2_regularization=3,early_stopping=False,random_state=20261018).fit(xx[parts['train']],y[parts['train']]);ii=parts['validation'];p=m.predict_proba(xx[ii])[:,1];vs={}
        for source in sorted(set(sources)):
            keep=sources[ii]==source;vs[source]=measure(y[ii][keep],p[keep],threshold(y[ii][keep],p[keep]))
        trial=dict(leaves=leaves,validation=vs,rank=[min(v['recall'] for v in vs.values()),float(np.mean([v['AP'] for v in vs.values()]))]);trials.append(trial);models.append(m);print('FIT',json.dumps(trial),flush=True)
    k=max(range(len(trials)),key=lambda i:tuple(trials[i]['rank']));m=models[k];ii=parts['calibration'];p=m.predict_proba(xx[ii])[:,1];ts={s:threshold(y[ii][sources[ii]==s],p[sources[ii]==s]) for s in sorted(set(sources))};t=max(ts.values());path=a.out/'model.joblib';joblib.dump(dict(model=m,indices=selected,feature_names=names,extractor_sha256=sha(a.extractor)),path);freeze=dict(artifact_sha256=sha(path),threshold=t,selection=trials[k],source_thresholds=ts,protocol_sha256=sha(a.out/'protocol.json'),mode='shadow_only',production_approved=False);(a.out/'freeze.json').write_text(json.dumps(freeze,indent=2)+'\n');(a.out/'trials.json').write_text(json.dumps(trials,indent=2)+'\n')
    ii=parts['test'];p=m.predict_proba(xx[ii])[:,1];result=dict(freeze=freeze,actual_fits=4,actual_boosting_rounds=600,consumed_regression={s:measure(y[ii][sources[ii]==s],p[sources[ii]==s],t) for s in sorted(set(sources))},seconds=time.perf_counter()-start);(a.out/'results.json').write_text(json.dumps(result,indent=2)+'\n');print('RESULT',json.dumps(result),flush=True)
if __name__=='__main__':main()
