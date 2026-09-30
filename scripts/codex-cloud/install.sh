#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
[[ "$(uname -s)" == Linux ]] || { echo 'Linux is required for the eBPF toolchain.' >&2; exit 1; }
case "$(uname -m)" in x86_64) go_arch=amd64; node_arch=x64 ;; aarch64) go_arch=arm64; node_arch=arm64 ;; *) echo 'Unsupported architecture.' >&2; exit 1 ;; esac
mkdir -p "$CODEX_TOOLS_HOME" "$CODEX_CACHE_HOME" "$GOMODCACHE" "$GOCACHE" "$BUN_INSTALL_CACHE_DIR" "$UV_CACHE_DIR" "$UV_PYTHON_INSTALL_DIR" "$CODEX_ROOT/reports/codex-cloud"
tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
need() { command -v "$1" >/dev/null 2>&1 || { echo "Missing required command: $1" >&2; exit 1; }; }
fetch() { curl -fL --retry 3 --retry-delay 2 --connect-timeout 15 -o "$2" "$1"; }
install_system() {
  local packages=(build-essential ca-certificates curl git make clang llvm lld libbpf-dev libelf-dev libclang-dev libcap-dev zlib1g-dev pkg-config protobuf-compiler python3 python3-venv python3-pip xz-utils unzip jq libssl-dev linux-libc-dev)
  local prefix=()
  if [[ "$(id -u)" -eq 0 ]]; then :
  elif command -v sudo >/dev/null && sudo -n true 2>/dev/null; then prefix=(sudo -n)
  else
    echo '[codex-cloud] No noninteractive apt privileges; checking preinstalled tools.' >&2
    for tool in cc curl git make clang llvm-readelf protoc python3 xz jq pkg-config; do need "$tool"; done
    pkg-config --exists libbpf libelf || { echo 'Missing libbpf/libelf development headers; apt privileges needed.' >&2; exit 1; }
    return
  fi
  need apt-get
  "${prefix[@]}" apt-get update
  "${prefix[@]}" env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "${packages[@]}"
}
install_go() {
  if [[ -x "$CODEX_GO_DIR/bin/go" && "$("$CODEX_GO_DIR/bin/go" version | awk '{print $3}')" == "go$CODEX_GO_VERSION" ]]; then return; fi
  local file="go${CODEX_GO_VERSION}.linux-${go_arch}.tar.gz" sha
  echo "[codex-cloud] Installing pinned Go $CODEX_GO_VERSION"
  fetch 'https://go.dev/dl/?mode=json&include=all' "$tmp/go-releases.json"
  sha="$(python3 - "$CODEX_GO_VERSION" "$file" "$tmp/go-releases.json" <<'PY'
import json, sys
version, filename, path = sys.argv[1:]
with open(path, encoding='utf-8') as f:
    releases = json.load(f)
print(next((item['sha256'] for release in releases if release['version'] == 'go' + version for item in release['files'] if item['filename'] == filename), ''))
PY
)"
  [[ -n "$sha" ]] || { echo "No official SHA-256 for $file" >&2; exit 1; }
  fetch "https://go.dev/dl/$file" "$tmp/$file"
  (cd "$tmp" && printf '%s  %s\n' "$sha" "$file" | sha256sum -c -)
  mkdir -p "$tmp/go-extract"; tar -xzf "$tmp/$file" -C "$tmp/go-extract"
  rm -rf "$CODEX_GO_DIR"; mv "$tmp/go-extract/go" "$CODEX_GO_DIR"
}
install_node() {
  if [[ -x "$CODEX_NODE_DIR/bin/node" && "$("$CODEX_NODE_DIR/bin/node" --version)" == "v$CODEX_NODE_VERSION" ]]; then return; fi
  local file="node-v${CODEX_NODE_VERSION}-linux-${node_arch}.tar.xz" line
  echo "[codex-cloud] Installing pinned Node.js $CODEX_NODE_VERSION"
  fetch "https://nodejs.org/dist/v$CODEX_NODE_VERSION/SHASUMS256.txt" "$tmp/SHASUMS256.txt"
  line="$(grep -F "  $file" "$tmp/SHASUMS256.txt" | awk -v f="$file" '$2 == f {print; exit}')"
  [[ -n "$line" ]] || { echo "No official SHA-256 for $file" >&2; exit 1; }
  fetch "https://nodejs.org/dist/v$CODEX_NODE_VERSION/$file" "$tmp/$file"
  (cd "$tmp" && printf '%s\n' "$line" | sha256sum -c -)
  tar -xJf "$tmp/$file" -C "$tmp"; rm -rf "$CODEX_NODE_DIR"
  mv "$tmp/node-v${CODEX_NODE_VERSION}-linux-${node_arch}" "$CODEX_NODE_DIR"
}
install_bun() {
  if [[ -x "$CODEX_BUN_DIR/bin/bun" && "$("$CODEX_BUN_DIR/bin/bun" --version)" == "$CODEX_BUN_VERSION" ]]; then return; fi
  echo "[codex-cloud] Installing pinned Bun $CODEX_BUN_VERSION"
  fetch https://bun.sh/install "$tmp/install-bun.sh"
  rm -rf "$CODEX_BUN_DIR"
  BUN_INSTALL="$CODEX_BUN_DIR" bash "$tmp/install-bun.sh" "bun-v$CODEX_BUN_VERSION"
  [[ "$("$CODEX_BUN_DIR/bin/bun" --version)" == "$CODEX_BUN_VERSION" ]] || { echo 'Bun version check failed.' >&2; exit 1; }
}
install_uv() {
  if command -v uv >/dev/null 2>&1; then return; fi
  # Only the installer tool has a fallback pin; Python project dependencies use uv.lock.
  local uv_version="${CODEX_UV_VERSION:-0.10.9}"
  python3 -m venv "$CODEX_UV_VENV"
  "$CODEX_UV_VENV/bin/python" -m pip install --disable-pip-version-check "uv==$uv_version"
  need uv
}
bash "$CODEX_ROOT/scripts/codex-cloud/detect.sh"
install_system
for tool in curl tar sha256sum python3 clang protoc pkg-config; do need "$tool"; done
install_go; install_node; install_bun; install_uv
[[ "$(go version | awk '{print $3}')" == "go$CODEX_GO_VERSION" ]] || { echo 'Go PATH mismatch.' >&2; exit 1; }
[[ "$(node --version)" == "v$CODEX_NODE_VERSION" ]] || { echo 'Node PATH mismatch.' >&2; exit 1; }
[[ "$(bun --version)" == "$CODEX_BUN_VERSION" ]] || { echo 'Bun PATH mismatch.' >&2; exit 1; }
uv python install "$CODEX_PY_VERSION"
(cd "$CODEX_ROOT/adapters/python" && uv lock --check && uv sync --frozen --python "$CODEX_PY_VERSION")
for dir in backend wrapper tools/agent-tui tools/dev-env-tui; do (cd "$CODEX_ROOT/$dir" && go mod download); done
# Pin the generator to the version declared by backend/go.mod; do not run make predev-go (@latest).
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
for dir in . frontend tools/bpf-ts; do (cd "$CODEX_ROOT/$dir" && bun install --frozen-lockfile); done
if [[ "${CODEX_FETCH_SUBMODULES:-0}" == 1 ]]; then (cd "$CODEX_ROOT" && git submodule update --init --recursive); fi
(cd "$CODEX_ROOT" && make SKIP_PREDEV=1 proto)
(cd "$CODEX_ROOT/backend/ebpf" && go generate && go generate gen_tls.go && go generate gen_cgroup.go && go generate gen_lsm.go)
# Catch a failed or silently inconsistent install before treating a snapshot as ready.
bash "$CODEX_ROOT/scripts/codex-cloud/validate.sh" doctor
printf '\n[codex-cloud] Installation complete. Run: bash scripts/codex-cloud/validate.sh full\n'
