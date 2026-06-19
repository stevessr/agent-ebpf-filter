# Wrapper 命令策略

`agent-wrapper` 是命令 shim / policy layer，用于在命令执行前询问后端策略。

## 源码

- `wrapper/main.go`
- `backend/app/*uds*`
- `backend/app/*behavior*`
- `backend/app/*path_policy*`

## 执行流程

```text
agent-wrapper <command> [args...]
  → trim args
  → net.DialTimeout("unix", "/tmp/agent-ebpf.sock", 500ms)
  → pb.WrapperRequest
  → backend policy decision
  → pb.WrapperResponse
  → handleDecision()
  → syscall.Exec()
```

## 决策

| Action | 行为 |
| --- | --- |
| ALLOW | 继续执行原命令 |
| BLOCK | 打印 `Execution Blocked`，退出 1 |
| ALERT | 打印 `Security Alert`，继续执行 |
| REWRITE | 使用 `RewrittenArgs` 替换命令和参数 |

## Metadata

Wrapper 从环境变量中提取 Agent context：

- `AGENT_EBPF_AGENT_RUN_ID` / `AGENT_RUN_ID`
- `AGENT_EBPF_TASK_ID` / `AGENT_TASK_ID`
- `AGENT_EBPF_CONVERSATION_ID` / `AGENT_CONVERSATION_ID`
- `AGENT_EBPF_TOOL_CALL_ID` / `AGENT_TOOL_CALL_ID`
- `AGENT_EBPF_TRACE_ID` / `TRACE_ID`
- `AGENT_EBPF_RISK_SCORE` / `AGENT_RISK_SCORE`
- `AGENT_EBPF_CWD` / `PWD`

并计算 `ArgvDigest`。

## 安全边界

- UDS socket 应 restrictive；
- backend 应验证 peer credentials；
- policy mutation 需要 runtime gate；
- wrapper 只覆盖经它启动的命令。
