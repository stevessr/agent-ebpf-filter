# CodeCity — Makepad GPU visualizer (experimental)

Independent, **unprivileged, read-only** Rust/Makepad viewer for Agent eBPF Filter's compact event feed. It does **not** replace Renew/MyGo, read file contents, access BPF maps or change policy. The MyGo v0.5.5 dependency update is merged separately in PR #111.

## Scene and capabilities

- GPU-instanced 3D cuboid buildings, orbit/zoom controls, per-directory pastel streets, camera-projected district and active file labels
- Cyan for reads, teal for ordinary file metadata/access, pink for writes, amber for alerts, red for blocked operations
- Vertical signal beams only for observed file events; 8 highlighted file names at most
- Bounded optional local source-tree **metadata-only** scan, never following symlinks or opening source files
- File event ownership based on actual `rootAgentPid`, `agentRunId` or explicit known harness tag, not arbitrary shell commands
- Bounded history (2,048 file events), dedupe cache (4,096 event IDs), capped 2,048 rendered buildings and no unbounded event queue
- Real-time transport-health indicator and message-loss counters separate from the event queue: failed reconnects do not appear healthy

## Running from a Linux devcontainer / CodeSpace

Build on a GPU-equipped Linux workstation or within a CodeSpace with working desktop display/graphics forwarding. Headless Codespaces cannot render a native 3D window by themselves.

```sh
cd desktop/visualizer-makepad
AGENT_EBPF_WORKSPACE_ROOT=/workspaces/agent-ebpf-filiter \
AGENT_EBPF_WS_URL=ws://127.0.0.1:8080/ws/event-summaries \
cargo run --release
```

For an authenticated backend, set `AGENT_EBPF_API_TOKEN` in the viewer's environment (it is sent in the WebSocket `X-API-KEY` header). Remote endpoints **must** use `wss://` with TLS and have the exact `/ws/event-summaries` path. Query-string credentials and URL userinfo are deliberately rejected. Never expose an unauthenticated eBPF backend directly on a public CodeSpace forwarded port.

### Remote CodeSpace without a shared filesystem

To populate the initial city when your GUI runs on a different machine from the CodeSpace, first export **file names only** in the CodeSpace:

```sh
python3 scripts/codecity-export-index.py /workspaces/agent-ebpf-filiter \
    --output /tmp/codecity-index.json
```

Copy that JSON to your own desktop over your existing authenticated channel, e.g. SCP. The viewer never connects to the CodeSpace filesystem on its own:

```sh
AGENT_EBPF_INDEX_JSON=/path/to/copied/codecity-index.json \
AGENT_EBPF_WORKSPACE_ROOT=/workspaces/agent-ebpf-filiter \
AGENT_EBPF_WS_URL=wss://YOUR-AUTHENTICATED-ENDPOINT/ws/event-summaries \
cargo run --release
```

The index is a bounded, static list of at most 2,048 relative filenames, excluding hidden directories, symlinks and common generated folders; no source text, file sizes, hashes or mtime are transmitted. Actual activity still flows live over the secured Agent eBPF event stream. Re-export or transfer a new index when you need fresh file discovery; this is **not** automatic remote filesystem sync. Without a shared mount, the directory contents shown may be older than the events.

**Rendering controls:** drag to orbit, mouse wheel to zoom, click/tap a tower to inspect the last in-scope event, ←/→ to step through recorded event indices, Esc to resume event replay.

### Adjustable time spans and Agent/Subagent lanes

The first toolbar has 1m, 5m, 15m, 1h and All presets. The `−`/`+` buttons halve/double the span (clamped from 10 seconds to 24 hours); **Older/Newer** moves the entire window by one span; **Live** resets the temporal offset and resumes latest events. Both tower activity colors and counters use **only events inside the chosen time interval**, not an unrelated lifetime total. Building geometry remains visible but dim when no matching file activity occurs. `All` means the retained local 2,048-event buffer, **not** all events since system boot.

