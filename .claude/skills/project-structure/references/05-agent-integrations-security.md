# 05 — Agent 集成与安全运行层

本层覆盖 wrapper、adapters、native hooks、MCP、AgentSight 兼容、auth、runtime gates 和 OS enforcement 安全边界。

## Agent 集成总览

```text
Python / Node Agent
  → adapters register PID
  → backend agent_pids map
  → eBPF event attribution

AI CLI native hooks
  → generated relay script
  → /hooks/event
  → native_hook event
  → Dashboard / AgentSight / OTLP

CLI command execution
  → agent-wrapper
  → /tmp/agent-ebpf.sock
  → policy engine
  → ALLOW/BLOCK/ALERT/REWRITE

External tools
  → /api/v1/** or /mcp
  → authMiddleware
  → event/config/query APIs
```

## Wrapper 集成

源码：

- `wrapper/main.go`
- `backend/uds_server.go`
- `backend/behavior.go`
- `backend/path_policy.go`
- `frontend/src/views/executor/Executor.vue`
- `frontend/src/components/terminal/RemoteWrapperTerminal.vue`

运行链路：

1. 用户或前端触发命令。
2. `agent-wrapper` 收集 pid/comm/args。
3. 通过 Unix socket `/tmp/agent-ebpf.sock` 发送 request。
4. backend policy engine 返回 decision：
   - `ALLOW`
   - `BLOCK`
   - `ALERT`
   - `REWRITE`
5. wrapper 执行、阻断或改写命令。
6. backend 发出 wrapper intercept event。

安全约束：

- UDS socket 应保持 restrictive，通常 `0600`。
- backend 应校验 peer credentials。
- policy mutation 受 runtime gate 控制。
- 文档不要把 wrapper 描述成 shell sandbox；它是命令 shim/policy layer。

## Python / Node adapters

源码：

- `adapters/python/agent_tracker.py`
- `adapters/python/README.md`
- `adapters/js/agentTracker.js`
- `adapters/js/README.md`
- `backend/registration_handlers.go`

职责：

- 当前 Agent 进程向 backend register。
- 结束时尽量 unregister。
- 提供 PID seed，让 eBPF / backend 关联 Agent 行为。

关键事实：

- PID registration 是 per-process。
- 子进程继承依赖 kernel fork/clone lineage 和 userspace parent fallback，而不是 adapter 自动递归注册所有子进程。
- `/register` / `/unregister` 在 release mode 受 auth 保护。

修改 adapters 时同步：

- protobuf registration message。
- backend handler。
- adapter README。
- agents.md 中注册语义。

## Native hooks

源码：

- `backend/hooks.go`
- `backend/hooks_detection.go`
- `backend/hooks_events.go`
- `backend/hooks_kiro_antigravity.go`
- `backend/config_handlers_hooks.go`
- `frontend/src/views/hooks/Hooks.vue`
- `frontend/src/components/hooks/`
- `frontend/src/data/hookCatalog.ts`
- `frontend/src/types/hooks.ts`

支持对象：

- Claude Code
- Gemini
- Codex
- Copilot
- Kiro
- Cursor
- 以及项目中已实现的其他 hook provider

链路：

```text
AI CLI hook stdin payload
  → generated relay script
  → curl POST /hooks/event
  → hookIngressAuthMiddleware
  → handleNativeHookEvent
  → normalize payload
  → pb.Event(type=native_hook)
  → WebSocket / EventEnvelope / AgentSight / OTLP
```

安全与运行时事实：

- hook relay scripts 依赖 host 安装 `curl`。
- `/hooks/event` 可使用 runtime token 或 per-hook secret。
- hook 安装、raw hook writes 属于危险能力，受 runtime gate 控制。
- hook callback endpoint 默认根据 `.port` 推导，可通过 `AGENT_HOOK_ENDPOINT` 覆盖。

新增 provider 时检查：

- detection 逻辑。
- install/uninstall 逻辑。
- payload parser。
- frontend catalog。
- hook event display。
- README / AGENTS / docs。

## MCP 集成

源码：

