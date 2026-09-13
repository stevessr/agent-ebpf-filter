# eBPF Capture Timing and TLS Audit

这份文档说明 Agent eBPF Filter 中“捕获时间”和“后端观察时间”的口径，以及 TLS plaintext 捕获链路的审计/性能处理。

## 为什么要区分两个时间

系统层事件至少存在两种不同的时钟语义：

1. **probe/capture time**：事件在 eBPF probe 中发生的时间。TLS uprobe 使用 `bpf_ktime_get_ns()`，属于 Linux monotonic clock；
2. **ingest/observation time**：ring/perf buffer 中的事件被 Go 后端实际读取、解码或处理的时间，属于 wall clock。

两者不能混为一谈。若系统繁忙、perf buffer 堆积或用户态处理出现背压，ingest time 会晚于 probe time；这个差值本身就是捕获链路健康度的一部分。

## TLS timing audit tuple

TLS 事件现在额外携带：

- `probe_timestamp_ns`：eBPF 原始时间戳，不经过 wall-clock 改写；
- `probe_clock`：`monotonic`、`unix_replay` 或 `unknown`；
- `ingest_timestamp`：事件进入统一 TLS processing pipeline 时的 wall-clock 时间；
- `capture_delay_ns`：通过当前 `CLOCK_MONOTONIC` 与 probe timestamp 对齐得到的 kernel/probe → userspace 处理延迟；
- `timestamp`：用户可读的捕获时刻。HTTP stream assembler 已经保留更早的消息语义时间时，不会被最后一个 chunk 的时间覆盖。

离线测试和 replay 仍允许直接提供 Unix nanoseconds；这种记录会显式标为 `unix_replay`，避免和真实 `bpf_ktime_get_ns()` 混淆。

同一个完成的 TLS transport record 只做一次 monotonic/wall-clock 对齐。一个 HTTP/2 record 即使解析出多条 frame/SSE 事件，也会复用同一个 observation，避免按派生事件重复读取时钟。

现有 probe counter 接口还会附带以下无敏感数据的用户态健康指标：

- `capture_delay_samples`：已经完成时钟对齐的 TLS transport record 数；
- `capture_delay_last_ns`：最近一次 probe → userspace 延迟；
- `capture_delay_max_ns`：进程生命周期内观察到的最大延迟；
- `capture_clock_monotonic`：真实 eBPF monotonic 样本数；
- `capture_clock_replay`：Unix-ns replay 样本数；
- `capture_clock_unknown`：无法可靠对齐时钟的样本数。

`capture_delay_max_ns` 明显抬高、同时 `perf_output_fail` / lost samples 增长时，通常应优先排查 perf buffer 压力与后端消费速度。

## 主 tracker 的 audit observation window

主 syscall tracker 当前 raw ABI 尚未增加独立的 kernel timestamp 字段。为避免把接收时间伪装成内核时间，后端为解码后的 eBPF 事件补充一个明确的 observation window：

- `last_seen_ms`：若事件/flow 尚未提供该字段，写入后端解码/风险处理阶段的观察时间；
- `first_seen_ms`：若没有更权威的 flow 时间，则默认等于 observation time；如果 eBPF enter/exit correlation 已提供可信的 `duration_ns`，则用 `observation - duration` 反推近似 syscall start；
- 已由 network flow aggregator 生成的 first/last 时间不会被覆盖；
- `tcp_state_change` 历史 ABI 会复用 `duration_ns` 打包 old/new state，因此明确排除在 duration-based start estimation 之外。

因此：`first_seen_ms/last_seen_ms` 对主 tracker 是**审计观察窗口**，不是 raw kernel ktime。TLS 的 `probe_timestamp_ns` 才是当前链路里真正由 eBPF probe 直接提供的 monotonic timestamp。

## TLS reassembly fast path

TLS payload 会被 eBPF 拆成最多 18 个 perf fragment。用户态 assembler 过去为每个 fragment 创建 `map[uint16][]byte` 中的独立 byte slice，完成后再分配一份完整 payload 并逐块 append。

现在改为：

- 每个 pending message 只分配一次固定 slot buffer；
- 使用 32-bit bitset 记录已经到达的 fragment index（18 fragments 足够）；
- 每个 fragment 直接写入自己的固定 slot；
- 完成后在同一个 buffer 内原地 compact；
- 无 per-fragment map entry / byte-slice allocation；
- 无最终 full-payload 二次分配；
- 保持乱序 fragment、非满尺寸测试 fragment、duplicate 检测、超时清理和 pending cap 行为。

这部分优化主要降低高并发 HTTPS / agent streaming 场景下的 Go GC 压力和重组分配量，不改变 eBPF wire format，因此不需要改变现有 BPF object ABI。

## 审计时的解释建议

排查事件延迟时优先看：

```text
probe_timestamp_ns
       ↓ monotonic→wall alignment
capture timestamp
       ↓ capture_delay_ns
TLS processing / ingest_timestamp
       ↓ protocol assembly / policy / broadcast
frontend / recording / exporter
```

如果 `capture_delay_ns` 持续增长，应进一步查看：

- TLS perf lost samples / dropped fragments；
- fragment assembler pending / dropped；
- broadcaster queue full / write failure；
- 后端 CPU/GC 与客户端消费速度。

对于普通 syscall tracker，则先使用 `first_seen_ms / last_seen_ms / duration_ns` 建立审计窗口；不要把 `last_seen_ms` 描述为 kernel timestamp。
