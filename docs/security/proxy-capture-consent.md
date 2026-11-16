# Opt-in HTTP / SOCKS context capture and certificate trust

Agent eBPF Filter has a separate local **HTTP, SOCKS5, SOCKS4/SOCKS4a** debugging proxy. This is not Domain Forward Proxy (the Host/SNI-based reverse proxy) and is not kernel eBPF capture.

## User workflow

Open **WebUI → Runtime Configuration → Proxy Capture** or **Renew Desktop → 管理软件联动 → 本机代理与上下文捕获**.

1. Explicitly set the HTTP, SOCKS5 and SOCKS4 ports; zero disables a protocol. At least one distinct, nonprivileged port (1024–65535) must be selected.
2. Save the configuration. Saving never opens a listener.
3. Read the consent prompt and explicitly Start. Only 127.0.0.1 is bound. Neither the system proxy nor any environment variables are changed.
4. Direct only your own, authorized application's traffic to the selected local proxy.
5. Stop capture when finished. Up to 300 sessions are retained only in memory and can be cleared manually; capture will **not** restart automatically after a backend restart.

HTTP transaction metadata contains the method, host, URL path (no query parameters), response status, elapsed time and content type. All request cookies and Authorization headers are excluded. JSON preview is **off by default**. When enabled, JSON bodies up to 4096 bytes are sampled and sensitive *key names* are masked. This is best-effort: arbitrary string values and personal data are not guaranteed to be removed. Do not collect data without consent.

SOCKS4/SOCKS5 and HTTPS CONNECT without explicit TLS interception record destination metadata only. When TLS interception is enabled for an allowlisted destination on port 443, SOCKS4/SOCKS5 CONNECT also supports HTTP/1.1 HTTPS-context capture (other SOCKS traffic stays opaque). No traffic is transparently intercepted. The capture proxy does not implement SOCKS UDP ASSOCIATE, HTTP/2 MITM, WebSocket upgrade or full mitmdump scripting.

## Explicit HTTPS trust steps

TLS interception is **off by default** and never installs a certificate automatically.

1. Click Generate CA and read/confirm the CA generation warning. This creates an independent local signing certificate (maximum 180 days) and a private key only on the backend host; existing CAs cannot be overwritten.
2. Download the public PEM from WebUI or use Renew Desktop's native certificate import action. Compare its SHA-256 fingerprint with the fingerprint shown in the app.
3. Choose, **manually**, whether to trust this certificate in the system root store. Windows: certlm.msc → Trusted Root Certification Authorities → Import, approve the OS prompt. macOS: import in Keychain Access and explicitly select trust. Linux: use the distribution's CA-management workflow with administrator authorization; browser trust stores may be independent. The WebUI cannot silently elevate browser permissions or install a system root.
4. Turn on TLS interception and enter an exact-domain allowlist. Wildcards such as *.example.com include subdomains, not the apex. Only allowlisted HTTP or SOCKS CONNECT destinations on HTTPS are decrypted and their SNI must match the CONNECT host. Upstream TLS certificates continue to be verified.
5. Save and confirm Start; remove the CA from the system trust store when debugging is over.

The CA private key must never be sent through the API. Root certificate trust is system-wide and enables TLS impersonation by anyone controlling the private key. Run only on controlled devices. The backend restricts configuration, CA generation and start/stop to authenticated *local* clients.

## Authenticated API routes

- GET /network/proxy-capture/status — state, ports, CA fingerprint
- PUT /network/proxy-capture/config — configuration, allowed only when stopped
- POST /network/proxy-capture/start — requires the exact explicit consent string I_UNDERSTAND_PROXY_CAPTURE
- POST /network/proxy-capture/stop — close listeners
- GET /network/proxy-capture/flows — most recent 300 captures
- DELETE /network/proxy-capture/flows — delete in-memory flows
- POST /network/proxy-capture/ca/generate — requires GENERATE_CAPTURE_CA
- GET /network/proxy-capture/ca — download the *public certificate*, never the key

Example: after choosing HTTP port 8899 and confirming Start, configure an authorized client with HTTP proxy http://127.0.0.1:8899. SOCKS5 proxy port 8900, if separately enabled, would be socks5h://127.0.0.1:8900.

Automatic OS trust-store changes and silent interception are intentionally unsupported.