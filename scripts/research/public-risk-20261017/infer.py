"""Trusted local safetensors inference. Advisory output, no actions or network."""
import argparse,json
from pathlib import Path
import numpy as np
from safetensors.torch import load_file
from transformers import BertTokenizerFast
from train_public import ROOT,RiskBert,TASKS,encoded,infer,sig
from train_agent_privacy import encode,predict

def main():
    p=argparse.ArgumentParser();p.add_argument('--task',choices=TASKS+['agent_privacy'],required=True);p.add_argument('--input',required=True);p.add_argument('--selection',default=str(ROOT/'artifacts/selected_models.json'));a=p.parse_args()
    selection=json.loads(Path(a.selection).read_text());group='agent_privacy' if a.task=='agent_privacy' else 'auxiliary';cfg=selection[group]
    m=RiskBert(pretrained=False);m.load_state_dict(load_file(str(ROOT/'artifacts'/cfg['weights'])));m.eval()
    tok=BertTokenizerFast(vocab_file=str(ROOT/'base_model/vocab.txt'),do_lower_case=True)
    rs=[json.loads(x) for x in Path(a.input).read_text().splitlines() if x.strip()]
    if a.task=='agent_privacy':
        if any(not isinstance(r.get('context'),str) or not isinstance(r.get('candidate'),str) for r in rs):raise ValueError('agent_privacy requires string context and candidate')
        e,trunc=encode(rs,tok);z=predict(m,e);head=cfg['scoring']
    else:
        if any(not isinstance(r.get('text'),str) for r in rs):raise ValueError('auxiliary input requires string text')
        z=infer(m,encoded(rs,tok,192))[:,TASKS.index(a.task)];head=cfg['scoring'][a.task];trunc=None
    cal=head['calibration'];prob=sig(z if cal is None else cal['slope']*z+cal['intercept'])
    for r,pp in zip(rs,prob):
        print(json.dumps({'id':r.get('id'),'task':a.task,'score':float(pp),'threshold':head['threshold'],'recommendation':'review' if pp>=head['threshold'] else 'observe','production_calibrated':False,'automatic_execution':False,'context_truncation_possible':True},ensure_ascii=False))
if __name__=='__main__':main()
