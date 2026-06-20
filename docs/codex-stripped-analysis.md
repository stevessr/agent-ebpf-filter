# Codex Stripped Binary 深度分析报告

## 🔍 问题诊断

### 二进制特征
```bash
$ file codex
codex: ELF 64-bit LSB pie executable, x86-64, 
       version 1 (SYSV), static-pie linked, stripped
```

**关键特征**:
- ✅ **静态链接 OpenSSL**: 字符串中包含 `SSL_write`/`SSL_read`
- ❌ **Stripped**: 无符号表，无法通过函数名 attach uprobe
- ❌ **Static-PIE**: 无动态链接，无法使用 LD_PRELOAD

### SSL 函数验证
```bash
$ strings codex | grep "^SSL_.*write\|^SSL_.*read"
SSL_read
SSL_read_early_data
SSL_write
SSL_write_early_data
```

**结论**: OpenSSL 已静态编译进二进制，但符号表被剥离。

---

## 🎯 解决方案矩阵

| 方案 | 可行性 | 精度 | 性能 | 复杂度 | 推荐度 |
|------|--------|------|------|--------|--------|
| **A. Syscall Tracing** | ✅ 高 | 中 | 高 | 低 | ⭐⭐⭐⭐⭐ |
| **B. Runtime Probing** | ✅ 中 | 高 | 中 | 中 | ⭐⭐⭐ |
| **C. Debug Symbols** | ⚠️ 低 | 高 | 高 | 低 | ⭐⭐ |
| **D. Offset Finding** | ❌ 低 | 高 | 高 | 极高 | ⭐ |

---

## 💡 推荐方案: Syscall-Level Tracing

### 原理
在系统调用层面拦截 Codex 的网络 I/O，无需符号表。

### 优势
1. **无需符号**: 适用所有 stripped binary
2. **高性能**: kprobe 开销极小
3. **可靠**: 基于内核稳定接口
4. **简单**: 复用现有 eBPF 基础设施

### 架构
```
Codex (stripped binary)
    ↓ SSL_write (内部，无法 attach)
    ↓ write() syscall
    ↓ 【kprobe 拦截点】← 我们在这里捕获
    ↓ 内核网络栈
    ↓ TLS/TCP
```

---

## 🛠️ 实现方案

### 方案 1: 扩展现有 syscall 跟踪 (最简单)

项目已有 `backend/ebpf/agent_tracker.c` 跟踪 syscalls，仅需添加过滤：

```c
// backend/ebpf/codex_syscall_filter.h

static __always_inline bool is_codex_process() {
    char comm[16];
    bpf_get_current_comm(&comm, sizeof(comm));
    return comm[0] == 'c' && comm[1] == 'o' && 
           comm[2] == 'd' && comm[3] == 'e' && 
           comm[4] == 'x';
}

static __always_inline bool is_https_socket(int fd) {
    // 简化版: 检查 fd 是否指向 socket
    // 实际需要通过 bpf_probe_read 获取 socket 结构
    return true; // 占位
}

// 在 existing kprobe/sys_write 中添加
SEC("kprobe/sys_write")
int trace_codex_write(struct pt_regs *ctx) {
    if (!is_codex_process())
        return 0;
    
    int fd = PT_REGS_PARM1(ctx);
    if (!is_https_socket(fd))
        return 0;
    
    // 捕获数据 (复用 TLS 事件结构)
    ...
}
```

### 方案 2: 独立 Codex 跟踪模块 (更精确)

创建 `backend/ebpf/codex_tracker.c`:

