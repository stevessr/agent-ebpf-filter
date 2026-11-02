package ml

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
	"time"
)

// Linear training must never report the fit-set accuracy as the independent
// validation accuracy. In particular, repeated command signatures cannot
// cross the split even when their feature values vary.
func TestLinearClassifiersUseIndependentGroupedHoldout(t *testing.T) {
	for _, mt := range []ModelType{ModelSVM, ModelRidge, ModelPerceptron, ModelPassiveAggressive} {
		t.Run(string(mt), func(t *testing.T) {
			store := holdoutIsolationStore()
			cfg := DefaultMLConfig()
			cfg.ModelType = mt
			cfg.NumTrees = 5
			cfg.MinSamplesLeaf = 2
			cfg.ValidationSplitRatio = 0.25
			trainer := NewModelTrainer()
			model, result := trainer.TrainWithConfig(store, cfg)
			if model == nil || result.Error != "" {
				t.Fatalf("linear classifier training failed: model=%T result=%+v", model, result)
			}
			if result.NumSamples != 80 || result.TrainSamples != 60 || result.ValidationSamples != 20 {
				t.Fatalf("expected a real 60/20 grouped holdout: %+v", result)
			}
			trainRaw, valRaw := trainer.LastTrainSamples(), trainer.LastValidationSamples()
			seen := make(map[[32]byte]bool, len(trainRaw))
			for _, sample := range trainRaw {
				seen[behaviorHoldoutKey(sample)] = true
			}
			for _, sample := range valRaw {
				if seen[behaviorHoldoutKey(sample)] {
					t.Fatal("validation contains a behavior used for fitting")
				}
			}
			trainAccuracy := EvalModelSamples(model, ToTrainSamples(trainRaw))
			valAccuracy := EvalModelSamples(model, ToTrainSamples(valRaw))
			if math.Abs(result.TrainAccuracy-trainAccuracy) > 1e-12 ||
				math.Abs(result.ValidationAccuracy-valAccuracy) > 1e-12 ||
				math.Abs(result.Accuracy-valAccuracy) > 1e-12 {
				t.Fatalf("reported accuracy differs from actual holdout: result=%+v train=%f val=%f", result, trainAccuracy, valAccuracy)
			}
			_, reportedVal, ratio, trainN, valN := trainer.SplitMetrics()
			if math.Abs(reportedVal-valAccuracy) > 1e-12 ||
				math.Abs(ratio-0.25) > 1e-12 || trainN != 60 || valN != 20 {
				t.Fatalf("status/history holdout metadata disagree: ratio=%f train=%d val=%d reported=%f", ratio, trainN, valN, reportedVal)
			}
		})
	}
}

func TestLinearClassifiersRejectDuplicatesOnlyValidation(t *testing.T) {
	for _, mt := range []ModelType{ModelSVM, ModelRidge, ModelPerceptron, ModelPassiveAggressive} {
		t.Run(string(mt), func(t *testing.T) {
			store := NewTrainingDataStore(24)
			for i := 0; i < 24; i++ {
				store.Add(TrainingSample{
					Label: 0, CommandLine: "same-command --same-args",
					Timestamp: time.Unix(int64(1_700_000_000+i), 0).UTC(),
				})
			}
			cfg := DefaultMLConfig()
			cfg.ModelType = mt
			trainer := NewModelTrainer()
			model, result := trainer.TrainWithConfig(store, cfg)
			if model != nil || result.Error == "" {
				t.Fatalf("duplicates-only data falsely trained/evaluated model: model=%T result=%+v", model, result)
			}
		})
	}
}

// Ridge's former tuning path used labeled[:len(trainSet)], even though the
// split only groups membership. One-hot command identities make that leakage
// especially serious: validation IDs accidentally included in the prefix
// can be memorized rather than predicted out of sample.
func TestRidgeAutoTuneScoresOnlyDisjointTrainingRows(t *testing.T) {
	const n = 48
	store := NewTrainingDataStore(n)
	for i := 0; i < n; i++ {
		var features [FeatureDim]float64
		features[i] = 1
		store.Add(TrainingSample{
			Features: features,
			Label: int32(i % 4),
			CommandLine: fmt.Sprintf("independent-identity-%02d", i),
			UserLabel: "regression",
			Timestamp: time.Unix(int64(1_700_000_000+i), 0).UTC(),
		})
	}
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRidge
	cfg.NumTrees = 3
	cfg.MaxDepth = 4
	cfg.MinSamplesLeaf = 1
	cfg.ValidationSplitRatio = 0.25
	minTrees, maxTrees := 3, 5
	minDepth, maxDepth := 4, 6
	trainer := NewModelTrainer()
	result, err := trainer.AutoTuneWithConfig(store, cfg, MLAutoTuneRequest{
		XAxis: "numTrees", YAxis: "maxDepth", GridSize: 3,
		Metric: "accuracy", ValidationSplitRatio: 0.25,
		MinX: &minTrees, MaxX: &maxTrees,
		MinY: &minDepth, MaxY: &maxDepth,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Cells) != 9 {
		t.Fatalf("expected 9 candidates, got %d", len(result.Cells))
	}
	training, validation, trainRaw, _, err := PrepareAutoTuneSplit(store.LabeledSamples(), 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if len(training)+len(validation) != n {
		t.Fatalf("unexpected split train=%d validation=%d", len(training), len(validation))
	}
	for _, cell := range result.Cells {
		weights := make([][FeatureDim + 1]float64, 4)
		seed := int64((cell.YIndex+1)*100000 + (cell.XIndex+1)*1000 +
			cell.NumTrees*31 + cell.MaxDepth*17 + cell.MinSamplesLeaf*13)
		rng := rand.New(rand.NewSource(seed))
		for c := range weights {
			for d := range weights[c] {
				weights[c][d] = (rng.Float64() - 0.5) * 0.01
			}
		}
		RidgeFit(weights, 4, trainRaw, float64(cell.NumTrees)/100.0+0.1)
		wantTrain := EvalLinearModel(weights, 4, training)
		wantVal := EvalLinearModel(weights, 4, validation)
		if math.Abs(cell.TrainAccuracy-wantTrain) > 1e-12 ||
			math.Abs(cell.ValidationAccuracy-wantVal) > 1e-12 {
			t.Fatalf("tuning cell (%d,%d) leaked split data: got train=%f val=%f want train=%f val=%f",
				cell.XIndex, cell.YIndex, cell.TrainAccuracy, cell.ValidationAccuracy, wantTrain, wantVal)
		}
	}
}
