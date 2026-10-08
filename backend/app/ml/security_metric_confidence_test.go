package ml

import "testing"

func TestWilsonLowerBoundPrefersWellSupportedRecall(t *testing.T) {
	oneOfOne := WilsonLowerBound(1.0, 1, 1.96)
	ninetyFiveOfHundred := WilsonLowerBound(0.95, 100, 1.96)
	if oneOfOne >= ninetyFiveOfHundred {
		t.Fatalf("1/1 lower bound=%v must not outrank 95/100 lower bound=%v", oneOfOne, ninetyFiveOfHundred)
	}
	if oneOfOne <= 0 || oneOfOne >= 0.5 {
		t.Fatalf("unexpected 1/1 Wilson lower bound: %v", oneOfOne)
	}
}

func TestSecurityMetricEvidenceUsesConservativeRecall(t *testing.T) {
	sparse := AttackImpactMetrics{
		ScoredSamples:      10,
		AttackSamples:      1,
		DestructionSamples: 1,
		DestructionRecall:  1,
	}
	wellSupported := AttackImpactMetrics{
		ScoredSamples:      200,
		AttackSamples:      100,
		DestructionSamples: 100,
		DestructionRecall:  0.95,
	}

	sparseScore := SecurityAutoTuneMetricScore("destructionRecall", 1, 1, AutoTuneClassificationMetrics{}, sparse)
	wellSupportedScore := SecurityAutoTuneMetricScore("destructionRecall", 0.95, 1, AutoTuneClassificationMetrics{}, wellSupported)
	if sparseScore >= wellSupportedScore {
		t.Fatalf("confidence-aware score should prefer 95/100 (%v) over 1/1 (%v)", wellSupportedScore, sparseScore)
	}

	evidence := SecurityAutoTuneMetricEvidence("destructionRecall", sparse)
	if evidence.ObservedValue != 1 || evidence.Support != 1 {
		t.Fatalf("unexpected evidence payload: %+v", evidence)
	}
	if evidence.ConservativeScore >= evidence.ObservedValue {
		t.Fatalf("conservative score must be below raw 1/1 recall: %+v", evidence)
	}
}

func TestSecurityMetricEvidenceUsesUpperBoundForErrorRates(t *testing.T) {
	attack := AttackImpactMetrics{
		ScoredSamples:        20,
		BenignSamples:        10,
		HighImpactSamples:    10,
		CatastrophicMissRate: 0,
		BenignFalsePositive:  0,
	}

	catastrophic := SecurityAutoTuneMetricEvidence("catastrophicMissRate", attack)
	if catastrophic.ObservedValue != 0 {
		t.Fatalf("unexpected observed catastrophic miss rate: %+v", catastrophic)
	}
	if catastrophic.ConservativeScore >= 1 {
		t.Fatalf("zero misses on only ten samples must retain uncertainty: %+v", catastrophic)
	}

	benign := SecurityAutoTuneMetricEvidence("benignFalsePositiveRate", attack)
	if benign.ConservativeScore >= 1 {
		t.Fatalf("zero benign false positives on only ten samples must retain uncertainty: %+v", benign)
	}
}

func TestDefaultSecurityEvaluationPolicyDocumentsRuntimeSemantics(t *testing.T) {
	policy := DefaultSecurityEvaluationPolicy()
	if policy.ConfidenceZ != 1.96 || policy.ConfidenceMethod != SecurityMetricConfidenceVersion {
		t.Fatalf("unexpected confidence policy: %+v", policy)
	}
	if policy.BenignBlockCost <= policy.BenignAlertCost || policy.BenignAlertCost <= policy.BenignRewriteCost {
		t.Fatalf("benign disruption costs must be ordered BLOCK > ALERT > REWRITE: %+v", policy)
	}
	if policy.BlockDetectionCredit <= policy.AlertDetectionCredit || policy.AlertDetectionCredit <= policy.RewriteDetectionCredit {
		t.Fatalf("attack containment credits must be ordered BLOCK > ALERT > REWRITE: %+v", policy)
	}
	if policy.CatastrophicMissPenalty <= 0 || policy.CatastrophicMissPenalty >= 1 {
		t.Fatalf("catastrophic miss penalty must be a bounded multiplier: %+v", policy)
	}
}
