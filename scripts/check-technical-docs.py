#!/usr/bin/env python3
"""Validate code-anchored technical manual, diagram fences, source paths and site navigation.

MkDocs Material is authoritative for rendering. This complements the build with fast,
repo-local checks to prevent technical chapters from becoming orphaned or stale.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs" / "technical"
CONFIG = ROOT / "docs" / "mkdocs.yml"
MIN_PAGES = 25
MIN_DIAGRAMS = 85
ANCHOR = re.compile(r"`((?:backend|frontend|desktop|adapters|proto|wrapper|kernel-ml|docs|scripts|tools|\.github)/[^`]+)`")
LOCAL_LINK = re.compile(r"\]\(\./([^\)#]+)(?:#[^)]*)?\)")
PAGE_LINK = re.compile(r"\]\(\./([^\)#]+)(?:#[^)]*)?\)")
FENCE = re.compile(r"^\s*`{3,}([^\s`]*)")
PAGES = sorted(DOCS.glob("*.md"))


def check_document(page: Path, config: str, index: str) -> tuple[int, list[str]]:
    issues: list[str] = []
    text = page.read_text(encoding="utf-8")
    slug = page.stem
    target = f"technical/{slug}.md"
    if f": {target}" not in config:
        issues.append("not in MkDocs technical navigation")
    if slug != "index" and f"(./{slug}.md)" not in index:
        issues.append("not in technical index")
    if not re.search(r"(?m)^# ", text):
        issues.append("missing page title")
    if not ANCHOR.search(text):
        issues.append("missing backticked code source anchor")
    fence_open = False
    kind = ""
    diagrams = 0
    for line in text.splitlines():
        match = FENCE.match(line)
        if not match:
            continue
        if not fence_open:
            fence_open = True
            kind = match.group(1).strip().lower()
            if kind == "mermaid":
                diagrams += 1
        else:
            fence_open = False
            kind = ""
    if fence_open:
        issues.append(f"unclosed {kind} fence")
    if diagrams < 2:
        issues.append(f"expected >=2 Mermaid diagrams; got {diagrams}")
    for raw in ANCHOR.findall(text):
        raw = raw.strip().rstrip(".,;:")
        if any(c.isspace() for c in raw):
            continue
        if any(c in raw for c in "*?["):
            exists = bool(list(ROOT.glob(raw)))
        else:
            exists = (ROOT / raw).exists()
        if not exists:
            issues.append(f"source path not found: {raw}")
    for target_link in LOCAL_LINK.findall(text):
        if not (page.parent / target_link).exists():
            issues.append(f"broken relative link: {target_link}")
    return diagrams, issues


def main() -> int:
    errors: list[str] = []
    pages = PAGES
    if len(pages) < MIN_PAGES:
        errors.append(f"expected >={MIN_PAGES} pages, found {len(pages)}")
    config = CONFIG.read_text(encoding="utf-8")
    index = (DOCS / "index.md").read_text(encoding="utf-8")
    diagrams = 0
    for page in pages:
        n, issues = check_document(page, config, index)
        diagrams += n
        for issue in issues:
            errors.append(f"{page.relative_to(ROOT)}: {issue}")
    if diagrams < MIN_DIAGRAMS:
        errors.append(f"expected >={MIN_DIAGRAMS} Mermaid diagrams; found {diagrams}")
    if errors:
        print("\n".join("FAIL: " + e for e in errors), file=sys.stderr)
        return 1
    print(f"OK: {len(pages)} pages, {diagrams} Mermaid diagrams, source paths and navigation checked")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
