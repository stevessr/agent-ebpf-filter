# Independent component boundaries — phase 1

> Scope: safe, incremental decomposition of `agent-ebpf-filter` into independently testable components. These are **Go library packages**, not separate deployable services yet. Current CLI, systemd, HTTP, protobuf and MyGo entrypoints remain unchanged.

## Why this boundary

The main Go `app` package previously combined configuration/HTTP orchestration with unrelated deterministic policy calculations. Those pure algorithms can be tested without root, kernel BTF, eBPF objects, a web server, or external services.

| Component | Imports | Responsible for | Must not do |
| --- | --- | --- | --- |
| `backend/internal/agentscope` | Go standard library | Scope list validation, defaults, exact comm/tag/verified-owner admission | Infer owner from a PID; call LSM/cgroup; load config files; start HTTP |
| `backend/internal/eventnoise` | Go standard library | Ignored-path normalization, path-boundary matching, ordinary telemetry suppression | Suppress denial/alert/high-risk events; change enforcement; persist settings |
| `backend/internal/agentidentity` | Go standard library | Bounded observed-root cache, run identity verification, TTL and conservative PID reuse handling | Guess an Agent owner from process name; trust child-declared roots; consume protobuf or change execution permissions |
| `backend/app` | App, protobuf, internal components | Adapt protobuf to pure policy input; own runtime locking, API routes, persistence, and verified root ancestry | Re-implement the same pure rules in multiple handlers |
| `desktop/renew` | Native UI / local IPC | Present validated state and read-only evidence, request authorized policy edits | Load eBPF independently; bypass backend runtime gates |

## Compatibility retained in this phase

1. `agentScopePolicy` and `agentScopeList` remain app-level aliases, preserving the JSON fields `capture`, `monitor`, `mode`, and `entries`.
2. Scope admission remains a case-insensitive **exact** command/tag match. A root name is used only after existing trusted ancestry logic has supplied it. A child cannot assert its own owner.
3. Capture and monitoring lists remain independent. Empty allowlists reject all; empty blocklists allow all. Admission decisions **do not** enforce execution denial.
4. `ignoredPaths: []` still disables path noise filtering. A `nil` list stays distinguishable during normalization.
5. Existing configured path prefixes retain prior behavior. Extra low-risk system paths only suppress ordinary open/read/metadata events. The `/usr/bin` exception only affects read/write events.
6. Any event marked BLOCK/DENY/ALERT, with risk score >= 60, or typed as `semantic_alert`/`agentsight_alert`, bypasses noise filtering.
7. No protobuf field changes, HTTP route changes, database migrations, process launcher changes or new privileges are introduced.
8. Security hardening: malformed UTF-8 Agent names are now rejected **before** lowercase normalization; prior normalization could replace invalid bytes with Unicode replacement characters. Valid policy files are unaffected.

## Data flows

```text
             ┌─────────────────────┐
Agent hooks ─┤                     │
eBPF events ─┤ app (context/IPC)   ├─────────> existing WS / archive / Renew
             └──────────┬──────────┘
                        │ invokes pure rules
             ┌──────────┴──────────┐
        internal/agentscope   internal/eventnoise
               internal/agentidentity
          scoped admission       noise admission
```

## Tests

Rootless component tests:

```sh
cd backend
go test -race ./internal/agentscope ./internal/agentidentity ./internal/eventnoise
```

Existing integration contracts to retain:

```sh
cd backend
go test ./app -run 'TestAgentScope|TestDefaultIgnoredEventPaths|TestNormalizeIgnoredEventPaths|TestPathMatchesIgnoredPrefix|TestShouldIgnoreEventPath'
go test ./...
```

The full backend integration suite requires the repository's declared Go toolchain and dependencies. Kernel enforcement still requires separate privileged smoke tests; the isolated tests make **no** claim about BPF LSM/cgroup effectiveness.

## Subsequent extraction plan (not implemented by this PR)

1. **Collector** — isolate eBPF loading, attachments, ringbuf reads, and capability detection behind a versioned event transport; retain a single privileged owner of pinned BPF resources.
2. **Correlation/analysis** — separate ancestry/session correlation, semantic alerts, and ML scoring from transport or user-facing HTTP handlers.
3. **Policy controller** — isolate authenticated policy validation/compilation from cgroup/LSM map application; add transactional apply/rollback and observed enforcement-state responses.
4. **Transport** — introduce explicit versioned protobuf/UDS contracts between collector, analysis and control components. Maintain embedded mode for backward compatibility.
5. **Desktop** — let Renew consume bounded read-only summaries through local IPC and request narrowly scoped control actions; preserve single-instance ownership and safe shutdown.

Do **not** turn these into independent privileged network daemons until auth, lifecycle, event loss, buffering, version skew and no-root degradation have been tested. Favor independent Go packages first, independently runnable processes only where isolation materially improves security or operations.

## Phase 2: observed Agent root identity extracted

The bounded Agent root cache has been moved from `app/agent_scope_root.go` into `internal/agentidentity`. Only an event with `pid == root_agent_pid` and a nonempty command establishes a root. Child sessions must match an observed root PID and its run token exactly; an unversioned event cannot silently downgrade a known run. Capacity remains 4096 and TTL remains 24 hours. The protobuf adaptation stays in `app` so future collectors or transports can share the identity engine without importing runtime HTTP/protobuf packages. Deterministic clock injection enables independent TTL tests. This cache is still **not** a complete historical event graph or a security boundary for untrusted caller-supplied PID metadata.
