"""Offline, independently sourced CRS expectation-positive challenge.

Expected rule detection is not proof of successful exploitation. Negative rule
tests are never relabelled globally benign, and nothing is sent to a server.
"""
import argparse,json
from pathlib import Path
from collections import Counter
import joblib,numpy as np,yaml
from http_model import transform,previous,view
from structured import extract
from privacy import sha
from corrected import probabilities

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',type=Path,required=True);ap.add_argument('--data',type=Path,required=True);ap.add_argument('--corpus',default='crs');ap.add_argument('--families',nargs='+',default=['http','http_structured']);a=ap.parse_args();base=a.root/a.corpus;protocol=json.loads((base/'protocol.json').read_text());events=[];meta=[];skips=Counter();files={};seen=set()
    prior=a.root/'evaluation/crs_predictions.jsonl'
    if a.corpus!='crs' and prior.exists():seen.update(json.loads(line)['http_sha256'] for line in prior.read_text().splitlines())
    r,_=previous.ecml(a.data);c,_=previous.csic(a.data);training_payloads={view(e,'payload') for e,y,l in r+c};del r,c
    for filename in protocol['files']:
        if not filename.endswith('.yaml'):continue
        path=base/filename
        if not path.exists():skips['unavailable_predeclared_file']+=1;continue
        files[filename]=sha(path);source=yaml.safe_load(path.read_text());rule=source['rule_id']
        for test in source['tests']:
            if test.get('enabled') is False:skips['disabled']+=1;continue
            stages=test.get('stages',[])
            if len(stages)!=1:skips['multistage']+=1;continue
            s=stages[0];expected=s.get('output',{}).get('log',{}).get('expect_ids',[])
            if rule not in expected:skips['not_positive_for_target_rule']+=1;continue
            i=s['input'];body=i.get('data','');target=i.get('uri','/');headers=i.get('headers',{})
            if not isinstance(body,str) or not isinstance(target,str) or any(k in i for k in ['raw_request','encoded_request','multipart_form']):skips['unsupported_representation']+=1;continue
            if not isinstance(headers,dict) or not all(isinstance(v,(str,int)) for v in headers.values()):skips['unsupported_headers']+=1;continue
            event=dict(http=dict(method=i.get('method','GET'),target=target,headers=[dict(name=k,value=str(v)) for k,v in headers.items()],headers_present=True,headers_preserve_duplicates=True,body=body,truncated=False,direction='outgoing'));k=previous.key(event)
            if k in seen:skips['duplicate_request']+=1;continue
            if view(event,'payload') in training_payloads:skips['development_payload_overlap']+=1;continue
            seen.add(k);events.append(event);meta.append(dict(id=f'{rule}-{test["test_id"]}',family=filename.split('/')[-2],http_sha256=k))
    result=dict(scope='expectation-positive offline '+a.corpus+' challenge; NOT native CRS execution or exploitation/interception success',revision=protocol['revision'],source_sha256=files,skips=dict(skips),n=len(events),models={});predictions=[]
    for family in a.families:
        directory=a.root/family;f=json.loads((directory/'freeze.json').read_text());p=directory/(f['selected']+'.joblib' if family=='http' else 'model.joblib')
        if sha(p)!=f['artifact_sha256']:raise ValueError('model checksum mismatch')
        b=joblib.load(p)
        if family in ['http_v2','http_structured_v3']:scores=probabilities(b,events,a.root/'behavior-features')
        elif family=='http':xx=transform(b,events)
        else:
            exe=a.root/'behavior-features'
            if sha(exe)!=b['extractor_sha256']:raise ValueError('extractor checksum mismatch')
            xx,names=extract(events,exe)
            if names!=b['feature_names']:raise ValueError('feature contract changed')
            xx=xx[:,b['indices']]
        if family not in ['http_v2','http_structured_v3']:scores=b['model'].predict_proba(xx)[:,1]
        hit=scores>f['threshold'];result['models'][family]=dict(model_sha256=f['artifact_sha256'],threshold=f['threshold'],n=len(events),hits=int(sum(hit)),recall=float(np.mean(hit)),false_negative=int(sum(~hit)),by_family={})
        for group in sorted({r['family'] for r in meta}):
            mask=np.array([r['family']==group for r in meta]);result['models'][family]['by_family'][group]=dict(n=int(sum(mask)),hits=int(sum(hit[mask])),recall=float(np.mean(hit[mask])))
        predictions.extend(dict(model=family,score=float(p),above_threshold=bool(p>f['threshold']),label=1,**r) for r,p in zip(meta,scores))
    out=a.root/'evaluation';out.mkdir(exist_ok=True);(out/(a.corpus+'_results.json')).write_text(json.dumps(result,indent=2)+'\n');(out/(a.corpus+'_predictions.jsonl')).write_text(''.join(json.dumps(r)+'\n' for r in predictions));print(json.dumps(result),flush=True)
if __name__=='__main__':main()
