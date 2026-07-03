# Backend

The backend is the privileged runtime of the project.

It is responsible for:

- loading / pinning eBPF maps and links,
- loading / attaching cgroup/connect and cgroup/sendmsg eBPF programs for kernel-side network blocking,
- loading / attaching BPF LSM programs for kernel-side file/exec blocking,
- consuming ring-buffer events from the kernel,
- annotating decoded kernel events with low-latency risk decisions and optionally feeding high-confidence decisions back into cgroup / BPF LSM policy maps,
- serving HTTP and WebSocket APIs,
- aggregating process / system telemetry,
- managing wrapper rules,
- receiving native AI CLI hook callbacks,
- hosting PTY shell sessions,
- optionally forwarding Host/SNI-based HTTP/HTTPS traffic on ports 80 and 443,
- routing cluster traffic through a master backend when cluster targets are selected.

## Key files

- `main.go` — routes, event broadcasting, system metrics, hook management, wrapper UDS
- `ebpf_runtime.go` — bootstrap / pin / privilege escalation flow; auto-attaches every tracepoint program compiled from `ebpf/agent_tracker.c` and skips tracepoints the running kernel does not expose
- `cgroup_sandbox_control.go` — cgroup/connect + sendmsg map loading, attach lifecycle, status, and block/unblock API handlers
- `lsm_enforcer_control.go` — BPF LSM map loading, attach lifecycle, status, and exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename block/unblock API handlers
- `app/events__kernel_risk.go` / `app/events__kernel_risk_feedback.go` — zero-copy decoded event risk annotation plus the optional, rate-limited feedback queue into cgroup IP/port and BPF LSM file/exec maps
- `shell_sessions.go` — persistent PTY session manager
- `domain_forward_proxy.go` — runtime-configurable HTTP/HTTPS reverse proxy for Host/SNI-based 80/443 forwarding
- `privileges.go` — drop spawned commands back to the invoking user
- `ebpf/agent_tracker.c` — eBPF source
- `ebpf/cgroup_sandbox.c` — cgroup/connect4 + connect6 and cgroup/sendmsg4 + sendmsg6 eBPF blocking source
- `ebpf/lsm_enforcer.c` — BPF LSM `bprm_check_security`, `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`, `inode_mkdir`, `inode_rmdir`, `inode_mknod`, and `inode_rename` blocking source
- `ebpf/gen.go` — `bpf2go` generation entrypoint

## Privilege model

The backend needs elevated privileges to create and attach eBPF programs.

Runtime behavior:

1. start backend normally,
2. backend checks whether it is already privileged,
3. if not, it relaunches itself, preferring desktop/polkit elevation (`pkexec`) when a graphical session is available, otherwise falling back to `sudo`,
4. eBPF maps and links are pinned under `/sys/fs/bpf/agent-ebpf`, and compiled tracepoint programs are attached when the running kernel exposes the matching tracepoint.

Spawned shells and wrapper-launched commands attempt to drop privileges back to the original invoking user using `SUDO_UID` / `SUDO_GID`.

## Pinned objects

Pinned map directory:

- `/sys/fs/bpf/agent-ebpf/maps`

Pinned link directory:

- `/sys/fs/bpf/agent-ebpf/links`

cgroup sandbox pinned directories:

- `/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps`
- `/sys/fs/bpf/agent-ebpf/cgroup_sandbox/links`

BPF LSM enforcer pinned directories:

- `/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps`
- `/sys/fs/bpf/agent-ebpf/lsm_enforcer/links`

The OS-level cgroup sandbox and BPF LSM policy maps are intentionally kept at
`0600` and should be mutated through the authenticated backend policy APIs.
Fresh boots start with empty OS-enforcement policy maps; the backend does not
install default block entries unless a privileged previous run left entries in
pinned maps.
When pinned maps already exist, startup preserves them if link/program reuse
fails instead of deleting the policy pins during an automatic fresh bootstrap.
Remove `/sys/fs/bpf/agent-ebpf/cgroup_sandbox` or
`/sys/fs/bpf/agent-ebpf/lsm_enforcer` manually only when you intentionally want
to reset stale kernel policy state.

