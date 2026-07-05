package app

import "testing"

func TestResearchSecurityEvaluationPostureRecommendations(t *testing.T) {
	report := ResearchSecurityEvaluationReport{
		Totals: ResearchSecurityEvaluationTotals{Total: 2, Labeled: 2, Risky: 1, Benign: 1, Passed: 1, Failed: 1},
		Metrics: ResearchSecurityEvaluationMetrics{
			Accuracy:          50,
			FalseNegativeRate: 50,
			BalancedAccuracy:  50,
		},
		ByCategory: []ResearchSecurityEvaluationGroup{
			{Key: "network-exfiltration", Total: 2, Passed: 1, Failed: 1, FalseNegatives: 1, AvgRiskScore: 82.5},
		},
		Findings: ResearchSecurityEvaluationFindings{
			FalseNegatives: []ResearchSecurityEvaluationSampleRow{{ID: "fn-1", FindingType: "false_negative"}},
		},
	}

	posture := buildResearchSecurityEvaluationPosture(report)
	if posture.Status != "critical" {
		t.Fatalf("posture.Status = %q, want critical: %+v", posture.Status, posture)
	}
	if posture.RiskScore <= 0 {
		t.Fatalf("expected positive risk score: %+v", posture)
	}
	if !hasPrefix(posture.BlockingReasons, "false_negatives:") || !hasPrefix(posture.BlockingReasons, "false_negative_rate:") {
		t.Fatalf("expected false negative blocking reasons, got %v", posture.BlockingReasons)
	}
	if !hasExact(posture.SuggestedActions, "tighten_detection_for_missed_risky_agent_behaviors") || !hasExact(posture.SuggestedActions, "add_false_negative_cases_to_training_dataset") {
		t.Fatalf("expected false-negative suggested actions, got %v", posture.SuggestedActions)
	}
	if len(posture.TopFailingCategories) != 1 || posture.TopFailingCategories[0].Key != "network-exfiltration" {
		t.Fatalf("unexpected top failing categories: %+v", posture.TopFailingCategories)
	}
}
