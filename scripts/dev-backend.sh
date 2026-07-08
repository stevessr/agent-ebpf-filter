#!/bin/bash
set -uo pipefail
# Auto-reload script with eBPF-aware hot-reload and privilege handling
# On startup: always clean old BPF pins to force a fresh bootstrap
# On shutdown: clean BPF pins so no stale state lingers

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEV_ENV_FILE="${DEV_ENV_FILE:-$ROOT/.env.dev}"
if [ -f "$DEV_ENV_FILE" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$DEV_ENV_FILE"
    set +a
fi
cd "$ROOT"

# ── Enable all capabilities by default in dev mode ─────────────────────────
# These defaults apply only when the env var is NOT already set (by .env.dev
# or the calling shell).  To opt out of a specific feature, export it as
# "false" before running `make dev-backend`.
: "${AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED:=true}"
: "${AGENT_RUNTIME_SHELL_SESSIONS_ENABLED:=true}"
: "${AGENT_RUNTIME_SYSTEM_RUN_ENABLED:=true}"
: "${AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED:=true}"
: "${AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED:=true}"
: "${AGENT_RUNTIME_TLS_CAPTURE_ENABLED:=true}"
: "${AGENT_RUNTIME_OTLP_ENABLED:=true}"
: "${AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED:=true}"
: "${AGENT_ML_ENABLED:=true}"
: "${AGENT_LLM_ENABLED:=true}"
export AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED \
       AGENT_RUNTIME_SHELL_SESSIONS_ENABLED \
       AGENT_RUNTIME_SYSTEM_RUN_ENABLED \
       AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED \
       AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED \
       AGENT_RUNTIME_TLS_CAPTURE_ENABLED \
       AGENT_RUNTIME_OTLP_ENABLED \
       AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED \
       AGENT_ML_ENABLED \
       AGENT_LLM_ENABLED

BACKEND_DIR="backend"
WRAPPER_PATH="${AGENT_WRAPPER_PATH:-$ROOT/agent-wrapper}"
BACKEND_BIN="$ROOT/backend/agent-ebpf-filter"
BPF_PIN_ROOT="/sys/fs/bpf/agent-ebpf"
PID=""

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "--- [Dev] Missing required command: $1 ---" >&2
        echo "--- [Dev] Run 'make predev' after installing the repo toolchain, then retry 'make dev'. ---" >&2
        exit 127
    fi
}

require_cmd go
require_cmd sudo
require_cmd find
require_cmd md5sum

cleanup() {
    echo "--- [Dev] Shutting down ---"
    [ -n "${PID:-}" ] && sudo kill "$PID" 2>/dev/null || true
    [ -n "${PID:-}" ] && wait "$PID" 2>/dev/null || true
    echo "--- [Dev] Cleaning BPF pins ---"
    sudo rm -rf "$BPF_PIN_ROOT" 2>/dev/null
    exit
}

trap cleanup SIGINT SIGTERM

get_checksum() {
    find "$BACKEND_DIR" proto/ \( -name "*.go" -o -name "*.c" -o -name "*.h" -o -name "*.proto" \) -exec md5sum {} + 2>/dev/null | md5sum
}