```c
//go:build ignore

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

char LICENSE[] SEC("license") = "GPL";

// 复用 TLS fragment 结构
struct codex_data_fragment {
    __u64 timestamp_ns;
    __u32 pid;
    __u32 tid;
    __u32 data_len;
    __u8 direction; // 0=read, 1=write
    char comm[16];
    char data[960];
};

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, 256 * 1024);
} codex_events SEC(".maps");

SEC("kprobe/sys_write")
int kprobe_codex_write(struct pt_regs *ctx) {
    char comm[16];
    bpf_get_current_comm(&comm, sizeof(comm));
    
    // 快速路径: 首字符检查
    if (comm[0] != 'c')
        return 0;
    
    // 完整比较
    if (comm[0] != 'c' || comm[1] != 'o' || comm[2] != 'd' ||
        comm[3] != 'e' || comm[4] != 'x' || comm[5] != '\0')
        return 0;
    
    int fd = (int)PT_REGS_PARM1(ctx);
    const char *buf = (const char *)PT_REGS_PARM2(ctx);
    size_t count = (size_t)PT_REGS_PARM3(ctx);
    
    if (count == 0 || count > 960)
        return 0;
    
    struct codex_data_fragment *frag = bpf_ringbuf_reserve(&codex_events, sizeof(*frag), 0);
    if (!frag)
        return 0;
    
    frag->timestamp_ns = bpf_ktime_get_ns();
    frag->pid = bpf_get_current_pid_tgid() >> 32;
    frag->tid = bpf_get_current_pid_tgid();
    frag->data_len = count > 960 ? 960 : count;
    frag->direction = 1; // write
    bpf_probe_read_user(frag->data, frag->data_len, buf);
    bpf_get_current_comm(&frag->comm, sizeof(frag->comm));
    
    bpf_ringbuf_submit(frag, 0);
    return 0;
}

SEC("kprobe/sys_read")
int kprobe_codex_read(struct pt_regs *ctx) {
    // 类似 write，direction = 0
    ...
}
```

### 方案 3: 端口过滤增强 (生产级)

添加 socket 信息提取:

```c
// 从 fd 获取 socket 信息
static __always_inline bool is_tls_socket(int fd) {
    // 通过 BPF helper 获取 socket
    struct socket *sock = sockfd_lookup(fd);
    if (!sock)
        return false;
    
    struct sock *sk = sock->sk;
    if (!sk)
        return false;
    
    // 检查端口
    __u16 dport = sk->__sk_common.skc_dport;
    dport = bpf_ntohs(dport);
    
    return dport == 443 || dport == 8443;
}
```

---

## 📊 对比: Uprobe vs Kprobe

### Claude Code (Node.js)
```
✅ Uprobe 方案:
   SSL_write() ← attach 这里
      ↓
   write()
      ↓
   syscall
```

### Codex (stripped)
```
❌ Uprobe 方案:
   SSL_write() ← 无符号，无法 attach
      ↓
✅ Kprobe 方案:
   write() ← attach 这里
      ↓
   syscall
```

---

## 🎯 最终推荐

### 当前阶段
**使用 Syscall Tracing**，原因：
1. 已有基础设施 (`agent_tracker.c`)
2. 仅需添加进程过滤
3. 适用所有 stripped binaries
4. 性能开销可接受

### 实现优先级
1. ✅ **阶段 1**: 添加 comm 过滤 (10 行代码)
2. ✅ **阶段 2**: 添加端口过滤 (50 行代码)
3. ⚠️ **阶段 3**: 完整 TLS 解析 (可选)

### 未来优化
如果 OpenAI 提供 debug symbols:
- 切换到 uprobe 方案
- 获得函数级粒度
- 降低误报率

---

## 📝 代码差异对比

### Node.js (符号可用)
```go
// uprobe 直接附加到 SSL_write
m.AttachStaticSSLUprobes(binPath, pid)
```

### Codex (stripped)
```go
// syscall 层捕获，comm 过滤
m.AttachCodexSyscallTracer(pid)
```

---

## ✅ 结论

**Codex stripped binary 的最佳方案是 Syscall-Level Tracing**

- 无需修改 Codex 二进制
- 无需符号表或偏移查找
- 复用现有 eBPF 基础设施
- 适用于所有 AI 工具的 stripped 版本

**实现成本**: ~100 行 eBPF C 代码 + ~50 行 Go 代码

**预期效果**: 可捕获 Codex 的所有 HTTPS 流量，精度略低于 uprobe 但完全可用。

---

## 相关导航

- [Codex workflows](codex-workflows.md)
- [Codex implementation complete](codex-implementation-complete.md)
- [Codex rustls 修复](codex-rustls-fix.md)
- [TLS / Codex 捕获与脱敏边界](security/redaction-privacy.md)
