# HTTPS / HTTP compression capabilities

This document applies to the **domain forwarding proxy** and the **TLS plaintext capture UI** in Agent eBPF Filter. The native Renew desktop UI consumes the same backend event data; it does not need or deploy a separate HTTPS MITM.

## HTTPS and negotiated protocols

- The configured HTTPS domain-forwarding listener accepts TLS 1.2+ (including TLS 1.3 as supported by Go) and advertises ALPN `h2`, `http/1.1`.
- The listener and outbound `http.Transport` explicitly enable HTTP/1.1 and HTTP/2, including when a custom DNS resolver is configured.
- Per-route server certificates, optional default certificates and **allowlisted-only** TLS interception remain unchanged. No additional certificate trust or capture permissions are enabled by this feature.
- HTTP/3/QUIC is **not** implemented here; advertising `h3` over a TCP listener would be incorrect.

## HTTP content encoding

When rewrite rules are enabled, bounded request and response bodies may be processed with:

| `Content-Encoding` | Read / rewrite / re-encode | Notes |
| --- | --- | --- |
| empty / `identity` | Yes | Original uncompressed body |
| `gzip` | Yes | Go gzip |
| `br` | Yes | Brotli |
| `zstd` | Yes | Zstandard, decoder memory bound |
| unknown or stacked coding | No | Transparent pass-through; no partial rewrite |

`Accept-Encoding` is not stripped from upstream requests. **This does not mean the proxy blindly recompresses every response**: a response is re-encoded only when a configured rewrite actually modifies it. Unmodified representations retain their original wire bytes and validators. Modified representations have stale ETags and digest fields removed; content length is recalculated. Signed, ranged and partial responses continue to skip rewriting.

The rewrite size limit applies to **both compressed wire bytes and decoded bytes**. A response above the configured limit is forwarded without rewriting. Expansion beyond the limit is not materialized. SSE (`text/event-stream`) remains streaming; compressed SSE is forwarded byte-for-byte **without body rewriting**. Existing WebSocket handling remains a separate streaming path.

## TLS plaintext capture / Renew

- When an opted-in TLS plaintext capture contains a complete HTTP request/response with `Content-Encoding: gzip`, `br` or `zstd`, its body is decoded **only for display** before the existing formatting, image folding and secret redaction steps.
- The original compressed byte count is retained. Display output is still limited to 32 KiB. Compressed frames that are incomplete, invalid or use unsupported stacked encodings display a fixed placeholder instead of raw binary bytes.
- A decompression limit hit or decode failure marks the preview truncated. Kernel capture-level truncation is never cleared.
- Capturing HTTPS plaintext is **not** equivalent to decrypting arbitrary network TLS traffic; availability still depends on enabled userspace TLS probes and successful reassembly. Existing consent, redaction and privilege gates remain in place.

## Validation

From the repository:

```sh
cd backend
go test ./internal/domainforwardproxy ./app/tls
```

Tests cover gzip/br/zstd codec round trips, rewritten request and response representations, byte/expansion bounds, signed and unsupported coding pass-through (existing tests), compressed SSE pass-through, captured-body redaction, and a real HTTP/2 ALPN TLS connection. A build with the repository's declared Go version (1.27.1+) is required.

### Boundaries

This change does **not** add QUIC/HTTP/3, proxy protocol translation for WebSocket over HTTP/2 (RFC 8441), automatic compression of the admin REST API, or arbitrary HTTPS decryption. These are separate transport and threat-model changes.
