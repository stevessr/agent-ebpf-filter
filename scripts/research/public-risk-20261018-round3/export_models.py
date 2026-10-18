"""Export own fitted checkpoints to non-executable safetensors + JSON."""
import json,hashlib
import numpy as np
import torch
from safetensors.torch import save_file,load_file
from train import OUT,probability,read,predict_bundle,epredict,EndToEnd,tokens
from paths import ROOT
def main():
    dest=ROOT/'release';dest.mkdir(exist_ok=True);records=[]
    paths=sorted(set(OUT.glob('*winner.pt'))|set(OUT.glob('*_seed*.pt'))|set(OUT.glob('*_best.pt')))
    for p in paths:
        b=torch.load(p,weights_only=False);tensors={};meta={k:v for k,v in b.items() if k not in ['state','mean','scale','coef']}
        if 'state' in b:tensors.update({'state.'+k:v.detach().cpu().contiguous() for k,v in b['state'].items()})
        for k in ['mean','scale','coef']:
            if k in b:tensors[k]=torch.from_numpy(np.ascontiguousarray(b[k]))
        f=dest/(p.stem+'.safetensors');save_file(tensors,str(f));(dest/(p.stem+'.json')).write_text(json.dumps(meta,indent=2));tt=load_file(str(f));restored={**meta}
        state={k[len('state.'):]:v for k,v in tt.items() if k.startswith('state.')}
        if state:restored['state']=state
        for k in ['mean','scale','coef']:
            if k in tt:restored[k]=tt[k].numpy()
        task=p.stem.split('_')[0]
        # Verify same numerical predictions on a fixed validation slice.
        if b['config']['front']=='finetuned_tiny':
            d=tokens(task,'validation');d={k:v[:8] for k,v in d.items()};m=EndToEnd(b['config']['back']);m.load_state_dict(b['state']);pa=epredict(m,d);m.load_state_dict(restored['state']);pb=epredict(m,d)
        else:
            x=read(task,b['config']['front'],'validation')['X'][:32];pa=probability(b,x);pb=probability(restored,x)
        err=float(np.max(abs(pa-pb)));assert err<1e-6
        records.append({'file':f.name,'bytes':f.stat().st_size,'sha256':hashlib.sha256(f.read_bytes()).hexdigest(),'max_prediction_error':err,'source_checkpoint':p.name,'config':b['config']})
    (OUT/'exports.json').write_text(json.dumps(records,indent=2));print('exported',len(records),'bytes',sum(r['bytes'] for r in records))
if __name__=='__main__':main()
