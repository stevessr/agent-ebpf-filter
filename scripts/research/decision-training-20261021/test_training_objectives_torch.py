"""Optional full-dependency smoke: differentiability of the quality loss.

With no Torch research environment this test is skipped, not silently passed.
"""
import os
import unittest

try:
    import torch
except (ImportError, OSError):
    TORCH_READY = False
else:
    # Code/import regressions must fail rather than masquerade as missing Torch.
    from training_objectives import pairwise_loss, training_loss
    TORCH_READY = True

if os.environ.get('REQUIRE_TORCH_TESTS') == '1' and not TORCH_READY:
    raise RuntimeError('PyTorch gradient tests are mandatory in this CI job')


@unittest.skipUnless(TORCH_READY, "PyTorch unavailable")
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

    def test_same_group_across_sources_does_not_create_training_pair(self):
        rows = [dict(source='a', group='same'), dict(source='b', group='same')]
        logits = torch.tensor([0.1, 0.9], requires_grad=True)
        loss = pairwise_loss(logits, torch.tensor([0.0, 1.0]), rows, [0, 1])
        self.assertEqual(float(loss.detach()), 0.0)
        loss.backward()
        self.assertEqual(logits.grad.tolist(), [0.0, 0.0])

    def test_gradient_descent_moves_risky_up_and_benign_down(self):
        loss = training_loss(self.logits, self.labels, self.rows, list(range(5)),
                             hard_negative_weight=3.0, pair_weight=0.4,
                             positive_weight=2.0, pair_strategy='hardest')
        loss.backward()
        self.assertTrue((self.logits.grad[self.labels == 1] < 0).all().item())
        self.assertTrue((self.logits.grad[self.labels == 0] > 0).all().item())

    def test_actual_optimizer_step_reduces_separable_objective(self):
        optimizer = torch.optim.SGD([self.logits], lr=0.1)
        def objective():
            return training_loss(self.logits, self.labels, self.rows, list(range(5)),
                                 hard_negative_weight=3.0, pair_weight=0.4,
                                 positive_weight=2.0, pair_strategy='hardest')
        before = objective()
        before.backward()
        optimizer.step()
        self.assertLess(float(objective().detach()), float(before.detach()))

    def test_extreme_logits_keep_loss_and_gradients_finite(self):
        logits = torch.tensor([-10000., 10000., 0., -10000., 10000.], requires_grad=True)
        loss = training_loss(logits, self.labels, self.rows, list(range(5)),
                             hard_negative_weight=3.0, pair_weight=0.4,
                             positive_weight=2.0, pair_strategy='hardest')
        self.assertTrue(torch.isfinite(loss).item())
        loss.backward()
        self.assertTrue(torch.isfinite(logits.grad).all().item())


if __name__ == "__main__":
    unittest.main()