Required maps:

- `agent_pids`
- `events`
- `tracked_comms`
- `tracked_paths`

## WebSocket streams

### `/ws`

Broadcasts `pb.Event` messages sourced from:

- kernel eBPF ring-buffer events, including syscall-derived TCP / UDP flow records with protobuf flow fields (`flow_id`, 5-tuple, transport, DNS / SNI / HTTP Host / ALPN metadata, bytes / packets, stale / historic status, and IP scope),
- wrapper interceptions,
- native AI CLI hook callbacks.

Kernel event payloads include syscall exit duration so the dashboard can render strace-style summaries without requiring a separate tracer.
They also carry `schema_version`, `gid`, `cgroup_id`, and inherited agent-run context when available. The backend now also normalizes them into versioned `EventEnvelope` records with `task_id` / `cwd` support for downstream consumers and can translate those envelopes into OTLP spans (`agent.run`, `codex.task`, `tool.call`, `mcp.call`, plus child process / file / network / policy spans).

Network enrichment APIs:

- `GET /network/flows?filter=&sort=&showHistoric=&limit=&cursor=` returns process / agent attributed flows and accepts RustNet-like filters such as `process:curl dport:443 sni:github.com state:ESTABLISHED`.
- `GET /network/flows/:flowID` returns one 5-tuple flow.
- `GET /network/dns-cache` returns the local DNS correlation cache.
- `GET /network/interfaces` returns per-interface counters including packets, errors, and drops.
- `GET /network/export/jsonl` exports flow metadata as JSONL. It does not export packet payload bytes.

Stable external API aliases:

- `GET /api/v1/health` returns service health, runtime gates, eBPF bootstrap status, and collector counters for external controllers.
- `GET /api/v1/openapi.json` returns a compact OpenAPI 3.0 summary.
- `/api/v1/events/*`, `/api/v1/research/*`, `/api/v1/agentsight/*`, `/api/v1/network/*`, `/api/v1/sandbox/*`, `/api/v1/policies/*`, `/api/v1/agents/*`, and `/api/v1/config/export` mirror the root API for automation and Kubernetes callers. Mutating policy aliases remain behind `policyManagementEnabledMiddleware()`.

Kernel-side network blocking APIs:

- `GET /sandbox/cgroup/status` returns cgroup/connect + sendmsg attach state, map availability, link pins, active block entries, and decision counters as `checked` / `blocked` / `allowed` plus legacy `connect*` aliases.
- `POST /sandbox/cgroup/block-cgroup` / `unblock-cgroup` writes the cgroup blocklist map.
- `POST /sandbox/cgroup/block-pid` / `unblock-pid` resolves a PID's cgroup v2 inode id and writes the cgroup blocklist map.
- `POST /sandbox/cgroup/block-ip` / `unblock-ip` writes the IPv4 or IPv6 blocklist map.
- `POST /sandbox/cgroup/block-port` / `unblock-port` writes the TCP/UDP destination-port blocklist map.

The mutating routes use `policyManagementEnabledMiddleware()`. The eBPF program rejects matching connects in the kernel; wrapper/hook policy is not involved in that decision path. IPv4 block entries are also applied to IPv4-mapped IPv6 destinations such as `::ffff:127.0.0.1`; mapped inputs normalize to the equivalent IPv4 key. Fresh map loads do not auto-block high-risk ports; add explicit entries through the API/UI or enable `runtime.kernelRiskFeedback` when that behavior is desired.

BPF LSM enforcement APIs:

