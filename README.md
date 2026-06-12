# Agent eBPF Filter

Linux-first observability and control plane for AI agents and developer CLIs.

This project combines:

- **kernel-space eBPF tracing** for selected syscalls,
- **kernel-side cgroup/connect + UDP sendmsg blocking** for selected cgroups, IPv4/IPv6 destinations, and TCP/UDP destination ports,
- **BPF LSM file/exec blocking** for selected executable paths, executable basenames, and file/directory basenames,
- **user-space PID registration** for agent opt-in,
- **command/path tagging** through pinned BPF maps,
- **wrapper- and hook-based interception** for AI CLIs,
- **optional Host/SNI-based HTTP/HTTPS forwarding** on ports 80 and 443,
- **a Go + Vue dashboard** for live inspection and control.

It is designed for local development workstations and lab environments where you want to see what an agent is opening, executing, connecting to, or attempting to modify.

---

## What the project currently does

### Kernel-side telemetry

The eBPF program listens to these core syscall tracepoints:

- `sys_enter_execve`
- `sys_enter_openat`
- `sys_enter_connect`
- `sys_enter_mkdirat`
- `sys_enter_unlinkat`
- `sys_enter_ioctl`
- `sys_enter_bind`
- `sys_enter_sendto`
- `sys_enter_recvfrom`

The kernel event payload now also carries syscall exit duration so the UI can render strace-style one-line summaries with timing context.

The runtime now auto-attaches the extended tracepoints compiled from `backend/ebpf/agent_tracker.c`; on kernels that do not expose a specific tracepoint, the backend skips that one and continues booting.

Events are written to a ring buffer and consumed by the Go backend. The hot path now decodes aligned native-endian ringbuf samples through an mmap-backed zero-copy view, then annotates the protobuf event with a low-latency user-space kernel-risk score/decision before broadcast. A separate opt-in feedback loop can take high-scoring decisions and, only when both `policyManagementEnabled` and `kernelRiskFeedback.enabled` are true, write deduplicated / rate-limited block entries into the cgroup IP/port maps or BPF LSM file/exec maps.

### OS-level network interception

The backend also loads `backend/ebpf/cgroup_sandbox.c` as cgroup/connect4 + connect6 plus cgroup/sendmsg4 + sendmsg6 eBPF programs. It attaches at the cgroup v2 root by default (`/sys/fs/cgroup`, or `AGENT_CGROUP_SANDBOX_PATH` when set) and pins maps/links under `/sys/fs/bpf/agent-ebpf/cgroup_sandbox`. Its policy maps are kept restrictive (`0600`) and should be mutated through the authenticated backend API rather than direct local map writes.

Unlike wrapper or native CLI hooks, this path rejects matching TCP/UDP connects and UDP sends in the kernel before the matching operation completes. IPv4 destination blocks are also honored for IPv4-mapped IPv6 sockets such as `::ffff:127.0.0.1`, so AF_INET6 clients cannot bypass an IPv4 block for the same endpoint; mapped inputs normalize to the equivalent IPv4 key. It starts with empty policy maps; blocks are only added through the Security Policies UI/API, existing pinned-map state from a previous privileged run, or the explicitly enabled kernel-risk feedback loop. There are no automatic default deny rules. The Configuration → Security Policies page exposes status, active block entries, counters, and controls for blocking/unblocking cgroup ids, a PID's current cgroup, IPv4/IPv6 destinations, and TCP/UDP destination ports. Mutating routes are protected by the same policy-management runtime gate as wrapper-rule edits.

### BPF LSM file and exec interception

When the running kernel supports BPF LSM, the backend also loads `backend/ebpf/lsm_enforcer.c` and attaches:

- `bprm_check_security` — rejects configured executable paths or executable basenames before `execve` completes.
- `file_open` — rejects configured file basenames before the open succeeds.
- `file_permission` — rejects configured basenames on existing file descriptors before read/write I/O continues.
- `mmap_file` — rejects configured basenames before a new mmap is created from an existing fd.
- `file_mprotect` — rejects configured basenames before an existing file-backed mapping can gain new protections.
- `inode_setattr` — rejects configured basenames before chmod/chown/truncate-style metadata changes succeed.
- `inode_create` / `inode_link` / `inode_symlink` / `inode_mkdir` / `inode_mknod` — reject configured basenames before creating files, hard links, symlinks, directories, FIFOs, or device nodes.
- `inode_unlink` / `inode_rmdir` / `inode_rename` — reject configured file or directory basenames before delete/rmdir/rename succeeds.

The maps/links are pinned under `/sys/fs/bpf/agent-ebpf/lsm_enforcer`; policy maps use restrictive (`0600`) permissions and are changed via the authenticated backend API. Like the cgroup sandbox, the LSM enforcer starts with empty policy maps unless a previous privileged run left pinned entries or the explicit kernel-risk feedback loop writes a high-confidence entry. The Configuration → Security Policies page exposes attach state, counters, active block entries, and controls for adding/removing executable-path, executable-name, and file/directory-name blocks. This remains a fast deterministic kernel decision path: wrapper/hook and ML/LLM policy can suggest entries, and the feedback worker can install future kernel map entries, but the synchronous LSM hook itself only evaluates its exact maps.

### TLS 明文捕获

### 数据脱敏机制

agent-ebpf-filter 实现了完整的**四层数据脱敏架构**，保护从 eBPF 内核采集到前端展示的整个数据流中的敏感信息。

**脱敏级别**：
- **None**：无脱敏（仅开发环境）
- **Basic**：脱敏明显的密码/token
- **Standard**：脱敏常见敏感信息（默认，推荐生产使用）
- **Strict**：最大化脱敏（高安全要求、合规审计）

**脱敏内容**（Standard 级别）：
- **路径**：用户主目录 → `~`，配置目录 → `<CONFIG>`
- **命令参数**：password、token、api_key、bearer、authorization
- **网络**：内网 IP → `<PRIVATE_IP>`，内部域名 → `<INTERNAL_DOMAIN>`
- **凭证**：HTTP headers、query params、JSON body 中的敏感字段

**架构设计**：
```
采集层 → 处理层 → 脱敏层 → 分发层
  ↓        ↓        ↓        ↓
eBPF → 归一化 → 脱敏引擎 → WS/JSONL/MCP/UI
```

