# 安全模型

Agent eBPF Filter 的安全模型由五层组成：权限、认证、runtime gates、策略语义、数据脱敏。

```mermaid
graph TB
    subgraph "L1: 权限层"
        CAP["CAP_BPF<br/>CAP_NET_ADMIN<br/>CAP_SYS_ADMIN"]
        Sudo["sudo/pkexec<br/>自提权"]
    end
    
    subgraph "L2: 认证层"
        Token["Runtime Access Token<br/>~/.config/.../runtime.json"]
        Auth["authMiddleware()<br/>release mode"]
        HookSecret["Per-Hook Secret<br/>X-Agent-Hook-Secret"]
    end
    
    subgraph "L3: Runtime Gate 层"
        ShellGate["shell_sessions_enabled"]
        SystemGate["system_run_enabled"]
        HookGate["hook_management_enabled"]
        PolicyGate["policy_management_enabled"]
        TLSGate["tls_capture_enabled"]
        OTLPGate["otlp_enabled"]
    end
    
    subgraph "L4: 策略语义层"
        Wrapper["Wrapper Policy<br/>ALLOW/BLOCK/ALERT/REWRITE"]
        Cgroup["cgroup eBPF<br/>exact IP/port block"]
        LSM["BPF LSM<br/>exec/file block"]
    end
    
    subgraph "L5: 数据保护层"
        Digest["argv_digest<br/>sha256(prompt)"]
        Redaction["redaction_level<br/>Basic/Standard/Strict"]
        Sanitize["sanitized_fields<br/>PII/secrets"]
        Truncate["body truncation<br/>max 64KB"]
    end
    
    CAP --> Auth
    Sudo --> CAP
    Token --> Auth
    Auth --> ShellGate
    Auth --> PolicyGate
    ShellGate --> Wrapper
    PolicyGate --> Cgroup
    PolicyGate --> LSM
    TLSGate --> Redaction
    Wrapper --> Digest
    Cgroup --> Sanitize
    
    style CAP fill:#fbb
    style Auth fill:#fbb
    style PolicyGate fill:#fbb
    style TLSGate fill:#fbb
    style Redaction fill:#bfb
    style Digest fill:#bfb
```

## 安全目标

- 只在授权环境中加载和管理 eBPF；
- release mode 保护敏感 API；
- 高风险能力默认关闭；
- policy mutation 通过认证后端 API；
- 避免普通事件流泄漏 secrets / TLS plaintext；
- 将观测、诊断、控制的边界说清楚。

## 权限层

```mermaid
sequenceDiagram
    participant User as User
    participant Main as Backend Main
    participant Cap as Capability Check
    participant Sudo as sudo/pkexec
    participant eBPF as eBPF Subsystem
    
    User->>Main: ./agent-ebpf-filter
    Main->>Cap: check CAP_BPF, CAP_NET_ADMIN
    
    alt has capabilities
        Cap-->>Main: ✓ OK
        Main->>eBPF: load programs
    else missing capabilities
        Cap->>Sudo: re-exec with elevation
        Sudo->>User: password prompt
        User-->>Sudo: authenticated
        Sudo->>Main: restart privileged
        Main->>Cap: check again
        Cap-->>Main: ✓ OK
        Main->>eBPF: load programs
    end
    
    eBPF->>eBPF: pin maps/links to bpffs
    eBPF->>eBPF: set restrictive permissions
    eBPF-->>Main: ready
    
    Note over Main: 后续子进程/shell 应 drop privileges
```

后端需要特权进行：

- eBPF load / attach；
- map / link pinning；
- cgroup / LSM attach；
- restrictive map permissions；
- 可选 80/443 binding。

子 shell / 命令应尽量 drop privileges 回调用用户。

## Auth 层