- `GET /sandbox/lsm/status` returns BPF LSM attach state, map availability, active block entries, and exec and file-operation counters.
- `POST /sandbox/lsm/block-exec-path` / `unblock-exec-path` writes the executable-path blocklist used by `bprm_check_security`.
- `POST /sandbox/lsm/block-exec-name` / `unblock-exec-name` writes the executable-basename blocklist used by `bprm_check_security`.
- `POST /sandbox/lsm/block-file-name` / `unblock-file-name` writes the basename blocklist used by `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`, `inode_mkdir`, `inode_rmdir`, `inode_mknod`, and `inode_rename`.

The mutating routes also use `policyManagementEnabledMiddleware()`. The LSM program returns `-EACCES` for matches before the target exec/open/read-write/mmap/mprotect/ftruncate/fchmod/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename completes. The optional kernel-risk feedback worker uses the same backend helpers to add exact executable paths, executable basenames, or file basenames after a scored event crosses its configured threshold.

Use `rtk make os-enforcement-preflight` to check host prerequisites such as
bpffs write access directly or through passwordless sudo / `OS_SMOKE_PRIVILEGE_CMD`,
root/passwordless sudo or a custom privilege command, cgroup v2, the selected cgroup attach path (including temporary cgroup creation when a privilege runner is available), BPF LSM visibility, compiled
cgroup/LSM object sections, and smoke-test tools (`curl` / `python3`).
Use `rtk make os-enforcement-check` for rootless static coverage. Use `rtk make
os-enforcement-smoke` against an already privileged backend, or `rtk make
os-enforcement-smoke-start` to build/start that backend automatically when the
host has root/passwordless sudo or an explicit `OS_SMOKE_PRIVILEGE_CMD` command
prefix and writable bpffs.

### `/ws/system`

Broadcasts `pb.SystemStats` messages that include:

- process list
- CPU usage
- memory stats
- GPU stats
- network and disk IO
- VM page-fault / swap counters

### `/ws/shell-sessions`

Broadcasts the full shell session list as JSON text messages whenever the session list changes.

Uses a pub/sub pattern:

- clients subscribe on connect, unsubscribe on disconnect,
- the server sends the current `shellSessions.List()` immediately and re-sends on every `Create`, `Delete`, or session state change,
- the broadcast is driven by `shellSession.onChange` callbacks and `shellSessionManager.notify()`.

### `/ws/shell`

Attaches to a persistent PTY session created through `/shell-sessions`.

Current behavior:

- one backend session may have **one active WebSocket attachment at a time**,
- the backend keeps a bounded output backlog so reconnecting clients can receive recent output.

`POST /shell-sessions` accepts either a normal shell launch, a wrapper-backed temporary terminal, or a custom command + args payload, which is what the Executor page uses for the Remote tab, tmux-backed coding CLIs, script runners, and shared launch environment overrides.
`GET /system/env` returns a filtered list of the backend process environment so the Executor launch-env tab can suggest already-present variables without leaking backend-only config such as `AGENT_*`, `GIN_MODE`, or `DISABLE_AUTH`.
`GET /system/bootstrap-health` reports the current kernel release, how many compiled tracepoint programs attached successfully, and which tracepoints were skipped because the running kernel does not expose them.
`GET /system/domain-forward/status` reports the optional 80/443 forwarding listener state, bound addresses, route count, DNS resolver override, and startup errors.
`POST /shell-sessions/:id/input` can inject raw bytes into an existing PTY session, which the tmux quick manager uses to send `Ctrl-b` shortcuts.

## HTTP endpoints

### Release-mode authenticated routes

The runtime access token protects:

- `GET /events/recent?type=&limit=` — historical events (used for initial WS load); `limit=all`/`0` returns the full retained window, and each record includes a normalized `Envelope`
- `GET /events/graph?...` — aggregated execution graph API for the current event retention window
- `GET /agentsight/events?format=json|array|jsonl` / `POST /agentsight/events` / `GET /agentsight/events.jsonl` — AgentSight-compatible export/import that merges retained `EventEnvelope` records, uploaded AgentSight traces, and TLS capture history into `{timestamp,source,pid,comm,data}` JSON/JSONL
- `GET /agentsight/runners` / `GET /agentsight/events/stats` / `GET /agentsight/events/runners/:id/stats` / `POST /agentsight/events/query` / `GET /agentsight/stream/merged` / `GET /agentsight/stream/runner/:id` — AgentSight logical runner status, storage stats, advanced query, and SSE stream compatibility; `/api/events`, `/api/runners`, and `/api/stream/*` mirror the original AgentSight frontend sync/upload/SSE surface
- `/research/sessions`, `/research/sessions/:id/tasks`, `/research/tasks/:taskId`, `/research/sessions/:id/events`, `/research/sessions/:id/results`, `/research/sessions/:id/export?format=jsonl|csv|bundle` — Research Processing v2 workbench APIs for persisted normalized research sessions, async scans/comparisons/exports, loop/risk correlations, and JSONL/CSV/bundle artifacts with manifests
- `GET /api/v1/health` / `GET /api/v1/openapi.json` — stable external API discovery endpoints
- `/api/v1/events/*`, `/api/v1/research/*`, `/api/v1/agentsight/*`, `/api/v1/network/*`, `/api/v1/sandbox/*`, `/api/v1/policies/*`, `/api/v1/agents/*`, `/api/v1/config/export` — stable external aliases for automation and Kubernetes callers
- `GET /ws/envelopes` — live `pb.EventEnvelopeBatch` stream for normalized event consumers
- `GET /metrics` — Prometheus exposition for collector / ringbuf decode / kernel-risk decision and feedback / queue / WS / per-type / per-pid counters
- `GET /system/bootstrap-health` — current kernel release plus tracepoint attach/skipped summary for the backend bootstrap
- `GET /system/otel-health` — OTLP exporter readiness / queue / active span counts
- `GET /system/domain-forward/status` — optional Host/SNI-based HTTP/HTTPS forwarding status
- `GET /ws/shell-sessions` — live shell session list (WebSocket JSON push)
- `POST /register`
- `POST /unregister`
- `POST /shell-sessions`
- `GET /shell-sessions`
- `DELETE /shell-sessions/:id`
- `POST /shell-sessions/:id/input`
- `GET /ws/shell`
- `GET /ws`
- `GET /ws/system`
- `GET /ws/camera`
- `GET /ws/sensors`
- `GET /ws/microphone`
- `GET /ws/ml-status`

`POST /hooks/event` accepts either the normal token or a per-hook secret via `X-Agent-Hook-Secret` + `X-Agent-CLI`.

### Routes behind `authMiddleware()` in release mode

Config routes:

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
- `/config/ml/tune` and `/config/ml/tune-models` — run parameter-grid tuning for the current model or cross-model tuning over selected built-in profiles, with progress surfaced through `/config/ml/status`; scoring can optimize validation accuracy, balanced accuracy, legal/ALLOW recall, or inference throughput
- `/config/ml/existing-commands`, `/config/ml/import-existing`, `/config/ml/assess`
- `/config/ml/llm/production-dataset/pull` — pull a cleaned OpenAI chat-style JSONL preview from the current training store for LLM fine-tuning
- `/config/ml/datasets/pull`, `/config/ml/datasets/import`, `/config/ml/datasets/agent-legal`, `/config/ml/datasets/export`, `DELETE /config/ml/datasets`
- the ML config also supports soft/hard/risk-stacked ensemble voting profiles, OpenAI-compatible LLM scoring, and post-training review; the frontend persists the LLM base URL, model, API key, timeout, temperature, max tokens, and validation split ratio
- the dataset importer accepts raw HTTP/HTTPS payloads or local file uploads, and will recursively expand common archives / compressed payloads such as zip, tar, gzip, bzip2, and xz before parsing rows
- the built-in Agent legal-behavior dataset adds normalized ALLOW samples for common coding-agent workflows such as repository inspection, file search, build/test commands, dependency metadata queries, and read-only container/service checks
- the frontend also exposes a curated classic OS-security dataset catalog for reference; one-click presets carry their own import format/label mode, and archival pages still need you to download or extract the actual data first
- `/plugins`, `/plugins/:id`, `/plugins/bpf/{templates,compile,load,unload}`, and `/plugins/visual/llm-compile` cover plugin CRUD, eBPF source build/load, and LLM natural-language-to-block compilation; mutating/plugin-load routes remain protected by `policyManagementEnabled`.

