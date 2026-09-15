package ml

import "strings"

// NormalizeSecurityAutoTuneMetric extends the legacy accuracy/speed objectives
// with attack-impact objectives. Existing metric names remain valid.
func NormalizeSecurityAutoTuneMetric(metric string) string {
	if normalized := NormalizeAutoTuneMetric(metric); normalized != "" {
		return normalized
	}
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "attackrecall", "attack_recall", "threatrecall", "threat_recall":
		return "attackRecall"
	case "highimpactrecall", "high_impact_recall", "criticalrecall", "critical_recall":
		return "highImpactRecall"
	case "intrusionrecall", "intrusion_recall":
		return "intrusionRecall"
	case "destructionrecall", "destruction_recall", "destructiverecall", "destructive_recall":
		return "destructionRecall"
	case "exfiltrationrecall", "exfiltration_recall", "exfilrecall", "exfil_recall":
		return "exfiltrationRecall"
	case "persistencerecall", "persistence_recall":
		return "persistenceRecall"
	case "riskweightedrecall", "risk_weighted_recall", "impactrecall", "impact_recall":
		return "riskWeightedRecall"
	case "securityutility", "security_utility", "securityscore", "security_score", "impactutility", "impact_utility":
		return "securityUtility"
	default:
		return ""
	}
}

// SecurityAutoTuneMetricScore returns a higher-is-better score for every
// supported objective. Sparse validation sets fall back to broader metrics so a
// missing attack vector does not make model selection arbitrary.
func SecurityAutoTuneMetricScore(metric string, validationAccuracy, throughput float64, classification AutoTuneClassificationMetrics, attack AttackImpactMetrics) float64 {
	switch metric {
	case "attackRecall":
		if attack.AttackSamples > 0 {
			return attack.AttackRecall
		}
	case "highImpactRecall":
		if attack.HighImpactSamples > 0 {
			return attack.HighImpactRecall
		}
		if attack.AttackSamples > 0 {
			return attack.AttackRecall
		}
	case "intrusionRecall":
		if attack.IntrusionSamples > 0 {
			return attack.IntrusionRecall
		}
	case "destructionRecall":
		if attack.DestructionSamples > 0 {
			return attack.DestructionRecall
		}
	case "exfiltrationRecall":
		if attack.ExfiltrationSamples > 0 {
			return attack.ExfiltrationRecall
		}
	case "persistenceRecall":
		if attack.PersistenceSamples > 0 {
			return attack.PersistenceRecall
		}
	case "riskWeightedRecall":
		if attack.AttackSamples > 0 {
			return attack.RiskWeightedRecall
		}
	case "securityUtility":
		if attack.AttackSamples > 0 || attack.BenignFalsePositive > 0 {
			return attack.SecurityUtility
		}
		// A fully benign validation slice can still rank models by how well they
		// preserve legitimate behavior.
		if classification.AllowRecall > 0 {
			return classification.AllowRecall
		}
	}
	return AutoTuneMetricScore(metric, validationAccuracy, throughput, classification)
}
