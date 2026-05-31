# 02 — 后端层

本层用于定位 Go backend 的文件、职责、调用链和修改同步点。

## 技术栈

- Go module：`backend/go.mod`
- Go 版本：`1.26.2`
- HTTP 框架：`github.com/gin-gonic/gin`
- WebSocket：`github.com/gorilla/websocket`
- eBPF：`github.com/cilium/ebpf`
- PTY：`github.com/creack/pty/v2`
- MCP：`github.com/modelcontextprotocol/go-sdk`
- OTLP：`go.opentelemetry.io/otel/*`
- protobuf：`google.golang.org/protobuf`

## 后端目录分层

```text
backend/
  main.go                      # 启动、wiring、遗留大入口
  routes.go                    # 路由注册总入口
  *_handlers.go                # HTTP handler 层
  *_ws.go                      # WebSocket handler / broadcaster
  runtime_state*.go            # runtime state、token、persistence
  event_*.go                   # event archive、context、envelope、recording
  network_*.go                 # network events / flows / enrichment
  tls_*.go                     # TLS capture、parser、assembler、store
  agentsight_*.go              # AgentSight API / analyzers
  ml_*.go + ml/                # ML model/training/sweep/LLM scoring
  cgroup_sandbox_*.go          # cgroup sandbox control/API
  lsm_enforcer_*.go            # BPF LSM enforcer control/API
  shell_session_*.go           # PTY session lifecycle
  hooks*.go                    # AI CLI hooks
  plugin*.go                   # plugin registry / visual LLM
  ebpf_runtime.go              # privileged eBPF bootstrap
  ebpf/                        # eBPF C + generated bindings
  pb/                          # generated protobuf Go files
```

## 路由层

主入口：`backend/routes.go`

路由分组：

- `registerWebSocketRoutes`
  - `/ws`
  - `/ws/system`
  - `/ws/camera`
  - `/ws/sensors`
  - `/ws/microphone`
  - `/ws/ml-status`
  - `/ws/envelopes`
  - `/ws/events/graph`
  - `/ws/tls-capture`
- `registerShellSessionRoutes`
  - `/shell-sessions`
  - `/ws/shell`
  - `/ws/shell-sessions`
- `registerEventRoutes`
  - `/events/recent`
  - `/events/graph`
  - `/events/recording/**`
- `registerNetworkRoutes`
  - `/network/flows`
  - `/network/tcp-state`
  - `/network/analyze`
  - `/network/dns-*`
  - `/network/interfaces`
  - `/network/export*`
  - `/network/geoip`
- `registerSandboxRoutes`
  - `/sandbox/cgroup/**`
  - `/sandbox/lsm/**`
- `registerUtilityRoutes`
  - `/metrics`
  - `/hooks/event`
  - `/register`
  - `/unregister`
  - `/cluster/heartbeat`
- `registerAuthenticatedAPIRoutes`
  - `/config/**`
  - `/system/**`
  - `/tls-capture/**`
  - `/codex/capture`
  - `/agentsight/**`
  - `/plugins/**`
  - `/data/**`
  - `/mcp`
  - `/cluster/**`
- `registerCompatibilityRoutes`
  - `/api/**`
  - `/api/v1/**`
- `registerStaticRoutes`
  - production frontend dist

新增路由时：

1. 放到最贴近的 `register*Routes` 分组。
2. 检查是否需要 `authMiddleware()`。
3. 危险能力加对应 feature gate middleware。
4. 若属于 public/external API，同步 `docs/external-api.md`。
5. 若前端调用，新增/更新 domain composable。

## Auth 与 feature gates

相关文件：

- `backend/helpers_auth.go`
- `backend/features.go`
- `backend/feature_helpers.go`
- `backend/runtime_state*.go`
- `backend/config_handlers_runtime.go`

关键事实：

- Dev mode 默认关闭 auth。
- Release mode 中多数敏感 API 需要 runtime access token。
- `/hooks/event` 可用普通 token 或 per-hook `X-Agent-Hook-Secret`。
- shell sessions、`/system/run`、hook management、policy mutation、TLS capture 等能力受 runtime feature gates 控制。

新增危险能力时：

- 需要设计 runtime gate。
- 后端 handler 使用 middleware 或显式检查。
- Config Runtime UI 暴露开关。
- 文档说明默认关闭和启用风险。

## Runtime state / persistence

相关文件：

- `backend/runtime_state_types.go`
- `backend/runtime_state_env.go`
- `backend/runtime_state_persistence.go`
- `backend/api_state.go`
- `backend/event_recording.go`

职责：

