#!/usr/bin/env python3
"""Check repository-local Markdown links used by the documentation set.

VitePress currently has ``ignoreDeadLinks`` enabled because the docs include
historical notes and links to source files outside ``docs/``.  This script is a
lighter repository-aware guard: it ignores external URLs and generated/reference
snapshots, but verifies that Markdown links to local docs, component READMEs and
source files still resolve.
"""

from __future__ import annotations

import argparse
import re
import sys
import urllib.parse
from pathlib import Path
from typing import Iterable


DEFAULT_ROOTS = (
    "README.md",
    "README_cn.md",
    "AGENTS.md",
    "CLAUDE.md",
    "docs",
    "backend/README.md",
    "frontend/README.md",
    "wrapper/README.md",
    "adapters/python/README.md",
    "adapters/js/README.md",
    "kernel-ml/README.md",
    ".devcontainer/README.md",
    "tools/dev-env-tui/README.md",
)

SKIP_PARTS = {
    ".git",
    ".vitepress/dist",
    "node_modules",
    "__pycache__",
}

LOCAL_LINK_RE = re.compile(
    r"""(?<!!)\[[^\]]*]\(([^)]+)\)|<a\s+[^>]*href=["']([^"']+)["']""",
    re.IGNORECASE,
)
VITEPRESS_LINK_RE = re.compile(r"""link:\s*["']([^"']+)["']""")

SCHEME_RE = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.-]*:")


def should_skip(path: Path, include_ref: bool) -> bool:
    text = path.as_posix()
    if not include_ref and text.startswith("docs/ref/"):
        return True
    return any(part in text for part in SKIP_PARTS)


def iter_markdown_files(root: Path, include_ref: bool) -> Iterable[Path]:
    if root.is_file() and root.suffix.lower() in {".md", ".mdx"}:
        if not should_skip(root, include_ref):
            yield root
        return
    if not root.is_dir():
        return
    for path in root.rglob("*"):
        if path.suffix.lower() not in {".md", ".mdx"}:
            continue
        if should_skip(path, include_ref):
            continue
        yield path


def normalize_target(raw: str) -> str:
    target = raw.strip()
    # Markdown links may include an optional title: [text](file.md "title").
    if " " in target and any(q in target for q in ("'", '"')):
        target = target.split()[0]
    target = target.strip("<>")
    target = target.split("#", 1)[0].split("?", 1)[0].strip()
    return urllib.parse.unquote(target)


def candidate_paths(markdown_file: Path, target: str) -> list[Path]:
    if target.startswith("/"):
        base = Path("docs") / target.lstrip("/")
    else:
        base = markdown_file.parent / target

    candidates = [base]
    if base.suffix == "":
        candidates.extend([base.with_suffix(".md"), base.with_suffix(".mdx"), base / "README.md", base / "index.md"])
    elif base.suffix == ".html":
        candidates.append(base.with_suffix(".md"))
    return candidates


def resolved_path(source_file: Path, raw: str) -> Path | None:
    target = normalize_target(raw)
    if is_ignored_target(raw, target):
        return None
    for candidate in candidate_paths(source_file, target):
        if candidate.exists():
            return candidate
    return None


def normalize_existing_path(path: Path) -> Path:
    try:
        return path.resolve().relative_to(Path.cwd().resolve())
    except Exception:
        return path


def is_ignored_target(raw: str, target: str) -> bool:
    if not target or target.startswith("#"):
        return True
    if raw.startswith("<") and raw.endswith(">") and SCHEME_RE.match(raw.strip("<>")):
        return True
    if SCHEME_RE.match(target):
        return True
    if target.startswith(("#", "mailto:", "tel:", "data:")):
        return True
    return False


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("paths", nargs="*", help="Markdown files or directories to scan")
    parser.add_argument("--include-ref", action="store_true", help="also scan docs/ref snapshots")
    parser.add_argument("--report", action="store_true", help="print a small inbound/outbound report for docs pages")
    args = parser.parse_args()

    roots = [Path(p) for p in (args.paths or DEFAULT_ROOTS)]
    files = sorted({p for root in roots for p in iter_markdown_files(root, args.include_ref)})
    missing: list[tuple[Path, str, list[Path]]] = []
    outbound: dict[Path, set[Path]] = {path: set() for path in files}
    inbound: dict[Path, set[Path]] = {}

    for markdown_file in files:
        text = markdown_file.read_text(encoding="utf-8", errors="ignore")
        for match in LOCAL_LINK_RE.finditer(text):
            raw = (match.group(1) or match.group(2) or "").strip()
            target = normalize_target(raw)
            if is_ignored_target(raw, target):
                continue
            candidates = candidate_paths(markdown_file, target)
            resolved = next((path for path in candidates if path.exists()), None)
            if resolved is None:
                missing.append((markdown_file, raw, candidates[:4]))
                continue
            resolved = normalize_existing_path(resolved)
            outbound.setdefault(markdown_file, set()).add(resolved)
            inbound.setdefault(resolved, set()).add(markdown_file)

    # VitePress nav/sidebar links are not Markdown, but they are first-class
    # documentation entrypoints and should be checked as well.
    vitepress_config = Path("docs/.vitepress/config.ts")
    if vitepress_config.exists():
        text = vitepress_config.read_text(encoding="utf-8", errors="ignore")
        for raw in VITEPRESS_LINK_RE.findall(text):
            target = normalize_target(raw)
            if is_ignored_target(raw, target):
                continue
            candidates = candidate_paths(vitepress_config, target)
            resolved = next((path for path in candidates if path.exists()), None)
            if resolved is None:
                missing.append((vitepress_config, raw, candidates[:4]))
                continue
            resolved = normalize_existing_path(resolved)
            outbound.setdefault(vitepress_config, set()).add(resolved)
            inbound.setdefault(resolved, set()).add(vitepress_config)

    if missing:
        print(f"checked_files={len(files)} missing_links={len(missing)}")
        for markdown_file, raw, candidates in missing:
            print(f"{markdown_file}: missing {raw!r}")
            print("  candidates: " + ", ".join(str(path) for path in candidates))
        return 1

    print(f"checked_files={len(files)} missing_links=0")
    if args.report:
        docs_pages = sorted(
            path for path in Path("docs").rglob("*.md")
            if not should_skip(path, args.include_ref)
        )
        weak = [
            (len(inbound.get(path, set())), len(outbound.get(path, set())), path)
            for path in docs_pages
        ]
        print("\nlowest_inbound:")
        for inbound_count, outbound_count, path in sorted(weak)[:25]:
            print(f"{inbound_count:2d} in {outbound_count:2d} out  {path}")
        print("\nlowest_outbound:")
        for inbound_count, outbound_count, path in sorted(weak, key=lambda row: (row[1], row[0], row[2]))[:25]:
            print(f"{inbound_count:2d} in {outbound_count:2d} out  {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
