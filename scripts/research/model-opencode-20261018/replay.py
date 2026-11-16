"""Actual frozen-model evaluation; no refitting or threshold selection."""
import argparse,json,hashlib,importlib.util
from pathlib import Path
from scoring import Scorer,metrics,sha

def main():
    p=argparse.ArgumentParser();p.add_argument('--assets',type=Path,required=True);p.add_argument('--traces',type=Path,required=True);p.add_argument('--out',type=Path,required=True);a=p.parse_args();a.out.mkdir(parents=True,exist_ok=True);model=Scorer(a.assets);reports={};predictions=[]
    # Previously collected common-task native tool inputs, with original hash gate.
    runs=json.loads((a.traces/'runs.json').read_text());samples=[];excluded=[]
    recipe=Path(__file__).parent.parent/'opencode-interception-20261018/run.py';spec=importlib.util.spec_from_file_location('fixture_recipe',recipe);recipe_module=importlib.util.module_from_spec(spec);spec.loader.exec_module(recipe_module)
    for run in runs:
        if run['positive_control']:continue
        path=a.traces/'logs'/(run['run_id']+'.jsonl')
        if sha(path)!=run['stdout_sha256'] or not run['task_complete']:
            excluded.append({'run_id':run['run_id'],'reason':'original_log_checksum_or_task_outcome_failed'});continue
        context=recipe_module.prompt(run['case'],a.traces/'fixtures'/run['run_id'])
        for e in map(json.loads,path.read_text().splitlines()):
            if e.get('type')!='tool_use':continue
            c=e['part'];candidate='Tool: '+c['tool']+'\nArguments: '+json.dumps(c['state'].get('input',{}),sort_keys=True,ensure_ascii=False)
            samples.append({'id':c['callID'],'context':context,'candidate':candidate,'label':0,'source':'actual_opencode_common_task_candidate'})
    scores=model.score('agent_privacy',samples);reports['opencode_common_privacy']=metrics([0]*len(samples),scores);reports['opencode_common_privacy']['excluded_runs']=excluded
    predictions.extend(dict(id=r['id'],source=r['source'],label=r['label'],**s) for r,s in zip(samples,scores))
    print('ACTUAL_OPENCODE_MODEL',reports['opencode_common_privacy'],flush=True)
    # Re-evaluate the original public held-out rows at the unchanged selected head thresholds.
    rows=[json.loads(l) for l in (a.assets/'agent_privacy_data/test.jsonl').read_text().splitlines()]
    scores=model.score('agent_privacy',rows);reports['public_privacyalign_replay']=metrics([r['label'] for r in rows],scores)
    predictions.extend(dict(id=r['id'],source='public_privacyalign_existing_test_replay',label=r['label'],**s) for r,s in zip(rows,scores))
    public=[json.loads(l) for l in (a.assets/'data/test.jsonl').read_text().splitlines()]
    for task in ['privacy','vulnerability','harmful_request']:
        rows=[r for r in public if r['task']==task];scores=model.score(task,rows);reports['public_'+task+'_replay']=metrics([r['label'] for r in rows],scores)
        predictions.extend(dict(id=r['id'],source='public_existing_test_replay',label=r['label'],**s) for r,s in zip(rows,scores))
        print('PUBLIC_MODEL_REPLAY',task,reports['public_'+task+'_replay'],flush=True)
    report={'weights_sha256':model.digests,'selection_sha256':sha(a.assets/'artifacts/selected_models.json'),'reports':reports,'fit_count':0,'threshold_changes':0,'production_approved':False,'live_tool_replay_scope':'candidate text with actual fixture user goal; not kernel outcomes','public_test_scope':'original consumed held-out test re-evaluation, not a new independent generalization test'}
    (a.out/'replay_results.json').write_text(json.dumps(report,indent=2)+'\n');(a.out/'replay_predictions.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in predictions));print(json.dumps(report,indent=2),flush=True)
if __name__=='__main__':main()
