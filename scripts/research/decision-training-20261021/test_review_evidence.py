"""Insufficient and source-shifted positives cannot qualify a frozen model."""
import math
import unittest

from automl_protocol import research_gate, shadow_review_metrics
from decision_protocol import confusion, lower_binomial95, upper_binomial95


def reviewed(source, group, label, identity):
    return dict(source=source, group=group, label=label, id=str(identity),
                domain='privacy_action', split='independent_review',
                annotation_source='human')


def negatives(source, n=400):
    return [reviewed(source, f'negative{i}', 0, f'n{i}') for i in range(n)]


class ReviewEvidenceTests(unittest.TestCase):
    def test_exact_recall_bounds_and_symmetry(self):
        self.assertAlmostEqual(lower_binomial95(1, 1), 0.05)
        self.assertEqual(lower_binomial95(0, 20), 0.0)
        self.assertAlmostEqual(lower_binomial95(20, 20), 0.05 ** (1/20))
        self.assertAlmostEqual(lower_binomial95(7, 20), 1-upper_binomial95(13, 20))
        self.assertLess(lower_binomial95(7, 20), lower_binomial95(8, 20))
        self.assertEqual(confusion([0], [0.1], 0.5)['recall_lower95'], None)

    def test_binomial_counts_reject_bool_fraction_and_empty(self):
        for numerator, denominator in ((True, 10), (1.0, 10), (1, 10.0), (0, 0), (-1, 10), (11, 10)):
            for fn in (lower_binomial95, upper_binomial95):
                with self.subTest(fn=fn, counts=(numerator, denominator)), self.assertRaises(ValueError):
                    fn(numerator, denominator)

    def test_single_detected_positive_is_insufficient(self):
        rows = negatives('a') + [reviewed('a', 'one_risk', 1, 'p')]
        result = shadow_review_metrics(rows, [0.1]*400 + [0.9], 0.5)
        self.assertEqual(result['recall'], 1)
        self.assertLess(result['fpr_upper95'], 0.01)
        self.assertAlmostEqual(result['recall_lower95'], 0.05)
        gate = research_gate(dict(fpr_upper95=0.001), result)
        self.assertFalse(gate['shadow_review_candidate'])
        self.assertIn('recall lower bound', ' '.join(gate['blockers']))

    def test_repeated_candidates_do_not_inflate_scenario_recall(self):
        rows = negatives('a') + [reviewed('a', 'one_risk', 1, f'p{i}') for i in range(100)]
        result = shadow_review_metrics(rows, [0.1]*400 + [0.9]*100, 0.5)
        self.assertGreater(result['recall_lower95'], 0.9)
        self.assertEqual(result['positive_scenarios'], 1)
        self.assertAlmostEqual(result['scenario_recall_lower95'], 0.05)
        self.assertFalse(research_gate(dict(fpr_upper95=0.001), result)['shadow_review_candidate'])

    def test_one_missed_action_marks_positive_scenario_incomplete(self):
        rows = negatives('a') + [reviewed('a', 'risk', 1, 'p1'), reviewed('a', 'risk', 1, 'p2')]
        result = shadow_review_metrics(rows, [0.1]*400 + [0.9, 0.1], 0.5)
        self.assertEqual(result['recall'], 0.5)
        self.assertEqual(result['fully_detected_positive_scenarios'], 0)
        self.assertEqual(result['scenario_recall_lower95'], 0)

    def test_missed_source_is_not_hidden_in_good_aggregate(self):
        rows = negatives('a') + negatives('b')
        rows += [reviewed(source, f'p{i}', 1, f'p{i}') for source in ('a', 'b') for i in range(40)]
        result = shadow_review_metrics(rows, [0.1]*800 + [0.9]*40 + [0.1]*40, 0.5)
        self.assertGreater(result['recall_lower95'], 0.2)
        self.assertGreater(result['scenario_recall_lower95'], 0.2)
        self.assertTrue(all(s['fpr_upper95'] < 0.01 for s in result['slices']['by_source']))
        gate = research_gate(dict(fpr_upper95=0.001), result)
        self.assertFalse(gate['shadow_review_candidate'])
        self.assertIn('source recall', ' '.join(gate['blockers']))

    def test_missing_positive_source_and_legacy_metrics_cannot_qualify(self):
        old = dict(fpr_upper95=0.001, scenario_fpr_upper95=0.001, recall=1.0)
        gate = research_gate(dict(fpr_upper95=0.001), old)
        self.assertFalse(gate['shadow_review_candidate'])
        self.assertIn('source-level', ' '.join(gate['blockers']))
        rows = negatives('a') + negatives('b') + [reviewed('a', f'p{i}', 1, f'p{i}') for i in range(40)]
        result = shadow_review_metrics(rows, [0.1]*800 + [0.9]*40, 0.5)
        self.assertIsNone(result['slices']['by_source'][1]['recall_lower95'])
        self.assertFalse(research_gate(dict(fpr_upper95=0.001), result)['shadow_review_candidate'])

    def test_tag_slice_has_scenario_support_and_recall_bounds(self):
        rows = [dict(reviewed('a', f'group{i}', 1, i), tag='ReadFile') for i in range(3)]
        rows += [dict(reviewed('a', 'normal', 0, 'n'), category='FILE_READ')]
        result = shadow_review_metrics(rows, [0.9, 0.9, 0.1, 0.1], 0.5)
        tag = result['slices']['by_tag'][0]
        self.assertEqual(tag['key'], 'file_read')
        self.assertEqual(tag['positive_scenarios'], 3)
        self.assertEqual(tag['fully_detected_positive_scenarios'], 2)
        self.assertGreater(tag['recall_lower95'], 0)
        self.assertEqual(tag['recall_lower95'], tag['scenario_recall_lower95'])


if __name__ == '__main__':
    unittest.main()
