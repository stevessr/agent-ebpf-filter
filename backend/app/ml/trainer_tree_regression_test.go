package ml

import (
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestForestMinLeafAppliesToEverySplit(t *testing.T) {
	// A single rare BLOCK outlier used to form a one-row leaf despite
	// minSamplesLeaf=3. Any trained child now contains at least three rows.
	rows := make([]TrainSample, 10)
	for i := range rows {
		rows[i].features[0] = float64(i) / 10
		if i == 9 {
			rows[i].label = 1
		}
	}
	point := findBestSplit(rows, 3, FeatureDim, rand.New(rand.NewSource(9)))
	if point.giniGain <= 0 {
		t.Fatalf("expected a nontrivial valid split, got %+v", point)
	}
	left, right := 0, 0
	for _, row := range rows {
		if row.features[point.featureIdx] < point.threshold {
			left++
		} else {
			right++
		}
	}
	if left < 3 || right < 3 {
		t.Fatalf("split ignored minSamplesLeaf: left=%d right=%d point=%+v", left, right, point)
	}
}

func TestForestStratifiedBootstrapReproducibleAndBalanced(t *testing.T) {
	var input []TrainSample
	for c, n := range []int{90, 5, 3, 2} {
		for i := 0; i < n; i++ {
			input = append(input, TrainSample{label: int32(c)})
		}
	}
	first := make([]TrainSample, 100)
	second := make([]TrainSample, 100)
	classStratifiedBootstrap(input, first, rand.New(rand.NewSource(2026)))
	classStratifiedBootstrap(input, second, rand.New(rand.NewSource(2026)))
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same seed produced different balanced bootstrap samples")
	}
	var counts [4]int
	for _, row := range first {
		counts[row.label]++
	}
	for c, n := range counts {
		if n != 25 {
			t.Errorf("class %d bootstrap count=%d; want 25", c, n)
		}
	}
}

func TestForestMajorityTieIsDeterministic(t *testing.T) {
	input := []TrainSample{{label: 1}, {label: 0}, {label: 1}, {label: 0}}
	for i := 0; i < 100; i++ {
		if got := majorityClass(input); got != 0 {
			t.Fatalf("unstable majority tie: %v", got)
		}
	}
}

func TestForestGiniAndEmptyEvaluation(t *testing.T) {
	if got := giniFromCounts([4]int{5, 5}, 10); math.Abs(got-0.5) > 1e-12 {
		t.Fatalf("gini=%v, want 0.5", got)
	}
	if got := EvaluateForest(nil, nil); got != 0 {
		t.Fatalf("empty holdout falsely scored %v, want 0", got)
	}
}
