# Agents、Adapters 与 PID 注册

Agents 通过 adapters 或外部调用注册当前进程 PID，使后续内核事件能关联到 Agent run / task / trace。

## 支持的 adapters

- Python：`adapters/python/agent_tracker.py`
- JavaScript：`adapters/js/agentTracker.js`

## 注册流程

```text
Agent process
  → adapter register()
  → POST /register
  → backend handler
  → processContextStore.Set(pid, context)
  → agent_pids BPF map
  → eBPF event attribution
```

## 上下文字段

注册 payload 可携带：

- `pid`
- `root_agent_pid`
- `agent_run_id`
- `task_id`
- `conversation_id`
- `turn_id`
- `tool_call_id`
- `tool_name`
- `trace_id`
- `span_id`
- `decision`
- `risk_score`
- `container_id`
- `argv_digest`
- `cwd`

## 语义边界

- PID registration 是 per-process；
- 子进程关联依赖 fork/clone lineage 和 userspace parent fallback；
- adapter 不是自动递归注册所有后代的 daemon；
- release mode 下 `/register` 和 `/unregister` 需要 auth。
