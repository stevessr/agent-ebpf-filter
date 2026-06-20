# 数据流

本页描述 Agent eBPF Filter 的核心数据流：内核事件流、wrapper 策略流、native hook 流、前端配置流、导出流。

## eBPF 事件流

```mermaid
sequenceDiagram
    participant Syscall as Linux Syscall
    participant eBPF as eBPF Tracepoint
    participant Maps as Pinned Maps
    participant Ringbuf as Ringbuf
    participant Reader as Go Ringbuf Reader
    participant Decoder as decodeBPFEventRecord
    participant Builder as buildKernelEventFromRaw
    participant Broadcast as Broadcast Channel
    participant Archive as Event Archive
    participant WS as WebSocket /ws
    participant Vue as Vue Dashboard
    
    Syscall->>eBPF: execve/openat/connect/...
    eBPF->>Maps: check tracked_comms/paths/pids
    Maps-->>eBPF: match result
    eBPF->>Ringbuf: submit event (zero-copy)
    Ringbuf->>Reader: RawSample available
    Reader->>Decoder: decode(RawSample)
    
    alt native little-endian + aligned
        Decoder->>Decoder: mmap-backed view (zero-copy)
    else fallback
        Decoder->>Decoder: binary.Read (copy)
    end
    
    Decoder->>Builder: raw bpfEvent
    Builder->>Builder: enrich process context
    Builder->>Broadcast: pb.Event
    Broadcast->>Archive: add to ring buffer
    Broadcast->>WS: broadcast to clients
    WS->>Vue: real-time event
    Archive->>Archive: optional JSONL write
```

关键文件：

- `backend/ebpf/agent_tracker.c`
- `backend/ebpf/agent_tracker_common.h`
- `backend/app/runtime__jobs_background.go`
- `backend/app/events__context_event.go`
- `backend/app/events__graph_execution.go`
- `proto/tracker_events.proto`

### ringbuf decode

后端 `decodeBPFEventRecord()` 会优先将 ringbuf RawSample 解释为 mmap-backed `bpfEvent` view：

```go
// 伪代码示例
func decodeBPFEventRecord(sample []byte) (*bpfEvent, error) {
    if len(sample) < minEventSize {
        return nil, ErrTooShort
    }
    
    // 条件：native little-endian + alignment
    if isNativeLittleEndian && isAligned(sample) {
        // zero-copy: 直接构造 view
        evt := (*bpfEvent)(unsafe.Pointer(&sample[0]))
        collectorMetricsStore.RecordRingbufDecode(true)
        return evt, nil
    }
    
    // fallback: copy path
    evt := &bpfEvent{}
    if err := binary.Read(bytes.NewReader(sample), binary.LittleEndian, evt); err != nil {
        return nil, err
    }
    collectorMetricsStore.RecordRingbufDecode(false)
    return evt, nil
}
```

这使热路径在常见 Linux x86_64 环境下减少复制成本，同时保留跨平台 fallback。

## Wrapper 策略流

```mermaid
sequenceDiagram
    participant User as User/Agent
    participant Wrapper as agent-wrapper
    participant UDS as /tmp/agent-ebpf.sock
    participant Engine as Backend Policy Engine
    participant Exec as syscall.Exec
    
    User->>Wrapper: agent-wrapper git push
    Wrapper->>Wrapper: trim args, extract env metadata
    Wrapper->>UDS: dial Unix socket
    Wrapper->>Engine: WrapperRequest(pid, comm, args, metadata)
    Engine->>Engine: evaluate rules + ML risk score
    
    alt ALLOW
        Engine-->>Wrapper: WrapperResponse(ALLOW)
        Wrapper->>Exec: exec git push
    else BLOCK
        Engine-->>Wrapper: WrapperResponse(BLOCK)
        Wrapper->>User: ❌ Execution Blocked
    else ALERT
        Engine-->>Wrapper: WrapperResponse(ALERT)
        Wrapper->>User: ⚠️ Security Alert
        Wrapper->>Exec: exec git push
    else REWRITE
        Engine-->>Wrapper: WrapperResponse(REWRITE, new_args)
        Wrapper->>Exec: exec <rewritten command>
    end
```

Wrapper 是命令 shim / policy layer。它不是完整 sandbox；它只覆盖经 wrapper 调用的命令路径。

## Native Hook 流

```mermaid
sequenceDiagram
    participant CLI as AI CLI (Claude/Gemini/Codex)
    participant Hook as Hook System
    participant Relay as Generated Relay Script
    participant Backend as Backend /hooks/event
    participant Normalize as Normalize Payload
    participant Context as Process Context Store
    participant Event as Event Pipeline
    
    CLI->>Hook: tool execution triggers hook
    Hook->>Relay: stdin payload (JSON)
    Relay->>Backend: curl POST /hooks/event
    Backend->>Backend: hookIngressAuthMiddleware (token or hook secret)
    Backend->>Normalize: raw JSON payload
    Normalize->>Normalize: extract tool_name, target_path, phase
    Normalize->>Context: enrichProcessContext
    Context-->>Normalize: root_agent_pid, trace_id
    Normalize->>Event: native_hook pb.Event
    Event->>Event: wrap in EventEnvelope
    Event->>Event: archive + broadcast + OTLP
```

Hooks 的价值是提供 AI CLI 语义，例如工具名、目标路径、摘要、长度和 hook phase。eBPF 仍提供系统事实。

