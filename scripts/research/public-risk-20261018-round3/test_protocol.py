"""Tests for data leakage, calibration constraint and trainable-encoder claims."""
import itertools,json,unittest
import numpy as np
import torch
from paths import ROOT
from prepare import rows,SPLITS
from train import threshold,metrics,configurations,Residual,Gated,EndToEnd
class ProtocolTests(unittest.TestCase):
    def test_group_isolation_and_cache_alignment(self):
        for task in ['privacy','code']:
            ds={s:rows(task,s) for s in SPLITS};gg={s:{r['group'] for r in rr} for s,rr in ds.items()}
            for a,b in itertools.combinations(gg,2):self.assertFalse(gg[a]&gg[b])
            for front in ['tiny','new']:
                for s,rr in ds.items():
                    d=dict(np.load(ROOT/'features'/f'{task}_{front}'/(s+'.npz'),allow_pickle=False));self.assertEqual(d['ids'].tolist(),[r['id'] for r in rr]);self.assertEqual(d['y'].tolist(),[r['label'] for r in rr]);self.assertTrue(np.isfinite(d['X']).all())
    def test_threshold_ties_cannot_break_fpr_cap(self):
        y=np.array([0]*99+[1]*140);p=np.array([.5]*239);t=threshold(y,p);self.assertGreater(t,.5);self.assertEqual(metrics(y,p,t)['FPR'],0)
        rng=np.random.default_rng(91)
        for _ in range(20):
            p=np.round(rng.random(len(y)),2);t=threshold(y,p);self.assertLessEqual(metrics(y,p,t)['FPR'],.05)
    def test_matrix_counts(self):self.assertEqual(sum(len(configurations(t))+2 for t in ['privacy','code']),34)
    def test_encoder_receives_gradient_for_both_poolers(self):
        for back in ['mean','cnn']:
            m=EndToEnd(back);x=torch.tensor([[101,2023,2003,1037,3231,102]]);mask=torch.ones_like(x);m(x,mask).sum().backward();g=m.encoder.embeddings.word_embeddings.weight.grad
            self.assertIsNotNone(g);self.assertGreater(float(g.abs().sum()),0);self.assertTrue(all(p.requires_grad for p in m.parameters()))
    def test_head_gradient(self):
        for cls in [Residual,Gated]:
            m=cls(12);m(torch.randn(8,12)).sum().backward();self.assertTrue(all(p.grad is not None for p in m.parameters()))
if __name__=='__main__':unittest.main()
