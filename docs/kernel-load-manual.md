# eBPF ML 模型内核加载与拦截测试手册

**目标**: 将 eBPF 程序加载到内核并测试进程拦截功能

---

## 📋 **前置要求**

### **1. 环境检查**
```bash
# 检查内核版本 (需要 5.10+)
uname -r

# 检查 bpftool
bpftool --version

# 检查权限
sudo -v
```

### **2. 文件准备**
```bash
cd backend
ls -lh ml_model_ebpf.o  # 应该看到 48.2 KB 的文件
```

---

## 🚀 **加载步骤**

### **Step 1: 加载程序到内核**

```bash
# 使用自动化脚本（推荐）
cd backend
sudo ./test_kernel_load.sh
```

**或手动执行**:
```bash
# 清理旧程序（如果存在）
sudo rm -f /sys/fs/bpf/ml_model

# 加载新程序
sudo bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model

# 验证加载
sudo bpftool prog show | grep ml_predict
ls -la /sys/fs/bpf/ml_model
```

---

### **Step 2: 验证程序信息**

```bash
# 查看所有 BPF 程序
sudo bpftool prog show

# 查看程序详情
sudo bpftool prog show pinned /sys/fs/bpf/ml_model

# 查看 Map
sudo bpftool map show
```

**预期输出**:
```
XXX: kprobe  name ml_predict_syscall  tag <hash>  gpl
        loaded_at <time>  uid 0
        xlated 41304B  jited 24576B  memlock 45056B  map_ids YYY
```

---

### **Step 3: 附加到 Kprobe (可选)**

⚠️ **警告**: 这会拦截所有 `execve` 系统调用，可能影响系统性能

```bash
# 附加到 sys_execve
sudo bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe sys_execve

# 或使用 perf
sudo perf probe -a sys_execve
sudo bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe p_sys_execve_0
```

---

### **Step 4: 监控内核日志**

**终端 1**: 持续监控
```bash
sudo cat /sys/kernel/debug/tracing/trace_pipe
```

**终端 2**: 触发事件
```bash
# 执行任意命令触发 execve
ls
echo "hello"
/bin/true
```

**预期日志**:
```
<...>-12345 [000] .... 12345.678: bpf_trace_printk: ML Model: BLOCK action detected
```

---

### **Step 5: 测试拦截功能**

#### **方法 A: 观察日志**
```bash
# 终端 1
sudo cat /sys/kernel/debug/tracing/trace_pipe

# 终端 2
for i in {1..5}; do ls > /dev/null; done
```

#### **方法 B: 统计事件**
```bash
# 查看 Map 内容（如果有统计）
sudo bpftool map dump name feature_map
```

#### **方法 C: 性能测试**
```bash
# 测试延迟影响
time for i in {1..1000}; do /bin/true; done

# 对比: 卸载 BPF 程序后再测一次
sudo rm /sys/fs/bpf/ml_model
time for i in {1..1000}; do /bin/true; done
```

---

## 🧪 **测试场景**

### **场景 1: 基础功能测试**

```bash
# 1. 加载程序
sudo bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model

# 2. 验证加载成功
sudo bpftool prog show | grep ml_predict

# 3. 检查 Map
sudo bpftool map show

# 4. 查看程序统计
sudo bpftool prog show pinned /sys/fs/bpf/ml_model --json | jq .
```

---

### **场景 2: 拦截测试（高级）**

```bash
# 1. 附加到 kprobe
sudo bpftool prog attach pinned /sys/fs/bpf/ml_model kprobe sys_execve

# 2. 开始监控
sudo cat /sys/kernel/debug/tracing/trace_pipe &

# 3. 创建测试进程
ls
python3 -c "print('test')"
/bin/echo "hello"

# 4. 观察输出
# 应该看到 "ML Model: BLOCK action detected" (如果模型预测为 BLOCK)

# 5. 停止监控
killall cat
```

---

### **场景 3: 性能测试**

