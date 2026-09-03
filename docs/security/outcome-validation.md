# Result-oriented security validation (Glasswing-inspired)

Agent eBPF Filter can optionally run a **result-oriented validation pass** after the normal Research security evaluation. The design is inspired by Cloudflare's public Project Glasswing and vulnerability-harness write-ups: discovery is intentionally broad, while remediation is driven by independently checked reachability and proof rather than by a model's confidence alone.

This mode is **opt-in**. Existing `security_eval` behavior remains prediction/classification-oriented unless an outcome option is explicitly selected.

> This implementation is Glasswing-inspired, not a reproduction of Cloudflare's private production harness. It implements the pieces that fit Agent eBPF Filter's Research Session model: evidence gating, correlation, independent refutation, deterministic deduplication, scope controls and structured reporting. It does not implement Cloudflare's fleet-wide cross-repository VDH/VVS orchestration.

## What is taken from the public Cloudflare design

Cloudflare describes a Vulnerability Discovery Harness (VDH) with stages such as Recon, Hunt, Validate and Gapfill, followed by a separate Vulnerability Validation System (VVS) that performs deduplication, judgment/reachability work and fixing. Their public guidance emphasizes several properties that matter here:

- treat discovery models as interchangeable components rather than the source of truth;
- persist state outside model context;
- use an isolated/adversarial validator that tries to disprove findings;
- mechanically validate structured output;
- deduplicate variants before they inflate the remediation queue;
- distinguish “a flaw exists” from “attacker-controlled input reaches the flaw”;
- optimize for sound, verifiable fixes rather than raw finding volume.

Agent eBPF Filter maps those principles onto runtime evidence rather than attempting to copy Cloudflare's entire fleet scanner.

## Evidence ladder

A model or heuristic saying “this may be vulnerable” is not the same as showing that an attacker can reach the code path and produce a security-relevant effect. Outcome validation therefore tracks an evidence ladder:

1. `hypothesis` — a candidate exists, but there is no accepted validation proof yet.
2. `reachable` — an authorized tracer/validator explicitly records that the relevant path is reachable.
3. `reproduced` — an authorized validation producer records a successful reproduction.
4. `impact_confirmed` — an authorized validation producer records confirmed security impact.

Only candidates at or above `minimumEvidence` become `actionable`. The default outcome threshold is `reproduced`.

Reproduction implies reachability. Confirmed impact implies both reproduction and reachability.

A candidate's original Research event does **not** prove reachability by itself. Correlation only associates a validator event with a candidate; a recognized proof marker must still be present and must pass the authorization and scope checks described below.

Built-in benchmark fixtures remain useful for evaluating the detector, but they are not evidence that a real deployment is exploitable. They are marked `not_applicable` during outcome validation.

## Safe defaults

Selecting an outcome corpus alias enables the following defaults:

- `minimumEvidence = reproduced`
- `adversarialReview = true`
- `requireAuthorization = true`
- `requireIndependentRefutation = true`
- `dedupeActionable = true`
- `correlationWindowSeconds = 30`

The correlation window is capped at 300 seconds even when a larger value is supplied.

These defaults mean that simply injecting `validation.reproduced=true` is **not sufficient**. The proof event must also be explicitly authorized when the normal outcome path is used.

## UI option

In **Research -> Security Eval -> Corpus**, choose one of:

- `当前会话 · 结果验证` (`session_outcome`)
- `内置 + 会话 · 结果验证` (`combined_outcome`)

The ordinary `combined`, `builtin`, and `session` options keep the existing prediction-oriented behavior.

The UI aliases intentionally use conservative defaults. Advanced validator/source/target scoping is configured through task `params` so that broad scope expansion is explicit and reviewable.

## API usage

Create/build a Research Session first, then submit the existing `security_eval` task with outcome validation enabled:

```bash
curl -X POST "$BASE/research/sessions/$SESSION_ID/tasks" \
  -H 'Content-Type: application/json' \
  -H "X-API-KEY: $TOKEN" \
  -d '{
    "action": "security_eval",
    "evaluationMode": "session",
    "includeLLM": false,
    "params": {
      "validationMode": "outcome",
      "minimumEvidence": "reproduced",
      "adversarialReview": true,
      "requireAuthorization": true,
      "requireIndependentRefutation": true,
      "dedupeActionable": true,
      "correlationWindowSeconds": 30,
      "allowedValidatorSources": ["validator-prod*", "trace-validator"],
      "allowedAuthorizationIds": ["change-SEC-1234"],
      "allowedTargets": [
        "https://staging.internal.example/*",
        "unix:///run/agent-ebpf-filter-test.sock"
      ]
    }
  }'
```