```mermaid
sequenceDiagram
    participant Client as Client (Vue/curl/MCP)
    participant Middleware as authMiddleware
    participant Store as runtimeSettingsStore
    participant Handler as API Handler
    
    Client->>Middleware: GET /config/runtime<br/>X-API-KEY: <token>
    Middleware->>Middleware: extract token from header/query
    Middleware->>Store: get access_token
    Store-->>Middleware: <expected-token>
    
    alt token matches
        Middleware-->>Handler: ✓ authorized
        Handler->>Handler: process request
        Handler-->>Client: 200 OK + data
    else token mismatch or missing
        Middleware-->>Client: 401 Unauthorized
    end
    
    Note over Middleware: dev mode: skip auth check<br/>release mode: enforce token
```

release mode 下敏感 API 使用 runtime access token。常见受保护面：

- `/config/**`
- `/system/**`
- `/ws*`
- `/metrics`
- `/register`
- `/unregister`
- `/events/recent`
- `/events/graph`
- `/shell-sessions*`
- `/sandbox/**`
- `/agentsight/**`
- `/api/**`
- `/api/v1/**`
- `/mcp`

## Runtime gate 层

```mermaid
graph LR
    Request[API Request] --> Auth{authenticated?}
    Auth -->|No| Reject401[401 Unauthorized]
    Auth -->|Yes| Gate{runtime gate?}
    
    Gate -->|shell_sessions| ShellCheck{enabled?}
    Gate -->|system_run| SystemCheck{enabled?}
    Gate -->|hook_management| HookCheck{enabled?}
    Gate -->|policy_management| PolicyCheck{enabled?}
    Gate -->|tls_capture| TLSCheck{enabled?}
    
    ShellCheck -->|No| Reject403Shell[403 Shell sessions disabled]
    ShellCheck -->|Yes| AllowShell[Allow]
    
    SystemCheck -->|No| Reject403System[403 System run disabled]
    SystemCheck -->|Yes| AllowSystem[Allow]
    
    PolicyCheck -->|No| Reject403Policy[403 Policy management disabled]
    PolicyCheck -->|Yes| AllowPolicy[Allow]
    
    style Reject401 fill:#fbb
    style Reject403Shell fill:#fbb
    style Reject403System fill:#fbb
    style Reject403Policy fill:#fbb
    style AllowShell fill:#bfb
    style AllowSystem fill:#bfb
    style AllowPolicy fill:#bfb
```

危险能力默认关闭：

- shell sessions.
- `/system/run`；
- hook management.
- policy management.
- TLS capture.
- OTLP export.
- domain forward.
- kernel risk feedback.

## 内核控制层

```mermaid
graph TB
    subgraph "Wrapper (用户态)"
        WrapperReq[WrapperRequest] --> PolicyEngine[Policy Engine]
        PolicyEngine --> Decision{decision}
        Decision -->|ALLOW| ExecAllow[exec original]
        Decision -->|BLOCK| Exit[exit 1]
        Decision -->|ALERT| ExecAlert[print alert + exec]
        Decision -->|REWRITE| ExecRewrite[exec rewritten]
    end
    
    subgraph "cgroup eBPF (内核态)"
        Connect[connect/sendmsg syscall] --> CgroupMap{cgroup_blocked?}
        CgroupMap -->|Yes| IPMap{IP blocked?}
        CgroupMap -->|No| AllowNet[allow]
        IPMap -->|Yes| DenyNet[-EPERM]
        IPMap -->|No| PortMap{port blocked?}
        PortMap -->|Yes| DenyNet
        PortMap -->|No| AllowNet
    end
    
    subgraph "BPF LSM (内核态)"
        ExecHook[bprm_check_security] --> ExecMap{exec path blocked?}
        ExecMap -->|Yes| DenyExec[-EPERM]
        ExecMap -->|No| ExecNameMap{exec name blocked?}
        ExecNameMap -->|Yes| DenyExec
        ExecNameMap -->|No| AllowExec[allow]
        
        FileHook[file_open] --> FileMap{file name blocked?}
        FileMap -->|Yes| DenyFile[-EACCES]
        FileMap -->|No| AllowFile[allow]
    end
    
    style Exit fill:#fbb
    style DenyNet fill:#fbb
    style DenyExec fill:#fbb
    style DenyFile fill:#fbb
    style AllowNet fill:#bfb
    style AllowExec fill:#bfb
    style AllowFile fill:#bfb
```

