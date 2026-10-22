"""Tag coverage accounting cannot silently turn unknown metadata into labels."""
import copy
import json
from pathlib import Path
import tempfile
from automl_review import review
import unittest

from error_analysis import error_slices
from tag_protocol import ALIASES, KNOWN_TAGS, normalize_tag, normalize_with_audit, normalize_rows
from freeze_contract import selected_freeze
from v6_protocol import VERSION, NAMES


class TagAuditTests(unittest.TestCase):
    def test_alias_dedup_unknown_suppression_and_coverage(self):
        raw = [dict(tags=['fileRead','read-file','unknown'],tag='customAction',label=0),
               dict(label=1), dict(category='未知',label=0)]
        original = copy.deepcopy(raw)
        rows, report = normalize_with_audit(raw)
        self.assertEqual(raw, original)
        self.assertEqual(rows[0]['tags'], ['custom_action','file_read'])
        self.assertEqual((report['input_values'], report['duplicate_values']), (5,1))
        self.assertEqual(report['explicit_unknown_values'],2)
        self.assertEqual(report['suppressed_unknown_values'],1)
        self.assertEqual(report['unknown_only_rows'],2)
        self.assertEqual(report['implicit_unknown_rows'],1)
        self.assertEqual(report['known_taxonomy_rows'],1)
        self.assertEqual(report['custom_tag_rows'],1)
        self.assertAlmostEqual(report['annotation_coverage'],1/3)
        self.assertEqual(normalize_rows(rows), rows)

    def test_empty_separators_are_consistently_unknown(self):
        for raw in ('', ' ', '-', '_', '___', '--', ' _- _ ', '\u3000＿－'):
            self.assertEqual(normalize_tag(raw),'unknown')

    def test_known_aliases_and_taxonomy_are_closed_and_idempotent(self):
        for raw, canonical in ALIASES.items():
            self.assertEqual(normalize_tag(raw), canonical)
            self.assertEqual(normalize_tag(canonical), canonical)
            self.assertIn(canonical,KNOWN_TAGS)
        for canonical in KNOWN_TAGS:
            self.assertEqual(normalize_tag(canonical),canonical)

    def test_limits_apply_after_nfkc_and_merged_inputs(self):
        self.assertEqual(normalize_tag('a'*64),'a'*64)
        for raw in ('a'*65, 'ﬃ'*22, 'file\u200bread', '\u202efile_read', 'file/read'):
            with self.assertRaises(ValueError): normalize_tag(raw)
        with self.assertRaises(ValueError):
            normalize_rows([dict(tags=['a']*16,tag=['a']*16,category='a')])
        self.assertEqual(normalize_rows([dict(tags=['a']*32)])[0]['tags'],['a'])

    def test_slices_expose_unknown_coverage_without_changing_metrics(self):
        rows=[dict(group='a',label=0),dict(group='b',label=1,tags=['FILE_READ'])]
        report=error_slices(rows,[.1,.9],.5)
        self.assertEqual(report['tag_coverage']['annotation_coverage'],.5)
        self.assertEqual(report['overall']['tp'],1)
        self.assertEqual(report['by_source'][0]['tag_coverage']['unknown_only_rows'],1)
        self.assertEqual(report['by_tag'][1]['key'],'unknown')
        self.assertEqual(report['by_tag'][1]['tag_coverage']['annotation_coverage'],0)

    def test_missing_and_explicit_unknown_are_distinct(self):
        rows, audit=normalize_with_audit([{},dict(tags=[]),dict(tags='UNKNOWN')])
        self.assertEqual(audit['implicit_unknown_rows'],2)
        self.assertEqual(audit['explicit_unknown_values'],1)
        self.assertEqual(audit['annotation_coverage'],0)
        self.assertIsNone(normalize_with_audit([])[1]['annotation_coverage'])


class FreezeAdapterTests(unittest.TestCase):
    def freeze(self):
        return dict(common=dict(request=dict(version=VERSION)), selected=NAMES[1],
            candidates={n:dict(threshold=.7,checkpoint_sha256='a'*64,
                               calibration=dict(fpr_upper95=.03)) for n in NAMES},
            mode='shadow_only',production_approved=False)

    def test_v6_selected_checkpoint_maps_to_existing_review_contract(self):
        selected=selected_freeze(self.freeze())
        self.assertEqual(selected['artifact_sha256'],'a'*64)
        self.assertEqual(selected['threshold'],.7)
        self.assertEqual(selected['selected'],NAMES[1])
        self.assertEqual(selected['empirical_calibration']['fpr_upper95'],.03)
        self.assertEqual(selected_freeze(selected),selected)

    def test_v6_freeze_runs_existing_review_with_fixed_selected_threshold(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            frozen=self.freeze()
            (root/'freeze.json').write_text(json.dumps(frozen))
            examples=[dict(id=str(y),group=str(y),source='new',domain='privacy_action',
                split='independent_review',annotation_source='test_fixture',label=y,
                artifact_sha256='a'*64,score=.8 if y else .2,tag='FileRead') for y in (0,1)]
            (root/'predictions.jsonl').write_text(''.join(json.dumps(r)+'\n' for r in examples))
            result=review(root/'freeze.json',root/'predictions.jsonl',root/'review.json')
            self.assertEqual(result['metrics']['tp'],1)
            self.assertEqual(result['metrics']['slices']['threshold'],.7)
            self.assertEqual(result['metrics']['slices']['tag_coverage']['annotation_coverage'],1)
            self.assertFalse(result['gate']['shadow_review_candidate'])
            self.assertFalse(result['gate']['production_approved'])

    def test_ambiguous_version_selection_and_threshold_fail(self):
        for change in (dict(selected='other'), dict(production_approved=True),
                       dict(common=dict(request=dict(version='future'))),dict(common=None),dict(candidates={})):
            with self.assertRaises(ValueError): selected_freeze(dict(self.freeze(),**change))
        frozen=self.freeze();frozen['candidates'][NAMES[1]]['threshold']=float('nan')
        with self.assertRaises(ValueError): selected_freeze(frozen)


if __name__=='__main__':
    unittest.main()
