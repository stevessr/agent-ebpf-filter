#!/bin/bash
# eBPF ML 模型内核加载与测试脚本
# 需要 root 权限执行

set -e

echo "=== eBPF ML 模型内核加载测试 ==="
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 sudo 执行此脚本"
    echo "   sudo ./test_kernel_load.sh"
    exit 1
fi

echo "✅ root 权限确认"
echo ""

# Step 1: 环境检查
echo "📋 Step 1: 环境检查"

# 检查内核版本
KERNEL_VERSION=$(uname -r)
echo "  内核版本：$KERNEL_VERSION"

KERNEL_MAJOR=$(echo $KERNEL_VERSION | cut -d. -f1)
KERNEL_MINOR=$(echo $KERNEL_VERSION | cut -d. -f2)

if [ "$KERNEL_MAJOR" -lt 5 ] || ([ "$KERNEL_MAJOR" -eq 5 ] && [ "$KERNEL_MINOR" -lt 10 ]); then
    echo "  ⚠️  内核版本较低，建议 5.10+"
else
    echo "  ✅ 内核版本符合要求"
fi

# 检查 bpftool
if ! command -v bpftool &> /dev/null; then
    echo "  ❌ bpftool 未安装"
    exit 1
fi
echo "  ✅ bpftool: $(bpftool --version | head -1)"

# 检查字节码文件
if [ ! -f "ml_model_ebpf.o" ]; then
    echo "  ❌ ml_model_ebpf.o 不存在"
    exit 1
fi
echo "  ✅ 字节码文件：$(ls -lh ml_model_ebpf.o | awk '{print $5}')"
echo ""

# Step 2: 清理旧程序
echo "📋 Step 2: 清理旧程序"
if [ -e "/sys/fs/bpf/ml_model" ]; then
    echo "  发现旧程序，清理中..."
    rm -f /sys/fs/bpf/ml_model
    echo "  ✅ 清理完成"
else
    echo "  ✅ 无需清理"
fi
echo ""

# Step 3: 加载程序
echo "📋 Step 3: 加载 eBPF 程序到内核"
echo "  执行：bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model"

if bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model 2>&1; then
    echo "  ✅ 加载成功"
else
    echo "  ❌ 加载失败"
    echo ""
    echo "  尝试详细模式..."
    bpftool -d prog load ml_model_ebpf.o /sys/fs/bpf/ml_model 2>&1 || true
    exit 1
fi
echo ""

# Step 4: 验证加载
echo "📋 Step 4: 验证程序已加载"

if [ -e "/sys/fs/bpf/ml_model" ]; then
    echo "  ✅ 程序已 pin 到 /sys/fs/bpf/ml_model"
else
    echo "  ❌ 未找到 pinned 文件"
    exit 1
fi

# 显示程序信息
echo "  程序列表："
bpftool prog show | grep -A 5 "ml_predict" || bpftool prog show | tail -6
echo ""

# Step 5: 创建测试进程并拦截
echo "📋 Step 5: 测试拦截功能"
echo ""
echo "  当前程序状态：已加载但未附加"
echo "  要启用拦截，需要手动附加到 kprobe:"
echo ""
echo "    # 附加到 sys_execve (拦截所有进程创建)"
echo "    sudo bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe sys_execve"
echo ""
echo "    # 查看内核日志"
echo "    sudo cat /sys/kernel/debug/tracing/trace_pipe"
echo ""
echo "    # 在另一个终端测试"
echo "    ls  # 会触发 execve"
echo ""
echo "  ⚠️  注意：附加后会影响所有进程创建，测试完记得卸载"
echo ""

# Step 6: 提供清理命令
echo "📋 Step 6: 清理命令"
echo "  卸载程序："
echo "    sudo rm /sys/fs/bpf/ml_model"
echo ""
echo "  查看当前程序："
echo "    sudo bpftool prog show"
echo ""

echo "✅ 程序加载完成！"
echo ""
echo "🎉 eBPF ML 模型已成功加载到内核"
echo "   文件位置：/sys/fs/bpf/ml_model"
echo "   程序类型：kprobe"
echo "   入口函数：ml_predict_syscall"
