#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
printf 'Go: %s (go.work; backend/wrapper/agent-tui/dev-env-tui modules)\n' "$CODEX_GO_VERSION"
printf 'Node: %s; Bun: %s (.devcontainer/Dockerfile)\n' "$CODEX_NODE_VERSION" "$CODEX_BUN_VERSION"
printf 'Python: %s (adapters/python/.python-version)\n' "$CODEX_PY_VERSION"
printf 'Build: Make, Clang/LLVM/BPF, protobuf, Vite/Vue, VitePress, TypeScript, uv\n'
printf 'Declared lockfiles/manifests:\n'
(cd "$CODEX_ROOT" && git ls-files | grep -E '(^|/)(go\.work|go\.(mod|sum)|bun\.lock|package\.json|pyproject\.toml|uv\.lock|\.python-version|Cargo\.(toml|lock)|rust-toolchain(\.toml)?|pubspec\.(yaml|lock)|\.nvmrc|\.node-version|\.tool-versions)$' || true)
unsupported="$(cd "$CODEX_ROOT" && git ls-files | grep -E '(^|/)(Cargo\.toml|pubspec\.yaml)$' | grep -v '^docs/ref/' || true)"
if [[ -n "$unsupported" ]]; then
  echo 'Rust/Flutter manifests newly detected; extend and pin the installer before publishing:' >&2
  printf '%s\n' "$unsupported" >&2
  exit 1
fi
printf 'No first-party Rust or Flutter manifests; corresponding SDKs intentionally omitted.\n'
