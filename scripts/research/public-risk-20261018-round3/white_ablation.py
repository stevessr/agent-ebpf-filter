"""Predeclared augmentation ablation; never chooses on white-audit labels."""
import json
import numpy as np
import torch
from paths import ROOT
from train import OUT,SEED,fit,predict_bundle,metrics,sha
def train():
    frozen=json.loads((OUT/'privacy_frozen.json').read_text());cfg={**frozen['selected'],'white_augmentation':True};rep=[]
    for seed in [SEED,SEED+1,SEED+2]:
        b,r=fit('privacy',cfg,seed);p=OUT/f'privacy_white_seed{seed}.pt';torch.save(b,p);rep.append({**r,'weights_sha256':sha(p)})
    (OUT/'white_frozen.json').write_text(json.dumps({'config':cfg,'replications':rep,'augmentation_rows':len(np.load(ROOT/'features/privacy_tiny/white_train.npz')['y']),'weight_per_row':1,'audit_not_used_for_selection':True},indent=2))
def evaluate():
    assert (OUT/'white_frozen.json').exists();out=[]
    for variant in ['base','white']:
        for seed in [SEED,SEED+1,SEED+2]:
            p=OUT/(f'privacy_white_seed{seed}.pt' if variant=='white' else 'privacy_winner.pt' if seed==SEED else f'privacy_seed{seed}.pt');b=torch.load(p,weights_only=False);r={'variant':variant,'seed':seed,'splits':{}}
            for s in ['white_audit','audit','regression']:
                d,pred=predict_bundle('privacy',b,s)
                if s=='white_audit':
                    n=len(pred);fp=int(sum(pred>=b['threshold']));r['splits'][s]={'n':n,'FP':fp,'FPR':fp/n,'note':'all-negative diagnostic; no accuracy/AP/recall claim, only two held-out task families'}
                else:r['splits'][s]=metrics(d['y'],pred,b['threshold'])
                np.savez_compressed(OUT/f'white_{variant}_{seed}_{s}_predictions.npz',p=pred,y=d['y'],groups=d['groups'],threshold=b['threshold'])
            out.append(r)
    (OUT/'white_results.json').write_text(json.dumps(out,indent=2));print('white ablation evaluated',flush=True)
if __name__=='__main__':
    import sys
    (train if sys.argv[1]=='train' else evaluate)()
