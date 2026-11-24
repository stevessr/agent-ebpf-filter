package ml

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestModelTrainerCancellationIsConcurrentSafe(t *testing.T) {
	trainer := &ModelTrainer{
		mu:         make(chan struct{}, 1),
		cancelCh:   make(chan struct{}),
		logMaxSize: 16,
	}
	trainer.BeginTraining()

	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			trainer.CancelTraining()
		}()
	}
	workers.Wait()
	if !trainer.IsCancelled() {
		t.Fatal("concurrent cancellation did not close the active cancel channel")
	}
	trainer.finishTraining()
	if trainer.IsRunning() {
		t.Fatal("trainer remained running after finish")
	}
}

func TestModelTrainerLogRingReturnsChronologicalTail(t *testing.T) {
	trainer := &ModelTrainer{logMaxSize: 3}
	for i := 0; i < 5; i++ {
		trainer.Logf("entry-%d", i)
	}
	logs := trainer.GetLogs(3)
	if len(logs) != 3 {
		t.Fatalf("GetLogs() length = %d, want 3", len(logs))
	}
	for i, want := range []string{"entry-2", "entry-3", "entry-4"} {
		if logs[i].Message != want {
			t.Fatalf("GetLogs()[%d] = %q, want %q", i, logs[i].Message, want)
		}
	}
	if total := trainer.LogTotal(); total != 5 {
		t.Fatalf("LogTotal() = %d, want 5", total)
	}
}

func TestModelTrainerStateSnapshot(t *testing.T) {
	trainer := &ModelTrainer{}
	trainedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	trainer.setTrainingResult(trainedAt, 0.9, 0.95, 0.9)
	trainer.setValidationRatio(0.2)
	trainer.setTrainingProgress(0.75)

	state := trainer.StateSnapshot()
	if state.LastTrain != trainedAt || state.Accuracy != 0.9 || state.TrainAccuracy != 0.95 || state.ValidationAccuracy != 0.9 || state.ValidationRatio != 0.2 || state.Progress != 0.75 {
		t.Fatalf("unexpected trainer state: %+v", state)
	}
}

func TestTrainWithConfigUsesCallerValidationRatio(t *testing.T) {
	store := NewTrainingDataStore(120)
	for index := 0; index < 120; index++ {
		var features [FeatureDim]float64
		features[0] = float64(index%4) / 3
		features[1] = float64(index%11) / 10
		store.Add(TrainingSample{
			Features:  features,
			Label:     int32(index % 4),
			UserLabel: "test",
		})
	}

	trainer := &ModelTrainer{
		mu:         make(chan struct{}, 1),
		cancelCh:   make(chan struct{}),
		logMaxSize: 16,
	}
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest
	cfg.NumTrees = 3
	cfg.MaxDepth = 4
	cfg.MinSamplesLeaf = 2
	cfg.ValidationSplitRatio = 0.25
	model, result := trainer.TrainWithConfig(store, cfg)
	if result.Error != "" || model == nil {
		t.Fatalf("TrainWithConfig() model/error = %T/%q", model, result.Error)
	}
	// This fixture repeats feature-identical behavior groups. They must remain
	// intact even if the holdout count differs by a few rows from exactly 25%.
	if result.TrainSamples+result.ValidationSamples != 120 || result.ValidationSamples < 27 || result.ValidationSamples > 33 {
		t.Fatalf("group-preserving split = train:%d validation:%d, expected roughly 90/30", result.TrainSamples, result.ValidationSamples)
	}
	if ratio := trainer.StateSnapshot().ValidationRatio; math.Abs(ratio-float64(result.ValidationSamples)/120) > 1e-9 {
		t.Fatalf("trainer validation ratio = %f, does not match actual %d/120", ratio, result.ValidationSamples)
	}
}
