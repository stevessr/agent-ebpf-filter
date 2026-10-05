# Domain Forward TLS Interception and Rewrite

`domainForwardProxy` is the user-space data plane for explicitly routed HTTP/HTTPS
traffic. The rewrite path is independent from the eBPF plaintext observer: eBPF can
observe supported TLS libraries, while this proxy can actively modify a request or
response only when traffic is routed through it.

## TLS interception boundary

Dynamic TLS interception is **off by default**. Enabling it requires all of:

- `tlsInterceptEnabled: true`;
- a local CA certificate and matching private key;
- a non-empty `tlsInterceptAllowlist`;
- client trust in that CA.

The allowlist accepts exact hosts and strict `*.suffix` patterns. A wildcard
authorizes subdomains only; it does not authorize the apex host. Dynamic leaf
certificates are only signed for allowlisted SNI names. The configured CA must be
currently valid and, when KeyUsage is present, must include certificate-signing
permission. CA expiry is rechecked before every cache-miss signature so a long-running
proxy cannot mint leaves after the CA expires. Signed leaves are parsed back into
complete x509 metadata before entering TLS, and generated leaves are cached in a
bounded 1024-host cache with concurrent read hits and oldest-expiry eviction. A host outside that dynamic allowlist does not receive a
generated certificate. Explicit route/default certificates remain available as
administrator-configured fallback material outside the dynamic MITM allowlist.

Example:

```json
{
  "domainForwardProxy": {
    "enabled": true,
    "httpPort": 8080,
    "httpsPort": 8443,
    "defaultScheme": "https",
    "allowAnyHost": false,
    "tlsInterceptEnabled": true,
    "tlsInterceptAllowlist": "api.openai.com,*.example.internal",
    "tlsInterceptCaCertFile": "/etc/agent-ebpf-filter/ca/ca.pem",
    "tlsInterceptCaKeyFile": "/etc/agent-ebpf-filter/ca/ca-key.pem",
    "tlsInterceptLeafTtlSeconds": 43200,
    "routes": [
      {
        "host": "api.openai.com",
        "upstream": "https://api.openai.com"
      }
    ]
  }
}
```

The CA private key must stay local. Do not commit it to the repository.

## Request and response rewrite order

For an eligible buffered request, the hot path applies:

1. top-level model alias mapping;
2. native local inference rewrite, when configured and loaded;
3. bounded literal rules.

For an eligible response it applies:

1. model-name restoration to the client-facing name;
2. native local inference rewrite;
3. bounded literal rules.

Responses with `text/event-stream` are processed one SSE event at a time rather
than buffering the complete stream. Multi-line `data:` fields are reassembled using
SSE semantics before model restoration/inference and are rendered back as data
fields afterwards. Oversized events are streamed through unchanged. Responses
WebSocket text frames use the same rewrite kernel.

Identity and gzip HTTP request/response bodies are eligible for bounded rewrite.
Both the encoded body and the decompressed payload are subject to the configured
rewrite limit; a gzip expansion beyond the limit is passed through unchanged.
Compressed SSE is intentionally left untouched so the proxy never turns an
unbounded event stream into whole-response buffering.

The body buffer defaults to 4 MiB and is capped at 64 MiB. An over-limit body is
passed through unchanged. Its original `io.Closer` is retained, so bypassing
rewrite does not leak a request/response stream. Response validators/digests are
removed only when payload bytes actually change; inspection-only passthrough keeps
the original validators.

## HTTP semantic safety

Body rewrite is conservative around HTTP semantics that cannot be regenerated
losslessly by the proxy.

Requests are passed through without body mutation when they carry a
`Content-Range`, HTTP Message Signature fields, AWS SigV4 payload signing,
Google content hashes, Azure SharedKey-style authorization, or recognizable
AWS/GCS/Azure/CloudFront signed-URL query parameters. Declared signature
trailers are treated as signed even before their values arrive at end-of-body.

When a request body is actually changed, stale body integrity fields are
removed from both headers and trailers:

- `Content-MD5`;
- `Digest`;
- `Content-Digest`;
- `Repr-Digest`.

Inspection-only request paths restore the original content length / transfer
encoding / trailer framing instead of silently converting a chunked or
trailer-bearing request into a fixed-length body. When a changed request still
has non-integrity trailers, the proxy leaves final framing selection to
`net/http` so HTTP/1.1 and HTTP/2+ can use their native trailer mechanisms.

HEAD responses and bodyless status classes (1xx, 204, 304) are never
body-rewritten, preserving representation metadata such as HEAD/304
`Content-Length` and validators. Responses with status `206 Partial Content`,
a `Content-Range`, `multipart/byteranges`, or HTTP Message Signature fields
in headers/trailers are also passed through unchanged.

