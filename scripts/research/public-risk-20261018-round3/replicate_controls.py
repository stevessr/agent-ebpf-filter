"""Repeat validation-selected TinyBERT-front control; no new configuration search."""
import json,sys
import torch
from train import OUT,SEED,fit,sha
task=sys.argv[1]
assert (OUT/f'{task}_frozen.json').exists()
base=torch.load(OUT/f'{task}_tiny_best.pt',weights_only=False);rep=[]
for seed in [SEED+1,SEED+2]:
    b,r=fit(task,base['config'],seed);p=OUT/f'{task}_control_seed{seed}.pt';torch.save(b,p);rep.append({**r,'weights_sha256':sha(p)})
(OUT/f'{task}_controls_frozen.json').write_text(json.dumps({'config':base['config'],'replications':rep,'no_architecture_reselection':True},indent=2))