**配置方式**：
1. 前端 UI：访问 **Config → Redaction** 标签页
2. 配置文件：编辑 `~/.config/agent-ebpf-filter/runtime.json`
3. 环境变量：`AGENT_REDACTION_LEVEL=standard`

详细文档：
- 📖 [Redaction 模块指南](backend/redaction/README.md)
- 📖 [完整文档](docs/sanitization.md)（英文）
- 📖 [使用指南](docs/sanitization_zh.md)（中文）

### TLS 明文捕获与密钥移除

TLS 明文捕获属于显式启用的高风险诊断能力，不是安全基线的一部分；本轮参赛实现只补齐安全的 hook 元数据、系统调用、网络元数据和用户态关联分析，不新增或强化加密库明文截获。

当 Runtime Config 中 `tlsCaptureEnabled` 显式开启时，后端可以通过 eBPF uprobes 挂载 OpenSSL、GnuTLS、NSS 和手动注册的 Go TLS 二进制，在加密发送前或解密接收后捕获 HTTPS 明文片段。片段在 Go 后端拼装后解析 HTTP request/response，并通过 `GET /ws/tls-capture`、`GET /tls-capture/recent`（支持 `limit=all`/`0` 返回保留窗口内全部记录）、`GET /tls-capture/libraries` 暴露给前端。

Go 进程可通过 `POST /tls-capture/go-binary` 手动注册；OpenSSL/GnuTLS/NSS 库路径可通过 `POST /tls-capture/library` 手动挂载。`POST /tls-capture/executable` 还支持传入可执行文件名或路径（例如 `claude`、`/usr/local/bin/claude`、`/proc/<pid>/exe`），后端会解析 PATH、symlink 和 shebang 解释器后尝试挂载 Go TLS 或 OpenSSL/GnuTLS/NSS 符号。只有在 `tlsCaptureEnabled=true` 时，后端才会每 60 秒自动扫描 `/proc` 发现的 Go TLS 进程。

Codex 定制适配可在源码级请求发送前把已构造的 reqwest/WebSocket 请求 POST 到 `POST /codex/capture`。该入口走认证、复用同一套 TLS plaintext store、**统一脱敏引擎**和**自动密钥移除机制**、AgentSight/EventEnvelope 输出和 bounded body 截断，因此适合 rustls/reqwest 这类 uprobe 不稳定的本地观测场景。参考源码补丁通过 `AGENT_EBPF_CODEX_CAPTURE_URL` 和 `AGENT_API_KEY` 显式启用，未配置时不产生上报。

#### 🔐 自动密钥移除机制

**所有TLS捕获的数据都会自动经过密钥移除处理**，防止敏感数据泄漏：

**移除内容**：
- ✅ **PEM格式密钥**：RSA/EC/OpenSSH私钥、证书、公钥
- ✅ **SSH密钥**：ssh-rsa、ssh-ed25519公钥
- ✅ **AWS凭证**：Access Key、Secret Key
- ✅ **JWT Token**：完整的JWT格式token
- ✅ **API密钥**：各种格式的api_key、access_key、secret_key
- ✅ **Bearer Token**：HTTP Authorization bearer
- ✅ **密码**：password、passwd、pwd字段

**处理示例**：
```
原始数据：
{
  "privateKey": "-----BEGIN RSA PRIVATE KEY-----\nMIIEpA...\n-----END RSA PRIVATE KEY-----",
  "apiKey": "sk_test_1234567890abcdef",
  "token": "eyJhbGci...完整JWT..."
}

自动处理后：
{
  "privateKey": "[PRIVATE_KEY_REMOVED]",
  "apiKey": "apiKey=[API_KEY_REMOVED]",
  "token": "[JWT_TOKEN_REMOVED]"
}
```

**集成位置**：
- URL解析前
- HTTP body处理前  
- Header sanitization前
- 所有/codex/capture入口

详细文档：📖 [SSL Hook密钥移除机制](docs/ssl_hook_key_removal.md)

**安全边界**：不做 MITM、不注入证书、不修改目标进程内存或控制流；Authorization、X-API-KEY、Cookie、Set-Cookie、Proxy-Authorization、URL query token/key/secret/password、JSON/form/text body 中常见密钥模式通过**统一脱敏引擎**和**自动密钥移除机制**两层防护处理，支持 4 个脱敏级别和自定义规则；body 截断至 16 KiB。TLS/HTTP/SSE/LLM 元数据会写入统一 `EventEnvelope`，并在 collector health 中累积 AgentSight parser/redaction counters。

### 80/443 域名流量转发

后端可在运行时开启一个 Host/SNI 感知的反向转发器，默认监听
`80` 和 `443`。HTTP 请求按 `Host` 转发；HTTPS 先用配置的默认证书或
路由级证书终止 TLS，再按解密后的 `Host` 转发。每条路由可写成：

```json
{
  "host": "example.com",
  "upstream": "https://example.com",
  "certFile": "/etc/agent-certs/example.pem",
  "keyFile": "/etc/agent-certs/example.key"
}
```

`host` 支持精确域名和 `*.example.com` 通配；`upstream` 为空时会转发到
`<defaultScheme>://<request-host>`，也可用 `{host}` 占位符。开启
`allowAnyHost` 后，未显式配置的域名也会自动转发到同名 upstream。
转发器使用直接 outbound dial，不继承 `HTTP_PROXY` / `HTTPS_PROXY`。如果测试
环境把所有域名解析回本机，可配置 `dnsResolver`（如 `1.1.1.1:53`）或显式
upstream，避免代理再次打回自身。
监听 80/443 需要 root 或 `CAP_NET_BIND_SERVICE`；HTTPS 至少需要一个默认
证书/私钥或路由级证书/私钥。状态通过 `GET /system/domain-forward/status`
查看，配置从 Configuration → Runtime Config 页面写入 `/config/runtime` 并即时应用。

### User-space telemetry and control

