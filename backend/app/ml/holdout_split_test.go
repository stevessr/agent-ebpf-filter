package ml

import (
	"fmt"
	"math"
	"testing"
)

func TestGroupedHoldoutIsDeterministicAndStratified(t *testing.T) {
	samples := make([]TrainingSample, 0, 80)
	for label := int32(0); label < 4; label++ {
		for i := 0; i < 20; i++ {
			var f [FeatureDim]float64
			f[label] = 1
			f[31] = float64(i+1) / 32.0
			samples = append(samples, TrainingSample{
				Features: f, Label: label,
				CommandLine: fmt.Sprintf("fixture-class-%d-command-%d", label, i),
			})
		}
	}
	train, val, rawTrain, rawVal, err := PrepareAutoTuneSplit(samples, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if len(train) != 60 || len(val) != 20 {
		t.Fatalf("unexpected class-balanced split train=%d val=%d", len(train), len(val))
	}
	var trainClasses, valClasses [4]int
	for _, s := range train {
		trainClasses[s.label]++
	}
	for _, s := range val {
		valClasses[s.label]++
	}
	for c := 0; c < 4; c++ {
		if trainClasses[c] != 15 || valClasses[c] != 5 {
			t.Fatalf("class %d distribution train=%d val=%d; want 15/5", c, trainClasses[c], valClasses[c])
		}
	}
	_, _, _, again, err := PrepareAutoTuneSplit(samples, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawTrain) != len(train) || len(rawVal) != len(val) || len(again) != len(rawVal) {
		t.Fatal("raw and vector split sizes do not agree")
	}
	for i, s := range rawVal {
		if behaviorHoldoutKey(s) != behaviorHoldoutKey(again[i]) {
			t.Fatalf("non-deterministic holdout at row %d", i)
		}
	}
}

func TestGroupedHoldoutKeepsRepeatedBehaviorTogether(t *testing.T) {
	var samples []TrainingSample
	for label := int32(0); label < 4; label++ {
		for group := 0; group < 12; group++ {
			for repeat := 0; repeat < 3; repeat++ {
				var f [FeatureDim]float64
				f[label] = 1
				f[31] = float64(group)/20 + float64(repeat)/1000
				samples = append(samples, TrainingSample{
					Features: f, Label: label,
					CommandLine: fmt.Sprintf("same-command-%d-%d", label, group),
				})
			}
		}
	}
	_, _, trainRaw, valRaw, err := PrepareAutoTuneSplit(samples, 0.25)
	if err != nil {
		t.Fatal(err)
	}
	keys := make(map[[32]byte]bool, len(trainRaw))
	for _, s := range trainRaw {
		keys[behaviorHoldoutKey(s)] = true
	}
	for _, s := range valRaw {
		if keys[behaviorHoldoutKey(s)] {
			t.Fatalf("repeated command crossed holdout boundary: %s", s.CommandLine)
		}
	}
}

func TestGroupedHoldoutRequiresIndependentBehavior(t *testing.T) {
	var samples []TrainingSample
	for i := 0; i < 12; i++ {
		samples = append(samples, TrainingSample{Label: 0, CommandLine: "same command"})
	}
	_, _, _, _, err := PrepareAutoTuneSplit(samples, 0.2)
	if err == nil {
		t.Fatal("duplicates-only data must not produce a fake holdout")
	}
}

func TestKNNExactMatchOutvotesNearbyClassMajority(t *testing.T) {
	m := NewKNNModel(5, "euclidean", "distance")
	m.NumClasses = 4
	m.Samples = make([][FeatureDim]float64, 5)
	m.Labels = []int32{1, 0, 0, 0, 0}
	m.Samples[0][0] = 0.5
	for i := 1; i < 5; i++ {
		m.Samples[i][0] = 0.5 + float64(i)/100
	}
	var query [FeatureDim]float64
	query[0] = 0.5
	pred := m.Predict(query)
	if pred.Action != 1 || math.Abs(pred.Confidence-1) > 1e-9 {
		t.Fatalf("exact match lost to neighbors: %+v", pred)
	}
	if pred.AnomalyScore > 1e-9 {
		t.Fatalf("exact match must not look anomalous due to ignored neighbors: %+v", pred)
	}
}