| 控制 | 语义 | 边界 |
| --- | --- | --- |
| cgroup sandbox | exact cgroup id、IPv4、IPv6、port | 不是 CIDR/range firewall |
| BPF LSM | exact exec path/name、file basename | 不是递归目录策略 |
| wrapper | command shim | 只覆盖经 wrapper 执行的命令 |

## 数据保护层

```mermaid
graph TB
    Event[Raw Event] --> HasArgv{has argv?}
    HasArgv -->|Yes| DigestArgv[argv_digest = sha256]
    HasArgv -->|No| SkipArgv
    DigestArgv --> HasPrompt
    SkipArgv --> HasPrompt
    
    HasPrompt{has prompt/response?}
    HasPrompt -->|Yes| DigestPrompt[digest + length only]
    HasPrompt -->|No| SkipPrompt
    DigestPrompt --> Redact
    SkipPrompt --> Redact
    
    Redact[Apply Redaction Level] --> CheckLevel{level?}
    CheckLevel -->|None| SkipRedact[skip]
    CheckLevel -->|Basic| BasicRedact[obvious secrets]
    CheckLevel -->|Standard| StdRedact[+ headers/tokens]
    CheckLevel -->|Strict| StrictRedact[+ PII/paths]
    
    BasicRedact --> Sanitize
    StdRedact --> Sanitize
    StrictRedact --> Sanitize
    SkipRedact --> Sanitize
    
    Sanitize[Mark sanitized_fields] --> Truncate{TLS/Codex body?}
    Truncate -->|Yes| TruncateBody[max 64KB + digest]
    Truncate -->|No| Final
    TruncateBody --> Final[Final Event]
    
    style DigestArgv fill:#bfb
    style DigestPrompt fill:#bfb
    style StrictRedact fill:#bfb
    style TruncateBody fill:#bfb
    style Final fill:#bfb
```

- argv digest；
- prompt / response digest + length；
- redaction levels；
- TLS / Codex body 截断；
- sanitized_fields；
- secrets / tokens / headers / query / JSON body redaction。
- 归档、JSONL 持久化、事件录制、OTel 导出、后台处理队列与 WebSocket 广播共用已脱敏的规范事件，避免原始敏感字段先于脱敏落盘或外发。

## 高风险能力清单

| 能力 | 风险 | 防护 |
| --- | --- | --- |
| TLS capture | 明文敏感数据 | 默认关闭、auth、runtime gate、redaction |
| system run / file access | 任意命令执行与主机文件访问 | critical feature、auth、`system_run` runtime gate、上传大小与文件类型限制 |
| shell sessions | 交互式 PTY | auth、runtime gate、privilege dropping |
| hook install | 修改用户 CLI 配置 | auth、runtime gate、确认授权 |
| policy mutation | 阻断网络/文件/执行 | auth、runtime gate、restrictive maps |
| domain forward | 80/443 public data plane | 默认关闭、auth-protected config |

## Kernel Enforcement Details

For security auditing and compliance, the following kernel-enforced security policies are active:
- **OS sandbox (`/sandbox/**`)**: Protects system calls and limits sandbox escapes.
- **Kernel-enforced policy paths**: Restricts executable paths and directories.
- **cgroup/connect and cgroup/sendmsg blocking for exact cgroup ids, IPv4/IPv6 destinations, and** network sockets.
- Handles **existing connected UDP send** operations securely.
- **IPv4 block entries are also honored for IPv4-mapped IPv6 socket** connections.
- **BPF LSM blocking for executable paths, executable basenames, and file or** directory operations.
- Intercepts **`file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`**, **`inode_mknod`, and `inode_rename`** kernel security hooks.
- Restricts **existing-fd `ftruncate` / `fchmod`** modifications.
- Protects **own cgroup / LSM policy-map mutation and attach lifecycle** from tampering.

---

## 相关导航

- [Runtime Gates 与 Auth](runtime-gates-auth.md)
- [策略语义](policy-semantics.md)
- [脱敏与隐私](redaction-privacy.md)
- [威胁模型](threat-model.md)
- [eBPF 与 OS Enforcement](../backend/ebpf-os-enforcement.md)
