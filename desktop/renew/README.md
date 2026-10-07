# Renew Desktop (MyGo Native UI)

Renew Desktop is the native desktop monitor for Agent eBPF Filter. Its interface is written entirely in Go with MyGo's `ui` package and is drawn by MyGo itself. It does **not** start a WebView, Vite, HTML, JavaScript, or the Vue Renew frontend.

The desktop process stays unprivileged. On Linux it reuses an already running backend or requests system authorization to start the bundled eBPF backend. Only the backend child gains privileges.

## Architecture

```text
eBPF / wrapper / hooks
        │
        ▼
Agent eBPF Filter backend
  ├─ /events/summaries
  ├─ /system/collector-health
  ├─ /system/tracked-comms
  └─ other existing REST / WS APIs
        ▲
        │ HTTP + existing token auth
        │
Renew Desktop
  └─ MyGo native UI (Go only, GPU drawn)
```

The native client now covers the low-noise daily-monitoring workflow: Overview, Events, Network, Processes, Monitoring, Wrapper Rules, and System. Events and rule sets use MyGo-native tables/forms/switches; network and process views aggregate only the current bounded event-summary window, so they do not pretend to be full traffic or process-history accounting. Full event payloads remain backend-owned and are not copied into desktop memory.

Monitoring switches write the existing `/config/event-types` contract and only change UI state after backend confirmation. Wrapper Rules use the existing `/config/rules` API for ALLOW/BLOCK/ALERT/REWRITE operations. The system-statistics WebSocket remains protobuf-native; the desktop does not add a JSON side channel just for rendering.

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
