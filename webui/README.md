# GPUI Web dashboard

This directory is the Rust/WASM replacement for the legacy Vue frontend on the `rust-variant` branch.

GPUI now ships a browser platform (`gpui_web`) upstream. The app is compiled to `wasm32-unknown-unknown` with Trunk and uses the same backend REST, protobuf and WebSocket contracts as the former Vue dashboard.

## Development

```bash
rustup toolchain install nightly --component rust-src rustfmt clippy --target wasm32-unknown-unknown
cargo install trunk --locked
cd webui
trunk serve
```

The backend remains the source of truth. Authentication reuses the existing browser storage keys:

- `agent-ebpf.apiToken`
- `agent-ebpf.clusterTarget`

The legacy `frontend/` tree is retained only as a migration fallback until GPUI feature parity tests cover every control surface.
