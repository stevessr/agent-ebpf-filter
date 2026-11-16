"""Frozen research inference. This module never issues enforcement decisions."""
import json, math
from pathlib import Path
import joblib
import torch
from transformers import BertTokenizerFast
from safetensors.torch import load_file
from privacy import PairBert,compact,encode,predict,sha
from http_model import transform
from structured import extract
from corrected import probabilities

class Frozen:
    def __init__(self,root,assets):
        self.root=Path(root);self.assets=Path(assets);torch.set_num_threads(2);self.config={};self.models={}
        for task in ['privacy','http','http_structured','http_v2','http_structured_v3']:
            directory=self.root/task;f=json.loads((directory/'freeze.json').read_text());name=f.get('selected','structured');path=directory/('model.joblib' if task in ['http_structured','http_v2','http_structured_v3'] else name+('.safetensors' if name.endswith('_pair') else '.joblib'))
            if sha(path)!=f['artifact_sha256']:raise ValueError('frozen artifact checksum mismatch')
            if sha(directory/'protocol.json')!=f['protocol_sha256']:raise ValueError('protocol checksum mismatch')
            if f['mode']!='shadow_only' or f['production_approved'] is not False:raise ValueError('research artifact required')
            t=f['threshold']
            if not math.isfinite(t) or not 0<=t<=1:raise ValueError('invalid threshold')
            if name.endswith('_pair'):
                base=self.assets/'base_model' if name=='tiny_pair' else self.root/'base_mini';m=PairBert(base,False);m.load_state_dict(load_file(str(path)));m.eval()
            else:m=joblib.load(path) # Only supervisor-owned, locally trained, digest-pinned artifacts.
            self.config[task]=f;self.models[task]=m
        self.tok=BertTokenizerFast(vocab_file=str(self.assets/'base_model/vocab.txt'),do_lower_case=True)
    def score(self,task,rows):
        f=self.config[task];m=self.models[task]
        if task=='privacy':
            if f['selected'].endswith('_pair'):p=predict(m,encode(rows,self.tok))
            else:
                selected=f['selection'];v,model=m;strings=[r['candidate'] if selected['view']=='candidate' else compact(r['context'],r['candidate'])+'\nACTION '+r['candidate'] for r in rows];p=model.predict_proba(v.transform(strings))[:,1]
        elif task=='http':p=m['model'].predict_proba(transform(m,rows))[:,1]
        elif task=='http_structured':
            exe=self.root/'behavior-features'
            if sha(exe)!=m['extractor_sha256']:raise ValueError('extractor checksum mismatch')
            x,names=extract(rows,exe)
            if names!=m['feature_names']:raise ValueError('feature contract mismatch')
            p=m['model'].predict_proba(x[:,m['indices']])[:,1]
        else:p=probabilities(m,rows,self.root/'behavior-features')
        return [dict(task=task,score=float(q),threshold=f['threshold'],above_threshold=bool(q>f['threshold']),model_sha256=f['artifact_sha256'],mode='shadow_only',automatic_execution=False) for q in p]
