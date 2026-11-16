"""Independent arithmetic, identity, split and real-call coverage reconciliation."""
import argparse,json,subprocess,hashlib,importlib.util
from pathlib import Path
from collections import Counter,defaultdict
import numpy as np
from privacy import measure,sha
from http_model import previous,fresh,view,template_key
from inference import Frozen

def read(p):return [json.loads(x) for x in p.read_text().splitlines()]
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',type=Path,required=True);ap.add_argument('--assets',type=Path,required=True);ap.add_argument('--data',type=Path,required=True);a=ap.parse_args();checks={};artifacts={}
    for family in ['privacy','http','http_structured','http_v2','http_structured_v3']:
        p=a.root/family;f=json.loads((p/'freeze.json').read_text());name='model.joblib' if family in ['http_structured','http_v2','http_structured_v3'] else f['selected']+('.safetensors' if f['selected'].endswith('_pair') else '.joblib');artifacts[family]=sha(p/name);checks[family+'_digest']=artifacts[family]==f['artifact_sha256'] and sha(p/'protocol.json')==f['protocol_sha256']
    splits={s:read(a.assets/'agent_privacy_data'/f'{s}.jsonl') for s in ['train','validation','calibration','test']};groups={s:{r['group'] for r in rr} for s,rr in splits.items()};checks['privacy_scenario_disjoint']=all(not groups[x]&groups[y] for x in groups for y in groups if x!=y)
    f=json.loads((a.root/'privacy/freeze.json').read_text());checks['privacy_data_digests']=all(sha(a.assets/'agent_privacy_data'/f'{s}.jsonl')==f['source_hashes'][s] for s in splits);pp=read(a.root/'privacy/test_predictions.jsonl');m=measure(np.array([r['label'] for r in pp]),np.array([r['score'] for r in pp]),f['threshold']);checks['privacy_regression_arithmetic']=m==json.loads((a.root/'privacy/results.json').read_text())['consumed_test']
    live=a.root/'live';runs=json.loads((live/'runs.json').read_text());scores=read(live/'model_scores.jsonl');live_checks=[];contexts=json.loads((live/'contexts.json').read_text());paired=[]
    for run in runs:
        raw=live/(run['case']+'.native.jsonl');events=read(raw);calls=[r['part'] for r in events if r.get('type')=='tool_use'];native=Counter(r['callID'] for r in calls);actual=Counter(r['call_id'] for r in scores if r['run_id']==run['case']);match=True
        for call in calls:
            candidate='Tool: '+call['tool']+'\nArguments: '+json.dumps(call.get('state',{}).get('input',{}),sort_keys=True,ensure_ascii=False);digest=hashlib.sha256(candidate.encode()).hexdigest();r=next((r for r in scores if r['run_id']==run['case'] and r['call_id']==call['callID']),None)
            match=match and r is not None and digest==r['input_sha256'];paired.append(dict(context=contexts[run['case']],candidate=candidate))
        live_checks.append(dict(case=run['case'],hash_matches=sha(raw)==run['stdout_sha256'],native_score_ids_match=native==actual,input_hashes_match=match,one_score_per_call=all(n==1 for n in actual.values()),n=len(calls),native_errors=sum(r.get('state',{}).get('status')=='error' for r in calls)))
    checks['live_log_integrity_and_coverage']=all(r['hash_matches'] and r['native_score_ids_match'] and r['input_hashes_match'] and r['one_score_per_call'] and r['n']>0 for r in live_checks);checks['live_scores_use_frozen_model']=all(r['model_sha256']==artifacts['privacy'] and r['above_threshold']==(r['score']>f['threshold']) for r in scores)
    oldpath=Path(__file__).resolve().parent.parent/'model-opencode-20261018/scoring.py';spec=importlib.util.spec_from_file_location('legacy',oldpath);old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old);legacy=old.Scorer(a.assets);ps=[]
    for off in range(0,len(paired),32):ps.extend(r['score'] for r in legacy.score('agent_privacy',paired[off:off+32]))
    recalibrated=json.loads((a.root/'evaluation/threshold_only_baseline.json').read_text())['threshold'];matched_privacy=dict(scope='legacy replay and new before-tool inference on identical new native candidates',old_legacy_threshold=measure(np.zeros(len(ps),int),np.asarray(ps),.5),old_calibration_only=measure(np.zeros(len(ps),int),np.asarray(ps),recalibrated),new_live=measure(np.zeros(len(scores),int),np.array([r['score'] for r in scores]),f['threshold']))
    # Report the degree of pseudo-replication in the fresh benign audit.
    rr,_=previous.ecml(a.data);cc,_=previous.csic(a.data);seen_templates={template_key(e) for e,y,l in rr+cc};del rr,cc
    fresh_rows,_=fresh(a.data);fr={previous.key(e):e for e,y,l in fresh_rows};pred=read(a.root/'http/fresh_predictions.jsonl');unique={};unseen=[]
    for row in pred:
        event=fr[row['id']];unique.setdefault(view(event,'payload'),row)
        if template_key(event) not in seen_templates:unseen.append(row)
    t=json.loads((a.root/'http/freeze.json').read_text())['threshold'];metrics={}
    for name,rows in [('unique_payload',list(unique.values())),('unseen_template_rows',unseen)]:
        metrics[name]=measure(np.zeros(len(rows),int),np.array([r['score'] for r in rows]),t) if rows else dict(n=0)
    # All four frozen families see exactly the same retained benign requests.
    frozen=Frozen(a.root,a.assets);events=[fr[row['id']] for row in pred];comparison={};output=[]
    test_scores=frozen.score('privacy',splits['test']);parity=max(abs(r['score']-s['score']) for r,s in zip(pp,test_scores));checks['portable_privacy_inference_parity']=parity<1e-6 and all(r['above_threshold']==s['above_threshold'] for r,s in zip(pp,test_scores))
    for family in ['http','http_structured','http_v2','http_structured_v3']:
        scores=[]
        for off in range(0,len(events),512):scores+=frozen.score(family,events[off:off+512])
        values=np.array([r['score'] for r in scores]);t=frozen.config[family]['threshold'];comparison[family]=measure(np.zeros(len(values),int),values,t);comparison[family]['model_sha256']=artifacts[family]
        output.extend(dict(id=r['id'],label=0,**s) for r,s in zip(pred,scores))
    (a.root/'evaluation/matched_benign_predictions.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in output));(a.root/'evaluation/matched_benign_comparison.json').write_text(json.dumps(comparison,indent=2)+'\n')
    checks['fresh_reconciliation']=len(pred)==json.loads((a.root/'http/results.json').read_text())['fresh_benign_audit']['metrics']['n']
    result=dict(checks=checks,all_checks_pass=all(checks.values()),artifacts=artifacts,live_checks=live_checks,privacy_reload_max_score_difference=parity,matched_privacy_comparison=matched_privacy,fresh_group_diagnostics=metrics,matched_benign_comparison=comparison,production_approved=False)
    out=a.root/'evaluation';out.mkdir(exist_ok=True);(out/'audit.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result),flush=True)
    if not result['all_checks_pass']:raise SystemExit(1)
if __name__=='__main__':main()