- **PID registration**: Python / Node adapters call `/register` and `/unregister`, optionally attaching `agent_run_id` / `task_id` / `tool_call_id` / `trace_id` / `cwd` style metadata.
- **Tracked command names**: common CLIs plus user-defined commands are tagged through `tracked_comms`.
- **Tracked paths**: exact path matches are tagged through `tracked_paths`.
- **Wrapper interception**: `agent-wrapper` asks the backend for `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`.
- **Native AI CLI hooks**: the backend can install CLI-aware hook config for Claude Code, Gemini CLI, Codex, GitHub Copilot, Kiro CLI, Augment, and Antigravity CLI, or wrapper aliases for Cursor / any CLI routed through the wrapper. Generated relay scripts are customized for each CLI's hook contract; hook events record safe prompt/response metadata (`sha256` digest + length) when supplied by the CLI, not raw prompt/response text.
- **Derived semantic alerts**: the backend can emit `semantic_alert` records such as `SECRET_ACCESS`, `UNEXPECTED_NETWORK_EGRESS`, `UNEXPECTED_CHILD_PROCESS`, `SEMANTIC_MISMATCH`, `RESOURCE_WASTING_LOOP`, and `MULTI_AGENT_FILE_CONTENTION` when observed behavior drifts from declared tool intent or multiple agents contend on the same path.
  Hook callbacks resolve against the backend's current port instead of assuming `8080`.

### UI surfaces

- **Dashboard**: live event stream with tag / type / PID / command / path filters, strace-style trace summaries with syscall timing, log-flow ordering, and an optional no-pagination mode
- **Monitor**: process / CPU / memory / GPU / IO / page-fault telemetry
- **Network**: RustNet-style flow workspace with per-process TCP / UDP flow attribution, DNS / SNI / HTTP Host / ALPN enrichment, interface traffic charts, staleness / historic flow indicators, and `process:` / `dport:` / `sni:` / `state:` style filters
- **追踪**: combines execution topology, behavior tracking, and recording/replay in one workspace; the topology tab keeps filters for run / tool / trace / pid / path / domain / risk / time, force-layout graph, node details, and one-click rule / training-sample actions
- **追踪 · 行为追踪**: 兼容 `docs/ref/agentsight` 的统一观测面板，提供 Log、Timeline、Process Tree、Metrics 四个视图，同时融合 `/events/recent` / `/ws/envelopes` 的 `EventEnvelope`、`/tls-capture/recent` / `/ws/tls-capture` 的详细 TLS/HTTP/SSE 记录，以及 `/ws/system` 转换出的 AgentSight system metrics；后端提供 `/agentsight/events`、`/agentsight/events.jsonl`、`/agentsight/events/stats`、`/agentsight/runners`、`/agentsight/events/query`、`/api/events` 和 `/api/v1/agentsight/*` 兼容导出/导入/统计/高级查询，统一输出 `{timestamp,source,pid,comm,data}` JSON/JSONL；支持自动/手动加载内置示例 trace、导入 AgentSight JSON/JSONL/log、统一搜索、进程树块展开、时间线缩放/平移、资源指标图表，以及 JSON / JSONL / CSV 导出
- **追踪 · 录制 / 回放**: file-backed and browser-memory recording / replay / export for captured events and graph snapshots
- **Explorer**: browse the host filesystem and add tracked paths
- **Executor**: open a temporary wrapper-backed PTY tab for ad-hoc commands, keep shell PTY sessions separate from tmux, and let the Remote tab self-destruct when you leave it
- **Executor**: launch coding CLIs in tmux, start Python/Node/Ruby/sh/pwsh/Deno/Bun scripts with optional virtualenv selection, and manage shared launch environment variables in a dedicated config tab with backend-detected env suggestions
- **Hook SSL（可选，默认关闭）**: TLS 明文日志，支持实时 WebSocket、进程/库/方向/域名过滤、HTTP/SSE 解析、LLM metadata / prompt digest、脱敏状态、body 搜索、body 和 curl 一键复制、库挂载状态查看，以及手动挂载 TLS 库、Go 二进制或 Claude Code 等 CLI 可执行文件
- **Hooks**: install or edit native hook configs / wrapper aliases
- **Plugins**: register custom eBPF plugins from templates, raw C source, or a low-code multi-block visual builder whose primary workspace is a Dify-style node workflow with a searchable draggable edge-snapping floating node-type library with animated edge hide/restore controls, draggable node types that can be dropped onto the canvas to create/restore Trigger / Condition / Map / Action / Code nodes, grid-snapped node movement, a manually resizable canvas, drag-to-connect ports, editable route/wire connections with disconnected-flow compile gates, undo/redo workspace history, animated wires, node-level inspectors for quick block editing, a draggable edge-snapping floating scenario-block recipe window with animated arrow hide/restore controls, a separate LLM-backed NLP Blocks Compiler tab for natural-language-to-block generation with local fallback, a separate Map / Blueprint tab for detailed condition-tree, state-map, action, and metadata editing, and a separate Generated eBPF C tab for generated source, compile logs, and load controls. A separate top-level TS pseudocode builder owns its own editor state, compile/register/load flow, and browser storage slot instead of syncing with the visual canvas. The visual builder still supports browser draft autosave/restore, JSON import/export, and compile-readiness validation.
- **ML**: first-level ML Classification page for status / parameters / model management / LLM scoring / training-set management, including a 45-profile local built-in model catalog with soft/hard/risk-stacked ensemble variants, native C runtime inference timing with CUDA / Intel iGPU capability detection, OpenAI-compatible LLM scoring that auto-saves to browser storage and syncs to the backend before scoring, validation split controls, square-grid auto parameter tuning with selectable granularity, live progress, and a heatmap preview
- **ML**: the training-set manager includes synthetic expansion presets, batch import of downloadable internet datasets, and the LLM subtab can pull a cleaned production training set directly from the current training store and export it as OpenAI chat JSONL
- **Configuration**: manage tags, tracked commands, tracked paths, wrapper rules, OS-level cgroup network blocking, BPF LSM exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename blocking, Visual eBPF Filter low-code block editing plus quick core-map rule editing, a dedicated Runtime Config first-level tab for visual editing of runtime log persistence, access token, retention, OTLP headers, TLS capture, optional zero-copy kernel-risk feedback gates, and optional 80/443 domain forwarding routes/certificates, a System Health tab for collector / zero-copy decode / kernel-risk feedback / eBPF bootstrap / OTLP / forwarder health, and a quick Linux 6.18 LTS syscall / eBPF docs popup preview backed by local snapshots
- **Cluster control**: master/slave routing, node switching, and forwarded inspection requests through the master backend

