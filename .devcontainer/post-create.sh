#!/usr/bin/env bash
set -euo pipefail

# Keep generated dependencies out of postCreate failures when the workspace is
# opened on a host/kernel that cannot expose eBPF features to Docker yet.
export PATH="/usr/local/go/bin:/go/bin:/usr/local/bun/bin:/usr/local/bin:$PATH"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_ENV_FILE="${DEV_ENV_FILE:-$ROOT/.env.dev}"
if [ -f "$DEV_ENV_FILE" ]; then
  set -a
  # shellcheck disable=SC1090
  . "$DEV_ENV_FILE"
  set +a
fi
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
  local check_path="$4"

  if [ -x "$target_dir/$check_path" ]; then
    return
  fi
  if [ ! -d "$source_dir" ]; then
    return
  fi

  echo "[devcontainer] Seeding ${label} from the workflow-built image."
  if [ -e "$target_dir" ]; then
    echo "[devcontainer] Existing ${label} is not usable in this container; replacing it."
    rm -rf "$target_dir"
  fi
  mkdir -p "$(dirname "$target_dir")"
  cp -a "$source_dir" "$target_dir"
}

seed_predev_dir /opt/agent-ebpf-predev/frontend/node_modules frontend/node_modules "frontend node_modules" ".bin/pbjs"
seed_predev_dir /opt/agent-ebpf-predev/adapters/python/.venv adapters/python/.venv "Python virtualenv" "bin/python"

if ! make --no-print-directory predev-check; then
  cat >&2 <<'EOF'
[devcontainer] Required development dependencies are missing from this container.
[devcontainer] postCreate does not install them from the network by default.
[devcontainer]
[devcontainer] This usually means VS Code opened a stale GHCR devcontainer image
[devcontainer] that was built before the workflow started running `make predev`.
[devcontainer] Re-run/wait for the "Devcontainer Image" workflow, pull the new
[devcontainer] branch image, and rebuild/reopen the Dev Container.
[devcontainer]
[devcontainer] If you intentionally want postCreate to install online anyway,
[devcontainer] reopen with DEVCONTAINER_POSTCREATE_INSTALL=1.
EOF

  if [ "${DEVCONTAINER_POSTCREATE_INSTALL:-}" = "1" ]; then
    echo "[devcontainer] DEVCONTAINER_POSTCREATE_INSTALL=1 set; running make predev online."
    make predev
  else
    exit 1
  fi
fi

echo "[devcontainer] Ready. Use: make dev"
