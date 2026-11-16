"""Bounded request-model study. No attack execution or destination contact.
ECML generated requests are split by template; CSIC is external, evaluation-only.
No model is authorized for blocking by this script, irrespective of metrics.
"""
import argparse, csv, hashlib, importlib.util, json, re, subprocess, tempfile, time
from collections import Counter, defaultdict
from array import array
from pathlib import Path
import numpy as np
from scipy.sparse import csr_matrix
from scipy.stats import beta
from sklearn.linear_model import SGDClassifier
from sklearn.metrics import accuracy_score, balanced_accuracy_score, f1_score, average_precision_score, confusion_matrix

oldpath=Path(__file__).parent.parent/'behavior-features-20261018/train_http.py'
spec=importlib.util.spec_from_file_location('previous_recipe',oldpath);old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old)
DIM=32768

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def http(raw):
    raw=raw.replace('\r\n','\n');first,sep,rest=raw.partition('\n')
    m=re.fullmatch(r'(\S+) (.*) HTTP/1\.[01]',first)
    if not m:return None
    head,sep,body=rest.partition('\n\n')
    if not sep:return None
    headers=[]
    for line in head.splitlines():
        name,sep,value=line.partition(':')
        if not sep:return None
        headers.append({'name':name,'value':value.lstrip()})
    return {'method':m[1],'target':m[2],'headers':headers,'headers_present':True,
            'headers_preserve_duplicates':True,'body':body,'truncated':False,'direction':'outgoing'}

def ecml(root):
    result=[];audit=Counter()
    for name in ['xml_train.txt','xml_test.txt']:
        data=(root/'OriginalDataSets/ecml_pkdd/dataset_ecml_pkdd_train_test'/name).read_bytes().decode('latin1')
        for label,raw in re.findall(r'^Start - Id: [^\n]+\nclass: ([^\n]+)\n(.*?)^End - Id: [^\n]+',data,re.M|re.S):
            h=http(raw.rstrip('\n')+'\n\n' if '\n\n' not in raw else raw)
            if h is None:audit['parse_rejected']+=1;continue
            result.append(({'source':'public_ecml_generated','http':h},int(label.strip()!='Valid'),label.strip()))
    audit['parsed']=len(result);return result,dict(audit)

def csic(root):
    result=[];audit=Counter()
    for name,label in [('normalTrafficTest.txt',0),('anomalousTrafficTest.txt',1)]:
        data=(root/'OriginalDataSets/csic_2010'/name).read_bytes().decode('latin1').replace('\r\n','\n')
        starts=list(re.finditer(r'^(?:GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH) .* HTTP/1\.[01]\n',data,re.M))
        for j,m in enumerate(starts):
            raw=data[m.start():starts[j+1].start() if j+1<len(starts) else len(data)]
            h=http(raw)
            if h is None:audit['parse_rejected']+=1;continue
            result.append(({'source':'public_csic_generated','http':h},label,'anomaly' if label else 'normal'))
    audit['parsed']=len(result);return result,dict(audit)

def key(record):
    h=record['http'];return hashlib.sha256(json.dumps(h,sort_keys=True,ensure_ascii=True).encode()).hexdigest()
def group(record):
    h=record['http'];return hashlib.sha256(old.template(h['method']+' '+h['target']+' '+h['body']).encode()).hexdigest()
def dedupe(rows):
    groups=defaultdict(list)
    for row in rows:groups[key(row[0])].append(row)
    kept=[];audit=Counter()
    for k,v in groups.items():
        if len({r[1] for r in v})>1:audit['conflicting_rows']+=len(v);continue
        kept.append(v[0]);audit['exact_duplicates']+=len(v)-1
    return kept,dict(audit)

