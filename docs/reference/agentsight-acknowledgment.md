# AgentSight 项目致敬

Agent eBPF Filter 项目在架构设计、技术选型和部分代码实现上受到了 [AgentSight](https://github.com/eunomia-bpf/agentsight) 项目的启发和影响。

## AgentSight 简介

**AgentSight** 是由 [eunomia-bpf](https://github.com/eunomia-bpf) 团队开发的开源项目，专注于使用 eBPF 技术对 AI Agent 进行系统级追踪和监控。

- **开源许可**: MIT License
- **主要特性**: 零插桩、eBPF 追踪、TLS 流量捕获、进程/文件/网络监控
- **技术栈**: Rust + eBPF (C) + Next.js
- **项目地位**: 发表于 ACM 会议，arXiv 论文编号 2508.02736

### AgentSight 的核心理念

AgentSight 提出了"系统级 AI Agent 可观测性"的理念：

> 传统的应用级观测工具（LangSmith、Langfuse、Phoenix）关注 traces、prompts、tokens 和 latency，但往往缺失 Agent 在系统边界的真实行为。AgentSight 通过 eBPF 和 TLS 流量追踪，在不需要 SDK、代理或源码修改的情况下，观测 Agent 的实际系统行为。

这一理念直接影响了 Agent eBPF Filter 的产品定位。

## 对本项目的影响

### 1. 架构设计借鉴

Agent eBPF Filter 借鉴了 AgentSight 的分层架构：

```text
AgentSight:
  eBPF Programs (C) → Rust Collector → SQLite + HTTP Server → Frontend

Agent eBPF Filter:
  eBPF Programs (C) → Go Backend → Protobuf + WebSocket → Vue Frontend
```

核心相似点：
- 内核态 eBPF 采集
- 用户态数据聚合与分析
- Web 可视化界面
- TLS 流量明文捕获（可选高风险诊断能力）

### 2. 技术方向启发

AgentSight 验证了以下技术方向的可行性：

| 技术 | AgentSight 验证 | Agent eBPF Filter 采纳 |
| --- | --- | --- |
| eBPF syscall tracing | ✅ execve/openat/connect | ✅ 扩展到 9 个 syscall |
| TLS uprobe capture | ✅ OpenSSL/BoringSSL | ✅ + GnuTLS/NSS/Go TLS |
| Process lineage | ✅ fork/clone tracking | ✅ + userspace fallback |
| Agent context | ✅ session/model/tokens | ✅ + tool_call_id/trace_id |
| Zero instrumentation | ✅ 核心卖点 | ✅ + adapters/hooks/wrapper 补充 |

### 3. 差异化方向

Agent eBPF Filter 在以下方面进行了扩展和差异化：

| 维度 | AgentSight | Agent eBPF Filter |
| --- | --- | --- |
| **语言栈** | Rust | Go |
| **前端** | Next.js + React | Vue 3 + Composition API |
| **存储** | SQLite | Protobuf + in-memory ring + optional JSONL |
| **控制能力** | 观测为主 | 观测 + wrapper policy + cgroup/LSM enforcement |
| **集成方式** | 纯 eBPF | eBPF + adapters + native hooks + wrapper |
| **目标场景** | 本地开发工作站 | 本地工作站 + 实验节点 + 操作系统课程答辩 |
| **安全边界** | TLS capture 作为核心能力 | TLS capture 默认关闭，作为高风险诊断能力 |

## 移植与改写的 Go 文件

以下 Go 文件的设计思路、数据结构或算法受到 AgentSight Rust 代码的启发，但实现语言、框架和细节均为本项目重新编写：

### 1. AgentSight 兼容层

| 文件 | 说明 |
| --- | --- |
| `backend/app/handlers__handlers_agentsight.go` | AgentSight 兼容 API 端点 |
| `backend/app/api__api_external.go` | External API 兼容路由 |
| `backend/app/agentsight__analyzers_agentsight.go` | AgentSight session 数据转换 |
| `backend/app/handlers__handlersagentsight_test.go` | 测试用例 |
| `backend/app/feature_build_agentsight.go` | AgentSight feature build tag |
| `backend/handlers/agentsight/test.go` | 旧 handler 测试（已迁移） |

### 2. TLS 流量处理

| 文件 | 说明 |
| --- | --- |
| `backend/app/tls__httpparsertls.go` | HTTP/TLS parser（受 AgentSight HTTPParser 启发） |
| `backend/app/tls__agentstreamlooptls.go` | TLS stream loop（受 AgentSight SSL runner 启发） |
| `backend/http/parser/tls.go` | TLS payload parser |
| `backend/agent/stream/loop/tls.go` | Agent stream TLS loop |

这些文件的核心逻辑（HTTP 请求/响应匹配、SSE 解析、流式处理）借鉴了 AgentSight `collector/src/framework/analyzers/http_parser.rs` 和 `collector/src/framework/analyzers/sse_processor.rs` 的设计思路，但使用 Go 语言重写，并适配 Agent eBPF Filter 的事件模型。

### 3. 指标收集

| 文件 | 说明 |
| --- | --- |
| `backend/app/observability__metrics_collector.go` | 系统指标收集（受 AgentSight system runner 启发） |

受 AgentSight `collector/src/framework/runners/system_runner.rs` 启发，但针对 Go 生态重新实现。

## 致谢

我们感谢 AgentSight 项目及其贡献者：

- 验证了 eBPF + TLS capture 对 AI Agent 观测的可行性
- 提供了系统级 Agent 可观测性的参考架构
- 开源了高质量的 eBPF 实现和文档

**AgentSight 项目**:
- GitHub: https://github.com/eunomia-bpf/agentsight
- arXiv: https://arxiv.org/abs/2508.02736
- ACM DOI: https://dl.acm.org/doi/10.1145/3766882.3767169
- License: MIT

## 差异声明

Agent eBPF Filter 是一个 **独立项目**，具有以下重要差异：

1. **不同的技术栈**: Go (后端) + Vue 3 (前端) vs Rust + Next.js
2. **不同的产品定位**: 观测 + 控制 vs 纯观测
3. **不同的安全模型**: TLS capture 默认关闭 vs 核心能力
4. **扩展的控制能力**: wrapper / cgroup / BPF LSM enforcement
5. **不同的集成方式**: adapters / hooks / wrapper 补充 eBPF
6. **不同的事件模型**: protobuf EventEnvelope vs JSON events
7. **不同的目标场景**: 包含操作系统课程答辩交付

## 许可证

Agent eBPF Filter 使用 **GPL-3.0** 许可证，与 AgentSight 的 MIT 许可证不同。

移植与改写的代码遵循以下原则：
- 受启发的设计思路和算法属于技术领域的公共知识
- 实现语言、框架和具体代码完全重写
- 数据结构和接口根据本项目需求重新设计
- 不包含 AgentSight 的源码复制

## 参考文档

- AgentSight 项目文档位于：`docs/ref/agentsight/`
- 本项目 AgentSight 兼容层文档：[MCP、External API 与 OTLP](/integrations/mcp-external-otlp)
- TLS capture 安全边界：[脱敏与隐私](/security/redaction-privacy)

---

**再次感谢 AgentSight 项目对开源社区的贡献！**
