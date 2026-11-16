"""Predeclared equal-budget 2x2 ablation; no test-driven search."""
VERSION = 'privacy_low_fpr_ablation.v6'
SEED, EPOCHS, BATCH_SIZE = 20261021, 6, 16
NAMES = ('v6_legacy_pair', 'v6_evidence_pair', 'v6_legacy_tail', 'v6_evidence_tail')


def spec_for(name):
    if name not in NAMES:
        raise ValueError('unknown v6 candidate')
    return dict(name=name, learning_rate=3e-5, pair_weight=0.2,
                hard_negative_weight=1.0, positive_weight=1.0, pair_strategy='all',
                context_view='evidence_pair.v1' if 'evidence' in name else 'legacy',
                tail_weight=0.2 if name.endswith('tail') else 0.0)


def validate_workers(workers):
    if len(workers) != 4 or {w['trial']['name'] for w in workers} != set(NAMES):
        raise ValueError('exactly four distinct predeclared candidates required')
    common = workers[0]['common']
    initial = workers[0]['trial']['initial_state_sha256']
    if not isinstance(initial, str) or len(initial) != 64:
        raise ValueError('missing initial model digest')
    for worker in workers:
        trial = worker['trial']
        if worker['common'] != common or trial['initial_state_sha256'] != initial:
            raise ValueError('workers differ in source, data, budget, or initialization')
        if trial['hyperparameters'] != spec_for(trial['name']):
            raise ValueError('candidate hyperparameters differ from predeclared request')
        history = trial['epoch_history']
        if [h['epoch'] for h in history] != list(range(1, EPOCHS + 1)):
            raise ValueError('incomplete training budget')
        if trial['selected_epoch'] not in range(1, EPOCHS + 1):
            raise ValueError('invalid selected epoch')
        if trial['validation'] != history[trial['selected_epoch'] - 1]['validation']:
            raise ValueError('selected metrics do not match selected epoch')
    return common