Accepted aliases for `validationMode` include `outcome`, `result`, `proof`, `poc`, and `glasswing`. `minimumEvidence` accepts `hypothesis`, `reachable`, `reproduced`, or `impact_confirmed`.

The normal prediction mode is equivalent to:

```json
{
  "params": {
    "validationMode": "prediction"
  }
}
```

### Advanced outcome parameters

| Parameter | Default | Meaning |
| --- | --- | --- |
| `minimumEvidence` | `reproduced` | Minimum evidence level required for `actionable=true`. |
| `adversarialReview` | `true` | Consume explicit refutation markers. |
| `requireAuthorization` | `true` | Ignore proof/refutation events without an authorization marker. |
| `requireIndependentRefutation` | `true` | A refuter must have a different validator identity from the producer of accepted proof. |
| `dedupeActionable` | `true` | Collapse actionable variants by stable `findingKey` in the remediation queue. |
| `correlationWindowSeconds` | `30` | Window for the weakest `comm + target` correlation. Clamped to `1..300`. |
| `allowedValidatorSources` | unrestricted | Optional exact or trailing-`*` source allowlist. |
| `allowedAuthorizationIds` | unrestricted | Optional exact authorization-ID allowlist. |
| `allowedTargets` | unrestricted | Optional exact or trailing-`*` target allowlist. Candidates outside it are `out_of_scope`. |

A trailing `*` is intentionally only a **prefix wildcard**, not a general regular expression. This keeps scope matching understandable and avoids surprising expansions.

## Validator evidence contract

Outcome validation does **not** launch arbitrary exploit commands against external systems. A separate, explicitly authorized validator — for example a disposable VM/container harness or a human-run PoC — feeds its result back into the Research Session as event `features`.

### Required authorization

With the default `requireAuthorization=true`, a proof or refutation event must contain one of:

- `validation.authorized = true`
- `outcome.authorized = true`
- `proof.authorized = true`

For auditable deployments, also provide:

- `validation.validatorId` — stable identity of the validator implementation/run actor;
- `validation.authorizationId` — change ticket, test authorization, CI run approval, or other scope grant;
- `validation.runId` — identifier for the isolated validation run.

When `allowedAuthorizationIds` is configured, the event's authorization ID must be in that allowlist. When `allowedValidatorSources` is configured, the event source must match that allowlist as well.

Example:

```json
{
  "source": "validator-prod-01",
  "traceId": "trace-abc",
  "features": {
    "validation": {
      "authorized": true,
      "validatorId": "glassbox-validator-b",
      "authorizationId": "change-SEC-1234",
      "runId": "validator-run-2026-09-03-001",
      "reproduced": true
    }
  }
}
```

The authorization marker is a policy gate inside the Research evidence model; it is not a cryptographic capability. Production ingestion should still restrict who can submit Research events and which authorization IDs are accepted.

## Proof markers

Flat dotted keys and nested objects are both accepted.

### Reachability

- `validation.reachable`
- `outcome.reachable`
- `trace.reachable`
- `proof.reachable`

### Reproduction

- `validation.reproduced`
- `validation.proof`
- `outcome.reproduced`
- `outcome.success`
- `poc.success`
- `proof.success`
- `exploit.reproduced`

### Confirmed impact

- `validation.impactConfirmed`
- `outcome.impactConfirmed`
- `impact.confirmed`
- `proof.impactConfirmed`

### Independent/adversarial refutation

- `validation.rejected`
- `validation.refuted`
- `outcome.rejected`
- `proof.rejected`

A refutation is considered independent by comparing validator identities. `validation.validatorId` is preferred; if it is absent, event `source` is used as the fallback identity. With `requireIndependentRefutation=true`, the same validator identity cannot prove and then independently refute its own finding.

## Candidate correlation

A proof event is associated with a candidate using the first matching rule below:

1. explicit `validation.candidateId` / `outcome.candidateId` / `proof.candidateId` equal to the sample `id`;
2. explicit `validation.candidateEventId` / equivalent equal to the candidate `eventId`;
3. exact event ID;
4. same `traceId`;
5. same `spanId`;
6. same `comm` + `target` within the configured correlation window.

The final `comm + target` fallback requires timestamps on both events. Missing timestamps do **not** create an unbounded match.

Explicit candidate IDs are recommended for validators that run outside the original trace because they avoid accidental association with another event that happens to share a command or target.

Correlation is recorded on each accepted `evidence[]` entry so exported reports explain *why* the backend associated the proof with the candidate.

## Target scope

`allowedTargets` makes the validation boundary explicit. A candidate outside the allowlist is marked:

```json
{
  "validationStatus": "out_of_scope",
  "actionable": false
}
```