## PID Registration 流

```mermaid
sequenceDiagram
    participant Agent as Parent Agent
    participant Child as Child Process
    participant Adapter as Adapter (register)
    participant Backend as Backend /register
    participant Store as processContextStore
    participant BPFMap as agent_pids BPF Map
    participant Tracker as eBPF Tracker

    %% 阶段 1：明确的单进程注册
    Note over Agent, BPFMap: 1. 显式 Per-Process 注册 (仅限当前进程)
    Agent->>Adapter: import agent_tracker 并调用 register()
    Adapter->>Adapter: collect PID, agent_run_id, trace_id
    Adapter->>Backend: POST /register (pb.RegisterRequest)
    Backend->>Store: Set(pid, ProcessContext)
    Backend->>BPFMap: seed agent_pids[pid] = 1
    Backend-->>Adapter: 200 OK

    %% 阶段 2：内核态的继承与谱系树建立
    Note over Agent, Tracker: 2. 子进程衍生 (依赖 fork/clone 谱系，无 Adapter 参与)
    Agent->>Tracker: fork 或 clone 系统调用
    Tracker->>BPFMap: lookup agent_pids[parent_pid] (命中)
    Tracker->>BPFMap: update agent_pids[child_pid] = 1 (内核态动态传播)
    Tracker->>Store: 异步上报 fork/clone 事件 (parent_pid -> child_pid)
    Store->>Store: 构建/更新进程树谱系 (Lineage)

    %% 阶段 3：子进程事件触发与兜底富化
    Note over Child, Store: 3. 子进程执行与 Backend Fallback 富化
    Child->>Tracker: execve / openat / connect 等系统调用
    Tracker->>BPFMap: lookup agent_pids[child_pid] -> found (已追踪)
    Tracker->>Store: 上报业务事件 (携带 child_pid)
    
    alt 直连上下文命中
        Store->>Store: 直接富化当前进程 Context
    else 触发 Backend Fallback (追溯谱系)
        Store->>Store: 沿 Lineage 树向上追溯父进程
        Store->>Store: 固定继承 ParentContext.agent_run_id
    end
```

注册语义是 per-process。子进程关联依赖 fork/clone lineage 与 backend fallback，不应描述成 adapter 自动递归注册所有后代。

## 前端配置流

```mermaid
sequenceDiagram
    participant User as User
    participant View as Vue Config View
    participant Composable as useConfigSecurity
    participant API as Backend /config/runtime
    participant Auth as authMiddleware
    participant Store as runtimeSettingsStore
    participant BPFMap as policy BPF Maps
    participant WS as WebSocket Broadcast
    
    User->>View: 修改 cgroup block IP
    View->>Composable: blockCgroupIP(ip)
    Composable->>API: POST /sandbox/cgroup/block-ip
    API->>Auth: check release token
    Auth-->>API: authorized
    API->>API: check policy_management gate
    API->>BPFMap: write cgroup_blocked_ips[ip] = 1
    BPFMap-->>API: success
    API->>Store: update in-memory state
    API->>WS: broadcast policy_change event
    API-->>Composable: 200 OK
    Composable->>View: refresh UI counters
    WS->>View: real-time policy update
```

示例：Config Security 页面操作 cgroup / LSM policy 时，需要：

1. release mode token；
2. policy management runtime gate；
3. 对应 build feature compiled in；
4. 后端 API 写入 map；
5. UI 刷新 status / counters。

## 导出流

```mermaid
graph TB
    subgraph "Event Sources"
        Archive["Event Archive<br/>(memory ring)"]
        Flows["Network Flows"]
        Graph["Execution Graph"]
        TLS["TLS/Codex Capture"]
        Metrics["Collector Metrics"]
    end
    
    subgraph "Export Targets"
        JSONL["JSONL File<br/>~/.config/.../events.jsonl"]
        Recording["Recording File<br/>browser/file replay"]
        PCAP["PCAP Export"]
        AgentSight["AgentSight JSON/CSV"]
        OTLP["OTLP Collector<br/>spans"]
        Prometheus["Prometheus<br/>/metrics"]
        MCP["MCP Tools<br/>/mcp"]
    end
    
    Archive -->|persistence enabled| JSONL
    Archive -->|record session| Recording
    Flows -->|export flows| PCAP
    Graph -->|export graph| AgentSight
    Archive -->|derive spans| OTLP
    Metrics -->|scrape| Prometheus
    Archive -->|tool: tail_events| MCP
    Graph -->|tool: query_graph| MCP
```


| 导出 | 源 | 目标 |
| --- | --- | --- |
| JSONL persistence | CapturedEventArchive | `~/.config/agent-ebpf-filter/events.jsonl` |
| Event recording | event archive / graph snapshots | file-backed 或 browser-memory replay |
| Network export | flow store | JSONL / PCAP |
| AgentSight export | EventEnvelope / TLS / metrics | JSON / JSONL / CSV |
| OTLP | derived spans | OTLP HTTP endpoint |
| Prometheus | collector/system metrics | `/metrics` |
| MCP | config/event APIs | `/mcp` streamable HTTP |

---

## 相关导航

- [总体架构](overview.md)
- [事件管线](../backend/event-pipeline.md)
- [协议与事件模型](protocol-events.md)
- [前端工作台](../frontend/workbench.md)
- [MCP、External API 与 OTLP](../integrations/mcp-external-otlp.md)