The backend can optionally persist captured events as JSONL under `~/.config/agent-ebpf-filter/events.jsonl`, now normalizes live events into versioned `EventEnvelope` records for REST / WebSocket / MCP consumers, exposes `/ws/envelopes` for protobuf envelope streaming, `/metrics` for Prometheus scraping, can export `agent.run` / `codex.task` / `tool.call` derived spans over OTLP HTTP, and provides an authenticated MCP SSE endpoint at `/mcp` using the runtime access token generated from the Configuration page. MCP clients may authenticate with `X-API-KEY`, `Authorization: Bearer`, or `?key=<token>`.

### MCP 工具

后端在 `/mcp` 端点暴露了以下 MCP 工具：

| 工具名称 | 描述 | 参数 |
|---------|------|------|
| `tail_events` | 获取最近的捕获事件 | `limit` (可选，默认 50，最大 500) |
| `config_snapshot` | 获取完整配置快照 | 无 |
| `add_tracked_command` | 添加追踪命令 | `command`, `tag` |
| `add_tracked_path` | 添加追踪路径 | `path`, `tag` |
| `query_events` | 按条件查询事件 | `eventType`, `comm`, `pid`, `limit` |
| `get_network_flows` | 获取网络流量摘要 | 无 |
| `get_system_health` | 获取系统健康状态 | 无 |
| `block_network_destination` | 阻止网络目标（IP/端口） | `ip` 或 `port` |
| `block_process_cgroup` | 阻止进程 cgroup 的网络 | `pid` |
| `block_file_access` | 使用 BPF LSM 阻止文件访问 | `path`, `basename`, `isExec` |

**注意**：`block_*` 工具需要在 Runtime Config 中启用 `policyManagementEnabled` 标志。

### Claude Code Skills

项目提供了三个 skills 用于操作 MCP 服务：

- **configure-security**: 配置安全策略（tracked commands/paths、wrapper 规则、网络/文件拦截）
- **analyze-network**: 分析网络流量，识别异常连接
- **monitor-process**: 深度监控特定进程的行为（文件访问、网络、子进程）

使用方式：在 Claude Code 中直接调用这些 skills，它们会自动使用项目的 MCP 工具。

For a rootless static check of the compiled enforcement objects and smoke script, run `rtk make os-enforcement-check`; to diagnose whether a host is ready for live kernel-deny validation, run `rtk make os-enforcement-preflight`. For a privileged live check of both OS-level enforcement paths, start the backend as root (for example with `rtk sudo -E env DISABLE_AUTH=true ./backend/agent-ebpf-filter`) and run `rtk make os-enforcement-smoke`; or set `OS_SMOKE_PRIVILEGE_CMD='sudo -E'` / another root command prefix and run `rtk make os-enforcement-smoke-start`. The smoke script verifies BPF LSM exec-path, exec-name, file-open, existing-fd read/write, mmap, mprotect, ftruncate/fchmod/setattr, create, link, symlink, unlink, mkdir, rmdir, mknod, and rename denial plus cgroup/connect PID-cgroup, IPv4/IPv6 destination, IPv4-mapped IPv6 destination, TCP destination-port, UDP connected-socket connect, existing connected UDP sends, and UDP sendto/sendmsg destination/port denial through the public APIs.

## Security and workflow docs

- `docs/threat-model.md`
- `docs/security-model.md`
- `docs/policy-semantics.md`
- `docs/otel-export.md`
- `docs/benchmark.md`
- `docs/codex-workflows.md`
- `docs/external-api.md`
- `docs/kubernetes.md`
- `docs/agentsight-grafana-compose.yml` and `docs/agentsight-promtail.yml` for optional Loki/Grafana visualization of persisted AgentSight JSONL events

## Cluster mode

The backend can run in either **master** or **slave** mode:

- **Default**: master
- **Slave mode**: set all of the following environment variables:
  - `AGENT_CLUSTER_MASTER_URL`
  - `AGENT_CLUSTER_ACCOUNT`
  - `AGENT_CLUSTER_PASSWORD`

Optional node identity overrides:

- `AGENT_CLUSTER_NODE_URL`
- `AGENT_CLUSTER_NODE_ID`
- `AGENT_CLUSTER_NODE_NAME`

When a master is selected in the web UI, it can forward supported requests to a slave backend. The currently selected target is stored in the browser and drives HTTP/WS routing through the master.

---

## Architecture

1. **eBPF bootstrap**
   - `backend/ebpf/agent_tracker.c` is compiled through `bpf2go`.
   - Maps and links are pinned under `/sys/fs/bpf/agent-ebpf`.

2. **Privileged backend**
   - `backend/main.go` self-elevates through `sudo` / `pkexec` if needed.
   - It opens pinned maps, consumes the ring buffer, and serves HTTP/WebSocket APIs.

3. **Agent registration**
   - Adapters register the current PID into the `agent_pids` BPF hash map.
   - Registered process context is mirrored in user space and inherited across child processes so descendants can carry `root_agent_pid`, `agent_run_id`, `tool_call_id`, and trace IDs.
   - The eBPF program only emits events when a PID, command, or path matches a tracked rule.

4. **Event fan-out**
   - Kernel events, wrapper events, and native hook events are normalized into protobuf messages.
   - The backend broadcasts them over `/ws` to the Vue frontend.

5. **Policy enforcement**
   - `agent-wrapper` connects to `/tmp/agent-ebpf.sock`.
   - The backend evaluates wrapper rules and returns `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`.
   - The cgroup sandbox evaluates cgroup/IP/port maps inside cgroup/connect and cgroup/sendmsg hooks and rejects matching outbound TCP connects, UDP connected-socket connects, existing connected UDP sends, unconnected UDP sendto/sendmsg, and IPv4-mapped IPv6 traffic for blocked IPv4 destinations at the kernel boundary.
   - The BPF LSM enforcer evaluates executable-path, executable-name, and file-name maps inside LSM hooks and rejects matching exec/open/read-write/mmap/mprotect/ftruncate/fchmod/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename attempts with `EACCES`.

---

## Repository layout

