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

- starts wrapper-routed commands through `POST /system/run`
- manages persistent PTY sessions through:
  - `POST /shell-sessions`
  - `GET /shell-sessions`
  - `DELETE /shell-sessions/:id`
  - `GET /ws/shell`
- each backend PTY session is single-attach: one active terminal WebSocket at a time

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
- manages runtime log persistence
- generates / rotates the backend access token for `/config` and `/mcp`
- imports / exports tag + command + path + wrapper-rule config

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

- `/ws`
- `/register`
- `/unregister`
- `/shell-sessions`
- `/mcp`
- `^/config/.*`
- `/system`

This lets the frontend follow the backend if it starts on `8080..8089`.

## Notes

- The frontend currently assumes local / trusted deployment.
- There is no built-in API-key login UX for release-mode backend auth.
- `src/pb/*` is generated; regenerate from the repo root with `make proto`.
