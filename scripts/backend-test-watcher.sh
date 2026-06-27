#!/bin/bash
# backend-test-watcher.sh
# 监听 backend/ 目录下 .go 文件变动，自动运行 go test
# 与 make dev-backend（自动重编译）并行工作
# 使用 polling 方式（无需 inotify-tools）

PROJECT_DIR="/home/steve/文档/vibe coding/agent-ebpf-filiter"
TIMESTAMP_FILE="/tmp/.backend-test-watcher-timestamp"
POLL_INTERVAL=4

# 初始时间戳
touch "$TIMESTAMP_FILE"

echo "[test-watcher] 开始监听 backend/**/*.go 文件变动（每 ${POLL_INTERVAL}s 轮询）"
echo "[test-watcher] 文件变动时将自动运行: go test ./backend/..."
echo "[test-watcher] PID: $$"
echo ""

while true; do
    CHANGED=$(find "$PROJECT_DIR/backend" -name "*.go" -newer "$TIMESTAMP_FILE" 2>/dev/null | head -5)
    if [ -n "$CHANGED" ]; then
        echo "[test-watcher] $(date '+%H:%M:%S') 检测到文件变动:"
        echo "$CHANGED" | sed "s|$PROJECT_DIR/||" | sed 's/^/  > /'
        echo ""

        # 更新时间戳（防止重复触发）
        touch "$TIMESTAMP_FILE"

        # 运行测试
        cd "$PROJECT_DIR/backend"
        go test ./... 2>&1
        EXIT_CODE=$?
        echo ""
        if [ "$EXIT_CODE" -eq 0 ]; then
            echo "[test-watcher] ✅ $(date '+%H:%M:%S') 所有测试通过"
        else
            echo "[test-watcher] ❌ $(date '+%H:%M:%S') 测试失败（exit code: $EXIT_CODE）"
        fi
        echo "────────────────────────────────────────"
    fi
    sleep "$POLL_INTERVAL"
done
