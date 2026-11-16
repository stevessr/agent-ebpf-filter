# 统一 Agent 消息驱动风险处理管线（第一阶段）

## 设计选择

保留现有 **一个消息摄取队列（appContext.Broadcast）和一个消费者（runEventBroadcaster）**。不再引入一个可能乱序或丢失关联事件的第二条异步队列。所有 eBPF、Native Hook、MCP、TLS、wrapper 及其他可归一化的 `pb.Event`，经 capture scope 与 process/cgroup attribution 后进入统一处理阶段：

```mermaid
flowchart LR
    A[eBPF / Hook / MCP / TLS / Wrapper] --> Q[Bounded Broadcast Queue]
    Q --> C[Scope Filter + Process Context]
    C --> N[Normalized Agent Message]
    N --> R[Bounded Session / Tool Correlation]
    N --> E[Existing Semantic Rules]
    R --> F[Normalized Findings]
    E --> F
    F --> D[EventEnvelope / Archive / WS / Execution Graph]
```

`internal/agentpipeline.Message` 只保存规范化类型、操作、进程与会话标识、显式工具权限和不可信信号；**不保存 prompt、tool arguments、模型输出或 TLS 明文**。原有 protobuf 与 WS schema 不变，派生风险统一由 `semantic_alert` 表示。

## 上下文作用域及关联

优先用 `agent_run_id + task_id + conversation_id + root_agent_pid`；其次是 conversation/trace 与 root PID，最后回退 root PID 或 PID。ToolCallID **只在会话内部**索引，避免不同 Agent 同名调用串线；无父级身份的 ToolCallID 不参与关联。来源未携带一致 run/root/trace 时会漏关联，不能靠猜测补齐。

- 默认最大 4096 个活跃会话，每会话 64 条 tool call，LRU 淘汰；TTL 5 分钟。
- 只接受当前流中的监控范围事件；按原事件顺序处理；重复结果每条因果链只告警一次。
- 提示词注入链要求上游**明确**标记为 external/untrusted/tool_output 且同时标记 high/critical 或 prompt_injection_detected；单纯接触网页/工具输出不判恶意。
- 工具读写权限必须由可信 Hook 明确传输 `side_effect_scope=read_only` 等元数据；不从工具名臆断。
- 计划模式 `permission_mode=plan` 后观测到实际副作用单独生成风险事件。
- `BLOCK` 与失败 syscall 不计作成功的写入、执行或外联行为。
- 不执行自动阻断，所有新 findings 为审计告警，避免因假阳性修改已有内核策略。

## 新增告警

| Code | 证据要求 |
| --- | --- |
| `TOOL_SCOPE_VIOLATION` | 同一 scoped ToolCallID 明确只读，却观察到实际 write/exec/egress |
| `PLAN_MODE_SIDE_EFFECT` | 同一 scoped ToolCallID 明确 plan mode，却观察到副作用 |
| `UNTRUSTED_INPUT_ACTION_CHAIN` | 同一 Agent 上游明确高风险不可信输入，后续出现副作用 |
| `TOOL_CALL_ID_CONFLICT` | 同一作用域的调用 ID 在活跃窗口内被不同工具复用 |

## 兼容、限制与验证

- 新代码不改变 `proto/tracker_events.proto`、历史事件归档、WebSocket 订阅 API 或现有语义规则。
- Hook 中 `risk_level` / `source` / `side_effect_scope` / `permission_mode` 作为已有压缩元数据读取；新增可选 `prompt_injection_detected=true` 只作为标记，不传输原文。
- 依赖因果字段的告警不能代替跨主机分布式追踪；多服务场景需要进一步引入带认证的传输消息 ID、watermark 与落盘重放。
- 本阶段的 queue full 仍按原采集策略记录丢弃，**不声称 at-least-once 或 exactly-once**。
- 单元测试覆盖：跨 run 隔离、工具只读冲突、显式高风险链、过期及容量、重复抑制、并发竞争和 BLOCK 行为。
- 运行 `cd backend && go test ./internal/agentpipeline ./app/...`；完整 CI 仍需 Go 1.27.3 与实际 eBPF 环境。


## 第二阶段：生命周期、作用域隔离和健康指标

### 工具调用状态机

`tool_call` 打开一个只在当前 Agent session 有效的 invocation；`tool_result` 将调用标记为已完成，**不会立即丢弃**，允许最多 2 秒的迟到内核事件关联（以摄取时间为准，重复结果不能延长宽限期）。后续同 ID 但不同 Turn 或新的调用会建立新的状态，不继承旧调用的只读/plan 声明。原有的 5 分钟 TTL 是状态缓存上限，不是已完成工具的活动期限。

`SessionEnd` 或明确的根 Agent 进程退出清理对应 session，普通子进程退出**不**终止整个 Agent run。收不到生命周期结束事件的上下文仍由 TTL 和 LRU 回收。

### 身份与证据安全

- 关联状态键采用按字段长度分帧的 SHA-256 派生短标识，并纳入可用的 container ID，避免分隔符构造和跨容器的 Agent ID/PID 重用碰撞，同时不在 LRU 状态里保留原始会话标识。
- 旧版语义告警的 `tool_call_id / task_id / trace_id` 现在尽可能与 Agent run、根 PID 或进程 PID 结合，而非跨 Agent 共用裸标识。
- 如果事件缺少可核实的 Run/Conversation/Root 证据，**宁可失去一部分关联，也不将不同 Agent 的工具行为错误拼接**。PID-only 是退化路径，需结合 session exit 和生命周期判断。
- 高风险不可信输入仍要求上游显式证据；单纯 `BeforeModel`、普通工具结果和无结果的 MCP 工具声明，不产生新的关联状态。
- `write` 必须报告正的写入字节数才计作数据修改；失败或拦截的系统调用不算已发生的副作用。
- 目前的 `semantic_alert` 是观测到的证据链风险，**不是对恶意意图的确定性证明**，自动阻断仍须独立测试和准入门槛。

### 健康接口

`GET /system/collector-health` 附加可选的 `agentRiskPipeline` 字段：

```json
{
  "agentRiskPipeline": {
    "sessions": 12,
    "maxSessions": 4096,
    "evictions": 0,
    "findings": 3,
    "activeCalls": 4,
    "completedCalls": 8,
    "taintedSessions": 1
  }
}
```

以上仅说明字段与示例值，不是测量结果。Renew/WebUI 可按需使用这些统计字段；现有客户端可忽略它们。

### 回归测试与后续工作

新增的回归测试覆盖生命周期完成窗口、重复 tool result、SessionEnd、根/子进程退出、多 Agent 的相同调用 ID、同 Run 不同 Turn、恶意分隔符构造的标识、过长作用域键、零字节写入、MCP/TLS 元数据与 collector health 统计。

仍需在真实内核与多 Agent 工具链上采集生产级 replay 样本，定量评估误报、漏报、p95/p99 处理时延、队列丢弃与跨进程关联质量。当前消息总线仍是单进程无持久重放的现有队列，**不得宣称 exactly-once 或可跨主机可靠交付**。
