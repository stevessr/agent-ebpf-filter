"""Label-free, bounded token planning for the evidence_pair.v1 ablation.

The model budget/architecture/vocabulary stay fixed. Only context/action token
selection changes. Retrieval uses token overlap inside this row, never labels,
tags, IDs, split names, corpus statistics, or model scores.
"""
from collections import Counter
import math
import re

VERSION = 'evidence_pair.v1'
STOP_WORDS = ('the a an and or to of in on for is are was be it this that with as by '
              'from at i you we he she they user request memory prior tool action')


def head_tail(tokens, limit):
    if len(tokens) <= limit:
        return list(tokens)
    first = (limit + 1) // 2
    return list(tokens[:first]) + list(tokens[-(limit - first):]) if limit > 1 else list(tokens[:1])


def best_window(tokens, limit, weights):
    """Max unique-overlap window, deterministic earliest tie; original order."""
    if len(tokens) <= limit:
        return list(tokens), 0
    counts = Counter(tokens[:limit])
    value = sum(weights.get(t, 0.0) for t in counts)
    best, start = value, 0
    for end in range(limit, len(tokens)):
        old, new = tokens[end - limit], tokens[end]
        counts[old] -= 1
        if not counts[old]: value -= weights.get(old, 0.0)
        if not counts[new]: value += weights.get(new, 0.0)
        counts[new] += 1
        if value > best + 1e-12:
            best, start = value, end - limit + 1
    return list(tokens[start:start + limit]), start


def _chunk_view(tokens, limit, weights):
    # Preserve the subject at the start, plus an overlap-selected later window.
    if len(tokens) <= limit:
        return list(tokens)
    anchor = min(8, limit // 4)
    window, offset = best_window(tokens[anchor:], limit - anchor, weights)
    return list(tokens[:anchor]) + window


def plan_pair(row, tok):
    context, action = row['context'][:65536], row['candidate'][:32768]
    tokenize = lambda text: tok.encode(text, add_special_tokens=False)
    stop = set(tokenize(STOP_WORDS)) | set(tok.all_special_ids)
    user, marker, rest = context.partition('\nUser memories: ')
    action_ids = tokenize(action)
    goal = head_tail(tokenize(user), 64 if marker else 128)
    chunks = []
    if marker:
        memories, prior_marker, prior = rest.partition('\nPrior tool trajectory: ')
        for prefix, text in [('Memory: ', memories), ('Prior tool: ', prior if prior_marker else '')]:
            for chunk in re.split(r'(?<=[.!?])\s+|\n', text):
                if chunk.strip():
                    chunks.append((tokenize(prefix), tokenize(chunk)))
    terms = set(action_ids) - stop
    frequency = Counter(t for _, ids in chunks for t in set(ids) if t in terms)
    weights = {t: 1 + math.log((1 + len(chunks)) / (1 + count)) for t, count in frequency.items()}
    budget = 128 - len(goal)
    ranked = []
    for index, (prefix, ids) in enumerate(chunks):
        view = _chunk_view(ids, max(1, budget - len(prefix)), weights)
        score = sum(weights.get(t, 0) for t in set(view)) / max(1, len(view)) ** .5
        ranked.append((-score, index))
    chosen = sorted(index for _, index in sorted(ranked)[:2])
    context_ids = list(goal)
    for position, index in enumerate(chosen):
        prefix, ids = chunks[index]
        available = (128 - len(context_ids)) // (len(chosen) - position)
        if available <= len(prefix):
            continue
        context_ids += prefix + _chunk_view(ids, available - len(prefix), weights)
    # A long action retains its beginning/end and a relevant middle window.
    # All action tokens stay in source order; no tokens are duplicated.
    action_truncated = len(action_ids) > 253
    if action_truncated:
        middle = action_ids[126:-63]
        overlap = {t: 1.0 for t in set(context_ids) - stop}
        window, _ = best_window(middle, 64, overlap)
        action_ids = action_ids[:126] + window + action_ids[-63:]
    sequence = [tok.cls_token_id] + context_ids + [tok.sep_token_id] + action_ids + [tok.sep_token_id]
    padding = 384 - len(sequence)
    if padding < 0:
        raise AssertionError('token budget exceeded')
    encoded = dict(input_ids=sequence + [tok.pad_token_id] * padding,
                   token_type_ids=[0] * (len(context_ids) + 2) + [1] * (len(action_ids) + 1) + [0] * padding,
                   attention_mask=[1] * len(sequence) + [0] * padding)
    audit = dict(context_tokens=len(context_ids), action_tokens=len(action_ids),
                 evidence_chunks=len(chosen), goal_truncated=len(tokenize(user)) > len(goal),
                 action_truncated=action_truncated,
                 shared_content_tokens=len((set(context_ids) & set(action_ids)) - stop))
    return encoded, audit


def plan_rows(rows, tok):
    planned = [plan_pair(row, tok) for row in rows]
    encoded = {key: [pair[0][key] for pair in planned] for key in
               ('input_ids', 'token_type_ids', 'attention_mask')}
    audit = {key: sum(pair[1][key] for pair in planned) for key in planned[0][1]} if planned else {}
    return encoded, dict(version=VERSION, rows=len(rows), totals=audit,
                         label_free=True, max_sequence_tokens=384)
