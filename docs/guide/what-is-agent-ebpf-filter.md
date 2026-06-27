# 项目是什么

Agent eBPF Filter 是一个 Linux-first 的 AI Agent 观测与控制平面。它把 eBPF、Go、Vue、CLI wrapper、native hooks、Python / Node adapters、MCP / OTLP / Prometheus 接口和可选的 ML / plugin 能力整合在一起，用于观察、关联、分析和约束本地 Agent 与开发者 CLI 的行为。

## 一句话解释

> 用内核事实看见 Agent 行为，用用户态语义解释行为，用可视化工作台呈现证据，用 wrapper / cgroup / BPF LSM 实施控制。

## 它解决的问题

AI Agent 或自动化 CLI 往往会产生大量系统行为：执行 shell、读取配置、调用网络、生成文件、安装依赖、修改仓库。仅看 prompt 或终端输出无法确认真实 OS 行为；仅看 syscall 又缺少 Agent 语义。Agent eBPF Filter 把两者合并。

| 问题 | 本项目的回答 |
| --- | --- |
| Agent 实际做了什么？ | eBPF tracepoint 捕获 exec/open/connect/sendto/recvfrom 等事实事件。 |
| 行为属于哪个 Agent run？ | adapters、hooks、wrapper 提供 root_agent_pid、agent_run_id、tool_call_id、trace_id 等上下文。 |
| 如何查看证据？ | Dashboard、Network、Execution Graph、AgentSight、录制/回放和导出。 |
| 如何约束危险行为？ | wrapper policy、cgroup destination blocking、BPF LSM 文件/执行阻断。 |
| 如何保护敏感信息？ | redaction level、digest、sanitized_fields、TLS capture 默认关闭。 |
| 如何对外集成？ | MCP、External API、OTLP、Prometheus、Kubernetes manifests。 |

## 项目边界

本项目适合：

- 本地开发工作站的 Agent 行为审计；
- 实验节点上的 eBPF / OS enforcement 演示；
- 比赛答辩中的操作系统能力展示；
- Agent 工具链的安全研究和策略原型验证；
- 对 syscall 事实、网络流、AgentSight 事件和 OTLP span 做联合分析。

本项目不应被描述为：

- 完整容器 sandbox；
- 默认开启 TLS 明文采集的代理；
- 自动递归拦截所有路径树的文件防火墙；
- CIDR/range 级网络策略引擎；
- 替代 Kubernetes / SELinux / AppArmor 的生产级强制访问控制系统。

## 当前实现主线

