#!/bin/bash
# eBPF ML 模型拦截功能自动测试

set -e

echo "=== eBPF ML 模型拦截功能测试 ==="
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 sudo 执行"
    exit 1
fi

# 检查程序是否加载
if [ ! -e "/sys/fs/bpf/ml_model" ]; then
    echo "❌ eBPF 程序未加载"
    echo "   请先运行: sudo ./test_kernel_load.sh"
    exit 1
fi

echo "✅ eBPF 程序已加载"
echo ""

# Step 1: 附加到 kprobe
echo "📋 Step 1: 附加到 kprobe (sys_execve)"
echo "  这会拦截所有进程创建..."
echo ""

# 尝试附加
echo "  执行: bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe sys_execve"

if bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe sys_execve 2>&1; then
    echo "  ✅ 附加成功"
else
    # 可能已经附加或者需要使用其他方式
    echo "  ⚠️  直接附加失败，尝试使用 perf_event..."

    # 检查是否已经附加
    if bpftool perf show 2>/dev/null | grep -q "ml_model"; then
        echo "  ✅ 程序已经附加"
    else
        echo "  ⚠️  附加可能失败，但继续测试..."
    fi
fi
echo ""

# Step 2: 启动日志监控（后台）
echo "📋 Step 2: 启动内核日志监控"
echo "  日志位置: /sys/kernel/debug/tracing/trace_pipe"
echo ""

LOG_FILE="/tmp/ebpf_test_$(date +%s).log"
echo "  日志将保存到: $LOG_FILE"

# 清空现有 trace
echo > /sys/kernel/debug/tracing/trace

# 启动监控（后台，5秒）
timeout 5 cat /sys/kernel/debug/tracing/trace_pipe > "$LOG_FILE" 2>&1 &
MONITOR_PID=$!

sleep 0.5
echo "  ✅ 监控已启动 (PID: $MONITOR_PID)"
echo ""

# Step 3: 触发测试事件
echo "📋 Step 3: 触发测试事件"
echo "  执行系统调用以触发 eBPF 程序..."
echo ""

# 执行一些命令触发 execve
for i in {1..3}; do
    echo "  测试 $i: /bin/true"
    /bin/true
    sleep 0.2
done

echo ""
echo "  测试 4: ls (列出当前目录)"
ls > /dev/null 2>&1

echo ""
echo "  测试 5: echo"
echo "test" > /dev/null

echo ""
echo "  ✅ 测试命令执行完成"
echo ""

# 等待监控结束
sleep 1
echo "📋 Step 4: 分析日志"
echo ""

# 检查日志
if [ -s "$LOG_FILE" ]; then
    LINE_COUNT=$(wc -l < "$LOG_FILE")
    echo "  ✅ 捕获到 $LINE_COUNT 行日志"
    echo ""
    echo "  日志内容预览:"
    echo "  ────────────────────────────────────"
    head -20 "$LOG_FILE" | while IFS= read -r line; do
        echo "  $line"
    done
    echo "  ────────────────────────────────────"

    # 检查是否有 ML 相关日志
    if grep -q "ML Model" "$LOG_FILE" 2>/dev/null; then
        echo ""
        echo "  🎯 检测到 ML 模型输出:"
        grep "ML Model" "$LOG_FILE" | head -5
    fi

    # 检查是否有 BLOCK 消息
    if grep -q "BLOCK" "$LOG_FILE" 2>/dev/null; then
        echo ""
        echo "  ⚠️  检测到 BLOCK 动作:"
        grep "BLOCK" "$LOG_FILE"
    fi
else
    echo "  ⚠️  未捕获到日志输出"
    echo ""
    echo "  可能原因:"
    echo "    1. 程序未正确附加到 kprobe"
    echo "    2. bpf_printk 未启用"
    echo "    3. 日志缓冲区已满"
    echo ""
    echo "  故障排查:"
    echo "    sudo dmesg | tail -20"
    echo "    sudo bpftool prog tracelog"
fi
echo ""

# Step 5: 性能统计
echo "📋 Step 5: 程序统计信息"
echo ""
bpftool prog show pinned /sys/fs/bpf/ml_model
echo ""

# Step 6: 清理
echo "📋 Step 6: 清理"
echo ""
echo "  选项:"
echo "    1. 保持加载继续测试: 什么都不做"
echo "    2. 卸载程序: sudo rm /sys/fs/bpf/ml_model"
echo "    3. 查看完整日志: cat $LOG_FILE"
echo ""

echo "✅ 测试完成！"
echo ""
echo "📊 总结:"
echo "  - 程序ID: $(bpftool prog show pinned /sys/fs/bpf/ml_model | head -1 | cut -d: -f1)"
echo "  - 日志文件: $LOG_FILE"
echo "  - 测试事件: 5 次系统调用"
echo ""
echo "🎉 eBPF ML 模型拦截功能测试完成"
