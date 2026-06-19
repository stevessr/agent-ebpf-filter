# 数据流

本页描述 Agent eBPF Filter 的核心数据流：内核事件流、wrapper 策略流、native hook 流、前端配置流、导出流。

## eBPF 事件流

```text
tracked PID / comm / path
  → eBPF tracepoint / cgroup / LSM program
  → pinned BPF maps + ringbuf
  → Go backend ringbuf reader
  → decodeBPFEventRecord()
  → buildKernelEventFromRaw()
  → broadcast channel
  → startEventBroadcaster()
  → pb.Event + EventEnvelope
  → archive / JSONL / WS / OTLP / MCP / AgentSight
  → Vue Dashboard / Network / ExecutionGraph / AgentSight
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

- 条件：host native little-endian 且样本地址满足 alignment；
- 否则：使用 `binary.Read` copy path；
- 记录指标：`collectorMetricsStore.RecordRingbufDecode(zeroCopy)`。

这使热路径在常见 Linux x86_64 环境下减少复制成本，同时保留跨平台 fallback。

## Wrapper 策略流

```text
用户 / Executor / Agent
  → agent-wrapper <command> [args]
  → 清理空白 args
  → 连接 /tmp/agent-ebpf.sock
  → pb.WrapperRequest(pid, comm, args, metadata)
  → backend policy engine
  → pb.WrapperResponse(action)
  → BLOCK / ALERT / REWRITE / ALLOW
  → syscall.Exec(final command)
```

Wrapper 是命令 shim / policy layer。它不是完整 sandbox；它只覆盖经 wrapper 调用的命令路径。

## Native Hook 流

```text
AI CLI hook stdin payload
  → generated relay script
  → curl POST /hooks/event
  → hookIngressAuthMiddleware()
  → handleNativeHookEvent()
  → normalize payload
  → processContext enrichment
  → native_hook event
  → EventEnvelope / Dashboard / AgentSight / OTLP / persistence
```

Hooks 的价值是提供 AI CLI 语义，例如工具名、目标路径、摘要、长度和 hook phase。eBPF 仍提供系统事实。

## PID Registration 流

```text
Python / Node agent
  → adapter register current PID
  → POST /register
  → processContextStore.Set(pid, context)
  → agent_pids BPF map seed
  → fork/clone lineage + userspace parent fallback
  → 后续 syscall event 获得 Agent context
```

注册语义是 per-process。子进程关联依赖 fork/clone lineage 与 backend fallback，不应描述成 adapter 自动递归注册所有后代。

## 前端配置流

```text
Vue view
  → domain composable
  → HTTP / WebSocket API
  → auth token / ?key=...
  → backend handler
  → runtimeSettingsStore / BPF map / policy store
  → response / broadcast
  → UI state refresh
```

示例：Config Security 页面操作 cgroup / LSM policy 时，需要：

1. release mode token；
2. policy management runtime gate；
3. 对应 build feature compiled in；
4. 后端 API 写入 map；
5. UI 刷新 status / counters。

## 导出流

| 导出 | 源 | 目标 |
| --- | --- | --- |
| JSONL persistence | CapturedEventArchive | `~/.config/agent-ebpf-filter/events.jsonl` |
| Event recording | event archive / graph snapshots | file-backed 或 browser-memory replay |
| Network export | flow store | JSONL / PCAP |
| AgentSight export | EventEnvelope / TLS / metrics | JSON / JSONL / CSV |
| OTLP | derived spans | OTLP HTTP endpoint |
| Prometheus | collector/system metrics | `/metrics` |
| MCP | config/event APIs | `/mcp` streamable HTTP |
