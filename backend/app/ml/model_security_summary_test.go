package ml

import "testing"

func TestSummarizeModelTuneSecurityWeightsByVectorSupport(t *testing.T) {
	candidates := []MLModelTuneCandidate{
		{
			ModelType:    "a",
			Label:        "A",
			Family:       "tree",
			FamilyLabel:  "树模型",
			FeatureClass: "tabular_128",
			AttackMetrics: AttackImpactMetrics{
				ScoredSamples:        20,
				BenignSamples:        10,
				AttackSamples:        10,
				HighImpactSamples:    1,
				DestructionSamples:   1,
				AttackRecall:         1,
				HighImpactRecall:     1,
				DestructionRecall:    1,
				SecurityUtility:      0.92,
				CatastrophicMissRate: 0,
			},
		},
		{
			ModelType:    "b",
			Label:        "B",
			Family:       "tree",
			FamilyLabel:  "树模型",
			FeatureClass: "tabular_128",
			AttackMetrics: AttackImpactMetrics{
				ScoredSamples:        200,
				BenignSamples:        100,
				AttackSamples:        100,
				HighImpactSamples:    100,
				DestructionSamples:   100,
				AttackRecall:         0.90,
				HighImpactRecall:     0.90,
				DestructionRecall:    0.90,
				SecurityUtility:      0.90,
				CatastrophicMissRate: 0.10,
			},
		},
	}

	summaries := SummarizeModelTuneSecurity(candidates)
	if len(summaries) != 1 {
		t.Fatalf("summary groups=%d want 1", len(summaries))
	}
	got := summaries[0]
	if got.ModelCount != 2 || got.SuccessfulModels != 2 {
		t.Fatalf("unexpected model counts: %+v", got)
	}
	// Weighted result should stay close to the 100-sample candidate rather than
	// averaging the 1/1 model and 90/100 model to 95%.
	if got.DestructionRecall >= 0.92 || got.DestructionRecall <= 0.89 {
		t.Fatalf("destruction recall should be support-weighted, got %v", got.DestructionRecall)
	}
	if got.HighImpactRecall >= 0.92 || got.HighImpactRecall <= 0.89 {
		t.Fatalf("high-impact recall should be support-weighted, got %v", got.HighImpactRecall)
	}
	if got.BestModelType != "a" {
		t.Fatalf("best security-utility model=%q want a", got.BestModelType)
	}
}

func TestSummarizeModelTuneSecuritySeparatesFamilyFeaturePairs(t *testing.T) {
	candidates := []MLModelTuneCandidate{
		{ModelType: "rf", Family: "tree", FamilyLabel: "树模型", FeatureClass: "tabular_128", AttackMetrics: AttackImpactMetrics{ScoredSamples: 10, SecurityUtility: 0.8}},
		{ModelType: "rf_mamba", Family: "tree", FamilyLabel: "树模型", FeatureClass: "sequence_state_space", AttackMetrics: AttackImpactMetrics{ScoredSamples: 10, SecurityUtility: 0.9}},
		{ModelType: "logistic", Family: "linear", FamilyLabel: "线性模型", FeatureClass: "tabular_128", AttackMetrics: AttackImpactMetrics{ScoredSamples: 10, SecurityUtility: 0.7}},
	}
	groups := SummarizeModelTuneSecurity(candidates)
	if len(groups) != 3 {
		t.Fatalf("family-feature groups=%d want 3", len(groups))
	}
	if groups[0].Key != "tree/sequence_state_space" {
		t.Fatalf("highest-security group should sort first, got %+v", groups[0])
	}
}
