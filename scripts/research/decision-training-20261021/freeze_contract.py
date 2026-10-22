"""Resolve legacy or v6 selected checkpoints without retuning any threshold."""
from decision_protocol import confusion
from v6_protocol import VERSION, NAMES, spec_for


def selected_freeze(freeze):
    if not isinstance(freeze, dict) or freeze.get('mode') != 'shadow_only' or freeze.get('production_approved') is not False:
        raise ValueError('expected an unapproved shadow-only freeze')
    if 'candidates' not in freeze:
        return freeze
    common = freeze.get('common')
    request = common.get('request') if isinstance(common, dict) else None
    if not isinstance(request, dict) or request.get('version') != VERSION:
        raise ValueError('unsupported multi-candidate freeze version')
    name = freeze.get('selected')
    candidates = freeze['candidates']
    if name not in NAMES or not isinstance(candidates, dict) or set(candidates) != set(NAMES):
        raise ValueError('invalid v6 frozen candidate set/selection')
    selected = candidates[name]
    if not isinstance(selected, dict):
        raise ValueError('invalid frozen selection')
    threshold = selected.get('threshold')
    # Validate strict threshold without touching any held-out scores.
    confusion([0, 1], [0., 1.], threshold)
    digest = selected.get('checkpoint_sha256')
    return dict(mode='shadow_only', production_approved=False,
        selected=name, threshold=threshold, artifact_sha256=digest,
        encoding_view=spec_for(name)['context_view'], requires_freeze_attestation=True,
        empirical_calibration=selected['calibration'])