```mermaid
flowchart LR
    %% 样式定制
    classDef agentCls fill:#e1f5fe,stroke:#03a9f4,stroke-width:1px;
    classDef kernelCls fill:#efebe9,stroke:#795548,stroke-width:1px;
    classDef backendCls fill:#e8f5e9,stroke:#4caf50,stroke-width:1px;
    classDef apiPathCls fill:#fff9c4,stroke:#fbc02d,stroke-width:1.5px;
    classDef mcpPathCls fill:#f3e5f5,stroke:#9c27b0,stroke-width:1.5px;
    classDef workbenchCls fill:#fff3e0,stroke:#ff9800,stroke-width:1px;
    classDef aiCls fill:#f5f5f5,stroke:#9e9e9e,stroke-width:1px;

    %% ==========================================
    %% 1. 左翼：并行数据采集源
    %% ==========================================
    subgraph Block_Telemetry ["并行数据采集与执行面"]
        direction TB
        subgraph Block_Agent ["用户态：AI Agent 模块"]
            A_Hook["Native Hooks<br/>(语义上报)"]:::agentCls
            A_Wrap["Agent-Wrapper<br/>(命令策略)"]:::agentCls
        end
        
        subgraph Block_Kernel ["内核态：Linux 内核 (eBPF)"]
            K_Trace["Tracepoint<br/>(Syscall 采集)"]:::kernelCls
            K_Cgroup["cgroup eBPF<br/>(网络阻断)"]:::kernelCls
            K_Lsm["BPF LSM<br/>(文件阻断)"]:::kernelCls
        end
    end

    %% ==========================================
    %% 2. 中核：Go Backend 核心处理
    %% ==========================================
    subgraph Block_Backend ["数据中心：Go Backend Core"]
        direction TB
        B_Ingest["数据接入与归一化<br/>(ringbuf decode / Event)"]:::backendCls
        B_Ctrl["控制与策略引擎<br/>(runtime config / auth / gates)"]:::backendCls
        B_Storage["持久化存储<br/>(archive / JSONL / recording)"]:::backendCls
        
        B_Ingest --> B_Ctrl --> B_Storage
    end

    %% ==========================================
    %% 3. 右翼：彻底分离的双路径网关
    %% ==========================================
    
    %% 路径 A：标准 API 网关
    subgraph Block_API_Path ["【路径 A】标准 API & 监控网关"]
        direction TB
        GW_API["REST / WebSocket Server"]:::apiPathCls
        GW_Metric["Telemetry Exporter<br/>(OTLP / Prometheus)"]:::apiPathCls
    end

    %% 路径 B：MCP 协议网关
    subgraph Block_MCP_Path ["【路径 B】MCP 智能体协议网关"]
        direction TB
        GW_MCP["MCP Server 核心"]:::mcpPathCls
        M_Tools["Tools 映射<br/>(阻断控制 API 化)"]:::mcpPathCls
        M_Res["Resources 映射<br/>(内核事件/语义上下文)"]:::mcpPathCls
        
        GW_MCP <--> M_Tools
        GW_MCP <--> M_Res
    end

    %% ==========================================
    %% 4. 消费者端
    %% ==========================================
    subgraph Block_Workbench ["传统控制台：Vue Workbench"]
        direction TB
        W_Dash["Dashboard / NetworkFlow"]:::workbenchCls
        W_Graph["Execution Graph / AgentSight"]:::workbenchCls
        W_Mgmt["Config / Security 管理"]:::workbenchCls
    end

    subgraph Block_AI_Ecosystem ["AI 生态：大模型与新型 IDE"]
        direction TB
        AI_Client["外部 MCP 客户端<br/>(Cursor / Windsurf / Copilot)"]:::aiCls
        AI_Agent["外部自主智能体集群<br/>(Multi-Agent Frameworks)"]:::aiCls
    end

    %% ==========================================
    %% 核心数据流拓扑关系
    %% ==========================================
    
    %% 数据输入与控制反馈
    A_Hook --> B_Ingest
    K_Trace --> B_Ingest
    B_Ctrl -.->|策略下发| K_Cgroup
    B_Ctrl -.->|策略下发| K_Lsm
    B_Ctrl <--> A_Wrap

    %% 核心分流点 (Backend Core 到双路径)
    B_Storage --> GW_API
    B_Ctrl <--> GW_API
    
    B_Storage --> M_Res
    B_Ctrl <--> M_Tools

    %% 路径 A 输出流向
    GW_API -->|WebSocket 广播 / REST| W_Dash
    GW_API -->|数据渲染| W_Graph
    W_Mgmt <-->|HTTP 配置下发| GW_API
    
    %% 路径 B 输出流向（标准的 MCP 协议双向交互）
    GW_MCP <-->|JSON-RPC over STDIO/SSE| AI_Client
    GW_MCP <-->|JSON-RPC over SSE| AI_Agent
```

## 代码入口

- 后端启动：`backend/app/main.go`
- 路由注册：`backend/app/routes.go`
- runtime settings：`backend/core/state_types.go`
- feature manifest：`backend/app/feature_manifest.go`
- 事件上下文：`backend/app/events/context_event.go`
- 主 eBPF：`backend/ebpf/agent_tracker.c`
- cgroup sandbox：`backend/ebpf/cgroup_sandbox.c`
- BPF LSM：`backend/ebpf/lsm_enforcer.c`
- 前端路由：`frontend/src/router/index.ts`
- wrapper：`wrapper/main.go`
- proto 事件：`proto/tracker_events.proto`

---

## 相关导航

- [快速开始](quick-start.md)
- [功能总览](capabilities.md)
- [总体架构](../architecture/overview.md)
- [安全模型](../security/model.md)
- [阅读路线](reading-paths.md)
