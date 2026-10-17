"""Fail if prepared splits have group overlap or text/target contract violations."""
import json
from pathlib import Path
from collections import defaultdict
ROOT=Path(__file__).resolve().parent
for directory in ['data','agent_privacy_data']:
    groups=defaultdict(lambda:defaultdict(set));counts={}
    for split in ['train','validation','calibration','test']:
        rs=[json.loads(x) for x in (ROOT/directory/(split+'.jsonl')).read_text().splitlines()]
        counts[split]=len(rs)
        for r in rs:
            assert r['label'] in (0,1)
            assert isinstance(r['text'],str) and r['text'].strip()
            task=r.get('task','agent_privacy');groups[task][split].add(r['group'])
    for task,sets in groups.items():
        for a,ga in sets.items():
            for b,gb in sets.items():
                if a!=b:assert not ga&gb,(directory,task,a,b)
    print(directory,'group isolation checked',counts)
for n in ['results.json','agent_privacy_results.json']:
    p=ROOT/'artifacts'/n
    if p.exists():
        r=json.loads(p.read_text())
        for name,results in r['models'].items():
            rr=[results] if 'test' in results else list(results.values())
            for x in rr:
                m=x['test'];c=m['confusion'];assert sum(c.values())==m['n'];assert c['tp']+c['fn']==m['positive']
                assert 0<=m['F1_positive']<=1 and 0<=m['PR_AUC_AP']<=1
        print(n,'metric counts checked')