- runtime access token 生成/加载。
- runtime config env override。
- event archive in-memory ring。
- optional JSONL persistence。
- event recording / replay。

存储位置：

- runtime config：`~/.config/agent-ebpf-filter/runtime.json`
- optional events JSONL：`~/.config/agent-ebpf-filter/events.jsonl`

## Event / Envelope / Execution Graph

相关文件：

- `backend/event_envelope.go`
- `backend/event_context.go`
- `backend/event_context_utils.go`
- `backend/execution_graph.go`
- `backend/event_recording.go`
- `backend/api_ws.go`
- `backend/hooks_events.go`

职责：

- 将 syscall、wrapper、hook、TLS、policy 等来源归一化为 `pb.Event` 和 `EventEnvelope`。
- 建立 agent run / process / tool / syscall / network / file / policy 之间的关系。
- 为 `/ws/envelopes`、`/events/graph`、AgentSight、OTLP 提供数据。

改事件上下文时检查：

- `EventEnvelope` 字段。
- execution graph builder。
- frontend execution graph filters/display。
- AgentSight aliases/import/export。
- OTLP span conversion。

## Network 层

相关文件：

- `backend/network_events.go`
- `backend/network_syscalls.go`
- `backend/network_syscalls_ext.go`
- `backend/network_event_flows.go`
- `backend/network_flow_aggregator.go`
- `backend/network_flow_store.go`
- `backend/network_enrichment*.go`
- `backend/bandwidth_tracker.go`
- `backend/pcap_export.go`
- `backend/protocol_detection.go`

职责：

- syscall-derived network events。
- flow aggregation / storage。
- TCP state、DNS cache、GeoIP、protocol detection。
- JSONL / PCAP export。

改网络事件时检查：

- eBPF 事件结构。
- `network_events.go` type ↔ string mapping。
- proto event fields。
- frontend `Network.vue`、`NetworkFlow.vue`、composables/network。
- tests：`network_*_test.go`。

## TLS / Codex capture 层

相关文件：

- `backend/tls_capture_controller.go`
- `backend/tls_capture_handlers.go`
- `backend/tls_capture_rules.go`
- `backend/tls_capture_store.go`
- `backend/tls_capture_types.go`
- `backend/tls_fragment_assembler.go`
- `backend/tls_http_parser.go`
- `backend/tls_http_stream_assembler.go`
- `backend/tls_probe_discovery.go`
- `backend/tls_probe_manager.go`
- `backend/tls_agent_stream*.go`
- `backend/codex/capture/handlers/handlers.go`
- `backend/codex_capture_sink.go`

职责：

- OpenSSL/GnuTLS/NSS/Go TLS probe discovery 和 uprobe 管理。
- TLS fragment 拼装。
- HTTP 解析。
- 敏感 header / URL query / JSON/form/text body 脱敏。
- TLS capture history/store/WS。
- Codex source-level capture ingest。

注意：

- TLS 明文捕获默认关闭，只在 runtime config 显式启用。
- 主 `pb.Event` 应携带 metadata/digest，避免泄漏敏感明文。
- Codex adapter 未配置时不发送任何捕获数据。

## AgentSight 层

相关文件：

- `backend/agentsight_handlers.go`
- `backend/agentsight_analyzers.go`
- `backend/agentsight_handlers_test.go`
- `backend/execution_graph.go`
- `backend/event_envelope.go`

职责：

- AgentSight-compatible event history / query / import / export。
- runners / events aliases。
- TLS history/stream 与 EventEnvelope 历史关联。
- analyzers for trace/process/metrics/log views。

前端对应：

- `frontend/src/views/agentsight/AgentSight.vue`（若仍保留）
- `frontend/src/components/agentsight/`
- `frontend/src/composables/agentsight/`
- 目前 `/agentsight/:tab?` 路由重定向到 ExecutionGraph behavior tab。

## Shell sessions / Executor 层

相关文件：

- `backend/shell_session_core.go`
- `backend/shell_session_handlers.go`
- `backend/shell_session_types.go`
- `backend/shell_ws.go`
- `backend/privileges.go`
- `backend/launch_env.go`
- `backend/process_cleanup.go`

职责：

- 创建 PTY shell。
- attach/detach WebSocket。
- session list/delete/input/cleanup。
- child process privilege dropping。
- launch env 管理。

约束：

- shell-session 逻辑放在 `shell_session_*.go`，不要再塞进 `main.go`。
- shell sessions 受 runtime gate 保护。
- 子命令降权逻辑属于 `privileges.go`。

## Wrapper / UDS policy 层

