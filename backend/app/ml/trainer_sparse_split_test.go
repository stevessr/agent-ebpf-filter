package ml

import (
	"math/rand"
	"testing"
)

func TestForestSparseSignalHoldout(t *testing.T) {
	// Only one of 128 dimensions varies. The held-out values differ from all
	// training values; constant padding must not turn a separable node into a
	// majority leaf just because the first random subspace missed the signal.
	training := make([]TrainSample, 40)
	for i := range training {
		training[i].label = int32(i % 2)
		training[i].features[73] = 0.1 + 0.8*float64(i%2) + float64(i/2)*0.001
	}
	var benign, block [FeatureDim]float64
	benign[73], block[73] = 0.15, 0.85
	correct := 0
	const trials = 64
	for seed := int64(0); seed < trials; seed++ {
		tree := DecisionTree{Nodes: buildTree(training, 0, 3, 2, 11, rand.New(rand.NewSource(seed)))}
		if tree.Predict(benign) == 0 {
			correct++
		}
		if tree.Predict(block) == 1 {
			correct++
		}
	}
	accuracy := float64(correct) / (2 * trials)
	t.Logf("sparse-signal paired holdout: correct=%d/%d accuracy=%.6f", correct, 2*trials, accuracy)
	if accuracy != 1 {
		t.Fatalf("random constant features discarded learnable signal: accuracy=%.6f", accuracy)
	}
}

func TestForestSparseFallbackRespectsRandomSubspace(t *testing.T) {
	// The first shuffled feature has a positive but imperfect split. A later
	// perfect feature must not replace it: fallback is not exhaustive CART.
	const seed = 18
	rng := rand.New(rand.NewSource(seed))
	order := make([]int, FeatureDim)
	for i := range order {
		order[i] = i
	}
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	rows := make([]TrainSample, 8)
	for i := range rows {
		if i >= 4 {
			rows[i].label = 1
		}
		rows[i].features[order[0]] = float64(i) / 8
		rows[i].features[order[1]] = float64(rows[i].label)
	}
	// Make the first feature imperfect while keeping a useful split.
	rows[3].features[order[0]], rows[4].features[order[0]] = rows[4].features[order[0]], rows[3].features[order[0]]
	split := findBestSplit(rows, 2, 1, rand.New(rand.NewSource(seed)))
	if split.featureIdx != order[0] || split.giniGain <= 0 {
		t.Fatalf("fallback replaced valid random subspace: %+v, first=%d", split, order[0])
	}
}

func TestForestSparseFallbackCannotInventSplit(t *testing.T) {
	rows := make([]TrainSample, 8)
	for i := range rows {
		rows[i].label = int32(i % 2)
	}
	split := findBestSplit(rows, 2, 11, rand.New(rand.NewSource(42)))
	if split.giniGain > 0 {
		t.Fatalf("constant vectors acquired a split: %+v", split)
	}
	// The only varying dimension isolates one row, violating minSamplesLeaf.
	rows[0].features[73] = 1
	split = findBestSplit(rows, 2, 11, rand.New(rand.NewSource(42)))
	if split.giniGain > 0 {
		t.Fatalf("fallback bypassed minimum leaf size: %+v", split)
	}
	if split = findBestSplit(rows, 1, 0, rand.New(rand.NewSource(42))); split.giniGain > 0 {
		t.Fatalf("disabled feature sampling unexpectedly split: %+v", split)
	}
}
