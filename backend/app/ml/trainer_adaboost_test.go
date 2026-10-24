package ml

import (
	"fmt"
	"testing"
)

func TestAdaBoostLearnsAllFourActions(t *testing.T) {
	store := NewTrainingDataStore(120)
	var exemplars [4][FeatureDim]float64
	for action := 0; action < 4; action++ {
		for d := 0; d < FeatureDim; d++ {
			if d%4 == action {
				exemplars[action][d] = 1
			}
		}
		for i := 0; i < 25; i++ {
			store.Add(TrainingSample{
				Features: exemplars[action],
				Label: int32(action),
				CommandLine: fmt.Sprintf("ada-action-%d-example-%d", action, i),
				UserLabel: "test",
			})
		}
	}
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelAdaBoost
	cfg.NumTrees = 60
	cfg.MaxDepth = 4
	cfg.MinSamplesLeaf = 2
	cfg.BalanceClasses = true
	model, result := NewModelTrainer().TrainWithConfig(store, cfg)
	if result.Error != "" || model == nil {
		t.Fatalf("AdaBoost training failed: %+v", result)
	}
	if result.TrainSamples+result.ValidationSamples != 100 || result.ValidationSamples < 15 {
		t.Fatalf("AdaBoost must report an independent holdout, not training-as-validation: %+v", result)
	}

	ada, ok := UnwrapModelType(model).(*AdaBoostModel)
	if !ok || len(ada.Stumps) == 0 {
		t.Fatalf("AdaBoost trained no stumps: %T", UnwrapModelType(model))
	}
	// The previous binary learner could not emit REWRITE or ALERT regardless
	// of the input; with SAMME the minority actions remain representable.
	for _, action := range []int32{2, 3} {
		if got := model.Predict(exemplars[action]); got.Action != action {
			t.Fatalf("model never learned action %d: %+v, stumps=%d", action, got, len(ada.Stumps))
		}
	}
}
