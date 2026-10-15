# Renew Windows native monitoring + eBPF for Windows

Windows desktop monitoring works without a driver via `CreateToolhelp32Snapshot`, `GetExtendedTcpTable`, and `GlobalMemoryStatusEx`. It currently supports process-tree snapshots, memory and sampled TCPv4/TCPv6 socket metadata. PID attribution uses observed executable ancestry only; generic `node.exe`, `cmd.exe`, and `powershell.exe` are not assumed to belong to an agent.

The optional `renew_connect.c` is a **real eBPF for Windows** program, not Linux eBPF: `cgroup/connect4` and `cgroup/connect6` add only global invocation counters to the `renew_conn` map. It always returns `BPF_SOCK_ADDR_VERDICT_PROCEED_SOFT`: **it never blocks traffic**. No HTTP payload, DNS domain, process-creation tracepoint, file protection, or agent-specific count is claimed.

Microsoft runtime: https://github.com/microsoft/ebpf-for-windows ; it requires the official eBPF for Windows runtime and compatible SDK headers, and officially supports Windows 11 or later / Windows Server 2022 or later. It is a separate driver installation, **not bundled or automatically elevated by Renew**. `bpftool` enables read-only map counter queries; the UI reports a missing tool as missing data, never as zero.

On a supported Windows host with a compatible LLVM/Clang and an installed Microsoft eBPF for Windows runtime, in PowerShell (administrator is needed for hook installation):

```powershell
$include = 'C:\\path\\to\\ebpf-for-windows\\build\\native\\include'
.\Install-RenewEbpf.ps1 -IncludeDir $include -Action Verify
.\Install-RenewEbpf.ps1 -IncludeDir $include -Action Install
netsh ebpf show programs
bpftool -j map dump name renew_conn
```

The script uses `netsh ebpf add program` for SDKs supporting ELF loading. Some newer runtimes require converting `.o` files into signed native `.sys` drivers; follow the matching release's native code generation and driver signing instructions, and do not disable Windows driver signing or enable test signing on production workstations. Verification failures are surfaced rather than silently ignoring the eBPF hook.

Windows eBPF hook invocation counters and Win32 PID socket snapshots are *different evidence sources*, and counters must not be attached to individual connections. Short-lived sockets may evade the 2-second Windows API poll. Neither the eBPF hook nor Win32 sampling proves that traffic was allowed or blocked by Windows Firewall. Linux-only policy pages are deliberately disabled in local Windows mode. The remote `--backend` path still works with an existing Linux Agent eBPF Filter server.
