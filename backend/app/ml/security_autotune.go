package ml

import (
	"math"
	"strings"
)

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
	case "catastrophicmissrate", "catastrophic_miss_rate", "catastrophicmiss", "catastrophic_miss":
		return "catastrophicMissRate"
	case "benignfalsepositiverate", "benign_false_positive_rate", "benignfpr", "benign_fpr":
		return "benignFalsePositiveRate"
	case "threatvectorcoverage", "threat_vector_coverage", "vectorcoverage", "vector_coverage":
		return "threatVectorCoverage"
	default:
		return ""
	}
}

// SecurityAutoTuneMetricComparable reports whether a validation slice actually
// contains the positive population required by a security-specific objective.
// Explicit vector objectives never silently fall back to generic accuracy.
func SecurityAutoTuneMetricComparable(metric string, attack AttackImpactMetrics) bool {
	switch metric {
	case "attackRecall", "riskWeightedRecall":
		return attack.AttackSamples > 0
	case "highImpactRecall", "catastrophicMissRate":
		return attack.HighImpactSamples > 0
	case "intrusionRecall":
		return attack.IntrusionSamples > 0
	case "destructionRecall":
		return attack.DestructionSamples > 0
	case "exfiltrationRecall":
		return attack.ExfiltrationSamples > 0
	case "persistenceRecall":
		return attack.PersistenceSamples > 0
	case "benignFalsePositiveRate":
		return attack.BenignSamples > 0
	case "securityUtility", "threatVectorCoverage":
		return attack.ScoredSamples > 0
	default:
		return true
	}
}

// SecurityAutoTuneMetricScore returns a higher-is-better ranking score for every
// supported objective. Rate objectives that are naturally lower-is-better are
// inverted here so all callers can keep one maximization path.
func SecurityAutoTuneMetricScore(metric string, validationAccuracy, throughput float64, classification AutoTuneClassificationMetrics, attack AttackImpactMetrics) float64 {
	if !SecurityAutoTuneMetricComparable(metric, attack) {
		return math.Inf(-1)
	}

	switch metric {
	case "attackRecall":
		return attack.AttackRecall
	case "highImpactRecall":
		return attack.HighImpactRecall
	case "intrusionRecall":
		return attack.IntrusionRecall
	case "destructionRecall":
		return attack.DestructionRecall
	case "exfiltrationRecall":
		return attack.ExfiltrationRecall
	case "persistenceRecall":
		return attack.PersistenceRecall
	case "riskWeightedRecall":
		return attack.RiskWeightedRecall
	case "securityUtility":
		return attack.SecurityUtility
	case "catastrophicMissRate":
		return 1.0 - attack.CatastrophicMissRate
	case "benignFalsePositiveRate":
		return 1.0 - attack.BenignFalsePositive
	case "threatVectorCoverage":
		return attack.ThreatVectorCoverage
	default:
		return AutoTuneMetricScore(metric, validationAccuracy, throughput, classification)
	}
}

// SecurityAutoTuneMetricValue returns the human-facing raw metric value. This
// differs from the ranking score for lower-is-better objectives such as miss/FPR.
func SecurityAutoTuneMetricValue(metric string, validationAccuracy, throughput float64, classification AutoTuneClassificationMetrics, attack AttackImpactMetrics) float64 {
	switch metric {
	case "attackRecall":
		return attack.AttackRecall
	case "highImpactRecall":
		return attack.HighImpactRecall
	case "intrusionRecall":
		return attack.IntrusionRecall
	case "destructionRecall":
		return attack.DestructionRecall
	case "exfiltrationRecall":
		return attack.ExfiltrationRecall
	case "persistenceRecall":
		return attack.PersistenceRecall
	case "riskWeightedRecall":
		return attack.RiskWeightedRecall
	case "securityUtility":
		return attack.SecurityUtility
	case "catastrophicMissRate":
		return attack.CatastrophicMissRate
	case "benignFalsePositiveRate":
		return attack.BenignFalsePositive
	case "threatVectorCoverage":
		return attack.ThreatVectorCoverage
	default:
		return AutoTuneMetricScore(metric, validationAccuracy, throughput, classification)
	}
}
