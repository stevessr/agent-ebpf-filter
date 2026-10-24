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

	for ti := 0; ti < numTrees; ti++ {
		bootstrap := make([]TrainSample, len(trainSet))
		for i := range bootstrap {
			bootstrap[i] = trainSet[rng.Intn(len(trainSet))]
		}
		nodes := buildAutoTuneTree(bootstrap, 0, maxDepth, minSamplesLeaf, featureSampleCount, rng)
		forest.Trees[ti] = DecisionTree{Nodes: nodes}
	}

	forest.IsTrained = true
	return forest
}

func buildAutoTuneTree(samples []TrainSample, depth, maxDepth, minSamplesLeaf, featureSampleCount int, rng *rand.Rand) []DecisionNode {
	if depth >= maxDepth || len(samples) < minSamplesLeaf*2 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	allSame := true
	firstLabel := samples[0].label
	for _, s := range samples[1:] {
		if s.label != firstLabel {
			allSame = false
			break
		}
	}
	if allSame {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: float32(firstLabel)}}
	}

	best := findBestSplit(samples, minSamplesLeaf, featureSampleCount, rng)
	if best.giniGain <= 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	var leftSamples, rightSamples []TrainSample
	for _, s := range samples {
		if s.features[best.featureIdx] < best.threshold {
			leftSamples = append(leftSamples, s)
		} else {
			rightSamples = append(rightSamples, s)
		}
	}
	if len(leftSamples) == 0 || len(rightSamples) == 0 {
		return []DecisionNode{{LeftChild: -1, RightChild: -1, LeafValue: majorityClass(samples)}}
	}

	leftNodes := buildAutoTuneTree(leftSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount, rng)
	rightNodes := buildAutoTuneTree(rightSamples, depth+1, maxDepth, minSamplesLeaf, featureSampleCount, rng)

	leftOffset := 1
	rightOffset := 1 + len(leftNodes)

	root := DecisionNode{
		FeatureIndex: uint8(best.featureIdx),
		Threshold:    float32(best.threshold),
		LeftChild:    int16(leftOffset),
		RightChild:   int16(rightOffset),
		LeafValue:    0,
	}
	nodes := []DecisionNode{root}

	for i := range leftNodes {
		n := &leftNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(leftOffset)
			n.RightChild += int16(leftOffset)
		}
	}
	nodes = append(nodes, leftNodes...)

	for i := range rightNodes {
		n := &rightNodes[i]
		if !n.IsLeaf() {
			n.LeftChild += int16(rightOffset)
			n.RightChild += int16(rightOffset)
		}
	}
	nodes = append(nodes, rightNodes...)
	return nodes
}