An empty `allowedTargets` means “do not add an additional target filter”; it does **not** mean “all targets are authorized for active exploitation.” This subsystem only consumes evidence and never launches an exploit itself.

For active validators, use a separate sandbox-level allowlist/network policy as the primary enforcement mechanism and use `allowedTargets` here as a second control and audit signal.

## Deduplication and finding keys

Cloudflare's public VVS design separates raw findings from a deduplicated triage queue. Agent eBPF Filter mirrors that idea mechanically:

- every candidate keeps its own sample row and evidence;
- an actionable sample receives a stable `findingKey`;
- `actionable` counts all actionable variants;
- `uniqueActionable` counts unique remediation records;
- `duplicateActionable` counts variants collapsed out of the remediation queue;
- `outcomeValidation.findings` contains the unique actionable queue when deduplication is enabled.

A validator can provide an explicit stable key using one of:

- `validation.findingKey`
- `outcome.findingKey`
- `proof.findingKey`
- `dedupe.key`
- `finding.key`

If no explicit key is supplied, the backend derives one deterministically from category, command and target; target-less findings also include trace/command context. Explicit keys are preferable when a validator understands the actual root cause.

## Conflicting evidence

Adversarial validation can disagree with prior evidence. The backend handles conflicts conservatively:

- independent refutation without confirmed impact => `validationStatus = rejected`, not actionable;
- a same-validator refutation => ignored when independent review is required;
- confirmed impact plus independent refutation => `validationStatus = conflicted`, `evidenceConflict = true`, and the finding remains actionable for human review.

Confirmed impact is not silently erased by a later contradictory marker.

## Report fields

When outcome mode is enabled, `results.securityEvaluation` adds:

- `validationMode: "outcome"`
- evidence threshold and validator-policy configuration;
- counts for `outOfScope`, `unproven`, `reachable`, `reproduced`, `impactConfirmed`, `rejected`, `conflicted`, `unauthorizedEvidence`, `nonIndependentRefutations`, `actionable`, `uniqueActionable`, and `duplicateActionable`;
- `outcomeValidation.findings` containing the deduplicated actionable queue.

Each sample can additionally contain:

- `validationStatus`
- `evidenceLevel`
- `findingKey`
- `reachable`
- `reproduced`
- `impactConfirmed`
- `evidenceConflict`
- `actionable`
- `validatorReason`
- structured `evidence[]`

Each accepted evidence record includes its correlation method, validator ID, authorization ID and validation run ID when available.

The outcome-mode `posture` is evidence-oriented: confirmed impact and unique reproduced findings drive blocking status; unauthorized evidence, non-independent refutations, conflicts and out-of-scope candidates are surfaced as evidence-integrity warnings. Ordinary detector FP/FN quality remains diagnostic context instead of being mistaken for proof of exploitability.

## Recommended workflow

A practical workflow aligned with the intent of Cloudflare's public harness architecture is:

1. **Recon / capture:** build a Research Session with representative runtime traces and known trust boundaries.
2. **Hunt:** run normal `security_eval` broadly using heuristics and, optionally, an LLM.
3. **Trace:** an authorized tracer determines whether attacker-controlled input can reach the candidate and records explicit reachability proof.
4. **Validate:** in a disposable, network-scoped environment, reproduce the issue and emit authorized proof with a run ID and authorization ID.
5. **Adversarial review:** use a distinct validator identity to try to disprove the candidate.
6. **Dedupe:** keep variants for research, but drive remediation from stable unique finding keys.
7. **Report:** export structured evidence and prioritize only actionable findings; retain unproven, rejected and out-of-scope candidates for audit/history.

This separation is deliberate: discovery can over-report to maximize recall, while remediation queues are driven by evidence instead of speculation.

## What this mode deliberately does not do

- It does not automatically execute exploit payloads.
- It does not treat a high LLM confidence score as proof.
- It does not treat the candidate event itself as reachability proof.
- It does not accept unauthorized proof by default.
- It does not let the same validator count as an independent adversarial reviewer by default.
- It does not expand target scope automatically.
- It does not delete duplicate samples; it only deduplicates the actionable queue.
- It does not claim fleet-wide cross-repository reachability analysis equivalent to Cloudflare's production VVS.

## Public references

- Cloudflare, **Project Glasswing: what Mythos showed us** (May 18, 2026): `https://blog.cloudflare.com/cyber-frontier-models/`
- Cloudflare, **Build your own vulnerability harness** (June 18, 2026): `https://blog.cloudflare.com/build-your-own-vulnerability-harness/`
- Cloudflare public security-audit skill: `https://github.com/cloudflare/security-audit-skill`
