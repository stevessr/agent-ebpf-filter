# OpenShell / NVIDIA Open Agent Safety Platform: boundary and watchdog prototype

Verified source baseline, 2026-09-29: [OpenShell v0.1.2](https://github.com/NVIDIA/OpenShell/releases/tag/v0.1.2);
there is no verified OpenShell 2.0 release. The [Open Agent Safety Platform
announcement](https://investor.nvidia.com/news/press-release-details/2026/NVIDIA-Launches-Open-Agent-Safety-Platform-to-Secure-Agents-From-Testing-to-Deployment/default.aspx)
(2026-09-28) describes OpenShell plus the Sentry reference design on BlueField-4.
This repository is **inspired by the architecture**, not a bundled OpenShell,
Sentry, DOCA, Vera, BlueField or SMT implementation.

Sources for the distinctions:
- [NVIDIA platform technical article](https://developer.nvidia.com/blog/nvidia-open-agent-safety-platform-a-reference-for-continuous-in-silicon-agent-monitoring/)
- [OpenShell policy security and failure semantics](https://github.com/NVIDIA/OpenShell/blob/main/architecture/security-policy.md)
- [OpenShell policy prover model coverage](https://docs.nvidia.com/openshell/latest/how-it-works/policies/prover)
- [OpenShell runtime isolation](https://github.com/NVIDIA/OpenShell/blob/main/architecture/sandbox.md)

## Implemented offline review, not active OS enforcement

Authenticated read-only endpoint:

`POST /research/safety/evaluate`

The endpoint accepts an operator-supplied policy lineage, optional prior
revision, optional direct descendant policies, and externally *claimed*
observation metadata. Strict JSON decoding rejects unknown fields/trailing
values; requests are bounded by the research 64-KiB body limit. No keys,
plaintext, actual file contents, or raw API bodies are required.

Example:

```json
{
  "lineage": [
    {
      "id":"operator", "generation":4, "uid":1000,
      "expiresAtMs":50000,
      "grants":[
        {"domain":"model","resource":"inference.example:443","operation":"INFER","subject":"agent-bin"},
        {"domain":"filesystem","resource":"/workspace","operation":"read"}
      ]
    },
    {
      "id":"worker","parentId":"operator","generation":2,"uid":1000,
      "expiresAtMs":40000,
      "grants":[
        {"domain":"model","resource":"inference.example:443","operation":"INFER","subject":"agent-bin"}
      ]
    }
  ],
  "watchdog": {
    "agentId":"worker", "nowMs":5000,
    "heartbeatTimeoutMs":1000, "denialWindowMs":600,
    "denialThreshold":3,
    "observations":[
      {"id":"hb1","agentId":"worker","sequence":1,"timestampMs":4500,"kind":"heartbeat"},
      {"id":"model1","agentId":"worker","sequence":2,"timestampMs":4600,"kind":"model_access",
        "grant":{"domain":"model","resource":"inference.example:443","operation":"INFER","subject":"agent-bin"}},
      {"id":"gen1","agentId":"worker","sequence":3,"timestampMs":4700,"kind":"generation","observedGeneration":2}
    ]
  }
}
```

### Boundary reviewer

The exact finite-grant subset checker rejects ambiguous/wildcard rules,
duplicate grants, PID/UID changes, wrong parent identity, invalid generation,
expiry expansion beyond parent, and any child grant missing from its direct
parent. An optional previous revision also catches rollback/replay, identifies
added grants and always requires human review before authority increases.

Domains: exact filesystem read/write path, network TCP host:port and
REST method/absolute-path operation, tool name/EXEC, model endpoint/INFER and
credential profile/endpoint INJECT. Network/model `subject` may constrain
a binary/profile; empty subject is explicit unconstrained access within the
finite model. Exact tuples prevent accidental cross-product of host, operation
and binary, and do not equate CONNECT with REST permission. Paths are lexical
only, not symlink-canonicalized, and there is no DNS/runtime authority proof.

The result is `within_boundary`, `exceeds_boundary` or `unsupported`, with
bounded counterexamples. **Only `within_boundary` establishes subset within
the deliberately limited exact-grant model.** It is not the official
OpenShell prover (which uses SMT and has its own partial coverage), does not
establish kernel enforcement, and must not be treated as a universal formal
security proof. Unsupported input does not silently become ALLOW.

### Sentry-inspired metadata watchdog

The watchdog examines a finite sequence of externally supplied event IDs,
agent identities, monotonic sequence numbers and timestamps. It flags:
- missing/stale heartbeat and loss of the controller,
- duplicate/replayed telemetry and stale policy generations,
- unauthorized model and credential destinations,
- subagent delegation beyond immediate-parent grants,
- denial bursts and authority expiration.

It emits one of `observe`, `review`, or `quarantine_recommended` with evidence
IDs and reason codes. Both `Applied` and `EnforcementApplied` are always false,
and `IndependentHardwareAttestation` is false. A caller can forge JSON: this is
a test/review engine, **not** a physically out-of-band watchdog. The current
implementation does not stop processes, revoke credentials or block network
flows.

NVIDIA's Sentry reference design uses a separate BlueField-4 DPU trust domain
and DOCA. Hardware-style guarantees, attested telemetry, identity verification,
model-path interception, and guaranteed quarantine latency require an
independent privileged control plane and are not provided by this endpoint.

## Safe production migration path

1. Integrate real OpenShell as an optional external sandbox backend; use its
   official prover on the **effective policy** including provider grants.
   Treat unsupported/inconclusive/error as failed verification, not approval.
2. Run a separately privileged watchdog (or supported DPU backend) protected
   from the agent cgroup/namespace. Use authenticated, replay-resistant event
   delivery, trust-rooted identity and monotonic policy generations.
3. Gate the entire model egress path through an independent credential/L7
   proxy, matching exact destination, binary, method/path, identity and
   delegation; do not infer method permissions from cgroup IP filtering.
4. Allow operators to explicitly approve verified policy expansions; preserve
   immutable audit trails and follow OpenShell failure-mode semantics. Static
   Landlock/process settings require recreating the sandbox, not hot reload.
5. For production quarantine, bind a trusted decision to the existing cgroup
   and BPF LSM controllers with independent authorization, idempotent
   generations, rollback testing and privileged smoke tests.

The separate research/detection-engineering replay introduced in PR #31
remains a complementary **candidate validation** stage; neither it nor this
review endpoint writes to runtime policy, nor should LLM output be interpreted
as operational authority.

### Test

```sh
cd backend
go test ./internal/agentboundary ./internal/detectionengineering ./app/research -count=1
```

Run with the repository's generated Protobuf and eBPF build artifacts for
the research package, as in CI.
