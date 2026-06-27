# 🌊 数据流管线全景

本篇深度解析 **Agent eBPF Filter** 的五维核心数据流向：**内核事件流、Wrapper 策略流、Native Hook 语义流、前端配置流以及多元导出流**。通过系统级时序与因果拓扑，向开发者展示用户态语义流如何与内核态客观事实进行高频、安全的交叉验证。

## 1. eBPF 内核事件流 (数据热路径)

内核事件流是系统的核心数据动脉，负责将高频发生的 Linux 系统调用（Syscalls）免拷贝、异步地泵入用户态后端，直至在前端工作台完成虚拟化瀑布流渲染。

```mermaid
sequenceDiagram
    participant Syscall as Linux Syscall
    participant eBPF as eBPF Tracepoint
    participant Maps as Pinned BPF Maps
    participant Ringbuf as BPF Ring Buffer
    participant Reader as Go Ringbuf Reader
    participant Decoder as decodeBPFEventRecord
    participant Builder as buildKernelEventFromRaw
    participant Broadcast as Broadcast Channel
    participant Archive as Event Archive
    participant WS as WebSocket 网关 (/ws)
    participant Vue as Vue Dashboard UI

    Syscall->>eBPF: execve / openat / connect 等调用触发
    eBPF->>Maps: 检索过滤条件 (tracked_comms / paths / pids)
    Maps-->>eBPF: 返回命中状态 (Match Result)
    eBPF->>Ringbuf: 异步提交事件报文 (Zero-copy Reserve)
    Ringbuf->>Reader: 通知 RawSample 事件就绪
    Reader->>Decoder: 投递原始切片 decode(RawSample)

    alt 满足 Native Little-Endian 且 内存对齐 (Aligned)
        Decoder->>Decoder: 内存指针对齐变换：mmap-backed view (零拷贝)
    else 异常场景或非对齐架构退化
        Decoder->>Decoder: binary.Read 深度复制缓冲 (Copy Path)
    end

    Decoder->>Builder: 规整后的 raw bpfEvent 结构体
    Builder->>Builder: 动态富化进程上下文 (Enrich Process Context)
    Builder->>Broadcast: 分发归一化的 pb.EventEnvelope
    Broadcast->>Archive: 压入内存环形缓冲区 (In-Memory Ring)
    Broadcast->>WS: 多路复用实时分发
    WS->>Vue: WebSocket 高频数据推送
    Archive->>Archive: (可选) 异步触发落盘 JSONL 固化

```

### 🔬 核心底控：Ringbuf 零拷贝解码机制

后端在 `decodeBPFEventRecord()` 热路径中实施了极致的性能压榨。在常见的 `x86_64`（小端、天然内存对齐）Linux 环境下，系统直接通过 `unsafe.Pointer` 进行指针对齐变换，消除应用层二次拷贝成本：

```go
func decodeBPFEventRecord(sample []byte) (*bpfEvent, error) {
    if len(sample) < minEventSize {
        return nil, ErrTooShort
    }

    // 黄金热路径：判断系统是否为标准小端架构且内存首地址天然对齐
    if isNativeLittleEndian && isAligned(sample) {
        // 【高性能零拷贝】通过指针直接将宿主机内存映射为结构体 View
        evt := (*bpfEvent)(unsafe.Pointer(&sample[0]))
        collectorMetricsStore.RecordRingbufDecode(true)
        return evt, nil
    }

    // 降级软路径：跨平台或错位数据退化到标准内存复制
    evt := &bpfEvent{}
    if err := binary.Read(bytes.NewReader(sample), binary.LittleEndian, evt); err != nil {
        return nil, err
    }
    collectorMetricsStore.RecordRingbufDecode(false)
    return evt, nil
}

```

#### 📌 关键源码入口

- **内核探针端**：`backend/ebpf/agent_tracker.c`、`backend/ebpf/agent_tracker_common.h`
- **常驻消费端**：`backend/app/runtime__jobs_background.go`
- **上下文聚合**：`backend/app/events__context_event.go`、`backend/app/events__graph_execution.go`
- **协议描述符**：`proto/tracker_events.proto`

## 🛡️ 2. Wrapper 命令策略流 (同步安全阻断)

Wrapper 作为命令拦截垫片（Shim），在 AI CLI 发起执行的“前置毫秒级”切入。它并非传统的沙盒（Sandbox），而是针对指定暴露的命令实施硬干预的策略控制层。

