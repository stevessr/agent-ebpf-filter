"""Resume evaluation from an already frozen model, without refitting or retuning.
Uses bounded Go scoring, and batches Python/Go parity checks to avoid retaining
both full source feature matrices in RAM. The model checksum must match freeze.json.
"""
import argparse, ast, hashlib, json, subprocess, tempfile, time
from pathlib import Path
import numpy as np
from scipy.special import expit
from train_production import ecml, csic, dedupe, key, group, features, metrics, old, sha

def score(rows,worker,model,digest):
    with tempfile.TemporaryFile(mode='w+') as f:
        for row in rows:f.write(json.dumps(row[0])+'\n')
        f.seek(0);p=subprocess.Popen([str(worker),'--model',str(model),'--sha256',digest],stdin=f,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        scores=[];keep=[]
        for i,line in enumerate(p.stdout):
            item=json.loads(line)
            if item.get('score') is not None:keep.append(i);scores.append(item['score'])
        error=p.stderr.read();assert p.wait()==0,error
    return np.array(scores),keep

def interval(y,p,g,t):
    ug,inv=np.unique(g,return_inverse=True);pred=p>t;n=len(ug)
    neg=np.bincount(inv,weights=y==0,minlength=n);fp=np.bincount(inv,weights=(y==0)&pred,minlength=n)
    pos=np.bincount(inv,weights=y==1,minlength=n);tp=np.bincount(inv,weights=(y==1)&pred,minlength=n)
    rng=np.random.default_rng(20261018);a=[];b=[]
    for _ in range(1000):
        ix=rng.integers(n,size=n)
        if neg[ix].sum():a.append(fp[ix].sum()/neg[ix].sum())
        if pos[ix].sum():b.append(tp[ix].sum()/pos[ix].sum())
    return {'groups':n,'fpr_95ci':np.quantile(a,[.025,.975]).tolist(),'recall_95ci':np.quantile(b,[.025,.975]).tolist()}

def main():
    p=argparse.ArgumentParser();p.add_argument('--data',type=Path,required=True);p.add_argument('--worker',type=Path,required=True);p.add_argument('--out',type=Path,required=True);p.add_argument('--training-log',type=Path);args=p.parse_args();start=time.monotonic()
    freeze=json.loads((args.out/'freeze.json').read_text());modelpath=args.out/'http_request_model.json';assert sha(modelpath)==freeze['model_sha256'];model=json.loads(modelpath.read_text());threshold=model['threshold'];protocol=json.loads((args.out/'protocol.json').read_text());assert sha(args.out/'protocol.json')==freeze['protocol_sha256']
    rows,ea=ecml(args.data);rows,ed=dedupe(rows);ea.update(ed);y=np.array([r[1] for r in rows]);groups=np.array([group(r[0]) for r in rows]);parts,seed=old.split_groups(y,groups)
    results={};predictions={};parity=0.
    for name in ['validation','calibration','test']:
        ids=parts[name];subset=[rows[i] for i in ids];prob,keep=score(subset,args.worker,modelpath,freeze['model_sha256']);labels=y[ids[keep]]
        result=metrics(labels,prob,threshold);result['abstained']=len(subset)-len(keep);result['group_bootstrap']=interval(labels,prob,groups[ids[keep]],threshold);results[name]=result;predictions[name]=prob.tolist()
        if name=='test':
            for offset in range(0,len(subset),128):
                batch=subset[offset:offset+128];x,k,ab=features(batch,args.worker);assert ab==0
                py=expit(x@np.asarray(model['weights'])+model['intercept']);parity=max(parity,float(np.max(np.abs(py-prob[offset:offset+len(batch)]))))
        print(name,result,flush=True)
    assert parity<1e-10
    external,ca=csic(args.data);external,cd=dedupe(external);ca.update(cd);known={key(r[0]) for r in rows};gg=set(groups)
    remaining=[r for r in external if key(r[0]) not in known and group(r[0]) not in gg];ca['overlap_removed']=len(external)-len(remaining)
    prob,keep=score(remaining,args.worker,modelpath,freeze['model_sha256']);labels=np.array([remaining[i][1] for i in keep]);external_result=metrics(labels,prob,threshold);external_result['abstained']=len(remaining)-len(keep);external_result['group_bootstrap']=interval(labels,prob,[group(remaining[i][0]) for i in keep],threshold)
    positive_total=sum(r[1] for r in remaining);external_result['recall_counting_abstentions_as_misses']=external_result['tp']/positive_total;external_result['coverage']=len(keep)/len(remaining)
    trials_path=args.out/'trials.json'
    if trials_path.exists():
        trials=json.loads(trials_path.read_text())
    elif args.training_log:
        trials=[]
        for line in args.training_log.read_text().splitlines():
            if line.startswith('validation alpha '):
                prefix,detail=line.split(' {',1);trials.append({'alpha':float(prefix.split()[-1]),'validation':ast.literal_eval('{'+detail)})
    else:
        raise ValueError('Frozen trials.json or original --training-log is required')
    assert len(trials)==len(protocol['candidates_alpha'])
    def gate(m):return m['fpr_upper95']<=.01 and m['group_bootstrap']['fpr_95ci'][1]<=.01 and m['recall']>=.9 and m['group_bootstrap']['recall_95ci'][0]>=.9 and m['abstained']/max(1,m['rows']+m['abstained'])<=.01
    report={'protocol':protocol,'source_revision':subprocess.check_output(['git','-C',str(args.data),'rev-parse','HEAD'],text=True).strip(),
      'source_sha256':{str(f.relative_to(args.data)):sha(f) for f in list(args.data.glob('OriginalDataSets/ecml_pkdd/dataset_ecml_pkdd_train_test/*.txt'))+list(args.data.glob('OriginalDataSets/csic_2010/*Test.txt'))},
      'splits':{k:{'rows':len(v),'groups':len(set(groups[v])),'positive':int(y[v].sum()),'negative':int((y[v]==0).sum())} for k,v in parts.items()},
      'audit':{'ecml':ea,'external_csic':ca},'split_seed':seed,'trials':trials,'fit_count':len(trials),'selected_alpha':freeze['selected_alpha'],'threshold':threshold,
      'validation_at_frozen_threshold':results['validation'],'calibration':results['calibration'],'heldout_ecml':results['test'],'external_csic':external_result,
      'model_sha256':freeze['model_sha256'],'model_bytes':modelpath.stat().st_size,'go_prediction_max_abs_error':parity,
      'gates':{'ecml_pass':gate(results['test']),'external_csic_pass':gate(external_result),'kernel_privacy_destruction_pass':False,'production_blocking_approved':False},
      'evaluation_seconds':time.monotonic()-start,'evaluation_resume':'No refit/threshold change. Bounded Go scoring and 128-record parity batches.'}
    (args.out/'results.json').write_text(json.dumps(report,indent=2)+'\n');(args.out/'private_splits.json').write_text(json.dumps({'groups':groups.tolist(),'partitions':{k:v.tolist() for k,v in parts.items()},'predictions':predictions,'external_predictions':prob.tolist()}))
    print(json.dumps({'heldout_ecml':report['heldout_ecml'],'external_csic':external_result,'gates':report['gates'],'parity':parity},indent=2),flush=True)
if __name__=='__main__':main()
