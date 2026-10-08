package ml

import (
	"math"
	"testing"
)

func gridSecuritySample(id int, label int32, categoryIndex int, networkFlags ...int) TrainSample {
	var features [FeatureDim]float64
	if categoryIndex >= 0 && categoryIndex < len(featureCategoryNames) {
		features[categoryIndex] = 1
	}
	features[31] = float64(id)
	for _, index := range networkFlags {
		if index >= 0 && index < FeatureDim {
			features[index] = 1
		}
	}
	return TrainSample{features: features, label: label}
}

func TestEvaluateAttackImpactTrainSamplesRecoversFeatureThreatVectors(t *testing.T) {
	samples := []TrainSample{
		gridSecuritySample(1, 1, 3),       // FILE_DELETE -> destruction
		gridSecuritySample(2, 1, 5, 122),  // NETWORK + reverse shell -> intrusion+persistence
		gridSecuritySample(3, 3, 5, 123),  // NETWORK + explicit exfil -> exfiltration
		gridSecuritySample(4, 0, 5),       // benign generic NETWORK remains vector-neutral
	}
	predictions := map[int]int32{
		1: 0, // catastrophic destructive miss
		2: 1,
		3: 3,
		4: 0,
	}
	metrics := EvaluateAttackImpactTrainSamples(samples, func(features [FeatureDim]float64) int32 {
		return predictions[int(features[31])]
	})

	if metrics.ScoredSamples != 4 || metrics.BenignSamples != 1 || metrics.AttackSamples != 3 {
		t.Fatalf("unexpected sample accounting: %+v", metrics)
	}
	if metrics.DestructionSamples != 1 || metrics.DestructionRecall != 0 {
		t.Fatalf("destruction miss not represented: %+v", metrics)
	}
	if metrics.IntrusionSamples != 1 || metrics.IntrusionRecall != 1 {
		t.Fatalf("reverse shell intrusion vector not recovered: %+v", metrics)
	}
	if metrics.PersistenceSamples != 1 || metrics.PersistenceRecall != 1 {
		t.Fatalf("reverse shell persistence vector not recovered: %+v", metrics)
	}
	if metrics.ExfiltrationSamples != 1 || metrics.ExfiltrationRecall != 1 {
		t.Fatalf("exfiltration vector not recovered: %+v", metrics)
	}
	if metrics.CatastrophicMissRate <= 0 {
		t.Fatalf("destructive ALLOW must be visible as catastrophic miss: %+v", metrics)
	}
}

func TestGenericNetworkFeatureDoesNotInventExfiltration(t *testing.T) {
	sample := gridSecuritySample(1, 1, 5)
	vectors := threatVectorsFromFeatures(sample.features)
	for _, vector := range vectors {
		if vector == ThreatExfiltration {
			t.Fatalf("generic NETWORK must not imply exfiltration: %v", vectors)
		}
	}
}

func TestSecurityObjectiveWithoutCoverageDoesNotFallbackToAccuracy(t *testing.T) {
	classification := AutoTuneClassificationMetrics{
		Accuracy:         1,
		AllowRecall:      1,
		BalancedAccuracy: 1,
	}
	attack := AttackImpactMetrics{
		ScoredSamples:      10,
		AttackSamples:      3,
		DestructionSamples: 0,
	}
	if SecurityAutoTuneMetricComparable("destructionRecall", attack) {
		t.Fatal("destructionRecall must be incomparable without destructive validation samples")
	}
	score := SecurityAutoTuneMetricScore("destructionRecall", 1, 1000, classification, attack)
	if !math.IsInf(score, -1) {
		t.Fatalf("missing objective coverage must not fall back to accuracy, got %v", score)
	}
}

func TestCatastrophicMissRateObjectiveUsesLowerIsBetterRanking(t *testing.T) {
	classification := AutoTuneClassificationMetrics{}
	safer := AttackImpactMetrics{ScoredSamples: 10, HighImpactSamples: 5, CatastrophicMissRate: 0.1}
	riskier := AttackImpactMetrics{ScoredSamples: 10, HighImpactSamples: 5, CatastrophicMissRate: 0.4}

	saferScore := SecurityAutoTuneMetricScore("catastrophicMissRate", 0, 0, classification, safer)
	riskierScore := SecurityAutoTuneMetricScore("catastrophicMissRate", 0, 0, classification, riskier)
	if saferScore <= riskierScore {
		t.Fatalf("lower catastrophic miss rate must rank higher: safer=%v riskier=%v", saferScore, riskierScore)
	}
	if SecurityAutoTuneMetricValue("catastrophicMissRate", 0, 0, classification, safer) != 0.1 {
		t.Fatal("human-facing catastrophic metric must preserve raw miss rate")
	}
}

func TestBenignActionCostsPreferRewriteOverAlertOverBlock(t *testing.T) {
	sample := gridSecuritySample(1, 0, 12) // benign DEVELOPMENT
	utility := func(pred int32) float64 {
		return EvaluateAttackImpactTrainSamples([]TrainSample{sample}, func([FeatureDim]float64) int32 {
			return pred
		}).SecurityUtility
	}
	allow := utility(0)
	rewrite := utility(2)
	alert := utility(3)
	block := utility(1)
	if !(allow > rewrite && rewrite > alert && alert > block) {
		t.Fatalf("expected ALLOW > REWRITE > ALERT > BLOCK utility, got %.3f %.3f %.3f %.3f", allow, rewrite, alert, block)
	}
}

func TestRewriteGetsPartialAttackCreditButNotHighImpactRecall(t *testing.T) {
	sample := gridSecuritySample(1, 1, 3) // high-impact FILE_DELETE
	metrics := EvaluateAttackImpactTrainSamples([]TrainSample{sample}, func([FeatureDim]float64) int32 {
		return 2 // REWRITE
	})
	if metrics.AttackRecall != 1 {
		t.Fatalf("rewrite should identify intervention-required attack, got %+v", metrics)
	}
	if metrics.HighImpactRecall != 0 {
		t.Fatalf("rewrite must not receive full high-impact containment credit: %+v", metrics)
	}
	if metrics.RiskWeightedRecall <= 0 || metrics.RiskWeightedRecall >= 1 {
		t.Fatalf("rewrite should receive partial risk-weighted credit: %+v", metrics)
	}
	if metrics.CatastrophicMissRate != 0 {
		t.Fatalf("rewrite is weak containment, not catastrophic ALLOW: %+v", metrics)
	}
}
