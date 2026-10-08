package app

import (
	"math"
	"strings"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/pb"
)

// AttackVectorScores separates attacker intent/impact from the global score.
// The global score is derived from the strongest vectors plus correlated
// secondary vectors; it is intentionally not a simple sum.
type AttackVectorScores struct {
	Global       float64 `json:"global"`
	Class        string  `json:"class"`
	Intrusion    float64 `json:"intrusion"`
	Destruction  float64 `json:"destruction"`
	Exfiltration float64 `json:"exfiltration"`
	Persistence  float64 `json:"persistence"`
}

// computeRiskScore keeps the existing 0-100, higher-is-riskier API while using
// host-centric attack vectors internally.
func computeRiskScore(classification *pb.BehaviorClassification, anomalyScore float64, mlPrediction ml.Prediction, netAudit NetworkAuditResult, llmAssessment *llmAssessment) float64 {
	return computeAttackVectorScores(classification, anomalyScore, mlPrediction, netAudit, llmAssessment).Global
}

func computeAttackVectorScores(classification *pb.BehaviorClassification, anomalyScore float64, mlPrediction ml.Prediction, netAudit NetworkAuditResult, llmAssessment *llmAssessment) AttackVectorScores {
	var out AttackVectorScores
	anomaly := clampFloat64(anomalyScore, 0, 1)
	confidenceBonus := 0.0

	if classification != nil {
		switch strings.ToUpper(strings.TrimSpace(classification.PrimaryCategory)) {
		case "SENSITIVE":
			out.Intrusion = 68
			out.Persistence = 24
		case "FILE_DELETE":
			out.Destruction = 78
		case "PROCESS_KILL":
			out.Destruction = 70
		case "FILE_PERMISSION":
			out.Intrusion = 52
			out.Persistence = 34
		case "PROCESS_EXEC":
			out.Intrusion = 30
			out.Persistence = 40
		case "FILE_WRITE":
			out.Destruction = 34
			out.Persistence = 26
		case "CONTAINER":
			out.Intrusion = 24
			out.Persistence = 42
		case "PACKAGE_MANAGER":
			out.Persistence = 30
		// NETWORK, DATABASE and COMPRESSION are context, not attack vectors by
		// themselves. Concrete network-audit findings or other evidence below
		// must establish intrusion/exfiltration/persistence intent.
		}

		switch classification.Confidence {
		case "high":
			confidenceBonus = 8
		case "medium":
			confidenceBonus = 4
		}
	}

	// Deterministic network findings act like strong managed-rule evidence.
	// They lift the relevant attack vector instead of blindly adding to every
	// kind of risk.
	if netAudit.Flags.ReverseShell {
		out.Intrusion = math.Max(out.Intrusion, 96)
		out.Persistence = math.Max(out.Persistence, 68)
	}
	if netAudit.Flags.DataExfil {
		out.Exfiltration = math.Max(out.Exfiltration, 98)
	}
	if netAudit.Flags.DNSTunnel {
		out.Exfiltration = math.Max(out.Exfiltration, 86)
		out.Persistence = math.Max(out.Persistence, 58)
	}
	if netAudit.Flags.PortScan {
		out.Intrusion = math.Max(out.Intrusion, 72)
	}
	if netAudit.Flags.FirewallManip {
		out.Intrusion = math.Max(out.Intrusion, 80)
		out.Destruction = math.Max(out.Destruction, 66)
		out.Persistence = math.Max(out.Persistence, 56)
	}
	if netAudit.Flags.SuspiciousPort {
		out.Intrusion = math.Max(out.Intrusion, 58)
		out.Persistence = math.Max(out.Persistence, 48)
	}
	if netAudit.Flags.ClearTextProto {
		out.Exfiltration = math.Max(out.Exfiltration, 28)
	}
	if netAudit.Flags.UnusualTarget {
		out.Exfiltration = math.Max(out.Exfiltration, 44)
	}

	// Anomaly evidence strengthens already identified vectors. If no vector is
	// known yet, it remains a global prior rather than inventing an attack type.
	vectorAnomaly := anomaly * 24
	out.Intrusion = addIfActive(out.Intrusion, vectorAnomaly+confidenceBonus)
	out.Destruction = addIfActive(out.Destruction, vectorAnomaly+confidenceBonus)
	out.Exfiltration = addIfActive(out.Exfiltration, vectorAnomaly+confidenceBonus+clampFloat64(netAudit.RiskScore*0.16, 0, 16))
	out.Persistence = addIfActive(out.Persistence, vectorAnomaly+confidenceBonus)

	modelPrior := 0.0
	if mlPrediction.Confidence >= 0.60 {
		switch mlPrediction.Action {
		case 1: // BLOCK
			modelPrior = 30 * clampFloat64(mlPrediction.Confidence, 0, 1)
		case 3: // ALERT
			modelPrior = 20 * clampFloat64(mlPrediction.Confidence, 0, 1)
		case 2: // REWRITE
			modelPrior = 9 * clampFloat64(mlPrediction.Confidence, 0, 1)
		}
	}

	llmPrior := 0.0
	if llmAssessment != nil && strings.TrimSpace(llmAssessment.Error) == "" {
		llmPrior = clampFloat64(llmAssessment.RiskScore*0.22, 0, 22)
		if llmAssessment.Confidence > 0 {
			llmPrior += clampFloat64(llmAssessment.Confidence*5, 0, 5)
		}
		switch llmAssessment.RecommendedAction {
		case "BLOCK":
			llmPrior += 8
		case "ALERT":
			llmPrior += 5
		case "REWRITE":
			llmPrior += 2
		}
	}

	out.Intrusion = clampFloat64(out.Intrusion, 0, 100)
	out.Destruction = clampFloat64(out.Destruction, 0, 100)
	out.Exfiltration = clampFloat64(out.Exfiltration, 0, 100)
	out.Persistence = clampFloat64(out.Persistence, 0, 100)

	generalPrior := clampFloat64(anomaly*24+modelPrior+llmPrior, 0, 100)
	first, second, third := topThreeScores(out.Intrusion, out.Destruction, out.Exfiltration, out.Persistence)
	global := math.Max(first, generalPrior)
	if first > 0 {
		// Correlated secondary vectors raise confidence that this is an attack,
		// but cannot dominate the strongest vector on their own.
		global += second*0.12 + third*0.05
	}
	out.Global = math.Round(clampFloat64(global, 0, 100))
	out.Class = attackRiskClass(out.Global)
	return out
}

func addIfActive(score, delta float64) float64 {
	if score <= 0 {
		return 0
	}
	return score + delta
}

func topThreeScores(values ...float64) (float64, float64, float64) {
	first, second, third := 0.0, 0.0, 0.0
	for _, value := range values {
		switch {
		case value >= first:
			third = second
			second = first
			first = value
		case value >= second:
			third = second
			second = value
		case value > third:
			third = value
		}
	}
	return first, second, third
}

func attackRiskClass(score float64) string {
	switch {
	case score >= 80:
		return "ATTACK"
	case score >= 60:
		return "LIKELY_ATTACK"
	case score >= 40:
		return "SUSPICIOUS"
	case score >= 20:
		return "LIKELY_CLEAN"
	default:
		return "CLEAN"
	}
}

// riskLevel maps a risk score to a severity level.
func riskLevel(score float64) string {
	switch {
	case score >= 80:
		return "CRITICAL"
	case score >= 60:
		return "HIGH"
	case score >= 40:
		return "MEDIUM"
	case score >= 20:
		return "LOW"
	default:
		return "SAFE"
	}
}
