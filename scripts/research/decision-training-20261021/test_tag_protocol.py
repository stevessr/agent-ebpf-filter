"""Tag, source-isolation and frozen-error diagnostics regression cases."""
import copy
import unittest

from automl_protocol import candidate_rank, research_gate, shadow_review_metrics
from decision_protocol import audit_splits, confusion, group_batches, scenario_key
from error_analysis import error_slices
from quality_protocol import group_folds
from tag_protocol import normalize_rows, normalize_tag, TAG_VERSION


def action(source, group, label, i=0, **extra):
    return dict(source=source, group=group, label=label, id=str(i),
                domain='privacy_action', split='independent_review',
                annotation_source='human_review', **extra)


class TagProtocolTests(unittest.TestCase):
    def test_format_aliases_share_one_canonical_tag(self):
        for raw in ('FILE_READ', ' file-read ', 'File Read', 'fileRead',
                    'ReadFile', 'ｆｉｌｅ＿ｒｅａｄ'):
            self.assertEqual(normalize_tag(raw), 'file_read')
        self.assertEqual(normalize_tag('HTTPRequest'), 'http_request')
        self.assertEqual(normalize_tag('NetworkRequest'), 'network')
        self.assertEqual(normalize_tag('文件读取'), 'file_read')
        self.assertEqual(normalize_tag('隐私泄露'), 'privacy_leak')

    def test_merge_sort_deduplicate_and_idempotence(self):
        raw = [dict(tags=['ReadFile', 'file-read', 'unknown'],
                    tag='Privacy Leakage', category='FILE_WRITE', label=0)]
        original = copy.deepcopy(raw)
        rows = normalize_rows(raw)
        self.assertEqual(rows[0]['tags'], ['file_read', 'file_write', 'privacy_leak'])
        self.assertEqual(rows[0]['tag_version'], TAG_VERSION)
        self.assertEqual(normalize_rows(rows), rows)
        self.assertEqual(raw, original)

    def test_unknown_tags_preserved_without_inventing_labels(self):
        raw = [dict(tags=['customAction', 'BLOCK'], label=0), dict(label=None), {}]
        rows = normalize_rows(raw)
        self.assertEqual(rows[0]['tags'], ['block', 'custom_action'])
        self.assertEqual(rows[0]['label'], 0)
        self.assertEqual(rows[1]['tags'], ['unknown'])
        self.assertIsNone(rows[1]['label'])
        self.assertNotIn('label', rows[2])

    def test_invalid_and_oversized_tags_fail(self):
        for raw in (None, 1, True, {}, ['a'] * 33, ['x' * 129],
                    ['file/read'], ['file\x00read'], ['a,b'], ['敏感']):
            with self.subTest(raw=raw), self.assertRaises(ValueError):
                normalize_rows([dict(tags=raw)])
        with self.assertRaisesRegex(ValueError, 'version'):
            normalize_rows([dict(tag_version='future.v2')])

    def test_metadata_normalization_does_not_rewrite_model_inputs(self):
        raw = dict(context='Ａ  B\n C', candidate='curl -X POST x',
                   source='Mixed-CASE', group='Group-A', id='Action-A',
                   domain='http_attack', label=1, category='FILE_READ')
        row = normalize_rows([raw])[0]
        for field in ('context', 'candidate', 'source', 'group', 'id', 'domain', 'label'):
            self.assertEqual(row[field], raw[field])


