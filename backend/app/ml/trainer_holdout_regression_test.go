package ml

import (
	"math"
	"testing"
)

func holdoutIsolationStore() *TrainingDataStore {
	store := NewTrainingDataStore(100)
	for i := 0; i < 80; i++ {
		var features [FeatureDim]float64
		features[i%4] = 1
		features[31] = float64(i+1) / 100
		features[32] = float64((i*7)%19) / 20
		store.Add(TrainingSample{
			Features:  features,
			Label:     int32(i % 4),
			UserLabel: "test",
		})
	}
	return store
}

// A sample cannot be both a training observation and a held-out evaluation
// observation. Reusing the full sample store for KNN yielded trivial accuracy.
func TestClassifierHoldoutIsolation(t *testing.T) {
	for _, modelType := range []ModelType{ModelKNN, ModelExtraTrees, ModelLogisticRegression} {
		t.Run(string(modelType), func(t *testing.T) {
			trainer := NewModelTrainer()
			cfg := DefaultMLConfig()
			cfg.ModelType = modelType
			cfg.ValidationSplitRatio = 0.25
			cfg.NumTrees = 5
			cfg.MinSamplesLeaf = 2
			if modelType == ModelLogisticRegression {
				cfg.MinSamplesLeaf = 100
			}
			model, result := trainer.TrainWithConfig(holdoutIsolationStore(), cfg)
			if result.Error != "" || model == nil {
				t.Fatalf("training: model=%T result=%+v", model, result)
			}
			if result.TrainSamples != 60 || result.ValidationSamples != 20 {
				t.Fatalf("want 60 train / 20 validation, got %+v", result)
			}
			train := trainer.LastTrainSamples()
			validation := trainer.LastValidationSamples()
			if len(train) != 60 || len(validation) != 20 {
				t.Fatalf("actual split train=%d validation=%d", len(train), len(validation))
			}
			ids := map[float64]bool{}
			for _, sample := range train {
				ids[sample.Features[31]] = true
			}
			for _, sample := range validation {
				if ids[sample.Features[31]] {
					t.Fatalf("holdout sample %.4f is also in training", sample.Features[31])
				}
			}
			if math.IsNaN(result.ValidationAccuracy) || result.ValidationAccuracy < 0 || result.ValidationAccuracy > 1 {
				t.Fatalf("invalid validation accuracy: %v", result.ValidationAccuracy)
			}
			if modelType == ModelKNN {
				knn, ok := UnwrapModelType(model).(*KNNModel)
				if !ok {
					t.Fatalf("expected KNN, got %T", UnwrapModelType(model))
				}
				if len(knn.Samples) != len(train) {
					t.Fatalf("KNN retained %d examples, expected only %d training rows", len(knn.Samples), len(train))
				}
				for _, sample := range knn.Samples {
					if !ids[sample[31]] {
						t.Fatalf("KNN contains unseen training ID %.4f", sample[31])
					}
				}
			}
		})
	}
}

func TestLogisticHoldoutDefaultsToValidation(t *testing.T) {
	trainer := NewModelTrainer()
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelLogisticRegression
	cfg.ValidationSplitRatio = 0 // default must not report training accuracy as validation accuracy
	cfg.MinSamplesLeaf = 100
	cfg.NumTrees = 5
	_, result := trainer.TrainWithConfig(holdoutIsolationStore(), cfg)
	if result.Error != "" || result.ValidationSamples == 0 || result.TrainSamples+result.ValidationSamples != 80 {
		t.Fatalf("invalid default split: %+v", result)
	}
}

func TestLogisticCUDATrainingCompatibility(t *testing.T) {
	model := NewLogisticModel(0.01, "l2", 100)
	labels := make([]int32, 128)
	if !model.cudaTrainingCompatible(labels) {
		t.Fatal("valid unweighted dataset must be eligible for CUDA")
	}
	model.ClassWeights = []float64{1, 5, 1, 1}
	if model.cudaTrainingCompatible(labels) {
		t.Fatal("CUDA must not silently discard loss class weights")
	}
	model.ClassWeights = nil
	labels[7] = -1
	if model.cudaTrainingCompatible(labels) {
		t.Fatal("invalid negative label must not reach GPU")
	}
	labels[7] = 4
	if model.cudaTrainingCompatible(labels) {
		t.Fatal("out-of-bounds label must not reach GPU")
	}
}