```bash
# 1. 创建性能测试脚本
cat > perf_test.sh << 'EOF'
#!/bin/bash
COUNT=10000
echo "测试 $COUNT 次 execve..."
time for i in $(seq 1 $COUNT); do
    /bin/true
done
EOF
chmod +x perf_test.sh

# 2. 无 BPF 基准测试
./perf_test.sh
# 记录时间

# 3. 加载 BPF 后测试
sudo bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model
./perf_test.sh
# 对比时间差异

# 4. 计算开销
# 开销 = (BPF 时间 - 基准时间) / 基准时间 * 100%
```

---

## 🔍 **故障排查**

### **问题 1: 加载失败**

**错误**: `libbpf: failed to load program`

**排查**:
```bash
# 详细模式加载
sudo bpftool -d prog load ml_model_ebpf.o /sys/fs/bpf/ml_model

# 检查 verifier 日志
sudo dmesg | tail -50

# 检查内核配置
zcat /proc/config.gz | grep BPF
```

**常见原因**:
- 内核版本过低 (<5.10)
- BPF 功能未启用
- 程序复杂度超限
- Map 定义问题

---

### **问题 2: 无法附加到 kprobe**

**错误**: `Error: failed to attach program`

**解决**:
```bash
# 检查 kprobe 是否存在
cat /sys/kernel/debug/tracing/available_filter_functions | grep sys_execve

# 使用 tracepoint 替代
sudo bpftool prog attach pinned /sys/fs/bpf/ml_model \
    tracepoint syscalls/sys_enter_execve
```

---

### **问题 3: 看不到日志输出**

**排查**:
```bash
# 检查 trace_pipe 权限
ls -la /sys/kernel/debug/tracing/trace_pipe

# 启用 tracing
sudo echo 1 > /sys/kernel/debug/tracing/tracing_on

# 检查 bpf_printk 是否启用
cat /sys/kernel/debug/tracing/trace_options | grep print-parent
```

---

## 🧹 **清理**

### **卸载程序**

```bash
# 删除 pinned 程序
sudo rm /sys/fs/bpf/ml_model

# 验证卸载
sudo bpftool prog show | grep ml_predict
# 应该没有输出

# 清理 kprobe（如果附加了）
sudo perf probe -d 'p:kprobes/sys_execve*'
```

### **完全重置**

```bash
# 卸载所有自定义 BPF 程序
sudo rm -f /sys/fs/bpf/ml_model*

# 清空 trace 缓冲区
sudo echo > /sys/kernel/debug/tracing/trace

# 重启系统（彻底清理）
sudo reboot
```

---

## 📊 **预期结果**

### **成功加载**
```
✅ 程序加载: /sys/fs/bpf/ml_model 存在
✅ 程序类型: kprobe
✅ 程序大小: ~48 KB
✅ Map 创建: feature_map
✅ 许可证: GPL
```

### **功能测试**
```
✅ 附加成功: 无错误
✅ 日志输出: 看到 bpf_trace_printk 消息
✅ 进程创建: 正常执行
✅ 性能影响: <5% 额外开销
```

### **性能指标**
```
预测延迟: ~2-5 μs (理论)
吞吐量: >100K QPS
CPU 占用: <1%
内存占用: 28 KB
```

---

## ⚠️ **安全提示**

1. **测试环境**: 建议在虚拟机或测试机器上测试
2. **性能影响**: 附加后会影响所有进程创建
3. **及时卸载**: 测试完成后及时清理
4. **备份数据**: 内核模块可能导致系统不稳定
5. **监控日志**: 持续观察系统日志

---

## 📚 **参考命令**

```bash
# 常用命令快速参考
sudo bpftool prog show                          # 列出所有程序
sudo bpftool prog dump xlated pinned <path>     # 查看翻译后的字节码
sudo bpftool prog dump jited pinned <path>      # 查看 JIT 编译后的代码
sudo bpftool map show                           # 列出所有 Map
sudo bpftool map dump name <name>               # 查看 Map 内容
sudo cat /sys/kernel/debug/tracing/trace_pipe  # 查看内核日志
sudo dmesg | tail -50                           # 查看系统日志
```

---

## 🎯 **下一步**

1. 成功加载后，进入 Phase 2: 性能测试
2. 测量实际延迟和吞吐量
3. 对比用户态实现
4. 生成性能报告

---

*手册版本: 1.0*  
*更新时间: 2026-06-07*