def features(rows,exe):
    values=array('d');indices=array('i');indptr=array('q',[0]);keep=[];abstained=0
    with tempfile.TemporaryFile(mode='w+') as f:
        for row in rows:f.write(json.dumps(row[0])+'\n')
        f.seek(0);p=subprocess.Popen([str(exe),'--vectors'],stdin=f,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        for j,line in enumerate(p.stdout):
            o=json.loads(line)
            if 'entries' not in o:abstained+=1;continue
            keep.append(j)
            for item in o['entries']:indices.append(item['index']);values.append(item['value'])
            indptr.append(len(values))
        err=p.stderr.read();code=p.wait()
        if code:raise RuntimeError('bounded vector export failed: '+err)
    return csr_matrix((np.frombuffer(values,dtype=np.float64),np.frombuffer(indices,dtype=np.int32),np.frombuffer(indptr,dtype=np.int64)),shape=(len(keep),DIM)),keep,abstained

def cp_upper(fp,n):return float(beta.ppf(.95,fp+1,n-fp)) if n>fp else 1.
def select_threshold(y,p,target=.01):
    # One-sided tolerance bound: choose the largest allowed negative order
    # statistic rank using n/alpha only, then set that threshold on calibration.
    # It assumes exchangeability and is explicitly not an OOD guarantee.
    neg=np.sort(p[y==0]);allowed=[k for k in range(len(neg)) if cp_upper(k,len(neg))<=target]
    if not allowed:return .999999999,False
    k=max(allowed);t=float(neg[len(neg)-k-1]);return max(1e-12,min(1-1e-12,t)),True

def metrics(y,p,t):
    pred=p>t;tn,fp,fn,tp=confusion_matrix(y,pred,labels=[0,1]).ravel()
    return {'rows':len(y),'accuracy':float(accuracy_score(y,pred)),'balanced_accuracy':float(balanced_accuracy_score(y,pred)),
            'f1':float(f1_score(y,pred,zero_division=0)),'AP':float(average_precision_score(y,p)),
            'recall':float(tp/max(1,tp+fn)),'fpr':float(fp/max(1,fp+tn)),'fpr_upper95':cp_upper(int(fp),int(fp+tn)),
            'tn':int(tn),'fp':int(fp),'fn':int(fn),'tp':int(tp)}

def main():
    p=argparse.ArgumentParser();p.add_argument('--data',type=Path,required=True);p.add_argument('--worker',type=Path,required=True);p.add_argument('--out',type=Path,required=True);args=p.parse_args();args.out.mkdir(parents=True,exist_ok=True)
    started=time.monotonic()
    protocol={'encoding':'byte-fnv1a-3-5-logtf-l2.v1','dimension':DIM,'training_source':'ECML-PKDD 2007 generated requests, original train/test repartitioned',
      'external_source':'CSIC 2010 normal/anomalous test requests; evaluation only','candidates_alpha':[1e-5,1e-4,1e-3],
      'selection':'validation recall under one-sided 95% FPR upper bound <= 1%, then AP; calibration threshold separate',
      'gates':{'fpr_upper95_max':.01,'recall_min':.90,'abstention_max':.01},'test_policy':'one evaluation after configuration freeze; no tuning on ECML holdout or CSIC',
      'limitations':['Historical generated traffic, not production representative.','No kernel outcomes or privacy leakage labels.','Binomial bound assumes IID/exchangeability; template-group intervals also reported.','Source-isolated external evaluation is mandatory.']}
    (args.out/'protocol.json').write_text(json.dumps(protocol,indent=2)+'\n')
    rows,audit=ecml(args.data);rows,dup=dedupe(rows);audit.update(dup);print('ECML parse',audit,flush=True)
    x,keep,abstained=features(rows,args.worker);rows=[rows[i] for i in keep];y=np.array([r[1] for r in rows]);groups=np.array([group(r[0]) for r in rows]);parts,seed=old.split_groups(y,groups)
    print('ECML matrix',x.shape,x.nnz,'abstained',abstained,flush=True)
    trials=[];models=[]
    for c in protocol['candidates_alpha']:
        m=SGDClassifier(loss='log_loss',alpha=c,max_iter=120,tol=1e-4,random_state=20261018,average=True)
        m.fit(x[parts['train']],y[parts['train']]);vp=m.predict_proba(x[parts['validation']])[:,1];vt,ok=select_threshold(y[parts['validation']],vp)
        mm=metrics(y[parts['validation']],vp,vt);trials.append({'alpha':c,'validation':mm,'threshold':vt,'bound_feasible':ok,'iterations':int(m.n_iter_)});models.append(m);print('validation alpha',c,mm,flush=True)
    chosen=max(range(len(trials)),key=lambda i:(trials[i]['bound_feasible'],trials[i]['validation']['recall'],trials[i]['validation']['AP']))
    m=models[chosen];cp=m.predict_proba(x[parts['calibration']])[:,1];threshold,feasible=select_threshold(y[parts['calibration']],cp)
    model={'encoding':protocol['encoding'],'kind':'http_request','weights':m.coef_[0].tolist(),'intercept':float(m.intercept_[0]),'threshold':threshold,'shadow_only':True}
    mp=args.out/'http_request_model.json';mp.write_text(json.dumps(model,separators=(',',':'))+'\n');digest=sha(mp)
    (args.out/'freeze.json').write_text(json.dumps({'model_sha256':digest,'selected_alpha':trials[chosen]['alpha'],'threshold':threshold,'protocol_sha256':sha(args.out/'protocol.json')},indent=2)+'\n')
    # Evaluation runs in a fresh process after this training process exits, so
    # its source arrays and sparse matrices cannot overlap the training cache.
    (args.out/'trials.json').write_text(json.dumps(trials,indent=2)+'\n')
    (args.out/'training_audit.json').write_text(json.dumps({'rows':len(rows),'abstained':abstained,'audit':audit,'elapsed_seconds':time.monotonic()-started},indent=2)+'\n')
    if abstained:
        raise RuntimeError('Training input abstentions require a versioned split protocol; do not evaluate reconstructed splits')
    print('Frozen. Run evaluate_frozen.py separately after this process exits.',flush=True)
if __name__=='__main__':main()
