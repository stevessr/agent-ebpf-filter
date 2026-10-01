# Rust variant

This branch is the long-lived Rust rewrite of Agent eBPF Filter. The compatibility target is the current `master` branch, not a redesign.

## Rules

- Keep the protobuf schema, REST paths, WebSocket payloads, UDS framing, environment variables and CLI flags compatible.
- Port behavior before deleting the old implementation.
- Prefer safe Rust. `unsafe` is restricted to kernel/eBPF and narrowly reviewed OS boundaries.
- Use Aya/Aya eBPF for kernel programs where the verifier and kernel support allow it.
- Do not silently weaken enforcement. A Rust component that is not feature-complete must remain explicitly experimental.
- Configuration, protobuf, Markdown, systemd/Kubernetes manifests and other declarative assets are not rewritten merely to change their file extension.

## Migration map

| Area | Current implementation | Rust target | Status |
| --- | --- | --- | --- |
| Protocol bindings | Go / JS generated code | `agent-proto` + prost | bootstrapped |
| Shared framing/context | Go helpers | `agent-common` | bootstrapped |
| Wrapper | Go | `agent-wrapper` | first compatible implementation |
| PID adapter | Python / Node | `agent-adapter` | first compatible CLI implementation |
| HTTP/UDS backend | Go/Gin | `agent-backend` (Axum/Tokio) | bootstrap server only |
| eBPF tracepoints/maps | C/libbpf | Aya eBPF | bootstrap program only |
| Web dashboard | Vue/TypeScript | GPUI Web + Rust/WASM | live migration: shell, routing, history + live protobuf stream |
| TUI/dev-env tools | Go | Rust | pending |
| ML/plugins/exporters | Go/Python | Rust | pending |
| kernel-ml DKMS module | C kernel module | Rust-for-Linux where viable | pending/kernel dependent |

## Build

```bash
cargo fmt --all --check
cargo clippy --workspace --all-targets --all-features -- -D warnings
cargo test --workspace
cargo build --workspace
```

The GPUI Web dashboard is an independent nightly/WASM workspace because upstream GPUI Web currently requires `wasm32-unknown-unknown`, atomics/shared memory and `build-std`:

```bash
make predev-webui
make frontend
# development:
make dev-frontend
```

Production assets are emitted to `webui/dist`. The backend and installer prefer this directory; `frontend/` is only a migration fallback.

The Aya eBPF crate is kept outside the default workspace because it uses a different target/toolchain:

```bash
cd rust/ebpf/agent-ebpf
cargo +nightly build --target bpfel-unknown-none -Z build-std=core
```

## Removal gate

Old Go/C/Python/TypeScript source stays on this branch until its Rust replacement passes the corresponding compatibility tests. This keeps the branch bisectable while the complete rewrite proceeds.
