"""Reproduce the existing public PrivacyAlign scenario-disjoint preparation."""
import argparse,collections,hashlib,json,re
from pathlib import Path

def norm(s):return re.sub(r'\s+',' ',s).strip().lower()
def digest(s):return hashlib.sha256(s.encode()).hexdigest()
def bucket(s):return int(digest(s)[:8],16)%100
def main():
    p=argparse.ArgumentParser();p.add_argument('--assets',type=Path,required=True);a=p.parse_args();raw=a.assets/'raw';out=a.assets/'agent_privacy_data';out.mkdir(parents=True,exist_ok=True);source={s:[json.loads(x) for x in (raw/f'PrivacyAlign_{s}.jsonl').read_text().splitlines()] for s in ['train','test']}
    def group(r):return digest(norm(r['user_instruction']+'\n'+r['trajectory']+'\n'+json.dumps(r['memories'],sort_keys=True)))
    testgroups={group(r) for r in source['test']};stats=collections.Counter();rows=[];seen={}
    for split in ['test','train']:
        for r in source[split]:
            g=group(r)
            if split=='train' and g in testgroups:stats['train_test_scenario_overlap']+=1;continue
            s='test' if split=='test' else 'train' if bucket(g)<75 else 'validation' if bucket(g)<88 else 'calibration';context='User request: '+r['user_instruction']+'\nUser memories: '+'\n'.join(r['memories'])+'\nPrior tool trajectory: '+r['trajectory']
            for side in ['a','b']:
                votes=[int(v) for annotation in r['annotations'] if type(v:=annotation.get('response_'+side+'_labels',{}).get('leaks')) is bool and annotation.get('preference')!='unsure']
                if not votes or sum(votes)*2==len(votes):stats['unknown_or_tie']+=1;continue
                candidate=r['response_'+side]['action_raw'];key=digest(norm(context+'\n'+candidate));label=int(sum(votes)*2>len(votes))
                if key in seen:
                    stats['duplicate_candidate']+=1
                    if seen[key]!=label:stats['conflicting_duplicate_label']+=1
                    continue
                seen[key]=label;rows.append(dict(id=str(r['id'])+'-'+side,group=g,split=s,context=context,candidate=candidate,text=context+'\nCandidate action: '+candidate,label=label,votes=len(votes)))
    for s in ['train','validation','calibration','test']:
        rr=[r for r in rows if r['split']==s];(out/f'{s}.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in rr));stats[s]=dict(n=len(rr),positive=sum(r['label'] for r in rr),groups=len({r['group'] for r in rr}))
    stats['source_sha256']={s:digest((raw/f'PrivacyAlign_{s}.jsonl').read_text()) for s in source};stats['prepared_sha256']={s:digest((out/f'{s}.jsonl').read_text()) for s in ['train','validation','calibration','test']};(out/'manifest.json').write_text(json.dumps(dict(stats),indent=2));print(json.dumps(dict(stats),indent=2))
if __name__=='__main__':main()
