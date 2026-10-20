#!/usr/bin/env bash
# Offline smoke test: verify the curl installer with mocked release assets.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TEMP="$(mktemp -d)"
trap 'rm -rf -- "$TEMP"' EXIT

case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "Skipping unsupported local test architecture" >&2; exit 0 ;;
esac

mkdir -p "$TEMP/stage/backend" "$TEMP/stage/frontend/dist" "$TEMP/stage/scripts" \
  "$TEMP/release" "$TEMP/bin"
printf '#!/bin/sh\nexit 0\n' > "$TEMP/stage/backend/agent-ebpf-filter"
printf '#!/bin/sh\nexit 0\n' > "$TEMP/stage/agent-wrapper"
chmod +x "$TEMP/stage/backend/agent-ebpf-filter" "$TEMP/stage/agent-wrapper"
printf '<html></html>\n' > "$TEMP/stage/frontend/dist/index.html"
cp "$ROOT/scripts/install-service.sh" "$TEMP/stage/scripts/install-service.sh"
printf 'Test README\n' > "$TEMP/stage/README.md"
printf 'Test AGENTS\n' > "$TEMP/stage/AGENTS.md"

archive="agent-ebpf-filter-linux-$arch.tar.gz"
tar -C "$TEMP/stage" -czf "$TEMP/release/$archive" .
(cd "$TEMP/release" && sha256sum "$archive" > SHA256SUMS)

cat > "$TEMP/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
url=""
while (($#)); do
  case "$1" in
    --output|-o) output="$2"; shift 2 ;;
    --retry|--connect-timeout) shift 2 ;;
    *) url="$1"; shift ;;
  esac
done
[[ -n "$output" && -n "$url" ]] || exit 2
cp "$TEST_RELEASE_DIR/$(basename "$url")" "$output"
EOF
chmod +x "$TEMP/bin/curl"

export TEST_RELEASE_DIR="$TEMP/release"
export PATH="$TEMP/bin:$PATH"

# Simulates "curl ... | bash" without network or root. The real service
# installer must run in dry-run mode and find every packaged input.
bash "$ROOT/scripts/install.sh" --version v1.2.3 --dry-run --no-start \
  --no-enable --prefix /opt/agent-ebpf-filter > "$TEMP/output"
grep -q 'Verifying SHA-256' "$TEMP/output"
grep -q 'Installation script completed' "$TEMP/output"

# Reject invalid tags before any download.
if bash "$ROOT/scripts/install.sh" --version ../../bad --dry-run > /dev/null 2>&1; then
  echo "Installer accepted an invalid tag" >&2
  exit 1
fi

# A corrupted checksum must prevent calling the privileged installer.
printf '%064d  %s\n' 0 "$archive" > "$TEMP/release/SHA256SUMS"
if bash "$ROOT/scripts/install.sh" --version v1.2.3 --dry-run > /dev/null 2>&1; then
  echo "Installer accepted a corrupted checksum" >&2
  exit 1
fi

echo "curl release installer smoke tests passed"
