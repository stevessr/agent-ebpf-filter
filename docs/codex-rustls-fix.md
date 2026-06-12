# Codex SSL/TLS 捕获修复总结

## 问题
Codex 是一个 Rust 编译的静态链接二进制（stripped），使用 rustls 而非 OpenSSL，之前尝试使用 syscall tracepoint 追踪效率低且精度差。

## 解决方案

### 1. 移除低效的 syscall 追踪
- 删除 `backend/ebpf/codex_syscall_tracker.c` 和相关 eBPF 代码
- 删除 `backend/app/tls__codex_tracker.go` 中的 `CodexSyscallTracker`
- 从启动流程中移除 Codex syscall tracker

### 2. 实现 rustls 函数偏移定位
创建 `backend/app/tls__probediscoveryrustls.go`，通过以下方式定位 rustls 关键函数：

- **符号表查找**（非 stripped 情况）：查找 `rustls::connection::Connection::write_tls` 和 `read_tls`
- **模式匹配**（stripped 情况）：
  - 函数序言模式：`push rbp; mov rbp, rsp; sub rsp, N`
  - 系统调用特征：`syscall` 指令 + write(1)/writev(20) 或 read(0)/readv(19) 系统调用号
  - 字节码模式：`mov eax, imm32` 或 `mov rax, imm32` 设置 syscall number

### 3. 更新 TLS 发现逻辑
修改 `backend/app/tls__probediscoverytls.go::DiscoverNodeProcesses()`：

- **Node.js/Bun/Deno**：继续使用 OpenSSL 符号附加（`SSL_write`/`SSL_read`）
- **Codex**：使用 `AttachRustlsUprobes()` 通过偏移量附加 uprobe

### 4. 实现 rustls uprobe 附加
创建 `backend/app/tls__probemanagerrustls.go`：

- 使用 `link.UprobeOptions{Address: offset}` 直接附加到偏移地址
- 复用现有的 eBPF 程序：`uprobe_ssl_write`、`uprobe_ssl_read`、`uretprobe_ssl_read`
- 设置库状态为 "rustls"

## 技术细节

### 文件结构
```
backend/app/
├── tls__probediscoveryrustls.go    # rustls 偏移定位器
├── tls__probemanagerrustls.go      # rustls uprobe 附加
├── tls__probediscoverytls.go       # 更新的发现逻辑
└── tls__probemanager_builtin.go    # 兼容性存根
```

### Codex 二进制特征
- **类型**：ELF 64-bit LSB pie executable, static-pie linked, stripped
- **TLS 库**：rustls 0.23.36（从字符串中可见）
- **无符号表**：所有符号被 stripped，需要运行时模式匹配

### 优势
1. **精准**：直接附加到 TLS 函数，而非系统调用层
2. **高效**：零开销（仅在 Codex 进程中触发）
3. **完整**：捕获所有 TLS 流量，包括应用层数据
4. **自动**：通过 `/proc` 扫描自动发现和附加

## 测试
```bash
# 测试 rustls 偏移定位
cd backend && go run ./cmd/test_rustls_offset.go $(which codex)

# 启动后端，自动发现会在 1 分钟内附加
./backend/agent-ebpf-filter

# 检查状态
curl http://localhost:8080/tls-capture/libraries
```

## 限制
- **模式匹配准确性**：stripped binary 的偏移定位依赖字节码模式，可能在不同 rustls 版本中失效
- **符号更优**：如果 OpenAI 提供带符号的 debug build，可以直接使用符号附加
- **x86_64 only**：当前模式仅支持 x86_64，ARM64 需要不同的字节码模式

## 相关文件变更
- 删除：4 个文件（syscall tracker 相关）
- 新增：4 个文件（rustls 支持）
- 修改：7 个文件（集成和清理）
- 净变化：-186 行（更简洁的实现）
