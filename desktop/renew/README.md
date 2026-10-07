# Renew Desktop (MyGo Native UI)

Renew Desktop is the native desktop monitor for Agent eBPF Filter. Its interface is written entirely in Go with MyGo's `ui` package and is drawn by MyGo itself. It does **not** start a WebView, Vite, HTML, JavaScript, or the Vue Renew frontend.

The desktop process stays unprivileged. On Linux it reuses an already running backend or requests system authorization to start the bundled eBPF backend. Only the backend child gains privileges.

## Architecture

```text
eBPF / wrapper / hooks
        │
        ▼
Agent eBPF Filter backend
  ├─ /events/summaries + /events/detail/:id
  ├─ /ws/event-summaries
  ├─ /ws/system (protobuf)
  ├─ /config/runtime + tracking/rule APIs
  ├─ /sandbox/cgroup + /sandbox/lsm
  └─ other existing REST / WS APIs
        ▲
        │ HTTP + existing token auth
        │
Renew Desktop
  └─ MyGo native UI (Go only, GPU drawn)
```

The native client now covers the low-noise daily-monitoring workflow: Overview, Events, Agent Sessions, Network, Processes, Monitoring, Wrapper Rules, Tracking, and System. Events and configuration surfaces use MyGo-native tables/forms/selects/tabs/switches; the browser runtime is not embedded.

The Events page keeps compact summaries bounded in desktop memory, consumes the existing `/ws/event-summaries` JSON stream for immediate updates, supports type/session/decision/attention filtering, expands history through the backend cursor API, and fetches a full record only when its detail modal is opened. The Agent Sessions page follows the browser Renew grouping rule: only summaries with Agent context participate, and sessions are isolated by harness plus run/conversation/root-process context. Event detail can show the raw JSON on demand and, when `policy_management` is enabled, uses the existing cgroup/BPF LSM APIs to block or unblock literal destination IPs, ports, and absolute executable paths. Hostnames are never submitted as IP enforcement targets.

Monitoring uses the existing `/config/runtime` PATCH contract and only changes UI state after backend confirmation. Nested loop/signal/research settings are read first and written back intact so toggling `enabled` does not reset their thresholds or queue parameters. The native page also controls TLS capture, persistence, and the explicit policy-management gate. Wrapper Rules use `/config/rules` for ALLOW/BLOCK/ALERT/REWRITE operations, while Tracking manages tags, commands, exact paths, and path prefixes through the existing config APIs.

System telemetry stays protobuf-native. Renew connects directly to `/ws/system`, decodes the existing `SystemStats` wire format in Go, and renders live CPU, memory, I/O, and process data. There is no JSON compatibility side channel. If that stream is unavailable, the Processes page falls back to activity derived from the bounded event-summary window.

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
- private Unix-socket token handoff;
- native monitoring state and presentation;
- MyGo packaging.

The backend keeps:

- eBPF and privileged operations;
- event persistence;
- authentication;
- REST / WebSocket protocol ownership;
- filtering, normalization and risk decisions.

Closing or crashing Renew closes the lifetime socket and gracefully stops **only** the backend instance it started. A reused system backend is left running.

## Validation

```bash
cd desktop/renew
go test ./...
go build ./...
go tool mygo build -platform linux/amd64
```
