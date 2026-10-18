"""Frozen public encoders; candidate-conditioned context retrieval without label input."""
import argparse,json,time,hashlib,os
from pathlib import Path
import numpy as np
import torch
from transformers import AutoModel,AutoTokenizer,T5EncoderModel
from paths import old_round
ROOT=Path(__file__).resolve().parent;SPLITS=['train','validation','calibration','confirmation','regression']
os.environ['TOKENIZERS_PARALLELISM']='false';torch.set_num_threads(4)
def load(task):return {s:[json.loads(x) for x in (ROOT/'data'/task/(s+'.jsonl')).read_text().splitlines()] for s in SPLITS}
def chunks(ids,n=190,cap=4):
 cc=[ids[i:i+n] for i in range(0,len(ids),n)] or [[]]
 if len(cc)>cap:cc=[cc[i] for i in np.linspace(0,len(cc)-1,cap,dtype=int)]
 return cc

def extract(task,base):
 t0=time.perf_counter();ds=load(task)
 folder=(old_round()/'base_model') if base=='tiny' else ROOT/'models'/('all-MiniLM-L6-v2' if task=='privacy' else 'codet5-small')
 if base=='tiny':
  from transformers import BertTokenizerFast
  tok=BertTokenizerFast(vocab_file=str(folder/'vocab.txt'),do_lower_case=True)
 else:tok=AutoTokenizer.from_pretrained(folder,local_files_only=True)
 model=(T5EncoderModel if task=='code' and base!='tiny' else AutoModel).from_pretrained(folder,local_files_only=True);model.eval()
 docs=[];keys={};index={};coverage={'original_tokens':0,'selected_tokens':0,'truncated_units':0,'units':0}
 def add(text,cap=1):
  key=hashlib.sha256((str(cap)+text).encode()).hexdigest()
  if key not in keys:
   ids=tok.encode(text,add_special_tokens=False);cc=chunks(ids,190,cap);ix=[]
   coverage['original_tokens']+=len(ids);coverage['selected_tokens']+=sum(map(len,cc));coverage['truncated_units']+=int(len(ids)>190*cap);coverage['units']+=1
   for c in cc:ix.append(len(docs));docs.append(tok.build_inputs_with_special_tokens(c))
   keys[key]=ix
  return keys[key]
 for split,rs in ds.items():
  for r in rs:
   if task=='privacy':
    prefix,_,trace=r['context'].partition('\nPrior tool trajectory: ')
    source=add(prefix,2);traceix=add(trace,6);candidate=add(r['candidate'],2)
    index[r['id']]=(candidate,source,traceix)
   else:
    ids=tok.encode(r['text'],add_special_tokens=False)
    if len(ids)>190:ids=ids[:64]+ids[-126:];coverage['truncated_units']+=1
    coverage['original_tokens']+=len(tok.encode(r['text'],add_special_tokens=False));coverage['selected_tokens']+=len(ids);coverage['units']+=1
    index[r['id']]=[len(docs)];docs.append(tok.build_inputs_with_special_tokens(ids))
 start=time.perf_counter();means=[];maxs=[]
 # Dynamic padding limits unnecessary work while preserving deterministic row order.
 with torch.no_grad():
  for off in range(0,len(docs),64):
   pack=tok.pad({'input_ids':docs[off:off+64]},padding=True,return_tensors='pt');pack={k:v for k,v in pack.items() if k in ['input_ids','attention_mask','token_type_ids']}
   if task=='code' and base!='tiny':pack.pop('token_type_ids',None)
   h=model(**pack).last_hidden_state;mask=pack['attention_mask'].unsqueeze(-1)
   mean=(h*mask).sum(1)/mask.sum(1).clamp(min=1);maximum=h.masked_fill(mask==0,-1e4).max(1).values
   means.append(torch.nn.functional.normalize(mean,dim=1).numpy());maxs.append(torch.nn.functional.normalize(maximum,dim=1).numpy())
   if off%640==0:print(task,base,'encoded',min(off+64,len(docs)),'/',len(docs),'seconds',round(time.perf_counter()-start,1),flush=True)
 mean=np.concatenate(means);maximum=np.concatenate(maxs);out=ROOT/'artifacts'/f'{task}_{base}';out.mkdir(exist_ok=True)
 for split,rs in ds.items():
  features=[]
  for r in rs:
   if task=='privacy':
    q,s,tt=index[r['id']];candidate=mean[q].mean(0);source=mean[s].mean(0);context=mean[s+tt];sim=context@candidate;w=np.exp((sim-sim.max())*5);w=w/w.sum();retrieved=(context*w[:,None]).sum(0)
    f=np.concatenate([candidate,source,np.abs(candidate-source),candidate*source,retrieved,context.mean(0)])
   else:
    ix=index[r['id']][0];f=np.concatenate([mean[ix],maximum[ix]])
   features.append(f)
  np.savez_compressed(out/(split+'.npz'),X=np.array(features,dtype=np.float32),y=np.array([r['label'] for r in rs]),ids=np.array([r['id'] for r in rs]),groups=np.array([r['group'] for r in rs]))
 meta={'task':task,'base':base,'encoder':str(folder),'frozen':True,'encoder_parameters':sum(p.numel() for p in model.parameters()),'hidden_size':mean.shape[1],'encoded_sequences':len(docs),'coverage':coverage,'wall_seconds':time.perf_counter()-t0,'encoding_seconds':time.perf_counter()-start}
 (out/'extraction.json').write_text(json.dumps(meta,indent=2));print('COMPLETE',json.dumps(meta),flush=True)
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--task',choices=['privacy','code'],required=True);p.add_argument('--base',choices=['tiny','new'],required=True);a=p.parse_args();extract(a.task,a.base)
