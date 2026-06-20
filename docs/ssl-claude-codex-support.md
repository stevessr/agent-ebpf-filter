# SSL/TLS 捕获增强 - Claude Code & Codex 支持

## 概述

新增了对 Claude Code 和 Codex AI CLI 工具的 SSL/TLS 流量捕获能力，通过 eBPF uprobes 附加到它们的静态链接 OpenSSL 实现。

## 支持的目标

### 1. Claude Code (Node.js)
- **路径**: `/home/steve/.local/share/fnm/node-versions/*/installation/bin/node`
- **检测**: 通过 cmdline 包含 `claude-code` 或 `@cometix`
- **SSL 实现**: 静态链接 OpenSSL，符号表可用
- **Attach 方法**: uprobe 到 `SSL_write`, `SSL_read`, `SSL_write_ex`, `SSL_read_ex`

### 2. Codex (原生二进制)
- **路径**: `~/.local/share/pnpm/store/*/codex/vendor/x86_64-unknown-linux-musl/bin/codex`
- **检测**: 
  - cmdline 包含 `codex` 或 `@openai`
  - 或可执行文件名为 `codex`
- **SSL 实现**: 静态链接 OpenSSL (musl libc)，但符号表被 stripped
- **Attach 方法**: 
  - 如果符号表可用：uprobe 到 SSL 函数
  - 如果 stripped：当前不支持（需要符号偏移或其他技术）

## 实现细节

### 新增函数

#### `backend/probe/manager/tls.go`
```go
// AttachStaticSSLUprobes 附加到静态链接 OpenSSL 的二进制文件
func (m *TLSProbeManager) AttachStaticSSLUprobes(binPath string, pid int) error
```

#### `backend/probe/discovery/tls.go`
```go
// DiscoverNodeProcesses 发现并附加 Node.js/Bun/Deno 进程
func (m *TLSProbeManager) DiscoverNodeProcesses()

// hasSSLSymbols 检查二进制文件是否有可用的 SSL 符号
func hasSSLSymbols(binPath string) bool
```

### 自动发现

在 `StartGoDiscoveryLoop` 中，每分钟执行：
1. `DiscoverGoProcesses()` - 发现 Go TLS 连接
2. `DiscoverNodeProcesses()` - 发现 Node.js/Codex 进程

### 过滤逻辑

1. 扫描 `/proc/[0-9]*/exe`
2. 过滤可执行文件名：`node`, `bun`, `deno`, `codex`
3. 读取 `/proc/[pid]/cmdline` 检查是否为目标进程
4. 检查符号表可用性
5. 附加 uprobe 到 SSL 函数

## 已跟踪的 SSL 函数

- `SSL_write` - 发送数据 (uprobe)
- `SSL_read` - 接收数据 (uprobe + uretprobe)
- `SSL_write_ex` - 扩展写入 (uprobe + uretprobe)
- `SSL_read_ex` - 扩展读取 (uprobe + uretprobe)

## 测试

### 测试 1: 检查 Node.js SSL 符号
```bash
cd backend
go run ./cmd/test_ssl_discovery.go
```

预期输出：
```
StaticTLS: true
Type: Claude Code (Node.js)
```

### 测试 2: 运行时附加
1. 启动 Claude Code 会话
2. 后端会在下一次发现周期（最多 1 分钟）自动附加
3. 检查 `/config` 页面的 TLS Libraries 状态

### 测试 3: 手动附加
```bash
# 通过 API
curl -H "X-API-KEY: $AGENT_API_KEY" \
  -X POST http://localhost:8080/tls/attach \
  -d '{"executable": "node", "pid": 12345}'
```

## 限制

### Codex Stripped Binary
Codex 的原生二进制文件是完全静态链接且符号表被 stripped 的：
```bash
$ file codex
codex: ELF 64-bit LSB pie executable, static-pie linked, stripped
```

虽然字符串中包含 SSL 函数名，但无法通过符号名直接 attach uprobe。

**可能的解决方案**：
1. 使用 `bpftrace` 或类似工具找到 SSL 函数的偏移
2. 使用 kprobe 跟踪系统调用层
3. 要求 OpenAI 提供带符号的 debug 版本

## 文件变更

- `backend/probe/manager/tls.go`: 新增 `AttachStaticSSLUprobes`
- `backend/probe/discovery/tls.go`: 新增 `DiscoverNodeProcesses`, `hasSSLSymbols`
- `backend/cmd/test_ssl_discovery.go`: 测试脚本
- `backend/cmd/test_node_ssl_attach.go`: Node.js 测试脚本

## 使用场景

1. **Claude Code 开发会话监控**：捕获所有 AI 请求/响应
2. **Codex API 调试**：查看发送到 OpenAI 的请求
3. **安全审计**：确保敏感数据不被意外发送
4. **性能分析**：分析 API 调用延迟和数据量

## 下一步

- [ ] 为 stripped Codex 二进制实现偏移查找
- [ ] 添加对 Bun 和 Deno 静态 TLS 的支持
- [ ] 实现 TLS 内容的智能过滤和搜索
- [ ] 添加 WebSocket 流量的特殊处理

---

## 相关导航

- [Codex workflows](codex-workflows.md)
- [Codex implementation complete](codex-implementation-complete.md)
- [SSL implementation summary](ssl-implementation-summary.md)
- [TLS Quickstart](backend/TLS_QUICKSTART.md)
- [脱敏与隐私](security/redaction-privacy.md)