The second toolbar isolates **All actors / Main / Child processes / Subagents ✓** and offers `◀ Lane` / `Lane ▶` to inspect one individual lane at a time. It keys actors using `agentRunId`, `rootAgentPid` and **actual executor `pid`**, so concurrent child process activity is not merged into its parent Agent's activity. The individual-lane cycle only includes actors observed in the current time span.

**Subagent trust boundary:** an OS child process is *not necessarily* an SDK Subagent. The experimental compact `/ws/event-summaries` adapter now emits an optional, **64-bit truncated SHA-256 pseudonym** as `subagentId` for authenticated `SubagentStart`/`SubagentStop` native-hook lifecycle signals only, derived from the existing privacy-safe digest in detailed telemetry. It does not send the raw SDK `agent_id`, prompt or a full-length SHA-256 digest. The shortened digest is the existing 8-byte prefix from native-hook detailed telemetry; it is a pseudonym, not a cryptographic proof of causal ownership. **Child processes** are explicitly labeled as candidates. **Subagents ✓** and per-lane selection can now isolate distinct observed hook identities, but **individual file syscalls still cannot be attributed to that Subagent** in the absence of a causal SDK ID on the file event. This is not a PPID-based or complete RPC call tree. Unattributed events remain separate, not silently assigned to an Agent. Separately, when the authenticated collector sends `native_hook` events named `SubagentStart`/`SubagentStop` (or `AgentSpawn`), the HUD displays in-window **HOOKS +start/-stop** counts. These signals never create a file tower or falsely attribute a file change, and still do not prove SDK parent-child causality or task duration. Counts are subject to the same bounded event transport and time filters. If a lifecycle signal has no usable identity, it remains an unverified signal and is not fused with another Subagent solely by PID.

Time-window selection depends on the backend `receivedAtMs` clock; events without positive timestamps are omitted from timed activity. For remote CodeSpaces, synchronize clocks (or account for clock drift). Buffered history and the 2,048-building limit are bounded, and the viewer cannot reconstruct events absent from capture. During replay, new events still enter the bounded history but do not rewrite the paused scene. The top HUD distinguishes LIVE/REPLAY/OFFLINE and lists file count, observed events, Agent-attributed events and dropped events. Replay is a *visualization* of the bounded event window; it does not rewind kernel state or recover evicted history.

By default only absolute event paths inside `/workspaces/<repo>` are displayed, and their UI paths retain the repository name to prevent collisions. An explicitly configured `AGENT_EBPF_WORKSPACE_ROOT` restricts activity to that repository even when that directory exists only in a **remote** CodeSpace. If that directory also exists on the renderer host as a real, non-symlink directory, it additionally seeds up to 2,048 files through local metadata-only enumeration. Remote inventory sync is not yet implemented. You can further constrain the stream view to **only confirmed Agent events** with `AGENT_EBPF_AGENT_ONLY=1`, or exact identity with `AGENT_EBPF_AGENT_RUN_ID=<run-id>` and/or `AGENT_EBPF_AGENT_ROOT_PID=<PID>`. Filters never infer an owner from an inherited shell name, file path, or substring.

The visual city heights are stable illustrative hashes **not** bytes, source lines, file risk or execution time.

## Limitations and architecture

This is a separate experimental desktop executable, not yet embedded in MyGo. Geometrically exact GPU depth-buffer picking (the current projected-rooftop hit-test is approximate), real GPU/label screenshots on actual machines, browser-native Makepad WASM deployment, automated remote CodeSpace manifest refresh, official Windows packaging, timeline persistence and source-code diffs are **not** verified/implemented. Without a local workspace root the city is populated by observed events only. No fabricated Agent edit events or complete-file-tree claims.

The observer is subject to eBPF capture scope, upstream redaction, event transport loss and the event summary window; it cannot infer code changes that the collector never saw. Source texts, API keys and contents of unrelated files remain outside the renderer.

## Validation

```sh
cargo check --manifest-path desktop/visualizer-makepad/Cargo.toml
cargo test --manifest-path desktop/visualizer-makepad/Cargo.toml
```

See [CodeCity CI](../../.github/workflows/codecity-makepad.yml). Compilation and unit tests cannot establish that a graphical Wayland/X11 session renders correctly; GPU interaction still requires a hands-on desktop smoke test.
