#!/bin/bash
# eBPF ML 模型内核加载与测试脚本 - BTF 兼容版本
# 需要 root 权限执行

set -e

echo "=== eBPF ML 模型内核加载测试 (BTF 兼容版) ==="
echo ""

# 检查 root 权限
if [ "$EUID" -ne 0 ]; then
    echo "❌ 请使用 sudo 执行此脚本"
    echo "   sudo ./test_kernel_load_btf.sh"
    exit 1
fi

echo "✅ root 权限确认"
echo ""

# Step 1: 环境检查
echo "📋 Step 1: 环境检查"

KERNEL_VERSION=$(uname -r)
echo "  内核版本：$KERNEL_VERSION"

# 检查 BTF 支持
if [ -f "/sys/kernel/btf/vmlinux" ]; then
    echo "  ✅ 内核 BTF: $(ls -lh /sys/kernel/btf/vmlinux | awk '{print $5}')"
else
    echo "  ⚠️  内核 BTF 不可用"
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

# Step 2: 检查 BTF 信息
echo "📋 Step 2: 检查 BTF 信息"

# 尝试检查 BTF
if bpftool btf dump file ml_model_ebpf.o &> /dev/null; then
    echo "  ✅ 字节码包含 BTF 信息"
    BTF_SIZE=$(bpftool btf dump file ml_model_ebpf.o | wc -l)
    echo "  BTF 条目：$BTF_SIZE 行"
else
    echo "  ⚠️  字节码缺少 BTF 信息"
    echo "  解决方案：使用 -g 标志重新编译"
    echo ""
    echo "  重新编译..."
    if [ -f "ml_model_ebpf.c" ]; then
        clang -O2 -target bpf -g -c ml_model_ebpf.c -o ml_model_ebpf.o
        echo "  ✅ 重新编译完成"
        echo "  新文件大小：$(ls -lh ml_model_ebpf.o | awk '{print $5}')"
    else
        echo "  ❌ ml_model_ebpf.c 不存在"
        exit 1
    fi
fi
echo ""

# Step 3: 清理旧程序
echo "📋 Step 3: 清理旧程序"
if [ -e "/sys/fs/bpf/ml_model" ]; then
    echo "  发现旧程序，清理中..."
    rm -f /sys/fs/bpf/ml_model
    echo "  ✅ 清理完成"
else
    echo "  ✅ 无需清理"
fi
echo ""

# Step 4: 加载程序
echo "📋 Step 4: 加载 eBPF 程序到内核"
echo "  执行：bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model"
echo ""

# 尝试加载
if bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model 2>&1; then
    echo ""
    echo "  ✅ 加载成功"
else
    EXIT_CODE=$?
    echo ""
    echo "  ⚠️  加载可能遇到问题"
    echo ""

    # 检查是否实际加载成功
    if [ -e "/sys/fs/bpf/ml_model" ]; then
        echo "  ✅ 但程序文件已创建，可能加载成功"
    else
        echo "  ❌ 加载确实失败"
        echo ""
        echo "  故障排查："
        echo "    1. 检查 dmesg: sudo dmesg | tail -20"
        echo "    2. 详细模式：bpftool -d prog load ml_model_ebpf.o /sys/fs/bpf/ml_model"
        echo "    3. 验证器日志：查看 dmesg 中的 BPF verifier 输出"
        exit 1
    fi
fi
echo ""

# Step 5: 验证加载
echo "📋 Step 5: 验证程序已加载"

if [ -e "/sys/fs/bpf/ml_model" ]; then
    echo "  ✅ 程序已 pin 到 /sys/fs/bpf/ml_model"
    ls -la /sys/fs/bpf/ml_model
else
    echo "  ❌ 未找到 pinned 文件"
    exit 1
fi
echo ""

# 显示程序信息
echo "  已加载的 BPF 程序："
bpftool prog show | tail -10
echo ""

# Step 6: 程序详情
echo "📋 Step 6: 程序详情"
echo "  查看程序详细信息..."
bpftool prog show pinned /sys/fs/bpf/ml_model
echo ""

# Step 7: Map 信息
echo "📋 Step 7: BPF Map 信息"
echo "  已创建的 Map:"
bpftool map show
echo ""

# Step 8: 使用说明
echo "📋 Step 8: 后续操作"
echo ""
echo "  ✅ 程序已成功加载到内核！"
echo ""
echo "  要启用拦截功能："
echo "    1. 附加到 kprobe:"
echo "       sudo bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe sys_execve"
echo ""
echo "    2. 查看内核日志："
echo "       sudo cat /sys/kernel/debug/tracing/trace_pipe"
echo ""
echo "    3. 在另一个终端测试："
echo "       ls  # 会触发 execve 系统调用"
echo ""
echo "  清理程序："
echo "    sudo rm /sys/fs/bpf/ml_model"
echo ""

echo "🎉 测试完成！"
