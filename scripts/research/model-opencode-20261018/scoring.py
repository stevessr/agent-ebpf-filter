"""Inference with the actual frozen public-data safetensors; no fitting/rules."""
import hashlib,json,math,time
from pathlib import Path
import torch
from torch import nn
from transformers import BertModel,BertConfig,BertTokenizerFast
from safetensors.torch import load_file

EXPECTED={'agent_privacy_transfer_balanced.safetensors':'9aec5b2e0ec582b9b5ec76cbf8f722a5379fa34d41e6c70e431328573f586845','bert_balanced.safetensors':'b9fdf336affc5f45d348ea8fd1af043194161c36c1fdf0adef9143c1c0979701'}
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
class RiskBert(nn.Module):
    def __init__(self,config):
        super().__init__();self.encoder=BertModel(BertConfig.from_json_file(str(config)))
        self.head=nn.Sequential(nn.Dropout(.2),nn.Linear(128,64),nn.GELU(),nn.Dropout(.2),nn.Linear(64,3))
    def forward(self,input_ids,attention_mask,token_type_ids):
        h=self.encoder(input_ids=input_ids,attention_mask=attention_mask,token_type_ids=token_type_ids).last_hidden_state
        mask=attention_mask.unsqueeze(-1);return self.head((h*mask).sum(1)/mask.sum(1).clamp(min=1))
class Scorer:
    def __init__(self,assets):
        torch.set_num_threads(4);self.assets=Path(assets)
        self.selection=json.loads((self.assets/'artifacts/selected_models.json').read_text())
        self.digests={name:sha(self.assets/'artifacts'/name) for name in EXPECTED}
        assert self.digests==EXPECTED,'Frozen weight checksum mismatch'
        assert sha(self.assets/'base_model/vocab.txt')=='07eced375cec144d27c900241f3e339478dec958f92fddbc551f295c992038a3'
        assert sha(self.assets/'base_model/config.json')=='508e1f01aae55d73355cbdd82609be2f43ba5a0d3428837adbe56cf8391f8b39'
        assert self.selection['agent_privacy']['scoring']['threshold']==.5
        assert self.selection['auxiliary']['scoring']['vulnerability']['threshold']==0.39785023346056037
        self.models={}
        for group in ['agent_privacy','auxiliary']:
            name=self.selection[group]['weights'];m=RiskBert(self.assets/'base_model/config.json');m.load_state_dict(load_file(str(self.assets/'artifacts'/name)));m.eval();self.models[group]=m
        self.tok=BertTokenizerFast(vocab_file=str(self.assets/'base_model/vocab.txt'),do_lower_case=True)
    def score(self,task,rows):
        start=time.perf_counter();trunc=[]
        if task=='agent_privacy':
            seq=[];types=[];masks=[]
            for r in rows:
                context=self.tok.encode(r['context'],add_special_tokens=False);candidate=self.tok.encode(r['candidate'],add_special_tokens=False)
                trunc.append({'context':len(context)>190,'candidate':len(candidate)>126})
                crop=lambda ids,n:ids if len(ids)<=n else ids[:n//3]+ids[-(n-n//3):]
                c=crop(context,190);a=crop(candidate,126);ids=[self.tok.cls_token_id]+c+[self.tok.sep_token_id]+a+[self.tok.sep_token_id];n=320-len(ids)
                seq.append(ids+[self.tok.pad_token_id]*n);types.append([0]*(len(c)+2)+[1]*(len(a)+1)+[0]*n);masks.append([1]*len(ids)+[0]*n)
            e={'input_ids':torch.tensor(seq),'token_type_ids':torch.tensor(types),'attention_mask':torch.tensor(masks)};group='agent_privacy';head=0;cfg=self.selection[group]['scoring']
        else:
            head=['privacy','vulnerability','harmful_request'].index(task);group='auxiliary';cfg=self.selection[group]['scoring'][task]
            trunc=[{'text':len(self.tok.encode(r['text'],add_special_tokens=True))>192} for r in rows]
            e=dict(self.tok([r['text'] for r in rows],padding='max_length',truncation=True,max_length=192,return_tensors='pt'))
        z=[]
        with torch.inference_mode():
            for off in range(0,len(rows),64):z.extend(self.models[group](**{k:v[off:off+64] for k,v in e.items()})[:,head].tolist())
        elapsed=(time.perf_counter()-start)*1000;result=[]
        for raw,t in zip(z,trunc):
            cal=cfg['calibration'];logit=raw if cal is None else cal['slope']*raw+cal['intercept'];p=1/(1+math.exp(-max(-60,min(60,logit))))
            result.append({'task':task,'score':p,'threshold':cfg['threshold'],'above_threshold':p>=cfg['threshold'],'model_sha256':self.digests[self.selection[group]['weights']],'truncation':t,'mode':'shadow_only','batch_elapsed_ms':elapsed,'automatic_execution':False})
        return result

def metrics(labels,scores):
    tp=fp=tn=fn=0
    for y,r in zip(labels,scores):
        p=r['above_threshold'];tp+=int(y==1 and p);fp+=int(y==0 and p);tn+=int(y==0 and not p);fn+=int(y==1 and not p)
    return {'n':len(labels),'tp':tp,'fp':fp,'tn':tn,'fn':fn,'recall':tp/(tp+fn) if tp+fn else None,'fpr':fp/(fp+tn) if fp+tn else None,'accuracy':(tp+tn)/len(labels) if labels else None,'above_threshold':tp+fp,'threshold':scores[0]['threshold'] if scores else None,'truncated_rows':sum(any(r['truncation'].values()) for r in scores)}
