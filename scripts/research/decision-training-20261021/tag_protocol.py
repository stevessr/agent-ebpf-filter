"""Versioned reporting metadata. Tags never create labels or model inputs."""
import re
import unicodedata

TAG_VERSION = "behavior_tags.v1"
MAX_TAGS = 32
# Runtime BehaviorCategory names plus explicit research diagnostic extensions.
# Membership is reporting metadata, never a label or a risk verdict.
KNOWN_TAGS = frozenset(('unknown file_read file_write file_delete file_permission '
    'network process_exec process_kill system_info package_manager database '
    'compression development container sensitive http_request privacy_leak destructive').split())
ALIASES = {
    "文件读取": "file_read", "读取文件": "file_read",
    "文件写入": "file_write", "写入文件": "file_write",
    "文件删除": "file_delete", "删除文件": "file_delete",
    "文件权限": "file_permission", "进程执行": "process_exec",
    "网络请求": "network", "隐私泄露": "privacy_leak",
    "破坏性操作": "destructive", "未知": "unknown",
    "read_file": "file_read", "file_reading": "file_read",
    "write_file": "file_write", "file_writing": "file_write",
    "delete_file": "file_delete", "file_deletion": "file_delete",
    "file_permissions": "file_permission", "permission_change": "file_permission",
    "exec": "process_exec", "execve": "process_exec", "process_execute": "process_exec",
    "kill_process": "process_kill", "network_request": "network",
    "http": "http_request", "https_request": "http_request",
    "privacy_leakage": "privacy_leak", "privacy_leaks": "privacy_leak",
    "destructive_action": "destructive", "destructive_behavior": "destructive",
    "package_management": "package_manager", "system_information": "system_info",
    "uncategorized": "unknown", "unlabeled": "unknown", "none": "unknown",
}


def normalize_tag(raw):
    """NFKC + camel/snake/kebab/space equivalence, with explicit semantic aliases.

    Other syntactically valid tags are preserved. Identity fields, raw payloads,
    labels and dataset domains are deliberately not passed through this function.
    """
    if not isinstance(raw, str) or len(raw) > 128:
        raise ValueError("invalid tag type or length")
    value = unicodedata.normalize("NFKC", raw).strip()
    if value in ALIASES:
        return ALIASES[value]
    if value in ("", "-", "_"):
        return "unknown"
    value = re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1_\2", value)
    value = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", value)
    value = re.sub(r"[\s_-]+", "_", value).strip("_").lower()
    if not value:
        return "unknown"
    if not re.fullmatch(r"[a-z][a-z0-9_]{0,63}", value):
        raise ValueError("invalid tag syntax")
    return ALIASES.get(value, value)


def normalize_rows(rows):
    """Return copies with sorted/deduplicated tags; never rewrite binary labels."""
    return normalize_with_audit(rows)[0]


def normalize_with_audit(rows):
    """Normalize once and return bounded aggregate tag coverage/loss accounting.

    Custom valid IDs are preserved and counted separately from the declared
    taxonomy. No raw contexts, actions, identities or tag spellings enter audit.
    Unknown-only rows do not count as annotated. Row coverage categories can
    overlap (one row may have both known and custom tags).
    """
    result = []
    audit = dict(rows=0, annotated_rows=0, unknown_only_rows=0,
        implicit_unknown_rows=0, known_taxonomy_rows=0, custom_tag_rows=0,
        input_values=0, canonicalized_values=0, duplicate_values=0,
        explicit_unknown_values=0, suppressed_unknown_values=0)
    for row in rows:
        if not isinstance(row, dict):
            raise ValueError("expected an action row")
        if row.get("tag_version", TAG_VERSION) != TAG_VERSION:
            raise ValueError("unsupported tag version")
        values = []
        for field in ("tags", "tag", "category"):
            if field not in row:
                continue
            raw = row[field]
            if isinstance(raw, str):
                raw = [raw]
            if not isinstance(raw, list) or len(raw) > MAX_TAGS:
                raise ValueError("expected bounded tag list")
            values.extend(raw)
        if len(values) > MAX_TAGS:
            raise ValueError("too many tags")
        canonical = [normalize_tag(v) for v in values]
        tags = set(canonical) or {"unknown"}
        unknown_count = canonical.count('unknown')
        audit['rows'] += 1
        audit['input_values'] += len(values)
        audit['canonicalized_values'] += sum(a != b for a, b in zip(values, canonical))
        audit['duplicate_values'] += len(canonical) - len(set(canonical))
        audit['explicit_unknown_values'] += unknown_count
        audit['implicit_unknown_rows'] += int(not values)
        if len(tags) > 1:
            tags.discard("unknown")
            audit['suppressed_unknown_values'] += unknown_count
        audit['unknown_only_rows'] += int(tags == {'unknown'})
        audit['annotated_rows'] += int(tags != {'unknown'})
        audit['known_taxonomy_rows'] += int(bool(tags & (KNOWN_TAGS - {'unknown'})))
        audit['custom_tag_rows'] += int(bool(tags - KNOWN_TAGS))
        normalized = dict(row, tags=sorted(tags), tag_version=TAG_VERSION)
        normalized.pop("tag", None)
        normalized.pop("category", None)
        result.append(normalized)
    audit['annotation_coverage'] = audit['annotated_rows'] / audit['rows'] if audit['rows'] else None
    audit['tag_version'] = TAG_VERSION
    audit['metadata_only'] = True
    audit['coverage_categories_overlap'] = True
    return result, audit
