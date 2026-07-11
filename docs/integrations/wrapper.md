# Wrapper 命令策略

`agent-wrapper` 是命令 shim / policy layer，用于在命令执行前询问后端策略。

## 源码

- `wrapper/main.go`
- `backend/app/*uds*`
- `backend/app/*behavior*`
- `backend/app/*path_policy*`

## 执行流程

```mermaid
sequenceDiagram
    participant User as User/Agent
    participant Wrapper as agent-wrapper
    participant Env as Environment
    participant UDS as /tmp/agent-ebpf.sock
    participant Backend as Backend Policy Engine
    participant Rules as Policy Rules Store
    participant ML as ML Risk Scorer
    participant Exec as syscall.Exec
    
    User->>Wrapper: agent-wrapper git push origin main
    Wrapper->>Wrapper: trim whitespace args
    Wrapper->>Env: read AGENT_RUN_ID, TRACE_ID, etc.
    Env-->>Wrapper: context metadata
    Wrapper->>Wrapper: compute argv_digest = sha256(args)
    
    Wrapper->>UDS: dial Unix socket (500ms timeout)
    UDS-->>Wrapper: connected
    Wrapper->>Backend: length-prefix + WrapperRequest{<br/>  pid: getpid()<br/>  comm: "git"<br/>  args: ["push","origin","main"]<br/>  metadata: {...}<br/>  argv_digest: "abc123..."<br/>}
    
    Backend->>Rules: lookup wrapper rules
    Rules-->>Backend: matching rules
    Backend->>ML: evaluate risk_score
    ML-->>Backend: 0.85 (high risk)
    
    alt risk_score > threshold
        Backend-->>Wrapper: WrapperResponse{action: BLOCK}
        Wrapper->>User: ❌ Execution Blocked: git push<br/>Risk score: 0.85
        Wrapper->>Wrapper: os.Exit(1)
    else rule matches + action=ALERT
        Backend-->>Wrapper: WrapperResponse{action: ALERT}
        Wrapper->>User: ⚠️ Security Alert: sensitive operation
        Wrapper->>Exec: exec git push origin main
    else rule matches + action=REWRITE
        Backend-->>Wrapper: WrapperResponse{<br/>  action: REWRITE<br/>  rewritten_args: ["push","--dry-run","origin","main"]<br/>}
        Wrapper->>User: ℹ️ Command rewritten (added --dry-run)
        Wrapper->>Exec: exec git push --dry-run origin main
    else default ALLOW
        Backend-->>Wrapper: WrapperResponse{action: ALLOW}
        Wrapper->>Exec: exec git push origin main
    end
```

## 决策类型

```mermaid
graph TB
    Request[WrapperRequest] --> Engine[Policy Engine]
    Engine --> Eval{evaluate rules}
    
    Eval --> Check1{high risk?}
    Check1 -->|Yes| Block[BLOCK<br/>exit 1]
    Check1 -->|No| Check2{alert rule?}
    
    Check2 -->|Yes| Alert[ALERT<br/>print + exec]
    Check2 -->|No| Check3{rewrite rule?}
    
    Check3 -->|Yes| Rewrite[REWRITE<br/>modify args]
    Check3 -->|No| Allow[ALLOW<br/>exec original]
    
    style Block fill:#fbb
    style Alert fill:#ffb
    style Rewrite fill:#bbf
    style Allow fill:#bfb
```

| Action | 行为 |
| --- | --- |
| ALLOW | 继续执行原命令 |
| BLOCK | 打印 `Execution Blocked`，退出 1 |
| ALERT | 打印 `Security Alert`，继续执行 |
| REWRITE | 使用 `RewrittenArgs` 替换命令和参数 |

## Metadata 提取

Wrapper 从环境变量中提取 Agent context：

```go
// wrapper/main.go 伪代码
func extractMetadata() *WrapperMetadata {
    return &WrapperMetadata{
        AgentRunID:      getEnv("AGENT_EBPF_AGENT_RUN_ID", "AGENT_RUN_ID"),
        TaskID:          getEnv("AGENT_EBPF_TASK_ID", "AGENT_TASK_ID"),
        ConversationID:  getEnv("AGENT_EBPF_CONVERSATION_ID", "AGENT_CONVERSATION_ID"),
        ToolCallID:      getEnv("AGENT_EBPF_TOOL_CALL_ID", "AGENT_TOOL_CALL_ID"),
        TraceID:         getEnv("AGENT_EBPF_TRACE_ID", "TRACE_ID"),
        RiskScore:       parseFloat(getEnv("AGENT_EBPF_RISK_SCORE", "AGENT_RISK_SCORE")),
        Cwd:             getEnv("AGENT_EBPF_CWD", "PWD"),
    }
}
```

环境变量优先级：

- `AGENT_EBPF_AGENT_RUN_ID` / `AGENT_RUN_ID`
- `AGENT_EBPF_TASK_ID` / `AGENT_TASK_ID`
- `AGENT_EBPF_CONVERSATION_ID` / `AGENT_CONVERSATION_ID`
- `AGENT_EBPF_TOOL_CALL_ID` / `AGENT_TOOL_CALL_ID`
- `AGENT_EBPF_TRACE_ID` / `TRACE_ID`
- `AGENT_EBPF_RISK_SCORE` / `AGENT_RISK_SCORE`
- `AGENT_EBPF_CWD` / `PWD`

并计算 `ArgvDigest`：

```go
func computeArgvDigest(args []string) string {
    joined := strings.Join(args, " ")
    hash := sha256.Sum256([]byte(joined))
    return hex.EncodeToString(hash[:])
}
```

## 配置示例

典型 wrapper rule 配置：

```json
{
  "rules": [
    {
      "name": "block-rm-rf",
      "command_pattern": "rm",
      "args_pattern": ".*-rf.*",
      "action": "BLOCK",
      "reason": "Dangerous recursive delete"
    },
    {
      "name": "alert-git-push",
      "command_pattern": "git",
      "args_pattern": "push.*",
      "action": "ALERT",
      "reason": "Pushing to remote repository"
    },
    {
      "name": "rewrite-npm-install-add-audit",
      "command_pattern": "npm",
      "args_pattern": "install",
      "action": "REWRITE",
      "rewrite_template": ["install", "--audit"],
      "reason": "Force npm audit on install"
    }
  ]
}
```

## 安全边界

```mermaid
graph TB
    Wrapper[agent-wrapper] -->|restrictive| UDS[Unix Socket<br/>/tmp/agent-ebpf.sock]
    UDS -->|peer cred check| Backend[Backend]
    Backend -->|requires| Auth[auth token<br/>release mode]
    Backend -->|requires| Gate[policy_management<br/>runtime gate]
    Backend -->|reads| Rules[Policy Rules]
    Backend -->|writes| Audit[Audit Log]
    
    Note1[只覆盖经 wrapper<br/>启动的命令]
    Note2[不是完整 sandbox]
    
    style UDS fill:#fbb
    style Auth fill:#fbb
    style Gate fill:#fbb
    style Note1 fill:#ffb
    style Note2 fill:#ffb
```

- UDS socket 应 restrictive；
- backend 应验证 peer credentials；
- protobuf 消息使用 4 字节大端长度前缀，单帧最大 4 MiB，并使用完整读写与 I/O 超时；
- policy mutation 需要 runtime gate；
- wrapper 只覆盖经它启动的命令。

---

## 相关导航

- [Agents、Adapters 与 PID 注册](agents.md)
- [Native Hooks](native-hooks.md)
- [事件管线](../backend/event-pipeline.md)
- [策略语义](../security/policy-semantics.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
