#!/bin/bash
# Auto-reload script with eBPF-aware hot-reload and privilege handling
# On startup: always clean old BPF pins to force a fresh bootstrap
# On shutdown: clean BPF pins so no stale state lingers

BACKEND_DIR="backend"
WRAPPER_PATH="$(pwd)/agent-wrapper"
BPF_PIN_ROOT="/sys/fs/bpf/agent-ebpf"

cleanup() {
    echo "--- [Dev] Shutting down ---"
    [ -n "$PID" ] && sudo kill $PID 2>/dev/null
    wait $PID 2>/dev/null
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
        # Use sudo -E to ensure eBPF loading privileges
        sudo -E DISABLE_AUTH=true AGENT_WRAPPER_PATH="$WRAPPER_PATH" ./backend/agent-ebpf-filter &
        PID=$!

        LAST_SUM=$(get_checksum)
        while true; do
            sleep 2
            CURRENT_SUM=$(get_checksum)
            if [ "$LAST_SUM" != "$CURRENT_SUM" ]; then
                echo "--- [Dev] Source code changed, restarting ---"
                sudo kill $PID 2>/dev/null
                wait $PID 2>/dev/null
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
