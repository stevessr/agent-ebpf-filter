"""Stdlib checks for bounded AutoML search and refusal to auto-promote."""
import unittest

from automl_protocol import (candidate_rank, leaderboard, research_gate,
                             shadow_review_metrics, trial_plan)


def action(group, label, i, **extra):
    return dict(id=str(i), group=group, label=label, domain="privacy_action",
                split="independent_review", source="new_confirmed_set",
                annotation_source="manual_double_review", **extra)


class AutoMLProtocolTests(unittest.TestCase):
    def test_default_trials_remain_same_budget(self):
        trials = trial_plan()
        self.assertEqual([t["name"] for t in trials],
                         ["mini_pair_baseline", "mini_hard_negative"])
        self.assertEqual([t["hard_negative_weight"] for t in trials], [1.0, 3.0])

    def test_deterministic_grid_and_explicit_budget(self):
        grid = dict(learning_rates=[3e-5], pair_weights=[0, 0.2],
                    hard_negative_weights=[1, 3])
        a = trial_plan(grid)
        self.assertEqual(a, trial_plan(grid))
        self.assertEqual(len(a), 4)
        with self.assertRaisesRegex(ValueError, "budget"):
            trial_plan(grid, max_trials=3)

    def test_search_bad_axes_and_nan(self):
        grid = dict(learning_rates=[3e-5], pair_weights=[0.2],
                    hard_negative_weights=[float("nan")])
        with self.assertRaisesRegex(ValueError, "invalid"):
            trial_plan(grid)
        grid["hard_negative_weights"] = [0.5]
        with self.assertRaisesRegex(ValueError, "range"):
            trial_plan(grid)
        grid["hard_negative_weights"] = [3.0, 3]
        with self.assertRaisesRegex(ValueError, "duplicate"):
            trial_plan(grid)

    def test_validation_only_selection_and_fixed_ties(self):
        def candidate(name, ap):
            return dict(name=name, checkpoint_sha256="a" * 64,
                        hyperparameters={}, calibration={"recall": 1},
                        validation=dict(cluster_recall95=dict(low=0.1),
                                        recall=0.3, AP=ap))
        ranks = leaderboard([candidate("z", 0.8), candidate("a", 0.8),
                             candidate("m", 0.9)])
        self.assertEqual([t["name"] for t in ranks], ["m", "a", "z"])
        self.assertEqual(candidate_rank(candidate("x", 0.7)["validation"]),
                         (0.1, 0.3, 0.7))

    def test_review_rejects_wrong_domain_and_partition(self):
        rows = [action("a", 0, 0), action("b", 1, 1)]
        rows[0]["domain"] = "http_attack"
        with self.assertRaisesRegex(ValueError, "cross-domain"):
            shadow_review_metrics(rows, [0.1, 0.9], 0.5)
        rows[0]["domain"] = "privacy_action"
        rows[1]["split"] = "consumed_test"
        with self.assertRaisesRegex(ValueError, "independent"):
            shadow_review_metrics(rows, [0.1, 0.9], 0.5)

    def test_review_uses_negative_scenarios_not_rows(self):
        rows = [action("normal_a", 0, 0), action("normal_a", 0, 1),
                action("normal_b", 0, 2), action("risk", 1, 3)]
        stats = shadow_review_metrics(rows, [0.99, 0.1, 0.2, 0.8], 0.5)
        self.assertEqual(stats["fp"], 1)
        self.assertEqual(stats["negative_scenarios"], 2)
        self.assertEqual(stats["false_positive_scenarios"], 1)
        self.assertGreater(stats["scenario_fpr_upper95"], stats["fpr_upper95"])

    def test_no_implicit_promotion_from_old_test_or_calibration(self):
        result = research_gate(dict(fpr_upper95=0.001))
        self.assertFalse(result["shadow_review_candidate"])
        self.assertFalse(result["production_approved"])
        self.assertIn("independent", " ".join(result["blockers"]))
        result = research_gate(dict(fpr_upper95=0.03))
        self.assertIn("calibration", " ".join(result["blockers"]))

    def test_review_gate_is_still_shadow_only(self):
        # Synthetic proof that satisfying all *statistical* gates still does
        # not authorize production activation or prove source provenance.
        rows = [action('normal_' + str(i), 0, i) for i in range(400)]
        rows += [action('risk_' + str(i), 1, 400+i) for i in range(30)]
        review = shadow_review_metrics(rows, [0.1]*400 + [0.9]*30, 0.5)
        gate = research_gate(dict(fpr_upper95=0.008), review)
        self.assertTrue(gate["shadow_review_candidate"])
        self.assertFalse(gate["production_approved"])
        self.assertTrue(gate["requires_human_provenance_review"])


if __name__ == "__main__":
    unittest.main()

