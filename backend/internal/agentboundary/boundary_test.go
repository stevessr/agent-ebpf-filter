package agentboundary

import "testing"

func rootPolicy() Policy {
	return Policy{
		ID: "operator", Generation: 4, UID: 1000, ExpiresAtMS: 50000,
		Grants: []Grant{
			{Domain: "filesystem", Resource: "/workspace", Operation: "read"},
			{Domain: "network", Resource: "api.github.com:443", Operation: "GET:/repos"},
			{Domain: "tool", Resource: "git", Operation: "EXEC"},
			{Domain: "model", Resource: "inference.example:443", Operation: "INFER", Subject: "agent-bin"},
			{Domain: "credential", Resource: "api.github.com:443", Operation: "INJECT", Subject: "github-token"},
		},
	}
}

func childPolicy() Policy {
	parent := rootPolicy()
	return Policy{
		ID: "worker", ParentID: parent.ID, UID: parent.UID,
		Generation: 1, ExpiresAtMS: 40000,
		Grants: append([]Grant(nil), parent.Grants[0], parent.Grants[2], parent.Grants[3]),
	}
}

func hasReason(report Report, reason string) bool {
	for _, c := range report.Counterexamples {
		if c.Reason == reason {
			return true
		}
	}
	return false
}

func TestLineageExactSubsetAndFailures(t *testing.T) {
	p, child := rootPolicy(), childPolicy()
	if r := CheckLineage([]Policy{p, child}); r.Status != "within_boundary" || r.Applied {
		t.Fatalf("expected bounded child, got %+v", r)
	}
	child.Grants = append(child.Grants, Grant{Domain: "network", Resource: "evil.example:443", Operation: "POST:/write"})
	r := CheckLineage([]Policy{p, child})
	if r.Status != "exceeds_boundary" || !hasReason(r, "child adds authority absent from immediate parent") {
		t.Fatalf("unexpected authority escalation result: %+v", r)
	}
	child = childPolicy()
	child.ExpiresAtMS = p.ExpiresAtMS + 1
	if r := CheckLineage([]Policy{p, child}); r.Status != "exceeds_boundary" {
		t.Fatalf("child expiry escaped parent: %+v", r)
	}
	child = childPolicy()
	child.ParentID = "another"
	if r := CheckLineage([]Policy{p, child}); r.Status != "unsupported" {
		t.Fatalf("broken ancestry should not pass: %+v", r)
	}
	child = childPolicy()
	child.UID = 0
	if r := CheckLineage([]Policy{p, child}); r.Status != "unsupported" {
		t.Fatalf("privilege change should not pass: %+v", r)
	}
}

func TestUnsupportedPolicyIsNotSilentlyProved(t *testing.T) {
	base := rootPolicy()
	var examples = []Grant{
		{Domain: "network", Resource: "*.example.com:443", Operation: "GET:/"},
		{Domain: "network", Resource: "api.github.com:443", Operation: "GET:/repos/*"},
		{Domain: "network", Resource: "api.github.com:443", Operation: "GET:/repos?x=1"},
		{Domain: "filesystem", Resource: "/workspace/../etc", Operation: "read"},
		{Domain: "credential", Resource: "api.github.com:443", Operation: "INJECT"},
		{Domain: "model", Resource: "https://inference.example", Operation: "INFER"},
		{Domain: "graphql", Resource: "api.github.com:443", Operation: "QUERY"},
	}
	for i, g := range examples {
		p := base
		p.Grants = append([]Grant(nil), base.Grants...)
		p.Grants = append(p.Grants, g)
		if r := CheckLineage([]Policy{p}); r.Status != "unsupported" {
			t.Errorf("case %d silently accepted: %+v", i, r)
		}
	}
}

func TestUpdateRequiresReviewOnNewGrantOrStaleGeneration(t *testing.T) {
	prev := childPolicy()
	candidate := childPolicy()
	candidate.Generation = prev.Generation + 1
	candidate.Grants = append(candidate.Grants, rootPolicy().Grants[1])
	r := CheckUpdate([]Policy{rootPolicy(), candidate}, &prev)
	if r.Status != "within_boundary" || !r.RequiresApproval || len(r.NewGrants) != 1 || r.Applied {
		t.Fatalf("new access must require review: %+v", r)
	}
	candidate.Generation = prev.Generation
	r = CheckUpdate([]Policy{rootPolicy(), candidate}, &prev)
	if r.Status != "exceeds_boundary" || !hasReason(r, "generation rollback or replay") {
		t.Fatalf("rollback must fail: %+v", r)
	}
	candidate = childPolicy()
	candidate.Generation = prev.Generation + 1
	r = CheckUpdate([]Policy{rootPolicy(), candidate}, &prev)
	if r.Status != "within_boundary" || r.RequiresApproval {
		t.Fatalf("pure revision should not expand authority: %+v", r)
	}
}

