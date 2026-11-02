package ml

// Autotune forest construction shared by the auto-tuner (package app),
// the sweep harness and the model registry tests.

import (
	"math"
	"math/rand"
)

// PrepareAutoTuneSplit partitions duplicates as complete groups, then stratifies
// the group assignment where class evidence permits. A missing validation class
// is reported by security metric coverage rather than invented as zero recall.
func PrepareAutoTuneSplit(labeled []TrainingSample, validationRatio float64) ([]TrainSample, []TrainSample, []TrainingSample, []TrainingSample, error) {
	return prepareGroupedHoldout(labeled, validationRatio)
}

func BuildAutoTuneForest(trainSet []TrainSample, numTrees, maxDepth, minSamplesLeaf int, seed int64) *DecisionForest {
	if numTrees < 1 {
		numTrees = 1
	}
	if maxDepth < 1 {
		maxDepth = 1
	}
	if minSamplesLeaf < 1 {
		minSamplesLeaf = 1
	}

	rng := rand.New(rand.NewSource(seed))
	forest := NewDecisionForest(numTrees, maxDepth, 4)
	featureSampleCount := int(math.Sqrt(float64(FeatureDim)))
	if featureSampleCount < 1 {
		featureSampleCount = 1
	}

	if len(trainSet) == 0 {
		// An empty training partition is not a fitted model; avoid Intn(0).
		forest.Trees = nil
		return forest
	}
	for ti := 0; ti < numTrees; ti++ {
		bootstrap := make([]TrainSample, len(trainSet))
		// Match the promoted trainer's class-stratified sampling. The
		// previous uniform bootstrap tuned a different model family and
		// could miss rare BLOCK/ALERT observations entirely.
		classStratifiedBootstrap(trainSet, bootstrap, rng)
		nodes := buildTree(bootstrap, 0, maxDepth, minSamplesLeaf, featureSampleCount, rng)
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
	}

	forest.IsTrained = true
	// The production trainer prunes using the fit partition only. Apply the
	// identical operation before evaluating this candidate on the holdout.
	forest.Prune(trainSet)
	return forest
}

// buildAutoTuneTree is retained for the lightweight ensemble builder, but
// delegates to the same tree builder as the production and tuning forests.
func buildAutoTuneTree(samples []TrainSample, depth, maxDepth, minSamplesLeaf, featureSampleCount int, rng *rand.Rand) []DecisionNode {
	return buildTree(samples, depth, maxDepth, minSamplesLeaf, featureSampleCount, rng)
}
