# Renew desktop

`desktop/renew` is the native desktop client for Renew. It is written in Go with MyGo's native `ui` package: there is no embedded HTML/JavaScript frontend and the window does not start a WebView.

The browser Renew UI under `frontend/` remains available at `/renew`. The two clients share the same backend APIs instead of sharing a rendering runtime.

## Architecture

```text
agent-ebpf-filter (privileged service)
  ├─ /events/summaries        compact history
  ├─ /ws/event-summaries     compact live activity
  ├─ /events/detail/:id      full event on demand
  └─ /ws/system              protobuf system/process telemetry
             ▲
             │ authenticated HTTP / WebSocket
             │
Renew desktop (ordinary user)
  ├─ client.go               REST + WebSocket transport/auth
  ├─ model.go                bounded native state + stream reconnects
  ├─ view.go                 MyGo native UI
  └─ backend.go              optional Linux backend launcher/lifetime
```

The privileged backend module does **not** depend on MyGo. The desktop client keeps a narrow protobuf wire decoder for the `/ws/system` fields it renders, keyed to `proto/tracker_system.proto`, so it stays on the existing wire format without importing the backend's generated `pb` build output.

## Native pages

The native client covers the always-on monitoring path:

- Overview: backend/stream state, CPU, memory, attention count, active Agent sessions, recent activity.
- Events: compact summaries, Agent/attention filters, search, full event JSON loaded only when selected.
- Processes: live process snapshot from `/ws/system`, sorted by CPU.
- Network: endpoint/domain aggregation from the bounded event-summary window.
- Monitoring: Lite / Daily / Deep profiles, kernel event groups, runtime processing, persistence and TLS capture toggles through `/config/runtime`.
- Wrapper rules: list, add/edit and confirm-delete ALLOW / ALERT / BLOCK / REWRITE rules through `/config/rules`.

System-level BPF LSM policy authoring, Research, Execution Graph, detailed TLS analysis and other specialist workbench features remain in the browser UI. **Open Web Workbench** launches `/renew` in the default browser; it is not embedded in the desktop process.

## Authentication and privilege boundary

Renew desktop itself stays unprivileged.

On Linux it first reuses an existing local backend. If none is available, the packaged app starts the bundled backend through the existing system authorization flow. A private `0700` Unix-socket session passes the runtime API token to the ordinary-user desktop and ties the owned privileged backend lifetime to the desktop process. Closing the desktop does not stop a separately managed/reused backend.

For an already-running local backend, Renew reads the existing runtime access token from `~/.config/agent-ebpf-filter/runtime.json`. For a remote/custom backend, provide the token in memory with:

```bash
AGENT_BACKEND_URL=https://host.example:8443 \
AGENT_API_KEY='...' \
./renew
```

The token is attached directly to HTTP/WebSocket requests as `X-API-KEY` and `Authorization: Bearer`; there is no browser `localStorage` token bridge.

## Development

The desktop development loop no longer requires Bun, Vite, or a frontend dev server:

```bash
make renew-desktop-dev
```

That builds the backend and starts `go tool mygo dev`. The desktop consumes the backend directly.

Direct MyGo development is also possible when a backend is already available:

```bash
cd desktop/renew
AGENT_BACKEND_URL=http://127.0.0.1:8080 go tool mygo dev
```

## Packaging

```bash
make renew-desktop-build
```

The Linux package bundles the eBPF backend and the native MyGo executable. It no longer bundles `frontend/dist`, Bun/Vite output, or WebKit content. The packaged Linux app still needs the desktop libraries used by MyGo itself, but not WebKitGTK for rendering Renew.

The standalone backend bundle is intentionally Linux-only because eBPF collection is Linux-only. Native MyGo clients on other platforms can point at a separately running Linux backend with `AGENT_BACKEND_URL` and `AGENT_API_KEY`.

## Validation

```bash
cd desktop/renew
go test ./...
go build ./...
go tool mygo build -platform linux/amd64
```

The desktop CI runs the same unit/compile/package checks.
