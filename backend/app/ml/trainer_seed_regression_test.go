package ml

import (
	"reflect"
	"testing"
)

// Offline candidates must not change merely because another process has
// consumed global random state or because a CI job runs a few seconds later.
func TestForestTrainingSeedReproducible(t *testing.T) {
	store := holdoutIsolationStore()
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest
	cfg.NumTrees = 9
	cfg.MaxDepth = 6
	cfg.MinSamplesLeaf = 2
	cfg.ValidationSplitRatio = 0.25
	cfg.TrainingSeed = 20261108

	var first *DecisionForest
	var firstAccuracy float64
	for run := 0; run < 2; run++ {
		trainer := NewModelTrainer()
		model, result := trainer.TrainWithConfig(store, cfg)
		if result.Error != "" || model == nil {
			t.Fatalf("seeded training run %d failed: %+v", run, result)
		}
		forest, ok := UnwrapModelType(model).(*DecisionForest)
		if !ok {
			t.Fatalf("expected forest model, got %T", model)
		}
		if run == 0 {
			first = forest
			firstAccuracy = result.ValidationAccuracy
			continue
		}
		if !reflect.DeepEqual(first.Trees, forest.Trees) {
			t.Fatal("identical training data and seed produced different tree structures")
		}
		if firstAccuracy != result.ValidationAccuracy {
			t.Fatalf("seeded held-out accuracy drifted: first=%v second=%v", firstAccuracy, result.ValidationAccuracy)
		}
	}
}