func baseWatchdog() WatchdogRequest {
	return WatchdogRequest{
		AgentID: "worker", NowMS: 5000, HeartbeatTimeoutMS: 1000,
		DenialWindowMS: 600, DenialThreshold: 3,
		Observations: []Observation{
			{ID: "hb1", AgentID: "worker", Sequence: 1, TimestampMS: 4500, Kind: "heartbeat"},
			{ID: "m1", AgentID: "worker", Sequence: 2, TimestampMS: 4600, Kind: "model_access",
				Grant: &Grant{Domain: "model", Resource: "inference.example:443", Operation: "INFER", Subject: "agent-bin"}},
			{ID: "g1", AgentID: "worker", Sequence: 3, TimestampMS: 4700, Kind: "generation", ObservedGeneration: 1},
		},
	}
}

func hasWatchdogReason(r WatchdogReport, code string) bool {
	for _, f := range r.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func TestWatchdogModelPathGenerationHeartbeatAndDrift(t *testing.T) {
	p := childPolicy()
	req := baseWatchdog()
	r := EvaluateWatchdog(p, nil, req)
	if r.Verdict != "observe" || r.QuarantineRecommended || r.EnforcementApplied || r.IndependentHardwareAttestation {
		t.Fatalf("valid metadata should be observation-only: %+v", r)
	}
	req.Observations[1].Grant = &Grant{Domain: "model", Resource: "unapproved.example:443", Operation: "INFER", Subject: "agent-bin"}
	r = EvaluateWatchdog(p, nil, req)
	if !hasWatchdogReason(r, "unauthorized_model_access") || !r.QuarantineRecommended {
		t.Fatalf("%+v", r)
	}
	req = baseWatchdog()
	req.Observations[2].ObservedGeneration = 0
	r = EvaluateWatchdog(p, nil, req)
	if !hasWatchdogReason(r, "policy_generation_mismatch") {
		t.Fatalf("%+v", r)
	}
	req = baseWatchdog()
	req.NowMS = 6000
	r = EvaluateWatchdog(p, nil, req)
	if !hasWatchdogReason(r, "heartbeat_timeout_or_unobserved") {
		t.Fatalf("%+v", r)
	}
	req = baseWatchdog()
	req.Observations = append(req.Observations,
		Observation{ID: "d1", AgentID: "worker", Sequence: 4, TimestampMS: 4800, Kind: "policy_denial"},
		Observation{ID: "d2", AgentID: "worker", Sequence: 5, TimestampMS: 4900, Kind: "policy_denial"},
		Observation{ID: "d3", AgentID: "worker", Sequence: 6, TimestampMS: 4999, Kind: "policy_denial"})
	r = EvaluateWatchdog(p, nil, req)
	if r.Verdict != "review" || !hasWatchdogReason(r, "repeated_policy_denials") {
		t.Fatalf("%+v", r)
	}
}

func TestWatchdogReplayDelegationAndDisconnect(t *testing.T) {
	p := childPolicy()
	req := baseWatchdog()
	req.Observations = append(req.Observations, Observation{
		ID: "m1", AgentID: "worker", TimestampMS: 4800, Sequence: 4, Kind: "heartbeat",
	})
	r := EvaluateWatchdog(p, nil, req)
	if !hasWatchdogReason(r, "duplicate_event_id") {
		t.Fatalf("%+v", r)
	}
	req = baseWatchdog()
	req.Observations = append(req.Observations, Observation{
		ID: "child", AgentID: "worker", TimestampMS: 4800, Sequence: 4, Kind: "delegate", ChildID: "unknown",
	})
	r = EvaluateWatchdog(p, nil, req)
	if !hasWatchdogReason(r, "unauthorized_subagent_delegation") {
		t.Fatalf("%+v", r)
	}
	req = baseWatchdog()
	req.Observations = append(req.Observations, Observation{
		ID: "lost", AgentID: "worker", TimestampMS: 4800, Sequence: 4, Kind: "supervisor_disconnected",
	})
	r = EvaluateWatchdog(p, nil, req)
	if !hasWatchdogReason(r, "lost_control_plane") || r.EnforcementApplied {
		t.Fatalf("%+v", r)
	}
}

func TestCaseCannotPromoteOrEnforce(t *testing.T) {
	p, child := rootPolicy(), childPolicy()
	report := EvaluateCase(SafetyCase{
		Lineage: []Policy{p, child}, Watchdog: baseWatchdog(),
	})
	if report.Boundary.Status != "within_boundary" || report.Watchdog.Verdict != "observe" ||
		report.Applied || report.Watchdog.EnforcementApplied {
		t.Fatalf("review-only endpoint must not mutate OS state: %+v", report)
	}
}

func TestSubagentCanDelegateOnlyItsOwnAuthority(t *testing.T) {
	parent := childPolicy()
	sub := Policy{
		ID: "subworker", ParentID: parent.ID, UID: parent.UID,
		Generation: 1, ExpiresAtMS: 30000,
		Grants: []Grant{parent.Grants[0]},
	}
	if r := CheckDirectDelegation(parent, sub); r.Status != "within_boundary" {
		t.Fatalf("valid subagent delegation rejected: %+v", r)
	}
	sub.Grants = append(sub.Grants, rootPolicy().Grants[1])
	if r := CheckDirectDelegation(parent, sub); r.Status != "exceeds_boundary" {
		t.Fatalf("subagent borrowed ancestor-only authority: %+v", r)
	}
}