`authMiddleware()` accepts `?key=<token>`, `X-API-KEY`, or `Authorization: Bearer <token>`.
The token is generated and stored by the runtime settings file at:

- `~/.config/agent-ebpf-filter/runtime.json`

Runtime feature flags in `/config/runtime` default dangerous capabilities to off:

- `shellSessionsEnabled`
- `systemRunEnabled`
- `hookManagementEnabled`
- `policyManagementEnabled`
- `kernelRiskFeedback` (`enabled`, `minRiskScore`, `enforceNetwork`, `enforceFileNames`, `enforceExec`, `maxActionsPerMinute`)

`/config/runtime` also stores `domainForwardProxy`, which controls the optional
data-plane reverse proxy. It is disabled by default and can bind HTTP/HTTPS
listeners (default 80/443), preserve request hosts, route by exact or wildcard
domain, use `{host}` in upstream URLs, optionally bypass local DNS with
`dnsResolver`, and load a default cert/key plus route-level cert/key files.
The proxy traffic itself is not protected by the backend API token; only the
configuration and status endpoints are.

That means shell sessions, `/system/run`, hook installation / raw hook writes, policy mutations, and kernel-risk feedback must be explicitly enabled before their mutating routes or background actions succeed. The feedback loop can also be seeded from `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENABLED`, `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MIN_SCORE`, `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_NETWORK`, `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_FILE_NAMES`, `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_EXEC`, and `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MAX_ACTIONS_PER_MINUTE`.

### Domain forward data plane

When `domainForwardProxy.enabled` is true, the backend starts/restarts public
listeners from the current runtime settings. HTTP requests are routed by
`Host`. HTTPS listeners require a default PEM cert/key or per-route cert/key;
SNI selects the certificate when possible, and the decrypted HTTP `Host` selects
the upstream. Routes support exact hosts and `*.domain` wildcards. Empty
upstreams forward to `<defaultScheme>://<request-host>`; configured upstreams
may include `{host}`. Outbound dials are direct and do not inherit
`HTTP_PROXY` / `HTTPS_PROXY`. If local DNS maps every domain back to the node,
configure `dnsResolver` or explicit upstreams to avoid loops.

### Cluster control

Cluster mode is configured entirely through environment variables:

- `AGENT_CLUSTER_MASTER_URL`
- `AGENT_CLUSTER_ACCOUNT`
- `AGENT_CLUSTER_PASSWORD`

If all three are present, the backend starts in **slave** mode and heartbeats to `AGENT_CLUSTER_MASTER_URL`. Otherwise it stays in **master** mode.

Optional identity overrides:

- `AGENT_CLUSTER_NODE_URL`
- `AGENT_CLUSTER_NODE_ID`
- `AGENT_CLUSTER_NODE_NAME`

Cluster state routes:

- `GET /cluster/state`
- `GET /cluster/nodes`

In master mode, supported web/API/WS paths can be forwarded to a selected slave target by sending `X-Cluster-Target` or `?cluster=<target>`. The master injects cluster credentials internally when proxying to the slave.

Export / import currently covers:

- tags
- tracked commands
- tracked paths
- wrapper rules
- runtime settings

System routes:

- `/system/ls`
- `/system/collector-health`
- `/system/otel-health`
- `/system/run`

MCP:

- `/mcp`

