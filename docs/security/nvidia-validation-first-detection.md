# NVIDIA-inspired validation-first detection engineering

This is an original, limited adaptation of NVIDIA/CrowdStrike's September 2026
validation-first detection engineering approach and NVIDIA OpenShell's runtime
boundary principles. It is **not** a port of SafeMind, Nemotron or OpenShell,
and does not claim their reported benchmark results.

References:
- https://developer.nvidia.com/blog/building-an-adaptive-agentic-cybersecurity-system-with-nvidia-nemotron/
- https://developer.nvidia.com/blog/add-runtime-controls-to-ai-agents-with-nvidia-openshell/
- https://developer.nvidia.com/blog/where-security-fits-in-an-ai-agent-stack/

## Implemented: bounded candidate-rule validation

A new authenticated, read-only endpoint replays a human- or model-authored
candidate against an existing, already-redacted Research Session:

`POST /research/sessions/:id/detections/replay`

Example JSON body:

```json
{
  "rule": {
    "id": "file-and-network-behavior",
    "description": "A correlated file access and curl process in one trace",
    "scope": "trace",
    "windowMs": 5000,
    "minSignals": 2,
    "signals": [
      {"field": "eventType", "value": "openat"},
      {"field": "comm", "value": "curl"}
    ]
  },
  "labels": {
    "trace:recorded-attack-run": "authoring_attack",
    "trace:separately-seeded-attack": "holdout_attack",
    "trace:independently-recorded-normal-run": "benign"
  }
}
```

The labels are supplied by an operator, using **actual scope IDs** present in
the selected session. The first request can omit `labels`; the response exposes
`observedScopes` to support subsequent manual annotations. Existing policy
decisions and risk scores are *not* treated as independent ground truth.
Only assign `holdout_attack` to a genuinely independent run; the engine cannot
prove provenance from a label string. Avoid replaying train and test evidence
from one repeated trace as if it were independent.

Supported exact-match fields are `eventType`, `source` and `comm`. Rules must
combine at least two different field dimensions, two to eight signals, a
10-600000 ms window, and a per-`trace` or per-`pid` correlation scope. The
linter rejects raw paths, IPs, URLs, wildcards, arbitrary field names, and
duplicate predicates. Unsupported fields cannot silently match. Incomplete
trace/PID attribution is counted as `skippedUnscoped`, never matched globally.
The replay sorts out-of-order telemetry and uses incremental sliding-window
signal counts. It captures bounded, event-ID-only evidence.

Operational bounds: 64 KiB request body, 20,000 events, 1,000 labels, eight
signals and 256 returned findings. The total unique detected-scope count is
reported separately if findings are truncated. The existing Research Session
store applies its own size, file-safety and authentication controls.

Report fields include:
- `issues`: schema/semantic lint diagnostics and invalid or missing labels.
- `findings`: correlated scopes, timestamps, predicate indices and evidence IDs.
- `metrics`: operator-labeled authoring coverage, held-out recall, benign false
  positive rate and precision; ratios without a denominator are omitted.
- `eligibleForHumanReview`: true only when both held-out and benign labels
  exist, every labeled held-out scope is detected and no labeled benign scope
  is flagged. This is a *candidate review gate*, not a security guarantee.
- `enforcementApplied`: always false for this endpoint.

No rule is installed, no sandbox permission is granted, no probe is attached,
and no plaintext TLS content is retrieved. An independent reviewer still
needs to assess behavioral grounding, scenario generalization and data
provenance; policy changes require the separately authenticated and explicitly
enabled policy-management route.

## Existing architecture and additional integration recommendations

| NVIDIA-inspired practice | Existing project building block | Follow-up boundary |
| --- | --- | --- |
| Schema-aware detection authoring | ResearchEvent, session files, source/event filters | Provide versioned enumeration of event fields and capture coverage before using an LLM |
| Telemetry grounding | eBPF, native hooks, AgentSight, optional TLS | Correlate run/tool/trace/proc identity; surface event loss and redaction blind spots |
| Artifact lint and replay | New bounded replay endpoint | Add replay suites with separately seeded attack and normal sessions |
| Independent review | Report and manual label workflow | Distinct reviewer context, signed rule provenance, human promotion |
| Enforcement outside the harness | Existing cgroup/BPF LSM/wrapper | Explicit capability boundary and authorization for each effect path |
| API-level governance | Existing TLS/API fingerprint/capture profiles | Authenticated outbound L7 supervisor required; cgroup/IP-only policy cannot distinguish GET from POST over TLS |
| Credential protection | Existing redaction | Move raw tokens to a separate credential proxy; redaction after capture is not secret isolation |

Do not auto-promote a rule based on one successful backtest or on model
confidence. Detection results may recommend *less* authority, never authorize
more. Keep TLS plaintext capture opt-in and redact before research export.
Document the exact effect paths protected by kernel controls: cgroup network
controls cannot inspect encrypted API methods by themselves; BPF LSM is
kernel-dependent and the current path matching has documented limitations.

## Local validation

```bash
cd backend
go test ./internal/detectionengineering ./app/research -count=1
```

The dedicated CI workflow also runs these checks on relevant pull requests.
A separate privileged Linux test is still required for cgroup/BPF LSM
enforcement; unit replay results make no kernel-enforcement claim.
