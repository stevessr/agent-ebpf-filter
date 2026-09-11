# Agent TUI

Terminal dashboard for a running Agent eBPF Filter backend. It subscribes to
the same protobuf `EventBatch` feed (`/ws`) the web dashboard consumes, backfills
recent history from `/events/recent`, and renders a live, filterable event table
with throughput and risk summaries.

```bash
make tui                                            # from the repository root
make tui TUI_ARGS="-backend http://host:8080 -token $TOKEN"
make tui-build && bin/agent-tui -history 20000      # standalone binary
```

## Connection discovery

The backend origin is taken from the first of: `-backend`, `AGENT_BACKEND_URL`,
`AGENT_BACKEND_PORT`, a `backend/.port` file written by a running dev backend
(searched upward from the current directory), then `http://127.0.0.1:8080`.

The API token (sent as `X-API-KEY`) comes from `-token`, `AGENT_ACCESS_TOKEN`,
or `~/.config/agent-ebpf-filter/runtime.json` (`accessToken`). Under `sudo`
the invoking user's home is used so the token the backend wrote for that user
is found. Dev-mode backends with auth disabled need no token.

## Keys

| Key            | Action                                                     |
|----------------|------------------------------------------------------------|
| `/`            | Edit the filter; `Enter` applies, `Esc` cancels            |
| `Esc`          | Clear the active filter / close an overlay                 |
| `Enter`        | Show every populated field of the selected event           |
| `Space`        | Pause / resume following the newest event                  |
| `↑ ↓ PgUp PgDn`, mouse wheel | Browse history (pauses following)            |
| `End`, `G`     | Jump to the newest event and follow again                  |
| `Home`         | Jump to the oldest retained event                          |
| `s`            | Cycle the sidebar histogram: event types → processes → tags |
| `c`            | Clear retained history and statistics                      |
| `?`            | Key reference                                              |
| `q`, `Ctrl+C`  | Quit                                                       |

## Filter syntax

Terms are ANDed. Free text matches comm, type, path, endpoint, domain, tag and
extra info case-insensitively. Keyed terms narrow to one field:

```
type:openat  comm:node  path:/etc  tag:"AI Agent"  net:github.com
decision:block  tool:Bash  run:<agent-run-id>  pid:1234  ppid:1  uid:1000  risk:>=40
```

Prefix a term with `-` to negate it: `-type:read -type:write`.

## Design notes

- Events are decoded once from the protobuf frame and stored by pointer in a
  fixed-capacity ring; the table is a virtual `tview.TableContent` over a
  filtered snapshot, so only the visible cells are ever materialised.
- Redraws are coalesced by a 200 ms ticker; the event rate never drives the
  draw rate.
- Event-derived text is escaped before rendering so process names or paths
  cannot inject tview style tags or terminal control sequences.
- Untrusted-input filtering (`containsFold`) and the clean-text fast path are
  allocation-free; see the unit tests for the guarantees.