The MCP server exposes event-tail and configuration-snapshot tools over SSE and uses the same runtime access token as the HTTP config routes.

Persistent event logs, when enabled from the Configuration page, are appended as JSONL at:

- `~/.config/agent-ebpf-filter/events.jsonl`

The collector health endpoint reports ringbuf event totals, reserve-fail / drop counts, zero-copy vs copy decode counters, kernel-risk evaluation counters/latency, kernel-risk feedback applied/dropped counters and last feedback error, backend queue length, event-stream WS client count, recent persisted-log append latency, and simple per-event-type counters so the frontend can warn when capture may be incomplete.
The OTLP health endpoint reports whether export is enabled / ready, the configured endpoint + service name, exporter queue length, active synthetic run / task / tool spans, total exported spans, dropped exporter events, and the last export error / timestamp.

Offline replay coverage now lives in the repo-level runtime benchmark suite:

- `benchmarks/runtime-replay/scenarios.json`
- `make runtime-benchmark`
- `reports/runtime-replay-*/summary.json`

## Wrapper integration

The backend exposes a Unix-domain socket at:

- `/tmp/agent-ebpf.sock`

`agent-wrapper` sends `pb.WrapperRequest`, the backend applies wrapper rules, then returns `pb.WrapperResponse`. The request can include optional run / trace metadata (`agent_run_id`, `task_id`, `tool_call_id`, `trace_id`, `span_id`, `root_agent_pid`, `argv_digest`, `cwd`, etc.) so descendant kernel events inherit the same execution context.
The socket is created with `0600` permissions and peer credentials are checked so only root or the original invoking user may connect.

Supported actions:

- `ALLOW`
- `BLOCK`
- `ALERT`
- `REWRITE`

## Hook integration

Supported hook targets:

- Claude Code
- Gemini CLI
- Codex
- GitHub Copilot CLI
- Kiro CLI
- Augment / Auggie CLI
- Antigravity CLI (`agy`)
- Cursor (wrapper alias mode)

Native hook configs are resolved relative to the real user home directory:

- `~/.claude/settings.json`
- `~/.gemini/settings.json`
- `~/.codex/hooks.json`
- `~/.kiro/agents/agent-ebpf-hook.json`
- `~/.copilot/config.json`
- `~/.augment/settings.json`
- `~/.gemini/antigravity-cli/plugins/agent-ebpf-hook-active/hooks.json`

Codex also requires the experimental feature flag below in `~/.codex/config.toml`, which the backend now enables automatically during native-hook install:

```toml
[features]
codex_hooks = true
```

Kiro native-hook install creates a managed agent cloned from `kiro_default` and temporarily points `chat.defaultAgent` in `~/.kiro/settings/cli.json` to that managed agent. On uninstall, the previous default agent is restored.
Antigravity CLI native-hook install creates an `agent-ebpf-hook-active` plugin directory with `plugin.json` and `hooks.json`; its relay script returns Antigravity-compatible JSON stdout while forwarding telemetry to the backend.

Wrapper aliases are written to:

- `~/.bashrc` or `~/.zshrc`

When native hooks are installed, the callback URL resolves from:

1. `AGENT_HOOK_ENDPOINT`, if set
2. current backend port from `.port`
3. fallback `http://127.0.0.1:8080/hooks/event`

Native hook entries call a generated relay script under the target CLI config directory's `hooks/` subdirectory instead of embedding a long inline `curl` command directly in the hook config.
Those relay scripts are CLI-aware, pass the event name when the CLI payload omits it, and send both `X-Agent-CLI` and a per-hook `X-Agent-Hook-Secret` header.
When a CLI supplies user prompt or response fields, the backend stores only safe metadata (`sha256` digest + character length) in `ExtraInfo`; it does not persist raw prompt or response text for semantic-loop analysis.

### TLS 明文捕获

TLS capture is an explicit opt-in diagnostic path (`tlsCaptureEnabled=true`) and is not part of the safe baseline used to satisfy the contest plan. Do not add new plaintext-interception hooks unless a task explicitly requests that higher-risk mode.

