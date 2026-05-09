# Security Model

## Current privilege split

Today the backend still combines:

- eBPF bootstrap / map access
- HTTP + WebSocket APIs
- UI/static serving
- local UDS wrapper control
- replay / export / storage

That means the current security posture depends heavily on:

- release-mode API authentication
- dangerous-feature runtime gating
- UDS peer-credential checks
- least-privilege child command execution

## Control-plane protections already present

- release-mode token auth for `/config/**`, `/system/**`, `/ws*`, `/metrics`, `/events/*`, `/register`, `/unregister`, and shell-session APIs
- per-hook secret support for `/hooks/event`
- PTY, `/system/run`, hook installation, and policy mutation disabled by default until explicitly enabled
- `/tmp/agent-ebpf.sock` created with `0600`
- peer credential verification on the UDS wrapper socket

## Data-plane trust assumptions

- kernel/eBPF events are treated as the factual execution source
- wrapper/native hook events are semantic declarations
- the system raises alerts when the semantic layer and factual layer diverge

## Planned hardening direction

The roadmap target is to split into:

- `agent-ebpf-privileged`
  - load / attach eBPF
  - manage pinned maps and links
  - read ringbuf
  - apply future cgroup / LSM policy
- `agent-ebpf-server`
  - HTTP / WS / UI
  - storage / replay
  - graph / export / OTLP / MCP

That split is **not fully implemented yet**. This document should stay honest about that boundary.

## Operational guidance

- treat the app as a local workstation tool unless reverse-proxy auth and host hardening are also in place
- rotate runtime tokens if you enable release-mode remote access
- keep hook secrets per CLI rather than reusing one global secret everywhere
- enable PTY or `/system/run` only when you explicitly need them