```mermaid
sequenceDiagram
    participant User as 用户 / AI 智能体 (User/Agent)
    participant Wrapper as agent-wrapper (劫持垫片)
    participant UDS as UDS 通信套接字 (/tmp/agent-ebpf.sock)
    participant Engine as 后端策略评估引擎 (Policy Engine)
    participant Exec as 宿主机原生执行 syscall.Exec

    User->>Wrapper: 触发调用命令 (例如：agent-wrapper git push)
    Wrapper->>Wrapper: 裁剪提取参数摘要、抓取当前环境变量元数据
    Wrapper->>UDS: 拨号连接本地 Unix Domain Socket
    Wrapper->>Engine: 投递同步握手 WrapperRequest(pid, comm, args, metadata)
    Engine->>Engine: 并发并发匹配过滤规则 + 调度本地 ML 进行行为风险评分

    alt 🟢 决策结果：ALLOW (放行)
        Engine-->>Wrapper: 返回核心决策 WrapperResponse(ALLOW)
        Wrapper->>Exec: 调用原生系统调用，透明替换执行目标命令 (exec git push)
    else 🔴 决策结果：BLOCK (阻断拦截)
        Engine-->>Wrapper: 返回核心决策 WrapperResponse(BLOCK)
        Wrapper->>User: ❌ 终端控制台抛出：Execution Blocked By Security Policy
    else 🟡 决策结果：ALERT (审计告警放行)
        Engine-->>Wrapper: 返回核心决策 WrapperResponse(ALERT)
        Wrapper->>User: ⚠️ 终端控制台打印安全警示，但允许执行
        Wrapper->>Exec: 调用原生系统调用透明替换执行目标 (exec git push)
    else 🔵 决策结果：REWRITE (动态重写改写)
        Engine-->>Wrapper: 返回核心决策 WrapperResponse(REWRITE, new_args)
        Wrapper->>Exec: 强行使用后端安全重写后的参数执行篡改后指令 (exec <rewritten command>)
    end

```

## 3. Native Hook 语义流 (应用层富化)

eBPF 只能捕获底层的系统调用事实，而 Native Hooks 则是 AI 框架的“眼睛”。它负责主动向后端上报更高级别的大模型语义（如：Tool 名、Prompt 上下文），帮助系统完成“因果链”拼图。

```mermaid
sequenceDiagram
    participant CLI as AI 智能体终端 (Claude / Gemini / Codex)
    participant Hook as 内置钩子系统 (Hook System)
    participant Relay as 自动化生成的脚本中继 (Relay Script)
    participant Backend as Go 后端路由 (/hooks/event)
    participant Normalize as 行为Payload归一化引擎
    participant Context as 进程动态上下文中心 (Store)
    participant Event as 事件分发管线 (Event Pipeline)

    CLI->>Hook: 智能体内部触发 Tool Execution，同步激活内部挂钩
    Hook->>Relay: 通过标准输入管道(stdin)向下倾倒原始 JSON Payload
    Relay->>Backend: 调用本地轻量 curl 触发：POST /hooks/event
    Backend->>Backend: 激活 hookIngressAuthMiddleware 门禁进行 Token 强校验
    Backend->>Normalize: 将原始 JSON 报文解包归一化
    Normalize->>Normalize: 精准析取 tool_name、target_path 以及当前执行阶段 phase
    Normalize->>Context: 关联当前进程：enrichProcessContext()
    Context-->>Normalize: 返回反查出的根智能体 root_agent_pid 与分布式 trace_id
    Normalize->>Event: 组装生成标准的 native_hook pb.Event
    Event->>Event: 包裹转换为全时空 EventEnvelope ➡️ 流向持久化/实时广播/OTLP 链路

```

## 🆔 4. PID Registration 信任树拓扑流 (生命周期关联)

> 🚨 **高频概念防错说明**
> 注册语义是**严格 Per-Process（单进程）**的显式声明。后代进程的追踪与关联并非依靠 Adapter 自动递归调用，而是依赖**内核态 fork/clone 谱系扩散**配合**后端 Fallback 追溯机制**完美闭环，请严格区分它们的职责边界。

