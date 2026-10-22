"""Versioned reporting metadata. Tags never create labels or model inputs."""
import re
import unicodedata

TAG_VERSION = "behavior_tags.v1"
MAX_TAGS = 32
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
    if not re.fullmatch(r"[a-z][a-z0-9_]{0,63}", value):
        raise ValueError("invalid tag syntax")
    return ALIASES.get(value, value)


def normalize_rows(rows):
    """Return copies with sorted/deduplicated tags; never rewrite binary labels."""
    result = []
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
        tags = {normalize_tag(v) for v in values} or {"unknown"}
        if len(tags) > 1:
            tags.discard("unknown")
        copy = dict(row, tags=sorted(tags), tag_version=TAG_VERSION)
        # Aliases are represented only once, making normalization idempotent.
        copy.pop("tag", None)
        copy.pop("category", None)
        result.append(copy)
    return result