while true; do
    echo "--- [Dev] Preparing Environment ---"
    # Remove root-owned build artifacts to prevent "Operation not permitted"
    if [ -f "backend/ebpf/agenttracker_bpfel.o" ]; then
        find backend/ebpf/ -name "agenttracker_bpf*" -user root -exec sudo rm -f {} +
    fi

    # Always wipe old BPF pins on startup to force a fresh eBPF bootstrap
    echo "--- [Dev] Cleaning old BPF pins for fresh bootstrap ---"
    sudo rm -rf "$BPF_PIN_ROOT" 2>/dev/null

    echo "--- [Dev] Building Backend ---"
    (cd backend/ebpf && go generate) && (cd backend && go build -o agent-ebpf-filter .)

    if [ $? -eq 0 ]; then
        echo "--- [Dev] Launching Backend ---"
        # Export first, then preserve by name. This keeps paths containing spaces
        # out of sudo's command/env assignment parser entirely. Keep this list
        # aligned with scripts/dev-env.sh groups that the privileged backend can
        # consume at startup.
        export DISABLE_AUTH="${DISABLE_AUTH:-true}"
        export GIN_MODE="${GIN_MODE:-debug}"
        export AGENT_WRAPPER_PATH="$WRAPPER_PATH"
        DEV_BACKEND_PRESERVE_ENV_KEYS=(
            DISABLE_AUTH GIN_MODE AGENT_API_KEY AGENT_ACCESS_TOKEN AGENT_BACKEND_PORT AGENT_REAL_HOME
            AGENT_WRAPPER_PATH AGENT_HOOK_ENDPOINT AGENT_SHELL_DIR AGENT_CGROUP_SANDBOX_PATH
            AGENT_EBPF_BOOTSTRAP AGENT_EBPF_NO_SANDBOX AGENT_EBPF_SANDBOX_STRICT
            AGENT_EBPF_NO_CAP_DROP AGENT_EBPF_NO_NO_NEW_PRIVS
            AGENT_CLUSTER_MASTER_URL AGENT_CLUSTER_NODE_URL AGENT_CLUSTER_NODE_ID AGENT_CLUSTER_NODE_NAME
            AGENT_CLUSTER_ACCOUNT AGENT_CLUSTER_PASSWORD
            AGENT_RUNTIME_LOG_PERSISTENCE_ENABLED AGENT_RUNTIME_LOG_FILE_PATH AGENT_RUNTIME_MAX_EVENT_COUNT
            AGENT_RUNTIME_MAX_EVENT_AGE AGENT_RUNTIME_SHELL_SESSIONS_ENABLED AGENT_RUNTIME_SYSTEM_RUN_ENABLED
            AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED
            AGENT_RUNTIME_TLS_CAPTURE_ENABLED AGENT_RUNTIME_OTLP_ENABLED AGENT_RUNTIME_OTLP_ENDPOINT
            AGENT_RUNTIME_OTLP_SERVICE_NAME AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED AGENT_RUNTIME_DOMAIN_HTTP_PORT
            AGENT_RUNTIME_DOMAIN_HTTPS_PORT AGENT_RUNTIME_DOMAIN_DEFAULT_SCHEME AGENT_RUNTIME_DOMAIN_ALLOW_ANY_HOST
            AGENT_RUNTIME_DOMAIN_DNS_RESOLVER AGENT_RUNTIME_DOMAIN_DIAL_TIMEOUT_SECONDS
            AGENT_RUNTIME_DOMAIN_CERT_FILE AGENT_RUNTIME_DOMAIN_KEY_FILE
            AGENT_ML_ENABLED AGENT_ML_MODEL_TYPE AGENT_ML_MODEL_PATH AGENT_ML_AUTO_TRAIN
            AGENT_ML_TRAIN_INTERVAL AGENT_ML_MIN_SAMPLES_FOR_TRAINING AGENT_ML_BLOCK_CONFIDENCE_THRESHOLD
            AGENT_ML_MIN_CONFIDENCE AGENT_ML_LOW_ANOMALY_THRESHOLD AGENT_ML_HIGH_ANOMALY_THRESHOLD
            AGENT_ML_ACTIVE_LEARNING_ENABLED AGENT_ML_FEATURE_HISTORY_SIZE AGENT_ML_NUM_TREES
            AGENT_ML_MAX_DEPTH AGENT_ML_MIN_SAMPLES_LEAF AGENT_ML_VALIDATION_SPLIT_RATIO
            AGENT_ML_BALANCE_CLASSES AGENT_LLM_ENABLED AGENT_LLM_BASE_URL AGENT_LLM_API_KEY
            AGENT_LLM_MODEL AGENT_LLM_TIMEOUT_SECONDS AGENT_LLM_TEMPERATURE AGENT_LLM_MAX_TOKENS
            AGENT_LLM_SYSTEM_PROMPT OPENAI_BASE_URL OPENAI_API_KEY OPENAI_MODEL
        )
        preserve_env="$(IFS=,; printf '%s' "${DEV_BACKEND_PRESERVE_ENV_KEYS[*]}")"
        sudo_backend_cmd=(
            sudo
            --preserve-env="$preserve_env"
            --
            "$BACKEND_BIN"
        )
        "${sudo_backend_cmd[@]}" &
        PID=$!

        LAST_SUM=$(get_checksum)
        while true; do
            sleep 2
            CURRENT_SUM=$(get_checksum)
            if [ "$LAST_SUM" != "$CURRENT_SUM" ]; then
                echo "--- [Dev] Source code changed, restarting ---"
                sudo kill "$PID" 2>/dev/null || true
                wait "$PID" 2>/dev/null || true
                break
            fi
        done
    else
        echo "--- [Dev] Build FAILED, waiting for changes ---"
        LAST_SUM=$(get_checksum)
        while true; do
            sleep 2
            CURRENT_SUM=$(get_checksum)
            if [ "$LAST_SUM" != "$CURRENT_SUM" ]; then break; fi
        done
    fi
done
