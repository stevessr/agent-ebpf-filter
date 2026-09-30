#!/usr/bin/env bash
set -Eeuo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/env.sh"
mode="${1:-full}"
[[ "$mode" == doctor || "$mode" == full ]] || { echo 'Usage: validate.sh [doctor|full]' >&2; exit 2; }
mkdir -p "$CODEX_ROOT/reports/codex-cloud"
report="$CODEX_ROOT/reports/codex-cloud/latest.md"
logdir="$CODEX_ROOT/reports/codex-cloud/logs"
mkdir -p "$logdir"
failures=0
{
  printf '# Codex Cloud environment verification\n\n'
  printf -- '- Timestamp (UTC): %s\n' "$(date -u +%FT%TZ)"
  printf -- '- Git commit: %s\n' "$(git -C "$CODEX_ROOT" rev-parse --verify HEAD 2>/dev/null || printf 'unavailable')"
  printf -- '- Mode: %s\n' "$mode"
  printf -- '- Host kernel: %s\n' "$(uname -r)"
  printf '\n## Tool versions\n\n'
  for tool in go node bun uv python3 protoc clang llvm-readelf; do
    if command -v "$tool" >/dev/null 2>&1; then
      case "$tool" in go) version="$(go version)" ;; node|bun) version="$($tool --version 2>&1 | head -1)" ;; uv) version="$(uv --version)" ;; python3) version="$(python3 --version)" ;; protoc) version="$(protoc --version)" ;; clang|llvm-readelf) version="$($tool --version | head -1)" ;; esac
      printf -- '- %s: %s\n' "$tool" "$version"
    else printf -- '- %s: MISSING\n' "$tool"; fi
  done
  printf '\n## Checks\n\n'
} > "$report"
run() {
  local id="$1"; shift
  local logfile="$logdir/$id.log" status
  if "$@" >"$logfile" 2>&1; then status=PASS
  else status=FAIL; failures=$((failures + 1)); fi
  printf -- '- **%s**: %s ([log](logs/%s.log))\n' "$id" "$status" "$id" >> "$report"
  printf '[codex-cloud] %s: %s\n' "$id" "$status"
  if [[ "$status" == FAIL ]]; then tail -n 12 "$logfile" >&2; fi
}
run manifest-inventory bash "$CODEX_ROOT/scripts/codex-cloud/detect.sh"
run declared-versions bash -c '[[ "$(go version | awk "{print \$3}")" == "go$CODEX_GO_VERSION" && "$(node --version)" == "v$CODEX_NODE_VERSION" && "$(bun --version)" == "$CODEX_BUN_VERSION" ]]'
run native-toolchain bash -c 'command -v protoc-gen-go && pkg-config --exists libbpf libelf && clang -target bpf -c -x c /dev/null -o "$CODEX_CACHE_HOME/clang-bpf-check.o" && test -x "$CODEX_ROOT/adapters/python/.venv/bin/python"'
run lockfiles bash -c 'for p in bun.lock frontend/bun.lock tools/bpf-ts/bun.lock adapters/python/uv.lock backend/go.sum; do test -s "$CODEX_ROOT/$p" || exit 1; done; cd "$CODEX_ROOT/adapters/python" && uv lock --check'
if [[ "$mode" == full ]]; then
  run shell-syntax bash -c 'find "$CODEX_ROOT/scripts/codex-cloud" -name "*.sh" -print0 | xargs -0 -r -n1 bash -n'
  run gofmt bash -c 'cd "$CODEX_ROOT"; files="$(git ls-files -- "*.go" | xargs -r gofmt -l)"; if [[ -n "$files" ]]; then printf "%s\n" "$files"; exit 1; fi'
  run protobuf bash -c 'cd "$CODEX_ROOT" && make SKIP_PREDEV=1 proto'
  run ebpf-generate bash -c 'cd "$CODEX_ROOT/backend/ebpf" && go generate && go generate gen_tls.go && go generate gen_cgroup.go && go generate gen_lsm.go'
  for dir in backend wrapper tools/agent-tui tools/dev-env-tui; do
    id="${dir//\//-}"
    run "$id-vet" bash -c 'cd "$CODEX_ROOT/$1" && go vet ./...' _ "$dir"
    run "$id-test" bash -c 'cd "$CODEX_ROOT/$1" && go test -count=1 -timeout=120s ./...' _ "$dir"
  done
  run go-build bash -c 'mkdir -p "$CODEX_ROOT/bin" && (cd "$CODEX_ROOT/backend" && go build -o "$CODEX_ROOT/bin/agent-ebpf-filter" .) && (cd "$CODEX_ROOT/wrapper" && go build -o "$CODEX_ROOT/bin/agent-wrapper" .)'
  run frontend-tests bash -c 'cd "$CODEX_ROOT/frontend" && bun run test:agentsight'
  run frontend-build bash -c 'cd "$CODEX_ROOT/frontend" && bun run build'
  run bpf-ts-check bash -c 'cd "$CODEX_ROOT/tools/bpf-ts" && bun run check'
  run docs-build bash -c 'cd "$CODEX_ROOT" && bun run docs:build'
  run python-adapter bash -c 'cd "$CODEX_ROOT/adapters/python" && uv run --frozen python -c "import requests, grpc_tools, google.protobuf; import agent_tracker" && uv run --frozen python -m compileall -q agent_tracker.py'
fi
{
  printf '\n## Host-only validation\n\n'
  printf 'NOT RUN: live BPF program load, verifier attach, BPF LSM and cgroup enforcement. These require an explicitly authorized privileged Linux/BTF host; a rootless cloud task must not claim success.\n\n'
  printf 'No real API tokens or production keys are required. Public package access and optional noninteractive sudo for apt are required during installation.\n\n'
  if [[ "$failures" -eq 0 ]]; then printf '## Result\n\n%s mode checks PASSED; privileged tests NOT RUN.\n' "$mode"; else printf '## Result\n\n%s check(s) FAILED. See logs.\n' "$failures"; fi
} >> "$report"
printf '[codex-cloud] Report: %s\n' "$report"
((failures == 0))
