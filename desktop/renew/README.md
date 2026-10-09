# 明镜高悬 · Agent eBPF Filter 桌面端（MyGo Native UI）

**察微知著 · 守护 Agent 边界** — 用 eBPF 观察 Agent 行为、关联会话、识别外联与策略事件。

「明镜高悬」是面向用户的宣传品牌，内部开发代号仍为 `renew`。为保持现有安装、CLI、REST 及 CI 兼容，`desktop/renew`、可执行文件 `renew`、Linux 命令 `agent-ebpf-renew`、应用标识符及浏览器 `/renew` 路由均不更改；MyGo 运行时名称、窗口标题与原生 UI 使用中文品牌。

明镜高悬是 Agent eBPF Filter 的原生桌面监控应用。界面完全使用 Go 和 MyGo `ui` 组件绘制，不依赖 WebView、Vite、HTML、JavaScript 或 Vue Renew 前端运行时。

## Native workspace layout

Renew renders an editor-inspired native workspace **entirely with MyGo widgets**. Its light and dark palettes follow the host OS appearance (including live changes through MyGo's system theme), while native accent colors, text scaling and high-contrast preferences remain available. A monochrome hand-mirror SVG is embedded and rendered as the activity-rail identity; collector health is shown in the main status indicators and System diagnostics, not in a redundant sidebar block. Its 54-DIP activity rail offers page shortcuts and toggles the navigation sidebar. The navigation starts at a compact 198 DIPs and has a **详细 › / 精简 ‹** switch for a 302-DIP detailed mode with more descriptive Chinese page names; both modes retain the same page IDs, selection and keyboard navigation. The sidebar width animates, and the inspector's responsive layout tracks the changing width. The center retains live eBPF monitoring, sessions, network/process tables and existing privileged management controls. A page toolbar provides search, stream pause and context-aware refresh (rules, tracking, eBPF module state and path permissions re-fetch their own data).

When the center has at least 760 DIPs left after the icon rail, animated navigation width, and 302-DIP inspector, Overview, Events, Sessions, Network, Processes and System can show the optional right incident-inspector rail. Collapsing the navigation makes more room for it without overlapping tables during the transition. On narrower windows, **风险研判工作台** is available as a dedicated page, so the investigation features are still accessible.

### Incident investigation

- **事件研判** shows risk-severity totals for the bounded summary window, the selected event, and the most recent alert/block events. Click any risk counter to filter the Events page to exactly that severity.
- **Fixed selection** pins an event by ID across live additions. If that summary ages out, the inspector announces it rather than silently replacing it. Event-table selections are also reconciled by ID when live inserts reorder rows.
- Actions support copying the redacted compact summary, selecting the event in its table, viewing the complete detail on explicit request, and read-only correlation by exact PID, event type or Agent session.
- **运行诊断** presents the backend's real capture health, ringbuffer loss, queue lengths, event/system transport status and actual CPU/memory telemetry. It never infers a clean capture from an empty alert list.
- Events, Agent sessions, network targets and processes provide direct drill-down into related events; switching correlation scopes clears incompatible prior event filters.
- All filters operate on the bounded 1200-summary in-memory window, with older-record pagination through the existing backend API. They do **not** change kernel capture rules or claim to search all history.

No new privileged API or AI inference service is introduced. As before, only the explicit event Details action retrieves a complete event payload; closing its modal discards it from the desktop. The workspace shell (`native_workspace.go`) and inspector (`native_inspector.go`) remain independent of the native IPC data path and original per-page monitoring implementations.

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

### Local terminal (GoRex-inspired)

The **终端** entry in the desktop sidebar opens a real PTY backed by
[MyGo's terminal plugin](https://github.com/egoist/mygo/tree/main/plugins/terminal),
the same Ghostty/libghostty-vt component used by
[GoRex](https://github.com/egoist/gorex). It uses MyGo's native renderer:
there is no web terminal, WebView, HTTP terminal endpoint, or shell command
execution via the backend.

- Open tabs on demand; split the active pane left/right or top/bottom,
  drag a separator to resize it, double-click to equalize it, or zoom it.
- New tabs and panes inherit the active terminal's working directory when
  the shell reports OSC 7, otherwise its initial directory. On first open,
  Renew launches the current user's login shell in the home directory.
- Each terminal has an 8 MiB scrollback budget, the Ghostty VT keyboard
  and mouse/Unicode handling, full-screen application support, and
  platform terminal copy/paste bindings (Ctrl+Shift+C/V on Linux).
- The terminal page works even when the monitoring backend has not started
  or has gone offline. Shells run under the **desktop user's UID**, never
  with the eBPF collector's elevated credentials.
- Closing a pane/tab hangs up that PTY. Closing Renew also closes all PTYs.
  Unlike GoRex's separate session server, **this initial embedded workspace
  intentionally does not keep processes alive across desktop restarts**.

The terminal native library, `libghostty-vt.so` on Linux, is resolved by
the terminal plugin. Use `go tool mygo dev` or a **complete MyGo package**
(`.deb` / `.tar.gz`) so the verified native library is included.
A copied executable without the native runtime is still usable for
monitoring, but its terminal page shows a clear load error.

### Manual eBPF module lifecycle

The **eBPF 模块** page under Management lists registered `kind=ebpf` plugins from `GET /plugins`, including attach kind/target, runtime-loaded status and last load error. **加载** calls `POST /plugins/bpf/load`; **卸载** requires confirmation and calls `POST /plugins/bpf/unload`. After a mutation the client re-reads `GET /plugins` and only reports success after the requested runtime state is confirmed. Failures are displayed without optimistic UI changes. The existing backend authorization and policy checks remain authoritative; no shell-based `bpftool`, `rmmod`, or unprivileged load path is introduced.

These controls target **registered custom eBPF programs only**, and detach their kernel links without deleting their plugin manifests. The `enabled` setting (startup behavior) is unchanged by manual load/unload. Built-in core tracing programs are not listed as plugin manifests, and Monitoring event-group switches still control event filtering rather than physically unloading core programs. New plugin registration/compilation remains in the browser workbench.

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

The build hook generates the backend protobuf/eBPF bindings, then MyGo links the backend library directly into Renew. The Linux output `build/linux-amd64/renew` contains the UI + backend; the terminal additionally uses a verified `libghostty-vt.so` native runtime supplied by the complete MyGo Linux package. No `agent-ebpf-filter` helper ELF, askpass script, or `frontend/dist` bundle is shipped beside it. The CI workflow also publishes that executable by itself as the `renew-linux-amd64-single-binary` artifact.

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

## MyGo v0.3.4 appearance and responsive UI

- The native Go desktop module pins **MyGo v0.3.4** (the Go CLI tool shares the module version); it does not add a WebView runtime.
- Windows 11 supports a Mica material and macOS supports a sidebar material where the compositor offers native vibrancy. The toolbar provides a session-only **纯色模式 / 系统材质** switch; all content tables and event details keep opaque surfaces for readability. Unsupported Windows versions and Linux (Wayland/X11) retain the full opaque navy theme. Do not imply Linux compositor material support.
- Navigation opens and closes with native, reduced-motion-aware animation. Page changes have a modest entrance transition; the toolbar automatically overflows actions at narrow sizes while preserving keyboard and screen-reader behavior.
- Search, event pinning, explicit detail requests and privileged backend permissions are unchanged. No additional collection, telemetry or network call is introduced by these UI effects.

### MyGo v0.3.5 follow-up: terminal and triage usability

- Use MyGo v0.3.5 for its fix when a window closes while a native frame is being rendered.
- The terminal action row is a native, keyboard-friendly toolbar with automatic overflow on narrow windows. The terminal tab strip uses an accessible native tablist and horizontal scrolling; tab titles are shortened on Unicode boundaries.
- A new terminal tab reveals the end of the strip. Closing a tab before the active tab preserves the current PTY, and closing the last tab clears stale focus state.
- Clicking an alert in the inspector always shows that retained redacted summary, even if the Events table currently filters it out. Event table filters remain unchanged until the user explicitly chooses "定位" or another correlation action.
