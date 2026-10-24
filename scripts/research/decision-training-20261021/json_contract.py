"""Bounded UTF-8 research input; duplicate keys and non-finite JSON fail closed."""
import json
import math


def _pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError('duplicate JSON key')
        result[key] = value
    return result


def _nonfinite(value):
    raise ValueError('non-finite JSON number')


def _float(value):
    number = float(value)
    if not math.isfinite(number):
        raise ValueError('non-finite JSON number')
    return number


def _decode(raw):
    try:
        result = json.loads(raw.decode('utf-8'), object_pairs_hook=_pairs,
                            parse_constant=_nonfinite, parse_float=_float)
    except (UnicodeDecodeError, RecursionError) as exc:
        raise ValueError('invalid UTF-8 or nested JSON input') from exc
    if not isinstance(result, dict):
        raise ValueError('expected JSON object')
    return result


def read_json(path, *, max_bytes=1 << 20):
    if type(max_bytes) is not int or max_bytes < 1:
        raise ValueError('invalid JSON byte budget')
    with path.open('rb') as stream:
        raw = stream.read(max_bytes + 1)
    if len(raw) > max_bytes:
        raise ValueError('JSON byte budget exceeded')
    return _decode(raw)


def read_jsonl(path, *, max_bytes=64 << 20, max_line_bytes=1 << 20, max_rows=100000):
    if any(type(v) is not int or v < 1 for v in (max_bytes, max_line_bytes, max_rows)):
        raise ValueError('invalid JSONL budget')
    rows, total = [], 0
    with path.open('rb') as stream:
        while True:
            raw = stream.readline(max_line_bytes + 1)
            if not raw:
                break
            total += len(raw)
            if len(raw) > max_line_bytes:
                raise ValueError('JSONL line budget exceeded')
            if total > max_bytes:
                raise ValueError('JSONL byte budget exceeded')
            if raw.strip():
                if len(rows) >= max_rows:
                    raise ValueError('JSONL row budget exceeded')
                rows.append(_decode(raw))
    return rows
