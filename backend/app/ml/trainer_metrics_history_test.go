package ml

import (
	"math"
	"testing"
)

func TestFinishMetricsRecordsTrueHoldoutStatistics(t *testing.T) {
	trainer := NewModelTrainer()
	trainer.finishMetrics(0.73, 0.95, 0.73, 100, 78, 22)
	history := trainer.GetHistory()
	if len(history) != 1 {
		t.Fatalf("training history length = %d", len(history))
	}
	got := history[0]
	if got.NumSamples != 100 || got.TrainSamples != 78 || got.ValidationSamples != 22 {
		t.Fatalf("incorrect sample counts in training history: %+v", got)
	}
	if math.Abs(got.TrainAccuracy-0.95) > 1e-10 || math.Abs(got.ValidationAccuracy-0.73) > 1e-10 {
		t.Fatalf("train and validation scores were conflated: %+v", got)
	}
	if math.Abs(got.ValidationSplitRatio-0.22) > 1e-10 ||
		math.Abs(trainer.StateSnapshot().ValidationRatio-0.22) > 1e-10 {
		t.Fatalf("incorrect effective holdout ratio: %+v", got)
	}
	trainer.finishMetrics(0.8, 0.8, 0.8, 20, 20, 0)
	latest := trainer.GetHistory()[1]
	if latest.ValidationSamples != 0 || latest.ValidationSplitRatio != 0 ||
		trainer.StateSnapshot().ValidationRatio != 0 {
		t.Fatalf("training-only metrics misrepresented as held-out validation: %+v", latest)
	}
}
