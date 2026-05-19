#!/usr/bin/env bash
set -euo pipefail

# Keep generated dependencies out of postCreate failures when the workspace is
# opened on a host/kernel that cannot expose eBPF features to Docker yet.
export PATH="/usr/local/go/bin:/go/bin:/usr/local/bun/bin:/usr/local/bin:$PATH"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if ! mountpoint -q /sys/fs/bpf; then
  echo "[devcontainer] /sys/fs/bpf is not mounted; trying to mount bpffs with sudo."
  sudo mount -t bpf bpf /sys/fs/bpf || true
fi

if command -v go >/dev/null; then
  go env -w GOPATH=/go >/dev/null
fi

seed_predev_dir() {
  local source_dir="$1"
  local target_dir="$2"
  local label="$3"

  if [ -d "$target_dir" ]; then
    return
  fi
  if [ ! -d "$source_dir" ]; then
    return
  fi

  echo "[devcontainer] Seeding ${label} from the workflow-built image."
  mkdir -p "$(dirname "$target_dir")"
  cp -a "$source_dir" "$target_dir"
}

seed_predev_dir /opt/agent-ebpf-predev/frontend/node_modules frontend/node_modules "frontend node_modules"
seed_predev_dir /opt/agent-ebpf-predev/adapters/python/.venv adapters/python/.venv "Python virtualenv"

make predev

echo "[devcontainer] Ready. Use: make dev"