- `backend/mcp_server.go`
- `backend/routes.go` 中 `/mcp`

职责：

- 暴露 config/event access tools。
- 通过 authenticated API route 提供 MCP handler。

修改 MCP tools 时检查：

- auth scope。
- tool schema 是否稳定。
- 是否会暴露敏感数据。
- docs 中是否需要说明。

## AgentSight / 外部 API 兼容

源码：

- `backend/agentsight_handlers.go`
- `backend/agentsight_analyzers.go`
- `backend/external_api.go`
- `backend/routes.go` compatibility routes
- `docs/external-api.md`

常见 endpoint 家族：

- `/agentsight/**`
- `/api/events*`
- `/api/runners*`
- `/api/stream*`
- `/api/v1/agentsight/*`
- `/api/v1/**`

修改兼容 API 时：

- 保持 aliases 行为一致。
- 不破坏现有 external clients。
- 同步 `docs/external-api.md`。
- 检查 auth。

## Auth 模型

Release mode 中后端要求 runtime access token 保护敏感 API，包括：

- `/config/**`
- `/system/**`
- `/ws*`
- `/metrics`
- `/register`
- `/unregister`
- `/agentsight/**`
- `/api/events*`
- `/api/runners*`
- `/api/stream*`
- `/shell-sessions*`
- `/events/recent`
- `/events/graph`
- `/sandbox/**`

Dev mode 默认关闭 auth。

前端 Runtime Config tab 会本地保存 access token，并在 WebSocket URL 附加 `?key=...`。

## Runtime feature gates

危险能力默认关闭，需在 `/config/runtime` 启用：

- shell sessions
- `/system/run`
- hook installation / raw hook writes
- policy mutation
- TLS capture
- OTLP export
- domain forward proxy

新增危险能力时：

1. 增加 runtime config 字段。
2. 增加 backend middleware/check。
3. 增加 Config UI 开关。
4. 文档说明默认关闭和风险。
5. 测试 release/dev 行为。

## OS enforcement 安全边界

### cgroup sandbox

- pins：`/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps`、`links`
- PID-based blocking 解析 PID 所在 cgroup v2 inode id。
- destination blocking 使用 exact IPv4 / IPv6 / TCP/UDP port maps。
- 不要描述成 CIDR/range policy。

### BPF LSM enforcer

- pins：`/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps`、`links`
- file-name policy：basename-based。
- exec policy：exact path 或 executable basename。
- 覆盖 exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename 等 hooks。

### Map mutation

- OS-level policy maps 应保持 restrictive (`0600`)。
- 通过 authenticated backend API 修改，不让非特权用户直接写 map。

## TLS capture 安全边界

- 默认关闭。
- 只在 Runtime Config 显式启用。
- 捕获明文属于高风险诊断能力。
- HTTP parser 和 Codex ingest 共用敏感 header、URL query、JSON/form/text body 脱敏。
- 主 `pb.Event` 应使用 metadata/digest，避免前端/持久化泄漏明文。

## Domain forward 安全边界

- 默认关闭。
- 通过 `/config/runtime` 配置。
- 启用后 backend 可绑定 public HTTP/HTTPS 端口（默认 80/443）。
- data-plane traffic 按 Host / TLS SNI route，不由 API token 保护。
- config 和 `/system/domain-forward/status` 受 auth 保护。
- HTTPS 需要 default cert/key 或 route-level cert/key paths。

## 外向/不可逆操作提醒

Agent 在这些操作前应确认或确保已有明确授权：

- 安装/卸载 service。
- 启动特权 eBPF/cgroup/LSM enforcement。
- 修改 hook 配置或写入 AI CLI hooks。
- 启用 TLS 明文捕获。
- 启用 domain forward 80/443。
- 清空持久化事件。
- 删除 shell sessions 或运行 `/system/run`。

## 安全文档同步

涉及安全边界时检查：

- `docs/security-model.md`
- `docs/threat-model.md`
- `docs/policy-semantics.md`
- `docs/architecture.md`
- `AGENTS.md`
- `README.md`
- component README
