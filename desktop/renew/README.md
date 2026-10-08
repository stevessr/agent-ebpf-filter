# 明镜高悬 · Agent eBPF Filter 桌面端（MyGo Native UI）

**察微知著 · 守护 Agent 边界** — 用 eBPF 观察 Agent 行为、关联会话、识别外联与策略事件。

「明镜高悬」是面向用户的宣传品牌，内部开发代号仍为 `renew`。为保持现有安装、CLI、REST 及 CI 兼容，`desktop/renew`、可执行文件 `renew`、Linux 命令 `agent-ebpf-renew`、应用标识符及浏览器 `/renew` 路由均不更改；MyGo 运行时名称、窗口标题与原生 UI 使用中文品牌。

明镜高悬是 Agent eBPF Filter 的原生桌面监控应用。界面完全使用 Go 和 MyGo `ui` 组件绘制，不依赖 WebView、Vite、HTML、JavaScript 或 Vue Renew 前端运行时。

The desktop process stays unprivileged. On Linux it reuses an already running backend or re-executes the **same Renew executable** in an internal backend mode and requests system authorization for that child. The backend is linked as a Go library; there is no packaged backend sidecar. Only the internal backend child gains privileges.

## Architecture

```text
single ELF: renew
  │
  ├─ normal mode ───────────────▶ MyGo native desktop (ordinary user)
  │                                  │
  │                                  ├─ existing backend? reuse it
  │                                  │
  │                                  └─ otherwise re-exec self:
  │                                      renew --internal-backend ...
  │                                               │
  │                                         pkexec / sudo -A
  │                                               │
  │                                               ▼
  └────────────────────────────▶ embedded Agent eBPF backend (privileged child)
                                      │
                                      ├─ eBPF / wrapper / hooks
                                      │
                                      ├══ private Unix socket ══▶ desktop
                                      │   DesktopEventSummary + SystemStats protobuf
                                      │
                                      └─ authenticated REST API on a second private
                                          Unix socket (no TCP port listener)
```

The native client now covers the low-noise daily-monitoring workflow: Overview, Events, Agent Sessions, Network, Processes, Monitoring, Wrapper Rules, Tracking, and System. Events and configuration surfaces use MyGo-native tables/forms/selects/tabs/switches; the browser runtime is not embedded.

For a backend started by Renew, the Events page no longer uses the generic WebSocket hot path. After the private lifetime-socket token handshake, the same socket becomes Native IPC v2 and receives a compact `DesktopEventSummary` as a length-prefixed protobuf frame. The full `EventEnvelope` and payload stay in backend retention. This bypasses HTTP routing, WebSocket framing, JSON summary serialization, and the backend's 50 ms browser batching window. The desktop IPC reader is also decoupled from rendering: events enter a bounded queue, are committed to MyGo at most once per ~16 ms frame, and are merged into the retained sorted window with a small-batch linear merge. The retained event window and realtime batch buffers are reused, while search/filter/aggregate results are cached by event version. This prevents event bursts from turning into one main-thread redraw, full sort, and large allocation per event. Initial history is loaded once; after the native stream attaches, the desktop stops re-fetching event summaries every two seconds. The existing `/ws/event-summaries` path is retained only for remote/custom instances and automatic fallback. The page supports type/session/decision/attention filtering and expands history through `/events/summaries?compact=1`. Selecting or double-clicking a row never fetches the complete event; only the explicit **Details** action calls `/events/detail/:id`. Closing the detail modal immediately drops the full payload from desktop memory, and the formatted raw JSON string is only materialized if the Raw JSON tab is opened. The Agent Sessions page follows the browser Renew grouping rule: only summaries with Agent context participate, and sessions are isolated by harness plus run/conversation/root-process context. Event detail can show the raw JSON on demand and, when `policy_management` is enabled, uses the existing cgroup/BPF LSM APIs to block or unblock literal destination IPs, ports, and absolute executable paths. Hostnames are never submitted as IP enforcement targets.

Monitoring uses the existing `/config/runtime` PATCH contract and only changes UI state after backend confirmation. Nested loop/signal/research settings are read first and written back intact so toggling `enabled` does not reset their thresholds or queue parameters. The native page also controls TLS capture, persistence, and the explicit policy-management gate. Wrapper Rules use `/config/rules` for ALLOW/BLOCK/ALERT/REWRITE operations, while Tracking manages tags, commands, exact paths, and path prefixes through the existing config APIs.

System telemetry uses the same native socket for a locally launched backend. The system sampler is transport-independent and shared by both clients: browser clients receive its protobuf snapshots over `/ws/system`, while Renew receives the identical `SystemStats` payload directly over Native IPC. This avoids a duplicated sampler while keeping the local desktop off the generic web transport. Remote/custom backends still use the WebSocket implementation, and a lost local IPC channel automatically falls back to it.

