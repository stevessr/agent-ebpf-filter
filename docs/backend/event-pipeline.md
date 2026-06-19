# 事件管线

事件管线负责把内核、wrapper、hooks、TLS、policy、system metrics 等不同来源统一成 `pb.Event` 与 `EventEnvelope`。

## 内核事件读取

`backend/app/runtime__jobs_background.go` 中：

```text
startKernelEventReader(rd)
  → rd.Read()
  → decodeBPFEventRecord(record.RawSample)
  → self PID 过滤
  → disabled comm / event type 过滤
  → buildKernelEventFromRaw(event)
  → broadcast <- event
```

## 解码策略

`decodeBPFEventRecord()`：

- 如果 RawSample 长度不足，返回错误；
- 如果 native little-endian 且内存对齐，则直接构造 `*bpfEvent` view；
- 否则 `binary.Read` 到新结构体；
- 记录 zero-copy / copy 指标。

## Process context

`backend/app/events__context_event.go` 将注册、wrapper 和 hook 的上下文归一化。

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

## Event schema

当前 `eventSchemaVersion` 是 `event.v3`。事件字段变化时必须同步 proto、生成物、后端构造、前端显示、图谱、AgentSight、OTLP 和 docs。
