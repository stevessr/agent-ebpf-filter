# Renew Desktop (MyGo Native UI)

Renew Desktop is the native desktop monitor for Agent eBPF Filter. Its interface is written entirely in Go with MyGo's `ui` package and is drawn by MyGo itself. It does **not** start a WebView, Vite, HTML, JavaScript, or the Vue Renew frontend.

The desktop process stays unprivileged. On Linux it reuses an already running backend or requests system authorization to start the bundled eBPF backend. Only the backend child gains privileges.

## Architecture

```text
eBPF / wrapper / hooks
        │
        ▼
Agent eBPF Filter privileged backend
  │
  ├══ private Unix socket ═════════════════════════════╗
  │   token handshake → Native IPC v1                 ║
  │   • EventEnvelope protobuf (per event, unbatched) ║
  │   • SystemStats protobuf                          ║
  │   • bounded async writer; backend hot path never  ║
  │     waits for the desktop renderer                ║
  │                                                   ▼
  │                                      Renew Desktop / MyGo
  │                                      native Go UI
  │
  └─ REST / WebSocket compatibility plane
      • on-demand detail/configuration operations
      • browser Renew
      • remote/custom backends
      • automatic fallback if local native IPC is lost
```

The native client now covers the low-noise daily-monitoring workflow: Overview, Events, Agent Sessions, Network, Processes, Monitoring, Wrapper Rules, Tracking, and System. Events and configuration surfaces use MyGo-native tables/forms/selects/tabs/switches; the browser runtime is not embedded.

For a backend started by Renew, the Events page no longer uses the generic WebSocket hot path. After the private lifetime-socket token handshake, the same socket becomes Native IPC v1 and receives each redacted `EventEnvelope` as a length-prefixed protobuf frame. This bypasses HTTP routing, WebSocket framing, JSON summary serialization, and the backend's 50 ms browser batching window. Initial history is loaded once; after the native stream attaches, the desktop stops re-fetching event summaries every two seconds. The existing `/ws/event-summaries` path is retained only for remote/custom instances and automatic fallback. The page supports type/session/decision/attention filtering, expands history through the backend cursor API, and fetches a full record only when its detail modal is opened. The Agent Sessions page follows the browser Renew grouping rule: only summaries with Agent context participate, and sessions are isolated by harness plus run/conversation/root-process context. Event detail can show the raw JSON on demand and, when `policy_management` is enabled, uses the existing cgroup/BPF LSM APIs to block or unblock literal destination IPs, ports, and absolute executable paths. Hostnames are never submitted as IP enforcement targets.

Monitoring uses the existing `/config/runtime` PATCH contract and only changes UI state after backend confirmation. Nested loop/signal/research settings are read first and written back intact so toggling `enabled` does not reset their thresholds or queue parameters. The native page also controls TLS capture, persistence, and the explicit policy-management gate. Wrapper Rules use `/config/rules` for ALLOW/BLOCK/ALERT/REWRITE operations, while Tracking manages tags, commands, exact paths, and path prefixes through the existing config APIs.

System telemetry uses the same native socket for a locally launched backend. The system sampler is transport-independent and shared by both clients: browser clients receive its protobuf snapshots over `/ws/system`, while Renew receives the identical `SystemStats` payload directly over Native IPC. This avoids a duplicated sampler while keeping the local desktop off the generic web transport. Remote/custom backends still use the WebSocket implementation, and a lost local IPC channel automatically falls back to it.

The browser `/renew` frontend remains available as an independent client and as the route to features not yet migrated to native widgets. It is no longer a desktop runtime or packaging dependency.

## Why a separate module?

MyGo requires Go 1.27+. The repository workspace uses Go 1.27.1, while `desktop/renew` remains its own module so desktop dependencies never enter the privileged backend module.

## Develop

From the repository root:

```bash
make renew-desktop-dev
```

Development builds the backend and starts `go tool mygo dev`. There is no Vite process.

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

The build hook compiles the CPU backend and stages it under the platform-specific MyGo resources directory. It no longer builds or bundles `frontend/dist`.

On Linux the native MyGo UI uses GTK for the window and MyGo's own renderer. WebKitGTK is not required by Renew Desktop.

## Runtime ownership

The desktop keeps:

- single-instance handling;
- native window lifecycle and saved window size;
- `AGENT_BACKEND_URL` / `--backend` selection;
- authorized startup and lifecycle supervision of its bundled backend;
- private Unix-socket token handoff and Native IPC v1 data stream;
- native monitoring state, protobuf decoding and presentation;
- MyGo packaging.

The backend keeps:

- eBPF and privileged operations;
- event persistence;
- authentication;
- native desktop IPC plus REST / WebSocket compatibility protocols;
- filtering, normalization and risk decisions.

Closing or crashing Renew closes the lifetime socket and gracefully stops **only** the backend instance it started. A reused system backend is left running.

## Validation

```bash
cd desktop/renew
go test ./...
go build ./...
go tool mygo build -platform linux/amd64
```
