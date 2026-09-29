package agentboundary

import (
	"sort"
)

const MaxObservations = 4096

// Observation is expected to originate from a separate trusted collector.
// The API that uses this engine is a simulation, not such a trusted collector.
type Observation struct {
	ID                 string `json:"id"`
	AgentID            string `json:"agentId"`
	TimestampMS        int64  `json:"timestampMs"`
	Sequence           uint64 `json:"sequence"`
	Kind               string `json:"kind"` // heartbeat, policy_denial, model_access, credential_use, delegate, generation, supervisor_disconnected
	Grant              *Grant `json:"grant,omitempty"`
	ChildID            string `json:"childId,omitempty"`
	ObservedGeneration uint64 `json:"observedGeneration,omitempty"`
}

type WatchdogRequest struct {
	AgentID            string        `json:"agentId"`
	NowMS              int64         `json:"nowMs"`
	HeartbeatTimeoutMS int64         `json:"heartbeatTimeoutMs"`
	DenialWindowMS     int64         `json:"denialWindowMs"`
	DenialThreshold    int           `json:"denialThreshold"`
	Observations       []Observation `json:"observations"`
}

type WatchdogFinding struct {
	Code    string `json:"code"`
	EventID string `json:"eventId,omitempty"`
}

type WatchdogReport struct {
	SchemaVersion                  string            `json:"schemaVersion"`
	Verdict                        string            `json:"verdict"` // observe | review | quarantine_recommended
	Findings                       []WatchdogFinding `json:"findings"`
	ObservedCount                  int               `json:"observedCount"`
	QuarantineRecommended          bool              `json:"quarantineRecommended"`
	EnforcementApplied             bool              `json:"enforcementApplied"`
	IndependentHardwareAttestation bool              `json:"independentHardwareAttestation"`
}

type SafetyCase struct {
	Lineage     []Policy        `json:"lineage"`
	Delegates   []Policy        `json:"delegates,omitempty"`
	Previous    *Policy         `json:"previous,omitempty"`
	FailureMode string          `json:"failureMode,omitempty"`
	Watchdog    WatchdogRequest `json:"watchdog"`
}

type SafetyReport struct {
	Boundary   Report         `json:"boundary"`
	Watchdog   WatchdogReport `json:"watchdog"`
	Transition TransitionPlan `json:"transition"`
	Applied    bool           `json:"applied"`
}

func finding(out *WatchdogReport, code, id string, quarantine bool) {
	if len(out.Findings) < 64 {
		out.Findings = append(out.Findings, WatchdogFinding{Code: code, EventID: id})
	}
	if quarantine {
		out.QuarantineRecommended = true
		out.Verdict = "quarantine_recommended"
	} else if out.Verdict == "observe" {
		out.Verdict = "review"
	}
}

