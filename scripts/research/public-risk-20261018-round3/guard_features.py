"""Token-level evidence features; NOT the release's production span decoder."""
import os
# Full-process opt-out must precede import/initialization; API alone is too late.
# https://github.com/microsoft/onnxruntime/blob/main/docs/Privacy.md
os.environ['ORT_DISABLE_TELEMETRY']='1'
import json,time,hashlib
import numpy as np
import onnxruntime as ort
ort.disable_telemetry_events()
from tokenizers import Tokenizer
from paths import ROOT
from prepare import rows,SPLITS
def main():
    folder=ROOT/'models/astrlink-guard/int8';tok=Tokenizer.from_file(str(folder/'tokenizer.json'))
    opt=ort.SessionOptions();opt.intra_op_num_threads=3;opt.inter_op_num_threads=1
    session=ort.InferenceSession(str(folder/'model_int8.onnx'),sess_options=opt,providers=['CPUExecutionProvider'])
    labels=json.loads((folder/'config.json').read_text())['id2label'];kinds=sorted({v.split('-',1)[-1] for v in labels.values() if v!='O'})
    kindids=[[int(i) for i,v in labels.items() if v.endswith('-'+k)] for k in kinds]
    cache={};seqs=0;orig=0;kept=0;truncated=0;t0=time.time()
    def encode(text):
        nonlocal seqs,orig,kept,truncated
        key=hashlib.sha256(text.encode()).hexdigest()
        if key in cache:return cache[key]
        ids=tok.encode(text,add_special_tokens=False).ids;orig+=len(ids)
        # 384-token chunks; at most three uniformly spread windows, overlap 64.
        cc=[ids[i:i+384] for i in range(0,max(1,len(ids)),320)]
        if len(cc)>3:cc=[cc[i] for i in np.linspace(0,len(cc)-1,3,dtype=int)];truncated+=1
        kept+=sum(map(len,cc));seqs+=len(cc);arr=[]
        for c in cc:
            xx=np.array([[0]+c+[2]],dtype=np.int64)
            z=session.run(['logits'],{'input_ids':xx,'attention_mask':np.ones_like(xx),'token_type_ids':np.zeros_like(xx)})[0][0,1:-1]
            if not len(z):continue
            z=np.exp(z-z.max(-1,keepdims=True));z/=z.sum(-1,keepdims=True)
            arr.append(np.stack([z[:,ii].sum(1) for ii in kindids],axis=1))
        a=np.concatenate(arr) if arr else np.zeros((1,len(kinds)))
        v=np.r_[a.max(0),(a>=.8).sum(0).clip(0,40)/40,a.mean(0)].astype('float32');cache[key]=v;return v
    dest=ROOT/'features/privacy_guard';dest.mkdir(parents=True,exist_ok=True)
    hybrid=ROOT/'features/privacy_hybrid';hybrid.mkdir(parents=True,exist_ok=True)
    for s in SPLITS:
        rs=rows('privacy',s);xx=[]
        for i,r in enumerate(rs):
            request,_,history=r['context'].partition('\nUser memories: ')
            c=encode(r['candidate']);q=encode(request);h=encode(history)
            xx.append(np.r_[c,q,h,np.maximum(c-q,0),c*h])
            if i%100==0:print(s,i,'seconds',round(time.time()-t0),flush=True)
        d=dict(np.load(ROOT/'features/privacy_new'/(s+'.npz'),allow_pickle=False));semantic=d['X'];d['X']=np.array(xx,dtype='float32');np.savez_compressed(dest/(s+'.npz'),**d)
        d['X']=np.c_[semantic,d['X']];np.savez_compressed(hybrid/(s+'.npz'),**d)
    meta={'kinds':kinds,'dimensions':len(kinds)*15,'decoder':'token BIO category probability aggregates; no production span parity','max_windows_per_text':3,'chunk_tokens':384,'stride':320,'sequences':seqs,'original_tokens':orig,'selected_tokens_with_overlap':kept,'truncated_unique_texts':truncated,'unique_texts':len(cache),'seconds':time.time()-t0,'frozen':True}
    (ROOT/'artifacts/guard_extraction.json').write_text(json.dumps(meta,indent=2));print(meta,flush=True)
if __name__=='__main__':main()
