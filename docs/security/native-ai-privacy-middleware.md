# Built-in AI privacy middleware (Renew / WebUI)

This feature belongs to **Agent eBPF Filter**, not to AstrLink. AstrLink stays an
optional, observer-only third-party integration in Renew. No AstrLink code, model
weights, account credentials, or dependency is imported into this project.

## Research baseline

AstrLink's published docs describe a local gateway that detects secrets and
PII, applies **warn / block / redact** policies, optionally restores placeholders
on the response, and records aggregate request activity. AstrLink also supports
multiple LLM wire protocols and model routing. The built-in implementation
below deliberately covers a smaller, verifiable surface:

| Capability | Native privacy gateway |
| --- | --- |
| Rule-based local detection | Secrets, emails, 10–15 digit phone candidates, checksum-validated payment cards |
| Request actions | Warn (passthrough), block (403), redact |
| Placeholder restoration | HTTP JSON/text and complete SSE events |
| Cross-request mapping | Never; random, per-request in-memory mappings |
| HTTP gzip payloads | Bounded decode, policy, re-encode |
| WebSocket frame inspection | **Not implemented**: rejects WS when policy enabled |
| LLM protocol conversion | Not implemented by the privacy layer |
| Local ML inference | Not yet integrated; rule engine only |
| MCP tool-call arguments | JSON member-aware classification, including task_history/session_memory default denial when routed over HTTP; no full MCP wire-level provenance tracking |
| Request metadata inspection | URL paths, decoded query parameters, custom headers and cookie egress guard; provider Authorization and API-key headers are explicitly exempt |
| eBPF attribution | Existing eBPF monitoring continues independently; no new claim of per-request causal attribution |

## Configuration

**WebUI:** Runtime Configuration → Domain Forward Proxy → 本地 AI 隐私过滤中间件.
**Renew Desktop:** 管理软件联动 → 内置 AI 隐私过滤网关.

The existing `/config/runtime` authenticated API stores the policy with
`domainForwardProxy.privacy`:

```json
{
  "domainForwardProxy": {
    "enabled": true,
    "httpPort": 8318,
    "bindAddress": "127.0.0.1",
    "httpsDisabled": true,
    "httpsPort": 8443,
    "defaultScheme": "https",
    "allowAnyHost": false,
    "routes": [
      {"host": "127.0.0.1", "upstream": "https://api.openai.com", "useUpstreamHost": true}
    ],
    "privacy": {
      "enabled": true,
      "action": "redact",
      "restoreResponse": true,
      "maxBodyBytes": 1048576,
      "categories": ["secret", "email", "phone", "payment_card"],
      "contextMode": "strict",
      "jsonFieldRules": [
        {"path": "/params/arguments/task_history", "action": "block"},
        {"path": "/payload/customerEmail", "action": "allow", "recipient": "api.openai.com"}
      ],
      "allowlist": []
    }
  }
}
```

For this specific **loopback-local** configuration, use the existing
OpenAI-compatible client base URL `http://127.0.0.1:8318/v1` (a client that
allows overriding its API base URL); send the provider API key using the
client's existing `Authorization` setting. Requests to the local proxy use
`Host: 127.0.0.1`, which resolves the explicit route, while
`useUpstreamHost: true` sends `Host: api.openai.com` to the upstream HTTPS
endpoint. No credential is saved in the proxy config.

`bindAddress: "127.0.0.1"` avoids exposing local unencrypted HTTP on other
interfaces. `httpsDisabled: true` disables the incoming HTTPS listener only;
outbound connections to upstream AI services still use HTTPS.

Example is illustrative: the proxy needs a suitable host route, inbound
listening ports and client configuration. HTTPS listener requires a separately
configured certificate; the privacy feature does **not** issue certificates or
silently enable MITM.

Routing is opt-in: only HTTP requests that **actually pass through** the
existing domain-forward data plane are inspected. Ordinary system TLS traffic
is not magically decrypted by eBPF. The policy defaults to disabled.
`action` accepts `warn`, `block`, and `redact`; invalid values normalize to
`block` rather than silently permitting.

## Failure behavior and data handling

- JSON values are inspected locally with a bounded, duplicate-key-rejecting
  decoder. JSON member names drive *field sensitivity decisions* without
  publishing them as raw findings; `text/plain` bodies also support pattern
  masking. Arbitrary multipart bodies and binary uploads remain unsupported
  and are **rejected** when protection is enabled (HTTP 422).
- In default **strict** context mode, known full-session / memory-export
  fields such as `task_history`, `session_memory` and `conversation_transcript`
  are denied independent of the global warn/redact action. Secret-labelled
  fields such as `api_key`, `private_key`, `password` are redacted even when
  their string values do not resemble common token regexes. Sensitive numeric
  JSON fields are denied instead of being silently converted into strings.
- Exact RFC 6901 `jsonFieldRules` support `allow`, `redact`, `drop` and `block`.
  `allow` is deliberately restricted to a scalar value **and** the explicitly
  named upstream recipient hostname; it never grants blanket sub-tree access.
  Invalid pointer rules fail closed, including on bodyless requests.
- URL paths, repeatedly decoded query parameters and non-authentication
  headers are also scanned; sensitive metadata is **denied** in redact/block
  modes because rewriting query/header values can invalidate semantics.
  Provider `Authorization` and API-key headers are exempt for authentication,
  so routes and recipients must be configured restrictively. Cookies are
  denied in strict mode. Metadata warn mode does not sanitize.
- Requests exceeding the configured maximum (default 1 MiB, cap 8 MiB),
  requests with unsupported content encoding, signed requests, or invalid
  encodings are rejected before an upstream request is issued. This is
  intentionally different from generic body rewriting, which may pass through.
- Matching requests are denied (403), forwarded unchanged (warn), or
  redacted before forwarding. A warning **does not** protect outbound PII.
- Redacted placeholders are unique per inbound HTTP request. Original values
  and mappings are retained only in request-scoped Go memory, never in
  persisted event logs or policy snapshots.
- HTTP response restoration uses bounded buffering; inability to decode or
  restore fails with a proxy error. SSE restoration works for placeholders
  contained in one complete SSE event even if underlying TCP reads split it;
  events larger than the limit or placeholders fragmented across **separate SSE
  events** may remain visibly redacted. Compressed SSE restoration is rejected.
- OpenAI Responses WebSockets are **rejected** when privacy protection is on,
  rather than bypassing an unimplemented frame protection path.
- Neither the UI nor the middleware records original request bodies. Existing
  proxy/error logging continues to apply, but this feature emits no matched
  plaintext values. Disable upstream server-side logging separately if needed.

Read [the 2025–2026 privacy-filter research review](../research/agent-privacy-filter-literature-2026.md)
for the paper-derived threat model, implementation mapping and evaluation plan.

Do not consider the patterns complete PII/secret detection. There are false
positives and false negatives, especially for regional telephone formats and
keys not matching common prefixes. For high-sensitivity traffic choose
**block** and run integration tests with representative synthetic data first.

## Validation

Run `go test ./internal/domainforwardproxy` from `backend/`, plus the
existing backend suite and `go test ./...` from `desktop/renew/`. WebUI
types/build checks use `bun run build` in `frontend/`.

AstrLink references:
- https://github.com/Calcium-Ion/AstrLink
- https://astrlink.com/en/
- https://astrlink.com/en/guard/
