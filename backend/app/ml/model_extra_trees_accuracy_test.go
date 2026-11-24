package ml

import (
	"math/rand"
	"testing"
)

// Only feature 73 carries the label. Extra Trees used to inspect a single
// random feature per node, and most trees collapsed to a majority-class leaf.
func TestExtraTreesRecoversSparseMinoritySignal(t *testing.T) {
	train := make([]TrainSample, 0, 100)
	for i := 0; i < 100; i++ {
		var features [FeatureDim]float64
		label := int32(0)
		if i >= 90 {
			label = 1
			features[73] = 1
		}
		train = append(train, TrainSample{features: features, label: label})
	}
	forest := BuildExtraTreesWithBalance(train, 21, 3, 2, 20261024, true)
	if !forest.IsTrained || len(forest.Trees) != 21 {
		t.Fatalf("forest failed to train: %+v", forest)
	}
	var allow, block [FeatureDim]float64
	block[73] = 1
	if pred := forest.Predict(allow); pred.Action != 0 {
		t.Errorf("benign example misclassified: %+v", pred)
	}
	if pred := forest.Predict(block); pred.Action != 1 {
		t.Errorf("minority BLOCK example missed: %+v", pred)
	}
}

func TestExtraTreesRespectMinSamplesLeaf(t *testing.T) {
	rng := rand.New(rand.NewSource(77))
	rows := make([]TrainSample, 50)
	for i := range rows {
		rows[i].features[4] = float64(i) / float64(len(rows))
		rows[i].label = int32(i % 4)
	}
	minLeaf := 5
	tree := buildExtraTree(rows, 0, 7, minLeaf, 8, rng)
	var visit func(int, []TrainSample)
	visit = func(index int, partition []TrainSample) {
		t.Helper()
		if index < 0 || index >= len(tree) {
			t.Fatalf("invalid tree node %d", index)
		}
		node := tree[index]
		if node.IsLeaf() {
			return
		}
		var left, right []TrainSample
		for _, row := range partition {
			if row.features[node.FeatureIndex] < float64(node.Threshold) {
				left = append(left, row)
			} else {
				right = append(right, row)
			}
		}
		if len(left) < minLeaf || len(right) < minLeaf {
			t.Fatalf("invalid leaf split: left=%d right=%d at node %d", len(left), len(right), index)
		}
		visit(int(node.LeftChild), left)
		visit(int(node.RightChild), right)
	}
	visit(0, rows)
}

func TestExtraTreesEmptyStoreCannotAppearTrained(t *testing.T) {
	forest := BuildExtraTrees(nil, 5, 4, 2, 100)
	if forest.IsTrained {
		t.Fatal("empty Extra Trees fitted a phantom classifier")
	}
}
