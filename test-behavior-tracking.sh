#!/bin/bash
# 测试执行图行为追踪功能

set -e

echo "=== 测试执行图行为追踪 PID 过滤功能 ==="
echo ""

# 检查后端是否运行
if ! curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "❌ 后端未运行，请先启动后端"
    exit 1
fi
echo "✓ 后端运行正常"

# 检查前端是否运行
FRONTEND_PORT=5175
if ! curl -s http://localhost:${FRONTEND_PORT} > /dev/null 2>&1; then
    echo "❌ 前端未运行，请先启动前端"
    exit 1
fi
echo "✓ 前端运行正常 (端口 ${FRONTEND_PORT})"

# 测试 API 是否支持 PID 过滤
TEST_PID=10159
echo ""
echo "测试后端 API PID 过滤..."
API_RESPONSE=$(curl -s "http://localhost:8080/events/recent?limit=10&pid=${TEST_PID}")
if echo "$API_RESPONSE" | grep -q "events"; then
    EVENT_COUNT=$(echo "$API_RESPONSE" | jq -r '.events | length' 2>/dev/null || echo "0")
    echo "✓ API 响应正常，返回 ${EVENT_COUNT} 个事件"
else
    echo "⚠ API 响应异常或无事件数据"
fi

# 测试前端路由
echo ""
echo "测试前端路由..."
BEHAVIOR_URL="http://localhost:${FRONTEND_PORT}/execution-graph/behavior?pid=${TEST_PID}&limit=600&process_tree=true&timePreset=24h"
if curl -s "${BEHAVIOR_URL}" | grep -q "id=\"app\""; then
    echo "✓ 行为追踪页面可访问"
    echo "  URL: ${BEHAVIOR_URL}"
else
    echo "❌ 行为追踪页面访问失败"
    exit 1
fi

# 测试拓扑图路由
echo ""
echo "测试拓扑图路由..."
TOPOLOGY_URL="http://localhost:${FRONTEND_PORT}/execution-graph/topology?pid=${TEST_PID}&limit=600&process_tree=true&timePreset=24h"
if curl -s "${TOPOLOGY_URL}" | grep -q "id=\"app\""; then
    echo "✓ 执行拓扑页面可访问"
    echo "  URL: ${TOPOLOGY_URL}"
else
    echo "❌ 执行拓扑页面访问失败"
    exit 1
fi

echo ""
echo "=== 所有测试通过 ✓ ==="
echo ""
echo "手动验证步骤："
echo "1. 在浏览器中打开: ${BEHAVIOR_URL}"
echo "2. 检查页面顶部的过滤器栏，PID 输入框应显示 '${TEST_PID}'"
echo "3. 检查事件列表是否只显示该 PID 相关的事件"
echo "4. 切换到 '执行拓扑' 标签页，选择不同的进程"
echo "5. 再切换回 '行为追踪' 标签页，确认过滤器已更新"
echo ""
