package app

import (
	"fmt"
	"testing"
	"time"
)

func TestNormalizeAutoTuneMetricAndGridSize(t *testing.T) {
	if got := normalizeAutoTuneMetric("legalRecall"); got != "allowRecall" {
		t.Fatalf("legalRecall normalized to %q, want allowRecall", got)
	}
	if got := normalizeAutoTuneMetric("balanced_accuracy"); got != "balancedAccuracy" {
		t.Fatalf("balanced_accuracy normalized to %q, want balancedAccuracy", got)
	}
	if got := normalizeAutoTuneGridSize(32); got != 31 {
		t.Fatalf("grid size 32 normalized to %d, want 31", got)
	}
	if got := normalizeAutoTuneGridSize(4); got != 5 {
		t.Fatalf("grid size 4 normalized to %d, want 5", got)
	}
}

func TestAutoTuneReportsLegalRecallBalancedAccuracyAndNormalization(t *testing.T) {
	oldConfig := mlConfig
	t.Cleanup(func() { mlConfig = oldConfig })

	store := newTrainingDataStore(160)
	for i := 0; i < 120; i++ {
		label := int32(i % 4)
		var features [FeatureDim]float64
		features[0] = float64(label) / 3.0
		features[1] = float64(i%10) / 10.0
		features[2] = float64((i/4)%10) / 10.0
		store.Add(TrainingSample{
			Features:    normalizeFeatureVector(features),
			Label:       label,
			Comm:        fmt.Sprintf("cmd-%d", label),
			Args:        []string{fmt.Sprintf("sample-%d", i)},
			Timestamp:   time.Unix(1700000000+int64(i), 0).UTC(),
			UserLabel:   "test",
			CommandLine: fmt.Sprintf("cmd-%d sample-%d", label, i),
		})
	}

	mlConfig = MLConfig{
		ModelType:            ModelRandomForest,
		NumTrees:             5,
		MaxDepth:             4,
		MinSamplesLeaf:       2,
		ValidationSplitRatio: 0.2,
		BalanceClasses:       true,
	}
	globalTrainer.ResetCancel()

	minTrees, maxTrees := 5, 9
	minDepth, maxDepth := 3, 5
	resp, err := globalTrainer.AutoTuneWithConfig(store, mlConfig, MLAutoTuneRequest{
		XAxis:                "numTrees",
		YAxis:                "maxDepth",
		GridSize:             3,
		Metric:               "balancedAccuracy",
		ValidationSplitRatio: 0.2,
		MinX:                 &minTrees,
		MaxX:                 &maxTrees,
		MinY:                 &minDepth,
		MaxY:                 &maxDepth,
	}, nil)
	if err != nil {
		t.Fatalf("AutoTune() error = %v", err)
	}
	if resp.Best == nil || len(resp.Cells) != 9 {
		t.Fatalf("best/cells = %#v/%d, want best and 9 cells", resp.Best, len(resp.Cells))
	}
	if resp.Metric != "balancedAccuracy" {
		t.Fatalf("Metric = %q, want balancedAccuracy", resp.Metric)
	}
	if resp.Best.BalancedAccuracy <= 0 || resp.Best.AllowRecall < 0 || resp.Best.AllowRecall > 1 {
		t.Fatalf("best metrics look invalid: %#v", resp.Best)
	}
	if resp.Normalization.SampleCount != 120 || resp.Normalization.BelowZeroValues != 0 || resp.Normalization.AboveOneValues != 0 {
		t.Fatalf("normalization report = %#v", resp.Normalization)
	}
}