```mermaid
sequenceDiagram
    participant Agent as 根智能体进程 (Parent Agent)
    participant Child as 衍生子进程 (Child Process)
    participant Adapter as 运行时适配器 (register)
    participant Backend as 后端注册端点 (/register)
    participant Store as processContextStore (状态树)
    participant BPFMap as eBPF 控制字典 (agent_pids Map)
    participant Tracker as eBPF 内核 Tracker

    %% 阶段 1：明确的单进程注册
    Note over Agent, BPFMap: 【阶段 1】显式 Per-Process 注册 (仅限当前注册单进程)
    Agent->>Adapter: 代码级 import agent_tracker 并显式调用 register()
    Adapter->>Adapter: 捕获当前进程真实 PID、自增 agent_run_id 与外部 trace_id
    Adapter->>Backend: 投递网络注册请求：POST /register (pb.RegisterRequest)
    Backend->>Store: 将 ProcessContext 状态树固化写入内存字典
    Backend->>BPFMap: 内核态打标：下发 BPF Map 键值对 agent_pids[pid] = 1
    Backend-->>Adapter: 返回 200 OK，同步信任树就绪

    %% 阶段 2：内核态的继承与谱系树建立
    Note over Agent, Tracker: 【阶段 2】内核态子进程衍生 (完全依赖内核谱系扩散，无应用层参与)
    Agent->>Tracker: 发起系统调用：fork() / clone() 衍生子进程
    Tracker->>BPFMap: 内核侧自动检索：lookup agent_pids[parent_pid] ➡️ 命中追踪树
    Tracker->>BPFMap: 内核态原地动态传播：自动对新进程追加标记 child_pid] = 1
    Tracker->>Store: 异步向用户态抛出轻量级 fork/clone 谱系生命周期事件
    Store->>Store: 在用户态内存动态编织/更新进程谱系血缘树 (Lineage)

    %% 阶段 3：子进程事件触发与误判兜底
    Note over Child, Store: 【阶段 3】子进程高频事件拦截与 Backend Fallback 深度富化
    Child->>Tracker: 发起敏感业务调用 (如 execve / openat / connect)
    Tracker->>BPFMap: 检索 BPF Map：lookup agent_pids[child_pid] ➡️ 命中，认定受控
    Tracker->>Store: 向数据管线异步抛出具体业务事件 (携带当前 child_pid)

    alt 场景 A：当前子进程的直连运行时上下文在状态树中命中
        Store->>Store: 直接提取直连上下文进行精细化属性富化
    else 场景 B：直连上下文未命中 (触发 Backend Fallback 追溯机制)
        Store->>Store: 激活 Fallback：沿着建立好的 Lineage 谱系树强行向上逆流追溯父辈进程
        Store->>Store: 找到根注册点，强行固定继承 ParentContext.agent_run_id 完成确定性关联
    end

```

## ⚙️ 5. 前端配置与策略下发流 (控制闭环)

当安全管理员或 AI 系统在 Vue Workbench 上进行控制变更（例如：动态封禁某 IP 行为）时，控制流必须突破五道防线，才能最终热加载注入内核态：

```mermaid
sequenceDiagram
    participant User as 安全审计员 / 控制端
    participant View as Vue Config 策略视窗
    participant Composable as useConfigSecurity (业务组合子)
    participant API as 后端动态路由 (/config/runtime)
    participant Auth as authMiddleware (特权门禁)
    participant Store as runtimeSettingsStore (内存热锁)
    participant BPFMap as policy BPF Maps (内核控制面)
    participant WS as WebSocket 实时广播总线

    User->>View: 鼠标点击/表单修改 cgroup 策略 (封禁特定恶意 IP)
    View->>Composable: 触发拦截函数 blockCgroupIP(ip)
    Composable->>API: 发起标准的网络配置请求：POST /sandbox/cgroup/block-ip
    API->>Auth: 【防线 1】强行校准判定：Release Mode Token 签名有效性
    Auth-->>API: 校验通过，授信提权
    API->>API: 【防线 2】检查 policy_management 运行时状态门控 (Runtime Gate)<br>【防线 3】确认对应 build feature 编译期标记已被引入 (Compiled In)
    API->>BPFMap: 【防线 4】直接调用 UAPI 强行改写内核：cgroup_blocked_ips[ip] = 1
    BPFMap-->>API: 内核 Maps 动态刷新更新成功
    API->>Store: 【防线 5】刷新用户态内存常驻热锁，保证全局状态一致性
    API->>WS: 向消息总线抛出 policy_change 广播事件
    API-->>Composable: 向调用端优雅返回标准：200 OK
    Composable->>View: 前端刷新对应阻断策略计数器 (Counters)
    WS->>View: 视窗级全量组件接收到广播，无需刷屏动态同步在线控制状态

```

## 📤 6. 多维导出分流矩阵

全归一化后的数据资产拥有完整的多元分流导出管线，通过模块化解耦，同时兼顾人类可读性（UI/JSONL）、系统排查取证（PCAP）以及 AI 协议集成（MCP）：

