package ml

import "math"

const SecurityMetricConfidenceVersion = "wilson-95-v1"

// SecurityEvaluationPolicy documents the asymmetric costs and confidence
// treatment used by attack-impact AutoML. Keeping this policy explicit makes
// model ranking explainable and lets callers record the exact evaluation
// semantics alongside experiment results.
type SecurityEvaluationPolicy struct {
	Version                 string  `json:"version"`
	ConfidenceMethod        string  `json:"confidenceMethod"`
	ConfidenceZ             float64 `json:"confidenceZ"`
	BenignBlockCost         float64 `json:"benignBlockCost"`
	BenignAlertCost         float64 `json:"benignAlertCost"`
	BenignRewriteCost       float64 `json:"benignRewriteCost"`
	BlockDetectionCredit    float64 `json:"blockDetectionCredit"`
	AlertDetectionCredit    float64 `json:"alertDetectionCredit"`
	RewriteDetectionCredit  float64 `json:"rewriteDetectionCredit"`
	CatastrophicMissPenalty float64 `json:"catastrophicMissPenalty"`
}

func DefaultSecurityEvaluationPolicy() SecurityEvaluationPolicy {
	return SecurityEvaluationPolicy{
		Version:                 "attack-impact-v2",
		ConfidenceMethod:        SecurityMetricConfidenceVersion,
		ConfidenceZ:             1.96,
		BenignBlockCost:         2.5,
		BenignAlertCost:         1.25,
		BenignRewriteCost:       0.5,
		BlockDetectionCredit:    1.0,
		AlertDetectionCredit:    0.88,
		RewriteDetectionCredit:  0.55,
		CatastrophicMissPenalty: 0.65,
	}
}

// SecurityMetricEvidence keeps the user-facing observed value separate from
// the conservative score used for AutoML ranking. For binomial recall/error
// rates the conservative score is a Wilson confidence bound, so tiny validation
// slices cannot win solely because they happened to score 1/1.
type SecurityMetricEvidence struct {
	Metric            string  `json:"metric"`
	Support           int     `json:"support"`
	ObservedValue     float64 `json:"observedValue"`
	ConservativeScore float64 `json:"conservativeScore"`
	ConfidenceMethod  string  `json:"confidenceMethod"`
	Comparable        bool    `json:"comparable"`
}

func WilsonLowerBound(rate float64, n int, z float64) float64 {
	if n <= 0 {
		return 0
	}
	if z <= 0 {
		z = 1.96
	}
	p := clampUnit(rate)
	denom := 1 + z*z/float64(n)
	center := p + z*z/(2*float64(n))
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*float64(n)))/float64(n))
	return clampUnit((center - margin) / denom)
}

func WilsonUpperBound(rate float64, n int, z float64) float64 {
	if n <= 0 {
		return 1
	}
	if z <= 0 {
		z = 1.96
	}
	p := clampUnit(rate)
	denom := 1 + z*z/float64(n)
	center := p + z*z/(2*float64(n))
	margin := z * math.Sqrt((p*(1-p)+z*z/(4*float64(n)))/float64(n))
	return clampUnit((center + margin) / denom)
}

// SecurityAutoTuneMetricEvidence returns a statistically conservative ranking
// score for rate metrics while preserving the raw value for display. Weighted
// utility metrics are not Bernoulli observations, so they remain untransformed.
func SecurityAutoTuneMetricEvidence(metric string, attack AttackImpactMetrics) SecurityMetricEvidence {
	policy := DefaultSecurityEvaluationPolicy()
	metric = NormalizeSecurityAutoTuneMetric(metric)
	evidence := SecurityMetricEvidence{
		Metric:           metric,
		ConfidenceMethod: "none",
		Comparable:       SecurityAutoTuneMetricComparable(metric, attack),
	}

	setRecall := func(value float64, support int) SecurityMetricEvidence {
		evidence.Support = support
		evidence.ObservedValue = value
		evidence.ConservativeScore = WilsonLowerBound(value, support, policy.ConfidenceZ)
		evidence.ConfidenceMethod = SecurityMetricConfidenceVersion
		return evidence
	}
	setInverseError := func(value float64, support int) SecurityMetricEvidence {
		evidence.Support = support
		evidence.ObservedValue = value
		evidence.ConservativeScore = 1 - WilsonUpperBound(value, support, policy.ConfidenceZ)
		evidence.ConservativeScore = clampUnit(evidence.ConservativeScore)
		evidence.ConfidenceMethod = SecurityMetricConfidenceVersion
		return evidence
	}

	switch metric {
	case "attackRecall":
		return setRecall(attack.AttackRecall, attack.AttackSamples)
	case "highImpactRecall":
		return setRecall(attack.HighImpactRecall, attack.HighImpactSamples)
	case "intrusionRecall":
		return setRecall(attack.IntrusionRecall, attack.IntrusionSamples)
	case "destructionRecall":
		return setRecall(attack.DestructionRecall, attack.DestructionSamples)
	case "exfiltrationRecall":
		return setRecall(attack.ExfiltrationRecall, attack.ExfiltrationSamples)
	case "persistenceRecall":
		return setRecall(attack.PersistenceRecall, attack.PersistenceSamples)
	case "catastrophicMissRate":
		return setInverseError(attack.CatastrophicMissRate, attack.HighImpactSamples)
	case "benignFalsePositiveRate":
		return setInverseError(attack.BenignFalsePositive, attack.BenignSamples)
	case "riskWeightedRecall":
		evidence.Support = attack.AttackSamples
		evidence.ObservedValue = attack.RiskWeightedRecall
		evidence.ConservativeScore = attack.RiskWeightedRecall
		evidence.ConfidenceMethod = "weighted-impact"
	case "securityUtility":
		evidence.Support = attack.ScoredSamples
		evidence.ObservedValue = attack.SecurityUtility
		evidence.ConservativeScore = attack.SecurityUtility
		evidence.ConfidenceMethod = "asymmetric-cost"
	case "threatVectorCoverage":
		evidence.Support = attack.IntrusionSamples + attack.DestructionSamples + attack.ExfiltrationSamples + attack.PersistenceSamples
		evidence.ObservedValue = attack.ThreatVectorCoverage
		evidence.ConservativeScore = attack.ThreatVectorCoverage
		evidence.ConfidenceMethod = "coverage"
	default:
		evidence.Support = attack.ScoredSamples
	}
	return evidence
}
