#!/bin/bash
# TLS 拦截功能演示脚本
# 演示 agent-ebpf-filter 的自动 TLS 明文捕获能力

set -e

BACKEND_URL="http://localhost:8080"
API_KEY="${AGENT_API_KEY:-}"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查后端是否运行
check_backend() {
    log_info "检查后端状态..."
    if ! curl -s -f "${BACKEND_URL}/health" > /dev/null 2>&1; then
        log_error "后端未运行，请先启动: cd backend && ./agent-ebpf-filter"
        exit 1
    fi
    log_success "后端运行正常"
}

# 检查 TLS 捕获状态
check_tls_status() {
    log_info "检查 TLS 捕获状态..."
    local response=$(curl -s -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/status")
    echo "$response" | jq '.'
}

# 启动 TLS 捕获
start_tls_capture() {
    log_info "启动 TLS 捕获..."
    local response=$(curl -s -X POST -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/start")
    echo "$response" | jq '.'
    log_success "TLS 捕获已启动"
}

# 附加默认 TLS 库
attach_defaults() {
    log_info "附加默认 TLS 库 (OpenSSL, GnuTLS, NSS)..."
    local response=$(curl -s -X POST -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/attach-defaults")
    echo "$response" | jq '.'
    log_success "默认库已附加"
}

# 查看已附加的库
list_libraries() {
    log_info "查看已附加的 TLS 库..."
    local response=$(curl -s -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/libraries")
    echo "$response" | jq '.'
}

# 执行测试 HTTPS 请求
test_https_request() {
    local url=$1
    log_info "执行测试请求: ${url}"
    curl -s -o /dev/null "${url}"
    sleep 1  # 等待事件被捕获
    log_success "请求完成"
}

# 查看最近捕获的事件
show_recent_events() {
    local limit=${1:-10}
    log_info "显示最近 ${limit} 个捕获事件..."
    local response=$(curl -s -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/recent?limit=${limit}")
    echo "$response" | jq '.events[] | {
        time: .timestamp,
        pid: .pid,
        process: .comm,
        library: .library,
        direction: .direction,
        method: .method,
        url: .url,
        host: .host,
        status: .status_code,
        plaintext_length: (.plaintext | length)
    }'
}

# 过滤特定进程的事件
show_process_events() {
    local process=$1
    log_info "显示进程 ${process} 的 TLS 事件..."
    local response=$(curl -s -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/recent?limit=100&filter=comm:${process}")
    echo "$response" | jq '.events[] | {
        method: .method,
        url: .url,
        host: .host,
        direction: .direction,
        plaintext_preview: (.plaintext | .[0:200])
    }'
}

# 查看完整明文数据
show_full_plaintext() {
    local limit=${1:-1}
    log_info "显示最近 ${limit} 个事件的完整明文..."
    local response=$(curl -s -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/recent?limit=${limit}")
    echo "$response" | jq -r '.events[] | "
═══════════════════════════════════════════════════════════
时间: \(.timestamp)
进程: \(.comm) (PID: \(.pid))
库: \(.library) | 方向: \(.direction)
请求: \(.method) \(.url)
主机: \(.host)
状态码: \(.status_code)
───────────────────────────────────────────────────────────
明文数据:
\(.plaintext)
═══════════════════════════════════════════════════════════
"'
}

# 演示场景 1：基础 HTTPS 拦截
demo_basic_https() {
    echo ""
    log_info "══════════════════════════════════════════════════"
    log_info "演示场景 1: 基础 HTTPS 拦截 (curl)"
    log_info "══════════════════════════════════════════════════"

    start_tls_capture
    attach_defaults

    log_info "执行 3 个测试请求..."
    test_https_request "https://api.github.com/users/octocat"
    test_https_request "https://httpbin.org/get"
    test_https_request "https://jsonplaceholder.typicode.com/posts/1"

    show_recent_events 5
}

# 演示场景 2：过滤特定进程
demo_process_filter() {
    echo ""
    log_info "══════════════════════════════════════════════════"
    log_info "演示场景 2: 过滤 curl 进程的 TLS 流量"
    log_info "══════════════════════════════════════════════════"

    test_https_request "https://api.github.com/zen"
    show_process_events "curl"
}

# 演示场景 3：查看完整明文
demo_full_plaintext() {
    echo ""
    log_info "══════════════════════════════════════════════════"
    log_info "演示场景 3: 查看完整 HTTP 明文"
    log_info "══════════════════════════════════════════════════"

    test_https_request "https://httpbin.org/headers"
    show_full_plaintext 1
}

# 演示场景 4：Go 程序 TLS 拦截
demo_go_program() {
    echo ""
    log_info "══════════════════════════════════════════════════"
    log_info "演示场景 4: Go 程序 TLS 拦截"
    log_info "══════════════════════════════════════════════════"

    log_info "创建测试 Go 程序..."
    cat > /tmp/tls_test.go <<'EOF'
package main

import (
    "fmt"
    "io"
    "net/http"
)

func main() {
    resp, err := http.Get("https://api.github.com/users/golang")
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
    body, _ := io.ReadAll(resp.Body)
    fmt.Printf("Response length: %d bytes\n", len(body))
}
EOF

    log_info "编译并运行 Go 程序..."
    go build -o /tmp/tls_test /tmp/tls_test.go
    /tmp/tls_test &
    local go_pid=$!

    sleep 2  # 等待自动发现循环检测到进程

    log_info "查看 Go 程序的 TLS 事件..."
    show_process_events "tls_test"

    wait $go_pid 2>/dev/null || true
    rm -f /tmp/tls_test /tmp/tls_test.go
}

# 演示场景 5：Node.js TLS 拦截
demo_node_program() {
    echo ""
    log_info "══════════════════════════════════════════════════"
    log_info "演示场景 5: Node.js TLS 拦截"
    log_info "══════════════════════════════════════════════════"

    if ! command -v node &> /dev/null; then
        log_warn "Node.js 未安装，跳过此演示"
        return
    fi

    log_info "创建测试 Node.js 程序..."
    cat > /tmp/tls_test.js <<'EOF'
const https = require('https');

https.get('https://api.github.com/users/nodejs', (resp) => {
    let data = '';
    resp.on('data', (chunk) => { data += chunk; });
    resp.on('end', () => {
        console.log('Response length:', data.length, 'bytes');
    });
}).on('error', (err) => {
    console.error('Error:', err.message);
});
EOF

    log_info "运行 Node.js 程序..."
    node /tmp/tls_test.js &
    local node_pid=$!

    sleep 3  # 等待自动发现

    log_info "查看 Node.js 的 TLS 事件..."
    show_process_events "node"

    wait $node_pid 2>/dev/null || true
    rm -f /tmp/tls_test.js
}

# 监控模式：持续显示新事件
monitor_mode() {
    log_info "══════════════════════════════════════════════════"
    log_info "监控模式：实时显示 TLS 事件 (Ctrl+C 退出)"
    log_info "══════════════════════════════════════════════════"

    local last_timestamp=0

    while true; do
        local response=$(curl -s -H "X-API-KEY: ${API_KEY}" "${BACKEND_URL}/api/tls-capture/recent?limit=50")
        local events=$(echo "$response" | jq -r ".events[] | select(.timestamp > ${last_timestamp}) | \"\(.timestamp)|\(.comm)|\(.method)|\(.host)\(.url)\"")

        if [ -n "$events" ]; then
            while IFS='|' read -r timestamp comm method url; do
                echo -e "${GREEN}[$(date -d @$((timestamp/1000000000)) '+%H:%M:%S')]${NC} ${BLUE}${comm}${NC} ${YELLOW}${method}${NC} ${url}"
                last_timestamp=$timestamp
            done <<< "$events"
        fi

        sleep 1
    done
}

# 主菜单
show_menu() {
    echo ""
    echo "═════════════════════════════════════════════════════════════"
    echo "  agent-ebpf-filter TLS 拦截功能演示"
    echo "═════════════════════════════════════════════════════════════"
    echo ""
    echo "  1) 检查 TLS 捕获状态"
    echo "  2) 启动 TLS 捕获 + 附加默认库"
    echo "  3) 查看已附加的 TLS 库"
    echo "  4) 演示场景 1: 基础 HTTPS 拦截"
    echo "  5) 演示场景 2: 过滤特定进程"
    echo "  6) 演示场景 3: 查看完整明文"
    echo "  7) 演示场景 4: Go 程序 TLS 拦截"
    echo "  8) 演示场景 5: Node.js TLS 拦截"
    echo "  9) 查看最近事件"
    echo " 10) 实时监控模式"
    echo "  0) 退出"
    echo ""
    echo "═════════════════════════════════════════════════════════════"
}

# 主程序
main() {
    check_backend

    if [ $# -eq 0 ]; then
        # 交互模式
        while true; do
            show_menu
            read -p "请选择操作 (0-10): " choice

            case $choice in
                1) check_tls_status ;;
                2) start_tls_capture; attach_defaults ;;
                3) list_libraries ;;
                4) demo_basic_https ;;
                5) demo_process_filter ;;
                6) demo_full_plaintext ;;
                7) demo_go_program ;;
                8) demo_node_program ;;
                9) show_recent_events 20 ;;
                10) monitor_mode ;;
                0) log_info "退出"; exit 0 ;;
                *) log_error "无效选项" ;;
            esac

            read -p "按 Enter 继续..."
        done
    else
        # 命令行模式
        case "$1" in
            status) check_tls_status ;;
            start) start_tls_capture; attach_defaults ;;
            libraries) list_libraries ;;
            demo1) demo_basic_https ;;
            demo2) demo_process_filter ;;
            demo3) demo_full_plaintext ;;
            demo4) demo_go_program ;;
            demo5) demo_node_program ;;
            recent) show_recent_events "${2:-10}" ;;
            process) show_process_events "${2:-curl}" ;;
            monitor) monitor_mode ;;
            full) demo_basic_https; demo_process_filter; demo_full_plaintext ;;
            *)
                echo "用法: $0 [command]"
                echo ""
                echo "命令:"
                echo "  status       - 检查 TLS 捕获状态"
                echo "  start        - 启动 TLS 捕获"
                echo "  libraries    - 列出已附加的库"
                echo "  demo1        - 基础 HTTPS 拦截演示"
                echo "  demo2        - 进程过滤演示"
                echo "  demo3        - 完整明文查看演示"
                echo "  demo4        - Go 程序演示"
                echo "  demo5        - Node.js 演示"
                echo "  recent [N]   - 显示最近 N 个事件"
                echo "  process [名] - 显示特定进程的事件"
                echo "  monitor      - 实时监控模式"
                echo "  full         - 运行所有演示"
                echo ""
                echo "无参数运行进入交互模式"
                exit 1
                ;;
        esac
    fi
}

main "$@"
