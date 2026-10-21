"""Optional full-dependency smoke: differentiability of the quality loss.

With no Torch research environment this test is skipped, not silently passed.
"""
import unittest

try:
    import torch
    from train_privacy_v2 import pairwise_loss, training_loss
    TORCH_READY = True
except (ImportError, OSError):
    TORCH_READY = False


@unittest.skipUnless(TORCH_READY, "Torch / safetensors / transformers research environment unavailable")
class QualityGradientTests(unittest.TestCase):
    def setUp(self):
        self.rows = [{"group": "one"}, {"group": "one"}, {"group": "one"},
                     {"group": "two"}, {"group": "two"}]
        self.labels = torch.tensor([1, 0, 0, 1, 0], dtype=torch.float32)
        self.logits = torch.tensor([0.1, 0.5, 0.8, 0.2, 0.6],
                                   dtype=torch.float32, requires_grad=True)

    def test_hardest_group_pair_exceeds_average(self):
        hardest = pairwise_loss(self.logits, self.labels, self.rows,
                                list(range(5)), strategy="hardest")
        averaged = pairwise_loss(self.logits, self.labels, self.rows,
                                 list(range(5)), strategy="all")
        self.assertGreaterEqual(float(hardest.detach()), float(averaged.detach()))

    def test_asymmetric_loss_gradients_are_finite(self):
        loss = training_loss(self.logits, self.labels, self.rows, list(range(5)),
                             hard_negative_weight=3.0, pair_weight=0.4,
                             positive_weight=2.0, pair_strategy="hardest")
        loss.backward()
        self.assertTrue(torch.isfinite(self.logits.grad).all().item())
        self.assertGreater(float(self.logits.grad.abs().sum()), 0.0)


if __name__ == "__main__":
    unittest.main()
