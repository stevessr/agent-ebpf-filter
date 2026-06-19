# 功能总览

Agent eBPF Filter 的功能可按“采集、关联、展示、控制、导出、交付”六类理解。

## 采集能力

### syscall telemetry

主 eBPF tracker 覆盖以下 syscall tracepoint：

- `execve`
- `openat`
- `connect`
- `mkdirat`
- `unlinkat`
- `ioctl`
- `bind`
- `sendto`
- `recvfrom`

事件通过 ringbuf 进入 Go 后端。后端使用 mmap-backed zero-copy view 优化 aligned native-endian 样本解码，并在不满足条件时回退到 copy path。

### network flow

网络层在 syscall-derived events 基础上聚合 flow，支持：

- TCP / UDP flow attribution；
- DNS / SNI / HTTP Host / ALPN enrichment；
- interface traffic charts；
- stale / historic flow 标记；
- GeoIP、TCP state、protocol detection；
- JSONL / PCAP export。

### TLS / Codex capture

TLS 明文捕获是可选高风险诊断能力，默认关闭。启用后支持 OpenSSL、GnuTLS、NSS 和 Go TLS 符号探测；Codex capture 通过源码级显式上报适配 rustls/reqwest 场景。

普通事件流应携带 metadata / digest / redaction state，而不是无保护地保存敏感明文。

## 关联能力

| 来源 | 语义 |
| --- | --- |
| PID registration | root_agent_pid、agent_run_id、task_id、cwd 等 |
| wrapper request | command、args digest、decision、risk_score、tool metadata |
| native hooks | hook_name、tool_name、target_path、payload metadata |
| EventEnvelope | process、tool、syscall、network、policy、TLS 统一事件包 |
| Execution Graph | agent / process / tool / syscall / file / network / policy 关系 |

## 展示能力

- Dashboard：实时事件流、过滤、modal、strace-style summary。
- Monitor：CPU、内存、GPU、IO、page fault、sensor、systemd、tracing。
- Network：network events、flows、details、graph、traffic chart。
- Execution Graph：进程、工具调用、文件、网络、策略图谱。
- AgentSight：Log、Timeline、Process Tree、Metrics、导入导出。
- Explorer：文件浏览、preview、tracked path 添加。
- Executor：PTY shell、tmux / launcher、wrapper terminal。
- Hooks：AI CLI hooks 检测、安装、配置。
- Config：runtime、安全策略、registry、docs、system health。
- ML：模型状态、训练、调参、LLM scoring、dataset。
- Plugins：自定义 eBPF plugin、visual builder、pseudocode builder。

## 控制能力

| 控制层 | 机制 | 语义 |
| --- | --- | --- |
| wrapper | `/tmp/agent-ebpf.sock` | ALLOW / BLOCK / ALERT / REWRITE |
| cgroup eBPF | cgroup connect/sendmsg | exact cgroup id、IPv4、IPv6、TCP/UDP port |
| BPF LSM | exec / file hooks | exact executable path、executable basename、file basename |
| runtime gates | `/config/runtime` | shell、system_run、hooks、policy、TLS、OTLP、domain forward |
| release auth | runtime access token | 保护敏感 API、WebSocket、MCP、external APIs |

## 导出能力

- JSONL persistence：可选写入 `~/.config/agent-ebpf-filter/events.jsonl`。
- Recording / replay：事件和图谱快照可录制与回放。
- AgentSight export：JSON / JSONL / CSV。
- Network export：JSONL / PCAP。
- OTLP：派生 agent.run / codex.task / tool.call spans。
- Prometheus：`/metrics`。
- MCP：`/mcp` 暴露 config/event tools。

## 交付能力

项目同时包含：

- devcontainer；
- Kubernetes manifests；
- systemd / rc.local fallback installer；
- benchmark / replay scripts；
- 操作系统设计赛答辩文档；
- AI 使用披露、第三方 notice、评测报告模板。
