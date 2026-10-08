#!/usr/bin/env bash
# Destructive only to one private temporary fixture. Never run against real secrets.
set -euo pipefail

if [[ "${AGENT_ALLOW_LSM_PATH_SMOKE:-0}" != "1" ]]; then
  echo "[lsm-path-smoke] Set AGENT_ALLOW_LSM_PATH_SMOKE=1 to explicitly opt in." >&2
  exit 2
fi
command -v jq >/dev/null || { echo "[lsm-path-smoke] jq required" >&2; exit 2; }
command -v curl >/dev/null || { echo "[lsm-path-smoke] curl required" >&2; exit 2; }

BACKEND_URL="${BACKEND_URL:-http://127.0.0.1:8080}"
TOKEN="${AGENT_ACCESS_TOKEN:-${TOKEN:-}}"
socket="${AGENT_API_SOCKET:-}"
fixture_dir="$(mktemp -d)"
chmod 700 "$fixture_dir"
fixture="$fixture_dir/lsm-access-test"
printf 'test data\n' > "$fixture"
chmod 600 "$fixture"

api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-fsS -X "$method" -H 'Content-Type: application/json')
  if [[ -n "$socket" ]]; then
    args+=(--unix-socket "$socket")
    BACKEND_URL="http://desktop.internal"
  fi
  if [[ -n "$TOKEN" ]]; then args+=(-H "X-API-KEY: $TOKEN"); fi
  if [[ -n "$body" ]]; then args+=(--data "$body"); fi
  curl "${args[@]}" "$BACKEND_URL$path"
}
rule() {
  local deny_read="$1" deny_write="$2" body
  body="$(jq -nc --arg path "$fixture" --argjson r "$deny_read" --argjson w "$deny_write" '{path:$path,denyRead:$r,denyWrite:$w}')"
  api PUT /sandbox/lsm/path-access "$body"
}
cleanup() {
  rule false false >/dev/null 2>&1 || echo "[lsm-path-smoke] WARNING: failed to revoke fixture policy: $fixture" >&2
  rm -rf -- "$fixture_dir"
}
trap cleanup EXIT

status="$(api GET /sandbox/lsm/status)"
if [[ "$(jq -r '.pathAccessSupported // false' <<<"$status")" != "true" ]]; then
  echo "[lsm-path-smoke] Loaded BPF LSM does not confirm exact-path access support." >&2
  exit 1
fi

# Use the test file only, and never replace an existing active rule.
if jq -e --arg p "$fixture" '.pathAccessRules // [] | any(.path == $p)' <<<"$status" >/dev/null; then
  echo "[lsm-path-smoke] Refusing to overwrite existing fixture policy." >&2
  exit 1
fi

cat "$fixture" >/dev/null
printf 'before\n' >>"$fixture"
rule true false >/dev/null
if cat "$fixture" >/dev/null 2>&1; then
  echo "[lsm-path-smoke] FAIL: read was not denied" >&2
  exit 1
fi
rule false true >/dev/null
cat "$fixture" >/dev/null
if (printf 'after\n' >>"$fixture") 2>/dev/null; then
  echo "[lsm-path-smoke] FAIL: write was not denied" >&2
  exit 1
fi
rule false false >/dev/null
cat "$fixture" >/dev/null
printf 'restored\n' >>"$fixture"
echo "[lsm-path-smoke] PASS: exact file read/write denied and successfully revoked"
