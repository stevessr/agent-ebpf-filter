#!/usr/bin/env python3
"""
Codex SSL Function Finder - 使用运行时跟踪方法
由于 Codex 是 stripped binary，使用 bpftrace 运行时定位
"""

import subprocess
import sys
import os

def check_requirements():
    """检查必要的工具"""
    tools = ['bpftrace', 'readelf']
    missing = []
    for tool in tools:
        if subprocess.run(['which', tool], capture_output=True).returncode != 0:
            missing.append(tool)
    return missing

def generate_bpftrace_script(binary_path):
    """生成 bpftrace 脚本来跟踪 Codex 的 SSL 调用"""

    script = f'''#!/usr/bin/env bpftrace

/*
 * Codex SSL Tracer
 * 跟踪 Codex 进程的 SSL_write/SSL_read 调用
 * 用法：sudo bpftrace codex_ssl_trace.bt
 */

BEGIN {{
    printf("Tracing Codex SSL calls... Press Ctrl-C to stop\\n");
    printf("%-8s %-16s %-10s %-10s\\n", "TIME", "COMM", "FUNC", "SIZE");
}}

// 跟踪 write/writev 系统调用 (SSL 最终会调用这些)
tracepoint:syscalls:sys_enter_write
/comm == "codex"/
{{
    printf("%-8s %-16s %-10s %-10d\\n",
        strftime("%H:%M:%S", nsecs),
        comm,
        "write",
        args->count);
}}

tracepoint:syscalls:sys_enter_writev
/comm == "codex"/
{{
    printf("%-8s %-16s %-10s %-10d\\n",
        strftime("%H:%M:%S", nsecs),
        comm,
        "writev",
        0);
}}

// 跟踪 read/readv 系统调用
tracepoint:syscalls:sys_enter_read
/comm == "codex"/
{{
    printf("%-8s %-16s %-10s %-10d\\n",
        strftime("%H:%M:%S", nsecs),
        comm,
        "read",
        args->count);
}}

tracepoint:syscalls:sys_enter_readv
/comm == "codex"/
{{
    printf("%-8s %-16s %-10s %-10d\\n",
        strftime("%H:%M:%S", nsecs),
        comm,
        "readv",
        0);
}}

END {{
    printf("\\nTracing stopped.\\n");
}}
'''
    return script

def find_text_section_info(binary_path):
    """获取 .text 段信息"""
    result = subprocess.run(
        ['readelf', '-S', binary_path],
        capture_output=True, text=True
    )

    for line in result.stdout.split('\n'):
        if '.text' in line and 'PROGBITS' in line:
            parts = line.split()
            # 找到地址和大小
            for i, part in enumerate(parts):
                if part == '.text':
                    addr = parts[i+2]
                    size = parts[i+4]
                    return addr, size
    return None, None

def main():
    if len(sys.argv) < 2:
        print("Usage: python3 find_codex_ssl.py <codex_binary>")
        sys.exit(1)

    binary = sys.argv[1]

    if not os.path.exists(binary):
        print(f"Error: Binary not found: {binary}")
        sys.exit(1)

    print(f"=== Codex SSL Function Analyzer ===\n")
    print(f"Target: {binary}\n")

    # 1. 检查工具
    print("1. Checking requirements...")
    missing = check_requirements()
    if missing:
        print(f"   ⚠️  Missing tools: {', '.join(missing)}")
        print(f"   Install: sudo apt install bpftrace binutils")
    else:
        print("   ✓ All tools available")
    print()

    # 2. 验证二进制信息
    print("2. Binary analysis:")
    result = subprocess.run(['file', binary], capture_output=True, text=True)
    print(f"   Type: {result.stdout.strip()}")

    addr, size = find_text_section_info(binary)
    if addr and size:
        print(f"   .text: {addr} (size: {size})")
    print()

    # 3. 检查 SSL 字符串
    print("3. SSL functions in strings:")
    result = subprocess.run(['strings', binary], capture_output=True, text=True)
    ssl_funcs = [line for line in result.stdout.split('\n')
                 if line.startswith('SSL_') and ('write' in line or 'read' in line)]
    for func in sorted(set(ssl_funcs)):
        print(f"   - {func}")
    print()

    # 4. 生成 bpftrace 脚本
    print("4. Solution: Runtime tracing approach")
    print("   Since Codex is stripped, use runtime tracing:")
    print()

    script_path = '/tmp/codex_ssl_trace.bt'
    script = generate_bpftrace_script(binary)

    with open(script_path, 'w') as f:
        f.write(script)
    os.chmod(script_path, 0o755)

    print(f"   Generated: {script_path}")
    print()
    print("   Usage:")
    print(f"     1. Run Codex in another terminal")
    print(f"     2. sudo bpftrace {script_path}")
    print(f"     3. Observe SSL traffic via syscalls")
    print()

    # 5. 替代方案
    print("5. Alternative approaches:")
    print("   A. Syscall-level tracing (kprobe):")
    print("      - Track write/read/sendto/recvfrom on TLS ports")
    print("      - Lower precision but works with stripped binaries")
    print()
    print("   B. LD_PRELOAD hook (if dynamic linking):")
    print("      - Not applicable (Codex is static-pie)")
    print()
    print("   C. Request debug symbols:")
    print("      - Contact OpenAI for debug build")
    print()

    # 6. 当前实现状态
    print("6. Current implementation status:")
    print("   ✓ Node.js (Claude Code): Fully supported")
    print("   ⚠️  Codex (native): Syscall-level tracing only")
    print()
    print("   For now, use:")
    print("   - Syscall tracing for Codex")
    print("   - Direct uprobe for Claude Code (Node.js)")

if __name__ == '__main__':
    main()