// EvaluateWatchdog performs bounded deterministic evaluation of externally
// supplied metadata. Neither event authenticity nor hardware isolation is
// established by this function: a separate transport/attestation is required.
func EvaluateWatchdog(policy Policy, descendants []Policy, req WatchdogRequest) WatchdogReport {
	out := WatchdogReport{
		SchemaVersion: "agent-watchdog.v1",
		Verdict:       "observe",
		Findings:      make([]WatchdogFinding, 0),
	}
	if reason := validatePolicy(policy); reason != "" {
		finding(&out, "invalid_effective_policy", "", true)
		return out
	}
	if req.AgentID != policy.ID || req.NowMS <= 0 || req.HeartbeatTimeoutMS < 100 ||
		req.HeartbeatTimeoutMS > 600000 || req.DenialWindowMS < 100 ||
		req.DenialWindowMS > 600000 || req.DenialThreshold < 2 ||
		req.DenialThreshold > 100 || len(req.Observations) > MaxObservations {
		finding(&out, "unsupported_watchdog_request", "", true)
		return out
	}
	byID := make(map[string]Policy, len(descendants))
	for _, child := range descendants {
		if child.ParentID == policy.ID {
			byID[child.ID] = child
		}
	}
	observed := append([]Observation(nil), req.Observations...)
	sort.SliceStable(observed, func(i, j int) bool {
		if observed[i].Sequence != observed[j].Sequence {
			return observed[i].Sequence < observed[j].Sequence
		}
		return observed[i].TimestampMS < observed[j].TimestampMS
	})
	seen := make(map[string]struct{}, len(observed))
	lastHeartbeat := int64(0)
	lastSequence := uint64(0)
	lastTimestamp := int64(0)
	denials := make([]int64, 0, req.DenialThreshold)
	for _, event := range observed {
		if event.ID == "" || len(event.ID) > 128 || event.AgentID != req.AgentID ||
			event.TimestampMS <= 0 || event.TimestampMS > req.NowMS || event.Sequence == 0 ||
			event.Sequence <= lastSequence || event.TimestampMS < lastTimestamp {
			finding(&out, "invalid_or_replayed_telemetry", event.ID, true)
			continue
		}
		if _, exists := seen[event.ID]; exists {
			finding(&out, "duplicate_event_id", event.ID, true)
			continue
		}
		seen[event.ID] = struct{}{}
		lastSequence, lastTimestamp = event.Sequence, event.TimestampMS
		out.ObservedCount++
		if policy.ExpiresAtMS != 0 && event.TimestampMS >= policy.ExpiresAtMS {
			finding(&out, "expired_authority", event.ID, true)
		}
		switch event.Kind {
		case "heartbeat":
			if event.Grant != nil || event.ChildID != "" {
				finding(&out, "ambiguous_heartbeat", event.ID, true)
			}
			lastHeartbeat = event.TimestampMS
		case "policy_denial":
			denials = append(denials, event.TimestampMS)
			cutoff := event.TimestampMS - req.DenialWindowMS
			n := 0
			for _, ts := range denials {
				if ts >= cutoff {
					denials[n] = ts
					n++
				}
			}
			denials = denials[:n]
			if len(denials) == req.DenialThreshold {
				finding(&out, "repeated_policy_denials", event.ID, false)
			}
		case "model_access":
			if event.Grant == nil || event.Grant.Domain != "model" || !HasGrant(policy, *event.Grant) {
				finding(&out, "unauthorized_model_access", event.ID, true)
			}
		case "credential_use":
			if event.Grant == nil || event.Grant.Domain != "credential" || !HasGrant(policy, *event.Grant) {
				finding(&out, "credential_scope_violation", event.ID, true)
			}
		case "delegate":
			child, ok := byID[event.ChildID]
			if !ok || CheckDirectDelegation(policy, child).Status != "within_boundary" {
				finding(&out, "unauthorized_subagent_delegation", event.ID, true)
			}
		case "generation":
			if event.ObservedGeneration != policy.Generation {
				finding(&out, "policy_generation_mismatch", event.ID, true)
			}
		case "supervisor_disconnected":
			finding(&out, "lost_control_plane", event.ID, true)
		default:
			finding(&out, "unknown_signal_kind", event.ID, true)
		}
	}
	if lastHeartbeat == 0 || req.NowMS-lastHeartbeat > req.HeartbeatTimeoutMS {
		finding(&out, "heartbeat_timeout_or_unobserved", "", true)
	}
	if policy.ExpiresAtMS != 0 && req.NowMS >= policy.ExpiresAtMS {
		finding(&out, "expired_authority", "", true)
	}
	return out
}

// EvaluateCase is explicitly read-only; a production enforcement adapter must
// live in an independent trust domain and obtain authenticated approval.
func EvaluateCase(input SafetyCase) SafetyReport {
	b := CheckUpdate(input.Lineage, input.Previous)
	plan := PlanTransition(input.Lineage, input.Previous, b, input.FailureMode)
	if len(input.Lineage) == 0 {
		return SafetyReport{Boundary: b, Transition: plan, Watchdog: WatchdogReport{
			SchemaVersion: "agent-watchdog.v1", Verdict: "quarantine_recommended",
			QuarantineRecommended: true, Findings: []WatchdogFinding{{Code: "missing_effective_policy"}},
		}}
	}
	p := input.Lineage[len(input.Lineage)-1]
	w := EvaluateWatchdog(p, input.Delegates, input.Watchdog)
	if b.Status != "within_boundary" {
		finding(&w, "boundary_not_verified", "", true)
	}
	if b.RequiresApproval {
		finding(&w, "policy_change_requires_approval", "", false)
	}
	return SafetyReport{Boundary: b, Watchdog: w, Transition: plan}
}