相关文件：

- `backend/uds_server.go`
- `backend/behavior.go`
- `backend/path_policy.go`
- `wrapper/main.go`

职责：

- `/tmp/agent-ebpf.sock` server。
- wrapper request → policy decision。
- ALLOW/BLOCK/ALERT/REWRITE。
- wrapper intercept event。

约束：

- UDS socket 应保持 restrictive (`0600`)。
- 验证 peer credentials：root / original invoking user。
- policy mutation 受 runtime gate 保护。

## Hooks 层

相关文件：

- `backend/hooks.go`
- `backend/hooks_detection.go`
- `backend/hooks_events.go`
- `backend/hooks_kiro_antigravity.go`
- `backend/config_handlers_hooks.go`

职责：

- 检测/安装 AI CLI hooks。
- 生成 relay scripts。
- `/hooks/event` ingest。
- 标准化 native hook payload。

注意：

- relay scripts 使用 `curl` POST 到 backend。
- callback URL 默认从 `.port` 推导，可由 `AGENT_HOOK_ENDPOINT` 覆盖。
- hook install / raw hook writes 是危险能力，受 runtime gate 保护。

## Config / System 层

相关文件：

- `backend/config_handlers.go`
- `backend/config_handlers_export.go`
- `backend/config_handlers_runtime.go`
- `backend/config_handlers_hooks.go`
- `backend/config_ml_handlers.go`
- `backend/system_handlers.go`
- `backend/system_handlers_hardware.go`
- `backend/system_handlers_stats.go`
- `backend/bootstrap_health.go`
- `backend/collector_metrics.go`

职责：

- tags、tracked comms、tracked paths、wrapper rules、runtime config、hook config、ML config。
- system stats、hardware info、collector health、bootstrap health。

新增配置字段时同步：

- proto config message。
- backend runtime config state / env。
- frontend config type。
- frontend config composable 和 tab。
- docs。

## ML 层

相关文件：

- `backend/ml_model.go`
- `backend/ml_trainer*.go`
- `backend/ml_sweep*.go`
- `backend/ml_llm*.go`
- `backend/ml_auto_tune*.go`
- `backend/ml_dataset*.go`
- `backend/ml_builtin_models.go`
- `backend/ml_command_safety.go`
- `backend/ml/*.go`

职责：

- command safety / anomaly / classifier。
- built-in datasets / remote datasets。
- training / validation / sweeps / reports。
- LLM scoring / batch。
- status WebSocket。

验证：

- `cd backend && go test ./...`
- `make runtime-benchmark`（若涉及 runtime replay）
- `scripts/ml-*.sh`（报告/扫参）

## Plugins 层

相关文件：

- `backend/plugins.go`
- `backend/plugin_handlers.go`
- `backend/plugin_visual_llm.go`
- `backend/plugin_visual_llm_parse.go`

职责：

- plugin registry。
- online eBPF builder。
- visual LLM compile。
- pseudo/visual representations 的后端支持。

重要约束：

- visual eBPF plugins 使用 `attachKind: "lsm"`。
- 仅 `unlink` / `do_unlinkat` 使用 `attachKind: "kprobe"`。
- 不要为非 unlink visual plugins 序列化 `attachKind: "none"`。

## Sandbox / OS enforcement 层

相关文件：

- `backend/cgroup_sandbox_control.go`
- `backend/cgroup_sandbox_handlers.go`
- `backend/cgroup_sandbox_ops.go`
- `backend/cgroup_attribution.go`
- `backend/lsm_enforcer_bootstrap.go`
- `backend/lsm_enforcer_control.go`
- `backend/lsm_enforcer_types.go`
- `backend/sandbox_linux.go`
- `backend/sandbox_stub.go`

事实：

- cgroup sandbox 基于 cgroup v2 inode id 和 blocklist maps。
- destination blocking 是 exact IP/IPv6/port，不是 CIDR/range。
- LSM file-name policy 是 basename-based。
- executable policy 是 exact path 或 basename。
- map 权限应保持 restrictive，通过 authenticated backend API 修改。

## 后端测试定位

- 单元测试通常与目标文件同目录：`*_test.go`。
- 网络：`network_*_test.go`。
- TLS：`tls_*_test.go`。
- AgentSight：`agentsight_handlers_test.go`。
- runtime state：`runtime_state_*_test.go`。
- eBPF runtime：`ebpf_runtime_test.go`。
- domain forward：`domain_forward_proxy_test.go`。
- ML：`ml_*_test.go`。

后端通用验证：

```bash
cd backend && go test ./...
cd backend && go build ./...
```