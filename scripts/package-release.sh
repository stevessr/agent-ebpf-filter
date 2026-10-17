#!/usr/bin/env bash
# Assemble the exact payload expected by scripts/install-service.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ARCH="${1:-}"
OUT="${2:-$ROOT/dist/release}"

case "$ARCH" in
  amd64|arm64) ;;
  *) echo "Usage: $0 {amd64|arm64} [output-dir]" >&2; exit 2 ;;
esac

[[ -f "$ROOT/backend/agent-ebpf-filter" ]] || { echo "Missing compiled backend" >&2; exit 1; }
[[ -f "$ROOT/agent-wrapper" ]] || { echo "Missing compiled wrapper" >&2; exit 1; }
[[ -d "$ROOT/frontend/dist" ]] || { echo "Missing compiled frontend" >&2; exit 1; }
[[ -f "$ROOT/frontend/dist/index.html" ]] || { echo "Missing frontend index.html" >&2; exit 1; }

mkdir -p "$OUT"
OUT="$(cd "$OUT" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf -- "$tmp"' EXIT

mkdir -p "$tmp/backend" "$tmp/frontend" "$tmp/scripts"
install -m 0755 "$ROOT/backend/agent-ebpf-filter" "$tmp/backend/agent-ebpf-filter"
install -m 0755 "$ROOT/agent-wrapper" "$tmp/agent-wrapper"
cp -a "$ROOT/frontend/dist" "$tmp/frontend/dist"
install -m 0755 "$ROOT/scripts/install-service.sh" "$tmp/scripts/install-service.sh"
install -m 0644 "$ROOT/README.md" "$tmp/README.md"
install -m 0644 "$ROOT/AGENTS.md" "$tmp/AGENTS.md"
install -m 0644 "$ROOT/LICENSE" "$tmp/LICENSE"

archive="$OUT/agent-ebpf-filter-linux-$ARCH.tar.gz"
tar -C "$tmp" -czf "$archive" backend frontend agent-wrapper scripts README.md AGENTS.md LICENSE
echo "Built $archive"
