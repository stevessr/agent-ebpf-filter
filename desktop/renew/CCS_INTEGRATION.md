# Renew × CC-Switch (CCS)

The native Renew desktop exposes **CCS** in the Tools sidebar and activity rail.

## Scope

- Reads only the current Provider's `app_type`, `name`, and `is_current` from the CCS `providers` table.
- Reads only `app_type`, `listen_address`, `listen_port`, `enabled`, and `proxy_enabled` from CCS `proxy_config`.
- Shows corresponding Agent eBPF event-summary counts and opens a **local, bounded** Events-page filter using observed executable names.
- Refreshes CCS metadata every 20 seconds, and on the page's **刷新 CCS** action. It does not change the independent event polling cadence.

CCS defaults to `~/.cc-switch/cc-switch.db`. If CCS uses a custom directory, set the **absolute database filename** in `AGENT_RENEW_CCS_DB` when launching Renew. The desktop host needs the `sqlite3` CLI in `PATH` with `-readonly` and `-json` support. The database is **never created** by this integration; if CCS or SQLite is unavailable, Renew remains usable.

```sh
AGENT_RENEW_CCS_DB="$HOME/.cc-switch/cc-switch.db" agent-ebpf-renew
```

## Data boundary

The integration runs **only in the unprivileged Renew desktop process**; no CCS database access is added to the privileged eBPF backend. Queries use SQLite's read-only CLI flag and hard-coded allowlisted `SELECT` columns. They never select `settings_config`, tokens, provider endpoint URLs, proxy request bodies, prompts, MCP configurations, or CCS secrets. No API proxy calls are intercepted, and no backend capture/enforcement policy is changed.

### Interpretation of the display

- **Current Provider** means *CCS stored selection*. Routing, aggregation, or failover may result in a different real upstream.
- **Proxy configuration** is stored configuration; it does **not** prove that a port is listening or that requests actually use it.
- **Related events** means the eBPF event's actual `comm` matches a recognized agent executable. It does not prove that an event was sent through CCS, or that subprocess actions and provider use were associated with that event.

This is the minimal read-only interoperability layer. Future opt-in API-level integration should use a stable, authenticated CCS interface rather than sniffing API keys or intercepting encrypted provider traffic.
