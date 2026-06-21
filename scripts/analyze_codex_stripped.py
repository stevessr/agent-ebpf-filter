#!/usr/bin/env python3
"""
Codex Stripped Binary 分析报告
"""

import subprocess
import sys

def main():
    codex_bin = sys.argv[1] if len(sys.argv) > 1 else "codex"

    print("=" * 70)
    print("Codex Stripped Binary Analysis")
    print("=" * 70)
    print()

    # 1. 二进制信息
    print("1. Binary Info:")
    result = subprocess.run(['file', codex_bin], capture_output=True, text=True)
    print(f"   {result.stdout.strip()}")
    print()

    # 2. SSL 字符串验证
    print("2. SSL Functions (via strings):")
    result = subprocess.run(['strings', codex_bin], capture_output=True, text=True)
    ssl_funcs = sorted(set([line for line in result.stdout.split('\n')
                            if line.startswith('SSL_') and ('write' in line or 'read' in line)]))
    for func in ssl_funcs[:10]:
        print(f"   ✓ {func}")
    print()

    # 3. 问题
    print("3. Problem:")
    print("   ❌ Binary is stripped - no symbol table")
    print("   ❌ Cannot attach uprobe by function name")
    print("   ❌ Static-pie linked - no LD_PRELOAD hooks")
    print()

    # 4. 解决方案
    print("4. Solutions:")
    print()
    print("   【方案 A】Syscall-level tracing (推荐)")
    print("   ├─ 跟踪 write/read/sendto/recvfrom 系统调用")
    print("   ├─ 过滤 Codex 进程 (comm == 'codex')")
    print("   ├─ 根据端口/地址识别 HTTPS 流量")
    print("   └─ 优点：无需符号表，适用所有 stripped binary")
    print()
    print("   【方案 B】bpftrace runtime probing")
    print("   ├─ 使用 bpftrace 在运行时附加")
    print("   ├─ 自动查找函数地址")
    print("   └─ 需要 root 权限")
    print()
    print("   【方案 C】Request debug symbols from OpenAI")
    print("   └─ 联系 OpenAI 获取带符号的 debug 版本")
    print()

    # 5. 实现建议
    print("5. Implementation Recommendation:")
    print()
    print("   对于当前项目：")
    print("   ✓ Claude Code (Node.js) - 使用 uprobe (符号可用)")
    print("   ✓ Codex (stripped) - 使用 kprobe on syscalls")
    print()
    print("   在 backend/ebpf/agent_tracker.c 中：")
    print("   - 已有 write/read 等 syscall 的 kprobe")
    print("   - 添加端口过滤 (443, 8443)")
    print("   - 添加进程过滤 (comm == 'codex')")
    print()

    # 6. 示例代码
    print("6. Example eBPF filter (pseudo-code):")
    print()
    print("""
    SEC("kprobe/sys_write")
    int trace_write(struct pt_regs *ctx) {
        char comm[16];
        bpf_get_current_comm(&comm, sizeof(comm));

        // 仅跟踪 codex 进程
        if (strcmp(comm, "codex") != 0)
            return 0;

        int fd = PT_REGS_PARM1(ctx);

        // 检查是否是 socket
        struct socket *sock = get_socket_from_fd(fd);
        if (!sock)
            return 0;

        // 检查是否是 HTTPS 端口
        __u16 dport = get_dest_port(sock);
        if (dport != 443 && dport != 8443)
            return 0;

        // 捕获数据
        ...
    }
    """)
    print()

    print("=" * 70)
    print("Conclusion: Use syscall-level tracing for stripped binaries")
    print("=" * 70)

if __name__ == '__main__':
    main()
