"""Paired errors stay source/scenario aligned and keep frozen operating points."""
import unittest
import json
from pathlib import Path
import tempfile
from automl_protocol import leaderboard
from decision_protocol import confusion
from low_fpr_ranking import low_fpr_ranking
from v6_protocol import NAMES, spec_for, VERSION

from ablation_analysis import analyze, average_precision, paired_changes, digest


class AblationAnalysisTests(unittest.TestCase):
    def test_ap_perfect_tied_and_reversed(self):
        self.assertEqual(average_precision([0,1],[.1,.9]),1)
        self.assertEqual(average_precision([0,1],[.5,.5]),.5)
        self.assertEqual(average_precision([0,1],[.9,.1]),.5)
        self.assertIsNone(average_precision([0],[.1]))

    def test_identical_scores_zero_deltas_in_every_replicate(self):
        rows=[dict(source=s,group=str(y),label=y) for s in 'ab' for y in (0,1)]
        result=paired_changes(rows,[.1,.9]*2,[.1,.9]*2,.5,.5,replicates=100)
        self.assertEqual(result['recall_delta'],0)
        self.assertEqual(result['fpr_delta_cluster95']['high'],0)
        self.assertEqual(result['recall_delta_cluster95']['low'],0)
        self.assertEqual(result['scenario_units'],4)

    def test_explicit_positive_gain_and_false_positive_cost(self):
        rows=[dict(group='a',label=1),dict(group='a',label=0)]
        report=paired_changes(rows,[.1,.1],[.9,.9],.5,.5,replicates=20)
        self.assertEqual(report['transitions']['newly_detected_positive'],1)
        self.assertEqual(report['transitions']['new_false_positive'],1)
        self.assertEqual(report['recall_delta_cluster95']['low'],1)
        self.assertEqual(report['fpr_delta_cluster95']['low'],1)
        self.assertEqual(report['scenario_units'],1)

    def test_source_isolation_and_original_thresholds(self):
        rows=[dict(source='a',group='same',label=1),dict(source='b',group='same',label=0)]
        report=paired_changes(rows,[.8,.2],[.8,.2],.5,.9,replicates=100)
        self.assertEqual(report['transitions']['lost_positive'],1)
        self.assertEqual(report['scenario_units'],2)
        self.assertEqual(report['threshold'],.9)
        for count in (0,True,10001):
            with self.assertRaises(ValueError): paired_changes(rows,[.8,.2],[.8,.2],.5,.9,replicates=count)

    def test_archive_verification_end_to_end_and_tamper_rejection(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp); final=root/'final';final.mkdir()
            def save(path,value): path.write_text(json.dumps(value,sort_keys=True)+'\n')
            common=dict(source_commit='a'*40,request=dict(version=VERSION))
            validation=dict(AP=1.,low_fpr_ranking=low_fpr_ranking([0,1],[.1,.9]))
            trials=[];candidates={};metrics=confusion([0,1],[.1,.9],.5)
            rows=[dict(id=str(y),source='s',group=str(y),label=y,tags=['unknown'],score=.9 if y else .1) for y in (0,1)]
            for name in NAMES:
                (final/(name+'.safetensors')).write_bytes(name.encode())
                trial=dict(name=name,hyperparameters=spec_for(name),initial_state_sha256='b'*64,
                    selected_epoch=1,validation=validation,
                    epoch_history=[dict(epoch=i,validation=validation) for i in range(1,7)],
                    checkpoint_sha256=digest(final/(name+'.safetensors')))
                trials.append(trial)
                worker=root/name;worker.mkdir()
                save(worker/'worker.json',dict(common=common,trial=trial))
                save(worker/'encoding_audit.json',dict(train=dict(rows=2)))
                candidates[name]=dict(threshold=.5,selected_epoch=1,checkpoint_sha256=trial['checkpoint_sha256'])
                (final/(name+'_test_predictions.jsonl')).write_text(''.join(json.dumps(r)+'\n' for r in rows))
            ranking=leaderboard(trials)
            save(final/'freeze.json',dict(common=common,selected=ranking[0]['name'],candidates=candidates,
                leaderboard=ranking,initial_state_sha256='b'*64,mode='shadow_only',production_approved=False))
            save(final/'results.json',dict(source_commit='a'*40,selected=ranking[0]['name'],
                freeze_sha256=digest(final/'freeze.json'),
                candidates={n:dict(metrics=metrics,AP=1.) for n in NAMES}))
            report=analyze(root,'a'*40)
            self.assertTrue(report['all_regression_metrics_reproduced'])
            self.assertEqual(report['candidates'][NAMES[0]]['tag_coverage']['annotation_coverage'],0)
            checkpoint=final/(NAMES[0]+'.safetensors')
            original=checkpoint.read_bytes();checkpoint.write_bytes(b'changed')
            with self.assertRaisesRegex(ValueError,'checkpoint digest'): analyze(root,'a'*40)
            checkpoint.write_bytes(original)
            rows[0]['score']=.9
            (final/(NAMES[0]+'_test_predictions.jsonl')).write_text(''.join(json.dumps(r)+'\n' for r in rows))
            with self.assertRaisesRegex(ValueError,'metrics do not reproduce'): analyze(root,'a'*40)


if __name__=='__main__':
    unittest.main()
