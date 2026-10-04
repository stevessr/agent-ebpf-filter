# Renew Desktop (MyGo)

Renew Desktop is a deliberately thin desktop shell for the `/renew` UI.

It uses [MyGo](https://github.com/egoist/mygo) and loads the **real Agent eBPF Filter backend origin** (for example `http://127.0.0.1:8080/renew`) instead of embedding a second copy of the frontend. This keeps REST, protobuf requests, WebSockets, auth storage and routing on the same origin as the normal web app.

## Why a separate module?

MyGo 0.2.6 requires Go 1.27.1+. The main Agent eBPF Filter backend keeps its existing Go toolchain, while `desktop/renew` can evolve and package independently.

No MyGo dependency enters the privileged backend module.

## Run

Start Agent eBPF Filter first, then:

```bash
cd desktop/renew
GOWORK=off go tool mygo doctor
GOWORK=off go tool mygo dev
```

The shell defaults to `http://127.0.0.1:8080`.

Override it when the backend uses another address:

```bash
AGENT_BACKEND_URL=http://127.0.0.1:9090 GOWORK=off go tool mygo dev
# or, for a directly built executable:
./agent-ebpf-renew --backend http://127.0.0.1:9090
```

## Build packages

```bash
cd desktop/renew

# Current platform
GOWORK=off go tool mygo build

# Linux desktop packages
GOWORK=off go tool mygo build -platform linux/amd64

# Cross-platform packages when needed
GOWORK=off go tool mygo build -platform linux/amd64,linux/arm64,windows/amd64,darwin/universal
```

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

The desktop shell owns only:

- native window lifecycle;
- persisted window size/state;
- single-instance behavior;
- backend URL selection;
- a useful offline/unavailable screen;
- platform packaging.

All monitoring logic remains in `frontend/src/components/renew` and `frontend/src/composables/renew`.
