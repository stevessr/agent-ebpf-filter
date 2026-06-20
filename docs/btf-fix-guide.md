# eBPF BTF 问题解决方案

**问题**: `libbpf: BTF is required, but is missing or corrupted.`

---

## 🔍 **根本原因**

现代 Linux 内核 (5.10+) 和 bpftool 默认要求 eBPF 程序包含 BTF (BPF Type Format) 信息。

BTF 提供类型信息，用于：
- 程序验证
- Map 共享
- CO-RE (Compile Once, Run Everywhere)
- 调试和 introspection

---

## ✅ **解决方案**

### **方案 1: 重新编译带 BTF** (推荐)

```bash
cd backend

# 使用 -g 标志编译（生成调试信息，包括 BTF）
clang -O2 -target bpf -g -c ml_model_ebpf.c -o ml_model_ebpf.o

# 验证 BTF 信息
bpftool btf dump file ml_model_ebpf.o | head -20
```

**效果**:
- ✅ 包含完整 BTF 信息
- ⚠️ 文件变大 (48.2 KB → 146.5 KB)
- ✅ 内核可以成功加载

---

### **方案 2: 使用自动化脚本**

```bash
cd backend
sudo ./test_kernel_load_btf.sh
```

脚本会：
1. ✅ 检测 BTF 信息
2. ✅ 自动重新编译（如果需要）
3. ✅ 加载到内核
4. ✅ 验证加载结果

---

### **方案 3: 优化 BTF 大小**

```bash
# 编译
clang -O2 -target bpf -g -c ml_model_ebpf.c -o ml_model_ebpf.o

# 剥离不必要的调试信息（保留 BTF）
llvm-strip --strip-unneeded ml_model_ebpf.o

# 检查大小
ls -lh ml_model_ebpf.o
```

**预期**:
- 从 146.5 KB 减少到 ~80-100 KB
- 保留 BTF，移除其他调试信息

---

## 📊 **文件大小对比**

| 版本 | 大小 | BTF | 说明 |
|:-----|:----:|:---:|:-----|
| **原始** | 48.2 KB | ❌ | 无 BTF，无法加载 |
| **带 BTF** | 146.5 KB | ✅ | 完整调试信息 |
| **优化后** | ~90 KB | ✅ | 保留 BTF，移除其他 |

---

## 🔧 **更新编译器**

更新 `compile_ebpf_model.go` 以默认生成 BTF：

```bash
# 在脚本末尾添加
echo "Compiling with BTF..."
clang -O2 -target bpf -g -c ml_model_ebpf.c -o ml_model_ebpf.o
llvm-strip --strip-unneeded ml_model_ebpf.o
```

---

## 📋 **验证步骤**

```bash
# 1. 检查内核 BTF 支持
ls -lh /sys/kernel/btf/vmlinux

# 2. 重新编译
clang -O2 -target bpf -g -c ml_model_ebpf.c -o ml_model_ebpf.o

# 3. 验证 BTF
bpftool btf dump file ml_model_ebpf.o | head

# 4. 加载
sudo bpftool prog load ml_model_ebpf.o /sys/fs/bpf/ml_model

# 5. 验证加载
sudo bpftool prog show | grep ml_predict
ls -la /sys/fs/bpf/ml_model
```

---

## 🎯 **现在执行**

```bash
cd backend
sudo ./test_kernel_load_btf.sh
```

脚本会自动处理所有问题！

---

## 相关导航

- [eBPF 与 OS Enforcement](backend/ebpf-os-enforcement.md)
- [Kernel load manual](kernel-load-manual.md)
- [验证、测试与 Benchmark](operations/verification-benchmark.md)
- [构建与运行](operations/build-and-run.md)
