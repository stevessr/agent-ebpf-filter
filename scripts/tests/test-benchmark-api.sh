#!/usr/bin/env bash
# Regression tests: status must come from an authenticated HTTP 200 JSON response.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../benchmark-api.sh
source "$ROOT_DIR/scripts/benchmark-api.sh"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/home/.config/agent-ebpf-filter"
printf '{"accessToken":"file-token"}\n' > "$tmp/home/.config/agent-ebpf-filter/runtime.json"
export BENCH_API_TOKEN_FILE="$tmp/home/.config/agent-ebpf-filter/runtime.json"
unset BENCH_API_KEY AGENT_API_KEY AGENT_EBPF_ACCESS_TOKEN || true

[[ "$(benchmark_api_token)" == "file-token" ]] || { echo "token file lookup failed" >&2; exit 1; }
export BENCH_API_KEY="env-token"
[[ "$(benchmark_api_token)" == "env-token" ]] || { echo "token override failed" >&2; exit 1; }
export BENCH_API_URL="http://127.0.0.1:49000"
MOCK_CURL_CODE=0
MOCK_CURL_BODY='{"attachedCount":2,"status":"partial"}'
curl() {
    [[ "$*" == *"--fail"* ]] || { echo "HTTP error checking is missing" >&2; return 97; }
    [[ "$*" == *"X-API-KEY: env-token"* ]] || { echo "missing authenticated header" >&2; return 99; }
    [[ "$*" == *"http://127.0.0.1:49000/system/bootstrap-health"* ]] || { echo "incorrect bootstrap URL" >&2; return 98; }
    if [[ "$MOCK_CURL_CODE" != "0" ]]; then
        return "$MOCK_CURL_CODE"
    fi
    printf '%s\n' "$MOCK_CURL_BODY"
}

health="$(benchmark_bootstrap_health)"
benchmark_has_attached_programs "$health" || { echo "valid partial attach rejected" >&2; exit 1; }

for bad in '{"attachedCount":0}' '{"attachedCount":"2"}' '{"status":"unknown"}' '{"attachedCount":null}' '{"attachedCount":-1}' '{"attachedCount":0.5}' 'not json'; do
    if benchmark_has_attached_programs "$bad" 2>/dev/null; then
        echo "accepted invalid attachment count: $bad" >&2
        exit 1
    fi
done

MOCK_CURL_CODE=22
if benchmark_bootstrap_health >/dev/null 2>&1; then
    echo "HTTP 401/403 was incorrectly accepted" >&2
    exit 1
fi
MOCK_CURL_CODE=0
unset BENCH_API_KEY
export BENCH_API_TOKEN_FILE="$tmp/missing.json"
if benchmark_bootstrap_health >/dev/null 2>&1; then
    echo "missing credentials were incorrectly accepted" >&2
    exit 1
fi
echo "Authenticated benchmark bootstrap checks passed"
