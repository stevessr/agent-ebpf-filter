#!/usr/bin/env bash
# Source from every Codex Cloud entry point; never source production .env files.
CODEX_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd -P)"
CODEX_TOOLS_HOME="${CODEX_TOOLS_HOME:-$HOME/.local/share/agent-ebpf-codex}"
CODEX_CACHE_HOME="${CODEX_CACHE_HOME:-$HOME/.cache/agent-ebpf-codex}"
CODEX_GO_VERSION="$(awk '$1 == "go" {print $2; exit}' "$CODEX_ROOT/go.work")"
CODEX_NODE_VERSION="$(sed -n 's/^ARG NODE_VERSION=//p' "$CODEX_ROOT/.devcontainer/Dockerfile" | head -n1)"
CODEX_BUN_VERSION="$(sed -n 's/.*bun-v\([0-9][0-9.]*\).*/\1/p' "$CODEX_ROOT/.devcontainer/Dockerfile" | head -n1)"
CODEX_PY_VERSION="$(tr -d '\r\n' < "$CODEX_ROOT/adapters/python/.python-version")"
if [[ -z "$CODEX_GO_VERSION" || -z "$CODEX_NODE_VERSION" || -z "$CODEX_BUN_VERSION" || -z "$CODEX_PY_VERSION" ]]; then
  echo '[codex-cloud] A repository SDK version declaration is missing.' >&2
  return 1 2>/dev/null || exit 1
fi
CODEX_GO_DIR="$CODEX_TOOLS_HOME/go-$CODEX_GO_VERSION"
CODEX_NODE_DIR="$CODEX_TOOLS_HOME/node-v$CODEX_NODE_VERSION"
CODEX_BUN_DIR="$CODEX_TOOLS_HOME/bun-v$CODEX_BUN_VERSION"
CODEX_UV_VENV="$CODEX_TOOLS_HOME/uv-venv"
export CODEX_ROOT CODEX_TOOLS_HOME CODEX_CACHE_HOME CODEX_GO_VERSION CODEX_NODE_VERSION CODEX_BUN_VERSION CODEX_PY_VERSION CODEX_GO_DIR CODEX_NODE_DIR CODEX_BUN_DIR CODEX_UV_VENV
export PATH="$CODEX_GO_DIR/bin:$CODEX_NODE_DIR/bin:$CODEX_BUN_DIR/bin:$CODEX_UV_VENV/bin:$HOME/go/bin:/usr/bin:$PATH"
export GOMODCACHE="$CODEX_CACHE_HOME/gomod" GOCACHE="$CODEX_CACHE_HOME/go-build" GOTOOLCHAIN=local
export BUN_INSTALL_CACHE_DIR="$CODEX_CACHE_HOME/bun" UV_CACHE_DIR="$CODEX_CACHE_HOME/uv" UV_PYTHON_INSTALL_DIR="$CODEX_CACHE_HOME/python"
export UV_LINK_MODE=copy
