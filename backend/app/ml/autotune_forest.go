package ml

// Autotune forest construction shared by the auto-tuner (package app),
// the sweep harness and the model registry tests.

import (
	"errors"
	"math"
	"math/rand"
	"sort"
)

func PrepareAutoTuneSplit(labeled []TrainingSample, validationRatio float64) ([]TrainSample, []TrainSample, []TrainingSample, []TrainingSample, error) {
	if len(labeled) < 2 {
		return nil, nil, nil, nil, errors.New("need at least 2 labeled samples for tuning")
	}
	if validationRatio <= 0 || validationRatio >= 0.5 {
		validationRatio = 0.20
	}

	samples := make([]TrainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = TrainSample{features: s.Features, label: s.Label}
	}

	shuffledRaw := append([]TrainingSample(nil), labeled...)
	rand.Shuffle(len(samples), func(i, j int) {
		samples[i], samples[j] = samples[j], samples[i]
		shuffledRaw[i], shuffledRaw[j] = shuffledRaw[j], shuffledRaw[i]
	})

	validationCount := int(math.Round(float64(len(samples)) * validationRatio))
	if validationCount < 1 {
		validationCount = 1
	}
	if validationCount >= len(samples) {
		validationCount = len(samples) - 1
	}

	trainCount := len(samples) - validationCount
	trainSet := append([]TrainSample(nil), samples[:trainCount]...)
	validationSet := append([]TrainSample(nil), samples[trainCount:]...)
	trainRaw := append([]TrainingSample(nil), shuffledRaw[:trainCount]...)
	validationRaw := append([]TrainingSample(nil), shuffledRaw[trainCount:]...)
	return trainSet, validationSet, trainRaw, validationRaw, nil
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

	best := findAutoTuneBestSplit(samples, featureSampleCount, rng)
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

func findAutoTuneBestSplit(samples []TrainSample, featureSampleCount int, rng *rand.Rand) splitPoint {
	best := splitPoint{giniGain: -1}
	parentGini := giniImpurity(samples)

	features := make([]int, FeatureDim)
	for i := range features {
		features[i] = i
	}
	rng.Shuffle(len(features), func(i, j int) { features[i], features[j] = features[j], features[i] })
	if featureSampleCount > len(features) {
		featureSampleCount = len(features)
	}
	selectedFeatures := features[:featureSampleCount]

	for _, fi := range selectedFeatures {
		sorted := append([]TrainSample(nil), samples...)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].features[fi] < sorted[j].features[fi]
		})

		for i := 1; i < len(sorted); i++ {
			if sorted[i].features[fi] == sorted[i-1].features[fi] {
				continue
			}
			threshold := (sorted[i].features[fi] + sorted[i-1].features[fi]) / 2.0
			leftSamples := sorted[:i]
			rightSamples := sorted[i:]
			if len(leftSamples) < 1 || len(rightSamples) < 1 {
				continue
			}

			leftWeight := float64(len(leftSamples)) / float64(len(sorted))
			gain := parentGini - leftWeight*giniImpurity(leftSamples) - (1-leftWeight)*giniImpurity(rightSamples)
			if gain > best.giniGain {
				best = splitPoint{
					featureIdx: fi,
					threshold:  threshold,
					giniGain:   gain,
				}
			}
		}
	}

	return best
}
