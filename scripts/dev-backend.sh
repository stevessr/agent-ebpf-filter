#!/bin/bash
# 自动重载脚本，支持带空格的路径

BACKEND_DIR="backend"
WRAPPER_PATH="$(pwd)/agent-wrapper"

# 优雅退出
trap "echo 'Stopping...'; [ -n \"$PID\" ] && sudo kill $PID; exit" SIGINT SIGTERM

get_checksum() {
    find "$BACKEND_DIR" proto/ -name "*.go" -o -name "*.c" -o -name "*.h" -o -name "*.proto" | xargs md5sum 2>/dev/null | md5sum
}

while true; do
    echo "--- [Dev] Building Backend ---"
    (cd backend/ebpf && go generate) && (cd backend && go build -o agent-ebpf-filter)
    
    if [ $? -eq 0 ]; then
        echo "--- [Dev] Launching Backend ---"
        # 使用 sudo -E 继承环境变量，避免反复输入密码
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
