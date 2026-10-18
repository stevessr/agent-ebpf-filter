"""Frozen public-source transfer, fair-threshold baseline and nuisance checks."""
import argparse,importlib.util,json,time
from pathlib import Path
from collections import Counter
import numpy as np
from inference import Frozen
from privacy import measure,threshold,sha

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--root',type=Path,required=True);ap.add_argument('--assets',type=Path,required=True);a=ap.parse_args();model=Frozen(a.root,a.assets);out=a.root/'evaluation';out.mkdir(exist_ok=True)
    # Attribute appropriateness is a different target from action leakage.
    # Ground-truth strings, private archetype and oracle utility are never inputs.
    public=a.root/'pisas';protocol=json.loads((public/'protocol.json').read_text());rows=[];files={}
    for folder in protocol['folders']:
        base=public/folder;s=json.loads((base/'scenario.json').read_text());labels=json.loads((base/'appropriateness.json').read_text())['attributes'];attrs=json.loads((base/'visibility.json').read_text())['attributes']
        for filename in ['scenario.json','appropriateness.json','visibility.json']:files[folder+'/'+filename]=sha(base/filename)
        task='\n'.join(x['content'] for x in s['timeline'] if x.get('type')=='task') or s['task'].get('description') or s['task'].get('type','')
        background='\n'.join(str(x.get('author',''))+': '+x['content'] for x in s['timeline'] if x.get('type')!='task')
        context='User request: '+task+'\nUser memories: '+background
        for attribute,label in labels.items():
            if label not in ['appropriate','inappropriate'] or attribute not in attrs:continue
            candidate=json.dumps(dict(type='tool_use',name='SlackSendMessage',arguments=dict(recipient='task-requester',message=attrs[attribute]['value'])),ensure_ascii=False)
            rows.append(dict(id=folder+'/'+attribute,group=folder,task=folder.split('/')[0],context=context,candidate=candidate,label=int(label=='inappropriate')))
    scores=[]
    for off in range(0,len(rows),32):scores+=model.score('privacy',rows[off:off+32])
    result=dict(scope='PiSAs task-appropriateness attribute disclosure diagnostic, adapted JSON action; NOT original benchmark scores, NOT observed privacy leaks',source_revision=protocol['revision'],scenario_count=len(protocol['folders']),input_policy='task text + artifact author/content only; no appropriateness enum, visibility list, oracle or archetype field',overall=measure(np.array([r['label'] for r in rows]),np.array([s['score'] for s in scores]),model.config['privacy']['threshold']),by_task={})
    for task in sorted({r['task'] for r in rows}):
        ii=[i for i,r in enumerate(rows) if r['task']==task];result['by_task'][task]=measure(np.array([rows[i]['label'] for i in ii]),np.array([scores[i]['score'] for i in ii]),model.config['privacy']['threshold'])
    (out/'pisas_predictions.jsonl').write_text(''.join(json.dumps(dict(id=r['id'],group=r['group'],label=r['label'],**s))+'\n' for r,s in zip(rows,scores)))
    (out/'pisas_source_hashes.json').write_text(json.dumps(files,indent=2)+'\n');(out/'pisas_results.json').write_text(json.dumps(result,indent=2)+'\n');print('PISAS',json.dumps(result),flush=True)
    # Identical old/new model operating-point protocol: empirical calibration 1%.
    oldpath=Path(__file__).resolve().parent.parent/'model-opencode-20261018/scoring.py';spec=importlib.util.spec_from_file_location('legacy',oldpath);old=importlib.util.module_from_spec(spec);spec.loader.exec_module(old);baseline=old.Scorer(a.assets)
    ds={s:[json.loads(x) for x in (a.assets/'agent_privacy_data'/f'{s}.jsonl').read_text().splitlines()] for s in ['calibration','test']};pp={}
    for split,rr in ds.items():
        pp[split]=[]
        for off in range(0,len(rr),32):pp[split]+=[s['score'] for s in baseline.score('agent_privacy',rr[off:off+32])]
        pp[split]=np.array(pp[split])
    t=threshold(np.array([r['label'] for r in ds['calibration']]),pp['calibration']);fair=dict(scope='old frozen BERT, threshold-only baseline; calibration-only refit, test previously consumed',threshold=t,consumed_test=measure(np.array([r['label'] for r in ds['test']]),pp['test'],t))
    (out/'threshold_only_baseline.json').write_text(json.dumps(fair,indent=2)+'\n');print('BASELINE',json.dumps(fair),flush=True)
    # Same request, nuisance-only changes. Deterministic invariance is not recall.
    events=[]
    for authority in ['127.0.0.1:8080','example.invalid','random-origin.invalid:443']:
        for agent in ['Python-urllib/3.12','OpenCode-Test','Mozilla/5.0']:
            events.append(dict(http=dict(method='POST',target='http://'+authority+'/api/echo?q=hello',headers=[dict(name='Host',value=authority),dict(name='User-Agent',value=agent),dict(name='Cookie',value='session=abc123')],body='name=unit+test')))
    ps=model.score('http',events);nuisance=dict(n=len(ps),max_score_difference=max(r['score'] for r in ps)-min(r['score'] for r in ps),model_sha256=model.config['http']['artifact_sha256'],scope='authority/User-Agent/Cookie invariance diagnostic only; header attacks remain a coverage gap for payload-only models')
    (out/'nuisance_results.json').write_text(json.dumps(nuisance,indent=2)+'\n');print('NUISANCE',json.dumps(nuisance),flush=True)
if __name__=='__main__':main()
