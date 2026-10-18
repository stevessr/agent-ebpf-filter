#!/usr/bin/env python3
"""Export a small CodeSpace file-name manifest without reading source files.

Use this on the CodeSpace side, then copy the JSON to the Makepad viewer host.
Only relative file names are serialized; no source content or file metadata.
"""
from __future__ import annotations

import argparse
import json
import os
import stat
import tempfile
from pathlib import Path

MAX_FILES = 2048
MAX_ENTRIES = 30000
MAX_DEPTH = 8
MAX_PATH_LEN = 360
SKIP = {
    ".git", ".cache", ".next", "node_modules", "target", "dist",
    "build", "__pycache__", "vendor", ".venv", "venv",
}


def collect_names(root: Path) -> list[str]:
    names: list[str] = []
    pending: list[tuple[Path, int]] = [(root, 0)]
    seen = 0
    while pending and seen < MAX_ENTRIES and len(names) < MAX_FILES:
        directory, depth = pending.pop()
        if depth > MAX_DEPTH:
            continue
        try:
            entries = sorted(os.scandir(directory), key=lambda entry: entry.name)
        except (OSError, PermissionError):
            continue
        for entry in entries:
            seen += 1
            if seen > MAX_ENTRIES or len(names) >= MAX_FILES:
                break
            if entry.name.startswith(".") or entry.name in SKIP:
                continue
            try:
                metadata = entry.stat(follow_symlinks=False)
            except OSError:
                continue
            if stat.S_ISLNK(metadata.st_mode):
                continue
            if stat.S_ISDIR(metadata.st_mode):
                if depth < MAX_DEPTH:
                    pending.append((Path(entry.path), depth + 1))
            elif stat.S_ISREG(metadata.st_mode):
                try:
                    relative = Path(entry.path).relative_to(root).as_posix()
                except ValueError:
                    continue
                if len(relative) <= MAX_PATH_LEN and not any(
                    ord(char) < 32 for char in relative
                ):
                    names.append(relative)
    return sorted(set(names))


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("workspace", type=Path, help="CodeSpace workspace root")
    parser.add_argument("--output", type=Path, required=True, help="Output JSON path")
    args = parser.parse_args()
    root = args.workspace.resolve(strict=True)
    if not root.is_dir():
        parser.error("workspace must be an existing directory")
    names = collect_names(root)
    payload = json.dumps({"version": 1, "files": names}, ensure_ascii=False, separators=(",", ":"))
    if len(payload.encode("utf-8")) > 1024 * 1024:
        parser.error("manifest exceeds 1 MiB")
    destination = args.output.expanduser().absolute()
    destination.parent.mkdir(parents=True, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".codecity-index-", suffix=".json", dir=destination.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as writer:
            writer.write(payload)
            writer.write("\n")
        os.chmod(temporary, 0o600)
        os.replace(temporary, destination)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)
    print(f"Wrote {len(names)} relative file paths to {destination}")


if __name__ == "__main__":
    main()
