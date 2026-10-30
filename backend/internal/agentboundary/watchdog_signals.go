package agentboundary

// evaluateEscapeSignal handles metadata-only signals from a trusted collector.
// No signal here proves a physical sandbox escape: the caller must independently
// verify kernel return values, process identity and provenance.
func evaluateEscapeSignal(policy Policy, event Observation, out *WatchdogReport) bool {
	// Reject overloaded fields instead of silently discarding contradictory
	// telemetry. ChildID and generation have meanings only for other kinds.
	if event.ChildID != "" || event.ObservedGeneration != 0 {
		switch event.Kind {
		case "network_attempt", "network_connected", "dns_query",
			"filesystem_access", "tool_exec", "monitor_tamper",
			"evaluation_artifact_mutation":
			finding(out, "ambiguous_escape_signal", event.ID, true)
			return true
		}
	}
	switch event.Kind {
	case "network_attempt":
		// An ungranted connect attempt can be blocked by the kernel. It is
		// suspicious, but cannot be reported as a successful escape.
		if !observedGrantAllowed(policy, event.Grant, "network") {
			finding(out, "unapproved_network_attempt", event.ID, false)
		}
	case "network_connected":
		// Only emit for independently observed successful connect outcomes;
		// a syscall entry or SYN alone is insufficient to claim success.
		if !observedGrantAllowed(policy, event.Grant, "network") {
			finding(out, "network_connection_outside_grants", event.ID, true)
		}
	case "dns_query":
		// DNS names are not proof of an established Internet connection.
		// Grant denotes the observed resolver endpoint (e.g. localhost:53),
		// not the untrusted text of a DNS name.
		if !observedGrantAllowed(policy, event.Grant, "network") {
			finding(out, "dns_egress_scope_unverified", event.ID, false)
		}
	case "filesystem_access":
		if !observedGrantAllowed(policy, event.Grant, "filesystem") {
			finding(out, "filesystem_access_outside_grants", event.ID, true)
		}
	case "tool_exec":
		if !observedGrantAllowed(policy, event.Grant, "tool") {
			finding(out, "tool_execution_outside_grants", event.ID, true)
		}
	case "monitor_tamper":
		// The collector reports an attempt to disable or modify monitoring;
		// it must not execute any requested action or trust agent prose.
		if event.Grant != nil {
			finding(out, "ambiguous_escape_signal", event.ID, true)
		} else {
			finding(out, "monitor_tamper_signal", event.ID, true)
		}
	case "evaluation_artifact_mutation":
		// Tests and benchmarks are often legitimately edited. This signal
		// prompts context-aware review, not an assertion of sabotage.
		if event.Grant != nil && event.Grant.Domain != "filesystem" {
			finding(out, "ambiguous_escape_signal", event.ID, true)
		} else {
			finding(out, "evaluation_artifact_review", event.ID, false)
		}
	default:
		return false
	}
	return true
}

func observedGrantAllowed(policy Policy, grant *Grant, domain string) bool {
	return grant != nil && grant.Domain == domain && HasGrant(policy, *grant)
}
