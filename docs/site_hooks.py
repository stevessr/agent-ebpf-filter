"""Compatibility hooks for root-relative links and old admonition containers."""
from __future__ import annotations

import os
import re
from pathlib import Path, PurePosixPath
from urllib.parse import urlsplit

DOCS = Path(__file__).resolve().parent
LINK = re.compile(r'(?P<start>!?\[[^\]\n]*\]\()(?P<url>/[^)\s]*)(?P<end>\))')
ADMONITION = re.compile(r"^:::\s*(tip|warning|danger|info|details)(?:\s+(.*?))?\s*$")
KINDS = {"tip": "tip", "warning": "warning", "danger": "danger", "info": "info", "details": "note"}


def _link(match, page):
    parsed = urlsplit(match.group("url"))
    if not parsed.path.startswith("/") or parsed.path.startswith("//"):
        return match.group(0)
    target = parsed.path.lstrip("/")
    if not target:
        target = "index.md"
    elif not PurePosixPath(target).suffix:
        if (DOCS / target / "index.md").is_file():
            target = target.rstrip("/") + "/index.md"
        else:
            target += ".md"
    file = DOCS / target
    if not file.is_file():
        return match.group(0)
    parent = PurePosixPath(page.file.src_path).parent.as_posix()
    relative = os.path.relpath(file.relative_to(DOCS).as_posix(), start=parent or ".").replace(os.sep, "/")
    suffix = ("?" + parsed.query if parsed.query else "") + ("#" + parsed.fragment if parsed.fragment else "")
    return match.group("start") + relative + suffix + match.group("end")


def _admonitions(markdown):
    lines = []
    in_fence = False
    fence_mark = ""
    container = False
    for line in markdown.splitlines(keepends=True):
        stripped = line.strip()
        if stripped.startswith(("```", "~~~")):
            marker = stripped[:3]
            if not in_fence:
                in_fence, fence_mark = True, marker
            elif marker == fence_mark:
                in_fence = False
        if not in_fence and not container:
            match = ADMONITION.fullmatch(stripped)
            if match:
                kind, title = KINDS[match.group(1)], match.group(2)
                lines.append('!!! ' + kind + (' "' + title.replace('"', "'") + '"' if title else "") + "\n")
                container = True
                continue
        if not in_fence and container and stripped == ":::":
            container = False
            continue
        lines.append(("    " + line if stripped else "\n") if container else line)
    return "".join(lines)


def on_page_markdown(markdown, page, config, files):
    return LINK.sub(lambda match: _link(match, page), _admonitions(markdown))
