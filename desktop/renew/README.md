# Renew Desktop (MyGo)

Renew Desktop opens the `/renew` WebUI and starts its bundled Linux backend when no existing backend is ready. The window remains unprivileged; only the backend requests elevation.

It uses [MyGo](https://github.com/egoist/mygo) and loads the **real Agent eBPF Filter backend origin** (for example `http://127.0.0.1:8080/renew`) instead of embedding a second copy of the frontend. This keeps REST, protobuf requests, WebSockets, auth storage and routing on the same origin as the normal web app.

## Why a separate module?

MyGo 0.2.7 requires Go 1.27.1+. The repository workspace now standardizes on Go 1.27.1, while `desktop/renew` remains its own module so MyGo dependencies do not enter the privileged backend module.

## Run

From the repository root:

```bash
make renew-desktop-dev
```

This builds the backend, starts Vite, then opens **`/renew`** in MyGo. Vite proxies REST and WebSockets to the selected backend (`backend/.port`, `AGENT_BACKEND_PORT`, or 8080). The desktop reuses a ready backend; otherwise it starts the newly built backend and waits for readiness. Closing the development session stops its Vite/MyGo processes and its owned backend. The backend is rebuilt on each invocation; Vue changes use Vite HMR.

```bash
# Connect to an existing/custom backend; no duplicate local backend is started.
AGENT_BACKEND_URL=http://127.0.0.1:9090 make renew-desktop-dev
# Choose another Vite port if 5173 is already occupied.
RENEW_FRONTEND_PORT=5174 make renew-desktop-dev
```

The packaged Linux app needs no running service: open it, complete the system authorization dialog, and it loads the bundled Renew WebUI from the backend's real origin. Linux automatically uses the bundled sudo askpass helper when Zenity or KDialog is available; otherwise it uses `pkexec` with the desktop PolicyKit agent. It does not wait for an invisible terminal password prompt. To override the graphical askpass helper:

```bash
SUDO_ASKPASS=/absolute/path/to/askpass ./desktop/renew/build/linux-amd64/renew
```

Renew never reads or stores the sudo password. Cancellation/startup failures appear in the window; backend logs are in `~/.config/agent-ebpf-filter/logs/renew-backend.log`. Packaged startup enforces release-mode authentication; only `make renew-desktop-dev` explicitly selects development auth behavior. The backend sends its API token through a user-private Unix socket, not command arguments or logs, and the window initializes the existing origin-scoped API token storage.

Closing/crashing the app closes that socket and gracefully stops **only the backend it started**. An already running service is reused and left running. Remote/custom origins are connect-only; automatic eBPF backend startup requires a local Linux HTTP origin.

Direct `go tool mygo dev` is still available for shell-only work against an existing backend. It does not build repository dependencies for you.

## Build packages

```bash
make renew-desktop-build
# Equivalent: cd desktop/renew && go tool mygo build
```

The MyGo build hook builds the CPU backend and current frontend, stages them under `resources/linux-<arch>/backend` and `resources/linux-<arch>/frontend/dist`, and includes both in the executable/install archive/Debian package. Runtime asset paths are absolute and independent of the launch working directory. Generated build/resource directories are ignored by Git.

The bundled eBPF backend is Linux-only. Windows/macOS shells can connect to a separately running Linux backend; they are not standalone eBPF packages.

MyGo's Linux build emits the application executable, desktop entry/install archive and a Debian package. The runtime uses the system WebKitGTK webview.

On Linux, install GTK 3 and WebKitGTK 4.1 runtime packages. A tray is intentionally not required for the first desktop variant, so AppIndicator is not a dependency.

## Architecture

```text
agent-ebpf-filter (privileged service)
  ├─ /ws, /ws/system, REST/protobuf APIs
  └─ /renew + frontend assets
             ▲
             │ real http(s) origin
             │
Renew Desktop (MyGo)
  └─ native window + system webview
```

The desktop shell owns:

- native window lifecycle;
- persisted window size/state;
- single-instance behavior;
- backend URL selection and supervised, authorized backend startup;
- a useful offline/unavailable screen;
- platform packaging.

All monitoring logic remains in `frontend/src/components/renew` and `frontend/src/composables/renew`.
