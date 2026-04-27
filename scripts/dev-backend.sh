#!/bin/bash
# Auto-reload script with eBPF-aware hot-reload and privilege handling

BACKEND_DIR="backend"
WRAPPER_PATH="$(pwd)/agent-wrapper"
BPF_C_FILE="backend/ebpf/agent_tracker.c"
BPF_CHECKSUM_FILE=".bpf_checksum"

trap "echo 'Stopping...'; [ -n \"$PID\" ] && sudo kill $PID; exit" SIGINT SIGTERM

get_checksum() {
    find "$BACKEND_DIR" proto/ \( -name "*.go" -o -name "*.c" -o -name "*.h" -o -name "*.proto" \) -exec md5sum {} + 2>/dev/null | md5sum
}

get_bpf_checksum() {
    [ -f "$BPF_C_FILE" ] && md5sum "$BPF_C_FILE" | awk '{print $1}'
}

while true; do
    echo "--- [Dev] Preparing Environment ---"
    # Remove root-owned build artifacts to prevent "Operation not permitted"
    if [ -f "backend/ebpf/agenttracker_bpfel.o" ]; then
        find backend/ebpf/ -name "agenttracker_bpf*" -user root -exec sudo rm -f {} +
    fi

    # When the eBPF C source changes, wipe old BPF pins to force fresh bootstrap
    CURRENT_BPF_SUM=$(get_bpf_checksum)
    SAVED_BPF_SUM=$(cat "$BPF_CHECKSUM_FILE" 2>/dev/null)

    if [ -n "$CURRENT_BPF_SUM" ] && [ "$CURRENT_BPF_SUM" != "$SAVED_BPF_SUM" ]; then
        if [ -n "$SAVED_BPF_SUM" ]; then
            echo "--- [Dev] eBPF C code changed, flushing old BPF pins ---"
        else
            echo "--- [Dev] First run or new eBPF code, cleaning BPF pins ---"
        fi
        sudo rm -rf /sys/fs/bpf/agent-ebpf 2>/dev/null
        echo "$CURRENT_BPF_SUM" > "$BPF_CHECKSUM_FILE"
    fi

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
