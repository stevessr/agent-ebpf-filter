"""Fast CPU/stdlib regression for data isolation and low-FPR operating points."""
import math
import unittest
from decision_protocol import (audit_splits, confusion, group_batches,
                               group_bootstrap_recall, operating_threshold,
                               upper_binomial95)


def row(group, label, *, action=None):
    return dict(group=group, label=label, context='ctx:' + group,
                candidate=action or ('act:' + str(label)))


class DecisionProtocolTests(unittest.TestCase):
    def setUp(self):
        self.splits = {
            'train': [row('t', 0), row('t', 1)],
            'validation': [row('v', 0), row('v', 1)],
            'calibration': [row('c', 0), row('c', 1)],
            'test': [row('h', 0), row('h', 1)]}

    def test_three_way_preflight_without_opening_test(self):
        self.assertEqual(audit_splits({k: v for k, v in self.splits.items() if k != 'test'})['validation']['groups'], 1)

    def test_strict_scenario_split(self):
        self.assertEqual(audit_splits(self.splits)['train']['groups'], 1)
        self.splits['test'][0]['group'] = 'train_invalid'
        self.splits['test'][1]['group'] = 't'
        with self.assertRaisesRegex(ValueError, 'scenario leakage'):
            audit_splits(self.splits)

    def test_reused_source_id_with_modified_text_leaks(self):
        self.splits['train'][0]['id'] = 'action-123'
        self.splits['test'][0]['id'] = 'action-123'
        self.splits['test'][0]['candidate'] = 'different action'
        with self.assertRaisesRegex(ValueError, 'source action ID leakage'):
            audit_splits(self.splits)

    def test_duplicate_candidate_independent_of_group(self):
        duplicate = dict(self.splits['train'][0], group='other')
        self.splits['test'][0] = duplicate
        with self.assertRaisesRegex(ValueError, 'duplicate context/action'):
            audit_splits(self.splits)

    def test_reject_unknown_and_ambiguous_labels(self):
        self.splits['train'][0]['label'] = None
        with self.assertRaisesRegex(ValueError, 'binary label'):
            audit_splits(self.splits)

    def test_batch_complete_pair_and_deterministic(self):
        rows = [row(f'g{g}', label) for g in range(11) for label in (0, 1)]
        batches = group_batches(rows, 5, 123)
        self.assertEqual(batches, group_batches(rows, 5, 123))
        self.assertEqual(sorted(i for batch in batches for i in batch), list(range(22)))
        self.assertTrue(all(len(batch) <= 5 and len(batch) > 0 for batch in batches))
        for group in range(11):
            self.assertTrue(any(2*group in b and 2*group+1 in b for b in batches))

    def test_oversized_group_fails_instead_of_splitting_pairs(self):
        with self.assertRaisesRegex(ValueError, 'exceeds batch size'):
            group_batches([row('g', x % 2) for x in range(6)], 4, 1)

    def test_empirical_fpr_budget_and_ties(self):
        y = [0] * 100 + [1] * 5
        p = [0.99, 0.98, 0.98] + [0.1] * 97 + [0.97] * 5
        op = operating_threshold(y, p)
        self.assertEqual(op['fp_budget'], 1)
        self.assertEqual(op['threshold'], 0.98)
        m = confusion(y, p, op['threshold'])
        self.assertEqual(m['fp'], 1)
        self.assertEqual(m['tp'], 0)

    def test_few_negatives_allow_no_false_positives(self):
        y = [0]*99 + [1]
        scores = [0.2]*99 + [0.9]
        op = operating_threshold(y, scores)
        self.assertEqual(op['fp_budget'], 0)
        self.assertEqual(confusion(y, scores, op['threshold'])['tp'], 1)
        self.assertGreater(upper_binomial95(0, 99), 0.029)
        self.assertLess(upper_binomial95(0, 99), 0.031)

    def test_invalid_and_nonfinite_calibration(self):
        for score in (float('nan'), float('inf')):
            with self.assertRaisesRegex(ValueError, 'non-finite'):
                operating_threshold([0, 1], [score, 0.5])
        with self.assertRaisesRegex(ValueError, 'no calibration negatives'):
            operating_threshold([1], [0.5])

    def test_calibration_rejects_ambiguous_numeric_values(self):
        for score in (True, False, "0.5", None, 10**1000):
            with self.subTest(score=str(score)[:30]), self.assertRaisesRegex(ValueError, 'scores'):
                operating_threshold([0, 1], [score, 0.9])
        for target in (True, False, "0.01", float("nan"), 10**1000):
            with self.subTest(target=str(target)[:30]), self.assertRaisesRegex(ValueError, 'target_fpr'):
                operating_threshold([0, 1], [0.1, 0.9], target)
        with self.assertRaisesRegex(ValueError, 'threshold'):
            confusion([0, 1], [0.1, 0.9], 10**1000)
        with self.assertRaisesRegex(ValueError, 'evaluation row'):
            confusion([0, 1], [10**1000, 0.9], 0.5)

    def test_bootstrap_rejects_unbounded_and_ambiguous_scores(self):
        rows = [row("a", 0), row("b", 1)]
        for scores in ((True, 0.9), ("0.1", 0.9), (10**1000, 0.9)):
            with self.assertRaisesRegex(ValueError, 'bootstrap row'):
                group_bootstrap_recall(rows, scores, 0.5)
        for replicates in (True, False, 0, 10001):
            with self.assertRaisesRegex(ValueError, 'bootstrap arguments'):
                group_bootstrap_recall(rows, (0.1, 0.9), 0.5, replicates=replicates)

    def test_accuracy_does_not_hide_class_imbalance(self):
        # Classifier abstaining on every positive may have high accuracy but
        # must report zero recall and zero F1.
        metrics = confusion([0]*9 + [1], [0.1]*10, 0.5)
        self.assertAlmostEqual(metrics['accuracy'], 0.9)
        self.assertEqual(metrics['precision'], 0.0)
        self.assertEqual(metrics['recall'], 0.0)
        self.assertEqual(metrics['balanced_accuracy'], 0.5)
        self.assertEqual(metrics['f1'], 0.0)
        self.assertEqual(metrics['specificity'], 1.0)

    def test_quality_metrics_for_partial_detection(self):
        metrics = confusion([0, 0, 1, 1], [0.9, 0.1, 0.7, 0.2], 0.5)
        self.assertAlmostEqual(metrics['accuracy'], 0.5)
        self.assertAlmostEqual(metrics['precision'], 0.5)
        self.assertAlmostEqual(metrics['recall'], 0.5)
        self.assertAlmostEqual(metrics['f1'], 0.5)
        self.assertAlmostEqual(metrics['balanced_accuracy'], 0.5)

    def test_binomial_upper_bound_monotonic(self):
        self.assertAlmostEqual(upper_binomial95(0, 100), 1-math.pow(0.05, 0.01))
        self.assertGreater(upper_binomial95(2, 100), upper_binomial95(1, 100))

    def test_cluster_bootstrap_reproducible(self):
        rows = [row('g1', 1), row('g1', 0), row('g2', 1), row('g2', 0)]
        scores = [0.8, 0.1, 0.2, 0.9]
        a = group_bootstrap_recall(rows, scores, 0.5, replicates=200, seed=5)
        self.assertEqual(a, group_bootstrap_recall(rows, scores, 0.5, replicates=200, seed=5))
        self.assertLessEqual(a['low'], a['high'])


if __name__ == '__main__':
    unittest.main()
