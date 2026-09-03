# Result-oriented security validation (Glasswing-inspired)

Agent eBPF Filter can optionally run a **result-oriented validation pass** after the normal Research security evaluation. The design is inspired by Cloudflare Project Glasswing's public vulnerability-harness write-ups: keep discovery broad, then reduce noise with reachability analysis, independent/adversarial validation, reproduction evidence, correlation/deduplication, and structured reporting.

This mode is **opt-in**. Existing `security_eval` behavior remains prediction/classification-oriented unless an outcome option is selected.

## Why this exists

A model or heuristic saying “this may be vulnerable” is not the same as showing that an attacker can reach the code path and produce a security-relevant effect. Outcome validation therefore tracks an evidence ladder:

1. `hypothesis` — a candidate exists, but there is no validation proof yet.
2. `reachable` — an authorized trace/validator explicitly records that the relevant path is reachable.
3. `reproduced` — an authorized validation producer records a successful reproduction marker.
4. `impact_confirmed` — an authorized validation producer records confirmed security impact.

Only candidates at or above `minimumEvidence` become `actionable` findings. The default outcome threshold is `reproduced`.

A candidate's original Research event does **not** prove reachability by itself. This prevents the evidence ladder from collapsing into “the detector saw an event, therefore the vulnerability is reachable.” Reproduction and confirmed impact imply reachability.

Built-in benchmark fixtures are useful for evaluating the detector itself, but they are not treated as proof that a real deployment is exploitable. They are marked `not_applicable` during outcome validation.

## UI option

In **Research -> Security Eval -> Corpus**, choose one of:

- `当前会话 · 结果验证` (`session_outcome`)
- `内置 + 会话 · 结果验证` (`combined_outcome`)

These aliases enable outcome validation with conservative defaults:

- `minimumEvidence = reproduced`
- `adversarialReview = true`

The ordinary `combined`, `builtin`, and `session` options keep the existing prediction-oriented behavior.

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
      "adversarialReview": true
    }
  }'
```

Accepted aliases for `validationMode` include `outcome`, `result`, `proof`, `poc`, and `glasswing`. `minimumEvidence` accepts `hypothesis`, `reachable`, `reproduced`, or `impact_confirmed`.

The normal mode is equivalent to:

```json
{
  "params": {
    "validationMode": "prediction"
  }
}
```

## Supplying proof evidence

Outcome validation intentionally does **not** launch arbitrary exploit commands against external systems. A separate, explicitly authorized validator (for example, a disposable VM/container test harness or a human-run PoC) should feed its result back into the Research Session as event `features`.

The validator recognizes the following truthy markers (flat dotted keys or nested objects are both accepted):

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

Example nested evidence:

```json
{
  "features": {
    "validation": {
      "reachable": true,
      "reproduced": true,
      "impactConfirmed": false
    }
  }
}
```

A proof event is correlated to a candidate in this order:

1. exact `eventId`;
2. same `traceId`;
3. same `spanId`;
4. same `comm` + `target` within a 30-second window.

Correlation only determines which proof belongs to which candidate; correlation alone is not considered proof. A recognized proof marker still has to be present.

This keeps the backend focused on evidence collection and validation while allowing the actual PoC environment to stay isolated and explicitly scoped.

## Report fields

When outcome mode is enabled, `results.securityEvaluation` adds:

- `validationMode: "outcome"`
- `outcomeValidation.minimumEvidence`
- `outcomeValidation.adversarialReview`
- counts for `unproven`, `reachable`, `reproduced`, `impactConfirmed`, `rejected`, and `actionable`
- `outcomeValidation.findings` containing only findings that passed the configured evidence threshold

Each sample can additionally contain:

- `validationStatus`
- `evidenceLevel`
- `reachable`
- `reproduced`
- `impactConfirmed`
- `actionable`
- `validatorReason`
- structured `evidence[]`

The outcome-mode `posture` is also evidence-oriented: confirmed impact and reproduced actionable findings drive blocking status, while ordinary detector FP/FN quality remains visible as diagnostic context instead of being mistaken for proof of exploitability.

## Recommended workflow

A practical workflow matching the intent of Cloudflare's public harness architecture is:

1. **Recon / capture:** build a Research Session with representative runtime traces.
2. **Hunt:** run normal `security_eval` (heuristics and optionally LLM) broadly.
3. **Trace:** an authorized tracer determines whether attacker-controlled input can reach the candidate and records explicit reachability proof.
4. **Validate:** in an authorized disposable environment, try a narrowly scoped reproduction and write proof markers back into the session.
5. **Adversarial review:** let a second validator explicitly refute candidates when the threat model, reachability, or observed effect does not hold.
6. **Report:** prioritize only `actionable` findings and retain unproven hypotheses separately.

This separation is deliberate: discovery can over-report to maximize recall, while remediation queues can be driven by evidence instead of speculation.
