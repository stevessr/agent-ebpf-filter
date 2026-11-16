"""Finite-sample selection, label-free token planning and worker isolation."""
import copy
import re
import unittest

from automl_protocol import candidate_rank
from evidence_encoding import best_window, head_tail, plan_pair
from low_fpr_ranking import low_fpr_ranking
from v6_protocol import NAMES, spec_for, validate_workers


class Tokenizer:
    cls_token_id, sep_token_id, pad_token_id = 101, 102, 0
    all_special_ids = [0, 100, 101, 102, 103]
    def __init__(self):
        self.words = {}
    def encode(self, text, add_special_tokens=False):
        return [self.words.setdefault(word, 200 + len(self.words))
                for word in re.findall(r'\w+|[^\w\s]', text.lower())]


class LowFprRankingTests(unittest.TestCase):
    def test_perfect_and_tied_scores(self):
        self.assertEqual(low_fpr_ranking([0, 1, 0, 1], [.1, .9, .2, .8])['standardized_partial_auc'], 1)
        self.assertAlmostEqual(low_fpr_ranking([0, 1, 0, 1], [.5] * 4)['standardized_partial_auc'], .5)
        self.assertLess(low_fpr_ranking([0, 1], [.9, .1])['standardized_partial_auc'], .5)

    def test_ties_permutation_and_monotonic_invariance(self):
        y, s = [0, 1, 0, 1, 1], [.8, .8, .1, .3, .9]
        value = low_fpr_ranking(y, s)['standardized_partial_auc']
        self.assertAlmostEqual(value, low_fpr_ranking(y[::-1], s[::-1])['standardized_partial_auc'])
        self.assertAlmostEqual(value, low_fpr_ranking(y, [x*x for x in s])['standardized_partial_auc'])

    def test_disjoint_small_fold_maxima_cannot_both_pass_one_percent(self):
        # Even with perfect positive separation, one negative maximum exceeds
        # the opposite calibration-fold maximum. This is sample resolution.
        a, b = [i / 1000 for i in range(51)], [i / 1000 for i in range(51, 108)]
        self.assertGreater(max(sum(x > max(b) for x in a) / len(a),
                               sum(x > max(a) for x in b) / len(b)), .01)
        good = dict(AP=1, low_fpr_ranking=low_fpr_ranking([0]*108+[1]*145, a+b+[.9]*145))
        poor = dict(AP=.8, low_fpr_ranking=low_fpr_ranking([0, 1], [.9, .1]))
        self.assertGreater(candidate_rank(good), candidate_rank(poor))

    def test_invalid_inputs(self):
        for target in (0, 1, float('nan'), True):
            with self.assertRaises(ValueError): low_fpr_ranking([0, 1], [.1, .9], target)
        for labels, scores in (([1], [.5]), ([0, 1], [.5, float('nan')]), ([0, 2], [.1, .9])):
            with self.assertRaises(ValueError): low_fpr_ranking(labels, scores)


class EvidenceEncodingTests(unittest.TestCase):
    def test_late_memory_survives_long_goal(self):
        tok = Tokenizer()
        row = dict(context='User request: ' + 'goal '*300 + '\nUser memories: ' +
                   'background '*500 + 'secret9999 address42.\nPrior tool trajectory: completed.',
                   candidate='send secret9999 address42')
        result, _ = plan_pair(row, tok)
        context = [x for x,t,m in zip(result['input_ids'],result['token_type_ids'],result['attention_mask']) if t == 0 and m]
        self.assertIn(tok.encode('secret9999')[0], context)
        self.assertEqual(len(result['input_ids']), 384)
        self.assertEqual(len(result['token_type_ids']), 384)
        self.assertEqual(len(result['attention_mask']), 384)

    def test_only_context_action_affect_encoding(self):
        tok = Tokenizer()
        row = dict(context='User request: reply\nUser memories: card123 kept private.', candidate='send card123')
        first, _ = plan_pair(row, tok)
        changed = dict(row, label=1, tags=['privacy_leak'], id='test', group='other', source='elsewhere')
        self.assertEqual(first, plan_pair(changed, tok)[0])

    def test_long_action_middle_match_survives_in_original_order(self):
        tok = Tokenizer()
        words = ['word'+str(i) for i in range(600)]
        row = dict(context='User request: read word300', candidate=' '.join(words))
        result, _ = plan_pair(row, tok)
        action = [x for x,t,m in zip(result['input_ids'],result['token_type_ids'],result['attention_mask']) if t == 1 and m and x != 102]
        self.assertEqual(len(action), 253)
        self.assertIn(tok.encode('word300')[0], action)
        original = tok.encode(row['candidate'])
        positions = [original.index(x) for x in action]
        self.assertEqual(positions, sorted(set(positions)))

    def test_windows_keep_earliest_tie_and_subject(self):
        self.assertEqual(best_window([1,2,3,4], 2, {})[1], 0)
        self.assertEqual(best_window([1,2,3,4], 2, {4:1})[0], [3,4])
        self.assertEqual(head_tail(list(range(10)),4), [0,1,8,9])

    def test_empty_and_unstructured_unicode_context(self):
        for text in ('', '普通上下文。', 'User request: x\nUser memories: '):
            result, _ = plan_pair(dict(context=text,candidate=''), Tokenizer())
            self.assertEqual(len(result['input_ids']), 384)
            self.assertTrue(set(result['attention_mask']) <= {0,1})


class WorkerContractTests(unittest.TestCase):
    def records(self):
        return [dict(common=dict(source='a'), trial=dict(name=n, initial_state_sha256='a'*64,
            hyperparameters=spec_for(n), selected_epoch=2, validation=dict(AP=.7),
            epoch_history=[dict(epoch=i, validation=dict(AP=.7)) for i in range(1,7)])) for n in NAMES]

    def test_budget_initialization_and_provenance_must_match(self):
        records = self.records()
        self.assertEqual(validate_workers(records), dict(source='a'))
        for field, value in [('common',dict(source='b')), ('initial_state_sha256','b'*64),
                             ('epoch_history',[]), ('hyperparameters',{})]:
            broken = copy.deepcopy(records)
            if field == 'common': broken[0][field] = value
            else: broken[0]['trial'][field] = value
            with self.assertRaises(ValueError): validate_workers(broken)
        with self.assertRaises(ValueError): validate_workers(records[:-1])
        with self.assertRaises(ValueError): validate_workers([records[0]]*4)


if __name__ == '__main__':
    unittest.main()