A rewritten full response drops stale validators/digests from headers and
trailers while preserving unrelated trailers. Trailer-bearing responses remain
trailer-capable instead of being forced to a fixed `Content-Length`; unchanged
responses preserve their original trailer framing.

## Fast model aliases

A model alias changes only the top-level JSON `model` field. Nested values
named `model` inside user/tool content are not rewritten.

```json
{
  "rewrite": {
    "enabled": true,
    "maxBodyBytes": 4194304,
    "modelRules": [
      {
        "host": "api.openai.com",
        "from": "client-model",
        "to": "provider-model"
      }
    ]
  }
}
```

For Responses streaming/WebSocket traffic, the mapping is retained by
`stream_id`. A continuation carrying `previous_response_id` inherits the
known mapping only when it does not provide an explicit `model`; an explicit
model is authoritative.

Steering state follows the current WebSocket continuation model. Accepted
`response.steer` submissions are tracked by `steer.id`. If the parent
response terminates while steering input is still queued, the lane mapping is
held until the queued input is either committed into a successor or definitively
fails. A `response.steer.pending` flow resumed by explicit
`response.create` must use the parent response's original lane and reuses that
held lane rather than appending duplicate stream state.

Response history distinguishes an explicit "no alias" mapping from a missing
mapping, so an unaliased steer successor cannot accidentally consume the next
queued request's alias. Pinned steering/terminal responses are protected while
the history remains bounded at 256 entries by evicting older unpinned responses.

Server `error` events are routed conservatively: named-lane/request errors can
release that lane, while connection-level errors such as
`websocket_connection_limit_reached` do not consume the default lane's queued
model state.

## Native inference fast path

The native inference layer is intentionally small and process-local. It does not add
Python, ONNX Runtime, CGO, or a second HTTP service to the proxy hot path.

A model contains quantized `int8` label weights. For every eligible token the
kernel hashes 1-, 2-, and 3-byte n-grams into a fixed feature vector and performs
integer accumulation. JSON keys are skipped; only string values are candidates.

Runtime configuration:

```json
{
  "rewrite": {
    "enabled": true,
    "inference": {
      "enabled": true,
      "modelFile": "/etc/agent-ebpf-filter/models/rewrite-int8.json",
      "direction": "both",
      "host": "api.openai.com",
      "pathPrefix": "/v1/responses",
      "contentType": "application/json",
      "minTokenBytes": 3,
      "maxTokenBytes": 256
    }
  }
}
```

Model file version 1:

```json
{
  "version": 1,
  "dimension": 256,
  "seed": 7,
  "labels": [
    {
      "name": "private_email",
      "bias": 0,
      "threshold": 42,
      "replacement": "<PRIVATE_EMAIL>",
      "weights": [0, 1, -2]
    }
  ]
}
```

`weights` must contain exactly `dimension` signed 8-bit values; the shortened
array above only illustrates the schema. The runtime bounds the dimension to 4096 and
the label count to 32. Replacements must be JSON-safe token strings without quotes,
backslashes, tabs, or newlines.

If the model cannot be loaded, the proxy reports `inferenceReady: false` and
records an error in domain-forward status. It does **not** stop the proxy: deterministic
model aliases and literal rules continue to work.

A CI smoke benchmark on an AMD EPYC runner measured approximately:

- model no-match: ~116 ns/op, 0 allocations;
- model match: ~585 ns/op;
- 200 model rules with a match: ~1.53 µs/op;
- native inference: ~2.17 µs/op for 1 label, ~2.19 µs/op for 8 labels, and
  ~3.35 µs/op for 32 labels;
- nested Responses model restoration over a ~28 KiB completed event:
  ~125 µs/op (~230 MB/s).

These values are regression signals, not hardware-independent latency guarantees.

## Responses API WebSocket

The proxy recognizes an HTTP Upgrade on `/v1/responses` and proxies text frames
bidirectionally. Supported metadata includes:

- `response.create`;
- `response.cancel`;
- `stream_id`;
- `response_id`;
- `previous_response_id`;
- current flat Responses request bodies and the legacy nested `response` form.

Binary frames are passed through unchanged.

## Agent context metadata

Capture keeps display plaintext bounded and sanitized, but derives protocol/context
metadata from the complete request before display truncation.

`prompt_digest` / `prompt_len` describe the selected current message.
For Responses requests, the complete visible upstream input is additionally summarized
as:

- `context_digest`;
- `context_len`;
- `context_items`.

The context digest covers all extractable `input` items in order, including
system/developer/user messages and tool/function outputs. Raw full context is not
copied into the protobuf bridge; the digest and counts are propagated to AgentSight
and OTLP metadata instead.

Protocol correlation fields are also retained through the Codex capture sink:

- `protocol_event`;
- `stream_id`;
- `response_id`;
- `previous_response_id`.

This prevents the upstream half of a Responses/WebSocket turn from appearing empty
merely because the UI body sample was truncated.
