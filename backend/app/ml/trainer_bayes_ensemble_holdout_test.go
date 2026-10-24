package ml

import (
	"fmt"
	"testing"
	"time"
)

func TestNaiveBayesAndEnsembleTrainOnIndependentBehaviorHoldouts(t *testing.T) {
	for _, mt := range []ModelType{ModelNaiveBayes, ModelEnsemble} {
		t.Run(string(mt), func(t *testing.T) {
			store := NewTrainingDataStore(96)
			base := time.Unix(1_700_000_000, 0)
			for i := 0; i < 96; i++ {
				label := int32(i % 4)
				var features [FeatureDim]float64
				features[label] = 1
				features[64] = float64(i) / 128
				store.Add(TrainingSample{
					Features: features, Label: label,
					CommandLine: fmt.Sprintf("unique-run-%s-%03d", mt, i),
					Timestamp: base.Add(time.Duration(i) * time.Second),
					UserLabel: "test",
				})
			}
			cfg := DefaultMLConfig()
			cfg.ModelType = mt
			cfg.ValidationSplitRatio = 0.25
			trainer := NewModelTrainer()
			model, result := trainer.TrainWithConfig(store, cfg)
			if result.Error != "" || model == nil {
				t.Fatalf("%s training failed: %+v", mt, result)
			}
			if result.NumSamples != 96 || result.TrainSamples+result.ValidationSamples != 96 ||
				result.ValidationSamples < 15 {
				t.Fatalf("not an independent holdout: %+v", result)
			}
			history := trainer.GetHistory()
			if len(history) != 1 || history[0].ValidationSamples != result.ValidationSamples {
				t.Fatalf("training history did not retain validation count: %+v", history)
			}
			trainKeys := make(map[[32]byte]bool)
			for _, sample := range trainer.LastTrainSamples() {
				trainKeys[behaviorHoldoutKey(sample)] = true
			}
			for _, sample := range trainer.LastValidationSamples() {
				if trainKeys[behaviorHoldoutKey(sample)] {
					t.Fatal("same behavior signature in train and validation")
				}
			}
		})
	}
}