```mermaid
graph TB
    %% 样式精细化
    classDef srcCls fill:#f3e5f5,stroke:#9c27b0,stroke-width:1px;
    classDef tgtCls fill:#ffffff,stroke:#333,stroke-width:1px;

    subgraph Src_Block ["【高能数据源】数据聚合缓冲区 (Event Sources)"]
        Archive["💾 内存事件快照环<br/>(Captured Event Archive)"]:::srcCls
        Flows["🌐 异步网络聚合流<br/>(Network Flows Store)"]:::srcCls
        Graph["🗺️ 实时因果拓扑图<br/>(Execution Graph)"]:::srcCls
        TLS["🔑 可选明文探测流<br/>(TLS/Codex Capture)"]:::srcCls
        Metrics["📊 性能与监控指标<br/>(Collector Metrics)"]:::srcCls
    end

    subgraph Tgt_Block ["【多元消费端】交付与集成矩阵 (Export Targets)"]
        JSONL["📄 结构化日志落盘<br/>(~/.config/.../events.jsonl)"]:::tgtCls
        Recording["📼 帧级快照录制件<br/>(Replay Engine 离线回放)"]:::tgtCls
        PCAP["🦈 标准二进制网络报文<br/>(PCAP Wireshark 格式)"]:::tgtCls
        AgentSight["🔬 AgentSight 精准交付物<br/>(JSON / CSV 复盘包)"]:::tgtCls
        OTLP["🔌 OpenTelemetry Exporter<br/>(分布式追溯链路 Spans)"]:::tgtCls
        Prometheus["📈 Prometheus 端点<br/>(/metrics 时序监控)"]:::tgtCls
        MCP["🤖 Model Context Protocol<br/>(/mcp Server 智能体端点)"]:::tgtCls
    end

    %% 流向绑定
    Archive -->|全量事件持久化开关开启| JSONL
    Archive -->|离线安全审计会话捕获| Recording
    Flows -->|全网流量沙箱取证抽头| PCAP
    Graph -->|工作台全局拓扑数据快照| AgentSight
    Archive -->|泛化演算派生分布式 Span 树| OTLP
    Metrics -->|数据抓取周期轮询| Prometheus
    Archive -->|内置工具：tail_events 动态流式推送| MCP
    Graph -->|内置工具：query_graph 图谱语义查询| MCP

```

### 📊 分流策略核心映射参考矩阵

| 导出通道名称               | 数据源头 (From)               | 物理流向目标 (To)                          | 核心应用场景与生产价值                                                                 |
| -------------------------- | ----------------------------- | ------------------------------------------ | -------------------------------------------------------------------------------------- |
| **JSONL Persistence**      | `CapturedEventArchive` 内存环 | `~/.config/agent-ebpf-filter/events.jsonl` | 本地安全合规审计，长期无损历史事件冷增量固化。                                         |
| **Event Recording**        | 事件流快照 / 拓扑图帧         | 文件系统流化件或浏览器 Memory              | 针对高危黑客攻防、恶意脚本破坏过程的“帧级”重现与复盘。                                 |
| **Network Export**         | L4/L7 异步 Flow 存储字典      | 结构化 JSONL 或标准二进制 `.pcap` 报文     | 用于网络侧流量特征二次深度挖掘与标准 WireShark 工具链取证。                            |
| **AgentSight Export**      | `EventEnvelope` 统一上下文    | 标准多维 JSON / 结构化 CSV 格式            | 提供跨团队安全分析报告的高效转换，方便报表化交付。                                     |
| **OpenTelemetry**          | 归一化系统调用离散事实        | 注册的外部远程 OTLP 采集网关               | 将底层的系统行为泛化，向上折叠为企业级 `agent.run` 分布式链路 Spans。                  |
| **Prometheus**             | 系统监测器 / 控制器指标       | 宿主机只读探针端点 `/metrics`              | 对接 Grafana 仪表盘，长期监控拦截成功率、延迟及环形缓冲溢出度。                        |
| **Model Context Protocol** | 后端全量配置与行为拓扑        | HTTP 可流化 `/mcp` 协议端点                | 将控制面降维为标准的 Tools 集合，允许外部大模型（如 Cursor）通过指令自主决策阻断状态。 |

## 🔗 相关导航

- 🗺️ [总体架构](overview.md) —— 解构项目 L0-L5 分层设计依赖视图
- 🚰 [事件管线](../backend/event-pipeline.md) —— 详解从内核空间到用户空间的数据降噪算法
- 🧬 [协议与事件模型](protocol-events.md) —— 查看全量 Protobuf 序列化定义规范
- 🖥️ [前端工作台](../frontend/workbench.md) —— 掌握大并发实时事件流下的前端零防抖虚拟化渲染实现
- 🔌 [MCP、External API 与 OTLP](../integrations/mcp-external-otlp.md) —— 分离路径下智能体协议网关的双向握手细节说明
