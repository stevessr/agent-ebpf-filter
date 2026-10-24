package ml

import (
	"math"
	"testing"
)

func TestAutoTuneAdaBoostSupportsFourActionsDeterministically(t *testing.T) {
	var X [][FeatureDim]float64
	var Y []int32
	for label := 0; label < 4; label++ {
		for i := 0; i < 24; i++ {
			var features [FeatureDim]float64
			for d := 0; d < FeatureDim; d++ {
				if d%4 == label { features[d] = 1 }
			}
			features[127] = float64(i) / 1000
			X = append(X, features)
			Y = append(Y, int32(label))
		}
	}
	first := trainAdaBoostFromDataBalanced(X, Y, 60, true)
	second := trainAdaBoostFromDataBalanced(X, Y, 60, true)
	if len(first.Stumps) == 0 || len(first.Stumps) != len(second.Stumps) {
		t.Fatalf("unexpected non-deterministic AutoTune weak learner counts: %d / %d", len(first.Stumps), len(second.Stumps))
	}
	for i, stump := range first.Stumps {
		if stump != second.Stumps[i] || math.Abs(first.Alphas[i]-second.Alphas[i]) > 1e-12 {
			t.Fatalf("candidate training is not reproducible at stump %d", i)
		}
	}
	for _, label := range []int32{2, 3} {
		var features [FeatureDim]float64
		for d := 0; d < FeatureDim; d++ {
			if d%4 == int(label) { features[d] = 1 }
		}
		prediction := first.Predict(features)
		if prediction.Action != label {
			t.Fatalf("AutoTune binary-regression: class=%d prediction=%+v", label, prediction)
		}
	}
}

func TestAutoTuneAdaBoostRejectsInvalidTrainingLabels(t *testing.T) {
	model := trainAdaBoostFromDataBalanced(make([][FeatureDim]float64, 2), []int32{0, 17}, 12, true)
	if len(model.Stumps) != 0 {
		t.Fatalf("invalid four-class labels generated a trained model: %v", model.Stumps)
	}
}
