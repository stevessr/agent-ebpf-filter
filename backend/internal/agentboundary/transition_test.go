package agentboundary

import "testing"

func TestTransitionFailClosedAndVerifiedLastValid(t *testing.T) {
	parent, previous := rootPolicy(), childPolicy()
	invalid := childPolicy()
	invalid.Generation = 2
	invalid.Grants = append(invalid.Grants, Grant{Domain: "tool", Resource: "danger", Operation: "EXEC"})
	chain := []Policy{parent, invalid}
	r := CheckUpdate(chain, &previous)
	if r.Status != "exceeds_boundary" {
		t.Fatalf("expected authority excess, got %+v", r)
	}
	closed := PlanTransition(chain, &previous, r, "fail_closed")
	if closed.Decision != "quarantine_recommended" || closed.PreviousPolicyRetained || closed.EnforcementApplied {
		t.Fatalf("invalid update should fail closed: %+v", closed)
	}
	retained := PlanTransition(chain, &previous, r, "retain_last_valid")
	if retained.Decision != "retain_last_valid" || !retained.PreviousPolicyRetained ||
		retained.ActiveGeneration != previous.Generation || retained.EnforcementApplied {
		t.Fatalf("verified previous policy should remain available: %+v", retained)
	}
	previous.Grants = append(previous.Grants, Grant{Domain: "tool", Resource: "danger", Operation: "EXEC"})
	retained = PlanTransition(chain, &previous, r, "retain_last_valid")
	if retained.Decision != "quarantine_recommended" || retained.EffectiveFailureMode != "fail_closed" {
		t.Fatalf("unverified previous policy may not be retained: %+v", retained)
	}
}

func TestTransitionReviewWhenAuthorityExpandedWithinBoundary(t *testing.T) {
	parent, prev := rootPolicy(), childPolicy()
	next := childPolicy()
	next.Generation = 2
	next.Grants = append(next.Grants, parent.Grants[1])
	chain := []Policy{parent, next}
	r := CheckUpdate(chain, &prev)
	if r.Status != "within_boundary" || !r.RequiresApproval {
		t.Fatalf("new grant must require human review: %+v", r)
	}
	plan := PlanTransition(chain, &prev, r, "retain_last_valid")
	if plan.Decision != "human_review_required" || !plan.PreviousPolicyRetained ||
		plan.ActiveGeneration != prev.Generation {
		t.Fatalf("new access cannot auto-promote: %+v", plan)
	}

	next = childPolicy()
	next.Generation = 2
	next.ExpiresAtMS = 45000
	r = CheckUpdate([]Policy{parent, next}, &prev)
	if r.Status != "within_boundary" || !r.RequiresApproval || len(r.ReviewReasons) == 0 {
		t.Fatalf("lifetime expansion must be reviewed: %+v", r)
	}
}

func TestTransitionRejectsInvalidModeAndForgedApproval(t *testing.T) {
	parent, prev := rootPolicy(), childPolicy()
	next := childPolicy()
	next.Generation = 2
	next.Grants = append(next.Grants, Grant{Domain: "tool", Resource: "not-in-parent", Operation: "EXEC"})
	chain := []Policy{parent, next}
	forgedReport := Report{Status: "within_boundary"}
	if plan := PlanTransition(chain, &prev, forgedReport, "fail_closed"); plan.Decision != "quarantine_recommended" {
		t.Fatalf("forged precomputed report bypassed subset verification: %+v", plan)
	}
	next = childPolicy()
	next.Generation = 2
	chain = []Policy{parent, next}
	if plan := PlanTransition(chain, &prev, forgedReport, "fail_open"); plan.Decision != "invalid_failure_mode" ||
		plan.EffectiveFailureMode != "fail_closed" || plan.EnforcementApplied {
		t.Fatalf("unknown failure mode silently accepted: %+v", plan)
	}
	req := baseWatchdog()
	result := EvaluateCase(SafetyCase{
		Lineage: chain, Previous: &prev, FailureMode: "fail_open", Watchdog: req,
	})
	if result.Transition.Decision != "invalid_failure_mode" || !result.Watchdog.QuarantineRecommended ||
		!hasWatchdogReason(result.Watchdog, "invalid_failure_mode") {
		t.Fatalf("invalid mode escaped watchdog review: %+v", result)
	}
}