```text
.
├── README.md
├── AGENTS.md                  # contributor / coding-agent guide
├── agents.md                  # runtime guide for agent registration and tracking
├── Makefile
├── proto/
│   └── tracker.proto          # source of truth for protobuf messages
├── backend/
│   ├── main.go                # HTTP API, WS streams, hooks, wrapper UDS, config
│   ├── ebpf_runtime.go        # pinned map/link bootstrap and privilege handoff
│   ├── shell_sessions.go      # persistent PTY session manager
│   ├── privileges.go          # privilege drop for spawned shells/commands
│   ├── ebpf/
│   │   ├── agent_tracker.c    # eBPF program
│   │   └── gen.go             # bpf2go generation entrypoint
│   └── pb/                    # generated Go protobufs
├── wrapper/
│   └── main.go                # agent-wrapper entrypoint
├── adapters/
│   ├── python/
│   │   └── agent_tracker.py   # Python PID registration helper
│   └── js/
│       └── agentTracker.js    # Node.js PID registration helper
└── frontend/
    └── src/
        ├── views/             # Dashboard / Monitor / Explorer / Executor / Hooks / Config
        ├── components/        # shell terminal UI
        └── pb/                # generated frontend protobuf bindings
```

---

## Requirements

### Host requirements

- Linux with eBPF support
- BTF available (modern distro kernels usually have this)
- bpffs mounted at `/sys/fs/bpf`
- `clang` / LLVM for eBPF code generation
- `protoc`
- `sudo` or `pkexec`

### Toolchain used by this repo

- **Go**: the repo currently declares **Go 1.26.2** in `go.work` / `go.mod`
- **Bun**: used for the frontend
- **Python**: `adapters/python/pyproject.toml` currently targets **Python 3.13+**
- `uv` for the Python adapter environment

> `make deps` installs some helper tools, but it does **not** install `protoc` for you.

---

## Quick start

### Development mode

```bash
make dev-env     # optional: interactive local/devcontainer env wizard
make predev
make dev
```

`make dev-env` launches the standalone Go TUI in `tools/dev-env-tui` (build it
with `make dev-env-build`, or use `make dev-env-cli` for the legacy shell
prompt). The TUI writes local-only `.env.dev` shell exports plus `.env.dev.mk`
Makefile variables. It covers core dev settings, ML/LLM behavior
(`AGENT_LLM_*` and OpenAI-compatible fallbacks), runtime app behavior toggles,
sandbox/cluster settings, devcontainer image overrides, CUDA, smoke tests,
replay, and ML sweep settings. On backend startup the runtime env overrides seed
Runtime Config for ML/LLM, OTLP/TLS/domain-forwarding, kernel-risk feedback,
shell/system/policy toggles, and retention. `make dev-env-doctor` prints the effective config with
secrets redacted and checks common tools; direct shell sessions can inherit it
with `set -a; . ./.env.dev; set +a`.

`make predev` installs the development dependencies and helper tools. It uses a
writable Go workspace for Go-installed helper binaries, so a stale local
`GOPATH=/go` setting falls back to `$HOME/go` instead of failing on hosts where
`/go` is not writable. `make dev` assumes those are already present and opens a
Zellij session with backend and frontend in separate panes.
Protocol Buffer JS/TS generation runs `protobufjs-cli` with Node.js; the
devcontainer image installs the official Node.js binary directly from
`nodejs.org` and does not install npm. Package installation still goes through
Bun; Node is only the runtime for `pbjs` / `pbts`.
CUDA acceleration is optional: when `/opt/cuda/bin/nvcc` and CUDA runtime libs
are absent, backend builds use the CPU-only CUDA stub instead of linking
`libcudart` / `libcuda`.

Build-time feature selection is available through `AGENT_BUILD_FEATURES` and
`AGENT_FRONTEND_BUILD_FEATURES`. The default `all` preserves the full current
workbench; `core` builds only the core event/runtime surface, and comma-separated
values such as `tls_capture,ml,plugins` map to Go build tags like
`agentfeat_tls_capture`. Runtime gates (`AGENT_RUNTIME_*`, `/config/runtime`) and
release-mode auth still decide whether dangerous compiled-in capabilities can be
used. The backend exposes the effective manifest at `GET /system/features`.
The GHCR devcontainer image built by GitHub Actions also runs `make predev`
during the image build and publishes a multi-arch manifest for `linux/amd64`
and `linux/arm64` (aarch64). Its post-create hook and `make exec` seed missing
workspace-local dependencies from the image before verifying
`make predev-check` without network installs. If post-create reports missing
dependencies, rebuild or pull the updated workflow image; set
`DEVCONTAINER_POSTCREATE_INSTALL=1` only when you explicitly want post-create to
run `make predev` online.
`make exec` and VS Code Dev Containers mount container-local volumes over
`frontend/node_modules` and `adapters/python/.venv`, so dependency installs from
the workflow image stay writable and do not fight host-created dependency trees.
Devcontainers pass through the host user's Git config read-only
(`~/.gitconfig` and `~/.config/git`) without mounting credentials, SSH keys, or
Git credential stores.

What it does:

- generates protobuf bindings,
- builds `agent-wrapper`,
- starts the backend hot-reload script, which self-elevates when needed and rebuilds the backend and eBPF program as needed,
- writes the chosen backend port to `backend/.port`,
- starts Vite for the frontend inside a Zellij session with a separate backend pane.

The frontend reads `backend/.port` and proxies API / WebSocket traffic automatically.
In desktop sessions, the backend prefers the system's graphical elevation flow (for example `pkexec`) before falling back to `sudo`.

### Production-style run

```bash
make run
```

This builds everything and runs the backend, which serves the compiled frontend from the same process.

### System service install

```bash
make install
```

`make install` builds the backend, frontend, and wrapper, then installs them
for boot-time operation. By default it copies the service payload to
`/opt/agent-ebpf-filter`, installs `agent-ebpf-filter` and `agent-wrapper` into
`/usr/local/bin`, writes `/etc/agent-ebpf-filter/agent-ebpf-filter.env`, and
registers a service. The installer prefers systemd when a running systemd
manager is present; otherwise it falls back to an `rc.local` entry plus
`/usr/local/sbin/agent-ebpf-filter-service`.

Useful overrides:

```bash
make install INSTALL_METHOD=systemd      # force systemd
make install INSTALL_METHOD=rc.local     # force rc.local fallback
make install INSTALL_START=0             # install/enable but do not start now
make uninstall
```

The installed service runs as root for eBPF/cgroup/LSM and low-port access,
sets `GIN_MODE=release`, and sets `AGENT_REAL_HOME` to the invoking user's home
so runtime state remains under `~/.config/agent-ebpf-filter`. In release mode,
the generated runtime access token is still required for protected APIs.

### Useful targets

```bash
make help
make proto
make backend
make wrapper
make frontend
make runtime-benchmark
make dev-image   # Print the GHCR devcontainer image for this branch
make dev-image-tag
make docker      # Pull the GitHub-built devcontainer image for this branch
make exec        # Start or attach to the privileged devcontainer shell
make install     # Install as systemd service, or rc.local fallback
make uninstall
make run-backend
make run-frontend
make clean
```

---

## Common workflows

### 1) Monitor a Python agent

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

with open("/tmp/example.txt", "w") as f:
    f.write("hello")
```

The backend registers the current PID with the default tag **`AI Agent`**.

### 2) Monitor a Node.js agent

```javascript
const AgentTracker = require('./agentTracker');

const tracker = new AgentTracker('http://127.0.0.1:8080');
tracker.start();
```

### 3) Track a CLI or path without adapter code

From the **Configuration** page you can:

- add a command name such as `git`, `python`, `node`, `bun`, or a custom binary,
- add an exact file or directory path,
- assign each rule to a tag.

### 4) Install AI CLI hooks

From the **Hooks** page you can manage:

- **Claude Code** native hook
- **Gemini CLI** native hook
- **Codex** native hook
- **GitHub Copilot CLI** native hook
- **Kiro CLI** native hook
- **Augment / Auggie CLI** native hook
- **Antigravity CLI (`agy`)** native hook
- **Cursor** wrapper alias

Native hook installation edits the target CLI config file in the user home directory and injects a generated relay script that forwards hook JSON to `POST /hooks/event`.
For Codex, the backend writes `~/.codex/hooks.json` and also enables `[features].codex_hooks = true` in `~/.codex/config.toml` to match the current official hooks setup.
For Kiro CLI, the backend creates a managed agent at `~/.kiro/agents/agent-ebpf-hook.json` from `kiro_default`, injects the native hook there, and points `chat.defaultAgent` in `~/.kiro/settings/cli.json` to that managed agent while the hook is installed.
For Antigravity CLI, the backend creates a native plugin under `~/.gemini/antigravity-cli/plugins/agent-ebpf-hook-active/`, writes `plugin.json` plus `hooks.json`, and uses an Antigravity-specific relay script that returns the required JSON stdout (`decision: allow`, `{}`, or empty injected-step responses) after forwarding telemetry.

### 5) Run commands through the wrapper

```bash
make wrapper
./agent-wrapper git status
./agent-wrapper rm -rf /tmp/demo
```

The wrapper sends the command to the backend over `/tmp/agent-ebpf.sock`, receives the decision, then executes the command.

---

## API overview

### Stable external API

External automation should prefer the versioned aliases under `/api/v1`.
They use the same runtime access token as the root API and expose an OpenAPI
summary at:

- `GET /api/v1/health` — service health, runtime feature gates, eBPF bootstrap status, and collector counters
- `GET /api/v1/openapi.json` — machine-readable OpenAPI 3.0 summary
- `GET /api/v1/events/recent` / `GET /api/v1/events/graph`
- `GET /api/v1/agentsight/events?format=json|array|jsonl`, `/api/v1/agentsight/events/stats`, `/api/v1/agentsight/runners`, `/api/v1/agentsight/events/query` — AgentSight-compatible export/import, runner status, storage stats, SSE, and advanced query aliases
- `GET /api/v1/network/flows`, `/api/v1/network/dns-cache`, `/api/v1/network/interfaces`, `/api/v1/network/export/jsonl`
- `GET /api/v1/sandbox/cgroup/status` / `GET /api/v1/sandbox/lsm/status`
- `POST /api/v1/policies/network/*` and `POST /api/v1/policies/lsm/*` — policy mutations, additionally gated by `policyManagementEnabled`
- `POST /api/v1/agents/register` / `POST /api/v1/agents/unregister`
- `GET /api/v1/config/export`

See `docs/external-api.md` for curl examples and `docs/kubernetes.md` plus
`deploy/kubernetes/` for the DaemonSet + Service integration.

### Event / control endpoints

- `GET /ws` — live legacy event stream (protobuf binary, all kernel/wrapper/hook events)
- `GET /ws/envelopes` — normalized `EventEnvelopeBatch` stream for observability consumers
- `GET /ws/system?interval=2000` — process/system telemetry stream
- `GET /ws/shell?session_id=...` — attach to a PTY session
- `GET /ws/shell-sessions` — live shell session list (WebSocket JSON push, pub/sub)
- `GET /events/recent?type=&limit=` — historical events for initial load (REST fallback); `limit=all`/`0` returns the full retained window, and each record includes a normalized `Envelope`
- `GET /agentsight/events?format=json|array|jsonl` / `POST /agentsight/events` / `GET /agentsight/events.jsonl` — AgentSight-compatible merged export/import of retained `EventEnvelope` records, uploaded AgentSight traces, and TLS capture history using semantic sources such as `file`, `process`, `http_parser`, `ssl`, `stdio`, `system`, and `policy`
- `GET /agentsight/events/stats` / `GET /agentsight/events/runners/:id/stats` / `POST /agentsight/events/query` / `GET /agentsight/runners` — AgentSight storage stats, runner-specific stats, advanced filtered query, and logical runner status for `process`, `tls`, `stdio`, `system`, `agent`, and `uploaded`
- `GET /api/events` / `POST /api/events` / `GET /api/events/stream` / `GET /api/runners` / `POST /api/events/query` / `GET /api/stream/merged` — authenticated compatibility aliases for the original AgentSight sync/upload/stats/SSE surface; `/api/events` returns JSONL text
- `GET /events/graph?...` — aggregated execution graph nodes / edges for the current retained event window; pass `replay_path=/path/to/events.jsonl` to render a recording file. The `/execution-graph/behavior` UI includes a real-duration flamegraph that uses explicit duration fields (`duration_ns`, `duration_ms`, OTEL `latency_ms`) and falls back to same-trace / same-agent / same-PID timestamp-gap inference.
- `GET /events/recording` / `POST /events/recording/start|stop|replay` — record live captured events to JSONL files and replay them into the execution graph
- `POST /events/recording/browser/save` — persist browser-memory execution-graph snapshots to a backend JSON file
- `GET /network/flows?filter=&sort=&showHistoric=&limit=&cursor=` — attributed TCP / UDP flow summaries with DPI fields (`dnsName`, `sni`, `httpHost`, `tlsAlpn`), process / agent context, rate counters, staleness, and risk
- `GET /network/flows/:flowID` — one enriched flow by stable 5-tuple flow ID
- `GET /network/dns-cache` — active DNS correlation cache
- `GET /network/interfaces` — per-interface RX / TX counters, packets, errors, drops, and timestamp
- `GET /network/export/jsonl` — metadata-only flow JSONL export with process / agent attribution
- `GET /tls-capture/recent|libraries|status|rules` / `POST /tls-capture/attach-defaults|library|go-binary|executable` — Hook SSL status, retained HTTP/TLS plaintext events, rule management, default TLS library attach, manual library/Go binary attach, and executable-path attach with PATH/symlink/shebang resolution for CLI bins such as Claude Code
- `GET /system/domain-forward/status` — optional 80/443 Host/SNI forwarding listener status, bound addresses, route count, resolver override, and startup errors
- `GET /sandbox/cgroup/status` — cgroup/connect + sendmsg OS-level blocking availability, pinned-map state, active block entries, and cgroup sock-address decision counters (`checked` / `blocked` / `allowed`, with legacy `connect*` aliases)
- `POST /sandbox/cgroup/block-cgroup` / `unblock-cgroup` — block or release outbound connects for a cgroup id
- `POST /sandbox/cgroup/block-pid` / `unblock-pid` — resolve a PID's cgroup v2 inode id and block or release that cgroup
- `POST /sandbox/cgroup/block-ip` / `unblock-ip` — block or release an IPv4 or IPv6 destination globally
- `POST /sandbox/cgroup/block-port` / `unblock-port` — block or release a TCP/UDP destination port globally
- `GET /sandbox/lsm/status` — BPF LSM attach state, pinned-map state, active block entries, and exec and file-operation counters
- `POST /sandbox/lsm/block-exec-path` / `unblock-exec-path` — block or release an executable path in `bprm_check_security`
- `POST /sandbox/lsm/block-exec-name` / `unblock-exec-name` — block or release an executable basename in `bprm_check_security`
- `POST /sandbox/lsm/block-file-name` / `unblock-file-name` — block or release a file/directory basename in `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`, `inode_mkdir`, `inode_rmdir`, `inode_mknod`, and `inode_rename`

The cgroup and LSM maps are loaded empty on first boot; mutating API calls or the optional kernel-risk feedback worker are required to install block entries, and both paths are protected by the runtime policy-management gate. Feedback is configured under `/config/runtime` as `kernelRiskFeedback` with `enabled`, `minRiskScore`, `enforceNetwork`, `enforceFileNames`, `enforceExec`, and `maxActionsPerMinute`.

For validation, `rtk make os-enforcement-preflight` checks host prerequisites such as bpffs write access directly or through passwordless sudo / `OS_SMOKE_PRIVILEGE_CMD`, root/passwordless sudo or custom privilege command, cgroup v2, the selected cgroup attach path (including temporary cgroup creation when a privilege runner is available), BPF LSM visibility, compiled cgroup/LSM object sections, and smoke-test tools (`curl` / `python3`). `rtk make os-enforcement-check` runs rootless object/script checks. `rtk make os-enforcement-smoke` expects a privileged backend that is already running; `rtk make os-enforcement-smoke-start` builds and starts one with `DISABLE_AUTH=true` when root, passwordless sudo, or an explicit `OS_SMOKE_PRIVILEGE_CMD` command prefix is available. The live smoke covers LSM exec/open/existing-fd read-write/mmap/mprotect/ftruncate/fchmod/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename denial and cgroup/connect PID-cgroup, TCP destination-port, UDP connected-socket destination/port, existing connected UDP sends, UDP sendto/sendmsg destination/port, IPv4-destination, IPv4-mapped IPv6-destination, and IPv6-destination denial.
- `GET /metrics` — Prometheus exposition for ringbuf / zero-copy decode / kernel-risk decision and feedback / queue / WS / per-type / per-pid counters
- `GET /system/otel-health` — OTLP exporter readiness / queue / active-span counters
- `POST /register` — register a PID
- `POST /unregister` — unregister a PID
- `POST /hooks/event` — receive native hook events
- `POST /shell-sessions` — create a persistent PTY session
- `GET /shell-sessions` — list PTY sessions
- `DELETE /shell-sessions/:id` — close a PTY session
- `POST /shell-sessions/:id/input` — inject raw bytes into a PTY session

In release mode, **all endpoints above require the runtime access token** except `POST /hooks/event`, which accepts either:

- the normal access token (`X-API-KEY`, `Authorization: Bearer`, or `?key=...`), or
- a per-hook secret via `X-Agent-Hook-Secret` paired with `X-Agent-CLI`.

### Config and system endpoints

Protected by the same release-mode access token:

- `/config/tags`
- `/config/comms`
- `/config/paths`
- `/config/rules`
- `/config/export`
- `/config/import`
- `/config/runtime`
- `/config/access-token`
- `/config/hooks`
- `/config/hooks/:id/raw`
- `/config/ml/tune` — start square-grid auto parameter tuning over the current ML hyperparameters and stream progress/state via `/config/ml/status`; the result payload includes heatmap-ready scores for validation accuracy or inference throughput
- `/config/ml/tune-models` — start cross-model auto tuning over selected built-in model profiles, compare validation accuracy or inference throughput, and optionally apply/save the best model configuration
- `/config/ml/existing-commands`, `/config/ml/import-existing`, `/config/ml/assess` — pull historical wrapper/hook command data into ML samples and run command safety assessment
- `/config/ml/datasets/pull`, `/config/ml/datasets/import`, `/config/ml/datasets/export`, `DELETE /config/ml/datasets` — fetch remote HTTP/HTTPS raw datasets or local file content, preview them, import them into the ML training store, export the current training set, or clear it in one step; archives are auto-expanded recursively for common zip / tar / gzip / bzip2 / xz payloads
- the first-level ML UI includes a curated classic OS-security dataset catalog for GTFOBins, LOLBAS, Claude Code Safety Net, ADFA, CERT Insider Threat, LANL host/network, and DARPA IDS references; it also exposes synthetic expansion presets and batch import of downloadable internet datasets, while archival pages still need you to download or extract the actual data files first
- `/system/ls`
- `/system/collector-health`
- `/system/otel-health`
- `/system/domain-forward/status`
- `/system/run`
- `/system/env`
- `/mcp` — MCP SSE endpoint (same auth as config routes)

Dangerous capabilities are also runtime-gated and default to **disabled** until explicitly enabled from `/config/runtime`:

- PTY / shell session creation and attachment
- `/system/run`
- hook installation / raw hook writes
- policy mutations (tags / comms / paths / prefixes / wrapper rules / cgroup sandbox maps / BPF LSM path/name maps for exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename / config import)
- kernel-risk feedback into cgroup IP/port maps and BPF LSM file/exec maps (`kernelRiskFeedback.enabled` also required)

The domain forwarder itself accepts public HTTP/HTTPS traffic without the
backend runtime token because it is a data-plane reverse proxy. Its
configuration and status routes remain protected by the release-mode access
token.

### Cluster endpoints

- `GET /cluster/state` — current node role and cluster mode
- `GET /cluster/nodes` — discovered slave nodes
- `POST /cluster/heartbeat` — slave heartbeat / registration (internal)

---

## Important behavior and limitations

### Domain forwarding is a reverse proxy, not eBPF policy

The optional 80/443 forwarder is implemented in the Go backend with
`httputil.ReverseProxy`. It does not change cgroup/LSM policy maps and does not
perform transparent packet NAT. HTTP uses the request `Host`; HTTPS requires TLS
termination with configured PEM cert/key files before the backend can inspect
the HTTP host. Route changes restart the forwarding listeners live, so active
proxied connections can be interrupted during a save.

### PID registration seeds process lineage

Registering a PID adds the process to `agent_pids` and seeds a process-context record.

- `execve` in-place keeps the PID and remains tracked.
- child processes created later now inherit tracking through `sched_process_fork` / `clone` plus parent-PID context fallback in user space.
- descendants can carry `root_agent_pid`, `agent_run_id`, `task_id`, `conversation_id`, `turn_id`, `tool_call_id`, `tool_name`, `trace_id`, `span_id`, `decision`, `argv_digest`, and `cwd` when the caller provides them.

### Command and path matching are exact-match maps

- `tracked_comms` uses a fixed 16-byte command key.
- `tracked_paths` uses a fixed 256-byte path key.

That means the current implementation is best for:

- short executable names like `git`, `node`, `python`, `bun`
- exact absolute paths you care about

It is **not** a recursive path policy engine.

### Export / import scope

`/config/export` and `/config/import` currently cover:

- tags
- tracked commands
- tracked paths

They currently include:

- tags
- tracked commands
- tracked paths
- wrapper rules

They still do **not** include raw native hook config files.

### Privilege model

- The backend must run with elevated privileges to bootstrap eBPF maps and links.
- Spawned shells and wrapper-launched commands attempt to drop back to the invoking user with `SUDO_UID` / `SUDO_GID`.
- The wrapper UDS socket at `/tmp/agent-ebpf.sock` is created with restrictive permissions and peer-credential checks for root / the original invoking user.

### Auth model

- In non-release mode, auth is disabled by default.
- In release mode, the runtime access token protects config, system, WebSocket, shell-session, register / unregister, metrics, event-history / graph, network-inspection, and OS sandbox (`/sandbox/**`) routes.
- `POST /hooks/event` accepts either that token or a per-hook secret.
- The Runtime Config tab persists the token locally and appends it to WebSocket URLs via `?key=...`.
- The System Health tab shows collector health, including ringbuf reserve-fail counters, zero-copy/copy decode counters, kernel-risk evaluation and feedback counters/latency, feedback errors, and per-event-type totals.

For anything beyond local use, put the app behind a trusted reverse proxy and tighten auth coverage.

---

## Documentation map

- [`AGENTS.md`](./AGENTS.md) — contributor / coding-agent guide
- [`agents.md`](./agents.md) — agent registration and tracking guide
- [`docs/architecture.md`](./docs/architecture.md) — component and data-flow architecture
- [`backend/README.md`](./backend/README.md) — backend internals and API surface
- [`frontend/README.md`](./frontend/README.md) — frontend structure and route map
- [`wrapper/README.md`](./wrapper/README.md) — wrapper protocol and behavior
- [`adapters/python/README.md`](./adapters/python/README.md) — Python adapter usage
- [`adapters/js/README.md`](./adapters/js/README.md) — Node adapter usage

---

## Troubleshooting

### Backend fails to start eBPF components

Check:

- kernel supports eBPF + BTF,
- `/sys/fs/bpf` is mounted,
- `clang` is installed,
- the backend can elevate via `sudo` or `pkexec`.

If the backend starts but some syscall tracepoints are missing on this kernel, open **Configuration → System Health** and check the **eBPF Bootstrap Health** card. It shows the kernel release plus any tracepoints that were skipped because the running kernel does not expose them.

### Frontend cannot reach backend in dev mode

- confirm `backend/.port` exists,
- confirm Vite is running from `frontend/`,
- confirm the backend actually started on the chosen port.

### Native hooks show installed but no events arrive

Check:

- the target CLI is reading the config file you edited,
- `curl` is installed,
- the backend is reachable at the current hook callback URL (typically `http://127.0.0.1:<port>/hooks/event`),
- the hook config file still contains the injected `agent-ebpf-hook-active` marker.

### Wrapper commands do not enforce policy

Check:

- the backend is running,
- `/tmp/agent-ebpf.sock` exists,
- `agent-wrapper` can connect to that socket,
- the command rule exists under **Configuration → Wrapper Rules**.
