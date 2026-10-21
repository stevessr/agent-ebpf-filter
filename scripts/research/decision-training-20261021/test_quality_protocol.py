"""Group-isolated validation and low-FPR objective search contract tests."""
import unittest
from quality_protocol import group_folds, crossfit_validation
from automl_protocol import trial_plan, candidate_rank


def samples(n=12):
    return [dict(group=f"scenario_{g}", label=y) for g in range(n) for y in (0, 1)]


class QualityProtocolTests(unittest.TestCase):
    def test_complete_scenarios_in_folds(self):
        rows = samples(10)
        folds = group_folds(rows)
        self.assertEqual(folds, group_folds(rows))
        self.assertEqual(sorted(folds[0] + folds[1]), list(range(len(rows))))
        for g in range(10):
            self.assertEqual(2*g in folds[0], (2*g+1) in folds[0])

    def test_crossfit_clean_signal(self):
        rows = samples()
        scores = [0.1 if r["label"] == 0 else 0.9 for r in rows]
        result = crossfit_validation(rows, scores)
        self.assertEqual(result["fp"], 0)
        self.assertEqual(result["tp"], 12)
        self.assertTrue(result["measured_out_of_fold"])
        self.assertFalse(result["proven_independent"])
        self.assertEqual(len(result["folds"]), 2)

    def test_crossfit_detects_shift_masked_by_self_threshold(self):
        rows = samples(8)
        folds = group_folds(rows)
        shifted = set(folds[1])
        scores = [0.9 if i in shifted and r["label"] == 0
                  else 0.1 if r["label"] == 0 else 0.95
                  for i, r in enumerate(rows)]
        result = crossfit_validation(rows, scores)
        self.assertGreater(result["fpr"], 0.1)

    def test_unstratifiable_folds_fail(self):
        rows = [dict(group=f"g{i}", label=int(i < 2)) for i in range(4)]
        with self.assertRaisesRegex(ValueError, "both labels"):
            crossfit_validation(rows, [0.1, 0.2, 0.8, 0.9])

    def test_quality_variants_retain_controls(self):
        base = trial_plan()
        quality = trial_plan(quality_search=True)
        self.assertEqual(quality[:2], base)
        self.assertEqual(len(quality), 4)
        self.assertEqual(quality[2]["pair_strategy"], "hardest")
        self.assertEqual(quality[3]["positive_weight"], 2.0)

    def test_optional_grid_and_budget(self):
        grid = dict(learning_rates=[3e-5], pair_weights=[0.2],
                    hard_negative_weights=[1, 3], positive_weights=[1, 2],
                    pair_strategies=["all", "hardest"])
        self.assertEqual(len(trial_plan(grid, max_trials=8)), 8)
        with self.assertRaisesRegex(ValueError, "budget"):
            trial_plan(grid, max_trials=7)
        with self.assertRaisesRegex(ValueError, "choose"):
            trial_plan(grid, quality_search=True)

    def test_rank_prefers_fpr_feasible_out_of_fold(self):
        weak = dict(AP=0.98, crossfit=dict(fpr=0.25, target_fpr=0.01,
                    cluster_recall95=dict(low=0.8), recall=0.9))
        good = dict(AP=0.7, crossfit=dict(fpr=0.01, target_fpr=0.01,
                    cluster_recall95=dict(low=0.1), recall=0.2))
        self.assertGreater(candidate_rank(good), candidate_rank(weak))


if __name__ == "__main__":
    unittest.main()
