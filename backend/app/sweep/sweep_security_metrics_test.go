package sweep

import (
	"testing"

	"agent-ebpf-filter/app/ml"
)

// sweepBlockAllModel predicts BLOCK for every sample regardless of features.
type sweepBlockAllModel struct{}

func (sweepBlockAllModel) Predict([ml.FeatureDim]float64) ml.Prediction {
	return ml.Prediction{Action: 1, Confidence: 1}
}

func (sweepBlockAllModel) Serialize(string) error { return nil }

func (sweepBlockAllModel) Type() ml.ModelType { return ml.ModelRandomForest }

func TestEvaluateClassMetricsBlockEverything(t *testing.T) {
	samples := []ml.TrainingSample{
		{Label: 0},
		{Label: 0},
		{Label: 0},
		{Label: 1},
		{Label: 1},
	}
	metrics := evaluateClassMetrics(sweepBlockAllModel{}, samples)
	if metrics.AllowPassRate != 0 {
		t.Fatalf("AllowPassRate = %v, want 0", metrics.AllowPassRate)
	}
	if metrics.BlockPassRate != 1 {
		t.Fatalf("BlockPassRate = %v, want 1", metrics.BlockPassRate)
	}
	if metrics.BalancedAccuracy != 0.5 {
		t.Fatalf("BalancedAccuracy = %v, want 0.5", metrics.BalancedAccuracy)
	}
	if metrics.Accuracy != 0.4 {
		t.Fatalf("Accuracy = %v, want 0.4", metrics.Accuracy)
	}
	if metrics.AllowTotal != 3 || metrics.AllowCorrect != 0 {
		t.Fatalf("ALLOW correct/total = %d/%d, want 0/3", metrics.AllowCorrect, metrics.AllowTotal)
	}
	if metrics.BlockTotal != 2 || metrics.BlockCorrect != 2 {
		t.Fatalf("BLOCK correct/total = %d/%d, want 2/2", metrics.BlockCorrect, metrics.BlockTotal)
	}
}

func TestBestSweepResultPrefersBalancedAccuracy(t *testing.T) {
	rows := []sweepResult{
		{ConfigSummary: "high-val-low-bal", ValidationAccuracy: 0.97, BalancedAccuracy: 0.80, AllowPassRate: 0.90, BlockPassRate: 0.70},
		{ConfigSummary: "failed-run", ValidationAccuracy: 1, BalancedAccuracy: 1, AllowPassRate: 1, BlockPassRate: 1, Error: "boom"},
		{ConfigSummary: "low-val-high-bal", ValidationAccuracy: 0.90, BalancedAccuracy: 0.95, AllowPassRate: 0.90, BlockPassRate: 1},
	}
	best := bestSweepResult(rows)
	if best.ConfigSummary != "low-val-high-bal" {
		t.Fatalf("bestSweepResult picked %q, want the higher balanced accuracy config", best.ConfigSummary)
	}
}

func TestSelectTopRepeatConfigsUsesCanonicalOrdering(t *testing.T) {
	profile := sweepProfile{Name: "security-metrics-order", Kind: "bar"}
	rows := []sweepResult{
		{ConfigSummary: "high-val-low-bal", ValidationAccuracy: 0.97, BalancedAccuracy: 0.80, NumSamples: 100, TrainSamples: 80, ValidationSamples: 20, HoldoutStrategy: holdoutStrategyGroupedBehaviorV1},
		{ConfigSummary: "failed-run", ValidationAccuracy: 1, BalancedAccuracy: 1, Error: "boom"},
		{ConfigSummary: "low-val-high-bal", ValidationAccuracy: 0.90, BalancedAccuracy: 0.95, NumSamples: 100, TrainSamples: 80, ValidationSamples: 20, HoldoutStrategy: holdoutStrategyGroupedBehaviorV1},
	}
	tasks := selectTopRepeatConfigs(profile, rows, 2, nil, nil)
	if len(tasks) != 2 {
		t.Fatalf("selectTopRepeatConfigs returned %d tasks, want 2", len(tasks))
	}
	if tasks[0].Config.ConfigSummary != "low-val-high-bal" {
		t.Fatalf("selectTopRepeatConfigs ranked %q first, want the balanced accuracy leader", tasks[0].Config.ConfigSummary)
	}
	if tasks[1].Config.ConfigSummary != "high-val-low-bal" {
		t.Fatalf("selectTopRepeatConfigs ranked %q second, want the raw-accuracy leader as runner-up", tasks[1].Config.ConfigSummary)
	}
	if tasks[0].Profile.Name != profile.Name {
		t.Fatalf("stability task carries profile %q, want %q", tasks[0].Profile.Name, profile.Name)
	}
}

func TestBestComparableSummaryUsesCanonicalOrdering(t *testing.T) {
	summaries := []repeatSummary{
		{Profile: "high-val-low-bal", Comparable: true, ValidationMean: 0.97, BalancedMean: 0.80},
		{Profile: "non-comparable-star", Comparable: false, ValidationMean: 1, BalancedMean: 1},
		{Profile: "low-val-high-bal", Comparable: true, ValidationMean: 0.90, BalancedMean: 0.95},
	}
	best := bestComparableSummary(summaries)
	if best == nil {
		t.Fatal("bestComparableSummary returned nil for summaries with comparable rows")
	}
	if best.Profile != "low-val-high-bal" {
		t.Fatalf("bestComparableSummary picked %q, want the balanced accuracy leader among comparable rows", best.Profile)
	}
	if only := bestComparableSummary([]repeatSummary{{Profile: "non-comparable", Comparable: false, BalancedMean: 1}}); only != nil {
		t.Fatalf("bestComparableSummary = %v, want nil when no summary is comparable", only)
	}
}
