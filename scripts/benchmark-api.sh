#!/usr/bin/env bash
# Authenticated bootstrap-health checks shared by performance benchmarks.
# Keep the API token in memory and never print it into CI logs.

benchmark_api_token() {
    local token_file="${BENCH_API_TOKEN_FILE:-${AGENT_REAL_HOME:-$HOME}/.config/agent-ebpf-filter/runtime.json}"
    local token="${BENCH_API_KEY:-${AGENT_API_KEY:-${AGENT_EBPF_ACCESS_TOKEN:-}}}"
    if [[ -n "$token" ]]; then
        printf '%s' "$token"
        return 0
    fi
    if [[ ! -r "$token_file" ]]; then
        printf '[benchmark] Missing API token file: %s (set BENCH_API_KEY or BENCH_API_TOKEN_FILE)\n' "$token_file" >&2
        return 1
    fi
    jq -er '.accessToken | select(type == "string" and length > 0)' "$token_file" || {
        echo '[benchmark] Missing accessToken in runtime settings' >&2
        return 1
    }
}

benchmark_bootstrap_health() {
    local token
    token="$(benchmark_api_token)" || return 1
    curl --fail --silent --show-error --connect-timeout 2 --max-time 5 \
        -H "X-API-KEY: $token" \
        "${BENCH_API_URL:-http://127.0.0.1:8080}/system/bootstrap-health"
}

benchmark_has_attached_programs() {
    local response="$1"
    jq -e '.attachedCount | type == "number" and . >= 1 and . == floor' >/dev/null <<<"$response"
}
