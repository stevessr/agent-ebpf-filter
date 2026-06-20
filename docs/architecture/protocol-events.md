# 协议与事件模型

协议层是本项目跨 Go 后端、Vue 前端、adapters 和外部集成的一致性源头。

## Proto 文件分工

| 文件 | 作用 |
| --- | --- |
| `proto/tracker.proto` | 聚合 import，保持下游兼容 |
| `proto/tracker_common.proto` | 通用类型 |
| `proto/tracker_events.proto` | Event、EventEnvelope、各种 payload、event enum |
| `proto/tracker_registration.proto` | register / unregister |
| `proto/tracker_config.proto` | config / runtime / security / ML |
| `proto/tracker_shell.proto` | shell session |
| `proto/tracker_system.proto` | system stats / runtime system |

## Event 基础字段

`proto/tracker_events.proto` 中 `Event` 是统一事实记录，包含：

- 进程：`pid`、`ppid`、`uid`、`gid`、`tgid`、`comm`；
- syscall：`type`、`event_type`、`retval`、`duration_ns`、`extra_info`；
- 文件：`path`、`extra_path`、`mode`、`bytes`；
- 网络：`flow_id`、`src_ip`、`dst_ip`、`dst_port`、`transport`、`dns_name`、`sni`、`http_host`、`tls_alpn`；
- Agent 语义：`root_agent_pid`、`agent_run_id`、`task_id`、`tool_call_id`、`tool_name`、`trace_id`、`span_id`、`cwd`；
- 策略：`decision`、`risk_score`、`behavior`；
- 隐私：`argv_digest`、`redaction_level`、`sanitized_fields`；
- 网络统计：bytes / packets / first_seen / last_seen / geo / scope。

## EventEnvelope

EventEnvelope 将不同来源的事件统一包装，用于：

- `/ws/envelopes`；
- `/events/graph`；
- AgentSight import/export/query；
- OTLP span derivation；
- recording / replay；
- external API compatibility。

Envelope 语义上包含：

- schema version；
- timestamp；
- source；
- event id；
- legacy event；
- typed payload oneof；
- process / tool / trace context。

## 字段同步链

新增事件字段时必须同步：

```text
proto/tracker_events.proto
  → make proto
  → backend generated pb
  → frontend generated pb
  → adapters generated pb
  → backend event construction
  → EventEnvelope / Execution Graph / AgentSight / OTLP
  → frontend filters / table / modal / types
  → docs / tests
```

## 兼容原则

- 不复用已发布字段号；
- 新字段尽量 optional / additive；
- JSONL persistence 应兼容旧数据；
- generated files 不手改；
- `tracker.proto` 保持聚合入口，不承载领域细节。

---

## 相关导航

- [数据流](data-flow.md)
- [事件管线](../backend/event-pipeline.md)
- [生成文件边界](../reference/generated-files.md)
- [前端组件与 Composables](../frontend/components-composables.md)
- [代码入口索引](../reference/code-entrypoints.md)
