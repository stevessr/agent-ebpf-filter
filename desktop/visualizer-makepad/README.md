# CodeCity — Makepad GPU visualizer (experimental)

An independent, **unprivileged, read-only** Rust/Makepad renderer for the Agent eBPF Filter compact event stream. It does **not** replace Renew/MyGo, access BPF maps, enforce policy, or read source-code contents.

## Why separate Makepad?

MyGo manages daily monitoring, alerts, native dialogs and OS integration. Makepad provides an optional 3D scene: pastel cyan/pink buildings grouped by top-level folders, camera orbit/zoom, and animated columns on real file operations. When `AGENT_EBPF_WORKSPACE_ROOT` points to an accessible local directory, a bounded, symlink-safe **metadata-only file index** seeds the initial buildings without reading file contents. Otherwise, buildings appear only for paths observed in the event stream; activity beams always require real observed events and are never invented from Agent prompts. Directory grouping is visual, not a source dependency graph.

## Running inside a Linux CodeSpace / devcontainer

Run Agent eBPF backend as normal, then in the devcontainer with GPU/display forwarding (or on a Linux desktop connected to a remote WSS event endpoint):

```sh
cd desktop/visualizer-makepad
AGENT_EBPF_WS_URL=ws://127.0.0.1:8080/ws/event-summaries \
AGENT_EBPF_WORKSPACE_ROOT=/workspaces/agent-ebpf-filiter \
cargo run --release
```

For secured API authentication set `AGENT_EBPF_API_TOKEN` in the process environment; this is transmitted as `X-API-KEY` on the WS handshake, never in the URL. Remote endpoints **must** be `wss://`. Do not forward an unauthenticated backend port publicly. A headless CodeSpace cannot display a native GPU window; run on a desktop with display forwarding or a GPU-enabled CodeSpace. A separate browser/WASM frontend is **not** claimed here.

Optional `AGENT_EBPF_WORKSPACE_ROOT` strictly filters observed paths by directory boundary; when omitted the viewer only accepts paths under `/workspaces/<repo>/`. Redacted/relative/non-file targets are omitted. If no events are present or capture is unhealthy, no fabricated activity is shown. Only existing compact-summary fields are used, respecting upstream collection scope and redaction.

**Scene:** drag to orbit; scroll to zoom. City buildings are instance-rendered GPU cuboids, with cyan for reads, turquoise for other file access, pink for writes, orange for alerts, red for blocked operations. Bright vertical pulses decay after new events; unrelated network and tool events are not painted as file writes.

## Status

An initial standalone Makepad integration, not a production-ready alternative to Renew. The current backend compact event stream does not provide a complete repository file tree or per-file source size/line count; buildings and height are visual encodings of observed file paths, not filesystem utilization. CodeSpace-to-desktop file-tree sync, timeline scrubbing, per-file picking and full cross-platform packaging remain follow-up work.

```sh
cargo test --manifest-path desktop/visualizer-makepad/Cargo.toml
cargo build --manifest-path desktop/visualizer-makepad/Cargo.toml --release
```