The browser `/renew` frontend has been removed. The main browser workbench remains independent; the native desktop UI does not ship a browser, a WebView, or a TCP HTTP listener. The privately started backend serves authenticated configuration and history requests over a second Unix socket in the desktop-owned 0700 session directory.

## Why a separate module?

MyGo requires Go 1.27+. Renew pins MyGo v0.2.18 and the repository workspace uses Go 1.27.1, while `desktop/renew` remains its own module so desktop dependencies never enter the privileged backend module. The v0.2.18 runtime also brings the native UI memory/cache fixes, Linux repaint/rendering improvements, and safer input handling added after v0.2.7 without changing Renew's privilege boundary.

## Develop

From the repository root:

```bash
make renew-desktop-dev
```

Development generates the backend protobuf/eBPF bindings and starts `go tool mygo dev`. The backend code is linked into the dev Renew binary; there is no second backend executable and no Vite process.

Connect to an existing backend:

```bash
AGENT_BACKEND_URL=http://127.0.0.1:9090 make renew-desktop-dev
```

For a protected remote/custom backend, provide its existing token explicitly:

```bash
AGENT_BACKEND_URL=https://host.example AGENT_API_TOKEN=... make renew-desktop-dev
```

Automatic eBPF backend startup is Linux-only and only for a local HTTP origin. Windows and macOS builds can connect to a separately running backend.

## Build packages

```bash
make renew-desktop-build
# Equivalent:
cd desktop/renew
go tool mygo build
```

The build hook generates the backend protobuf/eBPF bindings, then MyGo links the backend library directly into Renew. The Linux output `build/linux-amd64/renew` is the complete UI + backend executable. No `agent-ebpf-filter` helper ELF, askpass script, or `frontend/dist` bundle is shipped beside it. The CI workflow also publishes that executable by itself as the `renew-linux-amd64-single-binary` artifact.

On Linux the native MyGo UI uses GTK for the window and MyGo's own renderer. WebKitGTK is not required by Renew Desktop.

MyGo v0.2.18 deliberately packages Go applications with `CGO_ENABLED=0`. The embedded backend therefore uses pure-Go hardware fallbacks: core eBPF monitoring, policy enforcement, event capture, system/process statistics, and generic DRM fdinfo GPU telemetry remain available, while NVML-only NVIDIA detail fields and V4L2 camera capture are omitted from the single-file build. The standalone backend keeps those integrations when built with CGO enabled.

## Runtime ownership

The desktop keeps:

- single-instance handling;
- native window lifecycle and saved window size;
- `AGENT_BACKEND_URL` / `--backend` selection;
- same-executable backend dispatch, authorization and lifecycle supervision;
- private Unix-socket token handoff and Native IPC v2 compact-summary stream;
- native monitoring state, protobuf decoding and presentation;
- MyGo packaging.

The embedded backend library keeps:

- eBPF and privileged operations;
- event persistence;
- authentication;
- native desktop IPC plus REST / WebSocket compatibility protocols;
- filtering, normalization and risk decisions.

Closing or crashing Renew closes the lifetime socket and gracefully stops **only** the privileged internal-backend child it started. A reused system backend is left running. On systems without a PolicyKit authentication agent, the same Renew executable can also act as `SUDO_ASKPASS` and delegate the password UI to an installed `zenity` or `kdialog`; no helper script is packaged.

## Validation

```bash
cd desktop/renew
go test ./...
go build ./...
../../scripts/renew-desktop.sh prepare
go tool mygo build -skip-build-command -platform linux/amd64
```

## Tracking scope and filesystem noise

The tracking panel can filter by tag and activation status. Commands may be disabled; exact file and recursive directory rules remain enabled until deleted. The file picker uses MyGo's native dialog. **文件夹内文件** enumerates and registers existing immediate regular files as exact paths (a snapshot; new files need registering again); **文件** registers one exact file; **文件夹及其子文件** stores a persistent recursive prefix. Mutations still require the existing policy-management gate.

New runtime configs ignore `/proc` and `/tmp` by default. The extra built-in `/usr/bin` rule suppresses only low-risk read/write operations (not executable launches, renames, or policy alerts); the explicit empty `ignoredPaths: []` opt-out remains effective. Already saved custom ignored paths are not overwritten.

In bundled mode the privileged child listens only on an authenticated private Unix domain API socket, plus the separate lifetime/event-stream socket. It does **not** open TCP port 8080 or write a backend port file. When the user explicitly connects to a pre-existing local/remote backend, the desktop reuses that independently managed endpoint without changing how that service listens.
