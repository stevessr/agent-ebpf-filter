#!/usr/bin/env python3
"""Build or preview the docs-owned MkDocs configuration."""
from __future__ import annotations

import os
import subprocess
import sys
import tempfile
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SOURCE = ROOT / "docs" / "mkdocs.yml"


def main() -> int:
    command = sys.argv[1] if len(sys.argv) > 1 else "build"
    if command not in ("build", "serve"):
        print("Usage: python3 docs/build_site.py [build|serve]", file=sys.stderr)
        return 2
    fd, filename = tempfile.mkstemp(prefix=".mkdocs-pages-", suffix=".yml", dir=ROOT)
    config_path = Path(filename)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as output:
            output.write(SOURCE.read_text(encoding="utf-8"))
        args = [sys.executable, "-m", "mkdocs", command, "--config-file", str(config_path)]
        if command == "build":
            args.extend(["--site-dir", str(ROOT / "site")])
        return subprocess.call(args, cwd=ROOT)
    finally:
        config_path.unlink(missing_ok=True)


if __name__ == "__main__":
    raise SystemExit(main())
