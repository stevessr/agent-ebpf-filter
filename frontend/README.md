# Frontend

Vue 3 + TypeScript + Vite dashboard for the Agent eBPF Filter backend.

## Stack

- Vue 3
- TypeScript
- Vite
- Ant Design Vue
- ApexCharts (`vue3-apexcharts`)
- `@wterm/dom` for interactive PTY terminals
- `protobufjs` for decoding backend WebSocket payloads

## Route map

- `/` → `Dashboard.vue`
- `/monitor` → `Monitor.vue`
- `/network` → `Network.vue`
- `/explorer` → `Explorer.vue`
- `/executor` → `Executor.vue`
- `/hooks` → `Hooks.vue`
- `/config` → `Config.vue`

## Main responsibilities

### Dashboard

- connects to `GET /ws`
- decodes `pb.Event`
- filters by tag / type / PID / command / path
- can switch between newest-first and log-flow ordering
- can disable pagination and show the full event stream
- exports JSON / CSV

### Network

- connects to `GET /ws`
- shows syscall-derived network flow records from the eBPF stream
- focuses on `connect`, `bind`, `sendto`, and `recvfrom`
- filters by direction / type / tag / query
- exports JSON / CSV

### Monitor

- connects to `GET /ws/system`
- decodes `pb.SystemStats`
- shows process trees, GPU, CPU, memory, IO, and page-fault trends

### Explorer

- browses `GET /system/ls`
- adds tracked paths through `POST /config/paths`
- path rules are exact-match only; adding a directory does not recursively track descendants

### Executor

- has a dedicated Remote tab (`RemoteWrapperTerminal.vue`) that launches wrapper-routed commands into a temporary PTY session; it subscribes to the `/ws` event stream for real-time `wrapper_intercept` events with a one-time `GET /events/recent` as initial load
- leaving the Remote tab destroys that backend terminal and disconnects the wrapper event WebSocket
- the Shell tab (`LocalShellTerminal.vue`) manages persistent PTY sessions and receives live session list updates via the `/ws/shell-sessions` WebSocket (pub/sub push, no polling)
- each backend PTY session is single-attach: one active terminal WebSocket at a time
- includes dedicated subtabs for:
  - a remote wrapper tab for temporary wrapper-backed terminals
  - a shell tab for non-tmux interactive sessions
  - a tmux tab with coding-CLI launcher + tmux session quick tools
  - Python / Node / Ruby / sh / pwsh / Deno / Bun script launches with optional Python venv selection
  - a launch-env config tab for browser-persisted environment variables shared by all Executor launchers, plus a detected-env panel sourced from `GET /system/env`
- uses a path navigator drawer for browsing workdirs, scripts, and venv directories

### Hooks

- lists supported AI CLI hook targets
- installs / uninstalls hook config or wrapper aliases
- edits raw JSON / TOML hook config for native-hook targets
- uses `/config/hooks/:id/raw` for direct config editing

### Configuration

- manages tags
- manages tracked command names
- manages tracked paths
- manages wrapper rules
- manages master/slave cluster selection and node routing
- manages runtime log persistence
- generates / rotates the backend access token for `/config` and `/mcp`
- documents MCP query auth URLs such as `/mcp?key=<token>`
- imports / exports tag + command + path + wrapper-rule config
- provides ML subtabs for status / parameters / model management / training-set management
- pulls existing wrapper/native-hook command events into the ML sample browser, fetches remote HTTP/HTTPS raw datasets or local file content into the training store, and provides export / clear actions for the current dataset plus a full-command safety assessment panel that uses exact labeled samples as evidence
- includes a curated catalog of classic OS-security datasets such as ADFA, CERT Insider Threat, LANL host/network, and DARPA IDS corpora for quick reference; those catalog entries point to reference/archival pages and must be downloaded or extracted before import

## Development

Install dependencies:

```bash
bun install
```

Run Vite dev server:

```bash
bun run dev
```

Build production assets:

```bash
bun run build
```

## Dev proxy behavior

`vite.config.ts` reads `../backend/.port` and proxies:

- `/ws` (and subpaths like `/ws/system`, `/ws/shell`, `/ws/shell-sessions`)
- `/register`
- `/unregister`
- `/shell-sessions`
- `/events/recent`
- `/mcp`
- `^/config/.*`
- `/system`
- `/cluster`

This lets the frontend follow the backend if it starts on `8080..8089`.

## Cluster UI

The Configuration page now includes a cluster control panel that:

- shows the current node role and master/slave mode
- lists discovered slave nodes
- lets you route dashboard / monitor / network / executor traffic to a selected target

The active target is stored in `localStorage` and is applied to both HTTP requests and WebSocket URLs.

## Notes

- The frontend currently assumes local / trusted deployment.
- There is no built-in API-key login UX for release-mode backend auth.
- `src/pb/*` is generated; regenerate from the repo root with `make proto`.
