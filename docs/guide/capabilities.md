# 功能与状态总览

请先分清三种不同含义：**已写入代码**、**必须显式开启**、**需要特定 Linux 内核/外部程序才能实际生效**。以下按 `master@10e9766`（2026-10-15）整理，不将实验机制描述为默认启用的生产服务。

## 面向用户：原生桌面与 Web 工作台

| 能力 | 当前入口 | 关键实现与边界 |
| --- | --- | --- |
| 设备健康与风险优先级 | Renew「我的设备」「系统诊断」 | 区分连接、系统流、采集器与队列损失；暂无事件不等于安全 |
| 事件检索、风险过滤、详细取证 | Renew「事件」「风险研判」 | 桌面展示有界紧凑摘要；按需拉取完整详情，暂停/滑块只影响界面 |
| Agent 域名/IP 外联 | Renew「Agent 域名监控」 | 基于后端网络摘要与受验证归因；不是逐请求 API 日志或代理 |
| Claude / Codex 配置候选检查 | Renew 域名页的只读功能 | 限定读取本机 JSON/TOML 白名单，不能声称复现 CLI 最终有效配置 |
| Agent → SubAgent 派生 | Renew「Agent 派生」 | 放射/瀑布双视图；当前 PID/PPID 快照，最多 72 节点与 6 层；逻辑子 Agent 仅为候选或独立 Hook 信号 |
| 本机文件管理 / 编辑器 | Renew「工具」 | 纯 MyGo 编辑器、多标签；普通用户权限，冲突保护，拒绝符号链接；无需后端在线 |
| 终端与分离窗口 | Renew 原生工作区 | 终端标签可以分离/回收；部分只读监控页可分离，复用同一采集后端 |
| 研究型实时仪表盘 | Vue `frontend/` | Dashboard、Monitor、Network、Execution Graph、AgentSight、Hooks、Config、ML、Plugins 等 |
| 终端实时事件查看 | `make tui` | 复用后端 protobuf WebSocket 事件接口 |

## 采集与 Agent 归因

- **eBPF 追踪**：内核 tracepoint → ringbuf → Go 解码/过滤/分发，进程、文件与网络事件结合系统调用类型、PID 和时间戳。
- **来源语义**：Python/Node PID 注册、CLI Native Hooks 与 `agent-wrapper`，把 Run、Trace、Tool 与 OS 事实结合。不是每个 syscall 都天然具备完整 Agent 上下文。
- **进程后代**：使用受限的 root Agent 缓存、fork 继承与进程证据；考虑 PID 重用，不把普通 `bash`、`python` 自动提升为新 Agent。
- **Claude SubAgent Hook**：`SubagentStart` 与 `SubagentStop` 为两个独立 opt-in 管理项，需明确安装与重启会话。仅保留摘要化 ID 与类型等受限信息，不能据此自行推造父级调用边或任务时长。
- **网络富化**：连接与流聚合可结合 DNS/SNI/HTTP Host 等可用元数据；加密 DNS、ECH 或没采到的流量会导致缺项。

## 控制与安全边界

| 执行层 | 有什么 | 使用前核查 |
| --- | --- | --- |
| 用户态 Wrapper | ALLOW / ALERT / BLOCK / REWRITE | 命令必须实际经过 wrapper；不覆盖绕过它的直接执行 |
| cgroup eBPF | 部分 TCP/UDP IPv4/IPv6 的地址、端口阻断 | cgroup v2、程序附着、精确规则与授权 |
| BPF LSM | 执行/文件操作策略、部分精确路径读写限制 | 内核 BPF LSM 启用、规则已确认、路径和 hook 覆盖边界 |
| Runtime Auth/Gates | release token 与危险功能 gate | 编译进去不等于运行时启用；查看实际 feature/status |
| Redaction | 多级脱敏和内容/凭据处理 | 取证详情和导出需按敏感数据规范操作 |
| TLS 明文 | 可选诊断采集 | 默认关闭，必须显式授权和隔离使用 |

**区分三个容易混淆的功能**：`trackedPaths` 用于**采集观察**；`ignoredPaths` 用于**事件降噪**；`path-access` 才是需要 kernel hook 与授权的**读写阻断**。目录递归跟踪不意味着目录递归 LSM 阻断。

## 系统组件与发布状态

| 组件 | 状态 | 不能夸大的地方 |
| --- | --- | --- |
| Go 应用后端、eBPF collector | 当前主要执行路径 | eBPF 需要受支持的实际 Linux 环境 |
| 低耦合 Go 包（agentscope、eventnoise、collectorcodec 等） | 已拆分并用于单元测试 | 是可复用库，并非每个都是独立可部署服务 |
| 私有桌面 Unix IPC + 授权 API Socket | Renew 内置后端路径 | 不是默认暴露公网服务；复用已有后端时保留该服务原本的端口配置 |
| `componentipc` 协议、`componentproc`、`componentshm` | 明确 opt-in、分阶段验证 | IPC 和 memfd/ACK 原型不意味着生产数据面已经整体切换 |
| Web/MCP/External API/OTLP/Prometheus | 后端提供对应集成入口 | 各功能仍受 token、feature 和 runtime 配置影响 |
| Kubernetes、systemd、devcontainer | 仓库提供部署和工程文件 | 有清单/脚本不等于已在所有系统环境验证 |

## 快速验证

```bash
make os-enforcement-preflight
make os-enforcement-check
cd desktop/renew && go test ./...
# 文档站（回到仓库根目录）：
bun run docs:build
```

真正的内核拒绝测试应在已授权的隔离 Linux 主机进行；静态检查与离线 replay 不能替代 live enforcement 测试。

## 下一步

[实际演示](/guide/demo-tour) · [原生桌面](/frontend/renew) · [架构](/architecture/overview) · [内核权限模型](/security/model) · [组件独立性](/architecture/independent-components)。