class SourceAndQualityTests(unittest.TestCase):
    def test_same_local_group_and_id_are_separate_sources(self):
        rows = [action('a', 'same', 0), action('a', 'same', 1, 1),
                action('b', 'same', 0), action('b', 'same', 1, 1)]
        stats = shadow_review_metrics(rows, [0.9, 0.8, 0.1, 0.8], 0.5)
        self.assertEqual(stats['negative_scenarios'], 2)
        self.assertEqual(stats['false_positive_scenarios'], 1)
        self.assertEqual(stats['source_count'], 2)
        self.assertEqual([s['fp'] for s in stats['slices']['by_source']], [1, 0])

    def test_source_local_duplicate_id_is_still_rejected(self):
        rows = [action('a', 'one', 0), action('a', 'two', 1)]
        with self.assertRaisesRegex(ValueError, 'duplicate review ID'):
            shadow_review_metrics(rows, [0.1, 0.8], 0.5)

    def test_source_scoped_batching_and_folds(self):
        rows = [action(source, 'same', y, y) for source in 'abcd' for y in (0, 1)]
        batches = group_batches(rows, 2, 5)
        self.assertEqual(len(batches), 4)
        for batch in batches:
            self.assertEqual(len({scenario_key(rows[i]) for i in batch}), 1)
        folds = group_folds(rows)
        self.assertFalse({scenario_key(rows[i]) for i in folds[0]} &
                         {scenario_key(rows[i]) for i in folds[1]})

    def test_leakage_audit_rejects_normalized_duplicates(self):
        splits = {}
        for source, split in zip('abc', ('train', 'validation', 'calibration')):
            splits[split] = [dict(action(source, 'same', y, y),
                                 context=source, candidate='act' + str(y)) for y in (0, 1)]
        self.assertEqual(audit_splits(splits)['train']['groups'], 1)
        splits['train'][0].update(context='Ａ\t B', candidate='read   x')
        splits['validation'][0].update(context='A B', candidate='read x')
        with self.assertRaisesRegex(ValueError, 'duplicate context/action'):
            audit_splits(splits)

    def test_conflicting_normalized_payload_and_source_id_rejected(self):
        splits = {s: [dict(group=s, id=f'{s}-{y}', label=y,
                          context=s, candidate=str(y)) for y in (0, 1)]
                  for s in ('train', 'validation', 'calibration')}
        splits['train'].append(dict(splits['train'][0], label=1))
        with self.assertRaisesRegex(ValueError, 'conflicting source action ID'):
            audit_splits(splits)
        splits['train'][-1]['id'] = 'other'
        with self.assertRaisesRegex(ValueError, 'conflicting context/action labels'):
            audit_splits(splits)

    def test_slice_reports_exact_errors_and_missing_classes(self):
        rows = [action('a', 'one', 0, tags=['FileRead', 'read-file']),
                action('a', 'one', 1, 1, tags=['file-read']),
                action('b', 'two', 1, 2, tags=['FILE_WRITE'])]
        report = error_slices(rows, [0.8, 0.5, 0.9], 0.5)
        read, write = report['by_tag']
        self.assertEqual((read['key'], read['rows'], read['fp'], read['fn']),
                         ('file_read', 2, 1, 1))
        self.assertEqual((write['tp'], write['fpr']), (1, None))
        self.assertEqual(report['overall']['tp'], 1)
        self.assertTrue(report['overlapping_tag_slices'])

    def test_nan_threshold_and_ambiguous_numeric_values_rejected(self):
        for threshold in (float('nan'), float('inf'), True, '0.5'):
            with self.assertRaises(ValueError):
                confusion([0, 1], [0.1, 0.9], threshold)
        for label, score in ((True, 0.1), (1.0, 0.1), (0, '0.1'), (0, True)):
            with self.assertRaises(ValueError):
                confusion([label], [score], 0.5)

    def test_failed_candidates_rank_by_fpr_before_recall(self):
        def metrics(fpr, recall):
            return dict(AP=0.8, crossfit=dict(fpr=fpr, target_fpr=0.01,
                        recall=recall, cluster_recall95=dict(low=recall)))
        self.assertGreater(candidate_rank(metrics(0.02, 0.2)),
                           candidate_rank(metrics(0.20, 0.9)))

    def test_one_bad_fold_is_not_hidden_by_aggregate(self):
        good = dict(AP=0.7, crossfit=dict(fpr=0.01, target_fpr=0.01,
                    recall=0.2, cluster_recall95=dict(low=0.1)))
        bad = copy.deepcopy(good)
        bad['crossfit'].update(recall=0.9, folds=[dict(metrics=dict(fpr=0.02))])
        self.assertGreater(candidate_rank(good), candidate_rank(bad))

    def test_bad_source_fails_even_when_aggregate_passes(self):
        review = dict(fpr_upper95=0.001, scenario_fpr_upper95=0.001, recall=0.9,
                      slices=dict(by_source=[dict(fpr_upper95=0.02,
                                                  scenario_fpr_upper95=0.02)]))
        gate = research_gate(dict(fpr_upper95=0.001), review)
        self.assertFalse(gate['shadow_review_candidate'])
        self.assertIn('source FPR', ' '.join(gate['blockers']))

    def test_nonfinite_review_metrics_cannot_pass(self):
        for field in ('fpr_upper95', 'scenario_fpr_upper95', 'recall'):
            review = dict(fpr_upper95=0.001, scenario_fpr_upper95=0.001, recall=0.9)
            review[field] = float('nan')
            with self.assertRaises(ValueError):
                research_gate(dict(fpr_upper95=0.001), review)


if __name__ == '__main__':
    unittest.main()
