# 事件管线

事件管线负责把内核、wrapper、hooks、TLS、policy、system metrics 等不同来源统一成 `pb.Event` 与 `EventEnvelope`。

## 内核事件读取

`backend/app/jobs_background.go` 中：

```mermaid
flowchart TD
    Start["startKernelEventReader(rd)"] --> Read["rd.Read()"]
    Read --> Decode["decodeBPFEventRecord(record.RawSample)"]
    Decode --> SelfFilter["self PID 过滤"]
    SelfFilter --> DisabledFilter["disabled comm / event type 过滤"]
    DisabledFilter --> Build["buildKernelEventFromRaw(event)"]
    Build --> Broadcast["broadcast &lt;- event"]
```

## 解码策略

`decodeBPFEventRecord()`：

- 如果 RawSample 长度不足，返回错误；
- 如果 native little-endian 且内存对齐，则直接构造 `*bpfEvent` view；
- 否则 `binary.Read` 到新结构体；
- 记录 zero-copy / copy 指标。

## Process context

`backend/app/events/context_event.go` 将注册、wrapper 和 hook 的上下文归一化。

| 构造函数 | 来源 |
| --- | --- |
| `buildProcessContextFromRegister()` | `/register` payload |
| `buildProcessContextFromWrapperRequest()` | `pb.WrapperRequest` |
| `buildProcessContextFromHookPayload()` | native hook JSON payload |
| `normalizeProcessContext()` | 去空白、规范 decision、补 root pid、清理 risk score |
| `enrichEventContext()` | 将 context 注入 `pb.Event` |

## Broadcast 与 archive

事件进入 broadcast channel 后，后端会：

- 广播给 `/ws`；
- 构造 EventEnvelope；
- 写入 CapturedEventArchive；
- 按配置写 JSONL；
- 推送 `/ws/envelopes`；
- 更新 Execution Graph / AgentSight / OTLP 派生数据；
- 触发可选 kernel risk feedback。

## EventArchive

`backend/core/state_types.go` 中 `EventArchive` 是 bounded ring：

- `Add()` 超出容量时裁掉最旧记录；
- `Snapshot(limit)` 返回最新 N 条；
- `EvictOlderThan()` 按时间清理；
- `SetMax()` 动态调整容量；
- `Clear()` 清空内存记录。

## 语义关联状态

`BuildSemanticAlerts()` 会跨事件关联 secret access、chmod 后执行、fork window、
agentic resource loop 和多 Agent 文件写入。关联状态不能随长期运行或攻击者控制的
context/path 无限增长，因此 `SemanticAlertState` 使用五个受同一互斥锁保护的有界 LRU：

- secret、executable、fork 和 agentic-loop 各最多 4096 个 context；
- file-mutation 最多 8192 个 path；总容量上限为 24576；
- context、path、target、mode 和 prompt digest 都有字节上限，超长值使用稳定 SHA-256
  后缀保留区分度，不把原始大值留在状态中；
- `extra_info` 采用流式字段扫描，只检查前 64 KiB，prompt digest 最多 256 bytes，
  不再复制并切分整段 metadata；
- 与 prompt/API/file-I/O correlation 无关的事件在创建 key 和加锁前返回，不写入空 window；
- 每 5 秒由可取消的后台任务扫描 TTL，按容量淘汰则在插入时以 O(1) 完成。

`/system/collector-health` 暴露各类 entry 数、总量/上限、TTL/容量淘汰、受限值、
忽略的超限 metadata 和最后 GC 时间。Prometheus 对应暴露
`agent_ebpf_semantic_state_entries{kind=...}`、`*_max_entries` 及四个累计 counter。

## 异步 JSONL 持久化

`recordCapturedEvent()` 在完成 clone、schema 归一化和脱敏后，先写入内存
`EventArchive`，再把同一条记录非阻塞地提交给持久化 writer。事件捕获热路径不再执行
JSON 编码、文件写入或逐条 `Flush()`。

持久化 writer 的边界如下：

- 单消费者队列容量为 4096；队列满时只丢弃该条持久化副本，不阻塞 broadcast、
  WebSocket、录制或其他派生 worker；
- 使用 256 KiB 用户态缓冲区，按 128 条或 250 ms 批量刷盘；
- 单条记录复用录制管线的约 4 MiB JSONL 上限；单条编码失败只记失败并继续处理，
  文件写入/刷盘失败则终止当前 writer generation；
- 配置替换、禁用、清空日志和后端停机会先停止接收，并在 5 秒期限内排空已接受记录；
- 相同日志配置不会重启 writer；切换路径先准备新文件，再排空旧 generation，配置保存
  失败时回滚原 writer；
- 累计成功/失败计数只在实际刷盘完成后更新，不再把“未启用持久化”记为成功。

`/system/collector-health` 暴露 writer active/stopping、queue length/capacity、
pending、当前 generation 的 enqueued/persisted/failed/dropped、最后刷盘时间与最后错误。
Prometheus 同时暴露 active、queue、pending 和 generation failure/drop gauges。

## 持久化尾读

`RecentEventsContext()` 在读取持久化文件前插入 flush barrier，因此 barrier 之前已接受的
记录对本次读取可见。读取器基于请求开始时的文件大小快照，从文件尾部按 256 KiB
反向读取，只解析满足 `limit` 所需的最新有效 JSONL，再恢复时间顺序。它限制：

- 单次最多返回 50000 条（具体 HTTP/MCP 调用方可进一步收紧）；
- 单行最多约 4 MiB；
- 最多检查 250000 行和 128 MiB；
- 全程检查请求取消信号，并统一使用 10 秒处理期限。

取消会直接向上传播，不再继续扫描或写响应。非取消类文件读取失败会记录警告并回退到
有界内存 archive；格式错误或缺少 event 的单行会被跳过。

## Event schema

当前 `eventSchemaVersion` 是 `event.v3`。事件字段变化时必须同步 proto、生成物、后端构造、前端显示、图谱、AgentSight、OTLP 和 docs。

## 跨文档影响

事件管线是多条文档路线的交汇点：

| 变化 | 同步阅读 / 更新 |
| --- | --- |
| 新增 syscall event type 或字段 | [协议与事件模型](/architecture/protocol-events)、[生成文件边界](/reference/generated-files)、[eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement) |
| 新增 wrapper / hook / adapter context 字段 | [Agents、Adapters 与 PID 注册](/integrations/agents)、[Wrapper 命令策略](/integrations/wrapper)、[Native Hooks](/integrations/native-hooks) |
| 改 EventEnvelope、AgentSight projection 或 replay | [前端工作台](/frontend/workbench)、[路由与功能页](/frontend/routes-and-pages)、[MCP、External API 与 OTLP](/integrations/mcp-external-otlp) |
| 改 redaction / TLS / Codex ingest | [脱敏与隐私](/security/redaction-privacy)、[Sanitization](../security/sanitization.md)、[安全模型](/security/model) |
| 改 kernel risk scoring / feedback | [ML、Plugins 与扩展能力](/backend/ml-plugins)、[策略语义](/security/policy-semantics)、[验证页](/operations/verification-benchmark) |

维护建议：先用本页确认事件从哪里进入、在哪里归一化、广播到哪些出口，再根据 [文档地图](/reference/documentation-map) 的“变更影响链”检查是否漏掉前端、外部 API 或安全说明。
