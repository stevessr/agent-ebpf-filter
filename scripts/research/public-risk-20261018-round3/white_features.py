import os
os.environ['ORT_DISABLE_TELEMETRY']='1'
import hashlib,importlib.util,json
import numpy as np
import torch
from transformers import AutoModel,AutoTokenizer,BertTokenizerFast
import onnxruntime as ort
from tokenizers import Tokenizer
from paths import ROOT,R1,R2
ort.disable_telemetry_events();torch.set_num_threads(2)
def prepare():
    records=[json.loads(l) for l in (ROOT/'white/records.jsonl').read_text().splitlines()];ds={'white_train':[],'white_audit':[]}
    for r in records:
        if not r['accepted']:continue
        trace={'tool':'read','input':{'filePath':'fixture.json'},'output':r['fixture']}
        row={'id':'white:'+r['id'],'group':'white-family:'+r['family'],'label':0,'context':'User request: '+r['prompt']+'\nUser memories: \nPrior tool trajectory: '+json.dumps(trace),'candidate':r['final_text'],'source':'opencode_1.19.3_verified_readonly','model':r['model'],'label_vulnerability':None,'label_destruction':0}
        ds['white_train' if r['split']=='train' else 'white_audit'].append(row)
    for s,rr in ds.items():(ROOT/'data/privacy'/(s+'.jsonl')).write_text(''.join(json.dumps(r)+'\n' for r in rr))
    assert not {r['group'] for r in ds['white_train']}&{r['group'] for r in ds['white_audit']}
    return ds
def chunks(ids,cap):
    cc=[ids[i:i+190] for i in range(0,len(ids),190)] or [[]]
    return [cc[i] for i in np.linspace(0,len(cc)-1,cap,dtype=int)] if len(cc)>cap else cc
def semantic(ds,base):
    folder=R1/'base_model' if base=='tiny' else R2/'models/all-MiniLM-L6-v2'
    tok=BertTokenizerFast(vocab_file=str(folder/'vocab.txt'),do_lower_case=True) if base=='tiny' else AutoTokenizer.from_pretrained(folder,local_files_only=True)
    model=AutoModel.from_pretrained(folder,local_files_only=True).eval()
    spec=importlib.util.spec_from_file_location('r2_evidence',R2/'augment_evidence.py');module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    def enc(text,cap):
        docs=[tok.build_inputs_with_special_tokens(c) for c in chunks(tok.encode(text,add_special_tokens=False),cap)];x=tok.pad({'input_ids':docs},padding=True,return_tensors='pt')
        with torch.no_grad():h=model(**x).last_hidden_state;mask=x['attention_mask'][:,:,None];z=(h*mask).sum(1)/mask.sum(1).clamp(min=1);return nnorm(z).numpy()
    for s,rr in ds.items():
        xx=[]
        for r in rr:
            prefix,_,trace=r['context'].partition('\nPrior tool trajectory: ');q=enc(r['candidate'],2).mean(0);source=enc(prefix,2);history=enc(trace,6);context=np.r_[source,history];sim=context@q;w=np.exp((sim-sim.max())*5);w/=w.sum();retrieved=(context*w[:,None]).sum(0);src=source.mean(0);xx.append(np.r_[q,src,abs(q-src),q*src,retrieved,context.mean(0),module.evidence(r)])
        save(base,s,rr,np.array(xx,dtype='float32'))
def nnorm(z):return torch.nn.functional.normalize(z,dim=1)
def save(front,s,rr,x):
    dst=ROOT/'features'/f'privacy_{front}';dst.mkdir(exist_ok=True,parents=True);np.savez_compressed(dst/(s+'.npz'),X=x,y=np.array([r['label'] for r in rr]),ids=np.array([r['id'] for r in rr]),groups=np.array([r['group'] for r in rr]))
def guard(ds):
    folder=ROOT/'models/astrlink-guard/int8';tok=Tokenizer.from_file(str(folder/'tokenizer.json'));opt=ort.SessionOptions();opt.intra_op_num_threads=2;opt.inter_op_num_threads=1;sess=ort.InferenceSession(str(folder/'model_int8.onnx'),sess_options=opt,providers=['CPUExecutionProvider']);labels=json.loads((folder/'config.json').read_text())['id2label'];kinds=sorted({v.split('-',1)[-1] for v in labels.values() if v!='O'});kindids=[[int(i) for i,v in labels.items() if v.endswith('-'+k)] for k in kinds]
    def enc(text):
        ids=tok.encode(text,add_special_tokens=False).ids;cc=[ids[i:i+384] for i in range(0,max(1,len(ids)),320)]
        if len(cc)>3:cc=[cc[i] for i in np.linspace(0,len(cc)-1,3,dtype=int)]
        arr=[]
        for c in cc:
            x=np.array([[0]+c+[2]],dtype=np.int64);z=sess.run(['logits'],{'input_ids':x,'attention_mask':np.ones_like(x),'token_type_ids':np.zeros_like(x)})[0][0,1:-1]
            if not len(z):continue
            z=np.exp(z-z.max(-1,keepdims=True));z/=z.sum(-1,keepdims=True);arr.append(np.stack([z[:,ii].sum(1) for ii in kindids],axis=1))
        a=np.concatenate(arr) if arr else np.zeros((1,len(kinds)));return np.r_[a.max(0),(a>=.8).sum(0).clip(0,40)/40,a.mean(0)].astype('float32')
    for s,rr in ds.items():
        xx=[]
        for r in rr:
            request,_,history=r['context'].partition('\nUser memories: ');c=enc(r['candidate']);q=enc(request);h=enc(history);xx.append(np.r_[c,q,h,np.maximum(c-q,0),c*h])
        x=np.array(xx,dtype='float32');save('guard',s,rr,x);sem=np.load(ROOT/'features/privacy_new'/(s+'.npz'))['X'];save('hybrid',s,rr,np.c_[sem,x])
if __name__=='__main__':
    ds=prepare()
    for base in ['tiny','new']:semantic(ds,base)
    guard(ds);print('white features completed', {s:len(rr) for s,rr in ds.items()})
