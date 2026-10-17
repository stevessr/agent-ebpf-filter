#!/usr/bin/env bash
# Install the prebuilt Linux release without requiring a Go/Bun/LLVM toolchain.
# Example: curl -fsSL https://raw.githubusercontent.com/stevessr/agent-ebpf-filter/master/scripts/install.sh | bash
set -euo pipefail

REPO="stevessr/agent-ebpf-filter"
VERSION="latest"
METHOD="${INSTALL_METHOD:-auto}"
PREFIX="${INSTALL_PREFIX:-/opt/agent-ebpf-filter}"
BINDIR="${INSTALL_BINDIR:-/usr/local/bin}"
SYSCONFDIR="${INSTALL_SYSCONFDIR:-/etc/agent-ebpf-filter}"
ENABLE="${INSTALL_ENABLE:-1}"
START="${INSTALL_START:-1}"
DRY_RUN=0

log() { printf '[agent-ebpf-install] %s\n' "$*"; }
die() { printf '[agent-ebpf-install] ERROR: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Download and install an official Agent eBPF Filter Linux Release.

Usage: bash install.sh [options]

Options:
  --version TAG       Release tag, e.g. v1.2.3 (default: latest stable)
  --method METHOD     auto, systemd or rc.local (default: auto)
  --prefix PATH       Installation root (default: /opt/agent-ebpf-filter)
  --bindir PATH       Public binary directory (default: /usr/local/bin)
  --config-dir PATH   Service environment directory (default: /etc/agent-ebpf-filter)
  --no-start          Install without starting the service
  --no-enable         Install without enabling the service at boot
  --dry-run           Download and verify, then print installation operations
  -h, --help          Show this help

Pass arguments to a piped installer using: curl -fsSL URL | bash -s -- --version v1.2.3
The installer downloads from GitHub Releases, verifies SHA-256, and uses
scripts/install-service.sh from the verified archive. Privilege elevation is
performed only by that service installer.
EOF
}

while (($#)); do
  case "$1" in
    --version|--method|--prefix|--bindir|--config-dir)
      (($# >= 2)) || die "$1 requires a value"
      value="$2"
      case "$1" in
        --version) VERSION="$value" ;;
        --method) METHOD="$value" ;;
        --prefix) PREFIX="$value" ;;
        --bindir) BINDIR="$value" ;;
        --config-dir) SYSCONFDIR="$value" ;;
      esac
      shift 2
      ;;
    --no-start) START=0; shift ;;
    --no-enable) ENABLE=0; shift ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "Unknown option: $1" ;;
  esac
done

[[ "$(uname -s)" == "Linux" ]] || die "Prebuilt agent-ebpf-filter requires Linux (eBPF)."
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) die "Unsupported CPU architecture: $(uname -m) (expected x86_64 or aarch64)." ;;
esac

[[ "$VERSION" == latest || "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] ||
  die "Invalid release tag: $VERSION"
case "$METHOD" in auto|systemd|rc.local) ;; *) die "Invalid install method: $METHOD" ;; esac
for path in "$PREFIX" "$BINDIR" "$SYSCONFDIR"; do
  [[ "$path" == /* && "$path" != "/" ]] || die "Install paths must be absolute and not '/': $path"
done
for command_name in curl sha256sum tar awk mktemp; do
  command -v "$command_name" >/dev/null 2>&1 || die "Missing required command: $command_name"
done

archive="agent-ebpf-filter-linux-$ARCH.tar.gz"
if [[ "$VERSION" == latest ]]; then
  base_url="https://github.com/$REPO/releases/latest/download"
else
  base_url="https://github.com/$REPO/releases/download/$VERSION"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/agent-ebpf-install.XXXXXXXX")"
trap 'rm -rf -- "$tmp"' EXIT

log "Downloading $VERSION release for linux/$ARCH"
curl --fail --location --silent --show-error --retry 3 --connect-timeout 15 --output "$tmp/$archive" "$base_url/$archive"
curl --fail --location --silent --show-error --retry 3 --connect-timeout 15 --output "$tmp/SHA256SUMS" "$base_url/SHA256SUMS"

expected="$(awk -v file="$archive" '$2 == file {print $1; exit}' "$tmp/SHA256SUMS")"
[[ "${#expected}" -eq 64 && "$expected" =~ ^[[:xdigit:]]+$ ]] ||
  die "No valid SHA-256 entry for $archive in the release manifest"
log "Verifying SHA-256"
(cd "$tmp" && printf '%s  %s\n' "$expected" "$archive" | sha256sum --check --status) ||
  die "SHA-256 verification failed; refusing to install"

mkdir -p "$tmp/unpacked"
tar -xzf "$tmp/$archive" -C "$tmp/unpacked" --no-same-owner
for required in backend/agent-ebpf-filter agent-wrapper frontend/dist scripts/install-service.sh README.md AGENTS.md; do
  [[ -e "$tmp/unpacked/$required" ]] || die "Invalid release archive: missing $required"
done
[[ -f "$tmp/unpacked/backend/agent-ebpf-filter" && -f "$tmp/unpacked/agent-wrapper" ]] ||
  die "Release does not contain the required Linux binaries"

args=(install --method "$METHOD")
if ((DRY_RUN)); then args+=(--dry-run); fi
[[ "$ENABLE" == 0 ]] && args+=(--no-enable)
[[ "$START" == 0 ]] && args+=(--no-start)

log "Installing from verified release archive"
env "INSTALL_PREFIX=$PREFIX" "INSTALL_BINDIR=$BINDIR" \
  "INSTALL_SYSCONFDIR=$SYSCONFDIR" "INSTALL_METHOD=$METHOD" \
  "INSTALL_ENABLE=$ENABLE" "INSTALL_START=$START" \
  bash "$tmp/unpacked/scripts/install-service.sh" "${args[@]}"
log "Installation script completed"
