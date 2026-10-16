#!/usr/bin/env python3
"""Validate code-anchored technical manual, diagram fences, source paths and site navigation.

MkDocs Material is authoritative for rendering. This complements the build with fast,
repo-local checks to prevent technical chapters from becoming orphaned or stale.
"""
from __future__ import annotations

import re
import sys
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parent.parent
DOCS = ROOT / "docs" / "technical"
CONFIG = ROOT / "docs" / "mkdocs.yml"
MIN_PAGES = 25
MIN_DIAGRAMS = 85
ANCHOR = re.compile(r"`((?:backend|frontend|desktop|adapters|proto|wrapper|kernel-ml|docs|scripts|tools|\.github)/[^`]+)`")
LOCAL_LINK = re.compile(r"\]\(((?:\.{1,2}/)[^)\s#?]+)")
FENCE = re.compile(r"^ {0,3}((?:\x60){3,}|~{3,})(.*)$")
PAGES = sorted(DOCS.glob("*.md"))


def check_document(page: Path, config: str, index: str, root: Path = ROOT) -> tuple[int, list[str]]:
    """Check a page against the technical nav, index and repository tree."""
    issues: list[str] = []
    text = page.read_text(encoding="utf-8")
    slug = page.stem
    target = f"technical/{slug}.md"

    # Compare complete nav entries rather than an arbitrary substring/comment.
    nav_pages = re.findall(
        r"(?m)^\s*-\s*[^:\n]+:\s*(technical/[A-Za-z0-9_.-]+\.md)\s*$",
        config,
    )
    if nav_pages.count(target) != 1:
        issues.append(f"expected exactly one MkDocs nav entry; got {nav_pages.count(target)}")
    if slug != "index":
        index_links = re.findall(
            r"\]\(\./([A-Za-z0-9_.-]+\.md)(?:#[^)]*)?\)", index
        )
        if index_links.count(page.name) != 1:
            issues.append(f"expected exactly one technical index link; got {index_links.count(page.name)}")
    if not re.search(r"(?m)^# ", text):
        issues.append("missing page title")

    # Only CLOSED Mermaid blocks count. The closing fence must match the
    # opening delimiter (backticks or tildes) and be at least as long.
    opener: str | None = None
    kind = ""
    diagrams = 0
    for line in text.splitlines():
        match = FENCE.match(line)
        if not match:
            continue
        marker, info = match.groups()
        if opener is None:
            opener = marker
            kind = info.strip().split(maxsplit=1)[0].lower() if info.strip() else ""
        elif marker[0] == opener[0] and len(marker) >= len(opener) and not info.strip():
            if kind == "mermaid":
                diagrams += 1
            opener = None
            kind = ""
    if opener is not None:
        issues.append(f"unclosed {kind or 'plain'} fence")
    if diagrams < 2:
        issues.append(f"expected >=2 closed Mermaid diagrams; got {diagrams}")

    valid_anchors = 0
    for raw in ANCHOR.findall(text):
        raw = raw.strip().rstrip(".,;:")
        if any(c.isspace() for c in raw):
            continue
        if any(c in raw for c in "*?["):
            exists = bool(list(root.glob(raw)))
        else:
            exists = (root / raw).exists()
        if exists:
            valid_anchors += 1
        else:
            issues.append(f"source path not found: {raw}")
    if valid_anchors == 0:
        issues.append("missing resolvable backticked source anchor")

    # This is intentionally scoped to explicit ./ and ../ links. The generic
    # repository docs linker independently checks all other Markdown links.
    for raw_link in LOCAL_LINK.findall(text):
        relative_link = unquote(raw_link)
        resolved = (page.parent / relative_link).resolve()
        if not resolved.is_relative_to(root.resolve()) or not resolved.is_file():
            issues.append(f"broken or out-of-repository relative link: {raw_link}")
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
