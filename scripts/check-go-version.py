#!/usr/bin/env python3
"""Assert all Go workspace module directives and pinned CI defaults agree."""
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
TARGET = "1.27.3"
MODULES = (
    "backend/go.mod",
    "desktop/renew/go.mod",
    "tools/agent-tui/go.mod",
    "tools/dev-env-tui/go.mod",
    "wrapper/go.mod",
)
failures = []
for path in ("go.work", *MODULES):
    text = (ROOT / path).read_text(encoding="utf-8")
    m = re.search(r"(?m)^go\s+(\d+\.\d+(?:\.\d+)?)\s*$", text)
    if not m or m.group(1) != TARGET:
        failures.append(f"{path}: expected go {TARGET}, got {m.group(1) if m else 'missing'}")

pinned = (
    (".devcontainer/Dockerfile", f"ARG GO_VERSION={TARGET}"),
    ("Makefile", f"DEVCONTAINER_GO_VERSION ?= {TARGET}"),
)
for path, expected in pinned:
    if expected not in (ROOT / path).read_text(encoding="utf-8"):
        failures.append(f"{path}: missing {expected}")

for path in (ROOT / ".github/workflows").glob("*.yml"):
    if "1.27.1" in path.read_text(encoding="utf-8"):
        failures.append(f"{path.relative_to(ROOT)}: stale Go 1.27.1 pin")

if failures:
    print("\n".join(failures), file=sys.stderr)
    sys.exit(1)
print(f"Go {TARGET} workspace: {len(MODULES)} modules, CI pins and devcontainer aligned")
