package agentboundary

// TransitionPlan models the operator's choice after policy validation. It
// does not mutate BPF maps, start a sandbox, or activate a policy generation.
type TransitionPlan struct {
	SchemaVersion          string `json:"schemaVersion"`
	RequestedFailureMode   string `json:"requestedFailureMode"`
	EffectiveFailureMode   string `json:"effectiveFailureMode"`
	Decision               string `json:"decision"`
	ActiveGeneration       uint64 `json:"activeGeneration"`
	CandidateGeneration    uint64 `json:"candidateGeneration"`
	PreviousPolicyRetained bool   `json:"previousPolicyRetained"`
	EnforcementApplied     bool   `json:"enforcementApplied"`
}

// PlanTransition is a non-mutating analogue of OpenShell policy admission.
// A previous revision is retained only if it independently passes the same
// root-to-parent subset check, not merely because it was supplied as JSON.
func PlanTransition(chain []Policy, previous *Policy, report Report, mode string) TransitionPlan {
	out := TransitionPlan{
		SchemaVersion:        "agent-policy-transition.v1",
		RequestedFailureMode: mode,
		EffectiveFailureMode: "fail_closed",
		Decision:             "quarantine_recommended",
	}
	if len(chain) == 0 {
		return out
	}
	out.CandidateGeneration = chain[len(chain)-1].Generation
	previousValid := false
	if previous != nil {
		replacement := append([]Policy(nil), chain...)
		replacement[len(replacement)-1] = *previous
		previousValid = CheckLineage(replacement).Status == "within_boundary"
	}
	if mode == "retain_last_valid" && previousValid {
		out.EffectiveFailureMode = "retain_last_valid"
	}
	if report.Status != "within_boundary" {
		if out.EffectiveFailureMode == "retain_last_valid" {
			out.Decision = "retain_last_valid"
			out.PreviousPolicyRetained = true
			out.ActiveGeneration = previous.Generation
		}
		return out
	}
	if report.RequiresApproval {
		out.Decision = "human_review_required"
		if previousValid {
			out.ActiveGeneration = previous.Generation
			out.PreviousPolicyRetained = true
		}
		return out
	}
	out.Decision = "candidate_admissible"
	if previousValid {
		out.ActiveGeneration = previous.Generation
		out.PreviousPolicyRetained = true
	}
	return out
}
