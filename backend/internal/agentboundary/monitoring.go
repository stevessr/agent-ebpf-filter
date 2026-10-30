package agentboundary

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// MonitoringSummary is a policy-aware L0 triage result, not an LLM verdict.
// This research endpoint has no trustworthy independent telemetry or model
// inference; therefore it never emits P0 or applies enforcement.
type MonitoringSummary struct {
	SchemaVersion                  string   `json:"schemaVersion"`
	Layer0                         string   `json:"layer0"` // flagged | no_signal
	HighestPriority                string   `json:"highestPriority"` // P1 | P2 | none
	Reasons                        []string `json:"reasons,omitempty"`
	Layer1ReviewRequired           bool     `json:"layer1ReviewRequired"`
	Layer1Status                   string   `json:"layer1Status"` // pending_not_run | not_requested
	Layer2InvestigationRequired    bool     `json:"layer2InvestigationRequired"`
	IndependentEvidenceRequired    bool     `json:"independentEvidenceRequired"`
	EvidenceVerified               bool     `json:"evidenceVerified"`
	EnforcementApplied             bool     `json:"enforcementApplied"`
}

// SummarizeMisalignment escalates hard policy and monitor integrity signals
// above low-confidence suspicious behavior. Priorities are triage suggestions,
// never calibrated attack probabilities or confirmation of sandbox escape.
func SummarizeMisalignment(w WatchdogReport) MonitoringSummary {
	out := MonitoringSummary{
		SchemaVersion: "agent-misalignment-screen.v1",
		Layer0: "no_signal", HighestPriority: "none",
		Layer1Status: "not_requested",
	}
	if len(w.Findings) == 0 && !w.QuarantineRecommended {
		return out
	}
	out.Layer0 = "flagged"
	out.HighestPriority = "P2"
	out.Layer1ReviewRequired = true
	out.Layer1Status = "pending_not_run"
	for _, item := range w.Findings {
		if len(out.Reasons) < 64 {
			out.Reasons = append(out.Reasons, item.Code)
		}
		if isHighPriorityMisalignment(item.Code) {
			out.HighestPriority = "P1"
		}
	}
	if w.QuarantineRecommended {
		out.HighestPriority = "P1"
	}
	out.Layer2InvestigationRequired = out.HighestPriority == "P1"
	out.IndependentEvidenceRequired = out.Layer2InvestigationRequired
	return out
}

func isHighPriorityMisalignment(code string) bool {
	switch code {
	case "unapproved_network_attempt", "dns_egress_scope_unverified",
		"repeated_policy_denials", "evaluation_artifact_review",
		"policy_change_requires_approval":
		return false
	default:
		return true
	}
}

// ShouldAuditUnflagged offers a keyed, stable ~5% sample of otherwise
// unflagged events for review by a *separate* production scheduler.
// The secret must be random, private, at least 32 bytes and outside agent
// control. No key means no sampling: never silently use a predictable seed.
// The research endpoint does not claim that this scheduler is deployed.
func ShouldAuditUnflagged(secret []byte, runID, eventID string) bool {
	if len(secret) < 32 || len(runID) == 0 || len(runID) > 128 ||
		len(eventID) == 0 || len(eventID) > 128 {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(runID))
	mac.Write([]byte{0})
	mac.Write([]byte(eventID))
	sum := mac.Sum(nil)
	// 2^32 * .05 rounds to 214748365.
	return binary.BigEndian.Uint32(sum[:4]) < 214748365
}
