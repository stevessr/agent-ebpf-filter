"""Reconcile actual model digests and live native calls; never adjusts thresholds."""
import argparse,json,statistics
from pathlib import Path
from scoring import EXPECTED,sha,metrics
HTTP_SHA='b53b70d7f7bdf86ff0d24712ad6c10667f47463daf1abf4725e45c1a8d11a25a'
def main():
    p=argparse.ArgumentParser();p.add_argument('--live',type=Path,required=True);a=p.parse_args();scores=[json.loads(l) for l in (a.live/'model_scores.jsonl').read_text().splitlines()];http=[json.loads(l) for l in (a.live/'http_scores.jsonl').read_text().splitlines()];runs=json.loads((a.live/'runs.json').read_text());checks=[];native={}
    for run in runs:
        path=a.live/(run['case']+'.native.jsonl');checks.append(sha(path)==run['stdout_sha256']);calls=[e['part'] for e in map(json.loads,path.read_text().splitlines()) if e.get('type')=='tool_use'];native.update({c['callID']:c for c in calls});checks.append({c['callID'] for c in calls}=={s['call_id'] for s in scores if s['task']=='agent_privacy' and s['run_id']==run['case']})
    adjudicated=[];corrections=[]
    for r in scores:
        expected=EXPECTED['agent_privacy_transfer_balanced.safetensors' if r['task']=='agent_privacy' else 'bert_balanced.safetensors'];checks.append(r['model_sha256']==expected)
        row=dict(r)
        if r['task']=='vulnerability':
            source=a.live/'fixtures'/r['run_id']/Path(native[r['call_id']]['state']['input']['filePath']).name;row['code_sha256']=sha(source)
            if source.name=='c06.c':
                # The zero-divisor guard alone misses INT_MIN / -1 signed overflow.
                row['label']=1;row['annotation_correction']='original fixture annotation omitted signed division overflow; source bytes, scores and threshold unchanged';corrections.append(r['call_id'])
        adjudicated.append(row)
    checks.extend(r.get('model_digest')==HTTP_SHA for r in http)
    result={'all_live_log_hashes_and_call_ids_and_model_digests_verified':all(checks),'native_calls':len(native),'privacy_scored_calls':sum(s['task']=='agent_privacy' for s in scores),'http_records':len(http),'inference_abstentions':sum(r.get('score') is None for r in scores+http),'timeouts':sum(r['timeout'] for r in runs),'annotation_corrections':corrections,'threshold_changes':0,'fit_count':0,'actual_blocks':0,'mode':'shadow_only','production_approved':False}
    for task in ['agent_privacy','vulnerability']:
        rr=[r for r in adjudicated if r['task']==task];result[task]=metrics([r['label'] for r in rr],rr)
    lat=[r['latency_ms'] for r in scores if r['task']=='agent_privacy'];result['privacy_live_p50_ms']=statistics.median(lat);result['privacy_live_max_ms']=max(lat)
    (a.live/'adjudicated_scores.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in adjudicated));(a.live/'audit.json').write_text(json.dumps(result,indent=2)+'\n');assert all(checks);print(json.dumps(result,indent=2))
if __name__=='__main__':main()
