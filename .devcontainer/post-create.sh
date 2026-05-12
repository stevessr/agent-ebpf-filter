#!/usr/bin/env bash
set -euo pipefail

# Keep generated dependencies out of postCreate failures when the workspace is
# opened on a host/kernel that cannot expose eBPF features to Docker yet.
export PATH="/usr/local/go/bin:/go/bin:/usr/local/bun/bin:/usr/local/bin:$PATH"

if ! mountpoint -q /sys/fs/bpf; then
  echo "[devcontainer] /sys/fs/bpf is not mounted; trying to mount bpffs with sudo."
  sudo mount -t bpf bpf /sys/fs/bpf || true
fi

if command -v go >/dev/null; then
  go env -w GOPATH=/go >/dev/null
fi

make predev

echo "[devcontainer] Ready. Use: make dev"