- `GET /ws/tls-capture` — JSON WebSocket stream of `tls_plaintext` events。
- `GET /tls-capture/recent?limit=100` — recent in-memory TLS plaintext events。
- `GET /tls-capture/libraries` — current library attach status (OpenSSL, GnuTLS, NSS, Go)。
- `POST /tls-capture/go-binary` — manually attach Go TLS uprobes for `{ "path": "/path/to/bin", "pid": 123 }`。
- `POST /codex/capture` — authenticated Codex adapter ingest for locally customized Codex builds. It accepts constructed reqwest/WebSocket request metadata, sanitizes headers/query/body with the TLS redaction rules, stores bounded plaintext only in `TLSCaptureStore`, and emits `vendor=codex` metadata through the unified `EventEnvelope` stream.

Codex adapter usage:

```bash
export AGENT_EBPF_CODEX_CAPTURE_URL="http://127.0.0.1:${PORT}/codex/capture"
export AGENT_API_KEY="$(jq -r .accessToken ~/.config/agent-ebpf-filter/runtime.json)"
```

During event broadcast, the backend may also synthesize `semantic_alert` events (for example `SECRET_ACCESS`, `UNEXPECTED_NETWORK_EGRESS`, `UNEXPECTED_CHILD_PROCESS`, `SEMANTIC_MISMATCH`, `RESOURCE_WASTING_LOOP`, or `MULTI_AGENT_FILE_CONTENTION`) when child behavior conflicts with read-only style tool intent, repeated prompt/API/file-I/O windows suggest a runaway loop, or multiple agent contexts touch the same path in a short window.

## Build notes

Regenerate eBPF bindings:

```bash
cd ebpf && go generate
```

Regenerate only the cgroup sandbox bindings after editing `ebpf/cgroup_sandbox.c`:

```bash
cd ebpf && go generate gen_cgroup.go
```

Regenerate only the BPF LSM bindings after editing `ebpf/lsm_enforcer.c`:

```bash
cd ebpf && go generate gen_lsm.go
```

Build backend:

```bash
go build -o agent-ebpf-filter
```

Optional modules can be selected with feature tags. From the repo root,
`AGENT_BUILD_FEATURES=all make backend` keeps the full default workbench,
`AGENT_BUILD_FEATURES=core make backend` keeps only the core event/runtime
surface, and comma-separated values such as `tls_capture,ml` expand to
`agentfeat_tls_capture agentfeat_ml`. The manifest is exposed at
`GET /system/features`; runtime gates and auth still control sensitive features.

Or from the repo root:

```bash
make backend
```

With a privileged backend already running, the live OS-enforcement smoke gate is:

```bash
rtk make os-enforcement-smoke
```

It verifies BPF LSM exec-path, exec-name, file-open, existing-fd read/write, mmap, mprotect, ftruncate/fchmod/setattr, create, link, symlink, unlink, mkdir, rmdir, mknod, and rename denial plus
cgroup/connect PID-cgroup, TCP destination-port, UDP connected-socket destination/port, existing connected UDP sends, UDP sendto/sendmsg destination/port, IPv4-destination, IPv4-mapped IPv6-destination, and
IPv6-destination denial through the HTTP API when IPv6 loopback is available.

Without root, use the static object/script gate:

```bash
rtk make os-enforcement-check
```

It regenerates the cgroup/LSM bindings, checks the expected ELF sections, and
runs the targeted non-root Go tests.

To see why live smoke cannot run on a host yet:

```bash
rtk make os-enforcement-preflight
```

It checks bpffs/cgroup/BPF-LSM readiness, root/passwordless sudo or `OS_SMOKE_PRIVILEGE_CMD`, the
configured cgroup attach path, compiled cgroup/LSM object sections, and
smoke-script tools before you try the live kernel-deny gate.
