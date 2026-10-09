# Renew × Local Agent Management Gateways (CC-Switch / AstrLink)

The native Renew desktop exposes **管理软件联动** in the Tools sidebar and activity rail. The page combines independent CC-Switch and AstrLink read-only adapters.

## CC-Switch scope

- Reads only the current Provider's `app_type`, `name`, and `is_current` from the CCS `providers` table.
- Reads only `app_type`, `listen_address`, `listen_port`, `enabled`, and `proxy_enabled` from CCS `proxy_config`.
- Aggregates the last hour of CCS local-proxy request counts, failures, token usage and average latency (only scalar aggregates, no request bodies).
- Shows corresponding Agent eBPF event-summary counts and opens a **local, bounded** Events-page filter using observed executable names.
- Refreshes CCS metadata every 20 seconds, and on the page's **刷新 CCS** action. It does not change the independent event polling cadence.

CCS defaults to `~/.cc-switch/cc-switch.db`. If CCS uses a custom directory, set the **absolute database filename** in `AGENT_RENEW_CCS_DB` when launching Renew. The desktop host needs the `sqlite3` CLI in `PATH` with `-readonly` and `-json` support. The database is **never created** by this integration; if CCS or SQLite is unavailable, Renew remains usable.

```sh
AGENT_RENEW_CCS_DB="$HOME/.cc-switch/cc-switch.db" agent-ebpf-renew
```

## Data boundary

The integration runs **only in the unprivileged Renew desktop process**; no CCS database access is added to the privileged eBPF backend. Queries use SQLite's read-only CLI flag and hard-coded allowlisted `SELECT` columns. The aggregate query reads only numerical usage counters, not API authentication tokens. They never select `settings_config`, tokens, provider endpoint URLs, proxy request bodies, prompts, MCP configurations, or CCS secrets. No API proxy calls are intercepted, and no backend capture/enforcement policy is changed.

### Interpretation of the display

- **Current Provider** means *CCS stored selection*. Routing, aggregation, or failover may result in a different real upstream.
- **Proxy configuration** is stored configuration; it does **not** prove that a port is listening or that requests actually use it.
- **Related events** means the eBPF event's actual `comm` matches a recognized agent executable. It does not prove that an event was sent through CCS, or that subprocess actions and provider use were associated with that event.

This is the minimal read-only interoperability layer. Future opt-in API-level integration should use a stable, authenticated CCS interface rather than sniffing API keys or intercepting encrypted provider traffic.

## AstrLink observer integration

AstrLink uses its documented local **Control API**. It publishes `~/.astrlink/control-session.json` at runtime. The default desktop integration uses its **same-user Unix socket** (see `control_socket`) with the Observer role and never requests the desktop Operator token. On Windows the session locator may contain a per-start **Observer-only bearer token** for a loopback HTTP control address; Renew limits it to literal 127.0.0.1 / ::1 and never stores or displays that credential. A session with an invalid address is rejected. The locator may be overridden by `AGENT_RENEW_ASTRLINK_SESSION` (absolute path); AstrLink's own `ASTRLINK_AGENT_HOME` is also honored for the default lookup.

Only the following endpoints are queried with **GET**:

- `/control/v1/health` — readiness check.
- `/control/v1/services?limit=100` — project to service **name, kind and enabled** state; follow at most 3 pages / 300 services with a 512-byte cursor bound.
- `/control/v1/policies` — project to **name, enabled, detector and actions** only. Never fetch regex patterns or allowlist literals.
- `/control/v1/usage-summary` — last-hour **aggregate** request count, failures and input/output tokens. It never requests original request records, audit bodies, request headers, OAuth sessions, credentials or access tokens.

AstrLink refreshes every 30 seconds or on explicit **刷新 AstrLink**. Missing session files, stopped sockets and unsupported API versions show a degraded state without affecting the eBPF collector. The **查看 AstrLink 网关进程事件** shortcut searches only the existing bounded eBPF event summaries and matches observed `astrlink-core` / `astrlink` process names; it does **not** assert that a particular client request or privacy decision caused a specific kernel event. External gateway statistics and kernel process events have distinct provenance and are not automatically merged into one count.

### Trust boundary

No integration code runs as root or changes the privileged agent backend. The AstrLink Unix Socket authenticates same-user readers with Observer rights, and the Windows adapter uses only an observer token from the protected session locator. All responses are decoded into allowlisted typed fields and capped at 1 MiB. Redirects, HTTP non-loopback addresses, raw records and write methods are not used.
